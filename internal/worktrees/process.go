package worktrees

import (
	"context"
	"os/exec"
	"runtime"
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
		cmd.SysProcAttr = nativeGroupAttributes()
		cmd.Cancel = func() error { return nativeKillGroup(cmd.Process.Pid) }
	}
	cmd.WaitDelay = 2 * time.Second
}
func shellCommand(ctx context.Context, line, platform string) *exec.Cmd {
	if platform == "windows" {
		return exec.CommandContext(ctx, "cmd", "/C", line)
	}
	return exec.CommandContext(ctx, "sh", "-c", line)
}
