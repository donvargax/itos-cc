package features

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

// Coverage reports written by hand, and what itos-cc reads from each format.

// add writes the fixture's report and source, which reads text, and keeps
// it.
func (c *coverageState) add(f covFixture, source, report string) error {
	if err := c.writeIn(".", map[string]string{f.source: source, f.report: report}); err != nil {
		return err
	}
	c.fixtures = append(c.fixtures, f)
	return nil
}

// fixture is the one fixture a Given wrote.
func (c *coverageState) fixture() (covFixture, error) {
	if len(c.fixtures) != 1 {
		return covFixture{}, fmt.Errorf("%d reports were written, not one", len(c.fixtures))
	}
	return c.fixtures[0], nil
}

// reportFile writes a report of the format its name is usual for, one
// entry for a source of its language.
func (c *coverageState) reportFile(name string) error {
	switch name {
	case "go.out":
		if err := c.w.write("go.mod", "module example.com/demo\n\ngo 1.22\n"); err != nil {
			return err
		}
		f := covFixture{named: "a Go cover profile", report: name, source: "board/board.go", function: "Place",
			entry: "example.com/demo/board/board.go", unit: "statement", pieces: goBlocks(true, false, true)}
		return c.add(f, goSource("board"), "mode: set\n"+goRecords(f.entry, f.pieces))
	case "lcov.info":
		f := covFixture{named: "LCOV", report: name, source: "src/demo/board.ts", function: "place", entry: "src/demo/board.ts", unit: "line",
			pieces: lcovLines([2]int{2, 3}, [2]int{3, 1}, [2]int{4, 0}, [2]int{6, 2}, [2]int{7, 0})}
		return c.add(f, tsSource, lcovRecord(f.entry, f.pieces, nil))
	case "jacoco.xml":
		f := covFixture{named: "JaCoCo XML", report: name, source: "src/main/kotlin/demo/game/Board.kt", function: "place",
			entry: "demo/game/Board.kt", unit: "instruction", pieces: ktInstructions}
		return c.add(f, strings.Replace(ktSource, "package board", "package demo.game", 1), jacocoReport("demo/game", "Board.kt", f.pieces, nil))
	}
	return fmt.Errorf("no report is written as %s", name)
}

// ktInstructions are JaCoCo lines of ktSource's place, no branch among
// them: two of four run, eight of twelve instructions.
var ktInstructions = []covPiece{{line: 4, total: 2, covered: 2}, {line: 6, total: 4}, {line: 9, total: 3, covered: 3}, {line: 11, total: 3, covered: 3}}

// computed runs crap --json over the reports the Givens wrote and their
// sources.
func (c *coverageState) computed() error {
	var args []string
	switch {
	case c.split != nil:
		args = append(args, "--coverage-report", c.split.reports[0], "--coverage-report", c.split.reports[1], "board.go")
	case len(c.fixtures) > 0:
		var reports []string
		for _, f := range c.fixtures {
			if !slices.Contains(reports, f.report) {
				reports = append(reports, f.report)
				args = append(args, "--coverage-report", f.report)
			}
		}
		for _, f := range c.fixtures {
			if !slices.Contains(args, f.source) {
				args = append(args, f.source)
			}
		}
	default:
		return errors.New("no report was written to compute coverage from")
	}
	return c.w.runEnv(c.w.dir, c.w.env(), append([]string{"crap", "--json"}, args...)...)
}

// readAs is whether the one report was written as format and itos-cc read
// it so: its function has the coverage the report means in it.
func (c *coverageState) readAs(format string) error {
	f, err := c.fixture()
	if err != nil {
		return err
	}
	if f.named != format {
		return fmt.Errorf("the report is written as %s, not %s", f.named, format)
	}
	entries, err := c.listed()
	if err != nil {
		return err
	}
	want, _ := share(f.pieces, byUnit)
	return c.hasCoverage(entries, covFunction{f.source, f.function}, want)
}

// oneEntryFor is whether the one report has one entry, naming source, and
// itos-cc matched it to the source written, the one file it scores.
func (c *coverageState) oneEntryFor(source string) error {
	f, err := c.fixture()
	if err != nil {
		return err
	}
	if f.entry != source {
		return fmt.Errorf("the report's entry is %s, not %s", f.entry, source)
	}
	entries, err := c.listed()
	if err != nil {
		return err
	}
	for _, e := range entries {
		if filepath.ToSlash(e.File) != f.source || e.Coverage == nil {
			return fmt.Errorf("%s of %s is listed with coverage %s, not %s's entry's\n%s", e.Name, e.File, shown(e.Coverage), source, c.w.report())
		}
	}
	if len(entries) == 0 {
		return fmt.Errorf("no function is listed\n%s", c.w.report())
	}
	return nil
}

