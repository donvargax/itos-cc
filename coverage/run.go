package coverage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/project"
)

// Plan is how to measure one project: the directory to run in, the commands
// that produce coverage, and where the reports land. A project is one
// language under one build root (go.mod, package.json, pyproject.toml, a
// Gradle or Maven build).
type Plan struct {
	Language string
	Dir      string
	Sources  []string // the files it measures
	Commands [][]string
	Reports  []string // absolute
	// Existing are report locations to read with --use-existing-coverage,
	// in order of preference. Reports come first.
	Existing []string
	// Unsupported says why the commands cannot measure this project, such as
	// a test runner older than the one supported. Run reports it and runs
	// nothing; existing reports are still read.
	Unsupported string
	// CoverDir, when set, is the GOCOVERDIR the binaries the tests build
	// with go build -cover write to while the commands run. Run turns what
	// they wrote into the profile Integration names, read beside Reports,
	// and removes the directory.
	CoverDir    string
	Integration string
	// Unreached marks a plan of sources no test reaches (Reach): nothing
	// runs, and its language is measured, so they are loaded by no test.
	Unreached bool
	// OwnSources says the report speaks only for Sources: a Python or
	// Kotlin plan of the tests that reach them measures other files too,
	// which plans of their own tests measure.
	OwnSources bool
}

// Reach is the test files that reach a source, as graph.TestsImporting
// finds them.
type Reach func(source string) []string

// minVitest is the oldest Vitest major whose coverage is measured: the
// current one. Its v8 provider maps coverage onto the syntax tree, so its
// LCOV names both arms of every branch; older ones write V8 blocks.
const minVitest = 5

var markers = map[string][]string{
	"go":         {"go.mod"},
	"typescript": {"package.json"},
	"python":     {"pyproject.toml", "setup.py", "setup.cfg"},
	"kotlin":     {"build.gradle.kts", "build.gradle", "pom.xml"},
}

// Scope is which tests measure the sources.
type Scope int

const (
	// AllTests runs each build root's whole suite.
	AllTests Scope = iota
	// RelatedTests runs, in Go and TypeScript, only the tests that load the
	// sources, which measures them the same as the whole suite: a test that
	// does not load a file cannot cover it.
	RelatedTests
	// OwnTests measures each Go package by its own tests only, the tests
	// mutate kills its mutants with; TypeScript runs the related tests, and
	// Python and Kotlin, given the tests that reach each source, those
	// tests.
	OwnTests
)

// Plans groups sources by language and build root and returns one plan per
// group, with reports written under outDir. Python and Kotlin run the whole
// suite whatever the scope, but in OwnTests with reach set: each Python
// source is measured by the test files that reach it (PythonTests), and each
// Kotlin source by the test classes of its own build module those files
// declare (KotlinTests), one plan per build root and distinct set of those
// tests, its report speaking for its own sources alone (OwnSources), and
// sources no such test reaches have an Unreached plan. A Kotlin module's
// report measures its own tests alone, so a test class in another module
// that reaches a source measures none of it.
func Plans(sources []string, outDir string, scope Scope, reach Reach) []Plan {
	plans, _, _ := buildPlans(context.Background(), sources, outDir, scope, reach, nil)
	return plans
}

// PlansSupervised builds coverage plans while routing executable probes
// (Go package discovery and Python provider checks) through execute.
func PlansSupervised(ctx context.Context, sources []string, outDir string, scope Scope, reach Reach, execute CommandExecutor) ([]Plan, []CommandExecution, error) {
	if ctx == nil || execute == nil {
		return nil, nil, errors.New("supervised coverage planning needs a context and command executor")
	}
	return buildPlans(ctx, sources, outDir, scope, reach, execute)
}

