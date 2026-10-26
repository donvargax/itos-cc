package covrun

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/coverage/port"
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/project"
)

// Coverage per listed test: which of the tests a project lists in
// itos-cc.yaml (mutation.tests) execute each line, so a mutant runs only
// those that reach it. It takes one run of every test when the project's
// harness splits it: itos-cc sets TestCoverDirEnv to a directory, and the
// harness has the processes each test starts write their coverage under
// <dir>/<test ID>, each language's collector as integration coverage has
// it: GOCOVERDIR=<dir>/<test ID> for Go binaries built with -cover, and
// COVERAGE_FILE=<dir>/<test ID>/.coverage for Python processes, which
// coverage.py starts in with the rcfile itos-cc names in
// COVERAGE_PROCESS_START, NODE_V8_COVERAGE=<dir>/<test ID> for Node
// processes, and a JaCoCo destfile under <dir>/<test ID>/ for the JVMs a
// Kotlin harness starts with the agent JacocoAgentEnv names. go test passes
// the variable on to its test binaries, as it does not GOCOVERDIR. When
// nothing is written there, each test runs alone, every process it starts
// writing to a directory of its own: GOCOVERDIR, as integration coverage
// passes it, COVERAGE_FILE, so a Python test runner's own process counts
// too, NODE_V8_COVERAGE and JacocoDestDirEnv.

// TestCoverDirEnv names the directory under which a harness has each
// test's processes write their coverage: GOCOVERDIR=<dir>/<test ID>, or
// COVERAGE_FILE=<dir>/<test ID>/.coverage.
const TestCoverDirEnv = "ITOS_CC_TEST_COVERDIR"

// Test and PerTest are coverage-domain values, retained here for legacy runners.
type Test = coverage.Test
type PerTest = coverage.PerTest

// testCollector is how one language's collector measures the processes a
// listed test starts.
type testCollector struct {
	language string
	// dir is where its commands run, and what its reports' relative paths
	// start from.
	dir string
	// prepare, when set, readies it once, before any test runs, with none
	// of the variables of vars; it exits 3 when it cannot measure them,
	// printing why.
	prepare []string
	vars    []string
	// whole is the environment of the run of every test, each the one
	// that has every process of a run write to d.
	whole []string
	each  func(d string) []string
	// written says whether data was written to d, and convert is the
	// command that turns it into a report, under out, and that report.
	written func(d string) bool
	convert func(d, out string) ([]string, string)
	// missing, when set, says why the project lacks it: it is left out.
	missing string
}

// testCollectors is the collectors of the listed tests of root measuring
// sources, with dir for their data: Go's always, and TypeScript's, Kotlin's
// and Python's when sources hold files of theirs, from their build roots.
func testCollectors(ctx context.Context, root, dir string, sources []string) []testCollector {
	collectors := []testCollector{{
		language: "go",
		dir:      root,
		each:     eachTestEnv,
		written:  func(d string) bool { return hasCoverData(d, ownCoverHash()) },
		convert: func(d, out string) ([]string, string) {
			return []string{"go", "tool", "covdata", "textfmt", "-i=" + d, "-o=" + out + ".out"}, out + ".out"
		},
	}}
	if roots := buildRoots(ctx, sources, "typescript"); len(roots) > 0 {
		c := testCollector{
			language: "typescript",
			dir:      roots[0],
			each:     func(d string) []string { return []string{"NODE_V8_COVERAGE=" + d} },
			written: func(d string) bool {
				found, _ := filepath.Glob(filepath.Join(d, "coverage-*.json"))
				return len(found) > 0
			},
		}
		if c8 := project.NodeBin(roots[0], "c8"); c8 == "" {
			c.missing = "c8 is not installed, which reads the V8 coverage of the Node processes the tests start; add c8 to devDependencies"
		} else {
			c.convert = func(d, out string) ([]string, string) {
				return c8Report(c8, d, out), filepath.Join(out, "lcov.info")
			}
		}
		collectors = append(collectors, c)
	}
	if roots := buildRoots(ctx, sources, "kotlin"); len(roots) > 0 {
		collectors = append(collectors, kotlinTestCollector(roots[0], dir))
	}
	roots := buildRoots(ctx, sources, "python")
	if len(roots) == 0 {
		return collectors
	}
	py := pythonFor(roots[0])
	rc := filepath.Join(dir, ".python", "coveragerc")
	data := func(d string) string { return filepath.Join(d, ".coverage") }
	return append(collectors, testCollector{
		language: "python",
		dir:      roots[0],
		prepare:  append([]string{py, "-c", pythonStartScript, rc, data(filepath.Join(dir, ".python", "unsplit"))}, roots...),
		vars:     pythonCoverEnv(rc),
		whole:    pythonCoverEnv(rc),
		each: func(d string) []string {
			return append(pythonCoverEnv(rc), "COVERAGE_FILE="+data(d))
		},
		written: func(d string) bool {
			found, _ := filepath.Glob(filepath.Join(d, ".coverage.*"))
			return len(found) > 0
		},
		convert: func(d, out string) ([]string, string) {
			return []string{py, "-c", pythonCombineScript, data(d), out + ".info"}, out + ".info"
		},
	})
}

