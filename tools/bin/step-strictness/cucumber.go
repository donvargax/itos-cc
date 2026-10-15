package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// These are the formatter's public JSON fields, not copies of Godog's models.
// Keep the original report alongside this evidence, including fields we do not use.
type cukeFeature struct {
	URI      string     `json:"uri"`
	Elements []cukeCase `json:"elements"`
}
type cukeCase struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Line int    `json:"line"`
	Tags []struct {
		Name string `json:"name"`
	} `json:"tags"`
	Steps []cukeStep `json:"steps"`
}
type cukeStep struct {
	Keyword string `json:"keyword"`
	Name    string `json:"name"`
	Line    int    `json:"line"`
	Result  struct {
		Status string `json:"status"`
		Error  string `json:"error_message"`
	} `json:"result"`
}
type caseEvidence struct {
	URI        string     `json:"uri"`
	ID         string     `json:"id"`
	Line       int        `json:"line"`
	Name       string     `json:"name"`
	ScenarioID string     `json:"scenario_id"`
	Steps      []cukeStep `json:"steps"`
}

func (c caseEvidence) key() string { return fmt.Sprintf("%s:%s:%d", c.URI, c.ID, c.Line) }

func readCases(path string) ([]caseEvidence, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err = checkJSON(b); err != nil {
		return nil, fmt.Errorf("invalid cucumber report: %w", err)
	}
	var features []cukeFeature
	if err = json.Unmarshal(b, &features); err != nil {
		return nil, fmt.Errorf("invalid cucumber report: %w", err)
	}
	var cases []caseEvidence
	seen := map[string]bool{}
	for _, f := range features {
		for _, c := range f.Elements {
			if f.URI == "" || c.ID == "" || c.Line < 1 || len(c.Steps) == 0 {
				return nil, fmt.Errorf("report has incomplete case identity or no steps: %s %s", f.URI, c.ID)
			}
			id := ""
			for _, tag := range c.Tags {
				if strings.HasPrefix(tag.Name, "@ID-") {
					if id != "" {
						return nil, fmt.Errorf("multiple scenario IDs in %s", c.ID)
					}
					id = strings.TrimPrefix(tag.Name, "@")
				}
			}
			if id == "" {
				return nil, fmt.Errorf("missing scenario ID: %s", c.ID)
			}
			e := caseEvidence{URI: f.URI, ID: c.ID, Line: c.Line, Name: c.Name, ScenarioID: id, Steps: c.Steps}
			if seen[e.key()] {
				return nil, fmt.Errorf("duplicate case identity %s", e.key())
			}
			seen[e.key()] = true
			for _, s := range c.Steps {
				if s.Name == "" || s.Line < 1 {
					return nil, fmt.Errorf("invalid step identity in %s", c.ID)
				}
			}
			cases = append(cases, e)
		}
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("report ran zero scenarios")
	}
	return cases, nil
}

func phase(keyword, previous string) string {
	switch strings.TrimSpace(keyword) {
	case "Given", "When", "Then":
		return strings.TrimSpace(keyword)
	case "And", "But":
		return previous
	default:
		return ""
	}
}

func controlCases(cases []caseEvidence, ids map[string]bool) error {
	seen := map[string]bool{}
	for _, c := range cases {
		if !ids[c.ScenarioID] {
			return fmt.Errorf("control executed unexpected %s", c.ScenarioID)
		}
		seen[c.ScenarioID] = true
		for _, s := range c.Steps {
			if s.Result.Status != "passed" || s.Result.Error != "" {
				return fmt.Errorf("control %s step %d: %s %s", c.key(), s.Line, s.Result.Status, s.Result.Error)
			}
		}
	}
	for id := range ids {
		if !seen[id] {
			return fmt.Errorf("control omitted %s", id)
		}
	}
	return nil
}

func intendedStep(c caseEvidence, a assertion, edited bool) (int, error) {
	current := ""
	found := -1
	for i, s := range c.Steps {
		current = phase(s.Keyword, current)
		if c.URI == a.URI && s.Line == a.Line && (!edited || s.Name == a.Name) {
			if current != "Then" {
				return -1, fmt.Errorf("intended step is %s phase, not an assertion", current)
			}
			if found >= 0 {
				return -1, fmt.Errorf("ambiguous intended assertion")
			}
			found = i
		}
	}
	if found < 0 {
		return -1, fmt.Errorf("intended assertion missing in %s", c.key())
	}
	return found, nil
}

// Every expanded example comes from the actual green control. Skips after the
// expected failure are normal; skips before it, omitted cases and other failures
// are not evidence that the reviewed assertion noticed the edit.
func classify(cases, baseline []caseEvidence, p probe, exit int) (string, string, int) {
	want := map[string]caseEvidence{}
	for _, c := range baseline {
		if c.ScenarioID == p.ScenarioID {
			want[c.key()] = c
		}
	}
	if len(want) == 0 {
		return "invalid", "control has no selected cases", 0
	}
	if len(cases) != len(want) {
		return "invalid", "case inventory differs from green control", 0
	}
	failures, passes := 0, 0
	for _, c := range cases {
		b, ok := want[c.key()]
		if !ok || c.ScenarioID != p.ScenarioID {
			return "invalid", "unexpected case identity", 0
		}
		delete(want, c.key())
		index, err := intendedStep(b, p.Assertion, false)
		if err != nil {
			return "invalid", err.Error(), 0
		}
		actual, err := intendedStep(c, p.Assertion, true)
		if err != nil || actual != index {
			return "invalid", "intended assertion identity changed or disappeared", 0
		}
		if len(c.Steps) != len(b.Steps) {
			return "invalid", "step inventory differs from green control", 0
		}
		failed := false
		for i, s := range c.Steps {
			// A probe may change a feature literal, so only the reviewed assertion
			// name may differ from control; all other step identities stay fixed.
			if s.Line != b.Steps[i].Line || (i != index && s.Name != b.Steps[i].Name) || strings.TrimSpace(s.Keyword) != strings.TrimSpace(b.Steps[i].Keyword) {
				return "invalid", "unexpected step identity", 0
			}
			if i == index && s.Result.Status == "failed" && strings.Contains(s.Result.Error, p.Assertion.ErrorToken) {
				failed = true
				failures++
				continue
			}
			if failed && i > index && s.Result.Status == "skipped" && s.Result.Error == "" {
				continue
			}
			if s.Result.Status != "passed" || s.Result.Error != "" {
				return "invalid", fmt.Sprintf("unexpected %s at %s:%d: %s", s.Result.Status, c.URI, s.Line, s.Result.Error), 0
			}
		}
		if !failed {
			passes++
		}
	}
	if failures > 0 && passes > 0 {
		return "invalid", "some expanded cases did not fail at the assertion", failures
	}
	if failures > 0 && exit == 1 {
		return "detected", "all control examples reached the intended assertion and failed", failures
	}
	if failures == 0 && exit == 0 {
		return "undetected", "edited assertion still passed", 0
	}
	return "invalid", "process exit disagrees with structured assertion results", failures
}
