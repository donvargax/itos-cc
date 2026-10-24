// The steps of crap.feature: each function's CRAP score, as itos-cc crap
// prints it, writes it to .metrics/crap.json and reports it with --json.
// A function scored in a Given's file is scored through --coverage-report
// over a report the step writes by hand; the scenarios that run itos-cc
// crap as written measure coverage with the project's own tools, which a
// Given declares (Driver.Coverage). Like every step here, they read the
// binary's output alone, never its code.
package features

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// crapState is one crap.feature scenario's state beside the world's.
type crapState struct {
	w *world

	// functions are every function of the project a Given or the sample
	// wrote, its file slash-separated from the scratch folder.
	functions []function
	// languages are the languages the project's tests measure.
	languages []string
	// unscored is the file of the functions no coverage report applies to,
	// slash-separated from the scratch folder; "" for none.
	unscored string
	// report is the report a Given declared the project's tools write.
	report string
	// tests are the test files of the project, by slash-separated path.
	tests map[string]string
	// cc and coverage are those of the function a Given wrote to be scored
	// through a report written by hand, coverage in percent.
	cc, coverage int
}

// function is a function of a project a scenario wrote: its file,
// slash-separated from the scratch folder, and its name.
type function struct{ file, name string }

// crapEntry is one function's score as crap reports it, from its --json
// object, its table or .metrics/crap.json. Coverage and CRAP are nil where
// it shows N/A.
type crapEntry struct {
	Namespace  string   `json:"namespace"`
	Name       string   `json:"name"`
	Language   string   `json:"language"`
	File       string   `json:"file"`
	StartLine  int      `json:"start_line"`
	EndLine    int      `json:"end_line"`
	Complexity int      `json:"complexity"`
	Coverage   *float64 `json:"coverage"`
	CRAP       *float64 `json:"crap"`

	// shown is the function as a row of the table names it, "" for an
	// entry of an object.
	shown string
}

func (e crapEntry) id() string { return e.Namespace + "#" + e.Name }

// crapObject is crap's --json object.
type crapObject struct {
	Entries    []crapEntry      `json:"entries"`
	Unmeasured []map[string]any `json:"unmeasured"`
	Problems   []map[string]any `json:"problems"`
}

// shown is a score or a coverage as the table prints it: one decimal, or
// N/A for none.
func shown(v *float64) string {
	if v == nil {
		return "N/A"
	}
	return strconv.FormatFloat(*v, 'f', 1, 64)
}

// A row of crap's table: the CRAP, CC and COV% columns, then the function.
var crapRow = regexp.MustCompile(`^\s*(\S+)\s+(\d+)\s+(\S+)\s+(\S.*)$`)

// The function a row names: namespace#name, two spaces, file:line.
var rowFunction = regexp.MustCompile(`^(.*)#(.+)  (.+):(\d+)$`)

// The module every Go fixture is, so a function at its root is board#name.
const goModule = "module board\n\ngo 1.22\n"

// pom.xml of a Kotlin module built with Maven and no JaCoCo: a build root
// coverage cannot measure, which no tool needs to be installed to show.
const unmeasuredPom = "<project>\n  <modelVersion>4.0.0</modelVersion>\n</project>\n"

// unscoredKotlin is a Kotlin source of that module: functions of
// complexity 1 and 3.
const unscoredKotlin = "fun plain(a: Int): Int {\n    return a\n}\n\n" +
	"fun branchy(a: Int): Int {\n    if (a > 1) {\n        return 1\n    }\n    if (a > 2) {\n        return 2\n    }\n    return 0\n}\n"

const unscoredFile = "kt/src/main/kotlin/Board.kt"

// crapFeature declares crap.feature's steps, crapSteps, which
// tools/bin/assemble lists in steps_gen.go.
var crapFeature = featureFile{Name: "crap.feature", Steps: crapSteps}

