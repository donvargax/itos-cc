// Package processes is the process-ownership engine (ADR-0021): it runs an
// already configured command in a process group it owns, and returns only
// once the group's members and the readers of its output have joined, or
// reports the cleanup that did not finish by its deadline. An interrupt
// waits for the groups being killed and joined (decision 38). Mutation's
// workers, counted preparation and crap's coverage command run through it;
// it imports nothing of ours, so a caller converts its Result.
package processes

import (
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"
)

// PlatformAtBoundary detects the host only for zero-value legacy adapters.
// All process policy below receives this value; cmd supplies it explicitly.
func PlatformAtBoundary(platform string) string {
	if platform == "" {
		return runtime.GOOS
	}
	return platform
}

func killGroup(cmd *exec.Cmd, platform string, calls groupCalls) {
	if platform != "windows" {
		cmd.SysProcAttr = nativeGroupAttributes(true)
		cmd.Cancel = func() error { return cancelGroup(calls, cmd.Process.Pid) }
	}
	cmd.WaitDelay = 2 * time.Second
}

// cancelGroup kills the command's group when its context ends. os/exec's Wait
// reports the command's own exit only when Cancel returns os.ErrProcessDone:
// nil becomes ctx.Err() and any other error wraps as "exec: canceling Cmd",
// even for a command that exited 0 before the kill landed (bug-5). No process
// survives SIGKILL, so the exit status alone tells whether the kill landed
// first; the cleanup's own kill and probe still see to what is left.
func cancelGroup(calls groupCalls, pgid int) error {
	_ = calls.signal(-pgid, syscall.SIGKILL)
	return os.ErrProcessDone
}
