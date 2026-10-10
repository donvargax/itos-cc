package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// The scenarios of "Rule: Uncovered mutants as failures" in
// features/mutate.feature. They use the module of mutate_since_test.go, with
// coverage measured by the file's own tests: TestPlace executes Board#place,
// so its one mutant runs and is killed, and no test executes Board#clear, so
// its one mutant, on line 11, is uncovered.

// The one mutant of Board#clear: `<` → `<=` at src/board.go:11:11.
const clearLine, clearColumn = 11, 11

// mutateCovered runs itos-cc mutation run with args and coverage measured, no
// source file annotated, and returns what it printed.
func mutateCovered(t *testing.T, args ...string) outcome {
	t.Helper()
	return itosCc(t, append([]string{"mutation", "run", "--no-annotate", "--workers", "1"}, args...)...)
}

// uncoveredLine is how plain output lists an uncovered mutant.
func uncoveredLine(file string, line, column int, original, replacement, function string) string {
	return fmt.Sprintf("  uncovered %s:%d:%d %s → %s in %s\n", file, line, column, quote(original), quote(replacement), function)
}

// @ID-MUT-47
func TestAnUncoveredMutantFailsTheRun(t *testing.T) {
	boardRepo(t, nil)

	o := mutateCovered(t, "--fail-uncovered", boardSource)
	units := boardUnits(t)
	if u := units[placeID]; u.Killed != 1 || u.Survived != 0 || u.Uncovered != 0 {
		t.Fatalf("place in the snapshot: %+v, want its one mutant run and killed\n%s%s", u, o.stdout, o.stderr)
	}
	if u := units[clearID]; u.Uncovered != 1 {
		t.Fatalf("clear in the snapshot: %+v, want its one mutant uncovered\n%s%s", u, o.stdout, o.stderr)
	}
	want := uncoveredLine(boardSource, clearLine, clearColumn, "<", "<=", clearID)
	if !strings.Contains(o.stdout, want) {
		t.Errorf("stdout:\n%s\nwant it to list %q", o.stdout, want)
	}
	if o.code != 1 {
		t.Errorf("exit %d, want 1 for the uncovered mutant\n%s%s", o.code, o.stdout, o.stderr)
	}
}

// @ID-MUT-48
func TestAFileTheTestsNeverLoadFailsWhole(t *testing.T) {
	// No test loads package unused: its coverage measures it, and never
	// executes a line of it.
	unused := filepath.FromSlash("src/unused/unused.go")
	boardRepo(t, map[string]string{
		"src/unused/unused.go": "package unused\n\nfunc Half(i int) bool {\n\treturn i > 2\n}\n\nfunc Zero(i int) bool {\n\treturn i == 0\n}\n",
	})
	sites := cli(t, "mutation", "list", "--json", unused).stdout
	var scan struct {
		Sites []mutateSite `json:"sites"`
	}
	if err := json.Unmarshal([]byte(sites), &scan); err != nil || len(scan.Sites) != 3 {
		t.Fatalf("sites of %s: %v %+v, want 3", unused, err, scan.Sites)
	}

	o := mutateCovered(t, "--fail-uncovered", unused)
	for _, s := range scan.Sites {
		if want := uncoveredLine(s.File, s.Line, s.Column, s.Original, s.Replacement, s.Function); !strings.Contains(o.stdout, want) {
			t.Errorf("stdout lacks %q", want)
		}
	}
	if t.Failed() {
		t.Logf("stdout:\n%s\nstderr:\n%s", o.stdout, o.stderr)
	}
	if o.code != 1 {
		t.Errorf("exit %d, want 1 for the uncovered mutants\n%s%s", o.code, o.stdout, o.stderr)
	}
}

// @ID-MUT-49
func TestWithSinceOnlyTheJudgedFunctionsUncoveredMutantsFail(t *testing.T) {
	// No test executes either function.
	boardRepo(t, map[string]string{
		"src/board_test.go": "package board\n\nimport \"testing\"\n\nfunc TestNothing(t *testing.T) {}\n",
	})
	// A first run records clear's mutant as uncovered, so a run that judged
	// it from the snapshot would list it.
	if o := mutateCovered(t); o.code != 0 {
		t.Fatalf("the first run: exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}
	if u := boardUnits(t)[clearID]; u.Uncovered != 1 {
		t.Fatalf("clear in the snapshot: %+v, want its one mutant uncovered", u)
	}
	changePlace(t)

	o := mutateCovered(t, "--since", "base", "--fail-uncovered")
	want := uncoveredLine(boardSource, 7, 11, ">", ">=", placeID)
	if !strings.Contains(o.stdout, want) {
		t.Errorf("stdout:\n%s\nwant it to list %q", o.stdout, want)
	}
	if strings.Contains(o.stdout, "in "+clearID) {
		t.Errorf("stdout:\n%s\nwant no mutant of %s listed: it is not judged", o.stdout, clearID)
	}
	if o.code != 1 {
		t.Errorf("exit %d, want 1 for place's uncovered mutant\n%s%s", o.code, o.stdout, o.stderr)
	}
}

// @ID-MUT-50
func TestUncoveredMutantsAsJSON(t *testing.T) {
	boardRepo(t, nil)

	o := mutateCovered(t, "--fail-uncovered", "--json", boardSource)
	m := o.json(t)
	p := m.problem("mutation.uncovered")
	want := map[string]any{"file": boardSource, "line": float64(clearLine), "column": float64(clearColumn),
		"function": clearID, "original": "<", "replacement": "<="}
	if p == nil {
		t.Fatalf("no mutation.uncovered problem in:\n%s", o.stdout)
	}
	for k, v := range want {
		if p[k] != v {
			t.Errorf("problem %s = %v, want %v", k, p[k], v)
		}
	}
	if m.OK {
		t.Errorf("ok is true, want false\n%s", o.stdout)
	}
}

// @ID-MUT-51
func TestStrictModeRejectsSkippedCoverage(t *testing.T) {
	boardRepo(t, nil)
	o := cli(t, "mutation", "run", "--no-coverage", "--fail-uncovered", "--json", boardSource)
	wantProblem(t, o.json(t).problem("flags.conflict"), map[string]any{"flag": "--no-coverage"}, o.stdout)
	if o.code != 2 {
		t.Errorf("exit %d, want usage conflict 2\n%s%s", o.code, o.stdout, o.stderr)
	}
}
