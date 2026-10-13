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
		if filepath.Base(s.Uri) != "scrap.feature" || len(s.Steps) == 0 || !strings.HasPrefix(s.Steps[0].Text, `I run "itos-cc`) {
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
	sc.Step(`^it is scored$`, d.scored)
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

// --- Finding examples ---

// enclosure is a construct around an example, of a kind a sentence names:
// a describe block, a test class, a parent test.
type enclosure struct{ kind, name string }

// grouped is an example of a fixture and what encloses it, outermost first.
type grouped struct {
	name string
	in   []enclosure
}

// groupFixture is a test file of one framework and the examples it holds.
type groupFixture struct {
	file, source string
	examples     []grouped
}

// tsGrouped is a TypeScript test file of a framework, its import header
// and the words it writes its examples with: an example alone, describe,
// context and suite blocks around others, and one in a group call, a block
// no framework here groups by.
func tsGrouped(file, header, alone, inner string) groupFixture {
	source := header + "\n" +
		alone + `("stands alone", () => { expect(one()).toBe(1); });

describe("cart", () => {
  ` + inner + `("adds", () => { expect(add(1)).toBe(1); });
  context("when empty", () => {
    ` + inner + `("is zero", () => { expect(size()).toBe(0); });
  });
});

suite("totals", () => {
  ` + inner + `("sums", () => { expect(sum([1])).toBe(1); });
});

group("helpers", () => {
  ` + inner + `("formats", () => { expect(fmt(1)).toBe("1"); });
});
`
	cart := enclosure{"describe", "cart"}
	return groupFixture{file, source, []grouped{
		{"stands alone", nil},
		{"adds", []enclosure{cart}},
		{"is zero", []enclosure{cart, {"context", "when empty"}}},
		{"sums", []enclosure{{"suite", "totals"}}},
		{"formats", []enclosure{{"group", "helpers"}}},
	}}
}

// groupFixtures are the test files of each framework a scenario names.
var groupFixtures = map[string]map[string]groupFixture{
	"TypeScript": {
		"Vitest": tsGrouped("vitest.test.ts", `import { describe, it, test, expect } from "vitest";`+"\n", "test", "it"),
		"Jest":   tsGrouped("jest.test.ts", `import { describe, test, expect } from "@jest/globals";`+"\n", "test", "test"),
		"Mocha":  tsGrouped("mocha.test.ts", `import { expect } from "chai";`+"\n", "specify", "it"),
	},
	"Python": {
		"pytest": {"test_pytest_style.py", `def test_stands_alone():
    assert one() == 1


class TestCart:
    def test_adds(self):
        assert add(1) == 1

    class TestWhenEmpty:
        def test_is_zero(self):
            assert size() == 0
`, []grouped{
			{"test_stands_alone", nil},
			{"test_adds", []enclosure{{"class", "TestCart"}}},
			{"test_is_zero", []enclosure{{"class", "TestCart"}, {"class", "TestWhenEmpty"}}},
		}},
		"unittest": {"test_unittest_style.py", `import unittest


class TestParse(unittest.TestCase):
    def test_parses(self):
        self.assertEqual(parse("1"), 1)
`, []grouped{{"test_parses", []enclosure{{"class", "TestParse"}}}}},
	},
	"Go": {
		"testing": {"cart_test.go", `package cart

import "testing"

func TestStandsAlone(t *testing.T) {
    if One() != 1 {
        t.Fatal("one")
    }
}

func TestCart(t *testing.T) {
    t.Run("adds", func(t *testing.T) {
        if Add(1) != 1 {
            t.Fatal("add")
        }
    })
    t.Run("removes", func(t *testing.T) {
        if Remove(1) != 0 {
            t.Fatal("remove")
        }
    })
}
`, []grouped{
			{"TestStandsAlone", nil},
			{"adds", []enclosure{{"parent test", "TestCart"}}},
			{"removes", []enclosure{{"parent test", "TestCart"}}},
		}},
	},
	"Kotlin": {
		"JUnit": {"CartTest.kt", `class CartTest {
    @Test
    fun adds() {
        assertEquals(1, add(1))
    }

    @Nested
    inner class WhenEmpty {
        @Test
        fun isZero() {
            assertEquals(0, size())
        }
    }
}
`, []grouped{
			{"adds", []enclosure{{"class", "CartTest"}}},
			{"isZero", []enclosure{{"class", "CartTest"}, {"class", "WhenEmpty"}}},
		}},
		"kotest": {"CartSpec.kt", `class CartSpec : DescribeSpec({
    describe("cart") {
        it("adds") { add(1) shouldBe 1 }
        context("when empty") {
            it("is zero") { size() shouldBe 0 }
        }
    }
})

class TotalsSpec : FeatureSpec({
    feature("totals") {
        scenario("sums") { sum(listOf(1)) shouldBe 1 }
    }
})

class PriceSpec : BehaviorSpec({
    given("a price") {
        then("it formats") { fmt(1) shouldBe "1" }
    }
})

class AloneSpec : FunSpec({
    test("stands alone") { one() shouldBe 1 }
})
`, []grouped{
			{"adds", []enclosure{{"class", "CartSpec"}, {"describe", "cart"}}},
			{"is zero", []enclosure{{"class", "CartSpec"}, {"describe", "cart"}, {"context", "when empty"}}},
			{"sums", []enclosure{{"class", "TotalsSpec"}, {"feature", "totals"}}},
			{"it formats", []enclosure{{"class", "PriceSpec"}, {"given", "a price"}}},
			{"stands alone", []enclosure{{"class", "AloneSpec"}}},
		}},
	},
}

// listItems are the items of a sentence's list: "a, b, or c", "a and b".
func listItems(list string) []string {
	list = strings.NewReplacer(", or ", ", ", ", and ", ", ", " or ", ", ", " and ", ", ").Replace(list)
	return strings.Split(list, ", ")
}

// testFileUsing writes the test files of each framework the list names, in
// the language.
func (d *scrapState) testFileUsing(language, frameworks string) error {
	d.expected = map[string][]grouped{}
	for _, framework := range listItems(frameworks) {
		f, ok := groupFixtures[language][framework]
		if !ok {
			return fmt.Errorf("no %s test file is written with %s", language, framework)
		}
		if err := d.writeFile(f.file, f.source); err != nil {
			return err
		}
		d.expected[f.file] = f.examples
	}
	return nil
}

// enclosingKinds are the kinds of construct a sentence names as an
// example's group: "describe, context, or suite block", "test class",
// "parent test, for t.Run subtests", "test class, or kotest describe …".
func enclosingKinds(phrase string) ([]string, error) {
	var kinds []string
	for _, item := range listItems(phrase) {
		item = strings.TrimSuffix(strings.TrimPrefix(item, "kotest "), " block")
		switch {
		case item == "test class":
			kinds = append(kinds, "class")
		case item == "for t.Run subtests":
			if !slices.Contains(kinds, "parent test") {
				return nil, fmt.Errorf("%q follows no parent test", item)
			}
		case strings.HasPrefix(item, "for "):
			return nil, fmt.Errorf("no fixture groups %q", item)
		default:
			kinds = append(kinds, item)
		}
	}
	return kinds, nil
}

// groupedBy is whether the examples of each file are exactly the fixture's,
// each grouped by the constructs around it of the kinds named, outermost
// first and joined by " › ", and by no other. The fixtures must hold a
// construct of each kind named.
func (d *scrapState) groupedBy(phrase string) error {
	kinds, err := enclosingKinds(phrase)
	if err != nil {
		return err
	}
	for _, kind := range kinds {
		found := false
		for _, examples := range d.expected {
			for _, e := range examples {
				found = found || slices.ContainsFunc(e.in, func(x enclosure) bool { return x.kind == kind })
			}
		}
		if !found {
			return fmt.Errorf("the fixtures hold no %s, so the step cannot fail", kind)
		}
	}
	if d.w.exit != 0 {
		return fmt.Errorf("itos-cc exited %d\n%s", d.w.exit, d.w.report())
	}
	reports, err := d.scrapIn(d.w.dir)
	if err != nil {
		return err
	}
	for file, examples := range d.expected {
		var want, got []string
		for _, e := range examples {
			var names []string
			for _, x := range e.in {
				if slices.Contains(kinds, x.kind) {
					names = append(names, x.name)
				}
			}
			want = append(want, strings.Join(names, " › ")+"/"+e.name)
		}
		for _, e := range reports[file].Details {
			got = append(got, e.Group+"/"+e.Name)
		}
		if !slices.Equal(got, want) {
			return fmt.Errorf("the examples of %s are\n  %s\nnot\n  %s", file, strings.Join(got, "\n  "), strings.Join(want, "\n  "))
		}
		if !slices.ContainsFunc(outputLines(filepath.ToSlash(d.w.stdout)), func(line string) bool {
			return strings.HasPrefix(line, file+"  ") && strings.Contains(line, fmt.Sprintf("  examples=%d  ", len(examples)))
		}) {
			return fmt.Errorf("the run prints no line of %s with its %d examples\n%s", file, len(examples), d.w.report())
		}
	}
	return nil
}

// goTableTest is a Go test that loops over its cases, asserting in the
// loop's body.
const goTableTest = `package a

func TestInc(t *testing.T) {
    cases := []struct{ in, want int }{
        {1, 2},
        {2, 3},
    }
    for _, c := range cases {
        if got := inc(c.in); got != c.want {
            t.Errorf("inc(%d) = %d", c.in, got)
        }
    }
}
`

// setupFixture is a test file with shared setup of setupLines lines, and
// the same file without it.
type setupFixture struct {
	file, with, without string
	setupLines          int
}

var setupFixtures = map[string]setupFixture{
	"beforeEach": {"cart.test.ts", `import { beforeEach, it, expect } from "vitest";

let cart;
beforeEach(() => {
  cart = new Cart();
});

it("adds", () => {
  expect(cart.add(1)).toBe(1);
});
`, `import { beforeEach, it, expect } from "vitest";

let cart;

it("adds", () => {
  expect(cart.add(1)).toBe(1);
});
`, 3},
	"fixtures": {"test_fixture_cart.py", `import pytest

@pytest.fixture
def cart():
    return Cart()

def test_adds(cart):
    assert cart.add(1) == 1
`, `import pytest

def test_adds(cart):
    assert cart.add(1) == 1
`, 3},
	"setUp": {"test_setup_cart.py", `import unittest

class TestCart(unittest.TestCase):
    def setUp(self):
        self.cart = Cart()

    def test_adds(self):
        self.assertEqual(self.cart.add(1), 1)
`, `import unittest

class TestCart(unittest.TestCase):
    def test_adds(self):
        self.assertEqual(self.cart.add(1), 1)
`, 2},
	"@BeforeEach": {"CartTest.kt", `class CartTest {
    @BeforeEach
    fun setUp() {
        cart = Cart()
    }

    @Test
    fun adds() {
        assertEquals(1, cart.add(1))
    }
}
`, `class CartTest {
    @Test
    fun adds() {
        assertEquals(1, cart.add(1))
    }
}
`, 4},
}

// testFileWithSetup writes a test file with each kind of setup the list
// names.
func (d *scrapState) testFileWithSetup(list string) error {
	for _, kind := range listItems(list) {
		f, ok := setupFixtures[kind]
		if !ok {
			return fmt.Errorf("no test file is written with %s", kind)
		}
		d.setups = append(d.setups, f)
		if err := d.writeFile(f.file, f.with); err != nil {
			return err
		}
	}
	return nil
}

// setupNotSize is whether each file's setup lines are its setup's, and its
// examples measure as they do in the same file without the setup, which
// has none.
func (d *scrapState) setupNotSize() error {
	without := filepath.Join(d.w.dir, "without")
	for _, f := range d.setups {
		if err := d.w.writeIn(without, f.file, f.without); err != nil {
			return err
		}
	}
	bare, err := d.scrapIn(without, "--verbose")
	if err != nil {
		return err
	}
	for _, f := range d.setups {
		r, err := d.measuredReport(f.file)
		if err != nil {
			return err
		}
		if r.SetupLines != f.setupLines {
			return fmt.Errorf("%s counts %d setup lines, not %d", f.file, r.SetupLines, f.setupLines)
		}
		b := bare[f.file]
		if b.SetupLines != 0 {
			return fmt.Errorf("%s without its setup counts %d setup lines", f.file, b.SetupLines)
		}
		if len(r.Details) == 0 || len(r.Details) != len(b.Details) {
			return fmt.Errorf("%s has examples %+v, and without its setup %+v", f.file, r.Details, b.Details)
		}
		for i, e := range r.Details {
			if e.Lines != b.Details[i].Lines || e.Score != b.Details[i].Score {
				return fmt.Errorf("%q of %s is %d lines scoring %v, and without the setup %d scoring %v",
					e.Name, f.file, e.Lines, e.Score, b.Details[i].Lines, b.Details[i].Score)
			}
		}
	}
	return nil
}

// kotlinMethod writes the Kotlin class of the scenario, annotated as it
// says, with a @Test method making count assertEquals.
func (d *scrapState) kotlinMethod(method, count string) error {
	if d.kotlinClass == "" {
		return errors.New("no Kotlin class to put the method in")
	}
	n, err := countOf(count)
	if err != nil {
		return err
	}
	var source strings.Builder
	source.WriteString("package demo\n\n")
	for _, a := range d.annotations {
		source.WriteString(a + "\n")
	}
	source.WriteString("class " + d.kotlinClass + " {\n    @Test\n    fun " + method + "() {\n")
	for i := range n {
		fmt.Fprintf(&source, "        assertEquals(%d, list().size)\n", i)
	}
	source.WriteString("    }\n}\n")
	return d.writeFile(d.file, source.String())
}

// isAnExampleWith is whether the file's examples include group/name with
// that many assertions.
func (d *scrapState) isAnExampleWith(group, name string, assertions int) error {
	r, err := d.measuredReport(d.file)
	if err != nil {
		return err
	}
	for _, e := range r.Details {
		if e.Group == group && e.Name == name {
			if e.Assertions != assertions {
				return fmt.Errorf("%s/%s has %d assertions, not %d", group, name, e.Assertions, assertions)
			}
			return nil
		}
	}
	return fmt.Errorf("%s/%s is no example of %s: %+v", group, name, d.file, r.Details)
}

// --- Measuring examples ---

// examplesAre is whether the file's examples are the table's, in order:
// each row's name and every other column the table has.
func (d *scrapState) examplesAre(table *godog.Table) error {
	r, err := d.measuredReport(d.file)
	if err != nil {
		return err
	}
	header := table.Rows[0].Cells
	var want, got []string
	for _, row := range table.Rows[1:] {
		var cells []string
		for _, c := range row.Cells {
			cells = append(cells, c.Value)
		}
		want = append(want, strings.Join(cells, " | "))
	}
	for _, e := range r.Details {
		var cells []string
		for _, h := range header {
			switch h.Value {
			case "example":
				cells = append(cells, e.Name)
			case "assertions":
				cells = append(cells, strconv.Itoa(e.Assertions))
			case "decisions":
				cells = append(cells, strconv.Itoa(e.Decisions))
			case "mocks":
				cells = append(cells, strconv.Itoa(e.Mocks))
			case "lines":
				cells = append(cells, strconv.Itoa(e.Lines))
			case "table":
				cells = append(cells, map[bool]string{true: "yes", false: "no"}[e.Table])
			default:
				return fmt.Errorf("the table has a column %q no step reads", h.Value)
			}
		}
		got = append(got, strings.Join(cells, " | "))
	}
	if !slices.Equal(got, want) {
		return fmt.Errorf("the examples are\n  %s\nnot\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	return nil
}

// exampleCallingHelpers writes the Go file of the scenario: its helper,
// failing only as the scenario says, an asserting helper, and a test
// calling each as often as it says.
func (d *scrapState) exampleCallingHelpers(helper, times, asserting string) error {
	if helper != d.helper {
		return fmt.Errorf("no helper %q was written", helper)
	}
	calls, err := countOf(times)
	if err != nil {
		return err
	}
	checks, err := countOf(asserting)
	if err != nil {
		return err
	}
	var test strings.Builder
	test.WriteString("func TestParse(t *testing.T) {\n")
	for i := range calls {
		fmt.Fprintf(&test, "    %s(t, \"%c\")\n", helper, 'a'+i)
	}
	for range checks {
		test.WriteString("    same(t, parse(\"a\"), 1)\n")
	}
	test.WriteString("}\n")
	return d.writeFile("helpers_test.go", "package a\n\nfunc "+helper+"(t *testing.T, path string) {\n    "+d.helperFailure+"\n}\n\n"+
		"func same(t *testing.T, got, want int) {\n    if got != want {\n        t.Errorf(\"got %d, want %d\", got, want)\n    }\n}\n\n"+test.String())
}

// exampleContaining writes one example of the language holding the code
// alone, a block it opens given a body.
func (d *scrapState) exampleContaining(language, code string) error {
	switch language {
	case "TypeScript":
		return d.writeFile("checks.test.ts", "it(\"checks\", () => {\n  "+code+";\n});\n")
	case "Python":
		if strings.HasSuffix(code, ":") {
			code += "\n        parse(\"x\")"
		}
		return d.writeFile("test_checks.py", "def test_checks():\n    "+code+"\n")
	case "Go":
		return d.writeFile("checks_test.go", "package a\n\nfunc TestChecks(t *testing.T) {\n    "+code+"\n}\n")
	case "Kotlin":
		return d.writeFile("ChecksTest.kt", "class ChecksTest {\n    @Test\n    fun checks() {\n        "+code+"\n    }\n}\n")
	}
	return fmt.Errorf("no example is written in %s", language)
}

// callCounts is whether the one example's call is counted as an
// assertion, its only one, or not at all.
func (d *scrapState) callCounts(counts bool) error {
	e, err := d.onlyExample()
	if err != nil {
		return err
	}
	if want := map[bool]int{true: 1, false: 0}[counts]; e.Assertions != want {
		return fmt.Errorf("%q has %d assertions, not %d", e.Name, e.Assertions, want)
	}
	return nil
}

// caseTables are, for each way a scenario says a test's cases are written,
// a test of each form it names over count cases, by file.
func caseTables(count int) map[string]map[string]string {
	rows := func(row string) string { return strings.Repeat(row+"\n", count) }
	return map[string]map[string]string{
		"a Go []struct literal it ranges over, inline or named, with or without t.Run": {
			"inline_test.go": "package a\n\nfunc TestInc(t *testing.T) {\n    for _, c := range []struct{ in, want int }{\n" +
				rows("        {1, 2},") + "    } {\n        if got := inc(c.in); got != c.want {\n            t.Errorf(\"inc(%d) = %d\", c.in, got)\n        }\n    }\n}\n",
			"inline_run_test.go": "package a\n\nfunc TestInc(t *testing.T) {\n    for _, c := range []struct{ in, want int }{\n" +
				rows("        {1, 2},") + "    } {\n        t.Run(\"\", func(t *testing.T) {\n            if inc(c.in) != c.want {\n                t.Fail()\n            }\n        })\n    }\n}\n",
			"named_test.go": "package a\n\nfunc TestInc(t *testing.T) {\n    tests := []struct {\n        in, want int\n    }{\n" +
				rows("        {1, 2},") + "    }\n    for _, tt := range tests {\n        if got := inc(tt.in); got != tt.want {\n            t.Errorf(\"inc(%d) = %d\", tt.in, got)\n        }\n    }\n}\n",
			"named_run_test.go": "package a\n\nfunc TestInc(t *testing.T) {\n    tests := []struct {\n        in, want int\n    }{\n" +
				rows("        {1, 2},") + "    }\n    for _, tt := range tests {\n        t.Run(\"\", func(t *testing.T) {\n            if inc(tt.in) != tt.want {\n                t.Fail()\n            }\n        })\n    }\n}\n",
		},
		"a Python list it loops over, or a parametrize decorator": {
			"test_loop.py":        "def test_inc():\n    cases = [\n" + rows("        (1, 2),") + "    ]\n    for a, b in cases:\n        assert inc(a) == b\n",
			"test_parametrize.py": "@pytest.mark.parametrize(\"a,b\", [\n" + rows("    (1, 2),") + "])\ndef test_inc(a, b):\n    assert inc(a) == b\n",
		},
		"a TypeScript array it loops over with for…of, or it.each's argument": {
			"loop.test.ts": "it(\"incs\", () => {\n  const cases = [\n" + rows("    [1, 2],") + "  ];\n  for (const [a, b] of cases) {\n    expect(inc(a)).toBe(b);\n  }\n});\n",
			"each.test.ts": "it.each([\n" + rows("  [1, 2],") + "])(\"incs %i\", (a, b) => {\n  expect(inc(a)).toBe(b);\n});\n",
		},
		"a Kotlin listOf it loops over, or a @CsvSource": {
			"LoopTest.kt": "class LoopTest {\n  @Test fun inc() {\n    val cases = listOf(\n" + rows("      1 to 2,") + "    )\n    for ((a, b) in cases) { assertEquals(b, inc(a)) }\n  }\n}\n",
			"CsvTest.kt":  "class CsvTest {\n  @ParameterizedTest\n  @CsvSource(\n" + rows("    \"1, 2\",") + "  )\n  fun inc(a: Int, b: Int) { assertEquals(b, inc(a)) }\n}\n",
		},
	}
}

// caseTables writes a test of each form the phrase names, over count cases.
func (d *scrapState) caseTables(count int, phrase string) error {
	files, ok := caseTables(count)[phrase]
	if !ok {
		return fmt.Errorf("no test is written whose cases are %s", phrase)
	}
	return d.writeFiles(files)
}

// fewLines is the most code lines a table of cases is "a few" of: what a
// screenful's quarter holds.
const fewLines = 12

// tablesAreSmall is whether each file's one example is a table of a few
// code lines, though its raw lines hold every case, and is not large.
func (d *scrapState) tablesAreSmall() error {
	for _, file := range d.files {
		r, err := d.measuredReport(file)
		if err != nil {
			return err
		}
		if len(r.Details) != 1 {
			return fmt.Errorf("%s has %d examples, not one", file, len(r.Details))
		}
		e := r.Details[0]
		if !e.Table || e.Lines > fewLines || e.RawLines <= fewLines || slices.Contains(e.Smells, "large") {
			return fmt.Errorf("%q of %s: table %v, %d code lines of %d raw, smells %v", e.Name, file, e.Table, e.Lines, e.RawLines, e.Smells)
		}
	}
	return nil
}

// noSplit is whether no file's action or recommendation asks to split it.
func (d *scrapState) noSplit() error {
	for _, file := range d.files {
		r, err := d.measuredReport(file)
		if err != nil {
			return err
		}
		if r.Action == "MANUAL_SPLIT" {
			return fmt.Errorf("%s's action is MANUAL_SPLIT", file)
		}
		for _, rec := range r.Recommendations {
			if strings.Contains(rec.Text, "split") {
				return fmt.Errorf("%s is asked to split: %s", file, rec.Text)
			}
		}
	}
	return nil
}

// tsExample is a TypeScript example of m's code lines, decisions, mocks and
// assertions, each one line, the rest calls that are none of them.
func tsExample(name string, m scrapMetrics) (string, error) {
	pad := m.Lines - 2 - m.Decisions - m.Mocks - m.Assertions
	if pad < 0 {
		return "", fmt.Errorf("%d lines hold no example of %d decisions, %d mocks and %d assertions", m.Lines, m.Decisions, m.Mocks, m.Assertions)
	}
	var body strings.Builder
	fmt.Fprintf(&body, "it(%q, () => {\n", name)
	for i := range m.Decisions {
		fmt.Fprintf(&body, "  if (flag%d) { run%d(); }\n", i, i)
	}
	for i := range m.Mocks {
		fmt.Fprintf(&body, "  const mock%d = vi.fn();\n", i)
	}
	for i := range m.Assertions {
		fmt.Fprintf(&body, "  expect(value%d).toBe(%d);\n", i, i)
	}
	for i := range pad {
		fmt.Fprintf(&body, "  step%d();\n", i)
	}
	body.WriteString("});\n")
	return body.String(), nil
}

// scored measures the example and checks it has what the scenario says.
func (d *scrapState) scored() error {
	if err := d.measure(); err != nil {
		return err
	}
	e, err := d.onlyExample()
	if err != nil {
		return err
	}
	if s := d.stated; s != nil && (e.Lines != s.Lines || e.Decisions != s.Decisions || e.Mocks != s.Mocks || e.Assertions != s.Assertions) {
		return fmt.Errorf("the example measures %+v, not %+v", e.scrapMetrics, *s)
	}
	return nil
}

// measureSmells are the measures a scenario gives an example a smell by:
// the example with the measure, and one just short of it.
var measureSmells = []struct {
	measure       *regexp.Regexp
	with, without func(n int) scrapMetrics
}{
	{regexp.MustCompile(`^no assertions$`),
		func(int) scrapMetrics { return scrapMetrics{Lines: 5} },
		func(int) scrapMetrics { return scrapMetrics{Lines: 5, Assertions: 1} }},
	{regexp.MustCompile(`^more than (\d+) code lines$`),
		func(n int) scrapMetrics { return scrapMetrics{Lines: n + 1, Assertions: 1} },
		func(n int) scrapMetrics { return scrapMetrics{Lines: n, Assertions: 1} }},
	{regexp.MustCompile(`^a branch or loop$`),
		func(int) scrapMetrics { return scrapMetrics{Lines: 5, Decisions: 1, Assertions: 1} },
		func(int) scrapMetrics { return scrapMetrics{Lines: 5, Assertions: 1} }},
	{regexp.MustCompile(`^(\d+) or more mocks$`),
		func(n int) scrapMetrics { return scrapMetrics{Lines: n + 3, Mocks: n, Assertions: 1} },
		func(n int) scrapMetrics { return scrapMetrics{Lines: n + 3, Mocks: n - 1, Assertions: 1} }},
	{regexp.MustCompile(`^more than (\d+) assertions$`),
		func(n int) scrapMetrics { return scrapMetrics{Lines: n + 3, Assertions: n + 1} },
		func(n int) scrapMetrics { return scrapMetrics{Lines: n + 3, Assertions: n} }},
}

// exampleWithMeasure writes an example with the measure, "with", and one
// just short of it, "without", each in a file of its own.
func (d *scrapState) exampleWithMeasure(measure string) error {
	for _, s := range measureSmells {
		m := s.measure.FindStringSubmatch(measure)
		if m == nil {
			continue
		}
		n := 0
		if len(m) > 1 {
			n, _ = strconv.Atoi(m[1])
		}
		with, err := tsExample("with", s.with(n))
		if err != nil {
			return err
		}
		without, err := tsExample("without", s.without(n))
		if err != nil {
			return err
		}
		if err := d.writeFile("with.test.ts", with); err != nil {
			return err
		}
		return d.writeFile("without.test.ts", without)
	}
	return fmt.Errorf("no example is written with %s", measure)
}

// hasSmell is whether the example with the measure has the smell and the
// one just short of it does not.
func (d *scrapState) hasSmell(smell string) error {
	for file, want := range map[string]bool{"with.test.ts": true, "without.test.ts": false} {
		r, err := d.measuredReport(file)
		if err != nil {
			return err
		}
		if len(r.Details) != 1 {
			return fmt.Errorf("%s has %d examples, not one", file, len(r.Details))
		}
		if e := r.Details[0]; slices.Contains(e.Smells, smell) != want {
			return fmt.Errorf("%q, %+v, has the smells %v: %q is %v there", e.Name, e.scrapMetrics, e.Smells, smell, want)
		}
	}
	return nil
}

// --- Choosing an action ---

// The text output, line by line: a file's line, its warning, its
// recommendations and, with --verbose, its examples.
var (
	fileLine = regexp.MustCompile(`^(\S+)  (LEAVE_ALONE|AUTO_TABLE_DRIVE|AUTO_REFACTOR|MANUAL_SPLIT|REVIEW_FIRST)  ` +
		`pressure=(\d+\.\d)  examples=(\d+)  avg=(\d+\.\d)  max=(\d+\.\d)(?:  (improved|worse) from (\d+\.\d))?$`)
	recommendationLine = regexp.MustCompile(`^  (HIGH  |MEDIUM|LOW   ) (.+)$`)
	exampleLine        = regexp.MustCompile(`^    ( *\d+\.\d)  (\S+):(\d+)-(\d+) (".*") lines=(\d+) assertions=(\d+) decisions=(\d+) mocks=(\d+) \[(.*)\]$`)
)

// printedFile is what the text output prints of one file.
type printedFile struct {
	fields          []string // fileLine's submatches
	line            string
	warnings        []string
	recommendations []scrapRecommendation
	examples        [][]string // exampleLine's submatches
}

// printed reads the text output, every line a file's or one under it.
func printed(stdout string) ([]printedFile, error) {
	var files []printedFile
	for _, line := range outputLines(filepath.ToSlash(stdout)) {
		line = strings.TrimSuffix(line, "\r")
		if m := fileLine.FindStringSubmatch(line); m != nil {
			files = append(files, printedFile{fields: m, line: line})
			continue
		}
		if line == "" && len(files) == 0 {
			continue
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("%q comes before any file's line", line)
		}
		f := &files[len(files)-1]
		switch m := recommendationLine.FindStringSubmatch(line); {
		case m != nil:
			f.recommendations = append(f.recommendations, scrapRecommendation{strings.TrimSpace(m[1]), m[2]})
		case exampleLine.MatchString(line):
			f.examples = append(f.examples, exampleLine.FindStringSubmatch(line))
		case strings.HasPrefix(line, "  the last change"):
			f.warnings = append(f.warnings, line)
		default:
			return nil, fmt.Errorf("%q is no line of a file's report", line)
		}
	}
	return files, nil
}

// reported are the reports of the scenario's run by file: from its --json
// object, or with the text output from the .metrics/scrap.json it wrote,
// the text printing a line of each with its action, pressure, examples and
// scores.
func (d *scrapState) reported() (map[string]scrapReport, error) {
	w := d.w
	if w.last == nil {
		return nil, errors.New("itos-cc scrap was not run")
	}
	if d.reportedRun != w.last {
		d.reportedRun = w.last
		d.reportedFiles, d.reportedErr = d.readReported()
	}
	return d.reportedFiles, d.reportedErr
}

func (d *scrapState) readReported() (map[string]scrapReport, error) {
	w := d.w
	if w.exit != 0 {
		return nil, fmt.Errorf("itos-cc exited %d\n%s", w.exit, w.report())
	}
	if strings.HasPrefix(strings.TrimSpace(w.stdout), "{") {
		return reportsOf(w.stdout)
	}
	snapshot, err := d.snapshot()
	if err != nil {
		return nil, err
	}
	files, err := printed(w.stdout)
	if err != nil {
		return nil, fmt.Errorf("%v\n%s", err, w.report())
	}
	reports := map[string]scrapReport{}
	for _, f := range files {
		r, ok := snapshot[f.fields[1]]
		if !ok {
			return nil, fmt.Errorf("%s is printed, and .metrics/scrap.json has no report of it", f.fields[1])
		}
		want := []string{r.Action, fmt.Sprintf("%.1f", r.Pressure), strconv.Itoa(r.Examples), fmt.Sprintf("%.1f", r.AverageScore), fmt.Sprintf("%.1f", r.MaxScore)}
		if !slices.Equal(f.fields[2:7], want) {
			return nil, fmt.Errorf("%q is printed, and .metrics/scrap.json holds %v of it", f.line, want)
		}
		reports[r.File] = r
	}
	return reports, nil
}

// snapshot is what .metrics/scrap.json holds, by file.
func (d *scrapState) snapshot() (map[string]scrapReport, error) {
	raw, err := d.w.readIn(d.w.dir, ".metrics/scrap.json")
	if err != nil {
		return nil, fmt.Errorf("no .metrics/scrap.json: %v", err)
	}
	return reportsOf(string(raw))
}

// its is the report of the scenario's file, or of the one file the run
// reported.
func (d *scrapState) its() (scrapReport, error) {
	reports, err := d.reported()
	if err != nil {
		return scrapReport{}, err
	}
	if d.file != "" {
		r, ok := reports[d.file]
		if !ok {
			return r, fmt.Errorf("the run reports nothing of %s\n%s", d.file, d.w.report())
		}
		return r, nil
	}
	if len(reports) != 1 {
		return scrapReport{}, fmt.Errorf("the run reports %d files, not one\n%s", len(reports), d.w.report())
	}
	for _, r := range reports {
		return r, nil
	}
	return scrapReport{}, nil
}

// hasClusters is whether the file has count clusters of the kind, each of
// that many examples.
func (d *scrapState) hasClusters(count, kind string, examples int) error {
	r, err := d.its()
	if err != nil {
		return err
	}
	n, err := countOf(count)
	if err != nil {
		return err
	}
	var sizes []int
	for _, c := range r.Clusters {
		if c.Kind == kind {
			sizes = append(sizes, len(c.Examples))
		}
	}
	if len(sizes) != n || slices.ContainsFunc(sizes, func(s int) bool { return s != examples }) {
		return fmt.Errorf("%s has %q clusters of %v examples, not %d of %d: %+v", r.File, kind, sizes, n, examples, r.Clusters)
	}
	return nil
}

// firstFolds is whether the first recommendation folds the examples of the
// file's table cluster, naming each, into the idiom.
func (d *scrapState) firstFolds(idiom string) error {
	r, err := d.its()
	if err != nil {
		return err
	}
	if len(r.Recommendations) == 0 {
		return fmt.Errorf("%s has no recommendation", r.File)
	}
	first := r.Recommendations[0].Text
	if !strings.Contains(first, "fold them into "+idiom) {
		return fmt.Errorf("the first recommendation %q does not say to fold them into %s", first, idiom)
	}
	for _, c := range r.Clusters {
		if c.Kind != "table" {
			continue
		}
		for _, e := range c.Examples {
			if !strings.Contains(first, strconv.Quote(e.Name)) {
				return fmt.Errorf("the first recommendation %q does not name %q", first, e.Name)
			}
		}
		return nil
	}
	return fmt.Errorf("%s has no table cluster", r.File)
}

// dataOnly are, by language, four small examples that differ only in their
// data.
var dataOnly = map[string][2]string{
	"TypeScript": {"parse.test.ts", `it("parses one", () => { expect(parse("1")).toBe(1); });
it("parses two", () => { expect(parse("2")).toBe(2); });
it("parses ten", () => { expect(parse("10")).toBe(10); });
it("parses neg", () => { expect(parse("-4")).toBe(-4); });
`},
	"Python": {"test_parse.py", `def test_parses_one():
    assert parse("1") == 1

def test_parses_two():
    assert parse("2") == 2

def test_parses_ten():
    assert parse("10") == 10

def test_parses_neg():
    assert parse("-4") == -4
`},
	"Go": {"parse_test.go", `package parse

func TestParsesOne(t *testing.T) {
    if got := Parse("1"); got != 1 {
        t.Errorf("Parse = %d", got)
    }
}

func TestParsesTwo(t *testing.T) {
    if got := Parse("2"); got != 2 {
        t.Errorf("Parse = %d", got)
    }
}

func TestParsesTen(t *testing.T) {
    if got := Parse("10"); got != 10 {
        t.Errorf("Parse = %d", got)
    }
}

func TestParsesNeg(t *testing.T) {
    if got := Parse("-4"); got != -4 {
        t.Errorf("Parse = %d", got)
    }
}
`},
	"Kotlin": {"ParseTest.kt", `class ParseTest {
    @Test
    fun parsesOne() { assertEquals(1, parse("1")) }

    @Test
    fun parsesTwo() { assertEquals(2, parse("2")) }

    @Test
    fun parsesTen() { assertEquals(10, parse("10")) }

    @Test
    fun parsesNeg() { assertEquals(-4, parse("-4")) }
}
`},
}

func (d *scrapState) dataOnlyFile(language string) error {
	f, ok := dataOnly[language]
	if !ok {
		return fmt.Errorf("no %s test file is written", language)
	}
	return d.writeFile(f[0], f[1])
}

// recommendationNames is whether a recommendation of the file says to fold
// examples into the idiom.
func (d *scrapState) recommendationNames(idiom string) error {
	r, err := d.measuredReport(d.file)
	if err != nil {
		return err
	}
	for _, rec := range r.Recommendations {
		if strings.Contains(rec.Text, "fold them into "+idiom+":") {
			return nil
		}
	}
	return fmt.Errorf("no recommendation of %s folds its examples into %s: %+v", d.file, idiom, r.Recommendations)
}

// words are a text's words, lower-cased, without punctuation or articles.
func words(text string) []string {
	var out []string
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' || r == '/')
	}) {
		word = strings.Trim(word, ".")
		if word != "" && !slices.Contains([]string{"a", "an", "the", "this"}, word) {
			out = append(out, word)
		}
	}
	return out
}

// saysInOrder is whether text holds every word of what, in its order.
func saysInOrder(text, what string) bool {
	rest := words(text)
	for _, word := range words(what) {
		i := slices.Index(rest, word)
		if i < 0 {
			return false
		}
		rest = rest[i+1:]
	}
	return true
}

// aRecommendationSays is whether a recommendation of the file, of the
// confidence when one is given, says what the sentence says.
func (d *scrapState) aRecommendationSays(confidence, says string) error {
	r, err := d.its()
	if err != nil {
		return err
	}
	for _, rec := range r.Recommendations {
		if (confidence == "" || rec.Confidence == confidence) && saysInOrder(rec.Text, says) {
			return nil
		}
	}
	return fmt.Errorf("no %s recommendation of %s says to %s: %+v", confidence, r.File, says, r.Recommendations)
}

// alikeExamples writes three examples of sixteen lines whose bodies differ
// in one call, 0.9 alike as scrap rounds it, and checks scrap clusters all
// three at least percent alike, each larger than a table's example.
func (d *scrapState) alikeExamples(percent int) error {
	var source strings.Builder
	for i, step := range []string{"order.ship(customer.address);", "notify(customer.address);", "archive(order);"} {
		fmt.Fprintf(&source, `it("checks out order %d", () => {
  const cart = new Cart();
  cart.add({ sku: "a", price: 10 });
  cart.add({ sku: "b", price: 10 });
  cart.add({ sku: "c", price: 10 });
  const customer = new Customer("ada");
  customer.address = new Address("1 Main St");
  const payment = new Payment(customer, "card");
  payment.authorize(cart.total());
  const order = checkout(cart, customer, payment);
  order.confirm();
  %s
  const receipt = order.receipt();
  receipt.print();
  expect(order.total).toBe(30);
});

`, i+1, step)
	}
	if err := d.writeFile("checkout.test.ts", source.String()); err != nil {
		return err
	}
	reports, err := d.scrapIn(d.w.dir, d.file)
	if err != nil {
		return err
	}
	r := reports[d.file]
	for _, c := range r.Clusters {
		if len(c.Examples) == len(r.Details) && c.Similarity >= float64(percent)/100 {
			for _, e := range r.Details {
				if e.Lines <= fewLines {
					return fmt.Errorf("%q is %d lines, no large example", e.Name, e.Lines)
				}
			}
			return nil
		}
	}
	return fmt.Errorf("scrap clusters no three examples of %s at least %d%% alike: %+v", d.file, percent, r.Clusters)
}

// mockedFile writes count examples, mocked of them setting up mocks mocks,
// and checks scrap measures them so.
func (d *scrapState) mockedFile(count, mocked, mocks int) error {
	var source strings.Builder
	for i := range count {
		m := scrapMetrics{Lines: mocks + 4, Assertions: 1}
		if i < mocked {
			m.Mocks = mocks
		}
		example, err := tsExample(fmt.Sprintf("example %d", i+1), m)
		if err != nil {
			return err
		}
		source.WriteString(example + "\n")
	}
	if err := d.writeFile("mocked.test.ts", source.String()); err != nil {
		return err
	}
	reports, err := d.scrapIn(d.w.dir, d.file)
	if err != nil {
		return err
	}
	r, heavy := reports[d.file], 0
	for _, e := range r.Details {
		if e.Mocks >= mocks {
			heavy++
		}
	}
	if r.Examples != count || heavy != mocked {
		return fmt.Errorf("scrap finds %d examples, %d of them with %d or more mocks", r.Examples, heavy, mocks)
	}
	return nil
}

// troubledFile writes the file of d.examples examples, hot of them scoring
// at least score, spread round-robin across groups describe blocks, the
// others asserting and plain, and checks scrap measures them so.
func (d *scrapState) troubledFile(hot int, score float64, groups int) error {
	if d.examples < hot {
		return fmt.Errorf("%d examples hold no %d scoring %v or more", d.examples, hot, score)
	}
	blocks := make([]strings.Builder, groups)
	for i := range d.examples {
		var example string
		if i < hot {
			// Two branches and an assertion score 6.
			example = fmt.Sprintf("  it(\"troubled %d\", () => {\n    if (a%d) { run(); }\n    if (b%d) { run(); }\n    expect(value).toBe(%d);\n  });\n", i+1, i, i, i)
		} else {
			example = fmt.Sprintf("  it(\"plain %d\", () => {\n    expect(value).toBe(%d);\n  });\n", i+1, i)
		}
		blocks[i%groups].WriteString(example)
	}
	var source strings.Builder
	for i := range blocks {
		fmt.Fprintf(&source, "describe(\"part %d\", () => {\n%s});\n\n", i+1, blocks[i].String())
	}
	if err := d.writeFile("troubled.test.ts", source.String()); err != nil {
		return err
	}
	reports, err := d.scrapIn(d.w.dir, d.file)
	if err != nil {
		return err
	}
	r := reports[d.file]
	var found []string
	spread := map[string]bool{}
	for _, e := range r.Details {
		if e.Score >= score {
			found = append(found, e.Name)
			spread[e.Group] = true
		}
	}
	if r.Examples != d.examples || len(found) != hot || len(spread) != groups {
		return fmt.Errorf("scrap finds %d examples, %v scoring %v or more, across %d groups", r.Examples, found, score, len(spread))
	}
	return nil
}

// manyProblems is a TypeScript test file with more problems than
// recommendations are listed: four examples asserting nothing, two too
// large, three with logic, two mock-heavy, one asserting too much.
func manyProblems() string {
	var source strings.Builder
	write := func(name string, m scrapMetrics) {
		example, _ := tsExample(name, m)
		source.WriteString(example + "\n")
	}
	for i := range 4 {
		write(fmt.Sprintf("asserts nothing %d", i+1), scrapMetrics{Lines: 3 + i})
	}
	for i := range 2 {
		write(fmt.Sprintf("too large %d", i+1), scrapMetrics{Lines: 32 + i, Assertions: 1})
	}
	for i := range 3 {
		write(fmt.Sprintf("branches %d", i+1), scrapMetrics{Lines: 4 + i, Decisions: 1, Assertions: 1})
	}
	for i := range 2 {
		write(fmt.Sprintf("mocks %d", i+1), scrapMetrics{Lines: 6 + i, Mocks: 3, Assertions: 1})
	}
	write("asserts too much", scrapMetrics{Lines: 14, Assertions: 11})
	return source.String()
}

// rankedRecommendations is whether the file's recommendations come in the
// sentence's order of confidence, and the list holds two of them.
func (d *scrapState) rankedRecommendations(first, second, third, fourth string) error {
	if second != third {
		return fmt.Errorf("%s before %s, and %s before %s, is no order", first, second, third, fourth)
	}
	rank := map[string]int{first: 0, second: 1, fourth: 2}
	r, err := d.its()
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for i, rec := range r.Recommendations {
		if _, ok := rank[rec.Confidence]; !ok {
			return fmt.Errorf("recommendation %d is %s, no confidence the sentence orders", i+1, rec.Confidence)
		}
		seen[rec.Confidence] = true
		if i > 0 && rank[rec.Confidence] < rank[r.Recommendations[i-1].Confidence] {
			return fmt.Errorf("a %s recommendation follows a %s one: %+v", rec.Confidence, r.Recommendations[i-1].Confidence, r.Recommendations)
		}
	}
	if len(seen) < 2 {
		return fmt.Errorf("the recommendations are all %v, too few confidences to order", seen)
	}
	return nil
}

// problems are how many recommendations a report's measures call for: one
// for each smell of an example and each cluster.
func problems(r scrapReport) int {
	n := len(r.Clusters)
	for _, e := range r.Details {
		n += len(e.Smells)
	}
	return n
}

// atMostListed is whether the file lists most recommendations, its cap,
// printed and recorded, its examples calling for more.
func (d *scrapState) atMostListed(most int) error {
	r, err := d.its()
	if err != nil {
		return err
	}
	// The file calls for more, so the cap is what is listed: at most most,
	// and no fewer.
	if n := problems(r); n <= most {
		return fmt.Errorf("%s calls for %d recommendations, so the step cannot fail", r.File, n)
	}
	if len(r.Recommendations) != most {
		return fmt.Errorf("%s calls for %d recommendations and lists %d, not the %d it is capped at", r.File, problems(r), len(r.Recommendations), most)
	}
	files, err := printed(d.w.stdout)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.fields[1] == r.File && len(f.recommendations) != len(r.Recommendations) {
			return fmt.Errorf("%d recommendations are printed of %s, and %d recorded", len(f.recommendations), r.File, len(r.Recommendations))
		}
	}
	return nil
}

