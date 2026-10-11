package metrics

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// The Windows half of @ID-SNAP-04, "two runs writing the same snapshot at
// once leave one whole snapshot", on every system: Windows refuses a rename
// onto a snapshot, and the opening of one, with ERROR_ACCESS_DENIED or
// ERROR_SHARING_VIOLATION while another writer or reader holds it for a
// moment. These tests stand errHeld in for those errors, telling it as
// ephemeral as Windows' are, and inject it into the rename and the read.

// errHeld is the error a fake file operation fails with while another one
// holds the snapshot.
var errHeld = errors.New("held by another writer or reader")

// fakeFiles replaces the file operations and the ephemeral error test for
// t, putting them back when it ends.
func fakeFiles(t *testing.T, rename func(string, string) error, read func(string) ([]byte, error)) {
	t.Helper()
	oldRename, oldRead, oldEphemeral := osRename, osReadFile, ephemeral
	t.Cleanup(func() { osRename, osReadFile, ephemeral = oldRename, oldRead, oldEphemeral })
	ephemeral = func(err error) bool { return errors.Is(err, errHeld) }
	if rename != nil {
		osRename = rename
	}
	if read != nil {
		osReadFile = read
	}
}

// failing is an operation that fails with errHeld its first n calls, then
// does what real does; calls counts every call.
type failing struct {
	mu    sync.Mutex
	n     int
	calls int
}

func (f *failing) next() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.calls > f.n
}

func TestARenameRefusedForAMomentIsRetried(t *testing.T) {
	t.Chdir(t.TempDir())
	held := &failing{n: 3}
	fakeFiles(t, func(oldpath, newpath string) error {
		if !held.next() {
			return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: errHeld}
		}
		return os.Rename(oldpath, newpath)
	}, nil)
	if err := Write("x.json", map[string]string{"text": "whole"}); err != nil {
		t.Fatalf("a rename held three times: %v, want it retried until it lands", err)
	}
	var got map[string]string
	if ok, err := Read("x.json", &got); !ok || err != nil || got["text"] != "whole" {
		t.Errorf("read %v %v %v, want the snapshot written", ok, err, got)
	}
	if held.calls != 4 {
		t.Errorf("%d renames, want 4: three held, one that lands", held.calls)
	}
	if left, _ := filepath.Glob(filepath.Join(Dir(), "*.tmp")); len(left) > 0 {
		t.Errorf("staging files left behind: %v", left)
	}
}

// A rename that fails otherwise is not retried, and one still held after
// the retries' bound fails with the error it got, in bounded time.
func TestARenameThatCannotLandFails(t *testing.T) {
	t.Chdir(t.TempDir())
	other := errors.New("no such volume")
	calls := 0
	fakeFiles(t, func(string, string) error { calls++; return other }, nil)
	if err := Write("x.json", 1); !errors.Is(err, other) || calls != 1 {
		t.Errorf("a rename failing otherwise: %v after %d calls, want it once, unretried", err, calls)
	}
	fakeFiles(t, func(string, string) error { return errHeld }, nil)
	start := time.Now()
	if err := Write("x.json", 1); !errors.Is(err, errHeld) {
		t.Errorf("a rename held for good: %v, want the error it got", err)
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Errorf("a rename held for good took %v to fail, want a few seconds at most", took)
	}
}

func TestAReadRefusedForAMomentIsRetried(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := Write("x.json", map[string]string{"text": "whole"}); err != nil {
		t.Fatal(err)
	}
	held := &failing{n: 2}
	fakeFiles(t, nil, func(name string) ([]byte, error) {
		if !held.next() {
			return nil, &fs.PathError{Op: "open", Path: name, Err: errHeld}
		}
		return os.ReadFile(name)
	})
	var got map[string]string
	if ok, err := Read("x.json", &got); !ok || err != nil || got["text"] != "whole" {
		t.Errorf("a read held twice: %v %v %v, want it retried until it reads the snapshot", ok, err, got)
	}
	// A snapshot that is not there is none at once: absence is no moment's
	// hold.
	missing := 0
	fakeFiles(t, nil, func(name string) ([]byte, error) {
		missing++
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	})
	if ok, err := Read("y.json", &got); ok || err != nil || missing != 1 {
		t.Errorf("a missing snapshot: %v %v after %d reads, want none, at once", ok, err, missing)
	}
}

// Writers and readers of one snapshot in one process never hold it at the
// same moment, so they never make each other wait out Windows' refusals:
// two renames of it never overlap, nor a read and a rename.
func TestOneProcessNeverRenamesOrReadsASnapshotWhileItIsRenamed(t *testing.T) {
	t.Chdir(t.TempDir())
	var mu sync.Mutex
	renaming, reading, overlaps := 0, 0, 0
	enter := func(rename bool) {
		mu.Lock()
		defer mu.Unlock()
		if renaming > 0 || rename && reading > 0 {
			overlaps++
		}
		if rename {
			renaming++
		} else {
			reading++
		}
	}
	leave := func(rename bool) {
		mu.Lock()
		defer mu.Unlock()
		if rename {
			renaming--
		} else {
			reading--
		}
	}
	fakeFiles(t, func(oldpath, newpath string) error {
		enter(true)
		defer leave(true)
		time.Sleep(2 * time.Millisecond)
		return os.Rename(oldpath, newpath)
	}, func(name string) ([]byte, error) {
		enter(false)
		defer leave(false)
		time.Sleep(time.Millisecond)
		return os.ReadFile(name)
	})
	if err := Write("x.json", 0); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := Write("x.json", i); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer wg.Done()
			var n int
			if _, err := Read("x.json", &n); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if overlaps != 0 {
		t.Errorf("%d renames or reads of x.json began while another rename of it was in flight, want none", overlaps)
	}
}
