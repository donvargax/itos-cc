package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/metrics"
	"github.com/donvargax/itos-cc/mutate"
)

// The scenarios of "Rule: Checking cached results without running" in
// features/mutate.feature about a site the snapshot entry never recorded.
// They use the module of mutate_since_test.go with killedTests, Board#place
// given a second and a third site its test kills too, and stand for an
// operator a newer itos-cc adds by deleting one of place's mutants from
// the snapshot a run wrote.

// unrecordedRepo runs every mutant of src/board.go, all killed, then deletes
// one of Board#place's mutants from its entry, and returns that mutant.
func unrecordedRepo(t *testing.T) mutate.Mutant {
	t.Helper()
	boardRepo(t, killedTests)
	edit(t, boardSource, "return i > 5\n", "return i > 5 && i < 9\n")
	board := filepath.FromSlash("src/board_test.go")
	edit(t, board, "if b.place(5) || !b.place(6) {", "if b.place(5) || !b.place(6) || b.place(9) {")
	if o := mutateRun(t, boardSource); o.code != 0 {
		t.Fatalf("the run: exit %d, want every mutant killed\n%s%s", o.code, o.stdout, o.stderr)
	}
	snap, err := mutate.LoadSnapshot(filepath.ToSlash(boardSource))
	if err != nil || snap == nil {
		t.Fatalf("the snapshot of %s: %v", boardSource, err)
	}
	for i := range snap.Units {
		u := &snap.Units[i]
		if u.Namespace+"#"+u.Name != placeID {
			continue
		}
		if len(u.Mutants) < 2 {
			t.Fatalf("place's entry %+v, want two or more mutants", u)
		}
		gone := u.Mutants[len(u.Mutants)-1]
		u.Mutants = u.Mutants[:len(u.Mutants)-1]
		u.Sites--
		u.Killed--
		if err := metrics.Write(mutate.SnapshotName(filepath.ToSlash(boardSource)), snap); err != nil {
			t.Fatal(err)
		}
		return gone
	}
	t.Fatalf("no entry of %s in %+v", placeID, snap.Units)
	return mutate.Mutant{}
}

// @ID-MUT-106
func TestASiteTheEntryNeverRecordedMakesTheFunctionStale(t *testing.T) {
	gone := unrecordedRepo(t)

	o := mutationCheck(t, "--json", boardSource)
	m := o.json(t)
	p := m.problem("mutation.stale")
	wantProblem(t, p, map[string]any{"file": boardSource, "function": placeID}, o.stdout)
	site := fmt.Sprintf("%d:%d `%s` → `%s`", gone.Line, gone.Column, gone.Original, gone.Replacement)
	if msg, _ := p["message"].(string); !strings.Contains(msg, site) {
		t.Errorf("message %q, want it to name the unrecorded site, %s", msg, site)
	}
	if rules := m.rules(); len(rules) != 1 {
		t.Errorf("problems %v, want only place stale", m.Problems)
	}
	if o.code != 1 {
		t.Errorf("exit %d, want 1", o.code)
	}
}

// @ID-MUT-107
func TestMutationRunRunsOnlyTheSitesTheEntryNeverRecorded(t *testing.T) {
	gone := unrecordedRepo(t)

	o := mutateRun(t, "--json", boardSource)
	f := o.json(t).file(t, boardSource)
	if f.Ran != 1 {
		t.Errorf("ran %d mutants, want 1, the unrecorded site's\n%s%s", f.Ran, o.stdout, o.stderr)
	}
	var place int
	for _, m := range f.mutants(t) {
		if m.Function != placeID {
			continue
		}
		place++
		unrecorded := m.Line == gone.Line && m.Column == gone.Column && m.Original == gone.Original && m.Replacement == gone.Replacement
		switch {
		case unrecorded && (m.Reused || m.Outcome != "killed"):
			t.Errorf("the unrecorded site's mutant %+v, want it run and killed", m)
		case !unrecorded && (!m.Reused || m.Outcome != "killed"):
			t.Errorf("place's mutant %+v, want it reused, killed", m)
		}
	}
	if place < 2 {
		t.Errorf("place's mutants listed: %d, want 2 or more", place)
	}

	if o := mutationCheck(t, boardSource); o.code != 0 {
		t.Errorf("mutation check afterwards: exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}
}
