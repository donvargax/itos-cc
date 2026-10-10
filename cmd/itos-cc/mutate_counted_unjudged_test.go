//go:build !windows

package main

import (
	"strings"
	"testing"
)

// The scenario of "Rule: Counted and Python runs touch only what their
// judgments need" in features/mutate.feature that the
// counted-unjudged-language slice holds. Counted mode refuses Windows, so
// the step builds only where it runs; it drives the CLI and reads its
// --json object as raw maps, so it compiles against the current product.
//
// The TypeScript file lives in web/, whose package.json declares Vitest
// but which never installs it: there is no node_modules anywhere in the
// project, and tools are only ever taken from the project's own, so no
// TypeScript coverage tool is installed on any machine, CI's included.

// unjudgedFiles is a Go module whose test kills Compare's one mutant,
// beside web/label.ts.
func unjudgedFiles(label string) map[string]string {
	return map[string]string{
		"go.mod":           compareFiles["go.mod"],
		"main.go":          compareFiles["main.go"],
		"main_test.go":     compareFiles["main_test.go"],
		"web/package.json": "{\n  \"name\": \"web\",\n  \"private\": true,\n  \"devDependencies\": {\n    \"vitest\": \"^3.0.0\",\n    \"@vitest/coverage-v8\": \"^3.0.0\"\n  }\n}\n",
		"web/label.ts":     label,
	}
}

const (
	unjudgedLabel = "export function label(name: string): string {\n  return name;\n}\n"
	judgedLabel   = "export function label(name: string): boolean {\n  return name.length > 3;\n}\n"
)

// @ID-MUT-213
func TestAStrictCountedRunNeedsNoMeasurementOfALanguageWithNothingToJudge(t *testing.T) {
	t.Parallel()
	requireCountedPlatform(t)
	// Given a committed project with a Go module and a TypeScript file with
	// no mutation site, and no TypeScript coverage tool installed.
	moduleRepo(t, unjudgedFiles(unjudgedLabel))

	// When a strict count-one run judges both files.
	o := countedRun(t, "--count", "1", "--workers", "1", "--fail-uncovered", "main.go", "web/label.ts")
	logOutcome(t, &o)
	c := o.counted(t)

	// Then its Go functions are judged for coverage evidence and one Go
	// mutant is judged with a real outcome.
	if s := c.stage("strict-go-inventory"); s == nil || s["state"] != "complete" {
		t.Errorf("stages %v: want strict-go-inventory complete", c.Stages)
	}
	if len(c.Selected) != 1 || c.Selected[0]["file"] != "main.go" || c.Selected[0]["state"] != "judged" || c.Selected[0]["outcome"] != "killed" {
		t.Errorf("exit %d, selected %v, problems %v: want Compare's one mutant judged, killed", o.code, c.Selected, c.Problems)
	}
	// And no stage fails and no TypeScript coverage is measured.
	for _, s := range c.Stages {
		if s["state"] == "failed" {
			t.Errorf("stage %v failed: want none to", s)
		}
	}
	measuredGo := false
	for _, line := range coverageLines(o.stderr) {
		measuredGo = measuredGo || strings.Contains(line, "go test")
		if strings.Contains(line, "web") || strings.Contains(line, "vitest") {
			t.Errorf("TypeScript coverage was measured: %s", line)
		}
	}
	if o.code != 0 || c.problem("count.preparation-failed") != nil || !measuredGo {
		t.Errorf("exit %d, problems %v, coverage commands %v: want 0, no preparation failure, and Go measured", o.code, c.Problems, coverageLines(o.stderr))
	}

	// But when the TypeScript file has an eligible site the run fails with
	// "count.preparation-failed" at its coverage stage, as before.
	moduleRepo(t, unjudgedFiles(judgedLabel))
	judged := countedRun(t, "--count", "1", "--workers", "1", "--fail-uncovered", "main.go", "web/label.ts")
	logOutcome(t, &judged)
	j := judged.counted(t)
	if p := j.problem("count.preparation-failed"); judged.code != 1 || p == nil || p["stage"] != "coverage" {
		t.Errorf("with an eligible TypeScript site: exit %d, problems %v: want 1 with count.preparation-failed at stage coverage", judged.code, j.Problems)
	}
}