// eachNamesItsExample is whether every recommendation names an example of
// the file, quoted, with its line range.
func (d *scrapState) eachNamesItsExample() error {
	r, err := d.its()
	if err != nil {
		return err
	}
	if len(r.Recommendations) == 0 {
		return fmt.Errorf("%s has no recommendation", r.File)
	}
	for _, rec := range r.Recommendations {
		if !slices.ContainsFunc(r.Details, func(e scrapExample) bool {
			lines := fmt.Sprintf("%d-%d)", e.StartLine, e.EndLine)
			return strings.Contains(rec.Text, strconv.Quote(e.Name)+" (lines "+lines) || strings.Contains(rec.Text, strconv.Quote(e.Name)+" ("+lines)
		}) {
			return fmt.Errorf("%q names no example of %s with its line range", rec.Text, r.File)
		}
	}
	return nil
}

// --- Output and progress over time ---

// scrapSample is the project the scenarios that only say When I run run in:
// a clean Python file, a coverage matrix in TypeScript, a Python example
// that asserts nothing, and Go subtests with mocks and logic.
var scrapSample = map[string]string{
	"test_ok.py":    cleanPy,
	"parse.test.ts": "import { it, expect } from \"vitest\";\n\n" + dataOnly["TypeScript"][1],
	"test_x.py":     "def test_runs():\n    run()\n",
	"cart_test.go": `package cart

func TestCheckout(t *testing.T) {
    t.Run("charges", func(t *testing.T) {
        ctrl := gomock.NewController(t)
        bank := NewMockBank(ctrl)
        bank.EXPECT().Charge(1)
        Checkout(bank)
    })
    t.Run("refunds", func(t *testing.T) {
        if Refund() {
            log(1)
        } else {
            t.Error("no refund")
        }
    })
}
`,
}

