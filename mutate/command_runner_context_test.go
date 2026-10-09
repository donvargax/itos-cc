package mutate

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	runnerStartedPathEnv = "ITOS_COMMAND_RUNNER_STARTED_PATH"
	contextValueKey      = "runner-context-value"
)

// These tests exercise capabilities introduced by T-10. They are not red
// evidence against the original worker API.
func TestLegacyWorkerAdapterUsesBackgroundContext(t *testing.T) {
	t.Setenv(runnerFixtureMode, "success")
	w := newCommandRunnerWorker(t)
	root := t.TempDir()
	observedBackground := false
	w.runner.lifecycle.wait = func(ctx context.Context, cmd *exec.Cmd) error {
		observedBackground = ctx.Done() == nil && ctx.Err() == nil
		return cmd.Wait()
	}

	r, err := w.run(Command{Root: root, Dir: root, Args: commandRunnerHelperArgs()}, 0)
	if err != nil || !r.passed {
		t.Fatalf("legacy worker run = %+v, err = %v", r, err)
	}
	if !observedBackground {
		t.Fatal("legacy worker adapter did not pass a Background context to command execution")
	}
}

func TestCommandRunnerAcceptsParentContext(t *testing.T) {
	root := t.TempDir()
	started := filepath.Join(root, "started")
	t.Setenv(runnerFixtureMode, "loop-marker")
	t.Setenv(runnerStartedPathEnv, started)

	parent, cancel := context.WithCancel(context.WithValue(context.Background(), contextValueKey, "parent-value"))
	defer cancel()
	w := newCommandRunnerWorker(t)
	observedValue := ""
	w.runner.lifecycle.wait = func(ctx context.Context, cmd *exec.Cmd) error {
		observedValue, _ = ctx.Value(contextValueKey).(string)
		return cmd.Wait()
	}

	type runResult struct {
		r   result
		err error
	}
	done := make(chan runResult, 1)
	go func() {
		r, err := w.runContext(parent, Command{Root: root, Dir: root, Args: commandRunnerContextHelperArgs()}, 0)
		done <- runResult{r: r, err: err}
	}()
	if err := waitForFile(started, 2*time.Second); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("parent-cancelled command returned error: %v", got.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("parent context did not reach the command; helper did not return after cancellation")
	}
	if observedValue != "parent-value" {
		t.Fatalf("lifecycle observation got parent value %q, want %q", observedValue, "parent-value")
	}
}

func TestCommandRunnerAcceptsInstanceLocalLifecycleOperations(t *testing.T) {
	t.Setenv(runnerFixtureMode, "success")
	first, second := newCommandRunnerWorker(t), newCommandRunnerWorker(t)
	firstStarts, firstWaits, firstCompletions, secondCompletions := 0, 0, 0, 0
	first.runner.lifecycle.start = func(_ context.Context, cmd *exec.Cmd) error {
		firstStarts++
		return cmd.Start()
	}
	first.runner.lifecycle.wait = func(_ context.Context, cmd *exec.Cmd) error {
		firstWaits++
		return cmd.Wait()
	}
	first.runner.lifecycle.complete = func(_ context.Context, cmd *exec.Cmd, _ error) {
		firstCompletions++
		if cmd.ProcessState == nil {
			t.Error("first runner observed completion before Wait populated ProcessState")
		}
	}
	second.runner.lifecycle.complete = func(_ context.Context, cmd *exec.Cmd, _ error) {
		secondCompletions++
		if cmd.ProcessState == nil {
			t.Error("second runner observed completion before Wait populated ProcessState")
		}
	}

	for _, w := range []*worker{first, second} {
		root := t.TempDir()
		r, err := w.run(Command{Root: root, Dir: root, Args: commandRunnerHelperArgs()}, time.Second*10)
		if err != nil || !r.passed {
			t.Fatalf("runner execution = %+v, err = %v", r, err)
		}
	}
	if firstStarts != 1 || firstWaits != 1 || firstCompletions != 1 || secondCompletions != 1 {
		t.Fatalf("instance-local lifecycle calls = start %d, wait %d, first complete %d, second complete %d; want one call per configured operation", firstStarts, firstWaits, firstCompletions, secondCompletions)
	}
}

func TestCommandRunnerKeepsPreconfiguredStreams(t *testing.T) {
	t.Setenv(runnerFixtureMode, "streams")
	w := newCommandRunnerWorker(t)
	var stdout, stderr, resultOutput bytes.Buffer
	args := commandRunnerContextHelperArgs()
	cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
	cmd.Dir = t.TempDir()
	cmd.Env = os.Environ()
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	r, err := w.runner.run(context.Background(), cmd, &resultOutput)
	if err != nil || !r.passed {
		t.Fatalf("preconfigured command = %+v, err = %v", r, err)
	}
	if !strings.Contains(stdout.String(), "preconfigured-stdout") {
		t.Errorf("stdout stream %q was not preserved", stdout.String())
	}
	if !strings.Contains(stderr.String(), "preconfigured-stderr") {
		t.Errorf("stderr stream %q was not preserved", stderr.String())
	}
	if cmd.Stdout != &stdout || cmd.Stderr != &stderr {
		t.Fatal("runner replaced one or both preconfigured command streams")
	}
}

func TestCommandRunnerContextFixtureHelper(t *testing.T) {
	switch os.Getenv(runnerFixtureMode) {
	case "":
		return
	case "loop-marker":
		if err := os.WriteFile(os.Getenv(runnerStartedPathEnv), []byte("started"), 0o600); err != nil {
			t.Fatal(err)
		}
		for {
			time.Sleep(10 * time.Millisecond)
		}
	case "streams":
		fmt.Fprintln(os.Stdout, "preconfigured-stdout")
		fmt.Fprintln(os.Stderr, "preconfigured-stderr")
	}
}

func commandRunnerContextHelperArgs() []string {
	path, err := os.Executable()
	if err != nil {
		panic(err)
	}
	return []string{path, "-test.run=^TestCommandRunnerContextFixtureHelper$"}
}
