package main

import (
	"strings"
	"testing"
)

// The scenario of "Rule: Mutation results say what happened, and leave
// nothing stale behind" in features/mutate.feature that the
// unreached-baseline-not-run-fix slice holds. It runs on the project of
// ownTestsRepo (mutate_python_own_tests_test.go): no test the graph sees
// reaches u.py, so its own tests run nothing and no baseline runs for it,
// while test_a reaches a.py and passes. It needs a real python3 with pytest
// and coverage, and skips, naming the missing one, without them; CI runs it
// on ubuntu-latest (T-13).

// @ID-MUT-231
func TestAFileNoTestReachesReportsItsBaselineAsNotRun(t *testing.T) {
	t.Parallel()
	languageTools(t, "python")
	useEnv(t, "PYTEST_DISABLE_PLUGIN_AUTOLOAD", "1")
	// Given a Python project with a file no test reaches
	ownTestsRepo(t, "pytest")

	// When I run "itos-cc mutation run --json" for that file
	o := mutateCovered(t, "--json", "a.py", "u.py")
	logPytestRun(t, &o)

	// Then its "baseline" is "not-run" and its mutants are uncovered
	u := o.json(t).file(t, "u.py")
	if u.Baseline != "not-run" {
		t.Errorf("u.py: baseline %q, want \"not-run\": no test reaches it, so none ran", u.Baseline)
	}
	if u.Uncovered != 1 || u.Ran != 0 || u.Killed != 0 || u.Survived != 0 {
		t.Errorf("u.py: %+v, want its one mutant uncovered and none run", u)
	}
	for _, line := range strings.Split(o.stderr, "\n") {
		if strings.Contains(line, "baseline") && strings.Contains(line, "u.py") {
			t.Errorf("a baseline ran for u.py: %s", line)
		}
	}

	// And a file whose tests ran reports its baseline as "passed", as before
	if a := o.json(t).file(t, "a.py"); a.Baseline != "passed" {
		t.Errorf("a.py: baseline %q, want \"passed\": test_a ran without any mutant and passed", a.Baseline)
	}
}