const cleanPy = "def test_adds():\n    assert add(1, 2) == 3\n\ndef test_parses():\n    assert parse('x') == {'x': None}\n"

// actions are the words a file's action is printed as.
const actions = "LEAVE_ALONE|AUTO_TABLE_DRIVE|AUTO_REFACTOR|MANUAL_SPLIT|REVIEW_FIRST"

// eachFilePrintedAs is whether the run prints one line of each file it
// reports, in the form the sentence gives with file the path, ACTION the
// action and … a number, and nothing else on the line.
func (d *scrapState) eachFilePrintedAs(form string) error {
	reports, err := d.reported()
	if err != nil {
		return err
	}
	pattern := regexp.QuoteMeta(form)
	pattern = strings.Replace(pattern, "file", `(\S+)`, 1)
	pattern = strings.Replace(pattern, "ACTION", "(?:"+actions+")", 1)
	pattern = strings.ReplaceAll(pattern, "…", `\d+(?:\.\d+)?`)
	line := regexp.MustCompile("^" + pattern + "$")
	seen := map[string]int{}
	for _, text := range outputLines(filepath.ToSlash(d.w.stdout)) {
		if m := line.FindStringSubmatch(strings.TrimSuffix(text, "\r")); m != nil {
			seen[m[1]]++
		}
	}
	if len(reports) < 2 {
		return fmt.Errorf("the run reports %d files, too few to show the form", len(reports))
	}
	for file := range reports {
		if seen[file] != 1 {
			return fmt.Errorf("%s is printed %d times as %q\n%s", file, seen[file], form, d.w.report())
		}
	}
	return nil
}

