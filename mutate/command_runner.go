package mutate

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

var errMutationTimeout = errors.New("mutation command timed out")

// commandRunner executes an already configured command for one worker.
// Its zero value selects the platform's default command ownership behavior.
type commandRunner struct {
	lifecycle     commandLifecycle
	cleanupBudget *cleanupBudget // Shared run-abort budget; per-command cleanup never activates it.
}

// commandLifecycle can replace the start/wait boundary for one worker runner.
type commandLifecycle struct {
	// start replaces the default killGroup setup and Cmd.Start call. A custom
	// operation owns its own pre-start setup and starts the configured command.
	start func(context.Context, *exec.Cmd) error
	// wait replaces Cmd.Wait and is called only after start succeeds.
	wait func(context.Context, *exec.Cmd) error
	// complete observes the finished start/wait attempt before result mapping.
	complete func(context.Context, *exec.Cmd, error)
}

func (runner *commandRunner) run(ctx context.Context, cmd *exec.Cmd, output *bytes.Buffer) (result, error) {
	var started time.Time
	var err error
	waited := false
	if runner.lifecycle.start == nil && runner.lifecycle.wait == nil {
		started = time.Now()
		err = runOwnedCommand(ctx, cmd, output, runner.cleanupBudget)
		waited = true
	} else if runner.lifecycle.start != nil {
		started = time.Now()
		err = runner.lifecycle.start(ctx, cmd)
	} else {
		killGroup(cmd)
		started = time.Now()
		err = cmd.Start()
	}
	if err == nil && !waited {
		if runner.lifecycle.wait != nil {
			err = runner.lifecycle.wait(ctx, cmd)
		} else {
			err = cmd.Wait()
		}
	}
	elapsed, captured := time.Since(started), output.String()
	if runner.lifecycle.complete != nil {
		runner.lifecycle.complete(ctx, cmd, err)
	}
	r := result{elapsed: elapsed, output: captured, exitCode: -1}
	if ownedOutcomePrecedence() && err != nil && !isExit(err) {
		return r, err
	}
	if ownedOutcomePrecedence() && cmd.ProcessState != nil && cmd.ProcessState.ExitCode() >= 0 {
		r.exitCode = cmd.ProcessState.ExitCode()
		r.passed = r.exitCode == 0
		return r, nil
	}
	switch {
	case ownedOutcomePrecedence() && errors.Is(context.Cause(ctx), errMutationTimeout):
		r.timedOut = true
	case !ownedOutcomePrecedence() && ctx.Err() == context.DeadlineExceeded:
		r.timedOut = true
	case ownedOutcomePrecedence() && ctx.Err() != nil:
		r.cancelled = true
	case err == nil:
		r.passed, r.exitCode = true, 0
	case isExit(err):
		r.exitCode = err.(*exec.ExitError).ExitCode()
	default:
		return r, err
	}
	return r, nil
}
