package features

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Listed-test selections that are slow, hang or fail: the baseline's count, the
// timeouts and the problems they give.

func (l *listedMutation) slowProject() error {
	if err := l.project(); err != nil {
		return err
	}
	if err := l.replace("e2e/e2e_test.go", "\t\"testing\"\n)", "\t\"testing\"\n\t\"time\"\n)"); err != nil {
		return err
	}
	return l.replace("e2e/e2e_test.go", "\t\tcmd := exec.Command(bin, f.args...)", "\t\tif f.id == \"ID-A-02\" { time.Sleep(9*time.Second) } else { time.Sleep(700*time.Millisecond) }\n\t\tcmd := exec.Command(bin, f.args...)")
}

func (l *listedMutation) hangProject() error {
	if err := l.replace("main.go", "\t\"os\"\n)", "\t\"os\"\n\t\"time\"\n)"); err != nil {
		return err
	}
	if err := l.replace("main.go", "if n == 1 {\n", "if n == 1 {\n\t\tfor n > 1 {\n\t\t\ttime.Sleep(time.Second)\n\t\t}\n"); err != nil {
		return err
	}
	l.original = l.m.read("main.go")
	return l.declare()
}

func (l *listedMutation) baselineCount(id string, before bool) error {
	lines, err := l.records()
	if err != nil {
		return err
	}
	clean := fmt.Sprintf("%x", sha256.Sum256([]byte(l.original)))
	n, at, mutated := 0, -1, -1
	for i, record := range lines {
		if record["test"] != "run" || record["tests"] != id || record["coverdir"] != "" {
			continue
		}
		if record["source_sha256"] == clean {
			n++
			at = i
		} else if mutated < 0 {
			mutated = i
		}
	}
	if n != 1 || (before && (mutated < 0 || at > mutated)) {
		return fmt.Errorf("%s unmutated runs %d at %d, first mutant %d: %v", id, n, at, mutated, lines)
	}
	if id == "ID-A-02" {
		return l.listedKilled()
	}
	return nil
}

func (l *listedMutation) selectionTimeout() error {
	re := regexp.MustCompile(`baseline of ID-A-01 passed in ([0-9.]+)s: a mutant running it times out after ([0-9.]+)s`)
	match := re.FindStringSubmatch(l.m.w.stderr)
	if match == nil {
		return fmt.Errorf("selection baseline not announced\n%s", l.m.w.report())
	}
	baseline, _ := strconv.ParseFloat(match[1], 64)
	timeout, _ := strconv.ParseFloat(match[2], 64)
	if d := timeout - (3*baseline + 5); d < -0.25 || d > .25 {
		return fmt.Errorf("baseline %.2f timeout %.2f", baseline, timeout)
	}
	re = regexp.MustCompile(`then timeout with ID-A-01 \([0-9.]+s, the listed tests ([0-9.]+)s\)`)
	runs := re.FindAllStringSubmatch(l.m.w.stderr, -1)
	if len(runs) != 2 {
		return fmt.Errorf("%d timed out trials, want two", len(runs))
	}
	for _, run := range runs {
		elapsed, _ := strconv.ParseFloat(run[1], 64)
		if elapsed < timeout-.15 || elapsed > timeout+3.5 || elapsed >= 3*(baseline+9) {
			return fmt.Errorf("trial %.2f timeout %.2f, should use selection baseline", elapsed, timeout)
		}
	}
	return nil
}

func (l *listedMutation) timeoutKilled() error {
	entries, err := l.entries()
	if err != nil {
		return err
	}
	n := 0
	for _, e := range entries {
		if e["outcome"] == "timeout" {
			n++
			if e["scope"] != "listed" || !slices.Equal(entryIDs(e), []string{"ID-A-01"}) {
				return fmt.Errorf("unexpected timeout %v", e)
			}
		}
	}
	summary, ok := summaryOf(l.m.w.stdout, "main.go")
	killed := 0
	for _, e := range entries {
		if e["outcome"] == "killed" || e["outcome"] == "timeout" {
			killed++
		}
	}
	if n != 2 || !ok || summary.killed != killed {
		return fmt.Errorf("%d timeouts, summary %v, want killed count %d", n, summary, killed)
	}
	return nil
}

func (l *listedMutation) failingSelection() error {
	if err := l.project(); err != nil {
		return err
	}
	if err := l.replace("e2e/e2e_test.go", "\tsplit := os.Getenv(\"ITOS_CC_TEST_COVERDIR\")", "\tif os.Getenv(\"GREET_FAIL_ALONE\") != \"\" && *selected == \"ID-A-02\" { t.Fatal(\"selection fails alone\") }\n\tsplit := os.Getenv(\"ITOS_CC_TEST_COVERDIR\")"); err != nil {
		return err
	}
	if err := l.declare(); err != nil {
		return err
	}
	if err := l.record(); err != nil {
		return err
	}
	snap, err := l.snapshot()
	if err != nil {
		return err
	}
	n := 0
	for _, e := range snapshotEntries(snap) {
		if lineOf(e) == listedManyLine {
			e["outcome"] = "survived"
			n++
		}
	}
	if n != 2 {
		return fmt.Errorf("%d listed mutants before retry", n)
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	l.held = string(data) + "\n"
	if err := l.m.w.write(".metrics/mutate/main.go.json", l.held); err != nil {
		return err
	}
	l.m.w.variables = append(l.m.w.variables, "GREET_FAIL_ALONE=1")
	return l.declare()
}

func (l *listedMutation) selectionProblem() error {
	if err := l.m.w.problemIs("tests.selection-failed", "", ""); err != nil {
		return err
	}
	if !slices.Equal(entryIDs(map[string]any{"tests": l.m.w.problem["ids"]}), []string{"ID-A-02"}) {
		return fmt.Errorf("selection problem %v", l.m.w.problem)
	}
	return nil
}

func (l *listedMutation) selectionUnchanged() error {
	if got := l.m.read(".metrics/mutate/main.go.json"); got != l.held {
		return fmt.Errorf("selection failure changed snapshot\n%s", got)
	}
	for _, t := range trialsOf(l.m.w.stderr, "main.go") {
		if t.line == listedManyLine && (strings.Contains(t.outcome, "killed") || strings.Contains(t.outcome, "timeout")) {
			return fmt.Errorf("failed selection decided %v", t)
		}
	}
	return nil
}