func crapSteps(sc *godog.ScenarioContext, w *world) {
	c := &crapState{w: w}
	sc.Before(func(ctx context.Context, s *godog.Scenario) (context.Context, error) {
		*c = crapState{w: w}
		if filepath.Base(s.Uri) != "crap.feature" {
			return ctx, nil
		}
		// it is scored (steps.go) scores the function a Given wrote.
		w.scored = c.scored
		if len(s.Steps) == 0 || !strings.HasPrefix(s.Steps[0].Text, `I run "itos-cc`) {
			return ctx, nil
		}
		// The scenarios that only say When I run run in the sample.
		return ctx, c.sample()
	})

	// The formula.
	sc.Step(`^a function with complexity (\d+) and (\d+)% coverage$`, c.functionWith)
	sc.Step(`^its CRAP score is (\d+(?:\.\d+)?)$`, c.crapScoreIs)

	// A project scored through its own tools.
	sc.Step(`^a project with tests$`, c.projectWithTests)
	sc.Step(`^each language's coverage command runs, with its output on stderr$`, c.coverageCommandsRun)
	sc.Step(`^stdout is a table with the columns (.+)$`, c.tableWithColumns)
	sc.Step(`^each row names the function as "([^"]*)"$`, c.rowsName)
	sc.Step(`^rows are sorted from the highest CRAP score down$`, c.rowsSorted)

	// Functions no coverage applies to, or that no test loads.
	sc.Step(`^a function that no coverage report applies to$`, c.unscoredFunction)
	sc.Step(`^its CRAP and COV% are shown as "([^"]*)"$`, c.unscoredShownAs)
	sc.Step(`^it is listed after every scored function, by complexity$`, c.unscoredLast)
	sc.Step(`^coverage measured other Python files in the project$`, c.pythonProject)
	sc.Step(`^the report never mentions (\S+)$`, c.reportNeverMentions)
	sc.Step(`^the functions of (\S+) have (\d+)% coverage$`, c.functionsCovered)
	sc.Step(`^a TypeScript project whose tests import (\S+) but never (\S+), whose functions are simple$`, c.typescriptProject)
	sc.Step(`^no "([^"]*)" problem is reported$`, c.noProblem)
	sc.Step(`^the coverage report mentions no (\w+) file$`, c.reportMentionsNo)
	sc.Step(`^the (\w+) functions have no CRAP score$`, c.noCRAPScore)

	// The output and the snapshot.
	sc.Step(`^only the (\d+) highest-scoring functions are printed$`, c.onlyTop)
	sc.Step(`^\.metrics/crap\.json still records every function$`, c.recordsEveryFunction)
	sc.Step(`^each entry has (.+)$`, c.eachEntryHas)
	sc.Step(`^each unmeasured build root has (.+)$`, c.eachUnmeasuredHas)
	sc.Step(`^\.metrics/crap\.json holds every scored function$`, c.holdsEveryScored)

	// The threshold.
	sc.Step(`^a function scores (\d+\.\d)$`, c.functionScores)
	sc.Step(`^it says so for every function above the threshold, each a "([^"]*)" problem with (.+)$`, c.saysSoForEvery)
	sc.Step(`^every function scores (\d+) or less$`, c.everyFunctionAtMost)
	sc.Step(`^a project whose (tests do not compile|coverage tool is not installed|report is missing, with --use-existing-coverage|--coverage-report cannot be read)$`, c.projectWhose)
	sc.Step(`^a Go project whose tests do not compile$`, c.goTestsDoNotCompile)
	sc.Step(`^a Go project whose package example\.com/slow has a test that sleeps 3 seconds$`, c.goTestSleeps)
	sc.Step(`^its functions show "([^"]*)"$`, c.functionsShow)
	sc.Step(`^the problem is "([^"]*)", with (\w+(?:(?:, and |, | and )\w+)*)$`, c.problemWith)
	sc.Step(`^the selection holds no production source files$`, c.noProductionSources)
}

// Fixtures.

// goFunction is a function a Go fixture holds: its name, its complexity,
// and whether the fixture's test runs every line of it.
type goFunction struct {
	name    string
	cc      int
	covered bool
}

// goFile is a Go file of package board holding fns, each taking a and
// making cc-1 decisions on it, every one true for a of 100, and the LCOV of
// what a test calling each covered one with 100 runs, naming the file
// name.
func goFile(name string, fns []goFunction) (source, lcov string) {
	var b strings.Builder
	da := []string{"SF:" + name}
	line := 0
	emit := func(text string, measured, hit bool) {
		b.WriteString(text + "\n")
		line++
		if measured {
			da = append(da, fmt.Sprintf("DA:%d,%d", line, hits(hit)))
		}
	}
	emit("package board", false, false)
	for _, f := range fns {
		emit("", false, false)
		emit("func "+f.name+"(a int) int {", false, false)
		emit("\tn := 0", true, f.covered)
		for i := 1; i < f.cc; i++ {
			emit(fmt.Sprintf("\tif a > %d {", i), true, f.covered)
			emit("\t\tn++", true, f.covered)
			emit("\t}", false, false)
		}
		emit("\treturn n", true, f.covered)
		emit("}", false, false)
	}
	return b.String(), strings.Join(append(da, "end_of_record"), "\n") + "\n"
}

