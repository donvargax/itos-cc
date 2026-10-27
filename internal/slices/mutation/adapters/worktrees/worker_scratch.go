package worktrees

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// A run keeps its worker copies in a directory of the temporary directory
// of its own, named with one of workerPrefixes, and holds an exclusive lock
// on ownerMarker in it for as long as it runs: flock on Unix, LockFileEx on
// Windows. The OS releases the lock when the process ends however it ends,
// so a run killed before its cleanup leaves the marker unlocked, and the
// next run to make worker copies removes the directory. A lock the next run
// cannot take is a run still going, such as an agent's while a commit hook
// runs, and is left alone. A process ID would say the same until the ID is
// reused; the lock cannot be outlived.
//
// The marker is written only once the lock is taken, so an empty one is a
// run between making the file and locking it, or one killed there: like a
// directory with no marker at all, from an itos-cc before the marker or a
// run killed before it made one, it is removed only once nothing in it
// changed for unmarkedAge, longer than any run takes. Nothing in the
// temporary directory without one of workerPrefixes is ever touched.

// workerPrefixes are the prefixes of the directories that hold worker
// copies: a mutation run's, and a counted run's trials'.
var workerPrefixes = []string{"itos-cc-mutate-", "itos-cc-fresh-"}

// ownerMarker is the file of a worker directory its run holds locked.
const ownerMarker = "owner.lock"

// unmarkedAge is how long a worker directory with no marker written must
// have gone unchanged before it is taken for abandoned.
const unmarkedAge = 72 * time.Hour

// lockWait bounds how long a run waits for its own marker's lock, which
// another run's check of the directory may hold for an instant.
const lockWait = 5 * time.Second

// workerScratch is a run's directory of worker copies and the marker it
// holds locked, nil if it could not lock it.
type workerScratch struct {
	dir  string
	lock *os.File
}

// newWorkerScratch removes the worker directories killed runs left in tmp,
// "" for the system's temporary directory, then makes this run's there,
// named with prefix, and locks its marker. A marker it cannot lock leaves
// the run going, its directory judged by its age alone.
func newWorkerScratch(tmp, prefix, platform string) (*workerScratch, error) {
	if tmp == "" {
		tmp = os.TempDir()
	}
	removeAbandonedWorkers(tmp, platform)
	dir, err := os.MkdirTemp(tmp, prefix)
	if err != nil {
		return nil, err
	}
	return &workerScratch{dir: dir, lock: claimMarker(filepath.Join(dir, ownerMarker), platform)}, nil
}

// remove releases the lock, which Windows needs to delete the marker, and
// removes the directory.
func (s *workerScratch) remove() {
	if s.lock != nil {
		_ = s.lock.Close()
	}
	_ = os.RemoveAll(s.dir)
}

// claimMarker makes the marker at path, locks it and writes the run's
// process ID in it, and returns it held open, or nil when it could not.
func claimMarker(path, platform string) *os.File {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // exclusive owner marker at the caller's adapter-owned scratch path
	if err != nil {
		return nil
	}
	deadline := time.Now().Add(lockWait)
	for {
		held, err := tryLock(f, platform)
		if err == nil && held {
			break
		}
		if err != nil || time.Now().After(deadline) {
			_ = f.Close()
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := fmt.Fprintf(f, "itos-cc pid %d\n", os.Getpid()); err != nil {
		_ = f.Close()
		return nil
	}
	return f
}

// removeAbandonedWorkers removes each worker directory in tmp a killed run
// left behind. Failures are ignored: the next run tries again.
func removeAbandonedWorkers(tmp, platform string) {
	entries, err := os.ReadDir(tmp)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || !hasWorkerPrefix(e.Name()) {
			continue
		}
		if dir := filepath.Join(tmp, e.Name()); abandoned(dir, platform) {
			_ = os.RemoveAll(dir)
		}
	}
}

func hasWorkerPrefix(name string) bool {
	for _, p := range workerPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// abandoned says whether the worker directory dir is a killed run's: its
// marker written and unlocked, or, with no marker written, nothing in it
// changed for unmarkedAge. A marker it cannot open or lock but for not
// being there is a live run's, or another user's, and is left alone.
func abandoned(dir, platform string) bool {
	f, err := os.Open(filepath.Join(dir, ownerMarker)) //nolint:gosec // inspect only the owner marker beneath the discovered worker scratch directory
	if errors.Is(err, fs.ErrNotExist) {
		return unchangedFor(dir, unmarkedAge)
	}
	if err != nil {
		return false
	}
	held, err := tryLock(f, platform)
	if err != nil || !held {
		_ = f.Close()
		return false
	}
	info, err := f.Stat()
	// Closing releases the lock before the directory goes: Windows deletes
	// no file a handle holds open.
	_ = f.Close()
	if err != nil {
		return false
	}
	if info.Size() > 0 {
		return true
	}
	return unchangedFor(dir, unmarkedAge)
}

// unchangedFor says whether neither dir nor anything directly in it, a
// worker copy's directory, changed for age.
func unchangedFor(dir string, age time.Duration) bool {
	info, err := os.Stat(dir)
	if err != nil {
		return false
	}
	newest := info.ModTime()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			return false
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return time.Since(newest) > age
}
