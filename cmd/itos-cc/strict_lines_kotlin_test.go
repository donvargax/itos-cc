package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The scenario of "Rule: Strict runs prove executable line coverage in
// every language" in features/mutate.feature that the strict-lines-kotlin
// slice holds. It runs a real Gradle with JaCoCo, the build of
// kotlinOwnBuilds (mutate_kotlin_own_tests_test.go), and skips where no JDK
// or Gradle is installed; CI runs it on ubuntu-latest, where the language
// tools are. It reuses the Python and TypeScript scenarios' helpers.
//
// Calc.kt's reached is reached by CalcTest, which never takes its strict
// branch, so line 5 is the one line of it no test executes. Line 4, the if,
// and line 7, whose && never sees i < 100 false, each run one branch only:
// partly executed, they are covered. Its declaration, line 3, holds the
// default-argument bridge of label, which no test calls: JaCoCo names the
// line, all of it missed, but it is no line of the function's body. dormant
// has no mutation site and no test calls it: its body, lines 11 and 12,
// never runs. Span is a data class whose generated accessors JaCoCo puts on
// its declaration line, 15, which belongs to no function. Lonely.kt, in
// another package, is reached by no test, so none runs for it: its
// function, with no mutation site either, has one executable line, and
// strict mode must not pass it.
var strictKotlinFiles = map[string]string{
	"src/main/kotlin/calc/Calc.kt": "package calc\n\n" +
		"fun reached(i: Int, strict: Boolean, label: String = \"strict\"): Boolean {\n" +
		"    if (strict) {\n        throw IllegalStateException(label)\n    }\n" +
		"    return i > 5 && i < 100\n}\n\n" +
		"fun dormant(name: String, suffix: String = \"!\"): String {\n" +
		"    val label = name.trim() + suffix\n    return label\n}\n\n" +
		"data class Span(val from: String, val to: String)\n",
	"src/test/kotlin/calc/CalcTest.kt": "package calc\n\nimport kotlin.test.Test\nimport kotlin.test.assertFalse\nimport kotlin.test.assertTrue\n\n" +
		"class CalcTest {\n    @Test\n    fun reaches() {\n        assertTrue(reached(6, false))\n        assertFalse(reached(5, false))\n    }\n}\n",
	"src/main/kotlin/other/Lonely.kt": "package other\n\nfun lonely(name: String): String {\n    return name.uppercase()\n}\n",
}

// strictKotlinLines is each unexecuted executable line the strict run
// reports, as strictLineFindings lists them.
var strictKotlinLines = []string{
	"mutation.uncovered-statement src/main/kotlin/calc/Calc.kt calc#dormant:11",
	"mutation.uncovered-statement src/main/kotlin/calc/Calc.kt calc#dormant:12",
	"mutation.uncovered-statement src/main/kotlin/calc/Calc.kt calc#reached:5",
	"mutation.uncovered-statement src/main/kotlin/other/Lonely.kt other#lonely:4",
}

// requireNoGradleRun fails when o's log shows a coverage, baseline or test
// command, Gradle's included.
func requireNoGradleRun(t *testing.T, o outcome, what string) {
	t.Helper()
	requireNoTestRun(t, o, what)
	if strings.Contains(o.stderr, "gradle") {
		t.Errorf("%s ran Gradle:\n%s", what, o.stderr)
	}
}