// hits is an LCOV line's count: 1 for a line run, 0 for one not.
func hits(run bool) int {
	if run {
		return 1
	}
	return 0
}

// goTest is a test of package board calling each covered function of fns
// with 100.
func goTest(fns []goFunction) string {
	var b strings.Builder
	b.WriteString("package board\n\nimport \"testing\"\n\nfunc TestCovered(t *testing.T) {\n")
	for _, f := range fns {
		if f.covered {
			fmt.Fprintf(&b, "\tif %s(100) < 0 {\n\t\tt.Fatal(%q)\n\t}\n", f.name, f.name)
		}
	}
	b.WriteString("}\n")
	return b.String()
}

// goProject writes the Go module board in the folder dir of the scratch
// folder, "." for the scratch folder itself, its board.go holding fns and
// its test calling the covered ones, and declares that go test measures it.
func (c *crapState) goProject(dir string, fns []goFunction) error {
	source, lcov := goFile("board.go", fns)
	files := map[string]string{"go.mod": goModule, "board.go": source, "board_test.go": goTest(fns)}
	if err := c.writeIn(dir, files); err != nil {
		return err
	}
	for _, f := range fns {
		c.functions = append(c.functions, function{filepath.ToSlash(filepath.Join(dir, "board.go")), f.name})
	}
	c.languages, c.report = append(c.languages, "go"), lcov
	return c.w.d.Coverage(filepath.Join(c.w.dir, filepath.FromSlash(dir)), Coverage{Language: "go", Report: lcov})
}

// unmeasuredModule writes a Kotlin module in kt/ that coverage cannot
// measure, its functions of complexity 1 and 3, and declares no tools of
// it.
func (c *crapState) unmeasuredModule() error {
	if err := c.writeIn(".", map[string]string{"kt/pom.xml": unmeasuredPom, unscoredFile: unscoredKotlin}); err != nil {
		return err
	}
	c.unscored = unscoredFile
	c.functions = append(c.functions, function{unscoredFile, "plain"}, function{unscoredFile, "branchy"})
	return nil
}

// writeIn writes files, by slash-separated path, in the folder dir of the
// scratch folder.
func (c *crapState) writeIn(dir string, files map[string]string) error {
	for name, text := range files {
		if err := c.w.write(filepath.ToSlash(filepath.Join(dir, name)), text); err != nil {
			return err
		}
	}
	return nil
}

// sample is the project the scenarios that only say When I run run in: a
// Go module of 24 functions, f01 to f24, each of complexity its number,
// the first four wholly covered by its test and the rest never run, so no
// two score the same, and the Kotlin module coverage cannot measure.
func (c *crapState) sample() error {
	var fns []goFunction
	for i := 1; i <= 24; i++ {
		fns = append(fns, goFunction{name: fmt.Sprintf("f%02d", i), cc: i, covered: i <= 4})
	}
	if err := c.goProject(".", fns); err != nil {
		return err
	}
	return c.unmeasuredModule()
}

// functionWith writes board.go, whose function place makes cc-1 decisions,
// and coverage.info, a report measuring 20 of its lines, the first
// percent of them run.
func (c *crapState) functionWith(cc, percent int) error {
	const measured = 20
	if cc < 1 || 2*cc > measured || percent > 100 || percent*measured%100 != 0 {
		return fmt.Errorf("no function of complexity %d with %d%% of %d lines run is written here", cc, percent, measured)
	}
	c.cc, c.coverage = cc, percent
	lines := []string{"package board", "", "func place(a int) int {"}
	var statements []int
	statement := func(text string) {
		lines = append(lines, text)
		statements = append(statements, len(lines))
	}
	statement("\tn := 0")
	for i := 1; i < cc; i++ {
		statement(fmt.Sprintf("\tif a > %d {", i))
		statement("\t\tn++")
		lines = append(lines, "\t}")
	}
	for len(statements) < measured-1 {
		statement("\tn += 2")
	}
	statement("\treturn n")
	lines = append(lines, "}")
	report := []string{"SF:board.go"}
	for i, line := range statements {
		report = append(report, fmt.Sprintf("DA:%d,%d", line, hits(i < percent*measured/100)))
	}
	report = append(report, "end_of_record")
	return c.writeIn(".", map[string]string{
		"board.go":      strings.Join(lines, "\n") + "\n",
		"coverage.info": strings.Join(report, "\n") + "\n",
	})
}

