package main

import (
	"maps"
	"path/filepath"
	"strings"
	"testing"
)

// The scenario of "Rule: Mutation results say what happened, and leave
// nothing stale behind" in features/mutate.feature that the
// reused-baseline-not-run-fix slice holds. It runs on the Go module of
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

// baselineLines are the lines of stderr that say a baseline runs.
func baselineLines(stderr string) []string {
	var lines []string
	for _, line := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(line, "itos-cc: baseline ") {
			lines = append(lines, line)
		}
	}
	return lines
}

// @ID-MUT-240
func TestAFileWhoseMutantsAreAllReusedReportsItsBaselineAsNotRun(t *testing.T) {
	t.Parallel()
	// Given a file whose snapshot is fresh, so that a run reuses every one
	// of its mutants
	boardRepo(t, clearTest)
	if o := mutateRun(t, boardSource); o.code != 0 {
		t.Fatalf("the first run: exit %d, want 0 with both mutants killed\n%s%s", o.code, o.stdout, o.stderr)
	}

	// When I run "itos-cc mutation run --json" for that file
	o := mutateRun(t, "--json", boardSource)

	// Then its "baseline" is "not-run", and it ran no mutant
	f := o.json(t).file(t, boardSource)
	for _, id := range []string{placeID, clearID} {
		if reused, listed := reusedOf(t, o, id); listed != 1 || reused != 1 {
			t.Fatalf("%s: %d of %d mutants reused, want its one mutant reused\n%s", id, reused, listed, o.stdout)
		}
	}
	if f.Ran != 0 {
		t.Fatalf("%s: %+v, want no mutant run\n%s", boardSource, f, o.stderr)
	}
	if lines := baselineLines(o.stderr); len(lines) > 0 {
		t.Errorf("a baseline ran with every mutant reused: %q", lines)
	}
	if f.Baseline != "not-run" {
		t.Errorf("%s: baseline %q, want \"not-run\": every mutant was reused, so no baseline ran", boardSource, f.Baseline)
	}

	// And after an edit to one of its functions, the next run reports its
	// "baseline" as "passed"
	changePlace(t)
	o = mutateRun(t, "--json", boardSource)
	if f := o.json(t).file(t, boardSource); f.Baseline != "passed" || f.Ran != 1 {
		t.Errorf("%s after the edit: %+v, want its baseline passed and place's one mutant run\n%s", boardSource, f, o.stderr)
	}
}

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
