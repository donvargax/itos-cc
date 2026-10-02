//go:build windows

package mutate

import (
	"context"
	"os/exec"
	"time"
)

// killGroup bounds how long a timed-out command's children may hold its
// output open. Windows has no process groups to kill at once.
func killGroup(cmd *exec.Cmd) {
	cmd.WaitDelay = 2 * time.Second
}

func shellCommand(ctx context.Context, line string) *exec.Cmd {
	return exec.CommandContext(ctx, "cmd", "/C", line)
}