// scored scores the function a Given wrote, through the report it wrote.
func (c *crapState) scored() error {
	if c.cc == 0 {
		return errors.New("no function was written to be scored")
	}
	return c.w.runWith(c.w.env(), "crap --json --coverage-report coverage.info board.go")
}

// crapScoreIs is whether the run scored the one function written at want,
// with the complexity and the coverage the Given wrote it with.
func (c *crapState) crapScoreIs(want string) error {
	entries, err := c.listed()
	if err != nil {
		return err
	}
	if len(entries) != 1 {
		return fmt.Errorf("%d functions are scored, not the one written\n%s", len(entries), c.w.report())
	}
	e := entries[0]
	if e.Complexity != c.cc || shown(e.Coverage) != shown(ptr(float64(c.coverage))) {
		return fmt.Errorf("the function written has complexity %d and %s%% coverage, not %d and %d%%\n%s",
			e.Complexity, shown(e.Coverage), c.cc, c.coverage, c.w.report())
	}
	value, err := strconv.ParseFloat(want, 64)
	if err != nil {
		return err
	}
	if shown(e.CRAP) != shown(&value) {
		return fmt.Errorf("its CRAP score is %s, not %s\n%s", shown(e.CRAP), want, c.w.report())
	}
	return nil
}

func ptr(v float64) *float64 { return &v }

// projectWithTests writes a Go module whose functions score 12, 2 and 1:
// one never run, two its test covers.
func (c *crapState) projectWithTests() error {
	return c.goProject(".", []goFunction{
		{name: "place", cc: 3},
		{name: "score", cc: 2, covered: true},
		{name: "half", cc: 1, covered: true},
	})
}

// coverageCommands are, for each language a project's tests measure, a
// line of the command coverage runs, as itos-cc prints it, and a line of
// what that command prints.
var coverageCommands = map[string]struct{ command, output *regexp.Regexp }{
	"go": {regexp.MustCompile(`(?m)^itos-cc: coverage .+\$ go test .*-coverprofile`), regexp.MustCompile(`(?m)^ok\s+board\s`)},
}

// coverageCommandsRun is whether stderr shows, for each language the
// project's tests measure, the command coverage ran and what it printed,
// none of which stdout shows.
func (c *crapState) coverageCommandsRun() error {
	w := c.w
	if len(c.languages) == 0 {
		return errors.New("no project with tests was written")
	}
	for _, language := range c.languages {
		lines, ok := coverageCommands[language]
		if !ok {
			return fmt.Errorf("no coverage command is known for %s", language)
		}
		if !lines.command.MatchString(w.stderr) {
			return fmt.Errorf("stderr shows no %s coverage command\n%s", language, w.report())
		}
		if !lines.output.MatchString(w.stderr) || lines.output.MatchString(w.stdout) {
			return fmt.Errorf("the %s coverage command's output is not on stderr alone\n%s", language, w.report())
		}
	}
	return nil
}

// pythonProject writes a Python project whose test measures src/board.py
// and never imports src/unused.py, and declares coverage.py measures it.
func (c *crapState) pythonProject() error {
	c.tests = map[string]string{"tests/test_board.py": "from board import place\n\n\ndef test_place():\n    assert place(1) == 1\n"}
	files := map[string]string{
		"pyproject.toml": "[project]\nname = \"board\"\nversion = \"0.1.0\"\n\n[tool.pytest.ini_options]\npythonpath = [\"src\"]\n",
		"src/board.py":   "def place(a):\n    if a > 0:\n        return 1\n    return 2\n",
		"src/unused.py":  "def half(a):\n    return a / 2\n\n\ndef zero(a):\n    return a == 0\n",
	}
	for name, text := range c.tests {
		files[name] = text
	}
	if err := c.writeIn(".", files); err != nil {
		return err
	}
	c.languages = append(c.languages, "python")
	c.report = "SF:src/board.py\nDA:1,1\nDA:2,1\nDA:3,1\nDA:4,0\nend_of_record\n"
	return c.w.d.Coverage(c.w.dir, Coverage{Language: "python", Report: c.report})
}

