//go:build linux || darwin

package mutate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The ownership checks Linux and macOS share. Each scenario's test, in the
// Linux or the macOS file, runs the same check, so neither platform gets a
// weaker assertion.

const (
	ownershipModeEnv    = "ITOS_OWNERSHIP_MODE"
	ownershipRoleEnv    = "ITOS_OWNERSHIP_ROLE"
	ownershipChildEnv   = "ITOS_OWNERSHIP_CHILD_PID"
	ownershipGrandEnv   = "ITOS_OWNERSHIP_GRANDCHILD_PID"
	ownershipWorkEnv    = "ITOS_OWNERSHIP_LATE_WORK"
	ownershipReleaseEnv = "ITOS_OWNERSHIP_RELEASE"
	ownershipUnrelated  = "ITOS_OWNERSHIP_UNRELATED_PATH"

	// sharedCleanupLimit is ADR-0021's one five-second cleanup deadline.
	sharedCleanupLimit = 5 * time.Second
)

// startUnrelatedProcess starts the ownership helper in a process group of
// its own: the same executable as the owned processes, which cleanup must
// still leave alone.
func startUnrelatedProcess(t *testing.T, root string) *exec.Cmd {
	t.Helper()
	unrelated := exec.Command(os.Args[0], "-test.run=^TestOwnershipFixtureHelper$")
	unrelated.Env = append(os.Environ(), ownershipRoleEnv+"=unrelated", ownershipUnrelated+"="+filepath.Join(root, "unrelated.pid"))
	unrelated.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := unrelated.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unrelated.Process.Kill(); _, _ = unrelated.Process.Wait() })
	return unrelated
}

