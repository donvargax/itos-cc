// The steps of scrap.feature: the examples itos-cc scrap finds in test
// files, how it measures and scores them, and the action and the
// recommendations it gives each file, judged from what it prints and the
// .metrics/scrap.json it writes. A measurement is read from scrap --json
// --verbose over the scenario's files. Where a sentence states what a
// fixture holds (its counts, its scores, the pressure a run recorded), the
// fixture is checked against what the binary itself measures, so a fixture
// that does not hold what the sentence says fails instead of passing. Like
// every step here, they read the binary's output alone, never its code.
package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// scrapState is one scrap.feature scenario's state beside the world's.
type scrapState struct {
	w *world

	// files are the test files the scenario wrote, in order, and file the
	// one its sentences call "it" or "a file".
	files []string
	file  string
	// expected are the examples of the SCRAP-01 fixtures, with what
	// encloses each.
	expected map[string][]grouped
	// setups are the SCRAP-03 fixtures, each with its setup and without it.
	setups []setupFixture
	// kotlinClass and annotations are those of the Kotlin class SCRAP-04
	// writes.
	kotlinClass string
	annotations []string
	// helperFailure is the code of the Go helper SCRAP-06 writes.
	helper, helperFailure string
	// stated are the measurements the scenario's Given says its example
	// has.
	stated *scrapMetrics
	// before and after are the fixtures of a pressure recorded and of the
	// pressure now, beforePressure what the recording run measured.
	before, after  string
	beforePressure float64
	// kept is the report .metrics/scrap.json held of a file before the run.
	kept json.RawMessage
	// examples is the count of examples SCRAP-19's file has.
	examples int

	// measured are the reports of the last measuring run, by file.
	measured map[string]scrapReport

	// reportedRun is the run reportedFiles and reportedErr were read from.
	reportedRun   *run
	reportedFiles map[string]scrapReport
	reportedErr   error
}

