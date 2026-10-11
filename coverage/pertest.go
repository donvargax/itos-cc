package coverage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/project"
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
// COVERAGE_PROCESS_START. go test passes the variable on to its test
// binaries, as it does not GOCOVERDIR. When nothing is written there, each
// test runs alone, every process it starts writing to a directory of its
// own: GOCOVERDIR, as integration coverage passes it, and COVERAGE_FILE,
// so a Python test runner's own process counts too.

// TestCoverDirEnv names the directory under which a harness has each
// test's processes write their coverage: GOCOVERDIR=<dir>/<test ID>, or
// COVERAGE_FILE=<dir>/<test ID>/.coverage.
const TestCoverDirEnv = "ITOS_CC_TEST_COVERDIR"

// Test is one test a project lists: its ID, and the file that defines it as
// the list command names it, or "".
type Test struct {
	ID, File string
}

// PerTest is how to run a project's listed tests: shell command lines run
// through the platform shell in Root.
type PerTest struct {
	Root  string
	Tests []Test
	// All runs every test.
	All string
	// Select runs the tests ids.
	Select func(ids []string) string
}

// TestCoverage is each listed test's coverage.
type TestCoverage struct {
	ids     []string // those measured, in the list's order
	reports map[string]*Report
}

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
	// command that turns it into report.
	written func(d string) bool
	convert func(d, report string) []string
}

// testCollectors is the collectors of the listed tests of root measuring
// sources, with dir for their data: Go's always, and Python's when sources
// hold Python files, from their build roots.
func testCollectors(root, dir string, sources []string) []testCollector {
	collectors := []testCollector{{
		language: "go",
		dir:      root,
		each:     eachTestEnv,
		written:  hasCoverData,
		convert: func(d, report string) []string {
			return []string{"go", "tool", "covdata", "textfmt", "-i=" + d, "-o=" + report}
		},
	}}
	var roots []string
	for _, s := range sources {
		if spec := lang.Detect(s); spec == nil || spec.Name != "python" {
			continue
		}
		r := lang.FindUp(s, markers["python"]...)
		if r == "" {
			r = filepath.Dir(s)
		}
		if !slices.Contains(roots, r) {
			roots = append(roots, r)
		}
	}
	if len(roots) == 0 {
		return collectors
	}
	sort.Strings(roots)
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
		convert: func(d, report string) []string {
			return []string{py, "-c", pythonCombineScript, data(d), report}
		},
	})
}

// startTestCollectors readies collectors, through execute when it is not
// nil, and returns those that can measure the processes a test starts. One
// that cannot is logged and left out; one whose preparation fails
// otherwise is too, unless strict, when it is an error.
func startTestCollectors(ctx context.Context, collectors []testCollector, log io.Writer, execute CommandExecutor, strict bool) ([]testCollector, []CommandExecution, error) {
	var out []testCollector
	var calls []CommandExecution
	for _, c := range collectors {
		if len(c.prepare) == 0 {
			out = append(out, c)
			continue
		}
		p := Plan{Language: c.language, Dir: c.dir, Prepare: c.prepare, CoverEnv: c.vars}
		why, call, err := p.start(ctx, project.NoBytecodeEnv(os.Environ()), log, execute)
		calls = append(calls, call)
		if err != nil && strict {
			return nil, calls, err
		}
		if err != nil {
			why = err.Error()
		}
		if why != "" {
			fmt.Fprintf(log, "itos-cc: coverage: %s: the listed tests' %s processes go unmeasured: %s\n", c.dir, languageLabel(c.language), why)
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
// removes once read, for their data. It returns nil when the tests fail:
// the tests that fail without any mutant decide no mutant.
func MeasureTests(p PerTest, dir string, sources []string, log io.Writer) *TestCoverage {
	os.RemoveAll(dir)
	defer os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(log, "itos-cc: coverage: %v\n", err)
		return nil
	}
	collectors, _, _ := startTestCollectors(context.Background(), testCollectors(p.Root, dir, sources), log, nil, false)
	tc := &TestCoverage{reports: map[string]*Report{}}
	var ids []string
	for _, t := range p.Tests {
		ids = append(ids, t.ID)
	}
	if err := runShell(p.All, p.Root, wholeEnv(collectors, dir), log); err != nil {
		fmt.Fprintf(log, "itos-cc: coverage: the listed tests fail without any mutant, so no mutant runs them: %v\n", err)
		return nil
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
			if err := os.MkdirAll(d, 0o755); err != nil {
				fmt.Fprintf(log, "itos-cc: coverage: %v\n", err)
				return nil
			}
			if err := runShell(p.Select([]string{id}), p.Root, eachEnv(collectors, d), log); err != nil {
				fmt.Fprintf(log, "itos-cc: coverage: listed test %s fails without any mutant, so no mutant runs the listed tests: %v\n", id, err)
				return nil
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
		var reports []*Report
		for _, c := range collectors {
			if !c.written(d) {
				continue
			}
			profile := filepath.Join(dir, ".profiles", strconv.Itoa(i)+"."+c.language)
			os.MkdirAll(filepath.Dir(profile), 0o755)
			args := c.convert(d, profile)
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Dir, cmd.Env = c.dir, unsetEnv(project.NoBytecodeEnv(os.Environ()), c.vars...)
			if out, err := cmd.CombinedOutput(); err != nil {
				fmt.Fprintf(log, "itos-cc: coverage: %s: %s: %v\n%s", id, displayArgs(args), err, out)
				continue
			}
			entries, err := Load(profile)
			if err != nil {
				fmt.Fprintf(log, "itos-cc: coverage: %s: %v\n", id, err)
				continue
			}
			reports = append(reports, Build(sources, c.dir, entries))
		}
		if len(reports) == 0 {
			continue
		}
		tc.ids = append(tc.ids, id)
		tc.reports[id] = Merge(reports...)
	}
	return tc
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

func hasCoverData(dir string) bool {
	written, _ := filepath.Glob(filepath.Join(dir, "covmeta.*"))
	return len(written) > 0
}

// runShell runs line through the platform shell in dir, with env set in
// the environment and its output written to log.
func runShell(line, dir string, env []string, log io.Writer) error {
	fmt.Fprintf(log, "itos-cc: coverage %s$ %s\n", dir, line)
	name, flag := "sh", "-c"
	if runtime.GOOS == "windows" {
		name, flag = "cmd", "/C"
	}
	cmd := exec.Command(name, flag, line)
	cmd.Dir = dir
	cmd.Env = setEnv(project.NoBytecodeEnv(os.Environ()), env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	log.Write(out.Bytes())
	return err
}

// SetTests gives r the coverage of each listed test.
func (r *Report) SetTests(tc *TestCoverage) {
	if r != nil {
		r.tests = tc
	}
}

// LineTests is the IDs of the listed tests that executed line of file, in
// the list's order.
func (r *Report) LineTests(file string, line int) []string {
	if r == nil || r.tests == nil {
		return nil
	}
	var out []string
	for _, id := range r.tests.ids {
		if covered, _ := r.tests.reports[id].LineCovered(file, line); covered {
			out = append(out, id)
		}
	}
	return out
}