// highestPressureFirst is whether the files' lines come from the highest
// pressure down, and the run printed two pressures.
func (d *scrapState) highestPressureFirst() error {
	if _, err := d.reported(); err != nil {
		return err
	}
	files, err := printed(d.w.stdout)
	if err != nil {
		return err
	}
	var pressures []float64
	for _, f := range files {
		p, _ := strconv.ParseFloat(f.fields[3], 64)
		pressures = append(pressures, p)
	}
	if len(slices.Compact(slices.Clone(pressures))) < 2 {
		return fmt.Errorf("the files' pressures are %v, too few to order", pressures)
	}
	if !slices.IsSortedFunc(pressures, func(a, b float64) int { return cmp.Compare(b, a) }) {
		return fmt.Errorf("the files are printed with the pressures %v", pressures)
	}
	return nil
}

// recommendationsFollow is whether each file the run does not leave alone
// is followed by its recommendations, indented, each with its confidence,
// one of them printed at least.
func (d *scrapState) recommendationsFollow() error {
	reports, err := d.reported()
	if err != nil {
		return err
	}
	files, err := printed(d.w.stdout)
	if err != nil {
		return err
	}
	shown := 0
	for _, f := range files {
		r := reports[f.fields[1]]
		want := r.Recommendations
		if r.Action == "LEAVE_ALONE" {
			want = nil
		}
		if !slices.Equal(f.recommendations, want) {
			return fmt.Errorf("under %s are printed %+v, not %+v", r.File, f.recommendations, want)
		}
		shown += len(want)
	}
	if shown == 0 {
		return errors.New("the run prints no recommendation, so the step cannot fail")
	}
	return nil
}

