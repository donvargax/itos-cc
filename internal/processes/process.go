// Package processes is the process-ownership engine (ADR-0021): it runs an
// already configured command in a process group it owns, and returns only
// once the group's members and the readers of its output have joined, or
// reports the cleanup that did not finish by its deadline. An interrupt
// waits for the groups being killed and joined (decision 38). Mutation's
// workers, counted preparation and crap's coverage command run through it;
// it imports nothing of ours, so a caller converts its Result.
package processes

import (
	"errors"
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

func killGroup(cmd *exec.Cmd, platform string) {
	if platform != "windows" {
		cmd.SysProcAttr = nativeGroupAttributes(true)
		cmd.Cancel = func() error { return cancelGroup(cmd.Process.Pid) }
	}
	cmd.WaitDelay = 2 * time.Second
}

// cancelGroup kills the command's group when its context ends. A group
// already gone is os.ErrProcessDone (bug-5): the command exited before the
// cancel landed, and os/exec then reports its own result, never the kill's
// ESRCH as Wait's error.
func cancelGroup(pgid int) error {
	err := nativeSignalGroup(-pgid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
