package domain

import (
	"slices"
	"testing"
)

// importerSnapshot records one importing test and the listed tests filed,
// in a hashed file, printed, with a fingerprint, bare, with neither, and
// unread, in a file it could not hash.
func importerSnapshot() *Snapshot {
	return &Snapshot{
		Tests:              map[string]string{"a_test.go": "1"},
		Listed:             map[string]string{"filed": "a.feature", "printed": "", "bare": "", "unread": "b.feature"},
		ListedFiles:        map[string]string{"a.feature": "h", "b.feature": ""},
		ListedFingerprints: map[string]string{"printed": "f1"},
	}
}

func listedOutcome(offset int, outcome string, ids ...string) Mutant {
	return Mutant{Offset: offset, Original: ">", Replacement: ">=", Outcome: outcome, Scope: ScopeListed, Tests: ids}
}

func TestOnlyDatedListedKillsStandApartFromTheImportingTests(t *testing.T) {
	s := importerSnapshot()
	for _, c := range []struct {
		name  string
		m     Mutant
		rests bool
	}{
		{"a kill by a test of a hashed file", listedOutcome(1, Killed, "filed"), false},
		{"a timeout by a fingerprinted test", listedOutcome(1, Timeout, "printed"), false},
		{"an over-limit by both", listedOutcome(1, OverLimit, "filed", "printed"), false},
		{"a survivor of listed tests", listedOutcome(1, Survived, "filed"), true},
		{"a kill by a test with no file or fingerprint", listedOutcome(1, Killed, "bare"), true},
		{"a kill by a test whose file was unreadable", listedOutcome(1, Killed, "unread"), true},
		{"a kill one of whose tests is undated", listedOutcome(1, Killed, "filed", "bare"), true},
		{"a listed kill naming no test", listedOutcome(1, Killed), true},
		{"an own kill", Mutant{Outcome: Killed}, true},
		{"an all-tests kill", Mutant{Outcome: Killed, Scope: ScopeAllTests}, true},
		{"an uncovered mutant", Mutant{Outcome: Uncovered}, true},
	} {
		if got := s.RestsOnImporters(c.m); got != c.rests {
			t.Errorf("%s: rests on the importing tests %v, want %v", c.name, got, c.rests)
		}
	}
	unrecorded := *s
	unrecorded.Tests = nil
	if !unrecorded.RestsOnImporters(listedOutcome(1, Killed, "filed")) {
		t.Error("a snapshot that records no tests: want every outcome resting on them")
	}
	if !s.AnyRestsOnImporters(nil) {
		t.Error("an entry with no mutant: want it resting on the importing tests")
	}
}

func TestUsableKeepsOnlyWhatHoldsOnceTheImportingTestsChanged(t *testing.T) {
	s := importerSnapshot()
	s.Units = []UnitResult{{Name: "f", Hash: "h", Mutants: []Mutant{listedOutcome(1, Killed, "filed"), {Offset: 2, Outcome: Killed}}}}
	if got := Usable(s, s.Tests); got != s {
		t.Fatalf("the same tests: want the snapshot itself, got %+v", got)
	}
	got := Usable(s, map[string]string{"a_test.go": "2"})
	if len(got.Units) != 1 || got.Units[0].Hash != "h" || len(got.Units[0].Mutants) != 1 || got.Units[0].Mutants[0].Offset != 1 {
		t.Fatalf("changed tests: want the listed kill alone, got %+v", got.Units)
	}
	if len(s.Units[0].Mutants) != 2 {
		t.Fatal("Usable changed the snapshot")
	}
	if Usable(nil, nil) != nil {
		t.Fatal("no snapshot: want nil")
	}
}

