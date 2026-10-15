//go:build !windows

package worktrees

import "syscall"

const nativeLockExclusive = syscall.LOCK_EX
const nativeLockNonblocking = syscall.LOCK_NB

func nativeFileLock(fd uintptr, flags uint32, _ uint32, _ uint32, _ uint32) error {
	return syscall.Flock(int(fd), int(flags))
}
