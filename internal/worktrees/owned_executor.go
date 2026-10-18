package worktrees

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// OwnedExecutor supervises commands used by an opt-in preparation operation.
// Its cleanup budget is instance-local and shared only after parent abort.
// This does not change the legacy worker adapter or claim tree containment
// beyond Linux and macOS.
type OwnedExecutor struct {
	Platform string
	// Hold marks owned process groups being killed and joined, until the
	// release it returns is called, so an interrupt waits for them
	// (decision 38); cmd binds it per run, and nil marks nothing.
	Hold    func() (release func())
	cleanup cleanupBudget
}

// Run executes an already configured command without reconstructing its argv,
// directory, environment, or streams. Nonzero exits, start/wait/cleanup
// failures, and cancellation are all returned as errors.
func (e *OwnedExecutor) Run(ctx context.Context, cmd *exec.Cmd) error {
	if e == nil {
		return errors.New("owned executor must not be nil")
	}
	if cmd == nil {
		return errors.New("owned command must not be nil")
	}
	var output bytes.Buffer
	runner := commandRunner{cleanupBudget: &e.cleanup, platform: platformAtBoundary(e.Platform), hold: e.Hold}
	r, err := runner.run(ctx, cmd, &output)
	if err != nil {
		return fmt.Errorf("execute %s: %w", cmd.Path, err)
	}
	if r.Cancelled {
		if cause := context.Cause(ctx); cause != nil {
			return fmt.Errorf("execute %s cancelled: %w", cmd.Path, cause)
		}
		return fmt.Errorf("execute %s cancelled: %w", cmd.Path, ctx.Err())
	}
	if r.TimedOut {
		return fmt.Errorf("execute %s timed out", cmd.Path)
	}
	if !r.Passed {
		return fmt.Errorf("execute %s exited %d", cmd.Path, r.ExitCode)
	}
	return nil
}

// Command runs the already configured preparation baseline and returns its
// actual result using the same shared abort budget as subsequent trials.
func (e *OwnedExecutor) Command(ctx context.Context, cmd *exec.Cmd) (result, error) {
	var output bytes.Buffer
	runner := commandRunner{cleanupBudget: &e.cleanup, platform: platformAtBoundary(e.Platform), hold: e.Hold}
	return runner.run(ctx, cmd, &output)
}
