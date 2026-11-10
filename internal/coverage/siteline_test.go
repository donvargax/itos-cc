package coverage_test

import (
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
)

// A site on a later line of a statement split over several lines reads the
// coverage of the nearest line above it the report measures, no higher than
// the statement's first line (slice-79): coverage.py and Vitest mark such a
// statement on its first line alone.
func TestASiteOnAnUnmeasuredLineOfAStatementReadsItsStatementsLine(t *testing.T) {
	const file = "/p/calc.py"
	// add's return on lines 2 and 3, run; never's on 7 and 8, not run.
	r := coverage.FromLines("python", map[string][]coverage.Line{file: {{Line: 1, Covered: true}, {Line: 2, Covered: true}, {Line: 6, Covered: true}, {Line: 7}}})
	for _, c := range []struct {
		name                  string
		line, statement, want int
	}{
		{"a site on a statement's second line reads its first", 3, 2, 2},
		{"one never run reads its first, measured and never run", 8, 7, 7},
		{"a measured line reads itself", 2, 2, 2},
		{"a measured line never run reads itself", 7, 6, 7},
		{"no measured line from the statement down reads itself, unmeasured", 5, 4, 5},
		// The bound: line 2 is measured but above the statement's start.
		{"a measured line above the statement's first is never read", 4, 3, 4},
		{"a statement of 0 bounds nothing", 3, 0, 3},
		{"a statement after the line bounds nothing", 3, 9, 3},
	} {
		if got := r.SiteLine(file, c.line, c.statement); got != c.want {
			t.Errorf("%s: SiteLine(%d, %d) = %d, want %d", c.name, c.line, c.statement, got, c.want)
		}
	}
	first := coverage.FromLines("python", map[string][]coverage.Line{file: {{Line: 1, Covered: true}}})
	if got := first.SiteLine(file, 2, 1); got != 1 {
		t.Errorf("a statement starting on the file's first line: SiteLine = %d, want 1", got)
	}
	if got := r.SiteLine("/p/other.py", 3, 2); got != 3 {
		t.Errorf("a file the report never names: SiteLine = %d, want 3", got)
	}
	var none *coverage.Report
	if got := none.SiteLine(file, 3, 2); got != 3 {
		t.Errorf("no report: SiteLine = %d, want 3", got)
	}
}

// An arrow function's body on a later line, which Vitest measures by
// itself, keeps its own answer though its statement's first line ran: the
// mutant there is never taken for covered.
func TestAnArrowFunctionsBodyOnALaterLineKeepsItsOwnCoverage(t *testing.T) {
	const file = "/p/src/inc.ts"
	r := coverage.FromLines("typescript", map[string][]coverage.Line{file: {{Line: 2, Covered: true}, {Line: 3}}})
	line := r.SiteLine(file, 3, 2)
	if covered, measured := r.LineCovered(file, line); line != 3 || covered || !measured {
		t.Fatalf("the arrow's body reads line %d, covered %v and measured %v; want line 3, measured, never run", line, covered, measured)
	}
}

// A listed test's report measures a line the own tests' report never
// names: the site's line is chosen by what either measures, so LineTests
// reads the line the listed test's report marks.
func TestASiteLineIsChosenByWhatTheListedTestsMeasureToo(t *testing.T) {
	const file = "/p/demo.py"
	r := coverage.FromLines("python", map[string][]coverage.Line{})
	var tests coverage.TestCoverage
	tests.Add("scn1", coverage.FromLines("python", map[string][]coverage.Line{file: {{Line: 2, Covered: true}}}))
	r.SetTests(&tests)
	line := r.SiteLine(file, 3, 2)
	if ids := r.LineTests(file, line); line != 2 || len(ids) != 1 || ids[0] != "scn1" {
		t.Fatalf("the site reads line %d, reached by %v; want line 2, by scn1", line, ids)
	}
	if got := r.SiteLine(file, 5, 4); got != 5 {
		t.Fatalf("a line neither report measures: SiteLine = %d, want 5", got)
	}
}
