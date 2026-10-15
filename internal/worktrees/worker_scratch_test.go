package worktrees

import (
	"os"
	"path/filepath"
	"testing"
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
	lock := claimMarker(filepath.Join(live, ownerMarker))
	if lock == nil {
		t.Fatal("the marker could not be locked")
	}
	if abandoned(live) {
		t.Error("a directory whose marker is held is taken for abandoned")
	}
	removeAbandonedWorkers(tmp)
	if _, err := os.Stat(live); err != nil {
		t.Fatalf("a held directory was removed: %v", err)
	}
	lock.Close()
	if !abandoned(live) {
		t.Error("a directory whose written marker no one holds is not taken for abandoned")
	}

	settingUp := filepath.Join(tmp, "itos-cc-fresh-2")
	if err := os.Mkdir(settingUp, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(settingUp, ownerMarker), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if abandoned(settingUp) {
		t.Error("a new directory with an empty marker is taken for abandoned")
	}

	removeAbandonedWorkers(tmp)
	if _, err := os.Stat(live); err == nil {
		t.Error("the directory of a run that let go is left")
	}
	if _, err := os.Stat(settingUp); err != nil {
		t.Errorf("the directory with an empty marker was removed: %v", err)
	}
}
