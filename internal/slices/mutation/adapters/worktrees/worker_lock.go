package worktrees

import (
	"errors"
	"os"
	"syscall"
)

// tryLock is the common nonblocking ownership policy; native leaves only call
// flock or LockFileEx. Closing the file releases ownership on both systems.
func tryLock(f *os.File, platform string) (bool, error) {
	conflict := error(syscall.EWOULDBLOCK)
	if platform == "windows" {
		conflict = syscall.Errno(33)
	}
	for {
		err := nativeFileLock(f.Fd(), nativeLockExclusive|nativeLockNonblocking, 0, 1, 0)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, syscall.EINTR):
			continue
		case errors.Is(err, conflict):
			return false, nil
		}
		return false, err
	}
}
