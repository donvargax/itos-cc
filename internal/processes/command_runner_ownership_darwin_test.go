//go:build darwin

package processes

import (
	"path/filepath"
	"testing"
	"time"
)

// The scenarios of "Rule: macOS mutation commands return only after their
// owned process trees are cleaned up" in features/mutate.feature. Each runs
// the same checks as its Linux counterpart (ID-MUT-165 to 168), from
// command_runner_ownership_unix_test.go, with helper processes only: no
// mutant trial. macOS has no /proc, so liveness comes from kill(pid, 0) and
// ps's process state.

// @ID-MUT-187
func TestMacOSOwnTimeoutStopsOwnedProcessTree(t *testing.T) {
	checkOwnTimeoutStopsOwnedTree(t)
	t.Run("run-abort budget untouched", checkOwnTimeoutKeepsRunAbortBudget)
}

// @ID-MUT-188
func TestMacOSNormalReturnStopsOwnedDescendant(t *testing.T) {
	checkNormalReturnStopsOwnedDescendant(t)
	t.Run("run-abort budget untouched", checkSuccessKeepsRunAbortBudget)
}

// @ID-MUT-189
func TestMacOSParentCancellationIsNotAMutantDeadline(t *testing.T) {
	t.Run("cancelled", checkParentAbortSharesOneCleanupDeadline)
	t.Run("one deadline across scopes", checkCleanupScopesShareOneDeadline)
	t.Run("completed first", checkCompletedExitPrecedesLaterCancellation)
}

// @ID-MUT-190
func TestMacOSSupervisionFailuresDoNotPermitUnsafeFallback(t *testing.T) {
	t.Run("startup", checkOwnershipStartupFailureDoesNotRunCommand)
	t.Run("cleanup", checkCleanupFailureIsReturnedNotJudged)
	t.Run("captured group only", checkOnlyCapturedGroupIsTargeted)
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

	got, err := w.run(t.Context(), Command{Root: root, Dir: root, Args: ownershipHelperArgs()}, 3*time.Second)
	if err != nil || !got.Passed {
		t.Fatalf("command result = %+v, err = %v; want its successful exit", got, err)
	}
	if processCanRun(readPID(t, childPID)) || processCanRun(readPID(t, grandchildPID)) {
		t.Fatal("the captured owned group was not cleaned up")
	}
	if !processCanRun(unrelated.Process.Pid) {
		t.Fatal("cleanup reached a process outside the captured group that runs the same executable")
	}
}
