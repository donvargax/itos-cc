//go:build windows

package worktrees

import "golang.org/x/sys/windows"

const nativeLockExclusive = windows.LOCKFILE_EXCLUSIVE_LOCK
const nativeLockNonblocking = windows.LOCKFILE_FAIL_IMMEDIATELY

func nativeFileLock(fd uintptr, flags, reserved, low, high uint32) error {
	var overlapped windows.Overlapped
	return windows.LockFileEx(windows.Handle(fd), flags, reserved, low, high, &overlapped)
}
