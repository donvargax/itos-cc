package covrun

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

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/coverage/port"
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/project"
)

// Plan is how to measure one project: the directory to run in, the commands
// that produce coverage, and where the reports land. A project is one
// language under one build root (go.mod, package.json, pyproject.toml, a
// Gradle or Maven build, pubspec.yaml).
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
	// CoverDir, when set, is where the processes the tests start write
	// their coverage data while the commands run (collector.go): the
	// GOCOVERDIR of the binaries the tests build with go build -cover, the
	// data files of the Python processes coverage.py starts in, the .exec
	// files of the JVMs a Kotlin harness starts with the JaCoCo agent. Prepare,
	// when set, runs first to ready the collector, and exits 3 when it
	// cannot measure them, printing why: the commands then run without it
	// (Report.IntegrationMissing). CoverEnv is the environment the commands
	// get so those processes write there. When Written, a glob in CoverDir,
	// finds data once they ran, Convert turns it into the report Integration
	// names, read beside Reports as Integration data. CoverDir is removed.
	CoverDir string
	// CollectorMissing, when set, says why the project lacks the collector
	// of the processes its tests start, such as c8: the commands run
	// without it (Report.IntegrationMissing).
	CollectorMissing string
	// RunnerScripts are what a script URL of the test runner's own
	// processes holds, such as /node_modules/vitest/: the V8 coverage a
	// process that loaded one wrote is the runner's, not integration data,
	// and is dropped before Convert.
	RunnerScripts []string
	Prepare       []string
	CoverEnv      []string
	Written       string
	Convert       [][]string
	// ConvertWritten, when set, gives the Convert commands from the files
	// Written found, sorted, such as the .exec files jacococli merges.
	ConvertWritten func(written []string) [][]string
	// ConverterMissing, when set, says why what was written cannot be
	// turned into a report, such as no jacococli: data written is then a
	// missing collector (Report.IntegrationMissing), and none is nothing.
	ConverterMissing string
	Integration      string
	// Notice, when set, is logged before the commands run, such as that no
	// JaCoCo agent is cached, so the opt-in variable naming it is unset.
	Notice string
	// Unreached marks a plan of sources no test reaches (Reach): nothing
	// runs, and its language is measured, so they are loaded by no test.
	Unreached bool
	// OwnSources says the report speaks only for Sources: a Python or
	// Kotlin plan of the tests that reach them measures other files too,
	// which plans of their own tests measure.
	OwnSources bool
	// Unattested marks a plan whose command is the project's own, such as
	// a TypeScript coverage script: its report counts as any report does,
	// but proves no file's executable lines (Report.Lines), as no built-in
	// command made it.
	Unattested bool
	// NotCoverable are Sources a pragma makes not coverable at all, such
	// as Dart's coverage:ignore-file, which the tool honours by naming them
	// nowhere, loaded or not: a report its commands wrote then names each
	// with no line, so it has no coverage, rather than the none of a file
	// no test loaded.
	NotCoverable []string
}

// Reach is the test files that reach a source: the coverage port's, which
// a request carries (decision 47).
type Reach = port.Reach

// minVitest is the oldest Vitest major whose coverage is measured: the
// current one. Its v8 provider maps coverage onto the syntax tree, so its
// LCOV names both arms of every branch; older ones write V8 blocks.
const minVitest = 5

// markers are the files that make a folder a build root of each language,
// the one table of them: the in-process scenario driver reads it too
// (Markers), so a new language edits it alone.
var markers = map[string][]string{
	"go":         {"go.mod"},
	"typescript": {"package.json"},
	"python":     {"pyproject.toml", "setup.py", "setup.cfg"},
	"kotlin":     {"build.gradle.kts", "build.gradle", "pom.xml"},
	"dart":       {"pubspec.yaml"},
}

// Markers are the files that make a folder a build root of language, as the
// runner finds its roots, a copy the caller may change; nil for a language
// it measures none of.
func Markers(language string) []string {
	return slices.Clone(markers[language])
}

