package features

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

func (q *strictMutation) lineSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a Python project whose judged file has a function with no mutation site that no test executes, and a reached function with one line no test executes$`, func() error { return q.lineProject("python") })
	sc.Step(`^a TypeScript project tested with Vitest whose judged file has a function with no mutation site that no test executes, and a reached function with one line no test executes$`, func() error { return q.lineProject("typescript") })
	sc.Step(`^a Kotlin project built with Gradle and JaCoCo whose judged file has a function with no mutation site that no test executes, a reached function with one line no test executes, and a line a test executes only one branch of$`, func() error { return q.lineProject("kotlin") })
	sc.Step(`^each unexecuted executable line is reported as "mutation.uncovered-statement" with its file, function and line,(?: and the partly executed line is not,)? the mutant counts are unchanged, and the run fails$`, q.strictLines)
	sc.Step(`^"itos-cc mutation check --fail-uncovered" reports the same findings without running a test$`, func() error {
		if err := q.observed(); err != nil {
			return err
		}
		a, log, code, err := q.capture("mutation check --fail-uncovered --json " + q.paths())
		if err != nil {
			return err
		}
		if code != 1 || !slices.Equal(strictLineFacts(a), q.expected) {
			return fmt.Errorf("cached strict line findings exit%d: %v want%v", code, strictLineFacts(a), q.expected)
		}
		return q.quiet(log)
	})
	sc.Step(`^after a test that reaches the file changes, the check reports "mutation.coverage-stale" until a new run measures it again$`, q.staleLines)
	sc.Step(`^a strict (Python|TypeScript|Kotlin) run refuses --coverage-report, --use-existing-coverage and --coverage-command$`, func(_ string) error { return q.reportFlags() })
	sc.Step(`^without --fail-uncovered the run and the check report as before$`, func() error {
		for _, cmd := range []string{"run", "check"} {
			a, _, code, err := q.capture("mutation " + cmd + " --json " + q.paths())
			if err != nil {
				return err
			}
			before := q.warm
			if cmd == "check" {
				before = q.warmCheck
			}
			wantCode := 0
			if before["ok"] != true {
				wantCode = 1
			}
			if code != wantCode || len(strictLineFacts(a)) != 0 || !reflect.DeepEqual(strictCounts(a), strictCounts(before)) || !reflect.DeepEqual(strictProblemRecords(a), strictProblemRecords(before)) || !slices.Equal(strictOutcomeRecords(a), strictOutcomeRecords(before)) {
				return fmt.Errorf("non-strict %s changed exit%d: %v vs%v", cmd, code, a, before)
			}
			if cmd == "run" {
				if err := strictExecutionCounts(a); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
func (q *strictMutation) lineProject(language string) error {
	q.language = language
	files := strictPythonFiles
	c := Coverage{Language: language}
	t := Tests{Language: language}
	switch language {
	case "python":
		q.file = "calc.py"
		q.selected = []string{q.file, "lonely.py"}
		q.testFile = "tests/test_calc.py"
		q.expected = slices.Clone(strictPythonLines)
		q.m.w.variables = append(q.m.w.variables, "PYTEST_DISABLE_PLUGIN_AUTOLOAD=1")
		c.Report = "SF:calc.py\nDA:1,1\nDA:2,1\nDA:3,0\nDA:4,1\nDA:7,1\nDA:8,0\nDA:9,0\nend_of_record\nSF:lonely.py\nDA:1,0\nDA:2,0\nend_of_record\n"
		t.Command = "python3 -m pytest -q -x -p no:cacheprovider tests/test_calc.py"
	case "typescript":
		files = strictTypeScriptFiles
		q.file = "src/calc.ts"
		q.selected = []string{q.file, "src/lonely.ts"}
		q.testFile = "src/calc.test.ts"
		q.expected = slices.Clone(strictTypeScriptLines)
		c.Report = "SF:src/calc.ts\nDA:2,1\nDA:3,0\nDA:5,1\nDA:9,0\nDA:10,0\nend_of_record\nSF:src/lonely.ts\nDA:2,0\nend_of_record\n"
		t.Command = vitestCommand + q.file
	case "kotlin":
		files = strictKotlinFiles
		q.file = "src/main/kotlin/calc/Calc.kt"
		q.selected = []string{q.file, "src/main/kotlin/other/Lonely.kt"}
		q.testFile = "src/test/kotlin/calc/CalcTest.kt"
		q.expected = slices.Clone(strictKotlinLines)
		for file, text := range languageKotlinBuilds["gradle"] {
			if err := q.m.w.write(file, text); err != nil {
				return err
			}
		}
		c.Program = "gradle"
		c.Report = `<report><package name="calc"><sourcefile name="Calc.kt"><line nr="3" mi="1" ci="0"/><line nr="4" mi="0" ci="1"/><line nr="5" mi="1" ci="0"/><line nr="7" mi="1" ci="1"/><line nr="11" mi="1" ci="0"/><line nr="12" mi="1" ci="0"/></sourcefile></package><package name="other"><sourcefile name="Lonely.kt"><line nr="4" mi="1" ci="0"/></sourcefile></package></report>`
		t.Command = "gradle -p . test --fail-fast --tests calc.CalcTest"
		t.Surviving = []Site{{File: q.file, Line: 7, Original: "<", Replacement: "<="}}
	default:
		return fmt.Errorf("unknown strict language %s", language)
	}
	for file, text := range files {
		if err := q.m.w.write(file, text); err != nil {
			return err
		}
	}
	if err := q.m.w.d.Coverage(q.m.w.dir, c); err != nil {
		return err
	}
	if err := q.m.w.d.Tests(q.m.w.dir, t); err != nil {
		return err
	}
	a, _, code, err := q.capture("mutation run --json " + q.paths())
	if err != nil {
		return err
	}
	if code > 1 {
		return fmt.Errorf("plain %s setup exit%d: %v", language, code, a)
	}
	q.plain = a
	q.plainCheck, _, _, err = q.capture("mutation check --json " + q.paths())
	if err != nil {
		return err
	}
	// Observe the cache path independently: fully reused runs can legitimately
	// omit newly measured coverage labels. Never rewrite the cache to model it.
	q.warm, _, code, err = q.capture("mutation run --json " + q.paths())
	if err != nil {
		return err
	}
	wantCode := 0
	if q.plain["ok"] != true {
		wantCode = 1
	}
	if code != wantCode || !reflect.DeepEqual(strictCounts(q.warm), strictCounts(q.plain)) {
		return fmt.Errorf("warm plain control changed totals exit%d: %v vs%v", code, q.warm, q.plain)
	}
	q.warmCheck, _, _, err = q.capture("mutation check --json " + q.paths())
	return err
}
func strictLineFacts(a map[string]any) []string {
	var out []string
	for _, p := range checkProblems(a) {
		if slices.Contains([]any{"mutation.uncovered-statement", "mutation.coverage-missing", "mutation.coverage-stale", "mutation.coverage-unsupported"}, p["rule"]) {
			out = append(out, fmt.Sprintf("%v %s %v:%v", p["rule"], filepath.ToSlash(fmt.Sprint(p["file"])), p["function"], p["line"]))
		}
	}
	slices.Sort(out)
	return out
}
func strictCounts(a map[string]any) map[string][3]any {
	out := map[string][3]any{}
	for _, f := range objectList(a, "files") {
		if f["killed"] != nil {
			out[filepath.ToSlash(fmt.Sprint(f["file"]))] = [3]any{f["killed"], f["survived"], f["uncovered"]}
		}
	}
	return out
}
func (q *strictMutation) strictLines() error {
	a, err := q.object()
	if err != nil {
		return err
	}
	if !slices.Equal(strictLineFacts(a), q.expected) || !reflect.DeepEqual(strictCounts(a), strictCounts(q.plain)) {
		return fmt.Errorf("strict findings/counts %v %v, want %v %v", strictLineFacts(a), strictCounts(a), q.expected, strictCounts(q.plain))
	}
	for _, p := range q.findings(a, "mutation.uncovered-statement", "") {
		if p["original"] != nil || p["replacement"] != nil {
			return fmt.Errorf("statement invents mutant: %v", p)
		}
	}
	return q.m.w.exitsWith(1)
}
func (q *strictMutation) staleLines() error {
	text := q.m.read(q.testFile)
	comment := "\n// reaching test changed\n"
	if q.language == "python" {
		comment = "\n# reaching test changed\n"
	}
	if err := q.m.w.write(q.testFile, text+comment); err != nil {
		return err
	}
	a, log, code, err := q.capture("mutation check --fail-uncovered --json " + q.file)
	if err != nil {
		return err
	}
	ps := q.findings(a, "mutation.coverage-stale", "")
	if code != 1 || len(ps) != 2 {
		return fmt.Errorf("stale strict evidence exit%d: %v\n%s", code, a, log)
	}
	for _, p := range ps {
		if !strings.Contains(filepath.ToSlash(fmt.Sprint(p["message"])), q.testFile) {
			return fmt.Errorf("stale message does not name reaching test: %v", p)
		}
	}
	if _, err := q.fresh("mutation run --fail-uncovered --json "+q.paths(), 1); err != nil {
		return err
	}
	a, _, code, err = q.capture("mutation check --fail-uncovered --json " + q.paths())
	if err != nil {
		return err
	}
	if code != 1 || !slices.Equal(strictLineFacts(a), q.expected) {
		return fmt.Errorf("remeasured strict line findings: %v", a)
	}
	return nil
}
func (q *strictMutation) reportFlags() error {
	q.flags = nil
	for _, flag := range []string{"--coverage-report missing.lcov", "--use-existing-coverage", "--coverage-command 'go version'"} {
		a, log, code, err := q.capture("mutation run --fail-uncovered --json " + flag + " " + q.file)
		if err != nil {
			return err
		}
		ps := q.findings(a, "flags.conflict", "")
		name := strings.Fields(flag)[0]
		q.flags = append(q.flags, strictFlagObservation{flag: name, code: code, object: a, log: log})
		if code != 2 || len(ps) != 1 || ps[0]["flag"] != name {
			return fmt.Errorf("strict report flag%s exit%d: %v\n%s", flag, code, a, log)
		}
	}
	return nil
}

func strictProblemRecords(a map[string]any) []string {
	var out []string
	for _, p := range checkProblems(a) {
		b, _ := json.Marshal(p)
		out = append(out, string(b))
	}
	slices.Sort(out)
	return out
}
func strictOutcomeRecords(a map[string]any) []string {
	var out []string
	for _, f := range objectList(a, "files") {
		for _, m := range objectList(f, "mutants") {
			record := map[string]any{"file": f["file"]}
			for k, v := range m {
				if k != "reused" {
					record[k] = v
				}
			}
			b, _ := json.Marshal(record)
			out = append(out, string(b))
		}
	}
	slices.Sort(out)
	return out
}
func strictExecutionCounts(a map[string]any) error {
	for _, f := range objectList(a, "files") {
		ran, reused := 0, 0
		for _, m := range objectList(f, "mutants") {
			if m["reused"] == true {
				reused++
			} else if m["outcome"] != "uncovered" {
				ran++
			}
		}
		baseline := "not-run"
		if ran > 0 {
			baseline = "passed"
		}
		if f["baseline"] != baseline {
			return fmt.Errorf("execution baseline %v want%s: %v", f["baseline"], baseline, f)
		}
		if f["ran"] != float64(ran) || f["reused"] != float64(reused) {
			return fmt.Errorf("execution bookkeeping ran%d/reused%d: %v", ran, reused, f)
		}
	}
	return nil
}
