package worktrees

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// A worker directory whose marker its run holds is never abandoned, even
// to a check from the same process, and is once the run lets go; an empty
// marker, a run between making it and locking it, is judged by age.
func TestWorkerDirectoriesAreAbandonedOnlyOnceTheirLockIsFree(t *testing.T) {
	tmp := t.TempDir()
	live := filepath.Join(tmp, "itos-cc-mutate-1")
	if err := os.Mkdir(live, 0o700); err != nil {
		t.Fatal(err)
	}
	lock := claimMarker(filepath.Join(live, ownerMarker), runtime.GOOS)
	if lock == nil {
		t.Fatal("the marker could not be locked")
	}
	if abandoned(live, runtime.GOOS) {
		t.Error("a directory whose marker is held is taken for abandoned")
	}
	removeAbandonedWorkers(tmp, runtime.GOOS)
	if _, err := os.Stat(live); err != nil {
		t.Fatalf("a held directory was removed: %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if !abandoned(live, runtime.GOOS) {
		t.Error("a directory whose written marker no one holds is not taken for abandoned")
	}

	settingUp := filepath.Join(tmp, "itos-cc-fresh-2")
	if err := os.Mkdir(settingUp, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(settingUp, ownerMarker), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if abandoned(settingUp, runtime.GOOS) {
		t.Error("a new directory with an empty marker is taken for abandoned")
	}

	removeAbandonedWorkers(tmp, runtime.GOOS)
	if _, err := os.Stat(live); err == nil {
		t.Error("the directory of a run that let go is left")
	}
	if _, err := os.Stat(settingUp); err != nil {
		t.Errorf("the directory with an empty marker was removed: %v", err)
	}
}

// A worker directory with no marker is abandoned once neither it nor a
// worker copy in it changed for unmarkedAge, and not while a copy is newer;
// one another run removed between the listing and the check is not. The
// times are set, not waited for, and the directories are the test's own, so
// no other run in the system's temporary directory decides the outcome (T-217).
func TestUnmarkedWorkerDirectoriesAreAbandonedOnlyOnceUnchangedForTheirAge(t *testing.T) {
	tmp := t.TempDir()
	old := time.Now().Add(-2 * unmarkedAge)
	stale := unmarkedWorkers(t, tmp, "itos-cc-mutate-3", old, old)
	touched := unmarkedWorkers(t, tmp, "itos-cc-fresh-4", old, time.Now())
	gone := filepath.Join(tmp, "itos-cc-mutate-5")

	if !abandoned(stale, runtime.GOOS) {
		t.Error("an unmarked directory unchanged for twice its age is not taken for abandoned")
	}
	if abandoned(touched, runtime.GOOS) {
		t.Error("an unmarked directory whose worker copy just changed is taken for abandoned")
	}
	if abandoned(gone, runtime.GOOS) {
		t.Error("a directory gone before its check is taken for abandoned")
	}

	removeAbandonedWorkers(tmp, runtime.GOOS)
	if _, err := os.Stat(stale); err == nil {
		t.Error("the unmarked directory unchanged for twice its age is left")
	}
	if _, err := os.Stat(touched); err != nil {
		t.Errorf("the unmarked directory with a new worker copy was removed: %v", err)
	}
}

// unmarkedWorkers makes the worker directory name in tmp with no marker and
// one worker copy, the copy last changed at copied and the directory at
// made, set after the copy since making the copy changes the directory.
func unmarkedWorkers(t *testing.T, tmp, name string, made, copied time.Time) string {
	t.Helper()
	dir := filepath.Join(tmp, name)
	worker := filepath.Join(dir, "w0")
	if err := os.MkdirAll(worker, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(worker, copied, copied); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dir, made, made); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A pool makes its worker directory in the folder its Workers names, and
// sweeps that folder alone of what killed runs left, so a test's pools meet
// no other run's directories (T-219).
func TestAPoolMakesItsWorkerDirectoryInItsOwnFolder(t *testing.T) {
	tmp := t.TempDir()
	left := abandonedWorkers(t, tmp, "itos-cc-mutate-6")

	p := openPool(t, Workers{Platform: runtime.GOOS, TempDir: tmp}, mutants.FrozenTrials)
	if dir := p.scratch.dir; filepath.Dir(dir) != tmp || !strings.HasPrefix(filepath.Base(dir), "itos-cc-fresh-") {
		t.Errorf("a counted run's worker directory is %s, not an itos-cc-fresh- one in %s", dir, tmp)
	}
	if _, err := os.Stat(left); err == nil {
		t.Error("the directory a killed run left in the pool's folder is left")
	}
	if f := p.Close(t.Context()); f != nil {
		t.Fatal(f)
	}
	if _, err := os.Stat(p.scratch.dir); err == nil {
		t.Error("the pool's worker directory outlives its close")
	}
}

// A Workers naming no folder makes its worker directories in the system's
// temporary one, as a user's run does: here the test's own, through the
// variables the system's is read from.
func TestAPoolNamingNoFolderUsesTheSystemTemporaryOne(t *testing.T) {
	system := t.TempDir()
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, system)
	}
	left := abandonedWorkers(t, system, "itos-cc-fresh-7")

	p := openPool(t, Workers{Platform: runtime.GOOS}, mutants.MutationRun)
	defer func() {
		if f := p.Close(t.Context()); f != nil {
			t.Error(f)
		}
	}()
	if _, err := os.Stat(filepath.Join(system, filepath.Base(p.scratch.dir))); err != nil {
		t.Errorf("the worker directory %s is not in the system's temporary directory: %v", p.scratch.dir, err)
	}
	if _, err := os.Stat(left); err == nil {
		t.Error("the directory a killed run left in the system's temporary directory is left")
	}
}

// A pool whose folder cannot hold its worker directory, one that is not
// there, opens none and says why.
func TestAPoolWhoseFolderCannotHoldItsDirectoryOpensNone(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	_, failure := (Workers{Platform: runtime.GOOS, TempDir: missing}).Open(t.Context(), mutants.MutationRun, 1)
	var failed mutants.WorkerFailed
	if !errors.As(failure, &failed) || !errors.Is(failed.Cause, fs.ErrNotExist) {
		t.Errorf("a pool in a folder that is not there: %v, not a WorkerFailed for the missing folder", failure)
	}
	if _, err := os.Stat(missing); err == nil {
		t.Error("the missing folder was made")
	}
}

// openPool opens one worker of f's for purpose.
func openPool(t *testing.T, f Workers, purpose mutants.WorkPurpose) *pool {
	t.Helper()
	opened, failure := f.Open(t.Context(), purpose, 1)
	if failure != nil {
		t.Fatal(failure)
	}
	return opened.(*pool)
}

// abandonedWorkers makes the worker directory name in tmp as a killed run
// leaves it: its marker written and no longer locked.
func abandonedWorkers(t *testing.T, tmp, name string) string {
	t.Helper()
	dir := filepath.Join(tmp, name)
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ownerMarker), []byte("itos-cc pid 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}
