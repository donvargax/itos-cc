package domain

import (
	"slices"
	"testing"
)

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
		if got := Agrees(c.recorded, c.outcome); got != c.agree || (SampledMutant{Recorded: c.recorded, Outcome: c.outcome}).Agrees() != c.agree {
			t.Errorf("recorded %s, now %s: agrees %v, want %v", c.recorded, c.outcome, got, c.agree)
		}
	}
}

func TestClassifyTellsAFlakyMutantFromAMismatch(t *testing.T) {
	for _, c := range []struct {
		recorded string
		outcomes []string
		want     string
	}{
		{Killed, nil, ""},
		{Killed, []string{Killed}, Agreed},
		{Killed, []string{OverLimit}, Agreed},
		{Survived, []string{Survived, Killed}, Agreed},
		{Killed, []string{Survived}, ""},
		{Killed, []string{Survived, Survived}, ""},
		{Killed, []string{Survived, Killed}, Flaky},
		{Killed, []string{Survived, Survived, Timeout}, Flaky},
		{Survived, []string{Killed, Survived}, Flaky},
		{Survived, []string{Timeout, Survived}, Flaky},
		{Survived, []string{Timeout, Killed, OverLimit}, Mismatched},
		{Killed, []string{Survived, Survived, Survived}, Mismatched},
	} {
		if got := Classify(c.recorded, c.outcomes); got != c.want {
			t.Errorf("recorded %s, runs %v: %q, want %q", c.recorded, c.outcomes, got, c.want)
		}
		if got := (SampledMutant{Recorded: c.recorded, Outcomes: c.outcomes}).Verdict(); got != c.want {
			t.Errorf("recorded %s, runs %v: verdict %q, want %q", c.recorded, c.outcomes, got, c.want)
		}
	}
}

func TestTheDrawDependsOnTheSeedAndTheMutantAlone(t *testing.T) {
	a := Rank("4813e48", "src/a.go", "m#f", "3:>>>=")
	if b := Rank("4813e48", "src/a.go", "m#f", "3:>>>="); a != b {
		t.Errorf("one seed and mutant ranked %s and %s", a, b)
	}
	if b := Rank("other", "src/a.go", "m#f", "3:>>>="); a == b {
		t.Errorf("two seeds ranked one mutant alike, %s", a)
	}
}

func TestQueuedYieldsAJobRunAgainBeforeTheNextOne(t *testing.T) {
	queue := make(chan job, 3)
	for site := range 3 {
		queue <- job{site: site}
	}
	close(queue)
	var again []job
	var got []int
	for j := range queued(queue, &again) {
		got = append(got, j.site)
		if len(got) == 2 {
			again = append(again, j, j)
		}
	}
	if want := []int{0, 1, 1, 1, 2}; !slices.Equal(got, want) {
		t.Fatalf("yielded sites %v, want %v", got, want)
	}
	for _, stop := range []int{1, 2} {
		again = nil
		queue := make(chan job, 2)
		queue <- job{site: 0}
		queue <- job{site: 1}
		got = nil
		for j := range queued(queue, &again) {
			got = append(got, j.site)
			if len(got) == stop {
				break
			}
			again = append(again, job{site: 9})
		}
		if len(got) != stop {
			t.Fatalf("a loop that stopped after %d jobs yielded %v", stop, got)
		}
	}
}
