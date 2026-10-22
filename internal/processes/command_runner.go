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
// an error only for a start, wait or cleanup that failed. It caps nothing.
func (runner *Runner) Run(ctx context.Context, cmd *exec.Cmd, output *bytes.Buffer) (Result, error) {
	return runner.RunLimited(ctx, cmd, output, Limits{})
}

// RunLimited is Run under limits: a command over a cap is killed, its
// owned group joined as at its own timeout, within a cleanup budget of its
// own, and judged OverLimit, unless its parent cancelled it. What cmd
// writes to output is kept bounded (Result.Output) and written to output
// as kept once the command has ended; cmd's streams are given back as they
// were.
func (runner *Runner) RunLimited(ctx context.Context, cmd *exec.Cmd, output *bytes.Buffer, limits Limits) (Result, error) {
	platform := PlatformAtBoundary(runner.Platform)
	kept := &keptOutput{limit: limits.Output}
	originalStdout, originalStderr := cmd.Stdout, cmd.Stderr
	defer func() { cmd.Stdout, cmd.Stderr = originalStdout, originalStderr }()
	if output != nil && cmd.Stdout == output {
		cmd.Stdout = kept
	}
	if output != nil && cmd.Stderr == output {
		cmd.Stderr = kept
	}
	s := &supervision{kept: kept, limits: limits, platform: platform}
	var started time.Time
	var err error
	waited := false
	if runner.Lifecycle.start == nil && runner.Lifecycle.wait == nil {
		started = time.Now()
		err = runOwnedCommand(ctx, cmd, s, runner.Budget, runner.Lifecycle.Cleanup, runner.Hold)
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
	elapsed := time.Since(started)
	text, written, cut, leftOut := kept.kept()
	if output != nil {
		output.Reset()
		output.WriteString(text)
	}
	if runner.Lifecycle.Complete != nil {
		runner.Lifecycle.Complete(ctx, cmd, err)
	}
	r := Result{Elapsed: elapsed, Output: text, OutputBytes: written, OutputCut: cut, OutputLeftOut: leftOut, ExitCode: -1,
		PeakMemory: s.peak, MemoryMeasured: s.measured}
	if ownedOutcomePrecedence(platform) && err != nil && !isExit(err) {
		return r, err
	}
	// A cap the command went over killed it, whatever it then exited
	// with, unless its parent cancelled it: a judgment cut short is never
	// a kill.
	parentCancelled := ctx.Err() != nil && !errors.Is(context.Cause(ctx), ErrCommandTimeout)
	if over := s.over.which(); over != "" && !parentCancelled {
		r.OverLimit = over
		if cmd.ProcessState != nil {
			r.ExitCode = cmd.ProcessState.ExitCode()
		}
		return r, nil
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

// supervision is what one command's run is held to and what it used: its
// kept output, the caps of its limits and the one it went over, and, where
// the system measures it, its owned group's peak memory.
type supervision struct {
	kept     *keptOutput
	limits   Limits
	platform string
	over     overLimit
	peak     int64
	measured bool
}
