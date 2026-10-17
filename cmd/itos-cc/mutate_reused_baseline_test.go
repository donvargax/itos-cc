package main

import (
	"maps"
	"path/filepath"
	"testing"
)

// Beside ID-MUT-240, which holds the reused-baseline-not-run-fix slice's
// old test whole (T-101): a guard that files sharing a command share its
// baseline, which no scenario reaches. It runs on the Go module of
// boardRepo (mutate_since_test.go): a first run writes src/board.go's
// snapshot, so a second run reuses every one of its mutants and runs no
// baseline for its package, until an edit to one of its functions leaves a
// mutant to run. A survivor is judged again in every run, so clearTest
// kills clear's mutant too.

// clearTest is a test of the board module that kills clear's mutant.
var clearTest = map[string]string{"src/clear_test.go": `package board

import "testing"

func TestClear(t *testing.T) {
	var b Board
	if b.clear(3) || !b.clear(2) {
		t.Fatal("clear")
	}
}
`}

// Files sharing a command share its baseline: a file of a Go package whose
// mutants are all reused reports "passed" when another file of the package
// ran the package's baseline in the same run.
func TestAReusedFileReportsTheBaselineItsPackageRan(t *testing.T) {
	t.Parallel()
	other := filepath.FromSlash("src/other.go")
	extra := map[string]string{"src/other.go": "package board\n\nfunc twice(i int) int {\n\treturn i * 2\n}\n"}
	maps.Copy(extra, clearTest)
	boardRepo(t, extra)
	if o := mutateRun(t, boardSource, other); o.code != 1 {
		t.Fatalf("the first run: exit %d, want 1 for the survivors\n%s%s", o.code, o.stdout, o.stderr)
	}
	edit(t, other, "i * 2", "i * 3")

	o := mutateRun(t, "--json", boardSource, other)
	m := o.json(t)
	if f := m.file(t, other); f.Baseline != "passed" || f.Ran == 0 {
		t.Fatalf("%s: %+v, want its baseline passed and its mutants run\n%s", other, f, o.stderr)
	}
	if f := m.file(t, boardSource); f.Baseline != "passed" || f.Ran != 0 {
		t.Errorf("%s: %+v, want no mutant run and its baseline passed: its package's baseline ran for %s", boardSource, f, other)
	}
}
