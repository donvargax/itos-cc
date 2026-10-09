package mutate

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

// commandRunner executes an already configured command for one worker.
// Its zero value preserves the current platform killGroup behavior.
type commandRunner struct{}

func (*commandRunner) run(ctx context.Context, cmd *exec.Cmd, output *bytes.Buffer) (result, error) {
	killGroup(cmd)
	start := time.Now()
	err := cmd.Run()
	r := result{elapsed: time.Since(start), output: output.String(), exitCode: -1}
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
