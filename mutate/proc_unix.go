//go:build !windows

package mutate

import (
	"context"
	"os/exec"
	"syscall"
	"time"
)

// killGroup makes a timeout kill the test command and every process it
// started: test runners fork workers that would otherwise outlive it.
func killGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
}

func shellCommand(ctx context.Context, line string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", line)
}