func buildPlans(ctx context.Context, sources []string, outDir string, scope Scope, reach Reach, execute CommandExecutor) ([]Plan, []CommandExecution, error) {
	// tests is, for a Python or Kotlin source measured by the tests that
	// reach it, those tests as PythonTests runs them, or the test classes
	// of its own module KotlinTests finds in them, joined: a group of its
	// own.
	type key struct{ lang, dir, tests string }
	narrow := scope == OwnTests && reach != nil
	groups := map[key][]string{}
	for _, s := range sources {
		spec := lang.Detect(s)
		if spec == nil {
			continue
		}
		dir := lang.FindUp(s, markers[spec.Name]...)
		if dir == "" {
			dir = filepath.Dir(s)
		}
		k := key{spec.Name, dir, ""}
		switch {
		case spec.Name == "python" && narrow:
			k.tests = "\x00" + strings.Join(PythonTests(dir, reach(s)), "\x00")
		case spec.Name == "kotlin" && narrow:
			k.tests = "\x00" + strings.Join(KotlinTests(reach(s))[dir], "\x00")
		}
		groups[k] = append(groups[k], s)
	}
	var plans []Plan
	var executions []CommandExecution
	var failures []error
	for k, sources := range groups {
		if err := ctx.Err(); err != nil {
			failures = append(failures, err)
			break
		}
		srcs := sources
		if scope == AllTests {
			srcs = nil
		}
		out := filepath.Join(outDir, k.lang+"-"+shortHash(k.dir))
		var tests []string
		if k.tests != "" {
			out += "-" + shortHash(k.tests)
			if tests = strings.Split(k.tests, "\x00")[1:]; tests[0] == "" {
				plans = append(plans, Plan{Language: k.lang, Dir: k.dir, Sources: sources, Unreached: true})
				continue
			}
		}
		var p Plan
		switch k.lang {
		case "go":
			if execute == nil {
				p = goPlan(k.dir, out, srcs, scope == OwnTests)
			} else {
				pkgs, testing, calls, err := goScopeSupervised(ctx, k.dir, srcs, execute)
				executions = append(executions, calls...)
				if err != nil {
					failures = append(failures, err)
				}
				p = goPlanWithScope(k.dir, out, srcs, scope == OwnTests, pkgs, testing)
			}
		case "typescript":
			p = typescriptPlan(k.dir, out, srcs, scope == OwnTests)
		case "python":
			if execute == nil {
				p = pythonPlan(k.dir, out, tests)
			} else {
				var calls []CommandExecution
				var err error
				p, calls, err = pythonPlanSupervised(ctx, k.dir, out, tests, execute)
				executions = append(executions, calls...)
				if err != nil {
					failures = append(failures, err)
				}
			}
		case "kotlin":
			p = kotlinPlan(k.dir, tests)
		default:
			continue
		}
		p.Sources, p.OwnSources = sources, tests != nil
		plans = append(plans, p)
	}
	sort.Slice(plans, func(i, j int) bool {
		if a, b := plans[i].Language+plans[i].Dir, plans[j].Language+plans[j].Dir; a != b {
			return a < b
		}
		return plans[i].Sources[0] < plans[j].Sources[0]
	})
	return plans, executions, errors.Join(failures...)
}

// goPlan measures sources with the tests of every package whose test binary
// links one of theirs, or the whole module when sources is empty or go list
// cannot say. With own, each package is measured by its own tests alone.
func goPlan(dir, out string, sources []string, own bool) Plan {
	pkgs, testing := GoScope(dir, sources)
	return goPlanWithScope(dir, out, sources, own, pkgs, testing)
}

func goPlanWithScope(dir, out string, sources []string, own bool, pkgs, testing []string) Plan {
	report := filepath.Join(out, "coverage.out")
	args := []string{"go", "test", "-count=1", "-covermode=set", "-coverprofile=" + report}
	coverDir := ""
	if wrapper := execFlag(); wrapper != "" {
		// go test sets each test binary's GOCOVERDIR to a directory of its
		// own, and reads only the test binary's data from it, so a binary
		// a test builds with -cover would write where nothing reads it.
		// itos-cc runs each test binary itself, with GOCOVERDIR its own.
		coverDir = filepath.Join(out, "gocoverdir")
		args = append(args, "-exec", wrapper)
	}
	switch {
	case len(pkgs) > 0 && own:
		// Without -coverpkg each test binary measures its own package.
		args = append(args, pkgs...)
	case len(pkgs) > 0:
		args = append(append(args, "-coverpkg="+strings.Join(pkgs, ",")), testing...)
	default:
		args = append(args, "-coverpkg=./...", "./...")
	}
	return Plan{
		Language: "go",
		Dir:      dir,
		Commands: [][]string{args},
		Reports:  []string{report},
		Existing: []string{report, filepath.Join(dir, "coverage.out"), filepath.Join(dir, "cover.out")},
		CoverDir: coverDir,
		// Kept beside the report, so --use-existing-coverage reads it too.
		Integration: filepath.Join(out, "integration.out"),
	}
}

