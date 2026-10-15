//go:build !windows

package worktrees

import (
	"os"
	"syscall"
)

const lockConflict = syscall.EWOULDBLOCK

func nativeFileLock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
