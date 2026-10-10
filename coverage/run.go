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
	"sort"
	"strconv"
	"strings"

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
}

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
	// mutate kills its mutants with; TypeScript runs the related tests.
	OwnTests
)

// Plans groups sources by language and build root and returns one plan per
// group, with reports written under outDir. Python and Kotlin run the whole
// suite whatever the scope.
func Plans(sources []string, outDir string, scope Scope) []Plan {
	plans, _, _ := buildPlans(context.Background(), sources, outDir, scope, nil)
	return plans
}

// PlansSupervised builds coverage plans while routing executable probes
// (Go package discovery and Python provider checks) through execute.
func PlansSupervised(ctx context.Context, sources []string, outDir string, scope Scope, execute CommandExecutor) ([]Plan, []CommandExecution, error) {
	if ctx == nil || execute == nil {
		return nil, nil, errors.New("supervised coverage planning needs a context and command executor")
	}
	return buildPlans(ctx, sources, outDir, scope, execute)
}

func buildPlans(ctx context.Context, sources []string, outDir string, scope Scope, execute CommandExecutor) ([]Plan, []CommandExecution, error) {
	type key struct{ lang, dir string }
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
		groups[key{spec.Name, dir}] = append(groups[key{spec.Name, dir}], s)
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
			p = typescriptPlan(k.dir, out, srcs)
		case "python":
			if execute == nil {
				p = pythonPlan(k.dir, out)
			} else {
				var calls []CommandExecution
				var err error
				p, calls, err = pythonPlanSupervised(ctx, k.dir, out, execute)
				executions = append(executions, calls...)
				if err != nil {
					failures = append(failures, err)
				}
			}
		case "kotlin":
			p = kotlinPlan(k.dir)
		default:
			continue
		}
		p.Sources = sources
		plans = append(plans, p)
	}
	sort.Slice(plans, func(i, j int) bool {
		return plans[i].Language+plans[i].Dir < plans[j].Language+plans[j].Dir
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
// sources is empty. A coverage script or c8 runs the whole suite.
func typescriptPlan(dir, out string, sources []string) Plan {
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
	switch {
	case pkg.Scripts["coverage"] != "":
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

func pythonPlan(dir, out string) Plan {
	py := pythonFor(dir)
	data := filepath.Join(out, ".coverage")
	report := filepath.Join(out, "lcov.info")
	runner := []string{"-m", "unittest", "discover"}
	if exec.Command(py, "-c", "import pytest").Run() == nil {
		runner = []string{"-m", "pytest", "-q"}
	}
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

func pythonPlanSupervised(ctx context.Context, dir, out string, execute CommandExecutor) (Plan, []CommandExecution, error) {
	py := pythonFor(dir)
	var executions []CommandExecution
	probe := func(module string) error {
		cmd := exec.CommandContext(ctx, py, "-c", "import "+module)
		cmd.Dir, cmd.Stdout, cmd.Stderr = dir, io.Discard, io.Discard
		err := execute(ctx, cmd)
		executions = append(executions, CommandExecution{Args: append([]string{}, cmd.Args...), Dir: dir, Err: err})
		return err
	}
	runner := []string{"-m", "unittest", "discover"}
	pytestErr := probe("pytest")
	if pytestErr == nil {
		runner = []string{"-m", "pytest", "-q"}
	} else if ctx.Err() != nil {
		return Plan{}, executions, ctx.Err()
	}
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

func kotlinPlan(dir string) Plan {
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
		plan.Commands = [][]string{{"mvn", "-q", "jacoco:prepare-agent", "test", "jacoco:report"}}
		plan.Reports = []string{maven}
		return plan
	}
	gradle := "gradle"
	if root := lang.FindUp(filepath.Join(dir, "x"), "gradlew"); root != "" {
		gradle = filepath.Join(root, "gradlew")
	}
	if buildMentions(dir, "kover") {
		plan.Commands = [][]string{{gradle, "-p", dir, "koverXmlReport"}}
		plan.Reports = []string{kover}
	} else {
		plan.Commands = [][]string{{gradle, "-p", dir, "test", "jacocoTestReport"}}
		plan.Reports = []string{jacoco}
	}
	return plan
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
		if p.Unsupported != "" {
			fmt.Fprintf(log, "itos-cc: coverage: %s: %s\n", p.Dir, p.Unsupported)
			reports = append(reports, &Report{missing: []Unmeasured{{Dir: p.Dir, Language: p.Language, Cause: ToolMissing, Reason: p.Unsupported}}})
			continue
		}
		for _, r := range append(p.Reports, p.Integration) {
			os.Remove(r)
		}
		os.MkdirAll(filepath.Dir(p.Reports[0]), 0o755)
		var env []string
		if p.CoverDir != "" && os.MkdirAll(p.CoverDir, 0o755) == nil {
			env = append(os.Environ(), coverDirEnv+"="+p.CoverDir)
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
		r := load(p.Reports, p.integrate(log), p.Dir, sources, log)
		// Told by exit status and the report alone, never by what the
		// runner prints: a run that succeeded and wrote its report measured
		// its language, whichever files the report names.
		if failed == "" && len(r.missing) == 0 {
			r.languages = map[string]bool{p.Language: true}
			reports = append(reports, r)
			continue
		}
		reports = append(reports, p.measured(r, MeasuredNothing, "its coverage run measured none of its files"+failed))
	}
	return Merge(reports...)
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
		r := &Report{}
		for _, path := range p.Existing {
			if exists(path) {
				// A run's integration profile goes with its report.
				integration := ""
				if path == p.Reports[0] && p.Integration != "" && exists(p.Integration) {
					integration = p.Integration
				}
				r = load([]string{path}, integration, p.Dir, sources, log)
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
