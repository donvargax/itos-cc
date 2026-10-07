package mutate

import "testing"

func TestKilledAndTimeoutAgree(t *testing.T) {
	for _, c := range []struct {
		recorded, outcome string
		agree             bool
	}{
		{Killed, Killed, true},
		{Killed, Timeout, true},
		{Timeout, Killed, true},
		{Survived, Survived, true},
		{Killed, Survived, false},
		{Timeout, Survived, false},
		{Survived, Killed, false},
		{Survived, Timeout, false},
	} {
		if got := (SampledMutant{Recorded: c.recorded, Outcome: c.outcome}).Agrees(); got != c.agree {
			t.Errorf("recorded %s, now %s: agrees %v, want %v", c.recorded, c.outcome, got, c.agree)
		}
	}
}

func TestTheDrawDependsOnTheSeedAndTheMutantAlone(t *testing.T) {
	a := rank("4813e48", "src/a.go", "m#f", "3:>>>=")
	if b := rank("4813e48", "src/a.go", "m#f", "3:>>>="); a != b {
		t.Errorf("one seed and mutant ranked %s and %s", a, b)
	}
	if b := rank("other", "src/a.go", "m#f", "3:>>>="); a == b {
		t.Errorf("two seeds ranked one mutant alike, %s", a)
	}
}
