package domain

import (
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
)

// A site on a later line of a statement split over several lines takes its
// statement's coverage where its own line is unmeasured (slice-79):
// coverage.py marks such a statement on its first line alone. add's `+` on
// line 3 is reached by the listed test that runs line 2, and never's on
// line 8 is uncovered, as line 7 never ran.
func TestASiteOnAStatementsLaterLineTakesItsStatementsCoverage(t *testing.T) {
	f := parse(t, "/p/calc.py", "def add(a, b):\n    return (a\n            + b)\n\n\ndef never(a, b):\n    return (a\n            + b)\n")
	report := coverage.FromLines("python", map[string][]coverage.Line{f.Path: {{Line: 1, Covered: true}, {Line: 2, Covered: true}, {Line: 6, Covered: true}, {Line: 7}}})
	var tests coverage.TestCoverage
	tests.Add("scn1", coverage.FromLines("python", map[string][]coverage.Line{f.Path: {{Line: 2, Covered: true}}}))
	report.SetTests(&tests)
	s := &Execution{file: f, sites: Sites(f)}
	s.outcomes = make([]string, len(s.sites))
	if got := describe(s.sites); !slices.Equal(got, []string{"3:+>-", "8:+>-"}) {
		t.Fatalf("calc.py's sites %q, want add's and never's +", got)
	}
	for i, want := range []int{2, 7} {
		if s.sites[i].Statement != want {
			t.Fatalf("the site on line %d says its statement starts on line %d, want %d", s.sites[i].Line, s.sites[i].Statement, want)
		}
	}
	if err := markUncovered(t.Context(), []*Execution{s}, Options{Coverage: report, Listed: &Listed{}}); err != nil {
		t.Fatal(err)
	}
	if s.outcomes[0] != "" || !slices.Equal(s.reach[0], []string{"scn1"}) {
		t.Errorf("add's + is %q, reached by %v; want pending, reached by scn1", s.outcomes[0], s.reach[0])
	}
	if s.outcomes[1] != Uncovered || len(s.reach[1]) != 0 {
		t.Errorf("never's + is %q, reached by %v; want uncovered, reached by none", s.outcomes[1], s.reach[1])
	}
}
