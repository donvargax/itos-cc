package features

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/cucumber/godog"
)

// The steps of the Rule "A site on a later line of a statement takes its
// statement's coverage where its own line is unmeasured" (slice-79, idea
// mutation-continuation-line-reach). coverage.py's LCOV and Vitest's v8
// report give a DA line only to the line a statement starts on, so each
// Given declares, for the in-process driver's fake measurer, the report
// the real tools write: a DA for a split statement's first line alone,
// and one for an arrow function's body on a later line, which Vitest
// measures by itself. The binary driver runs the real tools.

const (
	// continuationTwoLines is demo.py: the `*` → `/` site of two_lines is
	// on line 3, the second line of the return statement line 2 starts.
	continuationTwoLines = "def two_lines(a, b):\n    return (a\n            * b)\n"
	// continuationTwoLinesTest calls two_lines without checking what it
	// returns, so its mutant survives it.
	continuationTwoLinesTest = "from demo import two_lines\n\n\ndef test_calls_without_checking():\n    two_lines(2, 3)\n"
	// continuationScenario is scn1, the one listed test: listing with
	// "list", and otherwise checking two_lines(2, 3) is 6 in a Python
	// process it starts, as an end-to-end test drives a program.
	continuationScenario = "import subprocess\nimport sys\n\nif sys.argv[1:] == [\"list\"]:\n    print(\"scn1\\tscenario.py\")\n    sys.exit(0)\n" +
		"code = \"import sys, demo; sys.exit(0 if demo.two_lines(2, 3) == 6 else 1)\"\n" +
		"sys.exit(subprocess.run([sys.executable, \"-c\", code]).returncode)\n"
	// continuationListed lists scn1 and runs it through scenario.py.
	continuationListed = "mutation:\n  tests:\n    list: python3 scenario.py list\n    run: python3 scenario.py {pattern}\n    ids_pattern: \"{ids}\"\n    join:\n      each: \"{id}\"\n      sep: \",\"\n"

	// continuationCalcPy and continuationCalcTS are calc.py and
	// src/calc.ts: add's `+` on line 3, the second line of the return
	// statement line 2 starts, which the test calls; never's on line 8,
	// the second line of the one line 7 starts, which nothing calls.
	continuationCalcPy     = "def add(a, b):\n    return (a\n            + b)\n\n\ndef never(a, b):\n    return (a\n            + b)\n"
	continuationCalcPyTest = "from calc import add\n\n\ndef test_add():\n    assert add(1, 2) == 3\n"
	continuationCalcTS     = "export function add(a: number, b: number): number {\n  return (a\n    + b);\n}\n\nexport function never(a: number, b: number): number {\n  return (a\n    + b);\n}\n"
	continuationCalcTSTest = "import { test, expect } from \"vitest\";\nimport { add } from \"./calc\";\n\ntest(\"add\", () => {\n  expect(add(1, 2)).toBe(3);\n});\n"

	// continuationIncTS is src/inc.ts: the arrow function's body, x + 1,
	// on line 3, which its test never runs, mapping an empty list.
	continuationIncTS     = "export function inc(xs: number[]): number[] {\n  return xs.map((x) =>\n    x + 1);\n}\n"
	continuationIncTSTest = "import { test, expect } from \"vitest\";\nimport { inc } from \"./inc\";\n\ntest(\"inc\", () => {\n  expect(inc([])).toEqual([]);\n});\n"

	continuationPyproject = "[project]\nname = \"fixture\"\nversion = \"0.0.0\"\n"
)

// continuationLine is the file the Given wrote, slash-separated from the
// scratch folder.
type continuationLine struct {
	m    *mutateState
	file string
}