// quietFile writes two examples sharing most of their structure, which
// scrap leaves alone with a LOW hint, and checks it does.
func (d *scrapState) quietFile(action, confidence string) error {
	if err := d.writeFile("quiet.test.ts", quietTs); err != nil {
		return err
	}
	reports, err := d.scrapIn(d.w.dir, d.file)
	if err != nil {
		return err
	}
	r := reports[d.file]
	if r.Action != action || !slices.ContainsFunc(r.Recommendations, func(rec scrapRecommendation) bool { return rec.Confidence == confidence }) {
		return fmt.Errorf("scrap gives %s %s and %+v, not %s with a %s hint", d.file, r.Action, r.Recommendations, action, confidence)
	}
	return nil
}

// quietTs is two examples, each asserting once, that share most of their
// structure: 0.8 alike, below what scrap asks a helper for.
const quietTs = `it("charges a card", () => {
  const cart = new Cart();
  cart.add({ sku: "a", price: 10 });
  cart.add({ sku: "b", price: 20 });
  const customer = new Customer("ada");
  const payment = new Payment(customer, "card");
  payment.authorize(cart.total());
  const order = checkout(cart, customer, payment);
  order.confirm();
  const receipt = order.receipt();
  expect(receipt.total).toBe(30);
});

it("charges a voucher", () => {
  const cart = new Cart();
  cart.add({ sku: "a", price: 10 });
  cart.add({ sku: "b", price: 20 });
  const customer = new Customer("ada");
  const payment = new Payment(customer, "card");
  payment.authorize(cart.total());
  const order = checkout(cart, customer, payment);
  order.confirm();
  extra1(order, receipt.lines);
  extra2(order, receipt.lines);
  expect(receipt.total).toBe(30);
});
`

