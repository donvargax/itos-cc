//go:build windows

package metrics

import (
	"errors"
	"math/rand/v2"
	"syscall"
	"time"
)

// errorSharingViolation is ERROR_SHARING_VIOLATION, which package syscall
// does not name.
const errorSharingViolation syscall.Errno = 32

// renameTimeout bounds how long rename keeps retrying.
const renameTimeout = time.Second

// rename moves the staged snapshot onto its path. On Windows, replacing a
// file that another rename is replacing at the same moment fails for a
// short while with ERROR_ACCESS_DENIED or ERROR_SHARING_VIOLATION, so rename
// retries those two errors with short sleeps for about a second, as Go's own
// cmd/go/internal/robustio does, and then returns the last error.
func rename(oldpath, newpath string) error {
	start := time.Now()
	delay := time.Millisecond
	for {
		err := osRename(oldpath, newpath)
		if err == nil || !ephemeral(err) || time.Since(start) >= renameTimeout {
			return err
		}
		// A random sleep keeps writers that collided from colliding again.
		time.Sleep(delay/2 + rand.N(delay/2+1))
		delay = min(2*delay, 50*time.Millisecond)
	}
}

// ephemeral reports whether err is one a rename on Windows gets while
// another process or goroutine holds the target for a moment.
var ephemeral = func(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	return errno == syscall.ERROR_ACCESS_DENIED || errno == errorSharingViolation
}