func continuationLineSteps(sc *godog.ScenarioContext, m *mutateState) {
	c := &continuationLine{m: m}
	sc.Step(`^a Python project whose two_lines returns "\(a" and then "\* b\)" on the next line, and whose own test calls two_lines\(2, 3\) without checking what it returns$`, c.twoLines)
	sc.Step(`^itos-cc\.yaml lists one test, scn1, a script that fails unless two_lines\(2, 3\) is 6$`, c.listed)
	sc.Step("^the `\\*` → `/` mutant on two_lines' second line survives its own tests, then runs the run command selecting scn1 alone, and is killed$", c.killedByScn1)
	sc.Step(`^its snapshot entry records scope "listed" and tests \["scn1"\]$`, c.snapshotListed)
	sc.Step(`^a (Python|TypeScript, tested with Vitest) file (calc\.py|src/calc\.ts) whose add returns "\(a" and then "\+ b" on the next line, which a test calls, and whose never returns the same over lines 7 and 8, which nothing calls$`, c.calc)
	sc.Step(`^the Python file calc\.py of ID-MUT-425$`, func() error { return c.calc("Python", "calc.py") })
	sc.Step(`^a TypeScript file src/inc\.ts, tested with Vitest, whose inc returns "xs\.map\(\(x\) =>" and then "x \+ 1\);" on the next line, and whose test checks only that inc\(\[\]\) is \[\]$`, c.inc)
	sc.Step(`^I run "itos-cc ([^"]*)" for (calc\.py|src/calc\.ts|src/inc\.ts)$`, func(args, file string) error {
		return m.w.runWith(m.w.env(), args+" "+file)
	})
	sc.Step("^the `\\+` → `-` mutant on line (\\d+) is \"(\\w+)\", and no test command ran for it$", c.notRun)
	sc.Step("^the `\\+` → `-` mutant on line (\\d+) is \"(\\w+)\", as the arrow function's body never ran$", c.notRun)
	sc.Step("^the `\\+` → `-` mutant on line (\\d+) is killed$", c.killed)
	sc.Step(`^the file's counts say (\d+) killed, (\d+) survived and (\d+) uncovered$`, c.counts)
	sc.Step(`^that mutant is "(\w+)", and no test command ran for it$`, c.countedNotRun)
}

// python writes a Python project of files beside its pyproject.toml, the
// test testfile importing file, declares what coverage.py measures of it,
// report, and what its tests do, surviving the sites of surviving and
// each listed test of listed as it says, and commits it.
func (c *continuationLine) python(file, testfile string, files map[string]string, report string, surviving []Site, listed *ListedTests) error {
	m, w := c.m, c.m.w
	c.file, m.file = file, file
	files["pyproject.toml"] = continuationPyproject
	if err := m.writeProject(map[string][]string{testfile: {file}}, files); err != nil {
		return err
	}
	if err := w.d.Coverage(w.dir, Coverage{Language: "python", Report: report}); err != nil {
		return err
	}
	t := Tests{Language: "python", Command: "python3 -m pytest -q -x -p no:cacheprovider " + testfile, Surviving: surviving, Listed: listed}
	if err := w.d.Tests(w.dir, t); err != nil {
		return err
	}
	if err := m.pythonProbe(true); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "."}, {"commit", "-qm", "python fixture"}} {
		if err := w.git(args...); err != nil {
			return err
		}
	}
	return nil
}

// typescript writes a TypeScript project tested with Vitest, file and its
// test, and declares what Vitest's v8 coverage measures of it, report, and
// that its tests kill every mutant they run.
func (c *continuationLine) typescript(file, source, test, report string) error {
	m, w := c.m, c.m.w
	c.file, m.file = file, file
	testfile := file[:len(file)-len(".ts")] + ".test.ts"
	if err := m.writeProject(map[string][]string{testfile: {file}}, map[string]string{"package.json": vitestPackage, file: source, testfile: test}); err != nil {
		return err
	}
	if err := w.d.Coverage(w.dir, Coverage{Language: "typescript", Report: report}); err != nil {
		return err
	}
	return w.d.Tests(w.dir, Tests{Language: "typescript", Command: vitestCommand + file})
}

// twoLines writes demo.py, whose own test survives the `*` mutant on line
// 3, coverage.py marking line 2 alone of the return statement.
func (c *continuationLine) twoLines() error {
	return c.python("demo.py", "test_demo.py", map[string]string{"demo.py": continuationTwoLines, "test_demo.py": continuationTwoLinesTest},
		"SF:demo.py\nDA:1,1\nDA:2,1\nend_of_record\n", []Site{{File: "demo.py", Line: 3, Original: "*", Replacement: "/"}}, nil)
}

