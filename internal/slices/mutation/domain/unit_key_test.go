package domain

import (
	"fmt"
	"slices"
	"testing"
)

// pairedFixture is a file's recorded results and what a run that judged
// only some of its functions makes of them.
type pairedFixture struct {
	was   string // the source the snapshot was recorded from
	broad []int  // the units of was whose one mutant rests on the whole suite
	drop  []int  // the units of was whose entry the snapshot never records
	now   string // the source now
	judge []int  // the units of now the run judged
}

// resultsOf is what the run writes for each function it keeps or judges,
// in order, as "name:stale", with "old" after the name for an entry kept
// with the hash it was recorded with, its function changed since. The
// whole-suite evidence of a broad mutant is never recorded, so it counts as
// changed.
func (c pairedFixture) resultsOf(t *testing.T) []string {
	t.Helper()
	before := parse(t, "x.py", c.was)
	snap := Build(before, "x.py", nil, Sites(before), slices.Repeat([]string{Survived}, len(Sites(before))))
	for _, i := range c.broad {
		for j := range snap.Units[i].Mutants {
			snap.Units[i].Mutants[j].Scope = ScopeAllTests
		}
	}
	var kept []UnitResult
	for i, u := range snap.Units {
		if !slices.Contains(c.drop, i) {
			kept = append(kept, u)
		}
	}
	snap.Units = kept

	f := parse(t, "x.py", c.now)
	current := Build(f, "x.py", nil, Sites(f), slices.Repeat([]string{Survived}, len(Sites(f))))
	judged := map[int]bool{}
	for _, i := range c.judge {
		judged[i] = true
	}
	s := &Execution{file: f, stored: &snap, judged: judged, testsHold: true,
		result: &FileResult{Snapshot: current}}
	s.result.Snapshot.Units = KeepUnjudged(s.result.Snapshot.Units, judged, s.stored, s.testsHold)
	s.markBroadStale()

	var out []string
	for _, u := range s.result.Snapshot.Units {
		name := u.Name
		if !slices.ContainsFunc(current.Units, func(n UnitResult) bool { return n.Hash == u.Hash }) {
			name += ":old"
		}
		out = append(out, fmt.Sprintf("%s:%v", name, u.Stale))
	}
	return out
}

func TestResultsPairWithTheirFunctionsByKey(t *testing.T) {
	const abc = "def a(x):\n    return x > 0\n\ndef b(x):\n    return x < 1\n\ndef c(x):\n    return not x\n"
	const sameName = "def a(x):\n    return x > 0\n\ndef a(x):\n    return x < 1\n"
	const swapped = "def a(x):\n    return x < 1\n\ndef a(x):\n    return x > 0\n"
	const sameBody = "def a(x):\n    return x > 0\n\ndef a(x):\n    return x > 0\n"
	for _, c := range []struct {
		name string
		pairedFixture
		want []string
	}{
		{ // 979960b1: one missing entry shifted every later one
			"a function the snapshot never recorded",
			pairedFixture{was: abc, broad: []int{0, 2}, drop: []int{1}, now: abc, judge: []int{1}},
			[]string{"a:true", "b:false", "c:true"},
		},
		{ // 979960b1: and the last kept entry moved past the end
			"the last function's entry missing",
			pairedFixture{was: abc, broad: []int{0, 1}, drop: []int{2}, now: abc, judge: []int{2}},
			[]string{"a:true", "b:true", "c:false"},
		},
		{ // 9a0d79fb: told apart by hash when they share a name
			"two functions sharing a name, different hashes, swapped",
			pairedFixture{was: sameName, broad: []int{0}, now: swapped},
			[]string{"a:false", "a:true"},
		},
		{ // 9a0d79fb: equal hashes pair in file order
			"two functions sharing a name and a hash",
			pairedFixture{was: sameBody, broad: []int{1}, now: sameBody},
			[]string{"a:false", "a:true"},
		},
		{ // 9a0d79fb: one judged, the other kept, each by its own entry
			"two functions sharing a name, one judged",
			pairedFixture{was: sameName, broad: []int{0, 1}, now: sameName, judge: []int{1}},
			[]string{"a:true", "a:false"},
		},
		{ // 2ed38ddf: a function whose hash alone changed keeps its entry
			"a function whose hash alone changed",
			pairedFixture{was: abc, broad: []int{1}, now: "def a(x):\n    return x > 0\n\ndef b(x):\n    return x <= 1\n\ndef c(x):\n    return not x\n"},
			[]string{"a:false", "b:old:true", "c:false"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := c.resultsOf(t); !slices.Equal(got, c.want) {
				t.Errorf("results %v, want %v", got, c.want)
			}
		})
	}
}