func scrapSteps(sc *godog.ScenarioContext, w *world) {
	d := &scrapState{w: w}
	sc.Before(func(ctx context.Context, s *godog.Scenario) (context.Context, error) {
		*d = scrapState{w: w}
		if filepath.Base(s.Uri) != "scrap.feature" {
			return ctx, nil
		}
		// it is scored (steps.go) scores the example.
		w.scored = d.scored
		if len(s.Steps) == 0 || !strings.HasPrefix(s.Steps[0].Text, `I run "itos-cc`) {
			return ctx, nil
		}
		// The scenarios that only say When I run run in the sample.
		return ctx, d.writeFiles(scrapSample)
	})

	// Finding examples.
	sc.Step(`^a (TypeScript|Python|Go|Kotlin) test file using (.+)$`, d.testFileUsing)
	sc.Step(`^each test case is an example, grouped by its enclosing (.+)$`, d.groupedBy)
	sc.Step(`^a Go test that loops over a table of cases and asserts in the loop body$`, func() error {
		return d.writeFile("table_test.go", goTableTest)
	})
	sc.Step(`^it is measured$`, func() error { return d.measure() })
	sc.Step(`^it is marked as a table$`, func() error {
		e, err := d.onlyExample()
		if err != nil {
			return err
		}
		if !e.Table {
			return fmt.Errorf("%q is not marked as a table: %+v", e.Name, e)
		}
		return nil
	})
	sc.Step(`^the loop does not count as logic$`, func() error {
		e, err := d.onlyExample()
		if err != nil {
			return err
		}
		if e.Decisions != 0 || slices.Contains(e.Smells, "logic") {
			return fmt.Errorf("%q has %d decisions and the smells %v", e.Name, e.Decisions, e.Smells)
		}
		return nil
	})
	sc.Step(`^a test file with ((?:[@\w]+, )*[@\w]+,? or [@\w]+)$`, d.testFileWithSetup)
	sc.Step(`^those lines count as setup, not as any example's size$`, d.setupNotSize)
	sc.Step(`^the Kotlin test file "([^"]*)" with a class annotated (@\w+(?: and @\w+)*)$`, func(name, annotations string) error {
		d.file, d.kotlinClass = name, strings.TrimSuffix(filepath.Base(name), ".kt")
		d.annotations = strings.Split(annotations, " and ")
		return nil
	})
	sc.Step(`^a @Test method "([^"]*)" with (one|two|\d+) assertEquals$`, d.kotlinMethod)
	sc.Step(`^"([^"/]*)/([^"]*)" is an example with (\d+) assertions?$`, d.isAnExampleWith)

	// Measuring examples.
	sc.Step(`^the (TypeScript|Python|Go|Kotlin) test file "([^"]*)":$`, func(_, name string, text *godog.DocString) error {
		return d.writeFile(name, text.Content+"\n")
	})
	sc.Step(`^the examples are:$`, d.examplesAre)
	sc.Step(`^a Go helper "(\w+)" whose only failure is (.+)$`, func(name, failure string) error {
		d.helper, d.helperFailure = name, strings.ReplaceAll(failure, "…", "path, nil, 0o644")
		return nil
	})
	sc.Step(`^an example that calls (\w+) (once|twice|\d+ times) and an asserting helper (once|twice|\d+ times)$`, d.exampleCallingHelpers)
	sc.Step(`^it has (\d+) assertions?$`, func(want int) error {
		e, err := d.onlyExample()
		if err != nil {
			return err
		}
		if e.Assertions != want {
			return fmt.Errorf("%q has %d assertions, not %d", e.Name, e.Assertions, want)
		}
		return nil
	})
	sc.Step(`^a (TypeScript|Python|Go|Kotlin) example containing (.+)$`, d.exampleContaining)
	sc.Step(`^it counts as an assertion$`, func() error { return d.callCounts(true) })
	sc.Step(`^a test calls a helper named "([^"]*)" defined outside the file$`, func(name string) error {
		return d.writeFile("helper.test.ts", "it(\"uses a helper\", () => {\n  const result = parse(\"x\");\n  "+name+"(result);\n});\n")
	})
	sc.Step(`^the call (counts|does not count) as an assertion$`, func(counts string) error {
		return d.callCounts(counts == "counts")
	})
	sc.Step(`^a pytest example whose body is a (\d+)-line triple-quoted string and (one|\d+) assertions?$`, func(lines int, assertions string) error {
		count, err := countOf(assertions)
		if err != nil {
			return err
		}
		return d.writeFile("test_fixture.py", "def test_parses():\n    src = \"\"\"\n"+strings.Repeat("line\n", lines)+"\"\"\"\n"+
			strings.Repeat("    assert parse(src)\n", count))
	})
	sc.Step(`^it spans (\d+) raw lines but only (\d+) code lines$`, func(raw, code int) error {
		e, err := d.onlyExample()
		if err != nil {
			return err
		}
		if e.RawLines != raw || e.Lines != code {
			return fmt.Errorf("%q spans %d raw lines and %d code lines, not %d and %d", e.Name, e.RawLines, e.Lines, raw, code)
		}
		return nil
	})
	sc.Step(`^a test whose (\d+) cases are (.+)$`, d.caseTables)
	sc.Step(`^it is a table of a few code lines, not a large example$`, d.tablesAreSmall)
	sc.Step(`^scrap does not ask to split it$`, d.noSplit)
	sc.Step(`^an example with (\d+) code lines, (\d+) branches or loops, (\d+) mocks, and (\d+) assertions$`, func(lines, decisions, mocks, assertions int) error {
		d.stated = &scrapMetrics{Lines: lines, Decisions: decisions, Mocks: mocks, Assertions: assertions}
		source, err := tsExample("scored", *d.stated)
		if err != nil {
			return err
		}
		return d.writeFile("scored.test.ts", source)
	})
	sc.Step(`^its score is (\d+(?:\.\d+)?)$`, func(want float64) error {
		e, err := d.onlyExample()
		if err != nil {
			return err
		}
		if e.Score != want {
			return fmt.Errorf("%q scores %v, not %v", e.Name, e.Score, want)
		}
		return nil
	})
	sc.Step(`^an example with (no assertions|more than \d+ code lines|a branch or loop|\d+ or more mocks|more than \d+ assertions)$`, d.exampleWithMeasure)
	sc.Step(`^it has the smell "([^"]*)"$`, d.hasSmell)

	// Choosing an action.
	sc.Step(`^(?:its|the file's) action is ([A-Z_]+)$`, func(action string) error {
		r, err := d.its()
		if err != nil {
			return err
		}
		if r.Action != action {
			return fmt.Errorf("%s's action is %s, not %s", r.File, r.Action, action)
		}
		return nil
	})
	sc.Step(`^its pressure is (\d+(?:\.\d+)?)$`, func(want float64) error {
		r, err := d.its()
		if err != nil {
			return err
		}
		if r.Pressure != want {
			return fmt.Errorf("%s's pressure is %v, not %v", r.File, r.Pressure, want)
		}
		return nil
	})
	sc.Step(`^it has no recommendations$`, func() error {
		r, err := d.its()
		if err != nil {
			return err
		}
		if len(r.Recommendations) != 0 {
			return fmt.Errorf("%s has the recommendations %+v", r.File, r.Recommendations)
		}
		return nil
	})
	sc.Step(`^it has (one|two|three|\d+) "(\w+)" clusters? of (\d+) examples$`, d.hasClusters)
	sc.Step(`^the first recommendation says to fold them into "([^"]*)"$`, d.firstFolds)
	sc.Step(`^a (\w+) test file whose small examples differ only in their data$`, d.dataOnlyFile)
	sc.Step(`^the recommendation names (.+)$`, d.recommendationNames)
	sc.Step(`^its first recommendation is (HIGH|MEDIUM|LOW) confidence: (.+)$`, func(confidence, says string) error {
		r, err := d.its()
		if err != nil {
			return err
		}
		if len(r.Recommendations) == 0 {
			return fmt.Errorf("%s has no recommendation", r.File)
		}
		first := r.Recommendations[0]
		if first.Confidence != confidence || !saysInOrder(first.Text, says) {
			return fmt.Errorf("%s's first recommendation is %s %q, not %s: %s", r.File, first.Confidence, first.Text, confidence, says)
		}
		return nil
	})
	sc.Step(`^several large examples whose bodies are at least (\d+)% alike$`, d.alikeExamples)
	sc.Step(`^a (HIGH|MEDIUM|LOW) recommendation says to (.+)$`, func(confidence, says string) error {
		return d.aRecommendationSays(confidence, says)
	})
	sc.Step(`^a recommendation says to (.+)$`, func(says string) error { return d.aRecommendationSays("", says) })
	sc.Step(`^a test file with (\d+) examples, (\d+) of which set up (\d+) or more mocks$`, d.mockedFile)
	sc.Step(`^a test file with (\d+) examples$`, func(count int) error {
		d.examples = count
		return nil
	})
	sc.Step(`^(\d+) examples scoring (\d+) or more, spread across (\d+) groups$`, d.troubledFile)
	sc.Step(`^a test file with many problems$`, func() error { return d.writeFile("problems.test.ts", manyProblems()) })
	sc.Step(`^(\w+) recommendations come before (\w+) ones, and (\w+) before (\w+)$`, d.rankedRecommendations)
	sc.Step(`^at most (\d+) recommendations are listed$`, d.atMostListed)
	sc.Step(`^each names the example and its line range$`, d.eachNamesItsExample)

	// Output and progress over time.
	sc.Step(`^each file is printed as "([^"]*)"$`, d.eachFilePrintedAs)
	sc.Step(`^files are listed from the highest pressure down$`, d.highestPressureFirst)
	sc.Step(`^recommendations follow, indented, with their confidence$`, d.recommendationsFollow)
	sc.Step(`^a file whose action is ([A-Z_]+) but has a (HIGH|MEDIUM|LOW) hint$`, d.quietFile)
	sc.Step(`^the hint is not printed$`, func() error {
		hint, err := d.hint()
		if err != nil {
			return err
		}
		if strings.Contains(w.stdout, hint) {
			return fmt.Errorf("the hint %q is printed\n%s", hint, w.report())
		}
		return nil
	})
	sc.Step(`^it is printed with (--\w+)$`, d.printedWith)
	sc.Step(`^every example is printed with its (.+)$`, d.everyExamplePrinted)
	sc.Step(`^each file's report has (.+)$`, d.eachReportHas)
	sc.Step(`^\.metrics/scrap\.json recorded a pressure of (\d+(?:\.\d+)?) for a file$`, d.recordedPressure)
	sc.Step(`^I run "itos-cc ?([^"]*)" and its pressure is now (\d+(?:\.\d+)?)$`, d.runWithPressure)
	sc.Step(`^its verdict is (\w+)$`, d.verdictIs)
	sc.Step(`^a file's pressure rose since the previous run$`, func() error {
		if err := d.recordedPressure(3); err != nil {
			return err
		}
		after, err := pressureFixture(4)
		if err != nil {
			return err
		}
		d.after = after
		return d.w.write(d.file, after)
	})
	sc.Step(`^its line ends with "([^"]*)" and the previous pressure$`, d.lineEndsWith)
	sc.Step(`^a warning says to (.+) before keeping the change$`, d.warningSays)
	sc.Step(`^\.metrics/scrap\.json holds reports for (\S+) and (\S+)$`, d.holdsReports)
	sc.Step(`^\.metrics/scrap\.json still holds (\S+)'s report$`, d.stillHolds)
	sc.Step(`^\.metrics/scrap\.json is not valid JSON$`, func() error {
		if err := d.writeFile("test_ok.py", cleanPy); err != nil {
			return err
		}
		return w.write(".metrics/scrap.json", "{ this is not JSON\n")
	})
	sc.Step(`^stderr says it is ignoring the unreadable snapshot$`, func() error {
		if !strings.Contains(w.stderr, "ignoring unreadable .metrics/scrap.json") {
			return fmt.Errorf("stderr does not say it is ignoring the unreadable snapshot\n%s", w.report())
		}
		return nil
	})
	sc.Step(`^\.metrics/scrap\.json holds (\{.*\})$`, func(snapshot string) error {
		if err := d.writeFile("test_ok.py", cleanPy); err != nil {
			return err
		}
		return w.write(".metrics/scrap.json", snapshot+"\n")
	})
	// The whole line, so a reason that goes on to name a Go type fails.
	sc.Step(`^stderr says it is ignoring the snapshot because (.+)$`, func(reason string) error {
		line := "itos-cc: ignoring unreadable .metrics/scrap.json: " + reason
		if !slices.Contains(strings.Split(strings.ReplaceAll(w.stderr, "\r\n", "\n"), "\n"), line) {
			return fmt.Errorf("stderr has no line %q\n%s", line, w.report())
		}
		return nil
	})
	sc.Step(`^the run completes$`, d.runCompletes)
	sc.Step(`^the selection holds no test files$`, func() error {
		return d.writeFiles(map[string]string{"src/cart.ts": "export function total(xs: number[]) {\n  return xs.length;\n}\n", "README.md": "# notes\n"})
	})
}