func TestFreshnessStalesOnTheImportingTestsOnlyWhatRestsOnThem(t *testing.T) {
	s := importerSnapshot()
	now := map[string]string{"a_test.go": "2"}
	tests := s.TestsChanged(now)
	kill := listedOutcome(1, Killed, "filed")
	own := Mutant{Offset: 2, Original: "1", Replacement: "0", Outcome: Killed}
	listedOnly := UnitResult{Hash: "h", Mutants: []Mutant{kill}}
	mixed := UnitResult{Hash: "h", Mutants: []Mutant{kill, own}}
	if got := FreshnessOf(&listedOnly, "h", tests, nil, nil, nil); got.State != SnapshotFresh || got.Tests != nil {
		t.Fatalf("dated listed kills alone: %+v", got)
	}
	if got := FreshnessOf(&mixed, "h", tests, nil, nil, nil); got.State != Stale || got.Tests != tests {
		t.Fatalf("an own kill beside them: %+v", got)
	}
	s.Units = []UnitResult{listedOnly}
	listed := s.ListedChanged(map[string]string{"a.feature": "h2"}, nil)
	if got := FreshnessOf(&listedOnly, "h", tests, listed, nil, nil); got.State != Stale || got.Tests != nil || !slices.Equal(got.Listed, []string{"a.feature"}) {
		t.Fatalf("the listed test's file changed too: %+v", got)
	}
	if got := FreshnessOf(&mixed, "h", tests, listed, nil, nil); got.Tests != tests || got.Listed != nil {
		t.Fatalf("a mixed entry names the importing tests first: %+v", got)
	}
	marked := listedOnly
	marked.Stale = true
	if got := FreshnessOf(&marked, "h", tests, nil, nil, nil); !got.Marked || got.State != Stale {
		t.Fatalf("a marked entry: %+v", got)
	}
	if got := FreshnessOf(&listedOnly, "h", CompareTests(s.Tests, now), nil, nil, nil); got.State != Stale {
		t.Fatalf("a change of no snapshot bears on every entry: %+v", got)
	}
	if got := FreshnessOf(&UnitResult{Hash: "h"}, "h", tests, nil, nil, nil); got.State != Stale {
		t.Fatalf("an entry with no mutant: %+v", got)
	}
}

func TestKeepUnjudgedMarksOnlyEntriesRestingOnChangedImportingTests(t *testing.T) {
	prev := importerSnapshot()
	kill := listedOutcome(1, Killed, "filed")
	kill.Line = 3
	own := Mutant{Offset: 2, Original: "1", Replacement: "0", Outcome: Killed}
	prev.Units = []UnitResult{
		{Namespace: "p", Name: "listed", Hash: "h1", Mutants: []Mutant{kill}},
		{Namespace: "p", Name: "mixed", Hash: "h2", Mutants: []Mutant{kill, own}},
		{Namespace: "p", Name: "moved", Hash: "h3", Mutants: []Mutant{kill}},
	}
	// The run kept listed's kill, which Usable left it, at its line now;
	// moved changed since, and is kept as recorded.
	now := []UnitResult{
		{Namespace: "p", Name: "listed", Hash: "h1", Mutants: []Mutant{{Offset: 1, Original: ">", Replacement: ">=", Outcome: Killed, Line: 9}}},
		{Namespace: "p", Name: "mixed", Hash: "h2"},
		{Namespace: "p", Name: "moved", Hash: "h4"},
	}
	got := KeepUnjudged(now, map[int]bool{}, prev, false)
	if len(got) != 3 || got[0].Stale || !got[1].Stale || got[2].Stale || got[2].Hash != "h3" {
		t.Fatalf("changed importing tests: want only mixed marked, got %+v", got)
	}
	if m := got[0].Mutants; len(m) != 1 || m[0].Line != 9 || !slices.Equal(m[0].Tests, []string{"filed"}) {
		t.Fatalf("the listed kill: want it as recorded at its line now, got %+v", m)
	}
	if got := KeepUnjudged(now, map[int]bool{}, prev, true); got[1].Stale {
		t.Fatalf("the importing tests held: want mixed unmarked, got %+v", got)
	}
}
