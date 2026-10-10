package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The scenario of "Rule: Strict runs prove executable line coverage in
// every language" in features/mutate.feature that the strict-lines-python
// slice holds. It runs a real pytest under coverage.py, and skips, naming
// what is missing, without python3, pytest and coverage; CI runs it on
// ubuntu-latest, where the language tools are installed.
//
// calc.py's reached is reached by tests/test_calc.py, which never takes
// its strict branch, so line 3 is the one line of it no test executes; its
// one mutant, `>` → `>=`, is killed. dormant has no mutation site and no
// test calls it: its body, lines 8 and 9, never runs, while its def line
// runs when the test imports calc.py and is not the function's to prove.
// lonely.py is a file no test reaches at all, so no test runs for it: its
// function, with no mutation site either, has one executable line that
// nothing measured, and strict mode must not pass it.
var strictPythonFiles = map[string]string{
	"pyproject.toml":     "[project]\nname = \"m\"\nversion = \"0.0.0\"\n",
	"calc.py":            "def reached(i, strict):\n    if strict:\n        raise ValueError(\"strict\")\n    return i > 5\n\n\ndef dormant(name):\n    label = name.strip()\n    return label\n",
	"tests/test_calc.py": "from calc import reached\n\n\ndef test_reached():\n    assert reached(6, False)\n    assert not reached(5, False)\n",
	"lonely.py":          "def lonely(name):\n    return name.upper()\n",
}

// strictPythonLines is each unexecuted executable line the strict run
// reports, as strictLineFindings lists them.
var strictPythonLines = []string{
	"mutation.uncovered-statement calc.py calc#dormant:8",
	"mutation.uncovered-statement calc.py calc#dormant:9",
	"mutation.uncovered-statement calc.py calc#reached:3",
	"mutation.uncovered-statement lonely.py lonely#lonely:2",
}

// strictLineRules are the rules of strict statement coverage, separate
// from mutant outcomes.
var strictLineRules = []string{"mutation.uncovered-statement", "mutation.coverage-missing", "mutation.coverage-stale", "mutation.coverage-unsupported"}

// strictLineFindings is each problem of o of a strict statement coverage
// rule, as "rule file function:line", sorted, with file in slash form.
func strictLineFindings(t *testing.T, o outcome) []string {
	t.Helper()
	var out []string
	for _, p := range o.json(t).Problems {
		rule, _ := p["rule"].(string)
		if !slices.Contains(strictLineRules, rule) {
			continue
		}
		file, _ := p["file"].(string)
		line, _ := p["line"].(float64)
		out = append(out, fmt.Sprintf("%s %s %v:%d", rule, filepath.ToSlash(file), p["function"], int(line)))
	}
	slices.Sort(out)
	return out
}

// strictPythonCounts is each file of a mutation run's --json output with
// its mutant counts, as "file killed/survived/uncovered".
func strictPythonCounts(t *testing.T, o outcome) []string {
	t.Helper()
	var out []string
	for _, f := range o.json(t).Files {
		out = append(out, fmt.Sprintf("%s %d/%d/%d", filepath.ToSlash(f.File), f.Killed, f.Survived, f.Uncovered))
	}
	slices.Sort(out)
	return out
}

// problemRules is the rule of each problem of o, sorted.
func problemRules(t *testing.T, o outcome) []string {
	t.Helper()
	var out []string
	for _, p := range o.json(t).Problems {
		rule, _ := p["rule"].(string)
		out = append(out, rule)
	}
	slices.Sort(out)
	return out
}

// requireNoTestRun fails when o's log shows a coverage, baseline or test
// command.
func requireNoTestRun(t *testing.T, o outcome, what string) {
	t.Helper()
	for _, ran := range []string{"itos-cc: coverage", "itos-cc: baseline", "pytest"} {
		if strings.Contains(o.stderr, ran) {
			t.Errorf("%s ran a command (%q):\n%s", what, ran, o.stderr)
		}
	}
}