// formatReport writes a report of format over a function of no decision,
// its pieces of different weights.
func (c *coverageState) formatReport(format string) error {
	switch format {
	case "LCOV":
		f := covFixture{format: format, report: "lcov.info", source: "src/board.ts", function: "place", entry: "src/board.ts", unit: "line",
			pieces: lcovLines([2]int{2, 3}, [2]int{3, 1}, [2]int{4, 0}, [2]int{6, 2}, [2]int{7, 0})}
		return c.add(f, tsSource, lcovRecord(f.entry, f.pieces, nil))
	case "Go cover profile":
		if err := c.w.write("go.mod", "module example.com/board\n\ngo 1.22\n"); err != nil {
			return err
		}
		f := covFixture{format: format, report: "coverage.out", source: "board.go", function: "Place", entry: "example.com/board/board.go",
			unit: "statement", pieces: goBlocks(true, false, true)}
		return c.add(f, goSource("board"), "mode: set\n"+goRecords(f.entry, f.pieces))
	}
	f := covFixture{format: format, report: "jacoco.xml", source: "src/main/kotlin/board/Board.kt", function: "place", entry: "board/Board.kt",
		unit: "instruction", pieces: ktInstructions}
	return c.add(f, ktSource, jacocoReport("board", "Board.kt", f.pieces, nil))
}

// weightedBy is whether the one report's function has the share of its
// pieces weighed by weight: one per line, or a unit the report's pieces
// must weigh in.
func (c *coverageState) weightedBy(weight string) error {
	f, err := c.fixture()
	if err != nil {
		return err
	}
	weigh := byUnit
	switch weight {
	case "one per line":
		weigh = onePerPiece
	case "the block's statement count":
		if f.unit != "statement" {
			return fmt.Errorf("the %s report's pieces have no statement count", f.format)
		}
	case "the line's instruction count":
		if f.unit != "instruction" {
			return fmt.Errorf("the %s report's pieces have no instruction count", f.format)
		}
	default:
		return fmt.Errorf("no weighting is known as %s", weight)
	}
	one, _ := share(f.pieces, onePerPiece)
	if own, _ := share(f.pieces, byUnit); f.unit != "line" && shown(&one) == shown(&own) {
		return errors.New("the report's pieces weigh the same whatever their weight")
	}
	entries, err := c.listed()
	if err != nil {
		return err
	}
	want, _ := share(f.pieces, weigh)
	return c.hasCoverage(entries, covFunction{f.source, f.function}, want)
}

// pickSource is pick as COV-22 writes it.
const pickSource = "def pick(x):\n    r = 0\n    if x > 0:\n        r = 1\n    else:\n        r = 2\n    return r\n"

// coveragePyReport writes the function the report is for in pick.py.
func (c *coverageState) coveragePyReport(text *godog.DocString) error {
	if text.Content+"\n" != pickSource {
		return fmt.Errorf("only pick as the scenario writes it is measured here, not:\n%s", text.Content)
	}
	return c.w.write("pick.py", pickSource)
}

// testsCallPick writes lcov.info as coverage.py writes it for tests that
// call pick(x) once: the arm x takes run, the other not, and the if's two
// branches, the one taken.
func (c *coverageState) testsCallPick(x int) error {
	arm, other := 4, 6
	if x <= 0 {
		arm, other = 6, 4
	}
	pieces := lcovLines([2]int{1, 1}, [2]int{2, 1}, [2]int{3, 1}, [2]int{arm, 1}, [2]int{other, 0}, [2]int{7, 1})
	slices.SortFunc(pieces, func(a, b covPiece) int { return a.line - b.line })
	taken := 0
	if arm == 6 {
		taken = 1
	}
	f := covFixture{format: "LCOV", report: "lcov.info", source: "pick.py", function: "pick", entry: "pick.py", unit: "line", pieces: pieces,
		decisions: []covDecision{{line: 3, branches: 2, taken: []int{taken}}}}
	var b strings.Builder
	b.WriteString("SF:pick.py\n")
	for _, p := range pieces {
		fmt.Fprintf(&b, "DA:%d,%d\n", p.line, p.hits)
	}
	fmt.Fprintf(&b, "LF:6\nLH:5\nFN:1,7,pick\nFNDA:1,pick\nFNF:1\nFNH:1\n")
	fmt.Fprintf(&b, "BRDA:3,0,jump to line 4,%d\nBRDA:3,0,jump to line 6,%d\nBRF:2\nBRH:1\nend_of_record\n", hits(arm == 4), hits(arm == 6))
	c.fixtures = append(c.fixtures, f)
	return c.w.write(f.report, b.String())
}