// hint is the recommendation of the file its action leaves alone.
func (d *scrapState) hint() (string, error) {
	r, err := d.its()
	if err != nil {
		return "", err
	}
	for _, rec := range r.Recommendations {
		return rec.Text, nil
	}
	return "", fmt.Errorf("%s has no hint", r.File)
}

// printedWith is whether the run, with the flag added, prints the hint
// under the file, with its confidence.
func (d *scrapState) printedWith(flag string) error {
	hint, err := d.hint()
	if err != nil {
		return err
	}
	r, _ := d.its()
	code, stdout, stderr, err := d.w.capture(d.w.last.dir, d.w.env(), append(slices.Clone(d.w.last.args), flag)...)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("with %s itos-cc exited %d\n%s%s", flag, code, stdout, stderr)
	}
	files, err := printed(stdout)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.fields[1] == r.File && slices.Contains(f.recommendations, scrapRecommendation{r.Recommendations[0].Confidence, hint}) {
			return nil
		}
	}
	return fmt.Errorf("with %s the hint %q is not printed under %s\n%s", flag, hint, r.File, stdout)
}

// everyExamplePrinted is whether, under each file, every example is
// printed with the measures the list names, as the run recorded them, and
// one has a smell.
func (d *scrapState) everyExamplePrinted(list string) error {
	reports, err := d.reported()
	if err != nil {
		return err
	}
	files, err := printed(d.w.stdout)
	if err != nil {
		return err
	}
	columns := map[string]func(m []string, e scrapExample) bool{
		"score":      func(m []string, e scrapExample) bool { return strings.TrimSpace(m[1]) == fmt.Sprintf("%.1f", e.Score) },
		"lines":      func(m []string, e scrapExample) bool { return m[6] == strconv.Itoa(e.Lines) },
		"assertions": func(m []string, e scrapExample) bool { return m[7] == strconv.Itoa(e.Assertions) },
		"decisions":  func(m []string, e scrapExample) bool { return m[8] == strconv.Itoa(e.Decisions) },
		"mocks":      func(m []string, e scrapExample) bool { return m[9] == strconv.Itoa(e.Mocks) },
		"smells":     func(m []string, e scrapExample) bool { return m[10] == strings.Join(e.Smells, " ") },
	}
	keys := listItems(list)
	for _, key := range keys {
		if columns[key] == nil {
			return fmt.Errorf("no example is printed with its %s", key)
		}
	}
	smelly := false
	for _, f := range files {
		r := reports[f.fields[1]]
		if len(f.examples) != len(r.Details) {
			return fmt.Errorf("%d examples of %s are printed, of %d", len(f.examples), r.File, len(r.Details))
		}
		for i, e := range r.Details {
			m := f.examples[i]
			if m[2] != r.File || m[3] != strconv.Itoa(e.StartLine) || m[4] != strconv.Itoa(e.EndLine) || m[5] != strconv.Quote(e.Name) {
				return fmt.Errorf("example %d of %s is printed as %q, not %q at %d-%d", i+1, r.File, m[0], e.Name, e.StartLine, e.EndLine)
			}
			for _, key := range keys {
				if !columns[key](m, e) {
					return fmt.Errorf("%q is printed with another %s than %+v", m[0], key, e)
				}
			}
			smelly = smelly || len(e.Smells) > 0
		}
	}
	if !smelly {
		return errors.New("no example has a smell, so the step cannot fail")
	}
	return nil
}

