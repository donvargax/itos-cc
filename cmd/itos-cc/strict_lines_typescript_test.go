package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The scenario of "Rule: Strict runs prove executable line coverage in
// every language" in features/mutate.feature that the
// strict-lines-typescript slice holds. It runs a real Vitest with its v8
// provider, the one languageTools finds, and skips where none is
// installed; CI runs it on ubuntu-latest, where the language tools are.
// It reuses the Python scenario's helpers (strict_lines_python_test.go).
//
// src/calc.ts's reached is reached by src/calc.test.ts, which never takes
// its strict branch, so line 3 is the one line of it no test executes; its
// one mutant, `>` → `>=`, is killed. dormant has no mutation site and no
// test calls it: its body, lines 9 and 10, never runs, and v8 names no line
// of a function declaration itself. src/lonely.ts is imported by no test,
// so Vitest's run never loads it: its function, with no mutation site
// either, has one executable line that nothing measured, and strict mode
// must not pass it.
var strictTypeScriptFiles = map[string]string{
	"package.json": `{"name": "strict", "private": true, "type": "module", ` +
		`"devDependencies": {"vitest": "5.0.2", "@vitest/coverage-v8": "5.0.2"}}` + "\n",
	"src/calc.ts": "export function reached(i: number, strict: boolean): boolean {\n  if (strict) {\n    throw new Error(\"strict\");\n  }\n  return i > 5;\n}\n\n" +
		"export function dormant(name: string): string {\n  const label = name.trim();\n  return label;\n}\n",
	"src/calc.test.ts": "import { expect, test } from \"vitest\";\nimport { reached } from \"./calc\";\n\n" +
		"test(\"reached\", () => {\n  expect(reached(6, false)).toBe(true);\n  expect(reached(5, false)).toBe(false);\n});\n",
	"src/lonely.ts": "export function lonely(name: string): string {\n  return name.toUpperCase();\n}\n",
}

// strictTypeScriptLines is each unexecuted executable line the strict run
// reports, as strictLineFindings lists them.
var strictTypeScriptLines = []string{
	"mutation.uncovered-statement src/calc.ts calc#dormant:10",
	"mutation.uncovered-statement src/calc.ts calc#dormant:9",
	"mutation.uncovered-statement src/calc.ts calc#reached:3",
	"mutation.uncovered-statement src/lonely.ts lonely#lonely:2",
}

// logRun logs o when t fails.
func logRun(t *testing.T, o *outcome) {
	t.Helper()
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("exit %d\nstdout:\n%s\nstderr:\n%s", o.code, o.stdout, o.stderr)
		}
	})
}