// buildRoots is the build roots of sources of language, sorted.
func buildRoots(ctx context.Context, sources []string, language string) []string {
	var roots []string
	for _, s := range sources {
		if spec := lang.Detect(s); spec == nil || spec.Name != language {
			continue
		}
		r := project.FindUp(ctx, s, markers[language]...)
		if r == "" {
			r = filepath.Dir(s)
		}
		if !slices.Contains(roots, r) {
			roots = append(roots, r)
		}
	}
	sort.Strings(roots)
	return roots
}

// startTestCollectors readies collectors, through execute when it is not
// nil, and returns those that can measure the processes a test starts. One
// that cannot is logged and left out; one whose preparation fails
// otherwise is too, unless strict, when it is an error.
func startTestCollectors(ctx context.Context, collectors []testCollector, log io.Writer, execute CommandExecutor, strict bool) ([]testCollector, []CommandExecution, error) {
	var out []testCollector
	var calls []CommandExecution
	for _, c := range collectors {
		if c.missing != "" {
			logf(log, "itos-cc: coverage: %s: the listed tests' %s processes go unmeasured: %s\n", c.dir, languageLabel(c.language), c.missing)
			continue
		}
		if len(c.prepare) == 0 {
			out = append(out, c)
			continue
		}
		p := Plan{Language: c.language, Dir: c.dir, Prepare: c.prepare, CoverEnv: c.vars}
		why, call, err := p.start(ctx, project.NoBytecodeEnv(nil), log, execute)
		calls = append(calls, call)
		if err != nil && strict {
			return nil, calls, err
		}
		if err != nil {
			why = err.Error()
		}
		if why != "" {
			logf(log, "itos-cc: coverage: %s: the listed tests' %s processes go unmeasured: %s\n", c.dir, languageLabel(c.language), why)
			continue
		}
		out = append(out, c)
	}
	return out, calls, nil
}

// languageLabel is how log lines name language.
func languageLabel(language string) string {
	if language == "go" {
		return "Go"
	}
	return strings.ToUpper(language[:1]) + language[1:]
}

// wholeEnv is the environment of the run of every listed test, with dir
// for their data.
func wholeEnv(collectors []testCollector, dir string) []string {
	env := []string{TestCoverDirEnv + "=" + dir}
	for _, c := range collectors {
		env = append(env, c.whole...)
	}
	return env
}

// eachEnv is the environment that has every process of a run of one
// listed test write its coverage to d.
func eachEnv(collectors []testCollector, d string) []string {
	var env []string
	for _, c := range collectors {
		env = append(env, c.each(d)...)
	}
	return env
}

// anyWritten says whether a collector found data in d.
func anyWritten(collectors []testCollector, d string) bool {
	for _, c := range collectors {
		if c.written(d) {
			return true
		}
	}
	return false
}

// MeasureTests runs p's tests for coverage of sources, with dir, which it
// removes once read, for their data. Tests that fail without any mutant
// decide no mutant: the run of every test, or in the fallback one test's
// run alone, that fails is port.TestsFailed, naming its command, unless
// ctx is done, when it gives nothing and the caller reports the stop.
func MeasureTests(ctx context.Context, p PerTest, dir string, sources []string, log io.Writer, execute CommandExecutor) (*coverage.TestCoverage, port.Failure) {
	// This unsupervised measure has always gone on when it cannot clear or
	// remove its folder; MeasureTestsSupervised reports either.
	_ = os.RemoveAll(dir)
	defer func() { _ = os.RemoveAll(dir) }()
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // the listed tests' coverage folder keeps its 0755; narrowing it would change what the run creates
		logf(log, "itos-cc: coverage: %v\n", err)
		return nil, nil
	}
	collectors, _, _ := startTestCollectors(ctx, testCollectors(ctx, p.Root, dir, sources), log, nil, false)
	tc := &coverage.TestCoverage{}
	var ids []string
	for _, t := range p.Tests {
		ids = append(ids, t.ID)
	}
	if err := runShell(ctx, p.All, p.Root, wholeEnv(collectors, dir), log, execute); err != nil {
		logf(log, "itos-cc: coverage: the listed tests fail without any mutant, so no mutant is judged: %v\n", err)
		return nil, testsFailed(ctx, p.All, err)
	}
	written := map[string]string{}
	for _, id := range ids {
		if d := filepath.Join(dir, id); anyWritten(collectors, d) {
			written[id] = d
		}
	}
	if len(written) == 0 {
		// The harness did not split the run: each test alone, every
		// process it starts writing to a directory of its own.
		for i, id := range ids {
			d := filepath.Join(dir, ".each", strconv.Itoa(i))
			if err := os.MkdirAll(d, 0o755); err != nil { //nolint:gosec // a listed test's own coverage folder keeps its 0755; narrowing it would change what the run creates
				logf(log, "itos-cc: coverage: %v\n", err)
				return nil, nil
			}
			if err := runShell(ctx, p.Select([]string{id}), p.Root, eachEnv(collectors, d), log, execute); err != nil {
				logf(log, "itos-cc: coverage: listed test %s fails without any mutant, so no mutant is judged: %v\n", id, err)
				return nil, testsFailed(ctx, p.Select([]string{id}), err)
			}
			if anyWritten(collectors, d) {
				written[id] = d
			}
		}
	}
	for i, id := range ids {
		d, ok := written[id]
		if !ok {
			continue
		}
		// A done context starts no command: the caller reports the stop.
		if ctx.Err() != nil {
			return nil, nil
		}
		var reports []*coverage.Report
		for _, c := range collectors {
			if !c.written(d) {
				continue
			}
			// A folder it cannot make fails the conversion below, which is logged.
			_ = os.MkdirAll(filepath.Join(dir, ".profiles"), 0o755) //nolint:gosec // the converted profiles' folder keeps its 0755; narrowing it would change what the run creates
			args, profile := c.convert(d, filepath.Join(dir, ".profiles", strconv.Itoa(i)+"."+c.language))
			cmd := exec.Command(args[0], args[1:]...) //nolint:gosec // the collector's conversion of a listed test's data, which this package names
			cmd.Dir, cmd.Env = c.dir, unsetEnv(project.NoBytecodeEnv(nil), c.vars...)
			if out, err := cmd.CombinedOutput(); err != nil {
				logf(log, "itos-cc: coverage: %s: %s: %v\n%s", id, displayArgs(args), err, out)
				continue
			}
			entries, err := Load(profile)
			if err != nil {
				logf(log, "itos-cc: coverage: %s: %v\n", id, err)
				continue
			}
			reports = append(reports, coverage.Build(ctx, Disk{}, sources, c.dir, entries))
		}
		if len(reports) == 0 {
			continue
		}
		tc.Add(id, coverage.Merge(reports...))
	}
	return tc, nil
}