// @ID-MUT-221
func TestAStrictPythonRunProvesEveryExecutableLineOfEveryJudgedFunction(t *testing.T) {
	t.Parallel()
	languageTools(t, "python")
	// pytest plugins installed beside pytest are no part of the scenario.
	useEnv(t, "PYTEST_DISABLE_PLUGIN_AUTOLOAD", "1")
	// Given a Python project whose judged file has a function with no
	// mutation site that no test executes, and a reached function with one
	// line no test executes
	dir := t.TempDir()
	for name, text := range strictPythonFiles {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(name)), text)
	}
	useDir(t, dir)
	selection := []string{"calc.py", "lonely.py"}
	// What a run and a check without --fail-uncovered report, before any
	// strict command runs.
	before := mutateCovered(t, append([]string{"--json"}, selection...)...)
	logPytestRun(t, &before)
	if before.code == 70 || before.code == 3 || len(before.json(t).Files) != 2 {
		t.Fatalf("setup run without --fail-uncovered: exit %d\n%s%s", before.code, before.stdout, before.stderr)
	}
	beforeCheck := mutationCheck(t, append([]string{"--json"}, selection...)...)

	// When I run "itos-cc mutation run --fail-uncovered --json" for the file
	strict := mutateCovered(t, append([]string{"--fail-uncovered", "--json"}, selection...)...)
	logPytestRun(t, &strict)
	// Then each unexecuted executable line is reported as
	// "mutation.uncovered-statement" with its file, function and line
	if got := strictLineFindings(t, strict); !slices.Equal(got, strictPythonLines) {
		t.Errorf("the strict run reports %q, want %q", got, strictPythonLines)
	}
	for _, p := range strict.json(t).Problems {
		if p["rule"] == "mutation.uncovered-statement" && (p["original"] != nil || p["replacement"] != nil) {
			t.Errorf("uncovered statement %v invents a mutation", p)
		}
	}
	// the mutant counts are unchanged
	if got, want := strictPythonCounts(t, strict), strictPythonCounts(t, before); !slices.Equal(got, want) {
		t.Errorf("the strict run counts mutants %q, want %q as without --fail-uncovered", got, want)
	}
	// and the run fails
	if strict.code != 1 {
		t.Errorf("the strict run exit %d, want 1", strict.code)
	}

	// And "itos-cc mutation check --fail-uncovered" reports the same
	// findings without running a test
	check := mutationCheck(t, append([]string{"--fail-uncovered", "--json"}, selection...)...)
	if got := strictLineFindings(t, check); !slices.Equal(got, strictPythonLines) {
		t.Errorf("the strict check reports %q, want %q as the run does\n%s%s", got, strictPythonLines, check.stdout, check.stderr)
	}
	if check.code != 1 {
		t.Errorf("the strict check exit %d, want 1\n%s", check.code, check.stdout)
	}
	requireNoTestRun(t, check, "the strict check")

	// And after a test that reaches the file changes, the check reports
	// "mutation.coverage-stale" until a new run measures it again
	test := filepath.Join(dir, "tests", "test_calc.py")
	data, err := os.ReadFile(test)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, test, string(data)+"\n\ndef test_again():\n    assert reached(7, False)\n")
	stale := mutationCheck(t, "--fail-uncovered", "--json", "calc.py")
	wantStale := []string{"mutation.coverage-stale calc.py calc#dormant:7", "mutation.coverage-stale calc.py calc#reached:1"}
	if got := strictLineFindings(t, stale); !slices.Equal(got, wantStale) {
		t.Errorf("after tests/test_calc.py changed, the strict check reports %q, want %q\n%s", got, wantStale, stale.stdout)
	}
	for _, p := range stale.json(t).Problems {
		if msg, _ := p["message"].(string); p["rule"] == "mutation.coverage-stale" && !strings.Contains(msg, "tests/test_calc.py") {
			t.Errorf("stale message %q, want it to name tests/test_calc.py", msg)
		}
	}
	if stale.code != 1 {
		t.Errorf("the stale check exit %d, want 1", stale.code)
	}
	requireNoTestRun(t, stale, "the stale check")
	again := mutateCovered(t, append([]string{"--fail-uncovered", "--json"}, selection...)...)
	logPytestRun(t, &again)
	if got := strictLineFindings(t, again); !slices.Equal(got, strictPythonLines) {
		t.Errorf("the strict run after the test changed reports %q, want %q", got, strictPythonLines)
	}
	measured := mutationCheck(t, append([]string{"--fail-uncovered", "--json"}, selection...)...)
	if got := strictLineFindings(t, measured); !slices.Equal(got, strictPythonLines) {
		t.Errorf("once a new run measured it, the strict check reports %q, want %q\n%s", got, strictPythonLines, measured.stdout)
	}

	// And a strict Python run refuses --coverage-report,
	// --use-existing-coverage and --coverage-command
	for _, args := range [][]string{
		{"--coverage-report", "lcov.info"},
		{"--use-existing-coverage"},
		{"--coverage-command", "python3 -m coverage lcov -o lcov.info"},
	} {
		command := append(append([]string{"mutation", "run", "--fail-uncovered", "--json"}, args...), "calc.py")
		o := cli(t, command...)
		p := o.json(t).problem("flags.conflict")
		if p == nil || p["flag"] != args[0] {
			t.Errorf("strict Python run with %v reports %v, want flags.conflict naming %s\n%s", args, p, args[0], o.stdout)
		}
		if o.code != 2 {
			t.Errorf("strict Python run with %v exit %d, want 2\n%s%s", args, o.code, o.stdout, o.stderr)
		}
	}

	// But without --fail-uncovered the run and the check report as before
	after := mutateCovered(t, append([]string{"--json"}, selection...)...)
	logPytestRun(t, &after)
	if got := strictLineFindings(t, after); len(got) != 0 {
		t.Errorf("the run without --fail-uncovered reports %q", got)
	}
	if got, want := strictPythonCounts(t, after), strictPythonCounts(t, before); !slices.Equal(got, want) {
		t.Errorf("the run without --fail-uncovered counts %q, want %q as before", got, want)
	}
	if got, want := problemRules(t, after), problemRules(t, before); after.code != before.code || !slices.Equal(got, want) {
		t.Errorf("the run without --fail-uncovered: exit %d %q, want exit %d %q as before", after.code, got, before.code, want)
	}
	afterCheck := mutationCheck(t, append([]string{"--json"}, selection...)...)
	if got, want := problemRules(t, afterCheck), problemRules(t, beforeCheck); afterCheck.code != beforeCheck.code || !slices.Equal(got, want) {
		t.Errorf("the check without --fail-uncovered: exit %d %q, want exit %d %q as before\n%s", afterCheck.code, got, beforeCheck.code, want, afterCheck.stdout)
	}
}
