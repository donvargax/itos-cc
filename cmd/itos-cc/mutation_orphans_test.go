package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/mutate"
)

// The scenario of "Rule: Mutation results say what happened, and leave
// nothing stale behind" in features/mutate.feature that the
// renamed-snapshot-left-behind-fix slice holds. It uses the module of
// mutate_since_test.go with killedTests, and writes beside src/board.go's
// snapshot raw ones of TypeScript files that are gone, so no Node is
// needed: the orphan names no file of the module's language.

// orphanSnapshot is a raw snapshot of the TypeScript file at rel that
// records a survivor, so judging it as a function would fail.
func orphanSnapshot(rel string) string {
	return `{
  "version": 1,
  "file": "` + rel + `",
  "language": "typescript",
  "tests": {},
  "units": [
    {
      "namespace": "` + strings.TrimSuffix(rel, ".ts") + `",
      "name": "old",
      "hash": "0000",
      "start_line": 1,
      "end_line": 3,
      "killed": 0,
      "survived": 1,
      "uncovered": 0,
      "sites": 1,
      "mutants": [
        {"line": 2, "column": 12, "offset": 30, "original": ">", "replacement": ">=", "outcome": "survived"}
      ]
    }
  ]
}
`
}

// orphanJSON is an entry of "orphans" in the --json output.
type orphanJSON struct {
	File     string `json:"file"`
	Snapshot string `json:"snapshot"`
}

// orphansOf is the "orphans" of the --json output, nil when the key is
// missing.
func (o outcome) orphansOf(t *testing.T) *[]orphanJSON {
	t.Helper()
	var out struct {
		Orphans *[]orphanJSON `json:"orphans"`
	}
	if err := json.Unmarshal([]byte(o.stdout), &out); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s\nstderr:\n%s", err, o.stdout, o.stderr)
	}
	return out.Orphans
}

// snapshotThere reports whether name, a relative one in the test's directory, is
// there.
func snapshotThere(t *testing.T, name string) bool {
	t.Helper()
	_, err := os.Lstat(inWD(t, filepath.FromSlash(name)))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return err == nil
}

// @ID-MUT-236
func TestASnapshotWhoseSourceIsGoneIsRemovedByTheNextRunOverItsDirectory(t *testing.T) {
	t.Parallel()
	boardRepo(t, killedTests)
	if o := mutateRun(t, boardSource); o.code != 0 {
		t.Fatalf("the run: exit %d, want every mutant killed\n%s%s", o.code, o.stdout, o.stderr)
	}
	const (
		orphan    = ".metrics/mutate/src/old.ts.json"
		elsewhere = ".metrics/mutate/lib/gone.ts.json"
		board     = ".metrics/mutate/src/board.go.json"
	)
	writeFile(t, filepath.FromSlash(orphan), orphanSnapshot("src/old.ts"))
	writeFile(t, filepath.FromSlash(elsewhere), orphanSnapshot("lib/gone.ts"))
	boardBefore, err := os.ReadFile(inWD(t, filepath.FromSlash(board)))
	if err != nil {
		t.Fatal(err)
	}

	// mutation check names the orphan, and fails nothing for it.
	o := mutationCheck(t, "--json", "src")
	if o.code != 0 {
		t.Errorf("mutation check src: exit %d, want 0: an orphan fails nothing\n%s%s", o.code, o.stdout, o.stderr)
	}
	if ps := o.json(t).Problems; len(ps) > 0 {
		t.Errorf("mutation check src reports %v, want no problem for the orphan", ps)
	}
	if _, ok := o.checkFunctions(t)[filepath.FromSlash("src/old.ts")]; ok {
		t.Errorf("mutation check src lists src/old.ts as a file checked, want it only named as an orphan\n%s", o.stdout)
	}
	want := orphanJSON{File: filepath.FromSlash("src/old.ts"), Snapshot: orphan}
	if got := o.orphansOf(t); got == nil || len(*got) != 1 || (*got)[0] != want {
		t.Errorf("mutation check src: \"orphans\" %v, want only %+v, not lib/gone.ts, which src does not cover\n%s", got, want, o.stdout)
	}
	o = mutationCheck(t, "src")
	if o.code != 0 || !strings.Contains(o.stdout, orphan) {
		t.Errorf("mutation check src: exit %d, want 0 and a line naming %s\n%s%s", o.code, orphan, o.stdout, o.stderr)
	}

	// mutation run removes it, and keeps every snapshot whose file exists
	// and every orphan its paths do not cover.
	o = mutateRun(t, "--json", "src")
	if o.code != 0 {
		t.Errorf("mutation run src: exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}
	if snapshotThere(t, orphan) {
		t.Errorf("after mutation run src, %s is still there, want the orphan removed", orphan)
	}
	if got := o.orphansOf(t); got == nil || len(*got) != 1 || (*got)[0] != want {
		t.Errorf("mutation run src: \"orphans\" %v, want only the one it removed, %+v\n%s", got, want, o.stdout)
	}
	if !snapshotThere(t, elsewhere) {
		t.Errorf("after mutation run src, %s is gone, want an orphan src does not cover kept", elsewhere)
	}
	boardAfter, err := os.ReadFile(inWD(t, filepath.FromSlash(board)))
	if err != nil {
		t.Fatalf("after mutation run src, the snapshot of src/board.go: %v", err)
	}
	if string(boardAfter) != string(boardBefore) {
		t.Errorf("mutation run src rewrote %s, want it kept as it was:\nbefore\n%s\nafter\n%s", board, boardBefore, boardAfter)
	}
	if snap, err := mutate.LoadSnapshotOf(wd(t), "src/board.go"); err != nil || snap == nil {
		t.Errorf("the snapshot of src/board.go after mutation run src: %v, %v, want it kept", snap, err)
	}

	// The run without paths covers the working directory: the project.
	o = mutateRun(t)
	if o.code != 0 || snapshotThere(t, elsewhere) {
		t.Errorf("mutation run: exit %d, %s there %v, want 0 and the orphan removed\n%s%s", o.code, elsewhere, snapshotThere(t, elsewhere), o.stdout, o.stderr)
	}
	if snapshotThere(t, ".metrics/mutate/lib") {
		t.Errorf("mutation run left .metrics/mutate/lib, want a directory it emptied removed")
	}
	if !snapshotThere(t, board) {
		t.Errorf("mutation run removed %s, a snapshot whose file exists", board)
	}
}