// Scope is which tests measure the sources: the coverage port's, which a
// request carries (decision 47), each named here as there.
type Scope = port.Scope

const (
	// AllTests runs each build root's whole suite.
	AllTests = port.AllTests
	// RelatedTests runs, in Go and TypeScript, only the tests that load the
	// sources.
	RelatedTests = port.RelatedTests
	// OwnTests measures each Go package by its own tests only, the tests
	// mutate kills its mutants with.
	OwnTests = port.OwnTests
)

// Plans groups sources by language and build root and returns one plan per
// group, with reports written under outDir. Python, Kotlin and Dart run
// the whole suite whatever the scope, but in OwnTests with reach set: each Python
// source is measured by the test files that reach it (PythonTests), each
// Dart source by those of them flutter test runs (DartTests), and each
// Kotlin source by the test classes of its own build module those files
// declare (KotlinTests), one plan per build root and distinct set of those
// tests, its report speaking for its own sources alone (OwnSources), and
// sources no such test reaches have an Unreached plan. A Kotlin module's
// report measures its own tests alone, so a test class in another module
// that reaches a source measures none of it.
func Plans(ctx context.Context, sources []string, outDir string, scope Scope, reach Reach) []Plan {
	plans, _, _ := buildPlans(ctx, sources, outDir, scope, reach, nil)
	return plans
}

// PlansSupervised builds coverage plans while routing executable probes
// (Go package discovery and Python provider checks) through execute.
func PlansSupervised(ctx context.Context, sources []string, outDir string, scope Scope, reach Reach, execute CommandExecutor) ([]Plan, []CommandExecution, error) {
	if execute == nil {
		return nil, nil, errors.New("supervised coverage planning needs a command executor")
	}
	return buildPlans(ctx, sources, outDir, scope, reach, execute)
}

