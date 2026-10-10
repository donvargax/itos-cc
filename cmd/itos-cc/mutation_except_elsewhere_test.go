package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The scenarios of exceptions-deleted-file, at the end of "Rule: Equivalent
// mutants excepted, each with a reason" in features/mutate.feature. They use
// the module of mutate_since_test.go, as the other scenarios of that rule
// do: Board#clear's survivor at src/board.go:11:11 stands for the one at
// src/board.ts:7:19, and src/grid.go for src/grid.ts. In Go, a file renamed
// within its package keeps its functions' namespace#name.

// countFiles is a source of package board that stays when src/board.go and
// its tests are deleted, with a test that kills its one mutant.
var countFiles = map[string]string{
	"src/count.go":      "package board\n\nfunc count(i int) bool {\n\treturn i > 5\n}\n",
	"src/count_test.go": "package board\n\nimport \"testing\"\n\nfunc TestCount(t *testing.T) {\n\tif count(5) || !count(6) {\n\t\tt.Fatal(\"count\")\n\t}\n}\n",
}

// deleteBoard deletes src/board.go and its tests.
func deleteBoard(t *testing.T) {
	t.Helper()
	for _, name := range []string{boardSource, filepath.FromSlash("src/board_test.go")} {
		if err := os.Remove(inWD(t, name)); err != nil {
			t.Fatal(err)
		}
	}
}

// renameBoard renames src/board.go to src/grid.go, gridSource, and commits
// the rename.
func renameBoard(t *testing.T) {
	t.Helper()
	gitIn(t, ".", "mv", "src/board.go", "src/grid.go")
	commitAll(t, "rename board.go")
}

// wantNoLine fails t when problem p names a line.
func wantNoLine(t *testing.T, p map[string]any, stdout string) {
	t.Helper()
	if line, ok := p["line"]; ok {
		t.Errorf("problem line = %v, want none\n%s", line, stdout)
	}
}

// @ID-MUT-141
func TestARunOfTheWholeProjectFailsAnEntryWhoseFileIsGone(t *testing.T) {
	t.Parallel()
	boardRepo(t, nil)
	survivorRun(t)
	writeExceptions(t, "", clearException(t, "clear is never called"))
	deleteBoard(t)

	o := mutateRun(t, "--json")
	p := o.json(t).problem("mutation.exception-stale")
	wantProblem(t, p, map[string]any{"file": boardSource, "function": clearID, "why": "gone"}, o.stdout)
	wantNoLine(t, p, o.stdout)
	if o.code != 1 {
		t.Errorf("exit %d, want 1", o.code)
	}
}

