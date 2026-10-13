package coverage

// TestCoverage is each listed test's coverage: which of the tests a
// project lists in itos-cc.yaml (mutation.tests) executed each line, so a
// mutant runs only those that reach it. The zero TestCoverage holds none.
type TestCoverage struct {
	ids     []string // those measured, in the list's order
	reports map[string]*Report
}

// Add records r as the coverage of the listed test id, after those added
// before it.
func (tc *TestCoverage) Add(id string, r *Report) {
	if tc.reports == nil {
		tc.reports = map[string]*Report{}
	}
	tc.ids = append(tc.ids, id)
	tc.reports[id] = r
}

// SetTests gives r the coverage of each listed test.
func (r *Report) SetTests(tc *TestCoverage) {
	if r != nil {
		r.tests = tc
	}
}

// LineTests is the IDs of the listed tests that executed line of file, in
// the list's order.
func (r *Report) LineTests(file string, line int) []string {
	if r == nil || r.tests == nil {
		return nil
	}
	var out []string
	for _, id := range r.tests.ids {
		if covered, _ := r.tests.reports[id].LineCovered(file, line); covered {
			out = append(out, id)
		}
	}
	return out
}
