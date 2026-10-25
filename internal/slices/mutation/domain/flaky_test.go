package domain

import (
	"strings"
	"testing"
)

func TestSettledWantsThreeRunsThatAgreeWithEachOther(t *testing.T) {
	for _, c := range []struct {
		outcomes []string
		want     string
	}{
		{nil, ""},
		{[]string{Killed}, ""},
		{[]string{Killed, Timeout}, ""},
		{[]string{Killed, Timeout, OverLimit}, Agreed},
		{[]string{Survived, Survived, Survived}, Agreed},
		{[]string{Killed, Survived}, Flaky},
		{[]string{Survived, Survived, Killed}, Flaky},
		{[]string{Timeout, Survived}, Flaky},
	} {
		if got := Settled(c.outcomes); got != c.want {
			t.Errorf("runs %v: %q, want %q", c.outcomes, got, c.want)
		}
	}
}

func TestAnAnnotationOfFlakyMutantsAloneCountsAndListsThem(t *testing.T) {
	snap := Snapshot{Units: []UnitResult{{Namespace: "m", Name: "a", Hash: "h1", Mutants: []Mutant{
		{Line: 2, Offset: 20, Original: ">", Replacement: ">=", Outcome: Survived, Flaky: true, Outcomes: []string{Survived, Killed}},
	}}}}
	want := "// itos-cc mutation: 0 killed, 0 survived, 0 uncovered, 1 flaky\n// flaky: line 2 `>` → `>=` in a\n// end itos-cc mutation\n"
	if got := Annotation(snap, ts, nil); got != want {
		t.Errorf("annotation\n%s\nwant\n%s", got, want)
	}
}

func TestAFlakyMutantAtASiteGoneCountsInNeitherKilledNorSurvived(t *testing.T) {
	entry := &UnitResult{Hash: "h", Killed: 2, Survived: 2, Uncovered: 2, Sites: 7, Mutants: []Mutant{
		{Offset: 1, Original: ">", Replacement: ">=", Outcome: Killed},
		{Offset: 2, Original: "<", Replacement: "<=", Outcome: Survived},
		{Offset: 3, Original: "+", Replacement: "-", Outcome: Killed, Flaky: true, Outcomes: []string{Killed, Survived}},
		{Offset: 4, Original: "0", Replacement: "1", Outcome: Uncovered},
		{Offset: 5, Original: "1", Replacement: "0", Outcome: Timeout},
		{Offset: 6, Original: "-", Replacement: "+", Outcome: Survived},
		{Offset: 7, Original: "*", Replacement: "/", Outcome: Uncovered},
	}}
	sites := []Site{{Offset: 1, Original: ">", Replacement: ">="}, {Offset: 2, Original: "<", Replacement: "<="}, {Offset: 4, Original: "0", Replacement: "1"}}
	got := OnSites(entry, "h", sites)
	if got.Sites != 3 || got.Killed != 1 || got.Survived != 1 || got.Uncovered != 1 || len(got.Mutants) != 3 {
		t.Errorf("on the sites now %+v, want the flaky mutant gone, counted in neither, and each other one less its own count", got)
	}
}

func TestExceptRefusesAFlakyMutant(t *testing.T) {
	for _, outcome := range []string{Survived, Killed} {
		err := survived(outcome, true)
		if err == nil || err.Uncovered || !strings.Contains(err.Reason, "flaky") {
			t.Errorf("a flaky mutant recorded %s: %+v, want it refused as flaky", outcome, err)
		}
	}
	if err := survived(Survived, false); err != nil {
		t.Errorf("a survivor refused: %+v", err)
	}
}

func TestAFlakyMutantNeedsARunAsASurvivorDoes(t *testing.T) {
	fresh := func(m Mutant) []FileCheck {
		return []FileCheck{{Functions: []FunctionCheck{{Freshness: Freshness{State: SnapshotFresh}, Mutants: []Mutant{m}}}}}
	}
	if !NeedsMutationCoverage(fresh(Mutant{Outcome: Killed, Flaky: true, Outcomes: []string{Killed, Survived}}), false) {
		t.Error("a flaky mutant recorded killed needs no run")
	}
	if NeedsMutationCoverage(fresh(Mutant{Outcome: Killed}), false) {
		t.Error("a kill needs a run")
	}
}
