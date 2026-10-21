package processes

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

// ErrCommandTimeout is the cause a caller gives a command's own timeout
// (context.WithTimeoutCause): the runner judges it TimedOut, never a
// parent's cancellation, and its cleanup never takes a shared budget.
var ErrCommandTimeout = errors.New("mutation command timed out")

// Runner executes an already configured command for one caller.
// Its zero value selects the platform's default command ownership behavior.
type Runner struct {
	Platform  string
	Lifecycle Lifecycle
	Budget    *CleanupBudget // Shared run-abort budget; per-command cleanup never activates it.
	// Hold marks the command's owned group being killed and joined, so a
	// later interrupt waits for it (decision 38); nil marks nothing.
	Hold func() func()
}

// Lifecycle can replace the start/wait boundary for one runner, and lets
// a caller's tests observe it; production leaves it zero.
type Lifecycle struct {
	// Cleanup observes the actual resolved deadline for focused adapter guards.
	Cleanup func(time.Time)
	// start replaces the default killGroup setup and Cmd.Start call. A custom
	// operation owns its own pre-start setup and starts the configured command.
	start func(context.Context, *exec.Cmd) error
	// wait replaces Cmd.Wait and is called only after start succeeds.
	wait func(context.Context, *exec.Cmd) error
	// Complete observes the finished start/wait attempt before result mapping.
	Complete func(context.Context, *exec.Cmd, error)
}

// Run executes cmd under ctx, its output captured, and judges how it ended:
// an error only for a start, wait or cleanup that failed.
func (runner *Runner) Run(ctx context.Context, cmd *exec.Cmd, output *bytes.Buffer) (Result, error) {
	platform := PlatformAtBoundary(runner.Platform)
	var started time.Time
	var err error
	waited := false
	if runner.Lifecycle.start == nil && runner.Lifecycle.wait == nil {
		started = time.Now()
		err = runOwnedCommand(ctx, cmd, output, runner.Budget, platform, runner.Lifecycle.Cleanup, runner.Hold)
		waited = true
	} else if runner.Lifecycle.start != nil {
		started = time.Now()
		err = runner.Lifecycle.start(ctx, cmd)
	} else {
		killGroup(cmd, platform)
		started = time.Now()
		err = cmd.Start()
	}
	if err == nil && !waited {
		if runner.Lifecycle.wait != nil {
			err = runner.Lifecycle.wait(ctx, cmd)
		} else {
			err = cmd.Wait()
		}
	}
	elapsed, captured := time.Since(started), output.String()
	if runner.Lifecycle.Complete != nil {
		runner.Lifecycle.Complete(ctx, cmd, err)
	}
	r := Result{Elapsed: elapsed, Output: captured, ExitCode: -1}
	if ownedOutcomePrecedence(platform) && err != nil && !isExit(err) {
		return r, err
	}
	if ownedOutcomePrecedence(platform) && cmd.ProcessState != nil && cmd.ProcessState.ExitCode() >= 0 {
		r.ExitCode = cmd.ProcessState.ExitCode()
		r.Passed = r.ExitCode == 0
		return r, nil
	}
	switch {
	case ownedOutcomePrecedence(platform) && errors.Is(context.Cause(ctx), ErrCommandTimeout):
		r.TimedOut = true
	case !ownedOutcomePrecedence(platform) && ctx.Err() == context.DeadlineExceeded:
		r.TimedOut = true
	case ownedOutcomePrecedence(platform) && ctx.Err() != nil:
		r.Cancelled = true
	case err == nil:
		r.Passed, r.ExitCode = true, 0
	case isExit(err):
		r.ExitCode = err.(*exec.ExitError).ExitCode()
	default:
		return r, err
	}
	return r, nil
}
