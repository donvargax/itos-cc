package worktrees

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
	r, err := w.run(t.Context(), c, 0)
	if err != nil {
		t.Fatalf("worker.run: %v", err)
	}
	if !r.Passed || r.ExitCode != 0 {
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
	for _, line := range strings.Split(r.Output, "\n") {
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
		if !strings.Contains(r.Output, want) {
			t.Errorf("combined output %q does not contain %q", r.Output, want)
		}
	}

	shell := "printf shell-output"
	if runtime.GOOS == "windows" {
		shell = "echo shell-output"
	}
	r, err = w.run(t.Context(), Command{Root: root, Dir: root, Shell: shell}, 0)
	if err != nil || !r.Passed || !strings.Contains(r.Output, "shell-output") {
		t.Errorf("shell command result = %+v, err = %v; want captured shell output", r, err)
	}
}

// TestCommandRunnerFixtureHelper runs only as the child command started by
// the worker tests; ordinary package test execution is inert. The runner's
// own fixture, with every mode its tests use, is internal/processes'.
func TestCommandRunnerFixtureHelper(t *testing.T) {
	switch os.Getenv(runnerFixtureMode) {
	case "":
		return
	case "setup":
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fmt.Fprintf(os.Stdout, "cwd=%s\ncustom=%s\npath=%s\n", cwd, os.Getenv("ITOS_COMMAND_RUNNER_CUSTOM"), os.Getenv("ITOS_COMMAND_RUNNER_PATH")); err != nil {
			t.Fatal(err)
		}
		if _, err := fmt.Fprintln(os.Stderr, "stderr-marker"); err != nil {
			t.Fatal(err)
		}
	case "success":
		if _, err := fmt.Fprintln(os.Stdout, "success-stdout"); err != nil {
			t.Fatal(err)
		}
		if _, err := fmt.Fprintln(os.Stderr, "success-stderr"); err != nil {
			t.Fatal(err)
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
				if _, err := fmt.Fprintln(os.Stdout, "fixture-finished"); err != nil {
					t.Fatal(err)
				}
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
