package worktrees

import (
	"context"
	"os/exec"
	"runtime"
	"syscall"
	"time"
)

// platformAtBoundary detects the host only for zero-value legacy adapters.
// All process policy below receives this value; cmd supplies it explicitly.
func platformAtBoundary(platform string) string {
	if platform == "" {
		return runtime.GOOS
	}
	return platform
}

func killGroup(cmd *exec.Cmd, platform string) {
	if platform != "windows" {
		cmd.SysProcAttr = nativeGroupAttributes(true)
		cmd.Cancel = func() error { return nativeSignalGroup(-cmd.Process.Pid, syscall.SIGKILL) }
	}
	cmd.WaitDelay = 2 * time.Second
}
func shellCommand(ctx context.Context, line, platform string) *exec.Cmd {
	if platform == "windows" {
		return exec.CommandContext(ctx, "cmd", "/C", line) //nolint:gosec // the person explicitly selected a shell test command
	}
	return exec.CommandContext(ctx, "sh", "-c", line) //nolint:gosec // the person explicitly selected a shell test command
}