// eachReportHas is whether every file of the object has each key the list
// names.
func (d *scrapState) eachReportHas(list string) error {
	var object struct {
		Files []map[string]any `json:"files"`
	}
	if err := json.Unmarshal([]byte(d.w.stdout), &object); err != nil || len(object.Files) == 0 {
		return fmt.Errorf("the object lists no file: %v\n%s", err, d.w.report())
	}
	for _, f := range object.Files {
		for _, key := range listItems(list) {
			if _, ok := f[key]; !ok {
				return fmt.Errorf("the report of %v has no %q", f["file"], key)
			}
		}
	}
	return nil
}

// pressureFixtures are single TypeScript examples by the pressure scrap
// gives their file, the score less one: decisions add 2.5, mocks beyond two
// 1.5 each, lines beyond fifteen a fifth each.
var pressureFixtures = map[float64]scrapMetrics{
	10:  {Lines: 8, Decisions: 4, Assertions: 1},
	4:   {Lines: 8, Decisions: 1, Mocks: 3, Assertions: 1},
	3.2: {Lines: 16, Mocks: 4, Assertions: 1},
	3:   {Lines: 8, Mocks: 4, Assertions: 1},
}

func pressureFixture(pressure float64) (string, error) {
	m, ok := pressureFixtures[pressure]
	if !ok {
		return "", fmt.Errorf("no fixture has the pressure %v", pressure)
	}
	return tsExample("pressed", m)
}