// numberWords are the numbers a sentence writes in words.
var numberWords = map[string]int{"none": 0, "one": 1, "two": 2, "three": 3, "four": 4}

// branchesTaken is whether the one function has percent coverage, the
// share of its branches taken the sentence words, which its report's
// decision records.
func (c *coverageState) branchesTaken(percent int, taken, branches string) error {
	f, err := c.fixture()
	if err != nil {
		return err
	}
	n, ok1 := numberWords[taken]
	m, ok2 := numberWords[branches]
	switch {
	case !ok1 || !ok2:
		return fmt.Errorf("%s of %s are no numbers known here", taken, branches)
	case len(f.decisions) != 1 || f.decisions[0].branches != m || len(f.decisions[0].taken) != n:
		return fmt.Errorf("the report's decisions are %+v, not one of %d branches, %d taken", f.decisions, m, n)
	case 100*n != percent*m:
		return fmt.Errorf("%d of %d branches is not %d%%", n, m, percent)
	}
	entries, err := c.listed()
	if err != nil {
		return err
	}
	return c.hasCoverage(entries, covFunction{f.source, f.function}, float64(percent))
}

// decisionIs is whether, in a report of the one fixture's format, what the
// sentence words is a decision: a probe report written over its source,
// its function's coverage that of the decision and not of its lines.
func (c *coverageState) decisionIs(decision string) error {
	f, err := c.fixture()
	if err != nil {
		return err
	}
	type probe struct {
		report string
		want   float64
	}
	var probes []probe
	switch m := regexp.MustCompile(`^a BRDA block with (\w+) or more branches$`).FindStringSubmatch(decision); {
	case m != nil:
		n, ok := numberWords[m[1]]
		if f.format != "LCOV" || !ok || n < 2 {
			return fmt.Errorf("no %s report is written with a BRDA block of %s branches", f.format, m[1])
		}
		ran := lcovLines([2]int{2, 1}, [2]int{3, 1}, [2]int{4, 1}, [2]int{6, 1}, [2]int{7, 1})
		// A block of n branches, one taken, decides; one of fewer, none
		// taken, does not, and every line ran.
		probes = []probe{
			{lcovRecord(f.entry, ran, []covDecision{{line: 3, branches: n, taken: []int{0}}}), 100.0 / float64(n)},
			{lcovRecord(f.entry, ran, []covDecision{{line: 3, branches: n - 1}}), 100},
		}
	case decision == "a line with branch counters, weighted by its branches":
		if f.format != "JaCoCo/Kover XML" {
			return fmt.Errorf("no %s report is written with branch counters", f.format)
		}
		// Every instruction ran: an if of two branches took one, and one
		// of four took three, (1 + 3) / (2 + 4).
		ran := []covPiece{{line: 4, total: 2, covered: 2}, {line: 5, total: 3, covered: 3}, {line: 6, total: 2, covered: 2},
			{line: 8, total: 4, covered: 4}, {line: 9, total: 3, covered: 3}, {line: 11, total: 2, covered: 2}}
		probes = []probe{{jacocoReport("board", "Board.kt", ran, []covDecision{{line: 5, branches: 2, taken: []int{0}}, {line: 8, branches: 4, taken: []int{0, 1, 2}}}), 100 * 4.0 / 6}}
	case strings.HasPrefix(decision, "never: ") && strings.HasSuffix(decision, "so statements decide"):
		if f.format != "Go cover profile" {
			return fmt.Errorf("the %s report is no Go profile", f.format)
		}
		blocks := goBlocks(true, false, true)
		want, _ := share(blocks, byUnit)
		probes = []probe{{"mode: set\n" + goRecords(f.entry, blocks), want}}
	default:
		return fmt.Errorf("no decision is known as %s", decision)
	}
	for i, p := range probes {
		name := fmt.Sprintf("probe-%d%s", i, path.Ext(f.report))
		if err := c.w.write(name, p.report); err != nil {
			return err
		}
		if err := c.w.runEnv(c.w.dir, c.w.env(), "crap", "--json", "--coverage-report", name, f.source); err != nil {
			return err
		}
		entries, err := c.listed()
		if err != nil {
			return err
		}
		if err := c.hasCoverage(entries, covFunction{f.source, f.function}, p.want); err != nil {
			return fmt.Errorf("by %s:\n%s\n%v", name, p.report, err)
		}
	}
	return nil
}

