package features

import (
	"fmt"
	"slices"
	"strings"
)

// How listed tests measure coverage, and the order their runs take, the own
// tests first.

func (l *listedMutation) inventory() error {
	snap, err := l.snapshot()
	if err != nil {
		return err
	}
	got, _ := snap["listed"].(map[string]any)
	if len(got) != 2 || got["ID-A-01"] != "a.feature" || got["ID-A-02"] != "" {
		return fmt.Errorf("listed inventory %v", got)
	}
	return nil
}

func (l *listedMutation) coverageRuns() []string {
	var out []string
	for _, line := range outputLines(l.m.w.stderr) {
		if strings.HasPrefix(line, "itos-cc: coverage ") && strings.Contains(line, "-tests=") {
			out = append(out, line)
		}
	}
	return out
}

func (l *listedMutation) splitCoverage() error {
	runs := l.coverageRuns()
	if len(runs) != 1 || !strings.HasSuffix(runs[0], "-tests=ID-A-01,ID-A-02") {
		return fmt.Errorf("coverage runs %q, want one selecting both", runs)
	}
	return nil
}

func (l *listedMutation) fallbackCoverage() error {
	runs := l.coverageRuns()
	for _, id := range []string{"ID-A-01", "ID-A-02"} {
		n := 0
		for _, r := range runs {
			if strings.HasSuffix(r, "-tests="+id) {
				n++
			}
		}
		if n != 1 {
			return fmt.Errorf("coverage runs %q, want one selecting %s alone", runs, id)
		}
	}
	return nil
}

func (l *listedMutation) coverageIDs() error {
	entries, err := l.entries()
	if err != nil {
		return err
	}
	for line, ids := range listedLineTests {
		if err := l.assertLines(entries, line, func(e map[string]any) bool { return e["scope"] == "listed" && slices.Equal(entryIDs(e), ids) }); err != nil {
			return err
		}
	}
	return nil
}

func (l *listedMutation) ownFirst() error {
	entries, err := l.entries()
	if err != nil {
		return err
	}
	if err := l.assertLines(entries, listedHalfLine, func(e map[string]any) bool { return e["outcome"] == "killed" && e["scope"] == nil && e["tests"] == nil }); err != nil {
		return err
	}
	for _, t := range trialsOf(l.m.w.stderr, "main.go") {
		if t.line == listedHalfLine && strings.Contains(t.outcome, "ID-A-") {
			return fmt.Errorf("own killed mutant ran listed tests: %v", t)
		}
	}
	return nil
}

func (l *listedMutation) listedKilled() error {
	if err := l.snapshotListed(); err != nil {
		return err
	}
	return l.ranAfterOwn(listedManyLine, "ID-A-02", "killed")
}

func (l *listedMutation) ranAfterOwn(line int, id, outcome string) error {
	n := 0
	for _, t := range trialsOf(l.m.w.stderr, "main.go") {
		if t.line == line {
			n++
			if t.outcome != "survived its own tests, then "+outcome+" with "+id {
				return fmt.Errorf("trial %v did not run own then %s", t, id)
			}
		}
	}
	if n != 2 {
		return fmt.Errorf("%d trials on line %d, want two\n%s", n, line, l.m.w.report())
	}
	return nil
}

func (l *listedMutation) firstCovered() error {
	entries, err := l.entries()
	if err != nil {
		return err
	}
	if err := l.assertLines(entries, listedOneLine, func(e map[string]any) bool { return e["outcome"] != "uncovered" }); err != nil {
		return err
	}
	if !slices.Contains(trials(l.m.w.stderr, "main.go"), listedOneLine) {
		return fmt.Errorf("covered mutants never ran")
	}
	return nil
}

func (l *listedMutation) onlyTwiceUncovered() error {
	entries, err := l.entries()
	if err != nil {
		return err
	}
	n := 0
	for _, e := range entries {
		if (lineOf(e) == listedTwiceLine) != (e["outcome"] == "uncovered") {
			return fmt.Errorf("unexpected uncovered mutant %v", e)
		}
		if e["outcome"] == "uncovered" {
			n++
		}
	}
	if n != 1 {
		return fmt.Errorf("%d uncovered, want one", n)
	}
	return l.m.w.exitsWith(1)
}