// reportNeverMentions is whether file is a source of the project that the
// report its tools write names nowhere, as none of its tests imports it.
func (c *crapState) reportNeverMentions(file string) error {
	switch {
	case c.report == "":
		return errors.New("no project's report was declared")
	case !c.w.d.Exists(filepath.Join(c.w.dir, filepath.FromSlash(file))):
		return fmt.Errorf("the project holds no %s", file)
	case strings.Contains(c.report, "SF:"+file+"\n"):
		return fmt.Errorf("the report mentions %s:\n%s", file, c.report)
	}
	module := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	for name, text := range c.tests {
		if strings.Contains(text, module) {
			return fmt.Errorf("%s imports %s", name, module)
		}
	}
	return nil
}

// functionsCovered is whether every function of file the run lists has
// percent coverage.
func (c *crapState) functionsCovered(file string, percent int) error {
	rows, err := c.rowsOf(file)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if shown(row.Coverage) != shown(ptr(float64(percent))) {
			return fmt.Errorf("%s has %s%% coverage, not %d%%\n%s", row.id(), shown(row.Coverage), percent, c.w.report())
		}
	}
	return nil
}

// typescriptProject writes a TypeScript project of Vitest whose one test
// imports imported and never other, every function of the two of
// complexity 1, and declares Vitest measures it.
func (c *crapState) typescriptProject(imported, other string) error {
	if filepath.Dir(imported) != filepath.Dir(other) || filepath.Ext(imported) != ".ts" || filepath.Ext(other) != ".ts" {
		return fmt.Errorf("%s and %s are not two TypeScript files of one folder", imported, other)
	}
	module := strings.TrimSuffix(filepath.Base(imported), ".ts")
	test := strings.TrimSuffix(imported, ".ts") + ".test.ts"
	c.tests = map[string]string{test: "import { expect, test } from \"vitest\";\nimport { place } from \"./" + module + "\";\n\n" +
		"test(\"place\", () => {\n  expect(place(5)).toBe(false);\n  expect(place(6)).toBe(true);\n});\n"}
	files := map[string]string{
		"package.json": `{"name": "m", "type": "module", "devDependencies": {"vitest": "5.0.2", "@vitest/coverage-v8": "5.0.2"}}` + "\n",
		imported:       "export function place(i: number): boolean {\n  return i > 5;\n}\n",
		other: "export function half(i: number): boolean {\n  return i > 2;\n}\n\n" +
			"export function zero(i: number): boolean {\n  return i === 0;\n}\n",
		test: c.tests[test],
	}
	if err := c.writeIn(".", files); err != nil {
		return err
	}
	// Simple: no function of either makes a decision.
	code, stdout, stderr, err := c.w.capture(c.w.dir, c.w.env(), "crap", "--no-coverage", "--json", imported, other)
	if err != nil {
		return err
	}
	var object crapObject
	if err := json.Unmarshal([]byte(stdout), &object); err != nil || code != 0 || len(object.Entries) < 2 {
		return fmt.Errorf("the project's functions could not be measured: exit %d, %v\n%s%s", code, err, stdout, stderr)
	}
	for _, e := range object.Entries {
		if e.Complexity != 1 {
			return fmt.Errorf("%s has complexity %d, not simple", e.id(), e.Complexity)
		}
	}
	c.languages = append(c.languages, "typescript")
	c.report = "SF:" + imported + "\nDA:1,1\nDA:2,1\nend_of_record\n"
	return c.w.d.Coverage(c.w.dir, Coverage{Language: "typescript", Report: c.report})
}

// noProblem is whether the last run, made with --json, reports no problem
// of the named rule or message text.
func (c *crapState) noProblem(rule string) error {
	object, report, err := c.lastJSON()
	if err != nil {
		return err
	}
	if strings.Contains(c.w.stderr, rule) {
		return fmt.Errorf("a %s problem is reported on stderr\n%s", rule, report)
	}

	for _, p := range object.Problems {
		if p["rule"] == rule || strings.Contains(fmt.Sprint(p["message"]), rule) {
			return fmt.Errorf("a %s problem is reported: %v\n%s", rule, p, report)
		}
	}
	return nil
}

// languageFiles are the extensions of the languages a sentence names.
var languageFiles = map[string]string{"Kotlin": ".kt"}