// listed adds scn1 to the project and declares it: its coverage marks
// line 2 of demo.py, as coverage.py marks a statement's first line alone,
// though running it runs line 3 too, where it kills the `*` mutant.
func (c *continuationLine) listed() error {
	w := c.m.w
	for name, text := range map[string]string{"scenario.py": continuationScenario, "itos-cc.yaml": continuationListed} {
		if err := w.write(name, text); err != nil {
			return err
		}
	}
	listed := &ListedTests{Files: map[string]string{"scn1": "scenario.py"}, Lines: map[string]map[int][]string{"demo.py": {2: {"scn1"}}},
		Unmarked: map[string]map[int][]string{"demo.py": {3: {"scn1"}}}, Elapsed: map[string]time.Duration{"scn1": time.Second}}
	t := Tests{Language: "python", Command: "python3 -m pytest -q -x -p no:cacheprovider test_demo.py",
		Surviving: []Site{{File: "demo.py", Line: 3, Original: "*", Replacement: "/"}}, Listed: listed}
	if err := w.d.Tests(w.dir, t); err != nil {
		return err
	}
	if err := w.git("add", "scenario.py", "itos-cc.yaml"); err != nil {
		return err
	}
	return w.git("commit", "-qm", "list scn1")
}

// calc writes calc.py or src/calc.ts, the report marking each return
// statement's first line alone: add's run, never's not.
func (c *continuationLine) calc(language, file string) error {
	if language == "Python" {
		return c.python(file, "test_calc.py", map[string]string{file: continuationCalcPy, "test_calc.py": continuationCalcPyTest},
			"SF:calc.py\nDA:1,1\nDA:2,1\nDA:6,1\nDA:7,0\nend_of_record\n", nil, nil)
	}
	return c.typescript(file, continuationCalcTS, continuationCalcTSTest, "SF:src/calc.ts\nDA:2,1\nDA:7,0\nend_of_record\n")
}

// inc writes src/inc.ts, Vitest marking the return statement's line 2 run
// and the arrow function's body on line 3, a line of its own, never run.
func (c *continuationLine) inc() error {
	return c.typescript("src/inc.ts", continuationIncTS, continuationIncTSTest, "SF:src/inc.ts\nDA:2,1\nDA:3,0\nend_of_record\n")
}

// entry is the one mutant of the Given's file on line turning original
// into replacement in the last run's --json object, and the file's own
// object.
func (c *continuationLine) entry(line int, original, replacement string) (map[string]any, map[string]any, error) {
	object, err := c.m.w.json()
	if err != nil {
		return nil, nil, err
	}
	f, err := languageRunFile(object, c.file)
	if err != nil {
		return nil, nil, fmt.Errorf("%w\n%s", err, c.m.w.report())
	}
	var found []map[string]any
	for _, mu := range objectList(f, "mutants") {
		if lineOf(mu) == line && mu["original"] == original && mu["replacement"] == replacement {
			found = append(found, mu)
		}
	}
	if len(found) != 1 {
		return nil, nil, fmt.Errorf("the object lists %d mutants of %s on line %d turning %s into %s, not one\n%s", len(found), c.file, line, original, replacement, c.m.w.report())
	}
	return found[0], f, nil
}

// ran are the trials of the Given's file on line the last run's stderr
// announces.
func (c *continuationLine) ran(line int) []trial {
	var out []trial
	for _, t := range trialsOf(c.m.w.stderr, c.file) {
		if t.line == line {
			out = append(out, t)
		}
	}
	return out
}

