//go:build !linux

package mutate

import (
	"bytes"
	"context"
	"os/exec"
)

type cleanupBudget struct{}

func linuxOutcomePrecedence() bool { return false }

// Non-Linux platforms retain their existing command lifecycle. Windows Job
// ownership is intentionally implemented and verified in a separate item.
func runOwnedCommand(_ context.Context, cmd *exec.Cmd, _ *bytes.Buffer, _ *cleanupBudget) error {
	killGroup(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}
