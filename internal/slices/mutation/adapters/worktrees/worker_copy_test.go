package worktrees

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// pristineReadies names the file the pristine helper's Ready appends a line
// to each time it runs, and that file with .fail added, when there, fails it.
const pristineReadies = "ITOS_PRISTINE_READIES"

// pristineHelper is the fixture helper's pristine mode. Run with
// -test.short it is the copy's Ready: it writes readied. Otherwise it is a
// test run that fails when it finds what an earlier run left, then leaves
// it: it writes left-behind and gone.txt, removes kept.txt and the
// node_modules link, rewrites source.txt and readied, and appends a line to
// build/runs.txt, a folder the copy leaves out, and node_modules/runs.txt,
// through the link.
func pristineHelper(t *testing.T) {
	if testing.Short() {
		readies := os.Getenv(pristineReadies)
		pristineAppend(t, readies)
		if _, err := os.Stat(readies + ".fail"); err == nil {
			t.Fatal("ready fails")
		}
		pristineWrite(t, "readied", "readied\n")
		return
	}
	if _, err := os.Lstat("left-behind"); err == nil {
		t.Fatal("left-behind is there")
	}
	if _, err := os.Stat("kept.txt"); err != nil {
		t.Fatalf("kept.txt is gone: %v", err)
	}
	for name, want := range map[string]string{"source.txt": "original\n", "readied": "readied\n"} {
		if got, err := os.ReadFile(name); err != nil || string(got) != want {
			t.Fatalf("%s holds %q, %v", name, got, err)
		}
	}
	if _, err := os.Stat(filepath.Join("node_modules", "dep.js")); err != nil {
		t.Fatalf("node_modules is not linked: %v", err)
	}
	pristineWrite(t, "left-behind", "left\n")
	pristineWrite(t, "gone.txt", "spoiled\n")
	pristineWrite(t, "source.txt", "rewritten\n")
	pristineWrite(t, "readied", "spoiled\n")
	if err := os.Remove("kept.txt"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("build", 0o755); err != nil {
		t.Fatal(err)
	}
	pristineAppend(t, filepath.Join("build", "runs.txt"))
	pristineAppend(t, filepath.Join("node_modules", "runs.txt"))
	if err := os.Remove("node_modules"); err != nil {
		t.Fatal(err)
	}
}

func pristineWrite(t *testing.T, name, text string) {
	if err := os.WriteFile(name, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func pristineAppend(t *testing.T, name string) {
	f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // a fixture's own file
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("ran\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// pristineProject lays out the project the pristine helper runs in, with
// a package.json, so a build/ beside it is build output, and a
// node_modules a copy links, and returns its folder, the command that runs
// the helper there, with the helper as Ready, and the file Ready counts
// its runs in.
func pristineProject(t *testing.T) (string, mutants.Command, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"package.json": "{}\n", "source.txt": "original\n", "kept.txt": "kept\n", "gone.txt": "gone\n",
		"mutant.txt": "a\n", filepath.Join("node_modules", "dep.js"): "dep\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	readies := filepath.Join(t.TempDir(), "readies")
	t.Setenv(runnerFixtureMode, "pristine")
	t.Setenv(pristineReadies, readies)
	args := commandRunnerHelperArgs()
	return root, mutants.Command{Root: root, Dir: root, Args: args, Ready: append(args[:len(args):len(args)], "-test.short")}, readies
}

// pristineWorker is the one real worker of a pool it opens.
func pristineWorker(t *testing.T) *boundWorker {
	t.Helper()
	opened, failure := (Workers{Platform: runtime.GOOS, TempDir: t.TempDir()}).Open(t.Context(), mutants.MutationRun, 1)
	if failure != nil {
		t.Fatal(failure)
	}
	p := opened.(*pool)
	t.Cleanup(func() {
		if f := p.Close(t.Context()); f != nil {
			t.Error(f)
		}
	})
	return p.workers[0]
}

func lines(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // a fixture's own file
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "\n")
}

// TestEveryRunStartsFromTheCopyAsMade holds slice-13 (issue #35) on every
// system's go test, with no tool: a baseline, then two mutants, on one
// worker, each run leaving behind what fails the next, all pass. What the
// copy leaves out (build/ beside a package.json) and the linked
// node_modules keep what the runs wrote, Ready runs once, and the restore,
// slowed here, stays out of each run's Elapsed.
func TestEveryRunStartsFromTheCopyAsMade(t *testing.T) {
	root, c, readies := pristineProject(t)
	w := pristineWorker(t)
	const slowed = 300 * time.Millisecond
	w.worker.restored = func() { time.Sleep(slowed) }
	mutant := &mutants.Mutation{Path: filepath.Join(root, "mutant.txt"), Source: []byte("a\n"), Site: mutants.Site{Start: 0, End: 1, Original: "a", Replacement: "b"}}
	for i, change := range []*mutants.Mutation{nil, mutant, mutant} {
		began := time.Now()
		r, failure := w.Run(t.Context(), c, change, mutants.Limits{})
		took := time.Since(began)
		if failure != nil || !r.Passed {
			t.Fatalf("run %d: %+v %v", i+1, r, failure)
		}
		if i > 0 && r.Elapsed+slowed > took {
			t.Errorf("run %d's Elapsed %v holds its restore: the run took %v, its restore at least %v", i+1, r.Elapsed, took, slowed)
		}
	}
	copied := w.worker.copies[root].dir
	if n := lines(t, filepath.Join(copied, "build", "runs.txt")); n != 3 {
		t.Errorf("the copy's build/runs.txt holds %d runs, not the 3 that wrote it", n)
	}
	if n := lines(t, filepath.Join(root, "node_modules", "runs.txt")); n != 3 {
		t.Errorf("the project's node_modules/runs.txt holds %d runs, not the 3 that wrote it through the link", n)
	}
	if n := lines(t, readies); n != 1 {
		t.Errorf("Ready ran %d times, not once", n)
	}
	for name, want := range map[string]string{"source.txt": "original\n", "kept.txt": "kept\n", "mutant.txt": "a\n", filepath.Join("node_modules", "dep.js"): "dep\n"} {
		if got, err := os.ReadFile(filepath.Join(root, name)); err != nil || string(got) != want { //nolint:gosec // a fixture's own file
			t.Errorf("the project's %s holds %q, %v", name, got, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "left-behind")); err == nil {
		t.Error("a run wrote into the project's own folder")
	}
}

// TestACopyItCannotRestoreIsCopiedAfresh: a restore that cannot copy a
// changed file again, here one the project no longer has, discards the
// copy and makes it afresh, with Ready, and the run goes on; when that
// fails too, the run is WorkerFailed, never a judgment.
func TestACopyItCannotRestoreIsCopiedAfresh(t *testing.T) {
	root, c, readies := pristineProject(t)
	w := pristineWorker(t)
	if r, failure := w.Run(t.Context(), c, nil, mutants.Limits{}); failure != nil || !r.Passed {
		t.Fatalf("baseline: %+v %v", r, failure)
	}
	if err := os.Remove(filepath.Join(root, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	if r, failure := w.Run(t.Context(), c, nil, mutants.Limits{}); failure != nil || !r.Passed {
		t.Fatalf("the run after a failed restore: %+v %v", r, failure)
	}
	if n := lines(t, readies); n != 2 {
		t.Errorf("Ready ran %d times, not again for the fresh copy", n)
	}
	if _, err := os.Lstat(filepath.Join(w.worker.copies[root].dir, "left-behind")); err != nil {
		t.Errorf("the fresh copy is not the one the run used: %v", err)
	}
	if err := os.WriteFile(readies+".fail", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "source.txt")); err != nil {
		t.Fatal(err)
	}
	r, failure := w.Run(t.Context(), c, nil, mutants.Limits{})
	var failed mutants.WorkerFailed
	if !errors.As(failure, &failed) || r.Passed {
		t.Fatalf("a restore and a fresh copy that both fail: %+v %v, not WorkerFailed", r, failure)
	}
}