// GoScope is the packages of sources in the module at dir, and the packages
// whose test binaries link any of them, their own included: integration
// tests in another package that import the code count, and packages that
// never load it are not run. Packages without tests are listed too, so they
// measure as 0%. Both are empty when go list fails.
func GoScope(dir string, sources []string) (pkgs, testing []string) {
	if len(sources) == 0 {
		return nil, nil
	}
	cmd := exec.Command("go", "list", "-test", "-f", "{{.ImportPath}}\t{{.Dir}}\t{{join .Deps \" \"}}", "./...")
	cmd.Dir = dir
	listing, err := cmd.Output()
	if err != nil {
		return nil, nil
	}
	dirs := map[string]bool{}
	for _, s := range sources {
		dirs[filepath.Dir(filepath.Clean(s))] = true
	}
	type entry struct {
		path string
		deps []string
	}
	var binaries []entry
	selected := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(listing)), "\n") {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 3 {
			continue
		}
		path := fields[0]
		switch {
		case strings.HasSuffix(path, ".test"):
			binaries = append(binaries, entry{strings.TrimSuffix(path, ".test"), strings.Fields(fields[2])})
		case !strings.Contains(path, " ") && dirs[filepath.Clean(fields[1])]:
			selected[path] = true
		}
	}
	tested := map[string]bool{}
	for p := range selected {
		tested[p] = true
	}
	for _, b := range binaries {
		for _, d := range b.deps {
			// A package recompiled for a test reads "p [q.test]".
			if d, _, _ := strings.Cut(d, " "); selected[d] {
				tested[b.path] = true
				break
			}
		}
	}
	for p := range selected {
		pkgs = append(pkgs, p)
	}
	for p := range tested {
		testing = append(testing, p)
	}
	sort.Strings(pkgs)
	sort.Strings(testing)
	return pkgs, testing
}

// goScopeSupervised is the fail-closed scope probe of opt-in preparation. It
// runs go list through execute and refuses a failed, empty or malformed
// listing, or one that omits an admitted source directory, instead of
// falling back to the whole module.
//
// It parses each line whole, so a package with no dependencies keeps its
// empty third field wherever it falls in the listing. GoScope above stays
// byte-for-byte the legacy parser for complete runs: it trims the listing's
// trailing whitespace, which drops a dependency-free package that go list
// prints last, and strict complete runs depend on what that selects (see
// TestStrictGoReportsExecutableFunctionInUnloadedPackageAsMissing).
func goScopeSupervised(ctx context.Context, dir string, sources []string, execute CommandExecutor) (pkgs, testing []string, executions []CommandExecution, resultErr error) {
	if len(sources) == 0 {
		return nil, nil, nil, nil
	}
	cmd := exec.CommandContext(ctx, "go", "list", "-test", "-f", "{{.ImportPath}}\t{{.Dir}}\t{{join .Deps \" \"}}", "./...")
	cmd.Dir = dir
	var listing bytes.Buffer
	cmd.Stdout = &listing
	cmd.Stderr = io.Discard
	err := execute(ctx, cmd)
	executions = append(executions, CommandExecution{Args: append([]string{}, cmd.Args...), Dir: dir, Err: err})
	if err != nil {
		return nil, nil, executions, fmt.Errorf("go list coverage scope in %s: %w", dir, err)
	}
	type listed struct{ path, dir, deps string }
	var entries []listed
	for _, line := range strings.Split(listing.String(), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 || fields[0] == "" || fields[1] == "" || filepath.Clean(fields[1]) == "." {
			return nil, nil, executions, fmt.Errorf("malformed go list coverage scope entry %q", line)
		}
		entries = append(entries, listed{fields[0], filepath.Clean(fields[1]), fields[2]})
	}
	if len(entries) == 0 {
		return nil, nil, executions, fmt.Errorf("go list returned an empty coverage scope in %s", dir)
	}
	dirs := map[string]bool{}
	for _, s := range sources {
		dirs[filepath.Dir(filepath.Clean(s))] = true
	}
	type binary struct {
		path string
		deps []string
	}
	var binaries []binary
	selected := map[string]bool{}
	listedDirs := map[string]bool{}
	for _, e := range entries {
		switch {
		case strings.HasSuffix(e.path, ".test"):
			binaries = append(binaries, binary{strings.TrimSuffix(e.path, ".test"), strings.Fields(e.deps)})
		case !strings.Contains(e.path, " "):
			listedDirs[e.dir] = true
			if dirs[e.dir] {
				selected[e.path] = true
			}
		}
	}
	for sourceDir := range dirs {
		if !listedDirs[sourceDir] {
			return nil, nil, executions, fmt.Errorf("go list omitted admitted source directory %s", sourceDir)
		}
	}
	tested := map[string]bool{}
	for p := range selected {
		tested[p] = true
	}
	for _, b := range binaries {
		for _, d := range b.deps {
			// A package recompiled for a test reads "p [q.test]".
			if d, _, _ := strings.Cut(d, " "); selected[d] {
				tested[b.path] = true
				break
			}
		}
	}
	for p := range selected {
		pkgs = append(pkgs, p)
	}
	for p := range tested {
		testing = append(testing, p)
	}
	sort.Strings(pkgs)
	sort.Strings(testing)
	return pkgs, testing, executions, nil
}

