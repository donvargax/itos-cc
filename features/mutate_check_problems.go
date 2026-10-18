package features

import (
	"fmt"
	"path/filepath"
	"strings"
)

// The problems mutation check reports, as its scenarios check them.

func checkProblems(obj map[string]any) []map[string]any {
	var out []map[string]any
	problems, _ := obj["problems"].([]any)
	for _, raw := range problems {
		problem, _ := raw.(map[string]any)
		out = append(out, problem)
	}
	return out
}

func (c *checkMutation) wantProblems(obj map[string]any, rule string, names []string, message string) error {
	problems := checkProblems(obj)
	if len(problems) != len(names) {
		return fmt.Errorf("problems %v, want %s for %v", problems, rule, names)
	}
	for i, p := range problems {
		name := names[i] //nolint:gosec // len(problems) == len(names) is checked above
		if p["rule"] != rule || filepath.ToSlash(fmt.Sprint(p["file"])) != c.s.file || !sinceFunction(listedSite{function: fmt.Sprint(p["function"])}, name) || !strings.Contains(fmt.Sprint(p["message"]), message) {
			return fmt.Errorf("problem %v, want %s for %s naming %q", p, rule, name, message)
		}
	}
	return nil
}

func (c *checkMutation) problem(rule, file, function string) error {
	obj, _, err := c.s.m.w.lastObject()
	if err != nil {
		return err
	}
	if file != c.s.file {
		return fmt.Errorf("fixture file %s, sentence says %s", c.s.file, file)
	}
	return c.wantProblems(obj, rule, []string{function}, "")
}

func (c *checkMutation) outcomeProblem(obj map[string]any, rule, function, outcome string, shift int) error {
	if err := c.wantProblems(obj, rule, []string{function}, ""); err != nil {
		return err
	}
	problems := checkProblems(obj)
	entries := snapshotEntries(map[string]any{"units": []any{c.s.units(c.s.before, function)[0]}})
	var expected map[string]any
	for _, entry := range entries {
		if entry["outcome"] == outcome {
			expected = entry
		}
	}
	if expected == nil {
		return fmt.Errorf("no recorded %s mutant", outcome)
	}
	for _, key := range []string{"line", "column", "original", "replacement"} {
		want := expected[key]
		if key == "line" {
			want = want.(float64) + float64(shift)
		}
		if problems[0][key] != want {
			return fmt.Errorf("problem %s=%v, want %v", key, problems[0][key], want)
		}
	}
	return nil
}

func (c *checkMutation) survivorProblem() error {
	obj, _, err := c.s.m.w.lastObject()
	if err != nil {
		return err
	}
	if err := c.outcomeProblem(obj, "mutation.survived", c.s.place, "survived", 0); err != nil {
		return err
	}
	// The old holder also guards the new location of an unchanged survivor.
	if err := c.s.m.w.write(c.s.file, "// moved down\n"+c.s.m.read(c.s.file)); err != nil {
		return err
	}
	obj, _, err = c.captureCheck(1)
	if err != nil {
		return err
	}
	return c.outcomeProblem(obj, "mutation.survived", c.s.place, "survived", 1)
}

func (c *checkMutation) uncoveredProblem() error {
	obj, _, err := c.captureCheck(1, "--fail-uncovered")
	if err != nil {
		return err
	}
	// Strict TypeScript obligations may add coverage-inventory problems too;
	// this sentence holds the recorded uncovered mutant and its exact subject.
	var uncovered []any
	for _, p := range checkProblems(obj) {
		if p["rule"] == "mutation.uncovered" {
			uncovered = append(uncovered, p)
		}
	}
	obj["problems"] = uncovered
	return c.outcomeProblem(obj, "mutation.uncovered", c.s.clear, "uncovered", 0)
}

func (c *checkMutation) missingProblem() error {
	obj, _, err := c.s.m.w.lastObject()
	if err != nil {
		return err
	}
	if err := c.wantProblems(obj, "mutation.stale", []string{c.s.place}, ""); err != nil {
		return err
	}
	want := fmt.Sprintf("%v:%v `%v` → `%v`", c.missing["line"], c.missing["column"], c.missing["original"], c.missing["replacement"])
	if !strings.Contains(fmt.Sprint(checkProblems(obj)[0]["message"]), want) {
		return fmt.Errorf("stale problem does not name missing site %s: %v", want, obj)
	}
	return nil
}