func buildPlans(ctx context.Context, sources []string, outDir string, scope Scope, reach Reach, execute CommandExecutor) ([]Plan, []CommandExecution, error) {
	// tests is, for a Python, Dart or Kotlin source measured by the tests
	// that reach it, those tests as PythonTests or DartTests runs them, or
	// the test classes of its own module KotlinTests finds in them, joined:
	// a group of its own.
	type key struct{ lang, dir, tests string }
	narrow := scope == OwnTests && reach != nil
	groups := map[key][]string{}
	for _, s := range sources {
		spec := lang.Detect(s)
		if spec == nil {
			continue
		}
		dir := project.FindUp(ctx, s, markers[spec.Name]...)
		if dir == "" {
			dir = filepath.Dir(s)
		}
		k := key{spec.Name, dir, ""}
		switch {
		case spec.Name == "python" && narrow:
			k.tests = "\x00" + strings.Join(PythonTests(dir, reach(s)), "\x00")
		case spec.Name == "kotlin" && narrow:
			k.tests = "\x00" + strings.Join(KotlinTests(ctx, reach(s))[dir], "\x00")
		case spec.Name == "dart" && narrow:
			k.tests = "\x00" + strings.Join(DartTests(dir, reach(s)), "\x00")
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
			p = kotlinPlan(ctx, k.dir, out, tests)
		case "dart":
			p = dartPlan(ctx, k.dir, out, sources, tests)
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

// TypeScriptRunner is what measures the coverage of the TypeScript
// package at dir, its sources measured by their related tests with own, or
// by the whole suite: "script", the project's own coverage script, where
// it has one and no related tests are run; "vitest" where Vitest runs its
// tests (UsesVitest), "jest" where it declares Jest; "c8" over its test
// script otherwise.
func TypeScriptRunner(dir string, own bool) string {
	pkg := readPackageJSON(dir)
	vitest := UsesVitest(dir)
	related := own && (vitest || pkg.has("jest"))
	switch {
	case pkg.Scripts["coverage"] != "" && !related:
		return "script"
	case vitest:
		return "vitest"
	case pkg.has("jest"):
		return "jest"
	}
	return "c8"
}

// UsesVitest is whether Vitest runs the tests of the TypeScript package at
// dir: it declares vitest, or a vitest resolves from its node_modules/.bin
// and its test script or a config beside its package.json says Vitest runs
// them (coverage.TypeScriptPackage), as mutation's test command decides.
func UsesVitest(dir string) bool {
	pkg := readPackageJSON(dir)
	p := coverage.TypeScriptPackage{
		Vitest: pkg.has("vitest"), Jest: pkg.has("jest"),
		VitestBin: project.NodeBin(dir, "vitest") != "", TestScript: pkg.Scripts["test"],
	}
	for _, name := range coverage.ViteConfigs {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			p.Config = true
			break
		}
	}
	return p.UsesVitest()
}

// goPlan measures sources with the tests of every package whose test binary
// links one of theirs, or the whole module when sources is empty or go list
// cannot say, its Notice then saying why. With own, each package is
// measured by its own tests alone.
func goPlan(dir, out string, sources []string, own bool) Plan {
	pkgs, testing, why := GoScope(dir, sources)
	plan := goPlanWithScope(dir, out, sources, own, pkgs, testing)
	if why != "" {
		plan.Notice = "measuring the whole module, as " + why
	}
	return plan
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
	plan := Plan{
		Language: "go",
		Dir:      dir,
		Commands: [][]string{args},
		Reports:  []string{report},
		Existing: []string{report, filepath.Join(dir, "coverage.out"), filepath.Join(dir, "cover.out")},
		// Kept beside the report, so --use-existing-coverage reads it too.
		Integration: filepath.Join(out, "integration.out"),
	}
	if coverDir != "" {
		plan.CoverDir, plan.CoverEnv, plan.Written = coverDir, []string{coverDirEnv + "=" + coverDir}, "covmeta.*"
		plan.Convert = [][]string{{"go", "tool", "covdata", "textfmt", "-i=" + coverDir, "-o=" + plan.Integration}}
	}
	return plan
}

// GoScope is the packages of sources in the module at dir, and the packages
// whose test binaries link any of them, their own included: integration
// tests in another package that import the code count, and packages that
// never load it are not run. Packages without tests are listed too, so they
// measure as 0%. Both are empty when go list fails, or lists no package in
// the folder of any source, and why then says which: go list failed, with
// the first line it wrote to stderr, or its exit status when it wrote none;
// or it lists no package in the first source's folder, and the first it
// lists is in another, so a folder named two ways shows at once (bug-10).
func GoScope(dir string, sources []string) (pkgs, testing []string, why string) {
	if len(sources) == 0 {
		return nil, nil, ""
	}
	cmd := exec.Command("go", "list", "-test", "-f", "{{.ImportPath}}\t{{.Dir}}\t{{join .Deps \" \"}}", "./...")
	cmd.Dir, cmd.Env = dir, project.InheritedEnvIn(dir)
	listing, err := cmd.Output()
	if err != nil {
		return nil, nil, "go list failed: " + firstStderrLine(err)
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
	first := "" // the folder of the first package listed
	for _, line := range strings.Split(strings.TrimSpace(string(listing)), "\n") {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 3 {
			continue
		}
		path := fields[0]
		switch {
		case strings.HasSuffix(path, ".test"):
			binaries = append(binaries, entry{strings.TrimSuffix(path, ".test"), strings.Fields(fields[2])})
		case !strings.Contains(path, " "):
			if first == "" {
				first = fields[1]
			}
			if dirs[filepath.Clean(fields[1])] {
				selected[path] = true
			}
		}
	}
	if len(selected) == 0 {
		why = "go list lists no package in " + filepath.Dir(filepath.Clean(sources[0]))
		if first != "" {
			why += "; the first it lists is in " + first
		}
		return nil, nil, why
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
	return pkgs, testing, ""
}

// firstStderrLine is the first line a failed command wrote to stderr, or
// the error itself, such as its exit status, when it wrote none.
func firstStderrLine(err error) string {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		for _, line := range strings.Split(string(exit.Stderr), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				return line
			}
		}
	}
	return err.Error()
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
	cmd.Dir, cmd.Env = dir, project.InheritedEnvIn(dir)
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

// vitestNoFloor sets the project's coverage thresholds to nothing, which the
// command line does over the config: itos-cc measures, and the project's
// own test script enforces its floor, which a run of one file's tests, every
// other file at 0%, would miss. autoUpdate is off, or Vitest would write
// what one file's tests measured into the project's config.
var vitestNoFloor = []string{"--coverage.thresholds.lines=0", "--coverage.thresholds.functions=0",
	"--coverage.thresholds.branches=0", "--coverage.thresholds.statements=0",
	"--coverage.thresholds.100=false", "--coverage.thresholds.autoUpdate=false"}

// jestNoFloor replaces the project's whole coverageThreshold, its path and
// glob keys included, with none, as vitestNoFloor does Vitest's.
const jestNoFloor = "--coverageThreshold={}"

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
	pm := project.PackageManager(dir)
	// Tools run from node_modules: a tool the project did not install is
	// never downloaded, since that would run code no lockfile pins.
	missing := func(what string) string {
		return fmt.Sprintf("%s is not installed; run %s install, or measure with --coverage-command", what, pm)
	}
	switch TypeScriptRunner(dir, own) {
	case "script":
		// The project's own script decides where it writes; the usual place
		// is coverage/lcov.info. No built-in command attests what it
		// measured, so its report proves no file's lines (Unattested).
		plan.Commands = [][]string{{pm, "run", "coverage"}}
		plan.Reports = []string{filepath.Join(dir, "coverage", "lcov.info")}
		plan.Unattested = true
	case "vitest":
		vitest := project.NodeBin(dir, "vitest")
		version := vitestVersion(dir, vitest)
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
			plan.Commands = [][]string{append(append(args, "--coverage.enabled",
				"--coverage.reporter=lcov", "--coverage.reportsDirectory="+out), vitestNoFloor...)}
			typescriptIntegration(&plan, dir, out)
		}
	case "jest":
		if jest := project.NodeBin(dir, "jest"); jest == "" {
			plan.Unsupported = missing("Jest")
		} else {
			args := []string{jest, "--coverage", "--coverageReporters=lcov", "--coverageDirectory=" + out, jestNoFloor}
			if len(sources) > 0 {
				args = append(append(args, "--findRelatedTests"), relativeTo(dir, sources)...)
			}
			plan.Commands = [][]string{args}
			typescriptIntegration(&plan, dir, out)
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

// kotlinPlan measures the Kotlin module at dir with its JaCoCo or Kover,
// running classes, the test classes KotlinTests gives, or the whole suite
// when classes is nil, and the JVMs its tests start with the JaCoCo agent
// it names (kotlinIntegration), their data under out.
func kotlinPlan(ctx context.Context, dir, out string, classes []string) Plan {
	kover := filepath.Join(dir, "build", "reports", "kover", "report.xml")
	jacoco := filepath.Join(dir, "build", "reports", "jacoco", "test", "jacocoTestReport.xml")
	maven := filepath.Join(dir, "target", "site", "jacoco", "jacoco.xml")
	plan := Plan{Language: "kotlin", Dir: dir, Existing: []string{kover, jacoco, maven}}
	runner := KotlinRunner(dir)
	if runner == "maven" {
		// The project's own JaCoCo, at the version it declares: naming the
		// plugin here would have Maven download one the project never chose.
		if !pomMentions(dir, "jacoco-maven-plugin") {
			plan.Unsupported = "jacoco-maven-plugin is not in pom.xml or a parent's; add it, or measure with --coverage-command"
			return plan
		}
		plan.Commands = [][]string{append([]string{"mvn", "-q", "jacoco:prepare-agent", "test", "jacoco:report"}, mavenTests(classes)...)}
		plan.Reports = []string{maven}
		kotlinIntegration(&plan, dir, out)
		return plan
	}
	gradle := "gradle"
	if root := project.FindUp(ctx, filepath.Join(dir, "x"), "gradlew"); root != "" {
		gradle = filepath.Join(root, "gradlew")
	}
	// The report task runs the test task, which the filter then narrows.
	task := "jacocoTestReport"
	plan.Reports = []string{jacoco}
	if runner == "kover" {
		task = "koverXmlReport"
		plan.Reports = []string{kover}
	}
	kotlinIntegration(&plan, dir, out)
	// While the agent is named, the test task runs even when Gradle deems
	// it up to date, so the JVMs its tests start write to this run.
	test := []string{"test"}
	if plan.CoverDir != "" {
		test = append(test, "--rerun")
	}
	args := []string{gradle, "-p", dir}
	if classes != nil {
		args = append(append(args, test...), gradleTests(classes)...)
	} else if task == "jacocoTestReport" || plan.CoverDir != "" {
		args = append(args, test...)
	}
	plan.Commands = [][]string{append(args, task)}
	return plan
}

// gradleTests is the --tests filter of a Gradle test task that runs
// classes, test classes by fully qualified name. The runner keeps this copy
// of mutation's filter, which runs a mutant's tests the same way, rather than
// import the mutation slice (ADR-0051).
func gradleTests(classes []string) []string {
	var out []string
	for _, class := range classes {
		out = append(out, "--tests", class)
	}
	return out
}

// mavenTests is the properties that have Surefire run classes, test classes
// by fully qualified name, in each module of a build that holds any of
// them, and none in one that holds none; nothing when classes is nil. The
// runner keeps this copy of mutation's filter rather than import the
// mutation slice (ADR-0051).
func mavenTests(classes []string) []string {
	if classes == nil {
		return nil
	}
	return []string{"-Dtest=" + strings.Join(classes, ","), "-Dsurefire.failIfNoSpecifiedTests=false"}
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
	if data, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil { //nolint:gosec // the selected project's package.json
		_ = json.Unmarshal(data, &pkg) // a manifest that does not parse names nothing, as if absent
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

// vitestVersion is the version of the package the installed vitest bin
// resolves into, its link followed to the nearest package.json above it,
// or where that cannot be read, of the vitest installed in the nearest
// node_modules; "" when neither can be read, any version admitted.
func vitestVersion(dir, bin string) string {
	if bin != "" {
		if target, err := filepath.EvalSymlinks(bin); err == nil {
			for d := filepath.Dir(target); filepath.Base(d) != "node_modules"; d = filepath.Dir(d) {
				var pkg struct {
					Name    string `json:"name"`
					Version string `json:"version"`
				}
				if data, err := os.ReadFile(filepath.Join(d, "package.json")); err == nil { //nolint:gosec // the package.json of the package the installed vitest bin resolves into
					if json.Unmarshal(data, &pkg) == nil && pkg.Name == "vitest" && pkg.Version != "" {
						return pkg.Version
					}
					break
				}
				if filepath.Dir(d) == d {
					break
				}
			}
		}
	}
	return installedVersion(dir, "vitest")
}

func installedVersion(dir, dep string) string {
	var pkg struct {
		Version string `json:"version"`
	}
	if module := project.NodeModule(dir, dep); module != "" {
		if data, err := os.ReadFile(filepath.Join(module, "package.json")); err == nil { //nolint:gosec // an installed dependency's package.json in the project's node_modules
			_ = json.Unmarshal(data, &pkg) // a manifest that does not parse names no version: running will tell
		}
	}
	return pkg.Version
}

func buildMentions(dir, word string) bool {
	for _, name := range []string{"build.gradle.kts", "build.gradle"} {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil && strings.Contains(string(data), word) { //nolint:gosec // the selected module's Gradle build file
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
		data, err := os.ReadFile(filepath.Join(d, "pom.xml")) //nolint:gosec // pom.xml at and above the selected module
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
	_, err := os.Stat(path) //nolint:gosec // only asks whether a path of the selected project is there
	return err == nil
}

func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:4])
}
