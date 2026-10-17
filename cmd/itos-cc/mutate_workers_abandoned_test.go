package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Beside ID-MUT-234, which holds the mutate-workers-abandoned-fix slice's
// old test whole (T-101): the guards of which worker copies a run removes,
// which the scenario does not reach. A mutation run keeps its worker
// copies in a directory of the temporary directory named itos-cc-mutate-
// (itos-cc-fresh- for a counted run's trials) and holds an exclusive lock
// on the file owner.lock in it, written after the lock is taken, for as
// long as it runs: flock on Unix, LockFileEx on Windows, both released by
// the OS when the process dies. A run killed before its cleanup leaves the
// directory with the marker unlocked; one still going holds it. The tests
// build both by hand in a temporary directory of their own, which the run
// is given as TMPDIR, TMP and TEMP: a killed run's with the marker written
// and no one holding it, a live one's with it held by a helper process,
// this test binary given holdLockArg. They run on the Go fixture of
// boardRepo, and need git and go.

// holdLockArg, as the first argument of this test binary, makes it a
// process that takes the lock on the file its second argument names,
// writes the marker, prints "held" and holds the lock until its standard
// input closes.
const holdLockArg = "itos-cc.test-hold-lock"

// ownerMarker is the file of a worker directory a run holds locked.
const ownerMarker = "owner.lock"

// holdLock is the helper process holdLockArg makes of this test binary.
func holdLock(path string) int {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	defer f.Close()
	if err := lockMarker(f); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if _, err := fmt.Fprintf(f, "itos-cc pid %d\n", os.Getpid()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Println("held")
	io.Copy(io.Discard, os.Stdin)
	return 0
}

// workerTemp is a temporary directory of the test's own, made the one
// itos-cc runs with on every OS.
func workerTemp(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		useEnv(t, key, tmp)
	}
	return tmp
}

// workerCopies makes a run's worker directory name in tmp, with copies of
// the project in w0 and w1, and returns it.
func workerCopies(t *testing.T, tmp, name string) string {
	t.Helper()
	dir := filepath.Join(tmp, name)
	for _, w := range []string{"w0", "w1"} {
		writeFile(t, filepath.Join(dir, w, "src", "board.go"), "package board\n")
	}
	return dir
}

// killedWorkers makes the worker directory name in tmp of a run killed before
// its cleanup: the marker written, its lock held by no one.
func killedWorkers(t *testing.T, tmp, name string) string {
	t.Helper()
	dir := workerCopies(t, tmp, name)
	writeFile(t, filepath.Join(dir, ownerMarker), "itos-cc pid 999999\n")
	return dir
}

// liveWorkers makes the worker directory name in tmp of a run still going: its
// marker locked by a helper process, which holds it until the test ends.
func liveWorkers(t *testing.T, tmp, name string) string {
	t.Helper()
	dir := workerCopies(t, tmp, name)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, holdLockArg, filepath.Join(dir, ownerMarker))
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stdin.Close()
		done := make(chan struct{})
		go func() { cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			cmd.Process.Kill()
			<-done
		}
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if strings.TrimSpace(line) != "held" {
		t.Fatalf("the helper did not take the lock on %s: %q, %v", dir, line, err)
	}
	return dir
}

// staleUnmarked makes the worker directory name in tmp as an itos-cc before
// the marker left it, with no marker, last changed age ago.
func staleUnmarked(t *testing.T, tmp, name string, age time.Duration) string {
	t.Helper()
	dir := workerCopies(t, tmp, name)
	then := time.Now().Add(-age)
	for _, p := range []string{filepath.Join(dir, "w0"), filepath.Join(dir, "w1"), dir} {
		if err := os.Chtimes(p, then, then); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Beside the scenario: a worker directory of an itos-cc before the marker
// is removed only once nothing in it changed for days, longer than any run
// takes; a counted run's trial copies are removed as a run's are; and
// nothing without itos-cc's prefixes is touched, however old.
func TestALaterRunRemovesOnlyItosCcWorkerCopiesClearlyAbandoned(t *testing.T) {
	t.Parallel()
	boardRepo(t, nil)
	tmp := workerTemp(t)
	oldUnmarked := staleUnmarked(t, tmp, "itos-cc-mutate-2000001", 10*24*time.Hour)
	youngUnmarked := staleUnmarked(t, tmp, "itos-cc-mutate-2000002", time.Hour)
	killedTrials := killedWorkers(t, tmp, "itos-cc-fresh-2000003")
	liveTrials := liveWorkers(t, tmp, "itos-cc-fresh-2000004")
	foreign := staleUnmarked(t, tmp, "other-tool-mutate-2000005", 10*24*time.Hour)
	foreignMarked := killedWorkers(t, tmp, "not-itos-cc-mutate-2000006")

	o := mutateCovered(t, "--no-coverage", boardSource)
	if o.code > 1 {
		t.Logf("exit %d\nstdout:\n%s\nstderr:\n%s", o.code, o.stdout, o.stderr)
	}

	if exists(oldUnmarked) {
		t.Errorf("%s, unmarked and unchanged for ten days, is left behind", oldUnmarked)
	}
	if exists(killedTrials) {
		t.Errorf("the killed counted run's trial copies %s are left behind", killedTrials)
	}
	if !exists(filepath.Join(youngUnmarked, "w0", "src", "board.go")) {
		t.Errorf("%s, unmarked but changed an hour ago, was removed", youngUnmarked)
	}
	if !exists(filepath.Join(liveTrials, "w0", "src", "board.go")) {
		t.Errorf("the trial copies %s of a counted run still going were removed", liveTrials)
	}
	for _, d := range []string{foreign, foreignMarked} {
		if !exists(filepath.Join(d, "w0", "src", "board.go")) {
			t.Errorf("%s, without itos-cc's prefix, was removed", d)
		}
	}
}