// testsFailed is the failure of command, a run of listed tests that ended
// with err, or nil once ctx is done, a stop the caller reports.
func testsFailed(ctx context.Context, command string, err error) port.Failure {
	if ctx.Err() != nil {
		return nil
	}
	failed := port.TestsFailed{Command: command, Reason: err.Error(), ExitCode: -1}
	if exit := (*exec.ExitError)(nil); errors.As(err, &exit) {
		failed.ExitCode = exit.ExitCode()
	}
	var cleanup port.CleanupFailed
	if errors.As(err, &cleanup) {
		failed.Cleanup = &cleanup
	}
	return failed
}

// eachTestEnv is the environment that gives the processes a command starts
// GOCOVERDIR=dir: directly, through ExecTest for test binaries go test runs
// with -cover, which it reaches through GOFLAGS where Executable can be
// quoted there.
func eachTestEnv(dir string) []string {
	env := []string{"GOCOVERDIR=" + dir, coverDirEnv + "=" + dir}
	if Executable != "" && !strings.ContainsAny(Executable, `"'`) {
		flag := `'-exec="` + Executable + `" ` + ExecArg + `'`
		env = append(env, "GOFLAGS="+strings.TrimSpace(os.Getenv("GOFLAGS")+" "+flag))
	}
	return env
}

// hasCoverData says whether a Go binary other than the one whose
// meta-data hash is own wrote coverage data to dir, first removing own's
// (dropCoverData): the measuring itos-cc's, which reaches a listed test's
// folder as go test's -exec program.
func hasCoverData(dir, own string) bool {
	dropCoverData(dir, own)
	written, _ := filepath.Glob(filepath.Join(dir, "covmeta.*"))
	return len(written) > 0
}

// runShell runs line through the platform shell in dir, with env set in
// the environment and its output written to log. Once ctx is done it starts
// nothing and returns its cause.
func runShell(ctx context.Context, line, dir string, env []string, log io.Writer, execute CommandExecutor) error {
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	logf(log, "itos-cc: coverage %s$ %s\n", dir, line)
	cmd := shellCommand(line)
	if execute != nil {
		cmd = shellCommandContext(ctx, line)
	}
	cmd.Dir = dir
	cmd.Env = setEnv(project.NoBytecodeEnv(nil), env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := runCommand(ctx, cmd, execute)
	logf(log, "%s", out.Bytes())
	return err
}

// Shell runs line, a person's command line such as --coverage-command,
// through the platform shell in the working directory, with its output
// written to log.
func Shell(line string, log io.Writer) error {
	cmd := shellCommand(line)
	cmd.Env = project.InheritedEnv()
	cmd.Stdout = log
	cmd.Stderr = log
	return cmd.Run()
}

// shellCommandContext is shellCommand under ctx, which kills it once done.
func shellCommandContext(ctx context.Context, line string) *exec.Cmd {
	cmd := shellCommand(line)
	owned := exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...) //nolint:gosec // the person's own command line, run through the platform shell
	owned.Args = cmd.Args
	return owned
}

// shellCommand is line run through the platform shell: sh, or cmd on
// Windows.
func shellCommand(line string) *exec.Cmd {
	name, flag := "sh", "-c"
	if runtime.GOOS == "windows" {
		name, flag = "cmd", "/C"
	}
	return exec.Command(name, flag, line) //nolint:gosec // the person's own command line, run through the platform shell
}
