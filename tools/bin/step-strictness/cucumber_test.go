package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func example(line int) caseEvidence {
	c := caseEvidence{URI: "example.feature", ID: "feature;same-name;examples;" + string(rune('0'+line)), Line: line, Name: "Same name", ScenarioID: "ID-DEMO-01"}
	for i, s := range []struct{ keyword, name string }{{"Given ", "ready"}, {"And ", "setup complete"}, {"When ", "run"}, {"Then ", "counts match"}, {"But ", "the reviewed assertion"}, {"And ", "later assertion"}} {
		step := cukeStep{Keyword: s.keyword, Name: s.name, Line: 10 + i}
		step.Result.Status = "passed"
		c.Steps = append(c.Steps, step)
	}
	return c
}
func reviewedProbe() probe {
	return probe{ScenarioID: "ID-DEMO-01", Assertion: assertion{URI: "example.feature", Line: 14, Name: "the reviewed assertion", ErrorToken: "wrong count"}}
}
func failingCase(line int) caseEvidence {
	c := example(line)
	c.Steps[4].Result.Status = "failed"
	c.Steps[4].Result.Error = "wrong count: got 1"
	c.Steps[5].Result.Status = "skipped"
	return c
}
func saveCases(t *testing.T, path string, cases []caseEvidence) {
	t.Helper()
	var features []map[string]any
	for _, c := range cases {
		features = append(features, map[string]any{"uri": c.URI, "elements": []any{map[string]any{"id": c.ID, "name": c.Name, "line": c.Line, "tags": []any{map[string]any{"name": "@" + c.ScenarioID}}, "steps": c.Steps}}})
	}
	b, err := json.Marshal(features)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestClassificationUsesActualExamplesAndAssertionPhase(t *testing.T) {
	base := []caseEvidence{example(2), example(3)}
	for _, tc := range []struct {
		name, want string
		exit       int
		alter      func([]caseEvidence) []caseEvidence
	}{
		{"intended including normal skipped cascade", "detected", 1, func(c []caseEvidence) []caseEvidence { return c }},
		{"case omitted", "invalid", 1, func(c []caseEvidence) []caseEvidence { return c[:1] }},
		{"zero scenarios", "invalid", 1, func([]caseEvidence) []caseEvidence { return nil }},
		{"wrong error at same assertion", "invalid", 1, func(c []caseEvidence) []caseEvidence { c[0].Steps[4].Result.Error = "After hook failed"; return c }},
		{"setup And", "invalid", 1, func(c []caseEvidence) []caseEvidence {
			c[0].Steps[1].Result.Status = "failed"
			c[0].Steps[1].Result.Error = "wrong count"
			return c
		}},
		{"When failed", "invalid", 1, func(c []caseEvidence) []caseEvidence { c[0].Steps[2].Result.Status = "failed"; return c }},
		{"different assertion", "invalid", 1, func(c []caseEvidence) []caseEvidence { c[0].Steps[3].Result.Status = "failed"; return c }},
		{"undefined", "invalid", 1, func(c []caseEvidence) []caseEvidence { c[0].Steps[4].Result.Status = "undefined"; return c }},
		{"ambiguous", "invalid", 1, func(c []caseEvidence) []caseEvidence { c[0].Steps[4].Result.Status = "ambiguous"; return c }},
		{"assertion skipped", "invalid", 1, func(c []caseEvidence) []caseEvidence { c[0].Steps[4].Result.Status = "skipped"; return c }},
		{"case never runs steps", "invalid", 1, func(c []caseEvidence) []caseEvidence { c[0].Steps = nil; return c }},
		{"different example identity", "invalid", 1, func(c []caseEvidence) []caseEvidence { c[0].Line = 99; return c }},
		{"only one example detects", "invalid", 1, func(c []caseEvidence) []caseEvidence { c[0] = example(2); return c }},
		{"compile exit with passing transcript", "invalid", 1, func([]caseEvidence) []caseEvidence { return []caseEvidence{example(2), example(3)} }},
		{"unchanged passing assertion", "undetected", 0, func([]caseEvidence) []caseEvidence { return []caseEvidence{example(2), example(3)} }},
		{"failed transcript with zero exit", "invalid", 0, func(c []caseEvidence) []caseEvidence { return c }},
		{"signal exit despite completed-looking report", "invalid", -1, func(c []caseEvidence) []caseEvidence { return c }},
		{"abnormal exit despite assertion report", "invalid", 2, func(c []caseEvidence) []caseEvidence { return c }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cases := tc.alter([]caseEvidence{failingCase(2), failingCase(3)})
			got, reason, _ := classify(cases, base, reviewedProbe(), tc.exit)
			if got != tc.want {
				t.Fatalf("%s: %s, want %s", got, reason, tc.want)
			}
		})
	}
	p := reviewedProbe()
	p.Assertion.Line = 11
	p.Assertion.Name = "setup complete"
	if _, err := intendedStep(base[0], p.Assertion, false); err == nil {
		t.Fatal("setup And accepted as assertion")
	}
}

func TestCucumberReportRequiresActualInventory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cucumber.json")
	if _, err := readCases(path); err == nil {
		t.Fatal("missing report accepted")
	}
	for _, bad := range []string{"not json", "null", "[]", `[{"uri":"a","uri":"b"}]`} {
		if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := readCases(path); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	saveCases(t, path, []caseEvidence{example(2), example(3)})
	cases, err := readCases(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 || cases[0].Name != cases[1].Name {
		t.Fatalf("lost expanded example identity: %+v", cases)
	}
	if err = controlCases(cases, map[string]bool{"ID-DEMO-01": true, "ID-DEMO-02": true}); err == nil {
		t.Fatal("omitted control selection accepted")
	}
	cases[0].Steps[0].Result.Status = "skipped"
	if err = controlCases(cases, map[string]bool{"ID-DEMO-01": true}); err == nil {
		t.Fatal("skipped control accepted")
	}
}
