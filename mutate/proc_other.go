//go:build !linux && !darwin

package mutate

import (
	"bytes"
	"context"
	"os/exec"
)

type cleanupBudget struct{}

func ownedOutcomePrecedence() bool { return false }

// Platforms other than Linux and macOS retain their existing command
// lifecycle. Windows Job ownership is a separate item (#29).
func runOwnedCommand(_ context.Context, cmd *exec.Cmd, _ *bytes.Buffer, _ *cleanupBudget) error {
	killGroup(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}
