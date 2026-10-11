package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/metrics"
	"github.com/donvargax/itos-cc/mutate"
)

// The scenario of "Rule: Mutation results say what happened, and leave
// nothing stale behind" in features/mutate.feature that the
// check-removed-operator-sites-fix slice holds, the reverse of
// mutation_unrecorded_test.go's. They use the module of
// mutate_since_test.go with killedTests, and stand for an operator a newer
// itos-cc removes by adding to Board#place's entry a surviving mutant at
// the place of its `>` site with a replacement no operator makes now, `<`.

// removedSiteRepo runs every mutant of src/board.go, all killed, then adds
// to Board#place's entry a survivor at a site the function no longer holds,
// and returns that mutant.
func removedSiteRepo(t *testing.T) mutate.Mutant {
	t.Helper()
	boardRepo(t, killedTests)
	if o := mutateRun(t, boardSource); o.code != 0 {
		t.Fatalf("the run: exit %d, want every mutant killed\n%s%s", o.code, o.stdout, o.stderr)
	}
	snap, err := mutate.LoadSnapshotOf(wd(t), filepath.ToSlash(boardSource))
	if err != nil || snap == nil {
		t.Fatalf("the snapshot of %s: %v", boardSource, err)
	}
	for i := range snap.Units {
		u := &snap.Units[i]
		if u.Namespace+"#"+u.Name != placeID {
			continue
		}
		var gone *mutate.Mutant
		for j := range u.Mutants {
			if m := u.Mutants[j]; m.Original == ">" && m.Replacement == ">=" {
				m.Replacement, m.Outcome = "<", mutate.Survived
				gone = &m
			}
		}
		if gone == nil {
			t.Fatalf("place's entry %+v, want its `>` → `>=` mutant", u)
		}
		u.Mutants = append(u.Mutants, *gone)
		u.Sites++
		u.Survived++
		data, err := json.MarshalIndent(snap, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(metrics.Name, mutate.SnapshotName(filepath.ToSlash(boardSource))), string(data)+"\n")
		return *gone
	}
	t.Fatalf("no entry of %s in %+v", placeID, snap.Units)
	return mutate.Mutant{}
}

// isMutant is whether m is at gone's place with its change.
func isMutant(m mutantJSON, gone mutate.Mutant) bool {
	return m.Line == gone.Line && m.Column == gone.Column && m.Original == gone.Original && m.Replacement == gone.Replacement
}

// @ID-MUT-235
func TestARecordedMutantWhoseSiteIsGoneIsLeftOut(t *testing.T) {
	t.Parallel()
	gone := removedSiteRepo(t)

	o := mutationCheck(t, "--json", boardSource)
	if p := o.json(t).problem("mutation.survived"); p != nil {
		t.Errorf("mutation check reports %v, want nothing for a mutant whose site is gone\n%s", p, o.stdout)
	}
	if o.code != 0 {
		t.Errorf("mutation check: exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}
	var place int
	for _, fn := range o.checkFunctions(t)[boardSource] {
		if fn.Function != placeID {
			continue
		}
		place++
		if fn.State != mutate.Fresh || fn.Survived != 0 || fn.Killed != 1 {
			t.Errorf("place in mutation check %+v, want fresh with 1 killed and no survivor", fn)
		}
	}
	if place != 1 {
		t.Errorf("mutation check lists place %d times, want once\n%s", place, o.stdout)
	}

	o = mutateRun(t, "--json", boardSource)
	f := o.json(t).file(t, boardSource)
	if f.Ran != 0 || o.code != 0 {
		t.Errorf("mutation run: ran %d mutants, exit %d, want 0 and 0\n%s%s", f.Ran, o.code, o.stdout, o.stderr)
	}
	for _, m := range f.mutants(t) {
		switch {
		case isMutant(m, gone):
			t.Errorf("mutation run lists %+v, a mutant whose site is gone", m)
		case m.Function == placeID && (!m.Reused || m.Outcome != mutate.Killed):
			t.Errorf("place's mutant %+v, want it reused, killed", m)
		}
	}
	u := boardUnits(t)[placeID]
	for _, m := range u.Mutants {
		if m.Original == gone.Original && m.Replacement == gone.Replacement {
			t.Errorf("place's entry after the run %+v, want it without the mutant whose site is gone", u)
		}
	}
	if u.Sites != 1 || u.Killed != 1 || u.Survived != 0 {
		t.Errorf("place's entry after the run: %d sites, %d killed, %d survived, want 1, 1, 0", u.Sites, u.Killed, u.Survived)
	}
	if o := mutationCheck(t, boardSource); o.code != 0 {
		t.Errorf("mutation check afterwards: exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}
}

// An exception of the mutant whose site is gone is stale as gone, as one of
// a site an edit removed is (ID-MUT-94).
func TestAnExceptionOfARemovedSiteIsGone(t *testing.T) {
	t.Parallel()
	gone := removedSiteRepo(t)
	u := boardUnits(t)[placeID]
	writeExceptions(t, "", exceptionYAML{File: "src/board.go", Function: placeID, Hash: u.Hash,
		LineInFunction: gone.Line - u.StartLine + 1, Column: gone.Column, Original: gone.Original,
		Replacement: gone.Replacement, Reason: "an operator itos-cc no longer has"})

	o := mutationCheck(t, "--json", boardSource)
	wantProblem(t, o.json(t).problem("mutation.exception-stale"), map[string]any{"file": boardSource, "function": placeID, "why": "gone"}, o.stdout)
	if o.code != 1 {
		t.Errorf("mutation check: exit %d, want 1 for the stale exception", o.code)
	}
}