func checkOwnTimeoutStopsOwnedTree(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	childPID := filepath.Join(root, "child.pid")
	grandchildPID := filepath.Join(root, "grandchild.pid")
	t.Setenv(ownershipModeEnv, "timeout")
	t.Setenv(ownershipRoleEnv, "parent")
	t.Setenv(ownershipChildEnv, childPID)
	t.Setenv(ownershipGrandEnv, grandchildPID)
	unrelated := startUnrelatedProcess(t, root)
	w := newCommandRunnerWorker(t)
	cleanupOwnershipFixture(t, childPID, grandchildPID)
	copyDir := filepath.Join(root, "worker-copy")
	if err := os.Mkdir(copyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := Command{Root: copyDir, Dir: copyDir, Args: ownershipHelperArgs()}

	started := time.Now()
	got, err := w.run(cmd, 300*time.Millisecond)
	took := time.Since(started)
	if err != nil || !got.timedOut || got.passed {
		t.Fatalf("own timeout result = %+v, err = %v; want timed out", got, err)
	}
	child := readPID(t, childPID)
	grandchild := readPID(t, grandchildPID)
	if processCanRun(child) || processCanRun(grandchild) {
		t.Fatalf("owned process tree remains after timeout: child=%d grandchild=%d", child, grandchild)
	}
	if took > 300*time.Millisecond+sharedCleanupLimit+time.Second {
		t.Errorf("own timeout returned after %v, want its cleanup bounded by the five-second deadline", took)
	}
	if err := os.RemoveAll(copyDir); err != nil {
		t.Errorf("worker copy cannot be removed after the timeout: %v", err)
	}

	if !processCanRun(unrelated.Process.Pid) {
		t.Fatal("unrelated process was affected by owned group cleanup")
	}
}

func checkNormalReturnStopsOwnedDescendant(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	childPID := filepath.Join(root, "child.pid")
	grandchildPID := filepath.Join(root, "grandchild.pid")
	lateWork := filepath.Join(root, "late-work")
	release := filepath.Join(root, "release")
	t.Setenv(ownershipModeEnv, "normal")
	t.Setenv(ownershipRoleEnv, "parent")
	t.Setenv(ownershipChildEnv, childPID)
	t.Setenv(ownershipGrandEnv, grandchildPID)
	t.Setenv(ownershipWorkEnv, lateWork)
	t.Setenv(ownershipReleaseEnv, release)
	w := newCommandRunnerWorker(t)
	cleanupOwnershipFixture(t, childPID, grandchildPID)
	cmd := Command{Root: root, Dir: root, Args: ownershipHelperArgs()}

	got, err := w.run(cmd, 3*time.Second)
	if err != nil || !got.passed || got.exitCode != 0 {
		t.Fatalf("normal command result = %+v, err = %v; want successful parent exit", got, err)
	}
	child := readPID(t, childPID)
	grandchild := readPID(t, grandchildPID)
	if processCanRun(child) || processCanRun(grandchild) {
		t.Fatalf("owned descendant remains after normal return: child=%d grandchild=%d", child, grandchild)
	}
	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := os.Stat(lateWork); err == nil {
		t.Fatal("owned descendant continued fixture work after command return")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func checkOwnershipStartupFailureDoesNotRunCommand(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	started := filepath.Join(root, "started")
	w := newCommandRunnerWorker(t)
	startErr := fmt.Errorf("injected ownership setup failure")
	w.runner.lifecycle.start = func(_ context.Context, _ *exec.Cmd) error {
		return startErr
	}

	_, err := w.run(Command{Root: root, Dir: root, Args: []string{"/bin/sh", "-c", "touch " + started}}, time.Second)
	if err != startErr {
		t.Fatalf("startup error = %v, want injected ownership error", err)
	}
	if _, err := os.Stat(started); !os.IsNotExist(err) {
		t.Fatalf("command ran after ownership setup failed (stat error %v)", err)
	}
}

// checkParentCancellationStopsOwnedTree cancels a supervised command's
// parent context with the shared run-abort budget given to its runner.
func checkParentCancellationStopsOwnedTree(t *testing.T, sharedBudget *cleanupBudget) {
	t.Helper()
	root := t.TempDir()
	childPID := filepath.Join(root, "child.pid")
	grandchildPID := filepath.Join(root, "grandchild.pid")
	started := filepath.Join(root, "started")
	t.Setenv(ownershipModeEnv, "timeout")
	t.Setenv(ownershipRoleEnv, "parent")
	t.Setenv(ownershipChildEnv, childPID)
	t.Setenv(ownershipGrandEnv, grandchildPID)
	t.Setenv(runnerStartedPathEnv, started)
	w := newCommandRunnerWorker(t)
	w.runner.cleanupBudget = sharedBudget
	cleanupOwnershipFixture(t, childPID, grandchildPID)
	parent, cancel := context.WithCancel(context.Background())
	type runResult struct {
		value result
		err   error
	}
	done := make(chan runResult, 1)
	go func() {
		value, err := w.runContext(parent, Command{Root: root, Dir: root, Args: ownershipHelperArgs()}, 10*time.Second)
		done <- runResult{value: value, err: err}
	}()
	if err := waitForFile(started, 2*time.Second); err != nil {
		cancel()
		t.Fatal(err)
	}
	if err := waitForFile(grandchildPID, 2*time.Second); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	select {
	case got := <-done:
		if got.err != nil || !got.value.cancelled || got.value.timedOut || got.value.passed {
			t.Fatalf("parent cancellation result = %+v, err = %v; want cancelled, not timeout or pass", got.value, got.err)
		}
	case <-time.After(sharedCleanupLimit + time.Second):
		t.Fatal("parent-cancelled command did not finish within cleanup bound")
	}
	if processCanRun(readPID(t, childPID)) || processCanRun(readPID(t, grandchildPID)) {
		t.Fatal("owned process tree remained after parent cancellation")
	}
}

func checkCompletedExitPrecedesLaterCancellation(t *testing.T) {
	t.Helper()
	for _, test := range []struct {
		name string
		mode string
		code int
	}{
		{name: "success", mode: "success", code: 0},
		{name: "nonzero", mode: "nonzero", code: 7},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(runnerFixtureMode, test.mode)
			w := newCommandRunnerWorker(t)
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			w.runner.lifecycle.complete = func(context.Context, *exec.Cmd, error) { cancel() }
			root := t.TempDir()
			got, err := w.runContext(parent, Command{Root: root, Dir: root, Args: commandRunnerHelperArgs()}, 0)
			if err != nil || got.cancelled || got.timedOut || got.exitCode != test.code || got.passed != (test.code == 0) {
				t.Fatalf("completed command result = %+v, err = %v; want actual exit %d before later cancellation", got, err, test.code)
			}
		})
	}
}

func checkCleanupFailureIsReturnedNotJudged(t *testing.T) {
	t.Helper()
	t.Setenv(runnerFixtureMode, "success")
	w := newCommandRunnerWorker(t)
	cleanupErr := fmt.Errorf("injected cleanup failure")
	w.runner.lifecycle.wait = func(_ context.Context, cmd *exec.Cmd) error {
		if err := cmd.Wait(); err != nil {
			return err
		}
		return cleanupErr
	}
	root := t.TempDir()
	got, err := w.run(Command{Root: root, Dir: root, Args: commandRunnerHelperArgs()}, time.Second)
	if err != cleanupErr {
		t.Fatalf("cleanup error = %v, want %v", err, cleanupErr)
	}
	if got.passed || got.timedOut || got.cancelled {
		t.Fatalf("cleanup failure produced a judgment: %+v", got)
	}
}

// checkOnlyCapturedGroupIsTargeted runs a command whose owned descendants
// outlive its successful exit beside an unrelated process of the same
// executable: cleanup stops the captured group's members and nothing else.
func checkOnlyCapturedGroupIsTargeted(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	childPID := filepath.Join(root, "child.pid")
	grandchildPID := filepath.Join(root, "grandchild.pid")
	t.Setenv(ownershipModeEnv, "normal")
	t.Setenv(ownershipRoleEnv, "parent")
	t.Setenv(ownershipChildEnv, childPID)
	t.Setenv(ownershipGrandEnv, grandchildPID)
	t.Setenv(ownershipWorkEnv, filepath.Join(root, "late-work"))
	t.Setenv(ownershipReleaseEnv, filepath.Join(root, "release"))
	unrelated := startUnrelatedProcess(t, root)
	w := newCommandRunnerWorker(t)
	cleanupOwnershipFixture(t, childPID, grandchildPID)

	got, err := w.run(Command{Root: root, Dir: root, Args: ownershipHelperArgs()}, 3*time.Second)
	if err != nil || !got.passed {
		t.Fatalf("command result = %+v, err = %v; want its successful exit", got, err)
	}
	if processCanRun(readPID(t, childPID)) || processCanRun(readPID(t, grandchildPID)) {
		t.Fatal("the captured owned group was not cleaned up")
	}
	if !processCanRun(unrelated.Process.Pid) {
		t.Fatal("cleanup reached a process outside the captured group that runs the same executable")
	}
}

// checkParentAbortSharesOneCleanupDeadline also checks that the parent's
// abort starts the shared run-abort deadline, which later scopes keep.
func checkParentAbortSharesOneCleanupDeadline(t *testing.T) {
	t.Helper()
	sharedBudget := &cleanupBudget{}
	checkParentCancellationStopsOwnedTree(t, sharedBudget)
	if sharedBudget.deadline.IsZero() {
		t.Fatal("parent abort did not activate the shared cleanup deadline")
	}
	if deadline := sharedBudget.until(); !deadline.Equal(sharedBudget.deadline) {
		t.Fatalf("parent abort cleanup deadline changed across scopes: %v then %v", sharedBudget.deadline, deadline)
	}
}

func checkSuccessKeepsRunAbortBudget(t *testing.T) {
	t.Helper()
	t.Setenv(runnerFixtureMode, "success")
	w := newCommandRunnerWorker(t)
	sharedBudget := &cleanupBudget{}
	w.runner.cleanupBudget = sharedBudget
	root := t.TempDir()
	got, err := w.run(Command{Root: root, Dir: root, Args: commandRunnerHelperArgs()}, 0)
	if err != nil || !got.passed {
		t.Fatalf("successful command = %+v, err = %v", got, err)
	}
	if !sharedBudget.deadline.IsZero() {
		t.Fatalf("ordinary success consumed the shared run-abort cleanup budget: deadline %v", sharedBudget.deadline)
	}
}

func checkOwnTimeoutKeepsRunAbortBudget(t *testing.T) {
	t.Helper()
	t.Setenv(runnerFixtureMode, "loop")
	w := newCommandRunnerWorker(t)
	sharedBudget := &cleanupBudget{}
	w.runner.cleanupBudget = sharedBudget
	root := t.TempDir()
	got, err := w.run(Command{Root: root, Dir: root, Args: commandRunnerHelperArgs()}, 60*time.Millisecond)
	if err != nil || !got.timedOut || got.passed {
		t.Fatalf("own timeout = %+v, err = %v; want own timeout judgment", got, err)
	}
	if !sharedBudget.deadline.IsZero() {
		t.Fatalf("own mutant timeout consumed the shared run-abort cleanup budget: deadline %v", sharedBudget.deadline)
	}
}

func checkCleanupScopesShareOneDeadline(t *testing.T) {
	t.Helper()
	budget := &cleanupBudget{}
	first, second := budget.until(), budget.until()
	if !first.Equal(second) {
		t.Fatalf("cleanup deadlines = %v and %v; want one shared deadline", first, second)
	}
	if limit := time.Until(first); limit > sharedCleanupLimit || commandCleanupLimit != sharedCleanupLimit {
		t.Fatalf("cleanup deadline %v away, limit %v; want ADR-0021's five seconds", limit, commandCleanupLimit)
	}
}

// TestOwnershipFixtureHelper only runs as an owned child or grandchild.
func TestOwnershipFixtureHelper(t *testing.T) {
	switch os.Getenv(ownershipRoleEnv) {
	case "child":
		grandchild := exec.Command(os.Args[0], "-test.run=^TestOwnershipFixtureHelper$")
		grandchild.Env = append(os.Environ(), ownershipRoleEnv+"=grandchild")
		grandchild.Stdout, grandchild.Stderr = os.Stdout, os.Stderr
		if err := grandchild.Start(); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(os.Getenv(ownershipGrandEnv), []byte(strconv.Itoa(grandchild.Process.Pid)), 0o600); err != nil {
			t.Fatal(err)
		}
		_ = grandchild.Process.Release()
		for {
			time.Sleep(10 * time.Millisecond)
		}
	case "grandchild":
		if os.Getenv(ownershipModeEnv) == "normal" {
			for {
				if _, err := os.Stat(os.Getenv(ownershipReleaseEnv)); err == nil {
					_ = os.WriteFile(os.Getenv(ownershipWorkEnv), []byte("continued"), 0o600)
					return
				}
				time.Sleep(5 * time.Millisecond)
			}
		}
		for {
			time.Sleep(10 * time.Millisecond)
		}
	case "parent":
		child := exec.Command(os.Args[0], "-test.run=^TestOwnershipFixtureHelper$")
		child.Env = append(os.Environ(), ownershipRoleEnv+"=child")
		if os.Getenv(ownershipModeEnv) != "normal" {
			child.Stdout, child.Stderr = os.Stdout, os.Stderr
		}
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(os.Getenv(ownershipChildEnv), []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			t.Fatal(err)
		}
		_ = child.Process.Release()
		grandchildDeadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(grandchildDeadline) {
			if _, err := os.Stat(os.Getenv(ownershipGrandEnv)); err == nil {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if _, err := os.Stat(os.Getenv(ownershipGrandEnv)); err != nil {
			t.Fatalf("grandchild did not start: %v", err)
		}
		if started := os.Getenv(runnerStartedPathEnv); started != "" {
			if err := os.WriteFile(started, []byte("started"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if os.Getenv(ownershipModeEnv) == "normal" {
			return
		}
		for {
			time.Sleep(10 * time.Millisecond)
		}
	case "unrelated":
		if err := os.WriteFile(os.Getenv(ownershipUnrelated), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
			t.Fatal(err)
		}
		for {
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func ownershipHelperArgs() []string {
	return []string{os.Args[0], "-test.run=^TestOwnershipFixtureHelper$"}
}

func readPID(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture pid %s: %v", path, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parse fixture pid %s: %v", path, err)
	}
	return pid
}

// processCanRun reports whether pid is a process that has not exited: one
// the kernel still lists that is no zombie. It works the same on Linux and
// macOS, which has no /proc. A ps that cannot answer counts as running, so
// a broken probe fails a check rather than passing it.
func processCanRun(pid int) bool {
	if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
		return false
	}
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	state := strings.TrimSpace(string(out))
	var exit *exec.ExitError
	if errors.As(err, &exit) && state == "" {
		return false // ps lists no such process
	}
	if err != nil {
		return true
	}
	return !strings.HasPrefix(state, "Z") && !strings.HasPrefix(state, "X")
}

func cleanupOwnershipFixture(t *testing.T, childPath, grandchildPath string) {
	t.Helper()
	t.Cleanup(func() {
		for _, path := range []string{childPath, grandchildPath} {
			if data, err := os.ReadFile(path); err == nil {
				if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
					_ = syscall.Kill(pid, syscall.SIGKILL)
				}
			}
		}
	})
}
