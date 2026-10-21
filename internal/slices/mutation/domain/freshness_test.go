package mutants

import (
	"reflect"
	"slices"
	"testing"
)

func TestFreshnessUsesTheRecordedJudgmentAndEveryDependency(t *testing.T) {
	site := Site{Offset: 2, Line: 8, Column: 3, Original: ">", Replacement: ">="}
	entry := UnitResult{Hash: "h", Sites: 1, Killed: 1, Mutants: []Mutant{{Offset: 2, Original: ">", Replacement: ">=", Outcome: Killed}}}
	if got := FreshnessOf(&entry, "h", nil, nil, nil, []Site{site}); got.State != SnapshotFresh {
		t.Fatal(got)
	}
	if got := FreshnessOf(nil, "h", nil, nil, nil, nil); got.State != Missing {
		t.Fatal(got)
	}
	if got := FreshnessOf(&entry, "new", nil, nil, nil, nil); got.State != Stale {
		t.Fatal(got)
	}
	tests := &TestChange{Changed: []string{"a_test.go"}}
	if got := FreshnessOf(&entry, "h", tests, nil, nil, nil); got.Tests != tests || got.State != Stale {
		t.Fatal(got)
	}
	marked := entry
	marked.Stale = true
	if got := FreshnessOf(&marked, "h", nil, nil, nil, nil); !got.Marked || got.State != Stale {
		t.Fatal(got)
	}
	listed := &ListedChange{listed: map[string]string{"id": "listed.feature"}, tests: map[string]bool{"listed.feature": true}}
	listedEntry := entry
	listedEntry.Mutants = []Mutant{{Scope: ScopeListed, Tests: []string{"id"}}}
	if got := FreshnessOf(&listedEntry, "h", nil, listed, nil, nil); !slices.Equal(got.Listed, []string{"listed.feature"}) || got.State != Stale {
		t.Fatal(got)
	}
	if got := FreshnessOf(&entry, "h", nil, nil, []string{"support.feature"}, nil); !slices.Equal(got.Broad, []string{"support.feature"}) || got.State != Stale {
		t.Fatal(got)
	}
	newSite := Site{Offset: 5, Line: 4, Column: 1, Original: "0", Replacement: "1"}
	got := FreshnessOf(&entry, "h", nil, nil, nil, []Site{site, newSite})
	if got.State != Stale || !reflect.DeepEqual(got.Unrecorded, []Site{newSite}) {
		t.Fatal(got)
	}
	if !(FunctionCheck{Freshness: got}).Judged() || !(FunctionCheck{Freshness: Freshness{State: SnapshotFresh}}).Judged() || (FunctionCheck{Freshness: Freshness{State: Stale}}).Judged() {
		t.Fatal("sample/except must distinguish retained judgments from missing or changed ones")
	}
}

func TestRemovedSitesDropTheirOwnCountsWithoutChangingTheRecord(t *testing.T) {
	outcomes := []string{Killed, Timeout, Survived, Uncovered}
	entry := UnitResult{Hash: "h", Sites: 4, Killed: 2, Survived: 1, Uncovered: 1}
	for i, o := range outcomes {
		entry.Mutants = append(entry.Mutants, Mutant{Offset: i, Original: "x", Replacement: "y", Outcome: o})
	}
	before := slices.Clone(entry.Mutants)
	for i, o := range outcomes {
		site := Site{Offset: i, Line: 9 - i, Column: 2, Original: "x", Replacement: "y"}
		got := OnSites(&entry, "h", []Site{site})
		if got.Sites != 1 || len(got.Mutants) != 1 || got.Mutants[0].Outcome != o || got.Killed+got.Survived+got.Uncovered != 1 {
			t.Fatal(got)
		}
		placed := Placed(got.Mutants, []Site{site})
		if placed[0].Line != site.Line || placed[0].Column != 2 {
			t.Fatal(placed)
		}
	}
	if !reflect.DeepEqual(entry.Mutants, before) || entry.Sites != 4 {
		t.Fatal("filter changed recorded evidence")
	}
	if OnSites(nil, "h", nil) != nil || OnSites(&entry, "other", nil) != &entry {
		t.Fatal("changed entries must remain stale")
	}
	all := []Site{}
	for i := range outcomes {
		all = append(all, Site{Offset: i, Original: "x", Replacement: "y"})
	}
	if OnSites(&entry, "h", all) != &entry {
		t.Fatal("unchanged population should retain entry")
	}
}