// noDecision writes two reports holding no decision: LCOV of a TypeScript
// function, by its lines, and a Go profile of a Go one, by its statements.
func (c *coverageState) noDecision() error {
	lcov := covFixture{format: "LCOV", report: "lcov.info", source: "src/board.ts", function: "place", entry: "src/board.ts", unit: "line",
		pieces: lcovLines([2]int{2, 1}, [2]int{3, 1}, [2]int{4, 0}, [2]int{6, 1}, [2]int{7, 0})}
	if err := c.add(lcov, tsSource, lcovRecord(lcov.entry, lcov.pieces, nil)); err != nil {
		return err
	}
	if err := c.w.write("go.mod", "module example.com/board\n\ngo 1.22\n"); err != nil {
		return err
	}
	profile := covFixture{format: "Go cover profile", report: "coverage.out", source: "board.go", function: "Place", entry: "example.com/board/board.go",
		unit: "statement", pieces: goBlocks(true, false, true)}
	return c.add(profile, goSource("board"), "mode: set\n"+goRecords(profile.entry, profile.pieces))
}

// shareOfLines is whether each report's function, of no decision, has the
// share of its lines or statements that ran.
func (c *coverageState) shareOfLines() error {
	if len(c.fixtures) == 0 {
		return errors.New("no report was written")
	}
	if err := c.computed(); err != nil {
		return err
	}
	entries, err := c.listed()
	if err != nil {
		return err
	}
	for _, f := range c.fixtures {
		if len(f.decisions) > 0 || f.unit == "instruction" {
			return fmt.Errorf("the %s report is not one of lines or statements alone", f.format)
		}
		want, _ := share(f.pieces, byUnit)
		if err := c.hasCoverage(entries, covFunction{f.source, f.function}, want); err != nil {
			return err
		}
	}
	return nil
}

// c8Source is a TypeScript function of an if that returns, whose
// statements are on lines 2, 3 and 5.
const c8Source = "export function place(a: number): number {\n  if (a > 1) {\n    return 1;\n  }\n  return 2;\n}\n"

// c8Report writes the source an LCOV report of c8's measures.
func (c *coverageState) c8Report() error {
	c.fixtures = append(c.fixtures, covFixture{format: "LCOV", report: "lcov.info", source: "src/board.ts", function: "place", entry: "src/board.ts", unit: "line"})
	return c.w.write("src/board.ts", c8Source)
}

// oneBranchBlocks writes the report as c8 writes it for a test of place(5):
// the if and its arm ran, the return after it not; the function body a
// block of one branch, taken, and the code after the arm one, not taken,
// the arm that ran left out.
func (c *coverageState) oneBranchBlocks() error {
	if len(c.fixtures) != 1 || c.fixtures[0].report != "lcov.info" {
		return errors.New("no c8 report's source was written")
	}
	f := &c.fixtures[0]
	f.pieces = lcovLines([2]int{2, 1}, [2]int{3, 1}, [2]int{5, 0})
	f.decisions = []covDecision{{line: 2, branches: 1, taken: []int{0}}, {line: 5, branches: 1}}
	return c.w.write(f.report, lcovRecord(f.entry, f.pieces, f.decisions))
}

// linesDecide is whether the report's blocks are each of one branch and
// the function has the share of its lines that ran.
func (c *coverageState) linesDecide() error {
	f, err := c.fixture()
	if err != nil {
		return err
	}
	if len(f.decisions) == 0 || slices.ContainsFunc(f.decisions, func(d covDecision) bool { return d.branches != 1 }) {
		return fmt.Errorf("the report's blocks are %+v, not of one branch each", f.decisions)
	}
	entries, err := c.listed()
	if err != nil {
		return err
	}
	want, _ := share(f.pieces, byUnit)
	return c.hasCoverage(entries, covFunction{f.source, f.function}, want)
}
