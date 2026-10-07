package coverage

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Coverage per listed test: which of the tests a project lists in
// itos-cc.yaml (mutation.tests) execute each line, so a mutant runs only
// those that reach it. It takes one run of every test when the project's
// harness splits it: itos-cc sets TestCoverDirEnv to a directory, and the
// harness gives the processes each test starts GOCOVERDIR=<dir>/<test ID>.
// go test passes the variable on to its test binaries, as it does not
// GOCOVERDIR. When nothing is written there, each test runs alone, with
// GOCOVERDIR its own, as integration coverage passes it. Only Go binaries
// built with -cover write such data so far.

// TestCoverDirEnv names the directory under which a harness gives each
// test's processes GOCOVERDIR=<dir>/<test ID>.
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

// TestCoverage is each listed test's coverage, and how long running them
// took.
type TestCoverage struct {
	ids     []string // those measured, in the list's order
	reports map[string]*Report
	elapsed time.Duration
}

// MeasureTests runs p's tests for coverage of sources, with dir, which it
// removes once read, for their data. It returns nil when the tests fail:
// the tests that fail without any mutant decide no mutant.
func MeasureTests(p PerTest, dir string, sources []string, log io.Writer) *TestCoverage {
	os.RemoveAll(dir)
	defer os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(log, "coverage: %v\n", err)
		return nil
	}
	tc := &TestCoverage{reports: map[string]*Report{}}
	var ids []string
	for _, t := range p.Tests {
		ids = append(ids, t.ID)
	}
	elapsed, err := runShell(p.All, p.Root, []string{TestCoverDirEnv + "=" + dir}, log)
	if err != nil {
		fmt.Fprintf(log, "coverage: the listed tests fail without any mutant, so no mutant runs them: %v\n", err)
		return nil
	}
	tc.elapsed = elapsed
	written := map[string]string{}
	for _, id := range ids {
		if d := filepath.Join(dir, id); hasCoverData(d) {
			written[id] = d
		}
	}
	if len(written) == 0 {
		// The harness did not split the run: each test alone, with a
		// GOCOVERDIR of its own.
		tc.elapsed = 0
		for i, id := range ids {
			d := filepath.Join(dir, ".each", strconv.Itoa(i))
			if err := os.MkdirAll(d, 0o755); err != nil {
				fmt.Fprintf(log, "coverage: %v\n", err)
				return nil
			}
			elapsed, err := runShell(p.Select([]string{id}), p.Root, eachTestEnv(d), log)
			if err != nil {
				fmt.Fprintf(log, "coverage: listed test %s fails without any mutant, so no mutant runs the listed tests: %v\n", id, err)
				return nil
			}
			tc.elapsed += elapsed
			if hasCoverData(d) {
				written[id] = d
			}
		}
	}
	for i, id := range ids {
		d, ok := written[id]
		if !ok {
			continue
		}
		profile := filepath.Join(dir, ".profiles", strconv.Itoa(i)+".out")
		os.MkdirAll(filepath.Dir(profile), 0o755)
		cmd := exec.Command("go", "tool", "covdata", "textfmt", "-i="+d, "-o="+profile)
		cmd.Dir = p.Root
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(log, "coverage: %s: go tool covdata textfmt: %v\n%s", id, err, out)
			continue
		}
		entries, err := Load(profile)
		if err != nil {
			fmt.Fprintf(log, "coverage: %s: %v\n", id, err)
			continue
		}
		tc.ids = append(tc.ids, id)
		tc.reports[id] = Build(sources, p.Root, entries)
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

// runShell runs line through the platform shell in dir, with env added to
// the environment and its output written to log, and returns how long it
// took.
func runShell(line, dir string, env []string, log io.Writer) (time.Duration, error) {
	fmt.Fprintf(log, "coverage: %s$ %s\n", dir, line)
	name, flag := "sh", "-c"
	if runtime.GOOS == "windows" {
		name, flag = "cmd", "/C"
	}
	cmd := exec.Command(name, flag, line)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)
	log.Write(out.Bytes())
	return elapsed, err
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

// TestsElapsed is how long running the listed tests for coverage took, 0
// when they were not.
func (r *Report) TestsElapsed() time.Duration {
	if r == nil || r.tests == nil {
		return 0
	}
	return r.tests.elapsed
}
