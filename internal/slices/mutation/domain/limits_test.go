package domain

import (
	"testing"
	"time"
)

// TestLimitsAreSizedFromTheBaseline holds q-23's sizing for every limit:
// the baseline's use times its factor, plus a fixed allowance, and no
// memory cap where the baseline's memory was not measured.
func TestLimitsAreSizedFromTheBaseline(t *testing.T) {
	f := Factors{Timeout: 10, Output: 10, Memory: 4}
	measured := f.LimitsOf(Usage{Elapsed: time.Second, Output: 2 << 20, Memory: 100 << 20, MemoryMeasured: true})
	if want := (Limits{Timeout: 15 * time.Second, Output: 36 << 20, Memory: 656 << 20}); measured != want {
		t.Errorf("limits %+v, want %+v", measured, want)
	}
	unmeasured := f.LimitsOf(Usage{Elapsed: time.Second, Output: 2 << 20, Memory: 100 << 20})
	if unmeasured.Memory != 0 || unmeasured.Output != 36<<20 {
		t.Errorf("limits where memory is not measured %+v, want no memory cap", unmeasured)
	}
}

// TestARunOverACapIsAKill holds over-limit beside timeout: a run over a
// cap is judged OverLimit, whatever it exited with, and counts as noticed
// wherever a kill does, sample's agreement among them.
func TestARunOverACapIsAKill(t *testing.T) {
	if got := Judged(CommandResult{OverLimit: LimitOutput, Passed: true}); got != OverLimit {
		t.Errorf("a run over its output cap is %q", got)
	}
	for _, outcome := range []string{Killed, Timeout, OverLimit} {
		if !Noticed(outcome) {
			t.Errorf("%s is not noticed", outcome)
		}
		if !(SampledMutant{Recorded: OverLimit, Outcome: outcome}).Agrees() {
			t.Errorf("a recorded over-limit kill disagrees with %s now", outcome)
		}
	}
	if Noticed(Survived) || Noticed(Uncovered) {
		t.Error("a survivor or an uncovered mutant is noticed")
	}
}

// TestKeptOutputSaysWhereBytesWereLeftOut reads a result's output as kept:
// whole while it wrote no more than it kept, else cut where the bytes were
// left out.
func TestKeptOutputSaysWhereBytesWereLeftOut(t *testing.T) {
	if o := (CommandResult{Output: "a\nb\n"}).Kept(); o.Cut || o.Text != "a\nb\n" {
		t.Errorf("whole output kept as %+v", o)
	}
	if o := (CommandResult{Output: "head\ntail\n", OutputBytes: 1000, OutputCut: 5, OutputLeftOut: 98}).Kept(); !o.Cut || o.At != 5 || o.LeftOut != 98 {
		t.Errorf("cut output kept as %+v", o)
	}
}

// TestASnapshotRecordsTheCapAnOverLimitKillWentOver holds the key added:
// an over-limit mutant records its cap and counts as killed, every other
// outcome records none, and a run reuses the kill with its cap.
func TestASnapshotRecordsTheCapAnOverLimitKillWentOver(t *testing.T) {
	f := parse(t, "x.py", "def a(x):\n    return x > 0\n")
	sites := Sites(f)
	outcomes := []string{OverLimit, Killed}
	snap := BuildLimited(f, "x.py", nil, sites, outcomes, nil, []string{LimitMemory, ""}, nil)
	u := snap.Units[0]
	if u.Killed != 2 || u.Survived != 0 {
		t.Fatalf("counts killed %d, survived %d", u.Killed, u.Survived)
	}
	for _, m := range u.Mutants {
		if want := map[string]string{OverLimit: LimitMemory, Killed: ""}[m.Outcome]; m.Limit != want {
			t.Errorf("%s records cap %q, want %q", m.Outcome, m.Limit, want)
		}
	}
	prev, scopes := RememberedWithScopes(&snap, f)
	for _, s := range sites {
		outcome, kept := prev.Kept(f, s, false)
		if !kept {
			t.Errorf("%s is not reused", outcome)
		}
		if outcome == OverLimit && scopes[s.Unit][s.Key()].Limit != LimitMemory {
			t.Errorf("the reused over-limit kill lost its cap: %+v", scopes[s.Unit][s.Key()])
		}
	}
}
