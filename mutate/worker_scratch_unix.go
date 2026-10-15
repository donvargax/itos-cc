//go:build !windows

package mutate

import (
	"errors"
	"os"
	"syscall"
)

// tryLock takes an exclusive flock on f without waiting: false when
// another open file, of this process or another, holds it.
func tryLock(f *os.File) (bool, error) {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, syscall.EINTR):
			continue
		case errors.Is(err, syscall.EWOULDBLOCK):
			return false, nil
		}
		return false, err
	}
}
