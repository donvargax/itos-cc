package features

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// A coverage run of the listed tests that fails without any mutant (slice-15,
// issue #15): the harness fails its run of every test when
// ITOS_CC_TEST_COVERDIR is set and GREET_FAIL_COVERAGE says so, as
// GREET_FAIL_ALONE fails a selection of ID-A-02 alone.

// failCoverageVariable has the harness fail every run given
// ITOS_CC_TEST_COVERDIR, which only the listed tests' coverage run is.
const failCoverageVariable = "GREET_FAIL_COVERAGE=1"

// listedRun is the command line the fixture's itos-cc.yaml makes of a
// selection of listed tests, its IDs joined by commas.
func listedRun(ids ...string) string {
	return "go test -count=1 ./e2e -args -tests=" + strings.Join(ids, ",")
}

func (l *listedMutation) failingCoverage() error {
	if err := l.project(); err != nil {
		return err
	}
	if err := l.replace("e2e/e2e_test.go", "\tbin := filepath.Join(t.TempDir(), \"greet.exe\")",
		"\tif os.Getenv(\"GREET_FAIL_COVERAGE\") != \"\" && os.Getenv(\"ITOS_CC_TEST_COVERDIR\") != \"\" { t.Fatal(\"the coverage run fails\") }\n\tbin := filepath.Join(t.TempDir(), \"greet.exe\")"); err != nil {
		return err
	}
	l.m.w.variables = append(l.m.w.variables, failCoverageVariable)
	return l.declare()
}

// coverageProblem is whether the run reports tests.coverage-failed naming
// the command that ran ids, and go test's exit code for a failing test.
func (l *listedMutation) coverageProblem(ids ...string) error {
	if err := l.m.w.problemIs("tests.coverage-failed", "", ""); err != nil {
		return err
	}
	if want := listedRun(ids...); l.m.w.problem["command"] != want {
		return fmt.Errorf("tests.coverage-failed names the command %v, want %q\n%s", l.m.w.problem["command"], want, l.m.w.report())
	}
	if l.m.w.problem["exit_code"] != float64(1) {
		return fmt.Errorf("tests.coverage-failed lacks exit_code 1: %v\n%s", l.m.w.problem, l.m.w.report())
	}
	return nil
}

// noProblem is whether the last run, made with --json, reports no problem
// of rule.
func (l *listedMutation) noProblem(rule string) error {
	object, report, err := l.m.w.lastObject()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	for _, one := range problems {
		if problem, _ := one.(map[string]any); problem["rule"] == rule {
			return fmt.Errorf("a problem is %q: %v\n%s", rule, problem, report)
		}
	}
	return nil
}

// coverageReached is whether the scenario has the listed tests' coverage run
// get as far as running them: a run of the harness is then no mutant's
// while it ran main.go as the fixture holds it.
func (l *listedMutation) coverageReached() bool {
	return slices.Contains(l.m.w.variables, failCoverageVariable) || slices.Contains(l.m.w.variables, "GREET_FAIL_ALONE=1")
}

// mutantRan is whether a record of the harness is a run of a mutant: any
// run of the listed tests where no coverage run reaches them, and otherwise
// one that ran another main.go than the fixture's.
func (l *listedMutation) mutantRan(record map[string]any) bool {
	if record["test"] != "run" {
		return false
	}
	if !l.coverageReached() {
		return true
	}
	return record["source_sha256"] != fmt.Sprintf("%x", sha256.Sum256([]byte(l.original)))
}

// snapshotUnwritten is whether the run wrote no snapshot of main.go: none
// is there, or the one an earlier run left (held) is as it was.
func (l *listedMutation) snapshotUnwritten() bool {
	if l.held != "" {
		return l.m.read(".metrics/mutate/main.go.json") == l.held
	}
	return !l.m.w.d.Exists(filepath.Join(l.m.w.dir, ".metrics", "mutate", "main.go.json"))
}
