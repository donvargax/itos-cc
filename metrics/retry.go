package metrics

import (
	"math/rand/v2"
	"path/filepath"
	"sync"
	"time"
)

// retryBudget bounds how long a snapshot's rename or read keeps retrying an
// ephemeral error, as Go's own cmd/go/internal/robustio bounds its retries.
const retryBudget = 2 * time.Second

// retry runs op until it succeeds, fails with an error that is not
// ephemeral, or retryBudget has passed since the first try, and returns its
// last error. Windows refuses a rename onto a file, and the opening of one,
// with ERROR_ACCESS_DENIED or ERROR_SHARING_VIOLATION while another rename
// or reader holds it for a moment; elsewhere no error is ephemeral, and op
// runs once.
func retry(op func() error) error {
	start := time.Now()
	delay := time.Millisecond
	for {
		err := op()
		if err == nil || !ephemeral(err) || time.Since(start) >= retryBudget {
			return err
		}
		// A random sleep keeps writers that collided from colliding again.
		time.Sleep(delay/2 + rand.N(delay/2+1))
		delay = min(2*delay, 50*time.Millisecond)
	}
}

// rename moves the staged snapshot onto its path, retrying while another
// writer or reader holds it.
func rename(oldpath, newpath string) error {
	return retry(func() error { return osRename(oldpath, newpath) })
}

// holds is, by a snapshot's absolute path, the lock its writers in this
// process rename under, one at a time, and its readers read under, none
// while a rename is in flight, so they never make each other wait out
// Windows' refusals. Other processes are held off by retry alone.
var holds sync.Map

// hold is the lock of the snapshot at path.
func hold(path string) *sync.RWMutex {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	l, _ := holds.LoadOrStore(filepath.Clean(path), &sync.RWMutex{})
	return l.(*sync.RWMutex)
}
