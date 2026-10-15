package worktrees

import (
	"errors"
	"os"
	"syscall"
)

// tryLock is the common nonblocking ownership policy; native leaves only call
// flock or LockFileEx. Closing the file releases ownership on both systems.
func tryLock(f *os.File) (bool, error) {
	for {
		err := nativeFileLock(f)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, syscall.EINTR):
			continue
		case errors.Is(err, lockConflict):
			return false, nil
		}
		return false, err
	}
}