// @ID-MUT-222
func TestAStrictTypeScriptRunProvesEveryExecutableLineOfEveryJudgedFunction(t *testing.T) {
	t.Parallel()
	modules := languageTools(t, "typescript")
	// Given a TypeScript project tested with Vitest whose judged file has a
	// function with no mutation site that no test executes, and a reached
	// function with one line no test executes
	dir := t.TempDir()
	if err := os.Symlink(modules, filepath.Join(dir, "node_modules")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	for name, text := range strictTypeScriptFiles {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(name)), text)
	}
	useDir(t, dir)
	selection := []string{filepath.FromSlash("src/calc.ts"), filepath.FromSlash("src/lonely.ts")}
	// What a run and a check without --fail-uncovered report, before any
	// strict command runs.
	before := mutateCovered(t, append([]string{"--json"}, selection...)...)
	logRun(t, &before)
	if before.code == 70 || before.code == 3 || len(before.json(t).Files) != 2 {
		t.Fatalf("setup run without --fail-uncovered: exit %d\n%s%s", before.code, before.stdout, before.stderr)
	}
	beforeCheck := mutationCheck(t, append([]string{"--json"}, selection...)...)

	// When I run "itos-cc mutation run --fail-uncovered --json" for the file
	strict := mutateCovered(t, append([]string{"--fail-uncovered", "--json"}, selection...)...)
	logRun(t, &strict)
	// Then each unexecuted executable line is reported as
	// "mutation.uncovered-statement" with its file, function and line
	if got := strictLineFindings(t, strict); !slices.Equal(got, strictTypeScriptLines) {
		t.Errorf("the strict run reports %q, want %q", got, strictTypeScriptLines)
	}
	for _, p := range strict.json(t).Problems {
		if p["rule"] == "mutation.uncovered-statement" && (p["original"] != nil || p["replacement"] != nil) {
			t.Errorf("uncovered statement %v invents a mutation", p)
		}
	}
	// the mutant counts are unchanged
	if got, want := strictMutantCounts(t, strict), strictMutantCounts(t, before); !slices.Equal(got, want) {
		t.Errorf("the strict run counts mutants %q, want %q as without --fail-uncovered", got, want)
	}
	// and the run fails
	if strict.code != 1 {
		t.Errorf("the strict run exit %d, want 1", strict.code)
	}

	// And "itos-cc mutation check --fail-uncovered" reports the same
	// findings without running a test
	check := mutationCheck(t, append([]string{"--fail-uncovered", "--json"}, selection...)...)
	if got := strictLineFindings(t, check); !slices.Equal(got, strictTypeScriptLines) {
		t.Errorf("the strict check reports %q, want %q as the run does\n%s%s", got, strictTypeScriptLines, check.stdout, check.stderr)
	}
	if check.code != 1 {
		t.Errorf("the strict check exit %d, want 1\n%s", check.code, check.stdout)
	}
	requireNoTestRun(t, check, "the strict check")

	// And after a test that reaches the file changes, the check reports
	// "mutation.coverage-stale" until a new run measures it again
	test := filepath.Join(dir, "src", "calc.test.ts")
	data, err := os.ReadFile(test)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, test, string(data)+"\ntest(\"again\", () => {\n  expect(reached(7, false)).toBe(true);\n});\n")
	stale := mutationCheck(t, "--fail-uncovered", "--json", filepath.FromSlash("src/calc.ts"))
	wantStale := []string{"mutation.coverage-stale src/calc.ts calc#dormant:8", "mutation.coverage-stale src/calc.ts calc#reached:1"}
	if got := strictLineFindings(t, stale); !slices.Equal(got, wantStale) {
		t.Errorf("after src/calc.test.ts changed, the strict check reports %q, want %q\n%s", got, wantStale, stale.stdout)
	}
	for _, p := range stale.json(t).Problems {
		if msg, _ := p["message"].(string); p["rule"] == "mutation.coverage-stale" && !strings.Contains(msg, "src/calc.test.ts") {
			t.Errorf("stale message %q, want it to name src/calc.test.ts", msg)
		}
	}
	if stale.code != 1 {
		t.Errorf("the stale check exit %d, want 1", stale.code)
	}
	requireNoTestRun(t, stale, "the stale check")
	again := mutateCovered(t, append([]string{"--fail-uncovered", "--json"}, selection...)...)
	logRun(t, &again)
	if got := strictLineFindings(t, again); !slices.Equal(got, strictTypeScriptLines) {
		t.Errorf("the strict run after the test changed reports %q, want %q", got, strictTypeScriptLines)
	}
	measured := mutationCheck(t, append([]string{"--fail-uncovered", "--json"}, selection...)...)
	if got := strictLineFindings(t, measured); !slices.Equal(got, strictTypeScriptLines) {
		t.Errorf("once a new run measured it, the strict check reports %q, want %q\n%s", got, strictTypeScriptLines, measured.stdout)
	}

	// And a strict TypeScript run refuses --coverage-report,
	// --use-existing-coverage and --coverage-command
	for _, args := range [][]string{
		{"--coverage-report", "lcov.info"},
		{"--use-existing-coverage"},
		{"--coverage-command", "vitest run --coverage.enabled --coverage.reporter=lcov"},
	} {
		command := append(append([]string{"mutation", "run", "--fail-uncovered", "--json"}, args...), filepath.FromSlash("src/calc.ts"))
		o := cli(t, command...)
		p := o.json(t).problem("flags.conflict")
		if p == nil || p["flag"] != args[0] {
			t.Errorf("strict TypeScript run with %v reports %v, want flags.conflict naming %s\n%s", args, p, args[0], o.stdout)
		}
		if o.code != 2 {
			t.Errorf("strict TypeScript run with %v exit %d, want 2\n%s%s", args, o.code, o.stdout, o.stderr)
		}
	}

	// But without --fail-uncovered the run and the check report as before
	after := mutateCovered(t, append([]string{"--json"}, selection...)...)
	logRun(t, &after)
	if got := strictLineFindings(t, after); len(got) != 0 {
		t.Errorf("the run without --fail-uncovered reports %q", got)
	}
	if got, want := strictMutantCounts(t, after), strictMutantCounts(t, before); !slices.Equal(got, want) {
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