// @ID-MUT-223
func TestAStrictKotlinRunProvesEveryExecutableLineOfEveryJudgedFunction(t *testing.T) {
	t.Parallel()
	languageTools(t, "kotlin")
	// Given a Kotlin project built with Gradle and JaCoCo whose judged file
	// has a function with no mutation site that no test executes, a reached
	// function with one line no test executes, and a line a test executes
	// only one branch of
	dir := t.TempDir()
	for name, text := range strictKotlinFiles {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(name)), text)
	}
	for name, text := range kotlinOwnBuilds["gradle"] {
		writeFile(t, filepath.Join(dir, name), text)
	}
	useDir(t, dir)
	calc := filepath.FromSlash("src/main/kotlin/calc/Calc.kt")
	selection := []string{calc, filepath.FromSlash("src/main/kotlin/other/Lonely.kt")}
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
	// "mutation.uncovered-statement" with its file, function and line, and
	// the partly executed line is not
	if got := strictLineFindings(t, strict); !slices.Equal(got, strictKotlinLines) {
		t.Errorf("the strict run reports %q, want %q", got, strictKotlinLines)
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
	if got := strictLineFindings(t, check); !slices.Equal(got, strictKotlinLines) {
		t.Errorf("the strict check reports %q, want %q as the run does\n%s%s", got, strictKotlinLines, check.stdout, check.stderr)
	}
	if check.code != 1 {
		t.Errorf("the strict check exit %d, want 1\n%s", check.code, check.stdout)
	}
	requireNoGradleRun(t, check, "the strict check")

	// And after a test that reaches the file changes, the check reports
	// "mutation.coverage-stale" until a new run measures it again
	test := filepath.Join(dir, "src", "test", "kotlin", "calc", "CalcTest.kt")
	data, err := os.ReadFile(test)
	if err != nil {
		t.Fatal(err)
	}
	// Another test of the same calls: the mutants' outcomes stay the same.
	writeFile(t, test, strings.Replace(string(data), "\n}\n", "\n\n    @Test\n    fun again() {\n        assertTrue(reached(6, false))\n    }\n}\n", 1))
	stale := mutationCheck(t, "--fail-uncovered", "--json", calc)
	wantStale := []string{
		"mutation.coverage-stale src/main/kotlin/calc/Calc.kt calc#dormant:10",
		"mutation.coverage-stale src/main/kotlin/calc/Calc.kt calc#reached:3",
	}
	if got := strictLineFindings(t, stale); !slices.Equal(got, wantStale) {
		t.Errorf("after CalcTest.kt changed, the strict check reports %q, want %q\n%s", got, wantStale, stale.stdout)
	}
	for _, p := range stale.json(t).Problems {
		if msg, _ := p["message"].(string); p["rule"] == "mutation.coverage-stale" && !strings.Contains(msg, "src/test/kotlin/calc/CalcTest.kt") {
			t.Errorf("stale message %q, want it to name src/test/kotlin/calc/CalcTest.kt", msg)
		}
	}
	if stale.code != 1 {
		t.Errorf("the stale check exit %d, want 1", stale.code)
	}
	requireNoGradleRun(t, stale, "the stale check")
	again := mutateCovered(t, append([]string{"--fail-uncovered", "--json"}, selection...)...)
	logRun(t, &again)
	if got := strictLineFindings(t, again); !slices.Equal(got, strictKotlinLines) {
		t.Errorf("the strict run after the test changed reports %q, want %q", got, strictKotlinLines)
	}
	measured := mutationCheck(t, append([]string{"--fail-uncovered", "--json"}, selection...)...)
	if got := strictLineFindings(t, measured); !slices.Equal(got, strictKotlinLines) {
		t.Errorf("once a new run measured it, the strict check reports %q, want %q\n%s", got, strictKotlinLines, measured.stdout)
	}

	// And a strict Kotlin run refuses --coverage-report,
	// --use-existing-coverage and --coverage-command
	for _, args := range [][]string{
		{"--coverage-report", filepath.FromSlash("build/reports/jacoco/test/jacocoTestReport.xml")},
		{"--use-existing-coverage"},
		{"--coverage-command", "gradle test jacocoTestReport"},
	} {
		command := append(append([]string{"mutation", "run", "--fail-uncovered", "--json"}, args...), calc)
		o := cli(t, command...)
		p := o.json(t).problem("flags.conflict")
		if p == nil || p["flag"] != args[0] {
			t.Errorf("strict Kotlin run with %v reports %v, want flags.conflict naming %s\n%s", args, p, args[0], o.stdout)
		}
		if o.code != 2 {
			t.Errorf("strict Kotlin run with %v exit %d, want 2\n%s%s", args, o.code, o.stdout, o.stderr)
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

// A strict run of the same Kotlin project built with Maven proves the same
// lines from jacoco-maven-plugin's report, whose XML lists every class of
// the module's output as Gradle's jacocoTestReport does, and its check
// gives the same verdict without running a test. It is no scenario of its
// own: @ID-MUT-223 names Gradle, and this holds the other build tool to it.
func TestAStrictKotlinRunBuiltWithMavenProvesTheSameLines(t *testing.T) {
	t.Parallel()
	languageTools(t, "maven")
	dir := t.TempDir()
	for name, text := range strictKotlinFiles {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(name)), text)
	}
	for name, text := range kotlinOwnBuilds["maven"] {
		writeFile(t, filepath.Join(dir, name), text)
	}
	useDir(t, dir)
	selection := []string{filepath.FromSlash("src/main/kotlin/calc/Calc.kt"), filepath.FromSlash("src/main/kotlin/other/Lonely.kt")}
	strict := mutateCovered(t, append([]string{"--fail-uncovered", "--json"}, selection...)...)
	logRun(t, &strict)
	if got := strictLineFindings(t, strict); !slices.Equal(got, strictKotlinLines) {
		t.Errorf("the strict Maven run reports %q, want %q", got, strictKotlinLines)
	}
	if strict.code != 1 {
		t.Errorf("the strict Maven run exit %d, want 1", strict.code)
	}
	check := mutationCheck(t, append([]string{"--fail-uncovered", "--json"}, selection...)...)
	if got := strictLineFindings(t, check); !slices.Equal(got, strictKotlinLines) || check.code != 1 {
		t.Errorf("the strict Maven check: exit %d %q, want exit 1 %q\n%s%s", check.code, got, strictKotlinLines, check.stdout, check.stderr)
	}
	requireNoTestRun(t, check, "the strict Maven check")
	if strings.Contains(check.stderr, "mvn") {
		t.Errorf("the strict Maven check ran Maven:\n%s", check.stderr)
	}
}