// reportMentionsNo is a Go module its test measures beside the Kotlin
// module coverage cannot measure, the report go test writes mentioning no
// file of language.
func (c *crapState) reportMentionsNo(language string) error {
	ext, ok := languageFiles[language]
	if !ok {
		return fmt.Errorf("no project is written whose report mentions no %s file", language)
	}
	if err := c.unscoredFunction(); err != nil {
		return err
	}
	if strings.Contains(c.report, ext+"\n") {
		return fmt.Errorf("the report mentions a %s file:\n%s", language, c.report)
	}
	return nil
}

// noCRAPScore is whether .metrics/crap.json records functions of language,
// each with no coverage and no CRAP score.
func (c *crapState) noCRAPScore(language string) error {
	recorded, err := c.snapshot()
	if err != nil {
		return err
	}
	n := 0
	for _, e := range recorded {
		if e.Language != strings.ToLower(language) {
			continue
		}
		n++
		if e.CRAP != nil || e.Coverage != nil {
			return fmt.Errorf("%s has coverage %s and CRAP %s\n%s", e.id(), shown(e.Coverage), shown(e.CRAP), c.w.report())
		}
	}
	if n == 0 {
		return fmt.Errorf(".metrics/crap.json records no %s function\n%s", language, c.w.report())
	}
	return nil
}

// worst is entries from the highest score down, those with none last from
// the highest complexity down, as crap orders them.
func worst(entries []crapEntry) []crapEntry {
	out := slices.Clone(entries)
	slices.SortStableFunc(out, func(a, b crapEntry) int {
		switch {
		case a.CRAP != nil && b.CRAP != nil:
			if c := cmp.Compare(*b.CRAP, *a.CRAP); c != 0 {
				return c
			}
		case a.CRAP != nil:
			return -1
		case b.CRAP != nil:
			return 1
		}
		return b.Complexity - a.Complexity
	})
	return out
}

// onlyTop is whether the table's rows are the n highest-scoring of the
// functions .metrics/crap.json records, more than n, in that order.
func (c *crapState) onlyTop(n int) error {
	w := c.w
	rows, err := c.table()
	if err != nil {
		return err
	}
	recorded, err := c.snapshot()
	if err != nil {
		return err
	}
	if len(recorded) <= n {
		return fmt.Errorf(".metrics/crap.json records %d functions, not more than %d\n%s", len(recorded), n, w.report())
	}
	if len(rows) != n {
		return fmt.Errorf("%d functions are printed, not %d\n%s", len(rows), n, w.report())
	}
	for i, e := range worst(recorded)[:n] {
		if rows[i].id() != e.id() || filepath.ToSlash(rows[i].File) != filepath.ToSlash(e.File) {
			return fmt.Errorf("row %d is %s, not %s, the %d-highest score\n%s", i+1, rows[i].id(), e.id(), i+1, w.report())
		}
	}
	return nil
}

// projectWhose writes, in board/ or web/, a project coverage cannot
// measure, as the row says, with the words a run of it needs for that.
func (c *crapState) projectWhose(problem string) error {
	switch problem {
	case "tests do not compile":
		return c.goTestsDoNotCompile()
	case "coverage tool is not installed":
		// Vitest is the project's, and no node_modules holds it: no tool
		// measures it, so none is declared.
		c.w.project = "web"
		return c.writeIn("web", map[string]string{
			"package.json": `{"name": "web", "devDependencies": {"vitest": "5.0.2", "@vitest/coverage-v8": "5.0.2"}}` + "\n",
			"src/view.ts":  "export function view(a: number): number {\n  if (a > 0) {\n    return 1;\n  }\n  return 2;\n}\n",
		})
	case "report is missing, with --use-existing-coverage":
		// No test runs with the flag, and no report is on disk.
		c.w.project, c.w.with = "board", []string{"--use-existing-coverage"}
		fns := []goFunction{{name: "place", cc: 2, covered: true}}
		source, _ := goFile("board.go", fns)
		return c.writeIn("board", map[string]string{"go.mod": goModule, "board.go": source, "board_test.go": goTest(fns)})
	}
	// --coverage-report cannot be read: a JaCoCo report cut short. The run
	// reads it alone, running no test.
	c.w.project, c.w.with = "board", []string{"--coverage-report", "coverage.xml"}
	source, _ := goFile("board.go", []goFunction{{name: "place", cc: 2}})
	if err := c.writeIn("board", map[string]string{"go.mod": goModule, "board.go": source}); err != nil {
		return err
	}
	return c.writeIn(".", map[string]string{"coverage.xml": "<report name=\"board\"><package name=\"board\">"})
}