// typescriptPlan measures sources with the tests that import them, through
// Vitest's related or Jest's findRelatedTests, or with every test when
// sources is empty. A coverage script or c8 runs the whole suite. With own,
// the scope mutation run judges mutants in, a project with Vitest or Jest
// is measured by the same related tests that judge its mutants, never by
// its coverage script: a line only an unrelated test executes is then
// uncovered, not a survivor no test it runs could kill.
func typescriptPlan(dir, out string, sources []string, own bool) Plan {
	report := filepath.Join(out, "lcov.info")
	plan := Plan{
		Language: "typescript",
		Dir:      dir,
		Reports:  []string{report},
		Existing: []string{report, filepath.Join(dir, "coverage", "lcov.info")},
	}
	pkg := readPackageJSON(dir)
	pm := project.PackageManager(dir)
	// Tools run from node_modules: a tool the project did not install is
	// never downloaded, since that would run code no lockfile pins.
	missing := func(what string) string {
		return fmt.Sprintf("%s is not installed; run %s install, or measure with --coverage-command", what, pm)
	}
	related := own && (pkg.has("vitest") || pkg.has("jest"))
	switch {
	case pkg.Scripts["coverage"] != "" && !related:
		// The project's own script decides where it writes; the usual place
		// is coverage/lcov.info.
		plan.Commands = [][]string{{pm, "run", "coverage"}}
		plan.Reports = []string{filepath.Join(dir, "coverage", "lcov.info")}
	case pkg.has("vitest"):
		version := installedVersion(dir, "vitest")
		vitest := project.NodeBin(dir, "vitest")
		switch {
		case vitest == "":
			plan.Unsupported = missing("Vitest")
		case vitestTooOld(version):
			plan.Unsupported = fmt.Sprintf("Vitest %s is not supported; upgrade to Vitest %d or later to measure coverage",
				version, minVitest)
		case project.NodeModule(dir, "@vitest/coverage-v8") == "":
			// Without the provider Vitest stops to ask.
			plan.Unsupported = fmt.Sprintf("@vitest/coverage-v8 is not installed; add @vitest/coverage-v8@%s to devDependencies",
				version)
		default:
			args := []string{vitest, "run"}
			if len(sources) > 0 {
				args = append([]string{vitest, "related", "--run"}, relativeTo(dir, sources)...)
			}
			plan.Commands = [][]string{append(args, "--coverage.enabled",
				"--coverage.reporter=lcov", "--coverage.reportsDirectory="+out)}
		}
	case pkg.has("jest"):
		if jest := project.NodeBin(dir, "jest"); jest == "" {
			plan.Unsupported = missing("Jest")
		} else {
			args := []string{jest, "--coverage", "--coverageReporters=lcov", "--coverageDirectory=" + out}
			if len(sources) > 0 {
				args = append(append(args, "--findRelatedTests"), relativeTo(dir, sources)...)
			}
			plan.Commands = [][]string{args}
		}
	default:
		if c8 := project.NodeBin(dir, "c8"); c8 == "" {
			plan.Unsupported = "the project has neither Vitest nor Jest, and c8 is not installed; " +
				"add c8 to devDependencies, or measure with --coverage-command"
		} else {
			plan.Commands = [][]string{{c8, "--reporter=lcov", "--reports-dir=" + out, pm, "run", "test"}}
		}
	}
	return plan
}