// pressureFile is the file whose pressure the progress scenarios follow.
const pressureFile = "pressed.test.ts"

// recordedPressure writes the file with a fixture of the pressure and runs
// scrap, which records it in .metrics/scrap.json, and checks it recorded
// that pressure.
func (d *scrapState) recordedPressure(pressure float64) error {
	before, err := pressureFixture(pressure)
	if err != nil {
		return err
	}
	d.before = before
	if err := d.writeFile(pressureFile, before); err != nil {
		return err
	}
	if _, err := d.scrapIn(d.w.dir); err != nil {
		return err
	}
	snapshot, err := d.snapshot()
	if err != nil {
		return err
	}
	if got := snapshot[pressureFile].Pressure; got != pressure {
		return fmt.Errorf(".metrics/scrap.json recorded %v for %s, not %v", got, pressureFile, pressure)
	}
	d.beforePressure = pressure
	return nil
}

// runWithPressure rewrites the file with a fixture of the pressure, runs
// itos-cc with args, and checks the run measured that pressure.
func (d *scrapState) runWithPressure(args string, pressure float64) error {
	after, err := pressureFixture(pressure)
	if err != nil {
		return err
	}
	d.after = after
	if err := d.w.write(pressureFile, after); err != nil {
		return err
	}
	if err := d.w.runWith(d.w.env(), args); err != nil {
		return err
	}
	r, err := d.its()
	if err != nil {
		return err
	}
	if r.Pressure != pressure {
		return fmt.Errorf("the run measured %s's pressure as %v, not %v", r.File, r.Pressure, pressure)
	}
	return nil
}

// verdictIs is whether the run judged the file so: in its line, a verdict
// other than unchanged with the previous pressure; and in --json's
// compare, the same two over again in a folder of their own.
func (d *scrapState) verdictIs(verdict string) error {
	if _, err := d.reported(); err != nil {
		return err
	}
	files, err := printed(d.w.stdout)
	if err != nil {
		return err
	}
	var line []string
	for _, f := range files {
		if f.fields[1] == pressureFile {
			line = f.fields
		}
	}
	if line == nil {
		return fmt.Errorf("%s is not printed\n%s", pressureFile, d.w.report())
	}
	if verdict == "unchanged" && line[7] != "" || verdict != "unchanged" && (line[7] != verdict || line[8] != fmt.Sprintf("%.1f", d.beforePressure)) {
		return fmt.Errorf("%q gives no verdict %s from %.1f", line[0], verdict, d.beforePressure)
	}
	again := filepath.Join(d.w.dir, "again")
	if err := d.w.writeIn(again, pressureFile, d.before); err != nil {
		return err
	}
	if _, err := d.scrapIn(again); err != nil {
		return err
	}
	if err := d.w.writeIn(again, pressureFile, d.after); err != nil {
		return err
	}
	reports, err := d.scrapIn(again)
	if err != nil {
		return err
	}
	if c := reports[pressureFile].Compare; c == nil || c.Verdict != verdict || c.PreviousPressure != d.beforePressure {
		return fmt.Errorf("--json compares %s as %+v, not %s from %v", pressureFile, c, verdict, d.beforePressure)
	}
	return nil
}

// lineEndsWith is whether the file's line ends with the words and the
// previous pressure.
func (d *scrapState) lineEndsWith(ending string) error {
	if _, err := d.reported(); err != nil {
		return err
	}
	files, err := printed(d.w.stdout)
	if err != nil {
		return err
	}
	want := fmt.Sprintf("%s %.1f", ending, d.beforePressure)
	for _, f := range files {
		if f.fields[1] == d.file {
			if !strings.HasSuffix(f.line, want) {
				return fmt.Errorf("%q does not end with %q", f.line, want)
			}
			return nil
		}
	}
	return fmt.Errorf("%s is not printed\n%s", d.file, d.w.report())
}

// warningSays is whether a warning under the file's line says to do what
// the sentence says before keeping the change.
func (d *scrapState) warningSays(what string) error {
	files, err := printed(d.w.stdout)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.fields[1] != d.file {
			continue
		}
		for _, warning := range f.warnings {
			if saysInOrder(warning, what+" before keeping") {
				return nil
			}
		}
		return fmt.Errorf("no warning under %q says to %s before keeping the change: %q", f.line, what, f.warnings)
	}
	return fmt.Errorf("%s is not printed\n%s", d.file, d.w.report())
}

// holdsReports writes the two test files and runs scrap, which records
// both, then changes the second, so a report of it measured again would
// differ from the one kept.
func (d *scrapState) holdsReports(first, second string) error {
	one, _ := tsExample("adds", scrapMetrics{Lines: 3, Assertions: 1})
	two, _ := tsExample("removes", scrapMetrics{Lines: 4, Assertions: 1})
	if err := d.writeFiles(map[string]string{first: one, second: two}); err != nil {
		return err
	}
	if _, err := d.scrapIn(d.w.dir); err != nil {
		return err
	}
	raw, err := d.w.readIn(d.w.dir, ".metrics/scrap.json")
	if err != nil {
		return err
	}
	var snapshot struct {
		Files []json.RawMessage `json:"files"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return err
	}
	for _, f := range snapshot.Files {
		var r scrapReport
		if json.Unmarshal(f, &r) == nil && r.File == second {
			d.kept = f
		}
	}
	if d.kept == nil {
		return fmt.Errorf(".metrics/scrap.json holds no report of %s\n%s", second, raw)
	}
	changed, _ := tsExample("removes", scrapMetrics{Lines: 4})
	return d.w.write(second, two+"\n"+strings.Replace(changed, "removes", "removes again", 1))
}

// stillHolds is whether .metrics/scrap.json holds the report of file it
// held before the run, unchanged, beside the run's own.
func (d *scrapState) stillHolds(file string) error {
	if _, err := d.reported(); err != nil {
		return err
	}
	raw, err := d.w.readIn(d.w.dir, ".metrics/scrap.json")
	if err != nil {
		return err
	}
	var snapshot struct {
		Files []json.RawMessage `json:"files"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return err
	}
	held, others := false, 0
	for _, f := range snapshot.Files {
		var r scrapReport
		if json.Unmarshal(f, &r) != nil {
			continue
		}
		if r.File == file {
			var got, want any
			_ = json.Unmarshal(f, &got)
			_ = json.Unmarshal(d.kept, &want)
			a, _ := json.Marshal(got)
			b, _ := json.Marshal(want)
			if string(a) != string(b) {
				return fmt.Errorf(".metrics/scrap.json holds\n%s\nof %s, not\n%s", a, file, b)
			}
			held = true
		} else {
			others++
		}
	}
	if !held || others == 0 {
		return fmt.Errorf(".metrics/scrap.json holds %s %v, beside %d others\n%s", file, held, others, raw)
	}
	return nil
}

// runCompletes is whether the run exited 0, printed the file, and wrote a
// snapshot that can be read.
func (d *scrapState) runCompletes() error {
	reports, err := d.reported()
	if err != nil {
		return err
	}
	if _, ok := reports[d.file]; !ok {
		return fmt.Errorf("the run does not report %s\n%s", d.file, d.w.report())
	}
	return nil
}
