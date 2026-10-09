package mutate

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const runnerFixtureMode = "ITOS_COMMAND_RUNNER_FIXTURE_MODE"

// TestWorkerRunPreservesCommandSetup characterizes worker.run before its
// execution body is extracted. These are equivalence guards, not behavior reds.
func TestWorkerRunPreservesCommandSetup(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ITOS_COMMAND_RUNNER_CUSTOM", "inherited-value")
	t.Setenv("ITOS_COMMAND_RUNNER_PATH", "prior-path")
	t.Setenv(runnerFixtureMode, "setup")
	w := newCommandRunnerWorker(t)

	c := Command{
		Root:     root,
		Dir:      filepath.Join(root, "pkg"),
		Args:     commandRunnerHelperArgs(),
		PathEnv:  "ITOS_COMMAND_RUNNER_PATH",
		PathDirs: []string{"bin"},
	}
	r, err := w.run(c, 0)
	if err != nil {
		t.Fatalf("worker.run: %v", err)
	}
	if !r.passed || r.exitCode != 0 {
		t.Fatalf("result = %+v, want successful command", r)
	}
	copyRoot := w.copies[root]
	wantDir := filepath.Join(copyRoot, "pkg")
	wantDir, err = filepath.EvalSymlinks(wantDir)
	if err != nil {
		t.Fatal(err)
	}
	wantPath := strings.Join([]string{filepath.Join(copyRoot, "bin"), "prior-path"}, string(os.PathListSeparator))
	gotDir := ""
	for _, line := range strings.Split(r.output, "\n") {
		if strings.HasPrefix(line, "cwd=") {
			gotDir = strings.TrimPrefix(line, "cwd=")
			break
		}
	}
	gotDir, err = filepath.EvalSymlinks(gotDir)
	if err != nil || gotDir != wantDir {
		t.Errorf("command cwd = %q, want resolved worker copy directory %q (resolve error %v)", gotDir, wantDir, err)
	}
	for _, want := range []string{"custom=inherited-value", "path=" + wantPath, "stderr-marker"} {
		if !strings.Contains(r.output, want) {
			t.Errorf("combined output %q does not contain %q", r.output, want)
		}
	}

	shell := "printf shell-output"
	if runtime.GOOS == "windows" {
		shell = "echo shell-output"
	}
	r, err = w.run(Command{Root: root, Dir: root, Shell: shell}, 0)
	if err != nil || !r.passed || !strings.Contains(r.output, "shell-output") {
		t.Errorf("shell command result = %+v, err = %v; want captured shell output", r, err)
	}
}

// TestCommandRunnerPreservesLegacyResults characterizes success, nonzero
// exit, own timeout, zero timeout and startup error through the original
// worker entrypoint. These are passing equivalence guards, not behavior reds.
func TestCommandRunnerPreservesLegacyResults(t *testing.T) {
	w := newCommandRunnerWorker(t)
	root := t.TempDir()
	base := Command{Root: root, Dir: root, Args: commandRunnerHelperArgs()}

	t.Run("success", func(t *testing.T) {
		t.Setenv(runnerFixtureMode, "success")
		r, err := w.run(base, 10*time.Second)
		if err != nil || !r.passed || r.timedOut || r.exitCode != 0 || !strings.Contains(r.output, "success-stdout") || !strings.Contains(r.output, "success-stderr") {
			t.Fatalf("result = %+v, err = %v", r, err)
		}
	})
	t.Run("nonzero", func(t *testing.T) {
		t.Setenv(runnerFixtureMode, "nonzero")
		r, err := w.run(base, 10*time.Second)
		if err != nil || r.passed || r.timedOut || r.exitCode != 7 || !strings.Contains(r.output, "nonzero-output") {
			t.Fatalf("result = %+v, err = %v", r, err)
		}
	})
	t.Run("own timeout", func(t *testing.T) {
		t.Setenv(runnerFixtureMode, "loop")
		r, err := w.run(base, 60*time.Millisecond)
		if err != nil || !r.timedOut || r.passed {
			t.Fatalf("result = %+v, err = %v", r, err)
		}
	})
	t.Run("zero timeout", func(t *testing.T) {
		t.Setenv(runnerFixtureMode, "success")
		r, err := w.run(base, 0)
		if err != nil || !r.passed || r.timedOut || r.exitCode != 0 {
			t.Fatalf("result = %+v, err = %v", r, err)
		}
	})
	t.Run("start error", func(t *testing.T) {
		c := Command{Root: root, Dir: root, Args: []string{filepath.Join(root, "missing-command")}}
		_, err := w.run(c, time.Second)
		if err == nil {
			t.Fatal("worker.run succeeded for a missing executable")
		}
	})
}