// pytestRunner is how a coverage plan runs pytest: without its cache
// plugin, as every pytest command itos-cc composes runs, so measuring writes
// no .pytest_cache into the project. Plugins it autoloads stay the
// project's.
var pytestRunner = []string{"-m", "pytest", "-q", "-p", "no:cacheprovider"}

// pythonRunner is the runner arguments of a Python coverage plan: pytest's
// or unittest's, over tests, the test files PythonTests gives, or over the
// whole suite when tests is nil.
func pythonRunner(pytest bool, tests []string) []string {
	switch {
	case pytest:
		return append(slices.Clone(pytestRunner), tests...)
	case tests != nil:
		return append([]string{"-m", "unittest"}, PythonModules(tests)...)
	}
	return []string{"-m", "unittest", "discover"}
}

// PythonTests is which of tests, the test files that reach a source, a
// Python own-test run in dir passes its runner: those inside dir named as
// pytest collects test files by default, test_*.py or *_test.py, relative
// to dir with forward slashes, sorted. conftest.py, __init__.py and other
// helpers are left out: pytest applies conftest.py files on its own, and a
// file it collects no test from would fail a run of it alone.
func PythonTests(dir string, tests []string) []string {
	out := []string{}
	for _, test := range tests {
		if !PythonTestFile(test) {
			continue
		}
		rel, err := filepath.Rel(dir, test)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			continue
		}
		if rel = filepath.ToSlash(rel); !slices.Contains(out, rel) {
			out = append(out, rel)
		}
	}
	sort.Strings(out)
	return out
}

// PythonTestFile says whether the file at path is named as pytest collects
// test files by default, test_*.py or *_test.py: a runnable test, not a
// conftest.py or a helper.
func PythonTestFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, ".py") && (strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py"))
}

// PythonModules is tests, test files relative to the directory the
// command runs in as PythonTests gives them, as the dotted modules
// python -m unittest imports: relative to src for one beneath it, which is
// on PYTHONPATH as the project's own sources are, else relative to the
// directory. A path no module name spells is left out.
func PythonModules(tests []string) []string {
	var out []string
	for _, test := range tests {
		name := strings.TrimSuffix(strings.TrimPrefix(test, "src/"), ".py")
		parts := strings.Split(name, "/")
		valid := true
		for _, part := range parts {
			valid = valid && pythonIdentifier(part)
		}
		if valid {
			out = append(out, strings.Join(parts, "."))
		}
	}
	return out
}

func pythonIdentifier(s string) bool {
	for i, r := range s {
		if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
			return false
		}
	}
	return s != ""
}

// pythonPlan measures dir's Python sources with tests, the test files that
// reach them as PythonTests gives them, or with the whole suite when tests
// is nil.
func pythonPlan(dir, out string, tests []string) Plan {
	py := pythonFor(dir)
	data := filepath.Join(out, ".coverage")
	report := filepath.Join(out, "lcov.info")
	runner := pythonRunner(exec.Command(py, "-c", "import pytest").Run() == nil, tests)
	plan := Plan{
		Language: "python",
		Dir:      dir,
		Reports:  []string{report},
		Existing: []string{report, filepath.Join(dir, "lcov.info"), filepath.Join(dir, "coverage", "lcov.info")},
	}
	if exec.Command(py, "-c", "import coverage").Run() != nil {
		plan.Unsupported = fmt.Sprintf("coverage.py is not installed for %s; install it there, or measure with --coverage-command", py)
		return plan
	}
	run := append([]string{py, "-m", "coverage", "run", "--branch", "--data-file=" + data, "--source=" + dir}, runner...)
	plan.Commands = [][]string{run, {py, "-m", "coverage", "lcov", "--data-file=" + data, "-o", report}}
	return plan
}

