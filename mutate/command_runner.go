package mutate

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

// commandRunner executes an already configured command for one worker.
// Its zero value preserves the current platform killGroup behavior.
type commandRunner struct {
	lifecycle commandLifecycle
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
	if runner.lifecycle.start != nil {
		started = time.Now()
		err = runner.lifecycle.start(ctx, cmd)
	} else {
		killGroup(cmd)
		started = time.Now()
		err = cmd.Start()
	}
	if err == nil {
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
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		r.timedOut = true
	case err == nil:
		r.passed, r.exitCode = true, 0
	case isExit(err):
		r.exitCode = err.(*exec.ExitError).ExitCode()
	default:
		return r, err
	}
	return r, nil
}