// scrapMetrics are an example's measurements, as --json gives them.
type scrapMetrics struct {
	Lines      int `json:"lines"`
	RawLines   int `json:"raw_lines"`
	Decisions  int `json:"decisions"`
	Assertions int `json:"assertions"`
	Mocks      int `json:"mocks"`
}

type scrapExample struct {
	Name      string   `json:"name"`
	Group     string   `json:"group"`
	StartLine int      `json:"start_line"`
	EndLine   int      `json:"end_line"`
	Table     bool     `json:"table"`
	Score     float64  `json:"score"`
	Smells    []string `json:"smells"`
	scrapMetrics
}

type scrapRecommendation struct {
	Confidence string `json:"confidence"`
	Text       string `json:"text"`
}

type scrapCluster struct {
	Kind       string  `json:"kind"`
	Similarity float64 `json:"similarity"`
	Examples   []struct {
		Name      string `json:"name"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	} `json:"examples"`
}

// scrapReport is one file's report.
type scrapReport struct {
	File            string                `json:"file"`
	Language        string                `json:"language"`
	Action          string                `json:"action"`
	Pressure        float64               `json:"pressure"`
	Examples        int                   `json:"examples"`
	SetupLines      int                   `json:"setup_lines"`
	AverageScore    float64               `json:"average_score"`
	MaxScore        float64               `json:"max_score"`
	Recommendations []scrapRecommendation `json:"recommendations"`
	Clusters        []scrapCluster        `json:"clusters"`
	Details         []scrapExample        `json:"details"`
	Compare         *struct {
		Verdict          string  `json:"verdict"`
		PreviousPressure float64 `json:"previous_pressure"`
	} `json:"compare"`
}

// writeFile writes one test file of the scenario's, the one "it" names
// when it is the only one.
func (d *scrapState) writeFile(name, text string) error {
	if !slices.Contains(d.files, name) {
		d.files = append(d.files, name)
	}
	if len(d.files) == 1 {
		d.file = name
	}
	return d.w.write(name, text)
}

// writeFiles writes the scenario's files, in the order of their names.
func (d *scrapState) writeFiles(files map[string]string) error {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if err := d.writeFile(name, files[name]); err != nil {
			return err
		}
	}
	return nil
}

// scrapIn runs itos-cc scrap with --json and args in dir, and gives its
// reports by file, the run having succeeded.
func (d *scrapState) scrapIn(dir string, args ...string) (map[string]scrapReport, error) {
	args = append([]string{"scrap", "--json"}, args...)
	code, stdout, stderr, err := d.w.capture(dir, d.w.env(), args...)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("itos-cc %s exited %d\n%s%s", strings.Join(args, " "), code, stdout, stderr)
	}
	return reportsOf(stdout)
}

// reportsOf reads scrap's --json object, its reports by slash-separated
// file.
func reportsOf(stdout string) (map[string]scrapReport, error) {
	var object struct {
		Files []scrapReport `json:"files"`
	}
	if err := json.Unmarshal([]byte(stdout), &object); err != nil {
		return nil, fmt.Errorf("standard output is not scrap's object: %v\n%s", err, stdout)
	}
	reports := map[string]scrapReport{}
	for _, r := range object.Files {
		r.File = filepath.ToSlash(r.File)
		reports[r.File] = r
	}
	return reports, nil
}

// measure runs scrap --json --verbose over the scenario's files, or files,
// as the scenario's run, and keeps its reports.
func (d *scrapState) measure(files ...string) error {
	w := d.w
	if len(files) == 0 {
		files = d.files
	}
	if len(files) == 0 {
		return errors.New("no test file to measure")
	}
	if err := w.runEnv(w.dir, w.env(), append([]string{"scrap", "--json", "--verbose"}, files...)...); err != nil {
		return err
	}
	if w.exit != 0 {
		return fmt.Errorf("itos-cc exited %d\n%s", w.exit, w.report())
	}
	var err error
	d.measured, err = reportsOf(w.stdout)
	return err
}

// measuredReport is the measured report of file, measuring the scenario's
// files first where nothing was measured: a Then with no When.
func (d *scrapState) measuredReport(file string) (scrapReport, error) {
	if d.measured == nil {
		if err := d.measure(); err != nil {
			return scrapReport{}, err
		}
	}
	r, ok := d.measured[file]
	if !ok {
		return r, fmt.Errorf("%s has no report: no example is found in it\n%s", file, d.w.report())
	}
	return r, nil
}

// onlyExample is the one example of the scenario's file.
func (d *scrapState) onlyExample() (scrapExample, error) {
	r, err := d.measuredReport(d.file)
	if err != nil {
		return scrapExample{}, err
	}
	if len(r.Details) != 1 {
		return scrapExample{}, fmt.Errorf("%s has %d examples, not one: %+v", d.file, len(r.Details), r.Details)
	}
	return r.Details[0], nil
}

// countOf is a count a sentence writes as a word or a number.
func countOf(word string) (int, error) {
	if n, ok := map[string]int{"one": 1, "once": 1, "two": 2, "twice": 2, "three": 3}[word]; ok {
		return n, nil
	}
	n, err := strconv.Atoi(strings.TrimSuffix(word, " times"))
	if err != nil {
		return 0, fmt.Errorf("%q is no count", word)
	}
	return n, nil
}