// goTestsDoNotCompile writes the Go module board in board/, whose test
// calls a function nowhere defined, and declares go test runs there and
// writes no report.
func (c *crapState) goTestsDoNotCompile() error {
	c.w.project = "board"
	source, _ := goFile("board.go", []goFunction{{name: "place", cc: 2}})
	if err := c.writeIn("board", map[string]string{
		"go.mod":        goModule,
		"board.go":      source,
		"board_test.go": "package board\n\nimport \"testing\"\n\nfunc TestPlace(t *testing.T) {\n\tif nowhere(1) != 1 {\n\t\tt.Fatal(\"place\")\n\t}\n}\n",
	}); err != nil {
		return err
	}
	c.languages = append(c.languages, "go")
	return c.w.d.Coverage(filepath.Join(c.w.dir, "board"), Coverage{Language: "go"})
}

// goTestSleeps is the module example.com/slow in the folder slow, whose one
// test sleeps 3 seconds, so a go test limit of 1s stops it. The in-process
// driver declares what go test does then: it prints the panic of a test
// binary that outlived its limit, ends the package's output with its FAIL
// line, exits 1, and writes a report that names the package's one block,
// never run, as it does for a package of -coverpkg whose binary wrote no
// counters.
func (c *crapState) goTestSleeps() error {
	c.w.project = "slow"
	if err := c.writeIn("slow", map[string]string{
		"go.mod":       "module example.com/slow\n\ngo 1.22\n",
		"slow.go":      "package slow\n\nfunc Place(i int) bool { return i > 5 }\n",
		"slow_test.go": "package slow\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\nfunc TestPlace(t *testing.T) {\n\ttime.Sleep(3 * time.Second)\n\tif !Place(6) {\n\t\tt.Fatal(\"place\")\n\t}\n}\n",
	}); err != nil {
		return err
	}
	c.languages = append(c.languages, "go")
	return c.w.d.Coverage(filepath.Join(c.w.dir, "slow"), Coverage{
		Language:      "go",
		Report:        "mode: set\nexample.com/slow/slow.go:3.26,3.40 1 0\n",
		CommandExit:   1,
		CommandOutput: "panic: test timed out after 1s\n\trunning tests:\n\t\tTestPlace (1s)\n\ngoroutine 33 [running]:\ntesting.(*M).startAlarm.func1()\nFAIL\texample.com/slow\t1.005s\nFAIL\n",
	})
}

// functionsShow is whether the run lists functions, each showing text as
// its CRAP and COV%.
func (c *crapState) functionsShow(text string) error {
	entries, err := c.listed()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("no function is listed\n%s", c.w.report())
	}
	for _, e := range entries {
		if shown(e.CRAP) != text || shown(e.Coverage) != text {
			return fmt.Errorf("%s shows %s and %s, not %s\n%s", e.id(), shown(e.CRAP), shown(e.Coverage), text, c.w.report())
		}
	}
	return nil
}

// problemWith is whether the last run, made with --json, reports a problem
// of rule giving every key the list names, its dir the project's folder,
// and its error output prints the problem's message.
func (c *crapState) problemWith(rule, list string) error {
	object, report, err := c.lastJSON()
	if err != nil {
		return err
	}
	for _, p := range object.Problems {
		if p["rule"] != rule {
			continue
		}
		if err := eachHas(rule+" problem", []map[string]any{p}, list, true); err != nil {
			return fmt.Errorf("%v\n%s", err, report)
		}
		if dir, ok := p["dir"].(string); ok && c.w.project != "" && filepath.ToSlash(dir) != c.w.project {
			return fmt.Errorf("the problem's dir is %s, not %s\n%s", dir, c.w.project, report)
		}
		if message, _ := p["message"].(string); message == "" || !strings.Contains(c.w.stderr, message) {
			return fmt.Errorf("the error output does not print the problem's message %q\n%s", message, report)
		}
		if c.w.problemSubject != nil {
			return c.w.problemSubject()
		}
		return nil
	}
	return fmt.Errorf("no problem has the rule %q\n%s", rule, report)
}

// noProductionSources writes a test file and a README alone.
func (c *crapState) noProductionSources() error {
	return c.writeIn(".", map[string]string{
		"tests/test_board.py": "def test_place():\n    assert 1 + 1 == 2\n",
		"README.md":           "# board\n",
	})
}

// Reading what crap printed and wrote.