func TestListedEvidenceAttributesChangesOnlyToItsDecidingTests(t *testing.T) {
	m := Mutant{Outcome: Killed, Scope: ScopeListed, Tests: []string{"a"}}
	s := Snapshot{Listed: map[string]string{"a": "a.feature", "b": "b.feature"}, ListedFiles: map[string]string{"a.feature": "a", "b.feature": "b"}, Support: map[string]string{"support": "s"}, Units: []UnitResult{{Mutants: []Mutant{m}}}}
	if s.ListedChanged(s.ListedFiles, s.Support) != nil {
		t.Fatal("matching listed inputs changed")
	}
	c := s.ListedChanged(map[string]string{"a.feature": "new", "b.feature": "b"}, map[string]string{"new": "n"})
	if got := c.Files([]Mutant{m}); !slices.Equal(got, []string{"a.feature", "new", "support"}) {
		t.Fatal(got)
	}
	if got := c.Files([]Mutant{{Scope: ScopeOwn}}); len(got) != 0 {
		t.Fatal(got)
	}
	if (*Snapshot)(nil).ListedChanged(nil, nil) != nil || (&Snapshot{}).ListedChanged(nil, nil) != nil {
		t.Fatal("no listed outcomes have no listed dependencies")
	}
	units := []UnitResult{{Mutants: []Mutant{{Tests: []string{"a", "b"}}}}}
	if got := ListedOf(units, map[string]string{"a": "new.feature"}, &s); !reflect.DeepEqual(got, map[string]string{"a": "new.feature", "b": "b.feature"}) {
		t.Fatal(got)
	}
	if ListedOf(nil, nil, nil) != nil {
		t.Fatal("empty listed inventory must stay absent")
	}
}

func TestPartialAndReusedOutcomesKeepTheirOwnBroadEvidence(t *testing.T) {
	s := Snapshot{Units: []UnitResult{{Mutants: []Mutant{{Scope: ScopeOwn}, {Scope: ScopeAllTests}, {Scope: "go version"}}}}}
	tests := map[string]string{"a_test.go": "old"}
	support := map[string]string{"support": "old"}
	RecordSuiteEvidence(&s, tests, support)
	tests["a_test.go"] = "new"
	support["support"] = "new"
	if !HasBroadOutcome(s.Units) || HasBroadOutcome([]UnitResult{{Mutants: []Mutant{{Scope: ScopeOwn}}}}) || s.Units[0].Mutants[0].SuiteEvidence != nil {
		t.Fatal(s)
	}
	for _, m := range s.Units[0].Mutants[1:] {
		if m.SuiteEvidence.Tests["a_test.go"] != "old" || m.SuiteEvidence.Support["support"] != "old" {
			t.Fatal(m)
		}
	}
	RecordSuiteEvidence(&s, nil, nil)
	if s.Units[0].Mutants[1].SuiteEvidence.Tests == nil || s.Units[0].Mutants[1].SuiteEvidence.Support == nil {
		t.Fatal("complete empty inventories must remain distinguishable from legacy evidence")
	}
	s.Tests = map[string]string{"x": "h"}
	if s.TestsChanged(s.Tests) != nil || s.TestsChanged(map[string]string{}) == nil {
		t.Fatal("importer changes lost")
	}
	MarkExcepted(s.Units, []MutantResult{{Function: "#", Site: Site{}, Excepted: "reviewed"}})
	if s.Units[0].Mutants[1].Excepted != "reviewed" {
		t.Fatal(s)
	}
	MarkExcepted(s.Units, nil)
}

func TestCoveragePlanningNeedsOnlyUncachedOrActionableWork(t *testing.T) {
	fresh := FileCheck{Functions: []FunctionCheck{{Freshness: Freshness{State: SnapshotFresh}, Mutants: []Mutant{{Outcome: Killed}, {Outcome: Survived, Excepted: "reviewed"}}}}}
	if NeedsMutationCoverage([]FileCheck{fresh}, false) || !NeedsMutationCoverage([]FileCheck{fresh}, true) {
		t.Fatal("cache planning changed")
	}
	fresh.Functions[0].Mutants[1].Excepted = ""
	if !NeedsMutationCoverage([]FileCheck{fresh}, false) {
		t.Fatal("unexcepted survivor must rerun")
	}
	fresh.Functions[0].State = Missing
	if !NeedsMutationCoverage([]FileCheck{fresh}, false) {
		t.Fatal("missing function must run")
	}
}
