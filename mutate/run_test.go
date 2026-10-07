package mutate

import (
	"slices"
	"testing"
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
	for i := range s.sites {
		s.outcomes[i], s.reused[i] = Killed, i%2 == 0
	}
	all := s.decided()
	var got []string
	for i, m := range all {
		got = append(got, describe([]Site{m.Site})[0])
		if j := slices.IndexFunc(s.sites, func(x Site) bool { return x == m.Site }); m.Reused != s.reused[j] || m.Outcome != Killed {
			t.Errorf("mutant %d: %+v, want the outcome and reuse of its site", i, m)
		}
	}
	if want := []string{"3:>>>=", "3:1>0", "5:<><=", "5:0>1"}; !slices.Equal(got, want) {
		t.Errorf("decided %q, want %q", got, want)
	}

	// Only the functions judged are listed: here f, not the callback.
	outer := all[len(all)-1].Function
	s.judged = map[string]bool{outer: true}
	for _, m := range s.decided() {
		if m.Function != outer {
			t.Errorf("%+v listed, want only the mutants of %s, the function judged", m, outer)
		}
	}
	if n := len(s.decided()); n != 2 {
		t.Errorf("%d mutants listed, want f's 2", n)
	}
}
