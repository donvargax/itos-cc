package mutate

import (
	"slices"
	"testing"
	"time"
)

func TestDecidedMutantsAreInLineOrder(t *testing.T) {
	// The route callback is a unit of its own inside f, so Sites lists f's
	// line 5 before the callback's line 3.
	f := parse(t, "x.ts", `export function f(app, a) {
  app.get("/u", (req, res) => {
    return req.x > 1;
  });
  return a < 0;
}
`)
	s := &fileState{file: f, sites: Sites(f)}
	s.outcomes = make([]string, len(s.sites))
	s.reused = make([]bool, len(s.sites))
	s.scopes = make([]string, len(s.sites))
	for i := range s.sites {
		s.outcomes[i], s.reused[i], s.scopes[i] = Killed, i%2 == 0, []string{ScopeOwn, ScopeAllTests}[i%2]
	}
	all := s.decided()
	var got []string
	for i, m := range all {
		got = append(got, describe([]Site{m.Site})[0])
		if j := slices.IndexFunc(s.sites, func(x Site) bool { return x == m.Site }); m.Reused != s.reused[j] || m.Scope != s.scopes[j] || m.Outcome != Killed {
			t.Errorf("mutant %d: %+v, want the outcome, reuse and scope of its site", i, m)
		}
	}
	if want := []string{"3:>>>=", "3:1>0", "5:<><=", "5:0>1"}; !slices.Equal(got, want) {
		t.Errorf("decided %q, want %q", got, want)
	}

	// Only the functions judged are listed: here f, not the callback.
	outer := all[len(all)-1].Function
	s.judged = map[int]bool{all[len(all)-1].Unit: true}
	for _, m := range s.decided() {
		if m.Function != outer {
			t.Errorf("%+v listed, want only the mutants of %s, the function judged", m, outer)
		}
	}
	if n := len(s.decided()); n != 2 {
		t.Errorf("%d mutants listed, want f's 2", n)
	}
}

// @ID-MUT-149
func TestAMutantsTimeoutLeavesAFixedAllowanceBeyondItsBaseline(t *testing.T) {
	if got, want := timeoutOf(50*time.Millisecond, 10), 5500*time.Millisecond; got != want {
		t.Errorf("a baseline of 50ms at the default factor of 10: a timeout of %v, want %v, ten times it plus 5s", got, want)
	}
}