func pythonPlanSupervised(ctx context.Context, dir, out string, tests []string, execute CommandExecutor) (Plan, []CommandExecution, error) {
	py := pythonFor(dir)
	var executions []CommandExecution
	probe := func(module string) error {
		cmd := exec.CommandContext(ctx, py, "-c", "import "+module)
		cmd.Dir, cmd.Stdout, cmd.Stderr = dir, io.Discard, io.Discard
		err := execute(ctx, cmd)
		executions = append(executions, CommandExecution{Args: append([]string{}, cmd.Args...), Dir: dir, Err: err})
		return err
	}
	pytestErr := probe("pytest")
	if pytestErr != nil && ctx.Err() != nil {
		return Plan{}, executions, ctx.Err()
	}
	runner := pythonRunner(pytestErr == nil, tests)
	data := filepath.Join(out, ".coverage")
	report := filepath.Join(out, "lcov.info")
	plan := Plan{
		Language: "python", Dir: dir,
		Reports:  []string{report},
		Existing: []string{report, filepath.Join(dir, "lcov.info"), filepath.Join(dir, "coverage", "lcov.info")},
	}
	if err := probe("coverage"); err != nil {
		if ctx.Err() != nil {
			return Plan{}, executions, ctx.Err()
		}
		plan.Unsupported = fmt.Sprintf("coverage.py is not installed for %s; install it there, or measure with --coverage-command", py)
		return plan, executions, nil
	}
	plan.Commands = [][]string{
		append([]string{py, "-m", "coverage", "run", "--branch", "--data-file=" + data, "--source=" + dir}, runner...),
		{py, "-m", "coverage", "lcov", "--data-file=" + data, "-o", report},
	}
	return plan, executions, nil
}

// kotlinPlan measures the Kotlin module at dir with its JaCoCo or Kover,
// running classes, the test classes KotlinTests gives, or the whole suite
// when classes is nil.
func kotlinPlan(dir string, classes []string) Plan {
	kover := filepath.Join(dir, "build", "reports", "kover", "report.xml")
	jacoco := filepath.Join(dir, "build", "reports", "jacoco", "test", "jacocoTestReport.xml")
	maven := filepath.Join(dir, "target", "site", "jacoco", "jacoco.xml")
	plan := Plan{Language: "kotlin", Dir: dir, Existing: []string{kover, jacoco, maven}}
	if exists(filepath.Join(dir, "pom.xml")) {
		// The project's own JaCoCo, at the version it declares: naming the
		// plugin here would have Maven download one the project never chose.
		if !pomMentions(dir, "jacoco-maven-plugin") {
			plan.Unsupported = "jacoco-maven-plugin is not in pom.xml or a parent's; add it, or measure with --coverage-command"
			return plan
		}
		plan.Commands = [][]string{append([]string{"mvn", "-q", "jacoco:prepare-agent", "test", "jacoco:report"}, MavenTests(classes)...)}
		plan.Reports = []string{maven}
		return plan
	}
	gradle := "gradle"
	if root := lang.FindUp(filepath.Join(dir, "x"), "gradlew"); root != "" {
		gradle = filepath.Join(root, "gradlew")
	}
	// The report task runs the test task, which the filter then narrows.
	task := "jacocoTestReport"
	plan.Reports = []string{jacoco}
	if buildMentions(dir, "kover") {
		task = "koverXmlReport"
		plan.Reports = []string{kover}
	}
	args := []string{gradle, "-p", dir}
	if classes != nil {
		args = append(append(args, "test"), GradleTests(classes)...)
	} else if task == "jacocoTestReport" {
		args = append(args, "test")
	}
	plan.Commands = [][]string{append(args, task)}
	return plan
}

// GradleTests is the --tests filter of a Gradle test task that runs
// classes, test classes by fully qualified name.
func GradleTests(classes []string) []string {
	var out []string
	for _, class := range classes {
		out = append(out, "--tests", class)
	}
	return out
}

// MavenTests is the properties that have Surefire run classes, test classes
// by fully qualified name, in each module of a build that holds any of
// them, and none in one that holds none; nothing when classes is nil.
func MavenTests(classes []string) []string {
	if classes == nil {
		return nil
	}
	return []string{"-Dtest=" + strings.Join(classes, ","), "-Dsurefire.failIfNoSpecifiedTests=false"}
}