// killedByScn1 is whether the `*` mutant on demo.py's line 3 ran its own
// tests, survived them, then ran scn1 alone and was killed, as stderr
// announces it and the --json object records it.
func (c *continuationLine) killedByScn1() error {
	mu, _, err := c.entry(3, "*", "/")
	if err != nil {
		return err
	}
	if mu["outcome"] != "killed" || mu["scope"] != "listed" || !slices.Equal(entryIDs(mu), []string{"scn1"}) {
		return fmt.Errorf("the `*` mutant on line 3 is %v, not killed by scn1\n%s", mu, c.m.w.report())
	}
	trials := c.ran(3)
	if len(trials) != 1 || trials[0].outcome != "survived its own tests, then killed with scn1" {
		return fmt.Errorf("the `*` mutant on line 3 ran %v, not its own tests then scn1 alone\n%s", trials, c.m.w.report())
	}
	return c.m.w.exitsWith(0)
}

// snapshotListed is whether demo.py's snapshot records the `*` mutant on
// line 3 killed by scn1.
func (c *continuationLine) snapshotListed() error {
	data, err := c.m.w.readIn(c.m.w.dir, ".metrics/mutate/"+c.file+".json")
	if err != nil {
		return err
	}
	var snap map[string]any
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	var found []map[string]any
	for _, e := range snapshotEntries(snap) {
		if lineOf(e) == 3 && e["original"] == "*" {
			found = append(found, e)
		}
	}
	if len(found) != 1 || found[0]["outcome"] != "killed" || found[0]["scope"] != "listed" || !slices.Equal(entryIDs(found[0]), []string{"scn1"}) {
		return fmt.Errorf("the snapshot records %v of the `*` mutant on line 3, not one killed by scn1\n%s", found, data)
	}
	return nil
}

// notRun is whether the `+` mutant on line is want, and no trial of it
// ran.
func (c *continuationLine) notRun(line int, want string) error {
	mu, _, err := c.entry(line, "+", "-")
	if err != nil {
		return err
	}
	if mu["outcome"] != want {
		return fmt.Errorf("the `+` mutant on line %d is %v, not %s\n%s", line, mu["outcome"], want, c.m.w.report())
	}
	if trials := c.ran(line); len(trials) != 0 {
		return fmt.Errorf("the `+` mutant on line %d ran: %v\n%s", line, trials, c.m.w.report())
	}
	return nil
}

// killed is whether the `+` mutant on line ran and was killed.
func (c *continuationLine) killed(line int) error {
	mu, _, err := c.entry(line, "+", "-")
	if err != nil {
		return err
	}
	if mu["outcome"] != "killed" || len(c.ran(line)) != 1 {
		return fmt.Errorf("the `+` mutant on line %d is %v, not run and killed\n%s", line, mu["outcome"], c.m.w.report())
	}
	return nil
}

// counts is whether the --json object counts the Given's file's mutants so.
func (c *continuationLine) counts(killed, survived, uncovered int) error {
	_, f, err := c.entry(3, "+", "-")
	if err != nil {
		return err
	}
	if f["killed"] != float64(killed) || f["survived"] != float64(survived) || f["uncovered"] != float64(uncovered) {
		return fmt.Errorf("%s counts killed %v, survived %v and uncovered %v, not %d, %d and %d\n%s", c.file, f["killed"], f["survived"], f["uncovered"], killed, survived, uncovered, c.m.w.report())
	}
	return nil
}

// countedNotRun is whether the counted run's one selected site, calc.py's
// `+` on line 8, is want in state and outcome, and no trial ran.
func (c *continuationLine) countedNotRun(want string) error {
	object, err := c.m.w.json()
	if err != nil {
		return err
	}
	selected := objectList(object, "selected")
	if len(selected) != 1 || filepath.ToSlash(fmt.Sprint(selected[0]["file"])) != c.file || lineOf(selected[0]) != 8 ||
		selected[0]["state"] != want || selected[0]["outcome"] != want {
		return fmt.Errorf("\"selected\" is not calc.py's line 8 site %s\n%s", want, c.m.w.report())
	}
	if trials := trialsOf(c.m.w.stderr, c.file); len(trials) != 0 {
		return fmt.Errorf("the run ran trials of %s: %v\n%s", c.file, trials, c.m.w.report())
	}
	return nil
}
