package features

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

func (q *strictMutation) changedSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a (go|python) project whose function has an executable line no test runs, committed at a base$`, q.changedProject)
	sc.Step(`^a commit since the base that changes another line of that function, which a test runs$`, q.changedCommit)
	sc.Step(`^the old unexecuted line is not reported and the run passes$`, func() error {
		a, err := q.object()
		if err != nil {
			return err
		}
		if len(strictChangedFacts(a)) != 0 {
			return fmt.Errorf("old line reported: %v", a)
		}
		return q.m.w.exitsWith(0)
	})
	sc.Step(`^"itos-cc mutation check --since <base> --fail-uncovered=lines" gives the same verdict$`, func() error {
		a, _, code, err := q.capture("mutation check --since base --fail-uncovered=lines --json " + q.file)
		if err != nil {
			return err
		}
		if code != 0 || len(strictChangedFacts(a)) != 0 {
			return fmt.Errorf("changed-line cached check exit%d: %v", code, a)
		}
		return nil
	})
	sc.Step(`^"itos-cc mutation run --since <base> --fail-uncovered" still reports the old line as "mutation.uncovered-statement"$`, func() error {
		a, _, code, err := q.capture("mutation run --since base --fail-uncovered --json " + q.file)
		if err != nil {
			return err
		}
		if code != 1 {
			return fmt.Errorf("whole-function strict run exit%d: %v", code, a)
		}
		return q.finding(a, "mutation.uncovered-statement", q.function, q.changed.oldLine)
	})
	sc.Step(`^a commit that adds a line no test runs is reported on that line, and the run fails$`, func() error {
		e := q.changed
		if err := q.s.replace(q.file, e.added[0], e.added[1]); err != nil {
			return err
		}
		if err := q.s.commit("add uncovered changed line"); err != nil {
			return err
		}
		if err := q.changedDeclarations(true); err != nil {
			return err
		}
		a, _, code, err := q.capture("mutation run --since base --fail-uncovered=lines --json " + q.file)
		if err != nil {
			return err
		}
		want := []string{fmt.Sprintf("mutation.uncovered-statement %s:%d", q.file, e.addedLine)}
		if code != 1 || !slices.Equal(strictChangedFacts(a), want) {
			return fmt.Errorf("new uncovered changed line exit%d findings%v want%v", code, strictChangedFacts(a), want)
		}
		return nil
	})
	sc.Step(`^I run "itos-cc mutation run --fail-uncovered=lines" without --since$`, func() error { return q.run("mutation run --fail-uncovered=lines --json") })
	sc.Step(`^it fails as a usage error, exit 2, naming --since$`, func() error {
		a, err := q.object()
		if err != nil {
			return err
		}
		if err := q.m.w.exitsWith(2); err != nil {
			return err
		}
		for _, p := range checkProblems(a) {
			if strings.Contains(fmt.Sprint(p["message"])+fmt.Sprint(p["fix"]), "--since") {
				return nil
			}
		}
		return fmt.Errorf("usage failure does not name --since: %v", a)
	})
	sc.Step(`^a counted Go run with --count 1 --since <base> --fail-uncovered=lines judges coverage only on the changed lines, as a complete run does$`, q.countedChanged)
}
func strictChangedFacts(a map[string]any) []string {
	var out []string
	for _, p := range checkProblems(a) {
		if slices.Contains([]any{"mutation.uncovered-statement", "mutation.coverage-missing", "mutation.coverage-stale", "mutation.uncovered"}, p["rule"]) {
			out = append(out, fmt.Sprintf("%v %s:%v", p["rule"], filepath.ToSlash(fmt.Sprint(p["file"])), p["line"]))
		}
	}
	slices.Sort(out)
	return out
}
func (q *strictMutation) changedProject(language string) error {
	q.language = language
	q.namespace = "example.com/lines"
	if language == "python" {
		q.namespace = "calc"
	}
	e := &changedLinesExamples[0]
	if language == "python" {
		e = &changedLinesExamples[1]
		q.m.w.variables = append(q.m.w.variables, "PYTEST_DISABLE_PLUGIN_AUTOLOAD=1")
	}
	q.changed = e
	q.file = e.file
	q.function = "Value"
	if language == "python" {
		q.function = "value"
	}
	for file, text := range e.files {
		if err := q.m.w.write(file, text); err != nil {
			return err
		}
		if strings.Contains(file, "test") {
			q.testFile = file
		}
	}
	// The test imports the judged file.
	q.m.w.imports(map[string][]string{q.testFile: {q.file}})
	if err := q.changedDeclarations(false); err != nil {
		return err
	}
	if err := q.m.w.git("init", "-q", "-b", "main"); err != nil {
		return err
	}
	if err := q.s.commit("base"); err != nil {
		return err
	}
	return q.m.w.git("tag", "base")
}
func (q *strictMutation) changedDeclarations(added bool) error {
	c := Coverage{Language: q.language}
	t := Tests{Language: q.language}
	if q.language == "go" {
		t.Command = "go test -count=1 -failfast ./."
		c.Report = "mode: set\nexample.com/lines/value.go:5.2,6.10 2 1\nexample.com/lines/value.go:7.3,8.1 1 0\nexample.com/lines/value.go:9.2,9.10 1 1\n"
		if added {
			c.Report = "mode: set\nexample.com/lines/value.go:5.2,6.10 2 1\nexample.com/lines/value.go:7.3,8.1 1 0\nexample.com/lines/value.go:9.2,9.10 1 1\nexample.com/lines/value.go:10.3,11.1 1 0\nexample.com/lines/value.go:12.2,12.10 1 1\n"
		}
	} else {
		t.Command = "python3 -m pytest -q -x -p no:cacheprovider tests/test_calc.py"
		c.Report = "SF:calc.py\nDA:1,1\nDA:2,1\nDA:3,1\nDA:4,0\nDA:5,1\nend_of_record\n"
		if added {
			c.Report = "SF:calc.py\nDA:1,1\nDA:2,1\nDA:3,1\nDA:4,0\nDA:5,1\nDA:6,1\nDA:7,0\nDA:8,1\nend_of_record\n"
		}
	}
	if err := q.m.w.d.Coverage(q.m.w.dir, c); err != nil {
		return err
	}
	if err := q.m.w.d.Tests(q.m.w.dir, t); err != nil {
		return err
	}
	if q.language == "python" {
		return q.m.pythonProbe(true)
	}
	return nil
}
func (q *strictMutation) changedCommit() error {
	e := q.changed
	if err := q.s.replace(q.file, e.changed[0], e.changed[1]); err != nil {
		return err
	}
	if err := q.s.replace(q.testFile, e.changedTest[0], e.changedTest[1]); err != nil {
		return err
	}
	return q.s.commit("change the executed default")
}
func (q *strictMutation) countedChanged() error {
	if err := q.m.w.d.Counted(); err != nil {
		return err
	}
	if err := q.changedProject("go"); err != nil {
		return err
	}
	if err := q.changedCommit(); err != nil {
		return err
	}
	a, log, code, err := q.capture("mutation run --count 1 --seed 4813e48 --workers 1 --since base --fail-uncovered=lines --json " + q.file)
	if err != nil {
		return err
	}
	selected := objectList(a, "selected")
	if code != 0 || len(selected) != 1 || selected[0]["state"] != "uncovered" || len(strictChangedFacts(a)) != 0 {
		return fmt.Errorf("counted changed-line selection exit%d: %v\n%s", code, a, log)
	}
	full, _, fullCode, err := q.capture("mutation run --since base --fail-uncovered=lines --json " + q.file)
	if err != nil {
		return err
	}
	if fullCode != code || !slices.Equal(strictChangedFacts(full), strictChangedFacts(a)) {
		return fmt.Errorf("counted vs complete changed-line verdict: %v vs%v", a, full)
	}
	// The whole-function control proves the sampled old uncovered site exists;
	// changed-line success cannot arise from an empty counted selection.
	whole, _, code, err := q.capture("mutation run --count 1 --seed 4813e48 --workers 1 --since base --fail-uncovered --json " + q.file)
	if err != nil {
		return err
	}
	want := []string{"mutation.uncovered value.go:7", "mutation.uncovered-statement value.go:7"}
	if code != 1 || !slices.Equal(strictChangedFacts(whole), want) {
		return fmt.Errorf("counted whole-function control exit%d: %v", code, whole)
	}
	return nil
}