// Run executes each plan and returns the coverage of sources. A plan whose
// commands fail still contributes any report it wrote, because failing tests
// still measure the code they ran; one that measured none of its files is
// Missing. A plan whose commands all succeed and write their reports
// Measures its language, and is never Missing: its files the report never
// names are untested, since no test loaded them. Problems are written to
// log.
func Run(plans []Plan, sources []string, log io.Writer) *Report {
	var reports []*Report
	for _, p := range plans {
		if p.Unreached {
			reports = append(reports, p.unreached(log))
			continue
		}
		if p.Unsupported != "" {
			fmt.Fprintf(log, "itos-cc: coverage: %s: %s\n", p.Dir, p.Unsupported)
			reports = append(reports, &Report{missing: []Unmeasured{{Dir: p.Dir, Language: p.Language, Cause: ToolMissing, Reason: p.Unsupported}}})
			continue
		}
		for _, r := range append(p.Reports, p.Integration) {
			os.Remove(r)
		}
		os.MkdirAll(filepath.Dir(p.Reports[0]), 0o755)
		env := project.NoBytecodeEnv(os.Environ())
		if p.CoverDir != "" && os.MkdirAll(p.CoverDir, 0o755) == nil {
			env = append(env, coverDirEnv+"="+p.CoverDir)
		}
		failed := ""
		for _, args := range p.Commands {
			fmt.Fprintf(log, "itos-cc: coverage %s$ %s\n", p.Dir, strings.Join(args, " "))
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Dir = p.Dir
			cmd.Env = env
			cmd.Stdout = log
			cmd.Stderr = log
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(log, "itos-cc: coverage: %s: %v\n", p.Language, err)
				failed = fmt.Sprintf("; %s: %v", args[0], err)
			}
		}
		r := load(p.Reports, p.integrate(log), p.Dir, p.measures(sources), log)
		// Told by exit status and the report alone, never by what the
		// runner prints: a run that succeeded and wrote its report measured
		// its language, whichever files the report names.
		if failed == "" && len(r.missing) == 0 {
			r.languages = map[string]bool{p.Language: true}
			p.prove(r)
			reports = append(reports, r)
			continue
		}
		reports = append(reports, p.measured(r, MeasuredNothing, "its coverage run measured none of its files"+failed))
	}
	return Merge(reports...)
}

// unreached is the report of an Unreached plan: its language measured, and
// none of its sources loaded, as no test reaches them.
func (p Plan) unreached(log io.Writer) *Report {
	fmt.Fprintf(log, "itos-cc: coverage %s: no test reaches %s, so none runs\n", p.Dir, strings.Join(relativeTo(p.Dir, p.Sources), " "))
	r := &Report{files: map[string][]Segment{}, branches: map[string][]Segment{}, languages: map[string]bool{p.Language: true}}
	p.prove(r)
	return r
}

// prove records, of r, the report of p's commands that all succeeded and
// wrote their reports, which of p's sources it lists completely, at its
// format's line precision (Report.Lines), and which no test loaded, so it
// names none of their lines (Report.Unloaded).
func (p Plan) prove(r *Report) {
	r.proven, r.unloaded = map[string]bool{}, map[string]Unloaded{}
	for _, s := range p.Sources {
		if r.Has(s) {
			r.proven[s] = true
		} else {
			r.unloaded[s] = Unloaded{Language: p.Language, Dir: p.Dir}
		}
	}
}

// measures is the sources p's report speaks for, of sources: its own
// Sources when it measures them alone (OwnSources).
func (p Plan) measures(sources []string) []string {
	if p.OwnSources {
		return p.Sources
	}
	return sources
}

// measured is r, Missing p when r measured none of p's files: the run
// failed, wrote an empty report, or loaded none of them. A report that
// measured some of them leaves the rest untested, not missing.
func (p Plan) measured(r *Report, cause Cause, why string) *Report {
	for _, s := range p.Sources {
		if r.Has(s) {
			r.missing = nil
			return r
		}
	}
	if len(p.Sources) > 0 {
		r.missing = []Unmeasured{{Dir: p.Dir, Language: p.Language, Cause: cause, Reason: why}}
	}
	return r
}

