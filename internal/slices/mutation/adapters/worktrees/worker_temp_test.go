package worktrees

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// tempReady and tempLeft name the files the temp helper writes in its
// temporary folder: as Ready, under -test.short, and as a run.
const tempReady, tempLeft = "readied", "left"

// tempHelper is the fixture child of the temporary folder tests. As Ready
// it writes tempReady in its temporary folder; as a run it prints TMPDIR,
// TMP, TEMP and Go's temporary folder resolved, whether tempReady and
// tempLeft were there, then leaves tempLeft.
func tempHelper(t *testing.T) {
	dir := os.TempDir()
	if testing.Short() {
		if err := os.WriteFile(filepath.Join(dir, tempReady), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, ready := os.Stat(filepath.Join(dir, tempReady))
	_, left := os.Stat(filepath.Join(dir, tempLeft))
	if _, err := fmt.Fprintf(os.Stdout, "TMPDIR=%s\nTMP=%s\nTEMP=%s\ngo=%s\nready=%v\nleft=%v\n", os.Getenv("TMPDIR"), os.Getenv("TMP"), os.Getenv("TEMP"), resolved, ready == nil, left == nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, tempLeft), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

// seenTemp is what the temp helper printed, by name.
func seenTemp(t *testing.T, r mutants.CommandResult) map[string]string {
	t.Helper()
	seen := map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(r.Output, "\r\n", "\n"), "\n") {
		if name, value, ok := strings.Cut(line, "="); ok {
			seen[name] = value
		}
	}
	if !r.Passed {
		t.Fatalf("the helper failed: %+v", r)
	}
	return seen
}

// TestWorkersGiveEachWorkerATemporaryFolder holds slice-62 in process:
// every command a worker starts in its copy, Ready among them, sees
// TMPDIR, TMP and TEMP name the worker's own folder beside its copies, kept
// across its runs; a run in the project sees the system's; and the run's
// end removes them with the scratch folder. ID-MUT-353 and ID-MUT-354 hold
// it through the binary, which the code proof does not measure.
func TestWorkersGiveEachWorkerATemporaryFolder(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	system := t.TempDir()
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, system)
	}
	t.Setenv(runnerFixtureMode, "temp")
	opened, failure := (Workers{Platform: runtime.GOOS, TempDir: t.TempDir()}).Open(t.Context(), mutants.MutationRun, 2)
	if failure != nil {
		t.Fatal(failure)
	}
	p := opened.(*pool)
	args := commandRunnerHelperArgs()
	c := mutants.Command{Root: root, Dir: root, Args: args, Ready: append(args[:len(args):len(args)], "-test.short")}
	folders := map[string]bool{}
	for i := range p.workers {
		want := filepath.Join(p.scratch.dir, fmt.Sprintf("w%d", i), "tmp")
		for run, left := range []bool{false, true} {
			r, failure := p.Worker(i).Run(t.Context(), c, nil, mutants.Limits{})
			if failure != nil {
				t.Fatal(failure)
			}
			seen := seenTemp(t, r)
			resolved, err := filepath.EvalSymlinks(want)
			if err != nil {
				t.Fatal(err)
			}
			if seen["TMPDIR"] != want || seen["TMP"] != want || seen["TEMP"] != want || seen["go"] != resolved {
				t.Errorf("worker %d's run %d saw %v, not %s", i, run, seen, want)
			}
			if seen["ready"] != "true" || seen["left"] != fmt.Sprint(left) {
				t.Errorf("worker %d's run %d found Ready's file %s and its earlier run's %s, not true and %v", i, run, seen["ready"], seen["left"], left)
			}
		}
		folders[want] = true
	}
	if len(folders) != 2 {
		t.Errorf("the two workers saw %v", folders)
	}
	run, failure := p.InProject(t.Context(), c)
	if failure != nil {
		t.Fatal(failure)
	}
	if seen := seenTemp(t, run.Result); seen["TMPDIR"] != system || seen["TMP"] != system || seen["TEMP"] != system {
		t.Errorf("the run in the project saw %v, not the system's %s", seen, system)
	}
	if f := p.Close(t.Context()); f != nil {
		t.Fatal(f)
	}
	for folder := range folders {
		if _, err := os.Stat(folder); !os.IsNotExist(err) {
			t.Errorf("the worker's temporary folder %s is left: %v", folder, err)
		}
	}
}

// TestWorkerFailsWithoutItsTemporaryFolder: a worker that cannot make its
// temporary folder makes no copy and runs nothing.
func TestWorkerFailsWithoutItsTemporaryFolder(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	w := &worker{dir: filepath.Join(file, "w0"), copies: map[string]*workerCopy{}}
	root := t.TempDir()
	if _, err := w.run(t.Context(), Command{Root: root, Dir: root, Args: commandRunnerHelperArgs()}, mutants.Limits{}); err == nil {
		t.Fatal("a worker whose folder is under a file ran a command")
	}
	if len(w.copies) != 0 {
		t.Errorf("the worker made copies %v", w.copies)
	}
}
