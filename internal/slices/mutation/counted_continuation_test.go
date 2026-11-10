package mutation

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
	selporttest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	mutantstest "github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

// A counted run reads a site on a statement's later line by its statement's
// coverage where its own line is unmeasured, as a complete run does
// (slice-79): coverage.py marks a statement split over several lines on its
// first line alone. never's `+` on line 8 is uncovered, line 7 never run,
// and add's on line 3 goes to scn1, the listed test that runs line 2, though
// no own test reaches calc.py.
func TestACountedRunReadsASiteOnAStatementsLaterLineByItsStatement(t *testing.T) {
	root := filepath.Join(t.TempDir(), "frozen")
	path := filepath.Join(root, "calc.py")
	src := "def add(a, b):\n    return (a\n            + b)\n\n\ndef never(a, b):\n    return (a\n            + b)\n"
	at := func(line, statement int, from int) mutants.Site {
		start := uint(strings.Index(src[from:], "+") + from)
		return mutants.Site{Start: start, End: start + 1, Line: line, Column: 13, Statement: statement, Original: "+", Replacement: "-"}
	}
	add, never := at(3, 2, 0), at(8, 7, strings.Index(src, "never"))
	report := coverage.FromLines("python", map[string][]coverage.Line{path: {{Line: 1, Covered: true}, {Line: 2}, {Line: 6, Covered: true}, {Line: 7}}})
	var tests coverage.TestCoverage
	tests.Add("scn1", coverage.FromLines("python", map[string][]coverage.Line{path: {{Line: 2, Covered: true}}}))
	report.SetTests(&tests)
	tree := selporttest.TreeOf(root, map[string]string{"calc.py": src}, nil)
	// scn1 passes alone and fails on every mutant it runs.
	workers := &mutantstest.Workers{Files: map[string]string{path: src}, Tests: map[string]map[string]mutantstest.Tests{root: {"python3 scenario.py scn1": {}}}}
	h := Handler{FreshWorkers: workers, Tree: tree, Toolchain: &mutantstest.Inputs{}}
	plan := &mutants.FreshPlan{FrozenRoot: root, Selected: []mutants.FreshCandidate{
		{Path: "calc.py", Function: "calc#add", Site: add}, {Path: "calc.py", Function: "calc#never", Site: never}}}
	listed := &mutants.Tests{List: "python3 scenario.py list", Run: "python3 scenario.py {pattern}", IDsPattern: "{ids}", Each: "{id}", Sep: ","}
	prep := &mutants.FreshPreparation{Report: report, Tests: listed}
	result := &countedResult{Selected: []countedSite{{File: "calc.py", Line: 3, Column: 13, State: "unattempted"}, {File: "calc.py", Line: 8, Column: 13, State: "unattempted"}}}
	var stale []mutants.StaleException
	if err := h.trials(t.Context(), CountedQuery{Workers: 1, TimeoutFactor: 10}, plan, prep, &mutants.FreshExceptions{}, result, &stale); err != nil {
		t.Fatal(err)
	}
	if site := result.Selected[1]; site.State != "uncovered" || site.Outcome != mutants.Uncovered {
		t.Errorf("never's + %+v, want uncovered", site)
	}
	if site := result.Selected[0]; site.State != "judged" || site.Outcome != mutants.Killed || site.Scope != mutants.ScopeListed || !slices.Equal(site.Tests, []string{"scn1"}) {
		t.Errorf("add's + %+v, want killed by scn1, the listed test reaching its statement", site)
	}
}
