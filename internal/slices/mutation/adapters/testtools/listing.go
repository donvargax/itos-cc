package testtools

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"

	"github.com/donvargax/itos-cc/internal/project"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// Lister returns stdout alone and streams diagnostics immediately to Log.
// Platform is supplied at composition, preserving the native shell selection.
type Lister struct {
	Platform string
	Log      io.Writer
}

func (l Lister) List(ctx context.Context, c mutants.Command) (mutants.TestListing, mutants.InputFailure) {
	if ctx.Err() != nil {
		return mutants.TestListing{}, mutants.InputCancelled{Cause: context.Cause(ctx)}
	}
	name, flag := "sh", "-c"
	if l.Platform == "windows" {
		name, flag = "cmd", "/C"
	}
	cmd := exec.CommandContext(ctx, name, flag, c.Shell) //nolint:gosec // execute the explicitly configured listing through its platform shell
	cmd.Dir, cmd.Env = c.Dir, project.InheritedEnvIn(c.Dir)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = l.Log
	err := cmd.Run()
	if ctx.Err() != nil {
		return mutants.TestListing{}, mutants.InputCancelled{Cause: context.Cause(ctx)}
	}
	out := mutants.TestListing{Stdout: stdout.String()}
	if err != nil {
		out.ExitCode = -1
		out.Reason = err.Error()
		var exited *exec.ExitError
		if errors.As(err, &exited) {
			out.ExitCode = exited.ExitCode()
		}
	}
	return out, nil
}