// IgnoreDir creates dir with a .gitignore that ignores everything in it, so
// raw reports stay out of version control while the rest of .metrics is
// committed.
func IgnoreDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*\n"), 0o644)
}

// Existing reads reports already on disk, the first one found per plan.
func Existing(plans []Plan, sources []string, log io.Writer) *Report {
	var reports []*Report
	for _, p := range plans {
		if p.Unreached {
			reports = append(reports, p.unreached(log))
			continue
		}
		r := &Report{}
		for _, path := range p.Existing {
			if exists(path) {
				// A run's integration profile goes with its report.
				integration := ""
				if path == p.Reports[0] && p.Integration != "" && exists(p.Integration) {
					integration = p.Integration
				}
				r = load([]string{path}, integration, p.Dir, p.measures(sources), log)
				break
			}
		}
		reports = append(reports, p.measured(r, NoReport, "no report on disk measures its files"))
	}
	return Merge(reports...)
}

// Files reads reports named on the command line, relative to the working
// directory.
func Files(paths []string, sources []string, log io.Writer) *Report {
	wd, _ := os.Getwd()
	return load(paths, "", wd, sources, log)
}

// load reads the reports at paths; each that cannot be read is Missing. The
// profile at integration, unless it is "", is read beside them as
// Integration data; one that cannot be read is only logged.
func load(paths []string, integration, base string, sources []string, log io.Writer) *Report {
	var all [][]Entry
	var missing []Unmeasured
	for _, path := range paths {
		entries, err := Load(path)
		if err != nil {
			fmt.Fprintf(log, "itos-cc: coverage: %v\n", err)
			missing = append(missing, Unmeasured{Report: path, Cause: Unreadable, Reason: err.Error()})
			continue
		}
		all = append(all, entries)
	}
	if integration != "" {
		entries, err := Load(integration)
		if err != nil {
			fmt.Fprintf(log, "itos-cc: coverage: %v\n", err)
		}
		for i := range entries {
			entries[i].Source = Integration
		}
		all = append(all, entries)
	}
	r := Build(sources, base, all...)
	r.missing = missing
	return r
}

type packageJSON struct {
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

func (p packageJSON) has(dep string) bool {
	return p.Dependencies[dep] != "" || p.DevDependencies[dep] != ""
}

func readPackageJSON(dir string) packageJSON {
	var pkg packageJSON
	if data, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
		json.Unmarshal(data, &pkg)
	}
	return pkg
}

// vitestTooOld reports whether version, as installed, is a major before
// minVitest. An unknown version is not too old: running it will tell.
func vitestTooOld(version string) bool {
	major, _, _ := strings.Cut(version, ".")
	n, err := strconv.Atoi(major)
	return err == nil && n < minVitest
}

func installedVersion(dir, dep string) string {
	var pkg struct {
		Version string `json:"version"`
	}
	if module := project.NodeModule(dir, dep); module != "" {
		if data, err := os.ReadFile(filepath.Join(module, "package.json")); err == nil {
			json.Unmarshal(data, &pkg)
		}
	}
	return pkg.Version
}

// pythonFor prefers the project's own virtualenv, so tests run with the
// project's dependencies.
func pythonFor(dir string) string {
	for _, venv := range []string{".venv", "venv"} {
		py := filepath.Join(dir, venv, "bin", "python")
		if exists(py) {
			return py
		}
	}
	return "python3"
}

func buildMentions(dir, word string) bool {
	for _, name := range []string{"build.gradle.kts", "build.gradle"} {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil && strings.Contains(string(data), word) {
			return true
		}
	}
	return false
}

// relativeTo is paths relative to dir, with forward slashes.
func relativeTo(dir string, paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			rel = p
		}
		out[i] = filepath.ToSlash(rel)
	}
	return out
}

// pomMentions reports whether the pom.xml in dir, or one in a directory
// above it, such as a parent's, mentions word.
func pomMentions(dir, word string) bool {
	for d := dir; ; d = filepath.Dir(d) {
		data, err := os.ReadFile(filepath.Join(d, "pom.xml"))
		if err != nil {
			return false
		}
		if strings.Contains(string(data), word) {
			return true
		}
		if filepath.Dir(d) == d {
			return false
		}
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:4])
}