// @ID-MUT-142
func TestMutationCheckOfTheWholeProjectFailsItToo(t *testing.T) {
	t.Parallel()
	boardRepo(t, countFiles)
	if o := mutateRun(t); o.code != 1 {
		t.Fatalf("the first run: exit %d, want 1 for clear's survivor\n%s%s", o.code, o.stdout, o.stderr)
	}
	writeExceptions(t, "", clearException(t, "clear is never called"))
	deleteBoard(t)
	// In Go, src/board_test.go is among the tests of src/count.go too, so
	// its deletion makes count's results stale: a run of count alone,
	// which judges no entry of another file, makes them fresh again.
	if o := mutateRun(t, filepath.FromSlash("src/count.go")); o.code != 0 {
		t.Fatalf("the run of src/count.go: exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}

	o := mutationCheck(t, "--json")
	m := o.json(t)
	wantProblem(t, m.problem("mutation.exception-stale"), map[string]any{"file": boardSource, "why": "gone"}, o.stdout)
	if p := m.problem("mutation.stale"); p != nil {
		t.Errorf("problem %v, want src/count.go's results fresh", p)
	}
	if o.code != 1 {
		t.Errorf("exit %d, want 1", o.code)
	}
}

// @ID-MUT-143
func TestWithSinceAnEntryWhoseFileTheRangeDeletedIsGone(t *testing.T) {
	t.Parallel()
	boardRepo(t, nil)
	survivorRun(t)
	writeExceptions(t, "", clearException(t, "clear is never called"))
	deleteBoard(t)
	commitAll(t, "delete board.go")

	o := mutateRun(t, "--json", "--since", "base")
	wantProblem(t, o.json(t).problem("mutation.exception-stale"), map[string]any{"file": boardSource, "why": "gone"}, o.stdout)
	if o.code != 1 {
		t.Errorf("exit %d, want 1", o.code)
	}
}

// @ID-MUT-144
func TestAnEntryWhoseFileWasRenamedHasMovedAndStillExceptsItsMutant(t *testing.T) {
	t.Parallel()
	boardRepo(t, nil)
	survivorRun(t)
	writeExceptions(t, "", clearException(t, "clear is never called"))
	renameBoard(t)

	o := mutateRun(t, "--json", "--since", "base")
	m := o.json(t)
	p := m.problem("mutation.exception-stale")
	wantProblem(t, p, map[string]any{"file": boardSource, "why": "moved", "new_file": gridSource}, o.stdout)
	if fix, _ := p["fix"].(string); !strings.Contains(fix, "src/grid.go") {
		t.Errorf("fix %q, want it to say to change the entry's file to src/grid.go", fix)
	}
	if p := m.problem("mutation.survived"); p != nil {
		t.Errorf("problem %v, want no survivor: the entry still excepts clear's mutant", p)
	}
	if got := readExceptions(t); len(got) != 1 || got[0].File != "src/board.go" {
		t.Errorf("mutation.exceptions %+v, want the one entry still naming src/board.go", got)
	}
	if o.code != 1 {
		t.Errorf("exit %d, want 1", o.code)
	}
}

// @ID-MUT-146
func TestARunOfTheWholeProjectFindsARenamedFilesEntryMoved(t *testing.T) {
	t.Parallel()
	boardRepo(t, nil)
	survivorRun(t)
	writeExceptions(t, "", clearException(t, "clear is never called"))
	renameBoard(t)

	o := mutateRun(t, "--json")
	m := o.json(t)
	wantProblem(t, m.problem("mutation.exception-stale"), map[string]any{"why": "moved", "new_file": gridSource}, o.stdout)
	if p := m.problem("mutation.survived"); p != nil {
		t.Errorf("problem %v, want no survivor: the entry still excepts clear's mutant", p)
	}
}

// A file renamed with its function changed holds no function of the
// entry's name and hash: the entry is gone, and the mutant at the new path
// is a survivor, as if it had no entry.
func TestAnEntryWhoseFileWasRenamedAndWhoseFunctionChangedIsGone(t *testing.T) {
	t.Parallel()
	boardRepo(t, nil)
	survivorRun(t)
	writeExceptions(t, "", clearException(t, "clear is never called"))
	gitIn(t, ".", "mv", "src/board.go", "src/grid.go")
	edit(t, gridSource, "i < 3", "i < 4")
	commitAll(t, "rename board.go and change clear")

	for _, args := range [][]string{{"--json"}, {"--json", "--since", "base"}} {
		o := mutateRun(t, args...)
		m := o.json(t)
		p := m.problem("mutation.exception-stale")
		wantProblem(t, p, map[string]any{"file": boardSource, "function": clearID, "why": "gone"}, o.stdout)
		wantNoLine(t, p, o.stdout)
		if _, ok := p["new_file"]; ok {
			t.Errorf("%v: problem %v, want no new_file", args, p)
		}
		wantProblem(t, m.problem("mutation.survived"), map[string]any{"file": gridSource, "function": clearID}, o.stdout)
		if o.code != 1 {
			t.Errorf("%v: exit %d, want 1", args, o.code)
		}
	}
}

// With paths, an entry for a file outside them is not judged, even when
// that file is gone.
func TestWithPathsAnEntryWhoseFileIsGoneIsNotJudged(t *testing.T) {
	t.Parallel()
	boardRepo(t, countFiles)
	if o := mutateRun(t); o.code != 1 {
		t.Fatalf("the first run: exit %d, want 1 for clear's survivor\n%s%s", o.code, o.stdout, o.stderr)
	}
	writeExceptions(t, "", clearException(t, "clear is never called"))
	deleteBoard(t)

	count := filepath.FromSlash("src/count.go")
	o := mutateRun(t, "--json", count)
	if p := o.json(t).problem("mutation.exception-stale"); p != nil || o.code != 0 {
		t.Errorf("mutation run %s: exit %d, problem %v, want exit 0 and none", count, o.code, p)
	}
	o = mutationCheck(t, "--json", count)
	if p := o.json(t).problem("mutation.exception-stale"); p != nil || o.code != 0 {
		t.Errorf("mutation check %s: exit %d, problem %v, want exit 0 and none", count, o.code, p)
	}
}