// TestCommandRunnerPreservesLifecycleOrdering observes the original worker
// entrypoint blocking between fixture start and release, then returning only
// after the fixture's completion marker and final output. It is a passing
// equivalence guard, not a cleanup-on-normal-return assertion.
func TestCommandRunnerPreservesLifecycleOrdering(t *testing.T) {
	root := t.TempDir()
	w := newCommandRunnerWorker(t)
	started := filepath.Join(root, "started")
	release := filepath.Join(root, "release")
	finished := filepath.Join(root, "finished")
	t.Setenv(runnerFixtureMode, "barrier")
	t.Setenv("ITOS_COMMAND_RUNNER_STARTED", started)
	t.Setenv("ITOS_COMMAND_RUNNER_RELEASE", release)
	t.Setenv("ITOS_COMMAND_RUNNER_FINISHED", finished)

	type runResult struct {
		r   result
		err error
	}
	done := make(chan runResult, 1)
	go func() {
		r, err := w.run(Command{Root: root, Dir: root, Args: commandRunnerHelperArgs()}, 5*time.Second)
		done <- runResult{r: r, err: err}
	}()
	consumed := false
	defer func() {
		_ = os.WriteFile(release, []byte("release"), 0o600)
		if !consumed {
			select {
			case <-done:
			case <-time.After(6 * time.Second):
				t.Error("worker.run helper did not stop within its bounded timeout")
			}
		}
	}()
	if err := waitForFile(started, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		consumed = true
		t.Fatalf("worker.run returned before fixture release: %+v", got)
	case <-time.After(30 * time.Millisecond):
	}
	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		consumed = true
		if got.err != nil || !got.r.passed || !strings.Contains(got.r.output, "fixture-finished") || got.r.elapsed < 20*time.Millisecond {
			t.Fatalf("worker.run result = %+v, err = %v", got.r, got.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("worker.run did not return after fixture release")
	}
	if _, err := os.Stat(finished); err != nil {
		t.Fatalf("completion marker missing when worker.run returned: %v", err)
	}
}

// TestCommandRunnerFixtureHelper runs only as the child command started by
// the characterization tests above; ordinary package test execution is inert.
func TestCommandRunnerFixtureHelper(t *testing.T) {
	switch os.Getenv(runnerFixtureMode) {
	case "":
		return
	case "setup":
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(os.Stdout, "cwd=%s\ncustom=%s\npath=%s\n", cwd, os.Getenv("ITOS_COMMAND_RUNNER_CUSTOM"), os.Getenv("ITOS_COMMAND_RUNNER_PATH"))
		fmt.Fprintln(os.Stderr, "stderr-marker")
	case "success":
		fmt.Fprintln(os.Stdout, "success-stdout")
		fmt.Fprintln(os.Stderr, "success-stderr")
	case "nonzero":
		fmt.Fprintln(os.Stdout, "nonzero-output")
		os.Exit(7)
	case "loop":
		for {
			time.Sleep(time.Second)
		}
	case "barrier":
		started := os.Getenv("ITOS_COMMAND_RUNNER_STARTED")
		release := os.Getenv("ITOS_COMMAND_RUNNER_RELEASE")
		finished := os.Getenv("ITOS_COMMAND_RUNNER_FINISHED")
		if err := os.WriteFile(started, []byte("started"), 0o600); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(release); err == nil {
				if err := os.WriteFile(finished, []byte("finished"), 0o600); err != nil {
					t.Fatal(err)
				}
				fmt.Fprintln(os.Stdout, "fixture-finished")
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal("fixture release was not signaled")
	}
}

func newCommandRunnerWorker(t *testing.T) *worker {
	t.Helper()
	return &worker{dir: t.TempDir(), copies: make(map[string]string)}
}

func commandRunnerHelperArgs() []string {
	path, err := os.Executable()
	if err != nil {
		panic(err)
	}
	return []string{path, "-test.run=^TestCommandRunnerFixtureHelper$"}
}

func waitForFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", path)
}
