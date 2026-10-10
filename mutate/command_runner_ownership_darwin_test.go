//go:build darwin

package mutate

import "testing"

// The scenarios of "Rule: macOS mutation commands return only after their
// owned process trees are cleaned up" in features/mutate.feature. Each runs
// the same check as its Linux counterpart (ID-MUT-165 to 168), from
// command_runner_ownership_unix_test.go, with helper processes only: no
// mutant trial. macOS has no /proc, so liveness comes from kill(pid, 0) and
// ps's process state.

// @ID-MUT-187
func TestMacOSOwnTimeoutStopsOwnedProcessTree(t *testing.T) {
	requireOwnedSupervision(t)
	checkOwnTimeoutStopsOwnedTree(t)
}

// @ID-MUT-188
func TestMacOSNormalReturnStopsOwnedDescendant(t *testing.T) {
	requireOwnedSupervision(t)
	checkNormalReturnStopsOwnedDescendant(t)
}

// @ID-MUT-189
func TestMacOSParentCancellationIsNotAMutantDeadline(t *testing.T) {
	requireOwnedSupervision(t)
	t.Run("cancelled", func(t *testing.T) { checkParentCancellationStopsOwnedTree(t, &cleanupBudget{}) })
	t.Run("completed first", checkCompletedExitPrecedesLaterCancellation)
}

// @ID-MUT-190
func TestMacOSSupervisionFailuresDoNotPermitUnsafeFallback(t *testing.T) {
	requireOwnedSupervision(t)
	t.Run("startup", checkOwnershipStartupFailureDoesNotRunCommand)
	t.Run("cleanup", checkCleanupFailureIsReturnedNotJudged)
	t.Run("captured group only", checkOnlyCapturedGroupIsTargeted)
}
