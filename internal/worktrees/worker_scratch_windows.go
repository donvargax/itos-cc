//go:build windows

package worktrees

import (
	"golang.org/x/sys/windows"
	"os"
	"syscall"
)

const lockConflict = syscall.Errno(33)

func nativeFileLock(f *os.File) error {
	var overlapped windows.Overlapped
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_FAIL_IMMEDIATELY|windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped)
}
