package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/metricsdir"
	mutation "github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/metrics"
)

// The scenarios of "Rule: Mutation results say what happened, and leave
// nothing stale behind" in features/mutation.feature that the
// check-removed-operator-sites-fix slice holds, the reverse of
// mutation_unrecorded_test.go's, and the check-left-out-count slice. They use the module of
// mutate_since_test.go with killedTests, and stand for an operator a newer
// itos-cc removes by adding to Board#place's entry a surviving mutant at
// the place of its `>` site with a replacement no operator makes now, `<`.

// removedSiteRepo runs every mutant of src/board.go, all killed, then adds
// to Board#place's entry a survivor at a site the function no longer holds,
// and returns that mutant.
func removedSiteRepo(t *testing.T) mutation.Mutant {
	t.Helper()
	return removedSiteRepoWith(t, mutation.Survived)
}

// removedSiteRepoWith is removedSiteRepo with the added mutant's outcome
// outcome, counted in the entry's counts.
func removedSiteRepoWith(t *testing.T, outcome string) mutation.Mutant {
	t.Helper()
	boardRepo(t, killedTests)
	if o := mutateRun(t, boardSource); o.code != 0 {
		t.Fatalf("the run: exit %d, want every mutant killed\n%s%s", o.code, o.stdout, o.stderr)
	}
	snap, err := loadMutationSnapshot(t.Context(), metricsdir.Store{RootDir: wd(t)}, filepath.ToSlash(boardSource))
	if err != nil || snap == nil {
		t.Fatalf("the snapshot of %s: %v", boardSource, err)
	}
	for i := range snap.Units {
		u := &snap.Units[i]
		if u.Namespace+"#"+u.Name != placeID {
			continue
		}
		var gone *mutation.Mutant
		for j := range u.Mutants {
			if m := u.Mutants[j]; m.Original == ">" && m.Replacement == ">=" {
				m.Replacement, m.Outcome = "<", outcome
				gone = &m
			}
		}
		if gone == nil {
			t.Fatalf("place's entry %+v, want its `>` → `>=` mutant", u)
		}
		u.Mutants = append(u.Mutants, *gone)
		u.Sites++
		switch outcome {
		case mutation.Survived:
			u.Survived++
		case mutation.Killed:
			u.Killed++
		default:
			t.Fatalf("an added mutant %s, want survived or killed", outcome)
		}
		data, err := json.MarshalIndent(metricsdir.SnapshotJSON(*snap), "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(metrics.Name, metricsdir.MutationName(filepath.ToSlash(boardSource))), string(data)+"\n")
		return *gone
	}
	t.Fatalf("no entry of %s in %+v", placeID, snap.Units)
	return mutation.Mutant{}
}

// isMutant is whether m is at gone's place with its change.
func isMutant(m mutantJSON, gone mutation.Mutant) bool {
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
		if fn.State != mutation.SnapshotFresh || fn.Survived != 0 || fn.Killed != 1 {
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
		case m.Function == placeID && (!m.Reused || m.Outcome != mutation.Killed):
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

// @ID-MUT-241
func TestCheckSaysHowManyRecordedMutantsItLeftOut(t *testing.T) {
	t.Parallel()
	removedSiteRepoWith(t, mutation.Killed)

	o := mutationCheck(t, "--json", boardSource)
	var out struct {
		Files []struct {
			File      string           `json:"file"`
			Functions []map[string]any `json:"functions"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(o.stdout), &out); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s\nstderr:\n%s", err, o.stdout, o.stderr)
	}
	var place int
	for _, f := range out.Files {
		for _, fn := range f.Functions {
			leftOut, has := fn["left_out"]
			switch {
			case f.File == boardSource && fn["function"] == placeID:
				place++
				if leftOut != float64(1) {
					t.Errorf("place in mutation check %v, want \"left_out\" 1", fn)
				}
			case has:
				t.Errorf("%s's %v has \"left_out\", want it only on place", f.File, fn)
			}
		}
	}
	if place != 1 {
		t.Errorf("mutation check lists place %d times, want once\n%s", place, o.stdout)
	}
	if o.code != 0 {
		t.Errorf("mutation check: exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}

	o = mutationCheck(t, boardSource)
	want := fmt.Sprintf("  left out %s:%d in %s: 1 recorded mutant at sites gone, which the next mutation run drops\n",
		boardSource, boardUnits(t)[placeID].StartLine, placeID)
	if !strings.Contains(o.stdout, want) || strings.Count(o.stdout, "left out ") != 1 {
		t.Errorf("mutation check prints\n%s\nwant one line %q", o.stdout, want)
	}
	if o.code != 0 {
		t.Errorf("mutation check: exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}
}
