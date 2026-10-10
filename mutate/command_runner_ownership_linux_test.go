//go:build linux

package mutate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The timeout, normal-return and startup tests were committed and run against
// T-10 before supervision changed. The cancellation, precedence, cleanup-error
// and shared-budget checks below were added after implementation; they verify
// the newly reachable lifecycle capabilities, not pre-change behavioral reds.
// Their bodies live in command_runner_ownership_unix_test.go, which macOS
// shares.

func TestLinuxOwnTimeoutStopsOwnedProcessTree(t *testing.T) {
	checkOwnTimeoutStopsOwnedTree(t)
}

func TestLinuxNormalReturnStopsOwnedDescendant(t *testing.T) {
	checkNormalReturnStopsOwnedDescendant(t)
}

func TestLinuxOwnershipStartupFailureDoesNotRunCommand(t *testing.T) {
	checkOwnershipStartupFailureDoesNotRunCommand(t)
}

func TestLinuxParentCancellationStopsOwnedTreeWithoutTimeoutJudgment(t *testing.T) {
	sharedBudget := &cleanupBudget{}
	checkParentCancellationStopsOwnedTree(t, sharedBudget)
	if sharedBudget.deadline.IsZero() {
		t.Fatal("parent abort did not activate the shared cleanup deadline")
	}
	if deadline := sharedBudget.until(); !deadline.Equal(sharedBudget.deadline) {
		t.Fatalf("parent abort cleanup deadline changed across scopes: %v then %v", sharedBudget.deadline, deadline)
	}
}

func TestLinuxSuccessDoesNotActivateRunAbortCleanupBudget(t *testing.T) {
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

func TestLinuxOwnTimeoutDoesNotActivateRunAbortCleanupBudget(t *testing.T) {
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

func TestLinuxCompletedExitPrecedesLaterCancellation(t *testing.T) {
	checkCompletedExitPrecedesLaterCancellation(t)
}

func TestLinuxCleanupFailureIsReturnedNotJudged(t *testing.T) {
	checkCleanupFailureIsReturnedNotJudged(t)
}

func TestLinuxCleanupScopesCanShareOneDeadline(t *testing.T) {
	budget := &cleanupBudget{}
	first, second := budget.until(), budget.until()
	if !first.Equal(second) {
		t.Fatalf("cleanup deadlines = %v and %v; want one shared deadline", first, second)
	}
	if limit := first.Sub(time.Now()); limit > sharedCleanupLimit || commandCleanupLimit != sharedCleanupLimit {
		t.Fatalf("cleanup deadline %v away, limit %v; want ADR-0021's five seconds", limit, commandCleanupLimit)
	}
}

// The Linux step of ID-MUT-190: Linux keeps its supervision unchanged. Its
// probe scans /proc and counts a group whose only member is a zombie as
// empty, rather than waiting for kill(-pgid, 0) to fail, which a zombie
// nobody reaps would hold off until the cleanup deadline.
//
// @ID-MUT-190
func TestLinuxProbeCountsAZombieOnlyGroupEmpty(t *testing.T) {
	member := exec.Command("/bin/sh", "-c", "exit 0")
	member.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := member.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = member.Wait() }()
	pid := member.Process.Pid
	stat := filepath.Join("/proc", strconv.Itoa(pid), "stat")
	for deadline := time.Now().Add(2 * time.Second); ; {
		data, err := os.ReadFile(stat)
		if err != nil {
			t.Fatal(err)
		}
		if fields := strings.Fields(string(data[strings.LastIndexByte(string(data), ')')+1:])); len(fields) > 0 && fields[0] == "Z" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the group member did not become a zombie")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := syscall.Kill(-pid, 0); err != nil {
		t.Fatalf("kill(-pgid, 0) = %v, want the zombie still to hold its group", err)
	}
	started := time.Now()
	if err := waitOwnedProcessGroup(pid, time.Now().Add(2*time.Second)); err != nil {
		t.Fatalf("zombie-only group: %v, want it counted empty", err)
	}
	if took := time.Since(started); took > time.Second {
		t.Fatalf("the probe waited %v on a zombie-only group, want it counted empty at once", took)
	}
}
