// The steps of coverage.feature: which coverage commands itos-cc runs for a
// project, as it announces each on stderr ("itos-cc: coverage <dir>$
// <command>"), and how it reads the reports those commands write or a
// person names, judged by the coverage crap then gives each function. A
// report a Given writes by hand stands for what a tool writes, and what it
// means is worked out from what the step wrote, never from itos-cc's code.
// Like every step here, they read the binary's output alone.
package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// coverageState is one coverage.feature scenario's state beside the
// world's.
type coverageState struct {
	w *world

	// project is the folder of the project a Given wrote, slash-separated
	// from the scratch folder, "." for the scratch folder itself.
	project string
	// roots are the project's build roots, slash-separated from the scratch
	// folder, and the language each one's coverage command measures.
	roots map[string]string
	// markers are the files the project's tests write when they run,
	// slash-separated from the scratch folder.
	markers []string
	// missing is the tool a Given left uninstalled, as itos-cc names it.
	missing string
	// pm is the package manager the project's scripts run with.
	pm string
	// want is the coverage each function has by the reports the project's
	// tools or a Given wrote, by its file, slash-separated from the folder
	// itos-cc runs in, and its name.
	want map[covFunction]float64

	// fixtures are reports a Given wrote by hand, each over one function.
	fixtures []covFixture
	// split is the two reports COV-35 writes over one file.
	split *covSplit

	// The matching scenarios: the folder a run goes from, slash-separated
	// from the scratch folder; the sources it selects there, which are
	// all a Given wrote; the paths it names, none for every source; the
	// report entries a Given named; whether they are a Go profile's; and,
	// for each report it reads in a run of its own, the entries it holds.
	runDir   string
	sources  []string
	paths    []string
	entries  []string
	goFormat bool
	cases    [][]string
	// matched is what each case's run listed.
	matched [][]crapEntry
}

// covFunction is a function as crap lists it: its file, slash-separated,
// and its name.
type covFunction struct{ file, name string }

// covPiece is one piece of code a report measures: an LCOV or JaCoCo line,
// or a Go block. Total is its weight in the report's unit and covered how
// much of that ran; hits is the count an LCOV line records, and span a Go
// block's line.column range.
type covPiece struct {
	line           int
	span           string
	total, covered float64
	hits           int
}

// covDecision is a decision a report records on a line: its branches, and
// which of them were taken.
type covDecision struct {
	line, branches int
	taken          []int
}

// covFixture is a report a Given wrote by hand over one function of one
// source: the format as each scenario names it, the files, slash-separated
// from the scratch folder, the path the report names the source by, the
// unit its pieces weigh in, and what it measures.
type covFixture struct {
	format, named   string
	report, source  string
	function, entry string
	unit            string
	pieces          []covPiece
	decisions       []covDecision
}

// covSplit is COV-35's two reports over board.go: what each measures of F,
// which has no decision, and of G's one decision.
type covSplit struct {
	reports [2]string
	f       [2][]covPiece
	g       [2]covDecision
}

// share is the covered part of pieces, in percent, each weighed by weigh;
// false when they weigh nothing.
func share(pieces []covPiece, weigh func(covPiece) (total, covered float64)) (float64, bool) {
	var all, ran float64
	for _, p := range pieces {
		t, c := weigh(p)
		all += t
		ran += c
	}
	if all == 0 {
		return 0, false
	}
	return 100 * ran / all, true
}

// byUnit weighs a piece in its report's own unit.
func byUnit(p covPiece) (float64, float64) { return p.total, p.covered }

// onePerPiece weighs every piece one, covered when any of it ran.
func onePerPiece(p covPiece) (float64, float64) {
	if p.covered > 0 {
		return 1, 1
	}
	return 1, 0
}

// lcovLines are the pieces of an LCOV record: a line and its hits for each
// pair.
func lcovLines(lineHits ...[2]int) []covPiece {
	var out []covPiece
	for _, lh := range lineHits {
		p := covPiece{line: lh[0], total: 1, hits: lh[1]}
		if lh[1] > 0 {
			p.covered = 1
		}
		out = append(out, p)
	}
	return out
}

// lcovRecord is an LCOV record naming its file path: a DA line for each
// piece and a BRDA block for each decision.
func lcovRecord(path string, pieces []covPiece, decisions []covDecision) string {
	var b strings.Builder
	b.WriteString("SF:" + path + "\n")
	for _, p := range pieces {
		fmt.Fprintf(&b, "DA:%d,%d\n", p.line, p.hits)
	}
	for i, d := range decisions {
		for branch := range d.branches {
			fmt.Fprintf(&b, "BRDA:%d,%d,%d,%d\n", d.line, i, branch, hits(slices.Contains(d.taken, branch)))
		}
	}
	b.WriteString("end_of_record\n")
	return b.String()
}

// goRecords are the lines of a Go cover profile naming path: one for each
// block, its statements and whether it ran.
func goRecords(path string, pieces []covPiece) string {
	var b strings.Builder
	for _, p := range pieces {
		fmt.Fprintf(&b, "%s:%s %d %d\n", path, p.span, int(p.total), hits(p.covered > 0))
	}
	return b.String()
}

// jacocoReport is a JaCoCo report of one source file of package pkg: each
// piece a line of its instructions, and each decision the branches of its
// line.
func jacocoReport(pkg, file string, pieces []covPiece, decisions []covDecision) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>\n<report name=\"%s\">\n  <package name=\"%s\">\n    <sourcefile name=\"%s\">\n", pkg, pkg, file)
	for _, p := range pieces {
		mb, cb := 0, 0
		for _, d := range decisions {
			if d.line == p.line {
				mb, cb = d.branches-len(d.taken), len(d.taken)
			}
		}
		fmt.Fprintf(&b, "      <line nr=\"%d\" mi=\"%d\" ci=\"%d\" mb=\"%d\" cb=\"%d\"/>\n", p.line, int(p.total-p.covered), int(p.covered), mb, cb)
	}
	b.WriteString("    </sourcefile>\n  </package>\n</report>\n")
	return b.String()
}

// The sources the fixtures measure, each of one function whose lines the
// pieces name.

// goSource is a Go file of package pkg whose function Place runs from line
// 3 to 10, an if on line 5.
func goSource(pkg string) string {
	return "package " + pkg + "\n\nfunc Place(a int) int {\n\tn := 0\n\tif a > 1 {\n\t\tn = 1\n\t}\n\tn += 2\n\treturn n\n}\n"
}

// goBlocks are goSource's blocks as go test writes them, each covered as
// ran says: n := 0 and the if, the if's arm, and the two statements after
// it.
func goBlocks(ran ...bool) []covPiece {
	blocks := []covPiece{{line: 3, span: "3.23,5.11", total: 2}, {line: 5, span: "5.11,7.3", total: 1}, {line: 7, span: "7.3,9.10", total: 2}}
	for i := range blocks {
		if ran[i] {
			blocks[i].covered = blocks[i].total
		}
	}
	return blocks
}

// goLines are goSource's statement lines as LCOV measures them: all run but
// the if's arm, four of five.
var goLines = lcovLines([2]int{4, 1}, [2]int{5, 1}, [2]int{6, 0}, [2]int{8, 1}, [2]int{9, 1})

// tsSource is a TypeScript function place whose statements are on lines 2,
// 3, 4, 6 and 7, an if on line 3.
const tsSource = "export function place(a: number): number {\n  let n = 0;\n  if (a > 1) {\n    n = 1;\n  }\n  n += 2;\n  return n;\n}\n"

// ktSource is a Kotlin function place of package board, its statements on
// lines 4, 5, 6, 8, 9 and 11, ifs on lines 5 and 8.
const ktSource = "package board\n\nfun place(a: Int): Int {\n    var n = 0\n    if (a > 1) {\n        n = 1\n    }\n" +
	"    if (a > 2 && a < 9) {\n        n += 2\n    }\n    return n\n}\n"

// pySource is a Python function place whose body is on lines 2 to 5.
const pySource = "def place(a):\n    n = 0\n    if a > 1:\n        n = 1\n    return n\n"

// pyLines are pySource's body lines as coverage.py measures them: all run
// but the if's arm, three of four.
var pyLines = lcovLines([2]int{2, 1}, [2]int{3, 1}, [2]int{4, 0}, [2]int{5, 1})

// shapeOf is the source of the language of name's extension, its function's
// name and the LCOV pieces of a run of it, for the matching scenarios.
func shapeOf(name string) (source, function string, pieces []covPiece, err error) {
	switch filepath.Ext(name) {
	case ".go":
		return goSource(path.Base(path.Dir(name))), "Place", goLines, nil
	case ".py":
		return pySource, "place", pyLines, nil
	}
	return "", "", nil, fmt.Errorf("no source is written here for %s", name)
}

func coverageSteps(sc *godog.ScenarioContext, w *world) {
	c := &coverageState{w: w}
	sc.Before(func(ctx context.Context, s *godog.Scenario) (context.Context, error) {
		*c = coverageState{w: w, project: ".", runDir: ".", want: map[covFunction]float64{}, roots: map[string]string{}}
		if filepath.Base(s.Uri) != "coverage.feature" || len(s.Steps) == 0 {
			return ctx, nil
		}
		// The scenarios that only say When I run, or that coverage is run,
		// run in the sample.
		first := s.Steps[0].Text
		if m := regexp.MustCompile(`^I run "itos-cc ?([^"]*)"$`).FindStringSubmatch(first); m != nil {
			return ctx, c.sample(m[1])
		}
		if first == "coverage is run" {
			return ctx, c.sample("")
		}
		return ctx, nil
	})

	// Rule: Chosen files run only the tests that load them.
	sc.Step(`^a/a\.go, tested by its own package and by e2e/e2e_test\.go, which imports package a$`, c.e2eModule)
	sc.Step(`^slow/slow_test\.go, which never loads package a$`, c.slowTest)
	sc.Step(`^coverage runs "([^"]*)"$`, func(pattern string) error { return c.runs(pattern, "") })
	sc.Step(`^coverage runs "([^"]*)", without (\S+)$`, c.runs)
	sc.Step(`^its coverage is the same as with the whole module's tests$`, c.sameAsWholeModule)
	sc.Step(`^I run "itos-cc ?([^"]*)" in a Vitest or Jest project$`, c.runInVitestProject)
	sc.Step(`^coverage runs (\w+) ([^,]+), or (\w+) ([^,]+)$`, c.runsEither)
	sc.Step(`^coverage runs the whole suite of each build root$`, c.wholeSuite)

	// Rule: Each build root gets the language's own coverage command.
	sc.Step(`^a (Go|TypeScript|Python|Kotlin) project whose build root holds (\S+)$`, c.projectWithMarker)
	sc.Step(`^nothing else$`, c.nothingElse)
	sc.Step(`^a "coverage" script$`, c.coverageScript)
	sc.Step(`^vitest (\d+) or later as a dependency$`, c.vitestDependency)
	sc.Step(`^jest as a dependency$`, c.jestDependency)
	sc.Step(`^neither vitest nor jest$`, c.neitherVitestNorJest)
	sc.Step(`^pytest (importable|not importable)$`, c.pytest)
	sc.Step(`^the build (mentions|does not mention) kover$`, c.kover)
	sc.Step(`^it or a parent declares jacoco-maven-plugin$`, c.jacocoPlugin)
	sc.Step(`^coverage is run(?: for it)?$`, func() error { return w.runWith(w.env(), "crap") })
	sc.Step(`^the command is (.+)$`, c.commandIs)
	sc.Step(`^a TypeScript project in a workspace whose root declares "packageManager": "([^"]*)"$`, c.packageManagerWorkspace)
	sc.Step(`^<pm> is (\w+)$`, c.pmIs)
	sc.Step(`^a TypeScript project in a workspace, with its tools installed at the workspace root$`, c.toolsAtWorkspaceRoot)
	sc.Step(`^vitest, jest, or c8 runs from the nearest node_modules/\.bin$`, c.nearestNodeBin)
	sc.Step(`^npx is never used, so nothing that the project's lockfile does not pin is downloaded$`, c.noNpx)
	sc.Step(`^a TypeScript project using vitest that is not installed$`, func() error { return c.nodeToolMissing("vitest", "vitest", "") })
	sc.Step(`^a TypeScript project using vitest ([\d.]+) without @vitest/coverage-v8 installed$`, func(version string) error {
		return c.nodeToolMissing("vitest", "@vitest/coverage-v8", version)
	})
	sc.Step(`^a TypeScript project using jest that is not installed$`, func() error { return c.nodeToolMissing("jest", "jest", "") })
	sc.Step(`^a TypeScript project using neither vitest nor jest, without c8 installed$`, func() error { return c.nodeToolMissing("", "c8", "") })
	sc.Step(`^a TypeScript project using vitest ([\d.]+)$`, func(version string) error { return c.nodeToolMissing("vitest", "", version) })
	sc.Step(`^a Python project whose python cannot import coverage$`, c.pythonWithoutCoverage)
	sc.Step(`^a Kotlin project built by Maven, with no pom\.xml declaring jacoco-maven-plugin$`, c.mavenWithoutJacoco)
	sc.Step(`^nothing runs for it$`, c.nothingRuns)
	sc.Step(`^stderr says what to install, or to use --coverage-command$`, c.saysWhatToInstall)
	sc.Step(`^only complexity is reported( for it)?$`, c.onlyComplexity)
	sc.Step(`^a Python project with a \.venv directory$`, c.pythonVenv)
	sc.Step(`^the \.venv's python runs the tests$`, c.venvRunsTests)
	sc.Step(`^a Kotlin Gradle module under a build root that holds gradlew$`, c.gradleWrapper)
	sc.Step(`^the gradlew wrapper is used instead of gradle$`, c.wrapperUsed)
	sc.Step(`^a repository with a Go module and a TypeScript package$`, c.goAndTypeScript)
	sc.Step(`^each build root runs its own coverage command$`, c.eachRootRuns)
	sc.Step(`^the reports are merged into one$`, c.mergedIntoOne)
	sc.Step(`^a project whose tests fail but still write a coverage report$`, c.failingTests)
	sc.Step(`^the failure is logged to stderr$`, c.failureLogged)
	sc.Step(`^the report is still used$`, c.reportStillUsed)
	sc.Step(`^reports are written under \.metrics/coverage/$`, c.reportsUnderMetrics)
	sc.Step(`^\.metrics/coverage/\.gitignore ignores everything in it$`, c.gitignoreIgnoresAll)

	// Rule: The user can bring their own coverage.
	sc.Step(`^a Go project with a coverage\.out from an earlier run$`, c.earlierRun)
	sc.Step(`^no tests are run$`, c.noTestsRun)
	sc.Step(`^coverage is read from the first existing report for each build root$`, c.firstExistingReport)
	sc.Step(`^both reports are read, relative to the working directory$`, c.bothReportsRead)
	sc.Step(`^"([^"]*)" runs through the platform shell$`, c.runsThroughShell)
	sc.Step(`^(\S+) is read afterwards$`, c.readAfterwards)

	// Rule: Report formats are detected from their content.
	sc.Step(`^a report file "([^"]*)"$`, c.reportFile)
	sc.Step(`^it is loaded$`, c.computed)
	sc.Step(`^it is read as (.+)$`, c.readAs)
	sc.Step(`^it has one entry for "([^"]*)"$`, c.oneEntryFor)
	sc.Step(`^a (LCOV|Go cover profile|JaCoCo/Kover XML) report$`, c.formatReport)
	sc.Step(`^the covered share of (?:a function|pick|the file|a function in "\w+") is computed$`, c.computed)
	sc.Step(`^each measured piece is weighted by (.+)$`, c.weightedBy)

	// Rule: A function with branches is scored by the branches it took.
	sc.Step(`^a coverage\.py LCOV report for:$`, c.coveragePyReport)
	sc.Step(`^the tests call pick\((-?\d+)\) once$`, c.testsCallPick)
	sc.Step(`^it is (\d+)%, (\w+) of its (\w+) branches$`, c.branchesTaken)
	sc.Step(`^a decision is (.+)$`, c.decisionIs)
	sc.Step(`^a function whose lines hold no decision in the report$`, c.noDecision)
	sc.Step(`^its coverage is the share of its lines or statements that ran$`, c.shareOfLines)
	sc.Step(`^an LCOV report from c8, Node's test runner, or a Vitest older than 5$`, c.c8Report)
	sc.Step(`^it lists the function body as a block and leaves out the arm that ran$`, c.oneBranchBlocks)
	sc.Step(`^those one-branch blocks are ignored and its lines decide$`, c.linesDecide)

	// Rule: Report paths are matched to source files.
	sc.Step(`^a report entry whose path is absolute or relative to the report's base directory$`, c.exactEntries)
	sc.Step(`^a report entry "([^"]*)"$`, func(entry string) error {
		c.entries = append(c.entries, entry)
		return nil
	})
	sc.Step(`^a source file "([^"]*)"$`, c.sourceFile)
	sc.Step(`^sources "([^"]*)" only, as when --changed selects it$`, func(source string) error { return c.sourcesUnder(source) })
	sc.Step(`^a report entry "([^"]*)" or "([^"]*)", a file that exists$`, c.existingEntries)
	sc.Step(`^sources "([^"]*)" only, and a project file "([^"]*)"$`, c.sourceBesideProjectFile)
	sc.Step(`^a Go profile entry "([^"]*)"$`, func(entry string) error {
		c.entries, c.goFormat = append(c.entries, entry), true
		return nil
	})
	sc.Step(`^a source reached through a symlinked directory$`, c.symlinkedSource)
	sc.Step(`^a report entry that names the same file by the symlink's target$`, func() error {
		c.entries = append(c.entries, "real/board.go")
		return nil
	})
	sc.Step(`^sources "([^"]*)" and "([^"]*)"$`, func(a, b string) error { return c.sourcesUnder(a, b) })
	sc.Step(`^a report entry "([^"]*)" from a base directory that holds neither$`, c.entryFromBase)
	sc.Step(`^a report written with "(.)" separators and sources with "(.)" separators, or the reverse$`, c.separators)
	sc.Step(`^it is matched to sources$`, c.matchToSources)
	sc.Step(`^it matches that source(?: file exactly| by its trailing path components)?$`, c.matches)
	sc.Step(`^the paths still match$`, c.matches)
	sc.Step(`^it matches neither source$`, c.matchesNeither)
	sc.Step(`^a report entry that matches no selected source$`, c.unmatchedEntry)
	sc.Step(`^the report is built$`, c.computed)
	sc.Step(`^that entry is ignored$`, c.entryIgnored)

	// Rule: A piece of code listed more than once counts once.
	sc.Step(`^a module with packages "(\w+)", "(\w+)", and "(\w+)", each fully covered by its own tests$`, c.threePackages)
	sc.Step(`^a profile from go test -coverpkg=\./\.\.\., which lists every block once per test binary$`, c.everyBinaryProfile)
	sc.Step(`^it is (\d+)%, as go tool cover -func says$`, c.asGoToolCover)
	sc.Step(`^two reports given with --coverage-report, such as unit and integration runs$`, c.twoReports)
	sc.Step(`^each covers a line of a file the other does not$`, c.eachCoversALine)
	sc.Step(`^a line or block is covered when any report covered it$`, c.anyReportCovers)
	sc.Step(`^a decision counts the branches of the report that took the most$`, c.mostBranches)

	// Rule: A function's coverage starts where calling it starts.
	sc.Step(`^its coverage starts at line (\d+)(?:, not at its first line (\d+))?$`, c.startsAt)

	// Rule: What itos-cc says while it measures.
	sc.Step(`^stderr has a line "([^"]*)" naming the command coverage runs$`, c.announcesCommand)
	sc.Step(`^no line of stderr begins "([^"]*)"$`, c.noLineBegins)
	sc.Step(`^a test of a/a\.go fails$`, c.failingSampleTest)
	sc.Step(`^stderr has a line beginning "([^"]*)" that gives the command's exit status$`, c.exitStatusLine)
}

// Writing projects.

// writeIn writes files, by slash-separated path, in the folder dir of the
// scratch folder.
func (c *coverageState) writeIn(dir string, files map[string]string) error {
	for name, text := range files {
		if err := c.w.write(path.Join(dir, name), text); err != nil {
			return err
		}
	}
	return nil
}

// abs is the absolute path of rel, slash-separated from the scratch folder,
// in the system's form.
func (c *coverageState) abs(rel string) string {
	return filepath.Join(c.w.dir, filepath.FromSlash(rel))
}

// declare declares what the project's own tools measure in the folder dir
// of the scratch folder, a build root of language.
func (c *coverageState) declare(dir string, cov Coverage) error {
	c.roots[dir] = cov.Language
	return c.w.d.Coverage(c.abs(dir), cov)
}

// markerTest is a Go test of package pkg that writes test-ran in its
// package's folder when it runs, then calls Place with a.
func markerTest(pkg string, a int) string {
	return "package " + pkg + "\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestPlace(t *testing.T) {\n" +
		"\t_ = os.WriteFile(\"test-ran\", nil, 0o644)\n" + fmt.Sprintf("\tif Place(%d) < 0 {\n\t\tt.Fatal(\"Place\")\n\t}\n}\n", a)
}

// sampleTest is the sample's test of package a: it writes test-ran in a/
// when it runs, then expects A(1) to be want.
func sampleTest(want int) string {
	return "package a\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestA(t *testing.T) {\n" +
		"\t_ = os.WriteFile(\"test-ran\", nil, 0o644)\n" + fmt.Sprintf("\tif A(1) != %d {\n\t\tt.Fatal(\"A\")\n\t}\n}\n", want)
}

// sampleA is the sample's a/a.go: A runs from line 3 to 8, an if on line 4.
const sampleA = "package a\n\nfunc A(x int) int {\n\tif x > 0 {\n\t\treturn 1\n\t}\n\treturn 0\n}\n"

// sampleRecipe is what the sample's Makefile's cover target runs.
const sampleRecipe = "go test -coverprofile=cover.out ./..."

// sample is the project the scenarios that only say When I run, or that
// coverage is run, run in: the Go module example.com/m, whose a/a.go its
// test covers and b/b.go no test runs, beside a/lcov.info and
// b/coverage.out, reports a person wrote of each, and a Makefile whose
// cover target writes cover.out; and a second module, example.com/other, in
// other/. Each test writes test-ran in its folder when it runs. args is the
// run's words: a --coverage-command run with --coverage-report declares the
// program it runs.
func (c *coverageState) sample(args string) error {
	if err := c.writeIn(".", map[string]string{
		"go.mod":          "module example.com/m\n\ngo 1.22\n",
		"a/a.go":          sampleA,
		"a/a_test.go":     sampleTest(1),
		"b/b.go":          "package b\n\nfunc B(x int) int {\n\treturn x + 1\n}\n",
		"a/lcov.info":     lcovRecord("a/a.go", lcovLines([2]int{4, 1}, [2]int{5, 1}, [2]int{7, 0}), nil),
		"b/coverage.out":  "mode: set\nexample.com/m/b/b.go:3.20,5.2 1 1\n",
		"Makefile":        "cover:\n\t" + sampleRecipe + "\n",
		"other/go.mod":    "module example.com/other\n\ngo 1.22\n",
		"other/o.go":      goSource("other"),
		"other/o_test.go": markerTest("other", 5),
	}); err != nil {
		return err
	}
	c.markers = []string{"a/test-ran", "other/test-ran"}
	c.want[covFunction{"a/a.go", "A"}] = 100 * 2.0 / 3
	c.want[covFunction{"b/b.go", "B"}] = 100
	words, err := commandWords(args)
	if err != nil {
		return err
	}
	program := ""
	if i := slices.Index(words, "--coverage-command"); i >= 0 && i+1 < len(words) && slices.Contains(words, "--coverage-report") {
		program, _, _ = strings.Cut(words[i+1], " ")
	}
	if err := c.declare(".", Coverage{Language: "go", Program: program}); err != nil {
		return err
	}
	return c.declare("other", Coverage{Language: "go"})
}

// failingSampleTest writes the sample, its test of a/a.go expecting what A
// does not give.
func (c *coverageState) failingSampleTest() error {
	if err := c.sample(""); err != nil {
		return err
	}
	return c.w.write("a/a_test.go", sampleTest(2))
}

// e2eModule writes the module example.com/m whose a/a.go its own test
// covers one arm of and e2e/e2e_test.go, importing package a, the other.
func (c *coverageState) e2eModule() error {
	if err := c.writeIn(".", map[string]string{
		"go.mod":      "module example.com/m\n\ngo 1.22\n",
		"a/a.go":      sampleA,
		"a/a_test.go": sampleTest(1),
		"e2e/e2e_test.go": "package e2e\n\nimport (\n\t\"testing\"\n\n\t\"example.com/m/a\"\n)\n\n" +
			"func TestE2E(t *testing.T) {\n\tif a.A(-1) != 0 {\n\t\tt.Fatal(\"A\")\n\t}\n}\n",
	}); err != nil {
		return err
	}
	return c.declare(".", Coverage{Language: "go"})
}

// slowTest writes slow/slow_test.go, a test importing nothing of the
// module.
func (c *coverageState) slowTest() error {
	text := "package slow\n\nimport \"testing\"\n\nfunc TestSlow(t *testing.T) {\n\tt.Log(\"slow\")\n}\n"
	if strings.Contains(text, "example.com/m/a") {
		return errors.New("slow/slow_test.go loads package a")
	}
	return c.w.write("slow/slow_test.go", text)
}

// vitestProject writes a TypeScript project in the folder dir of the
// scratch folder using Vitest and its v8 provider, its test of
// src/board.ts calling place, and declares Vitest measures it, linking the
// pinned node_modules there unless modules says they are elsewhere.
func (c *coverageState) vitestProject(dir string, modules bool) error {
	if err := c.writeIn(dir, map[string]string{
		"package.json": `{"name": "board", "type": "module", "devDependencies": {"vitest": "5.0.2", "@vitest/coverage-v8": "5.0.2"}}` + "\n",
		"src/board.ts": tsSource,
		"src/board.test.ts": "import { expect, test } from \"vitest\";\nimport { place } from \"./board\";\n\n" +
			"test(\"place\", () => {\n  expect(place(5)).toBe(3);\n});\n",
	}); err != nil {
		return err
	}
	if !modules {
		c.roots[dir] = "typescript"
		return nil
	}
	return c.declare(dir, Coverage{Language: "typescript"})
}

// runInVitestProject writes a Vitest project and runs itos-cc there with
// args.
func (c *coverageState) runInVitestProject(args string) error {
	if err := c.vitestProject(".", true); err != nil {
		return err
	}
	return c.w.runWith(c.w.env(), args)
}

// projectWithMarker writes a project of language at the scratch folder
// holding marker, with a source and, but for Kotlin, a test of it. Go and
// Python declare their tools; a Kotlin project's runs find neither Gradle
// nor Maven, so its command is chosen and never runs.
func (c *coverageState) projectWithMarker(language, marker string) error {
	files := map[string]string{}
	switch language {
	case "Go":
		files["go.mod"], files["board.go"], files["board_test.go"] = "module example.com/m\n\ngo 1.22\n", goSource("board"), markerTest("board", 5)
	case "TypeScript":
		files["package.json"], files["src/board.ts"] = `{"name": "board"}`+"\n", tsSource
	case "Python":
		files[marker], files["board.py"], files["test_board.py"] = pythonMarker(marker), pySource, pythonTest
	case "Kotlin":
		files[marker], files["src/main/kotlin/board/Board.kt"] = kotlinBuild(marker), ktSource
		c.w.without = []string{"gradle", "mvn"}
	}
	if _, ok := files[marker]; !ok {
		return fmt.Errorf("no %s project is written whose build root holds %s", language, marker)
	}
	if err := c.writeIn(".", files); err != nil {
		return err
	}
	switch language {
	case "Go":
		return c.declare(".", Coverage{Language: "go"})
	case "TypeScript":
		c.roots["."], c.pm = "typescript", "npm"
	case "Kotlin":
		c.roots["."] = "kotlin"
	}
	return nil
}

// pythonTest is a test of pySource's place, as unittest and pytest both run
// it.
const pythonTest = "import unittest\n\nfrom board import place\n\n\nclass TestPlace(unittest.TestCase):\n" +
	"    def test_place(self):\n        self.assertEqual(place(5), 1)\n"

// pythonMarker is a Python build root's marker file.
func pythonMarker(marker string) string {
	if marker == "setup.py" {
		return "from setuptools import setup\n\nsetup(name=\"board\")\n"
	}
	return "[project]\nname = \"board\"\nversion = \"0.1.0\"\n"
}

// kotlinBuild is a Kotlin build file, mentioning neither Kover nor JaCoCo.
func kotlinBuild(marker string) string {
	switch marker {
	case "build.gradle.kts":
		return "plugins {\n    kotlin(\"jvm\") version \"2.2.0\"\n}\n"
	case "build.gradle":
		return "plugins {\n    id 'org.jetbrains.kotlin.jvm' version '2.2.0'\n}\n"
	}
	return "<project>\n  <modelVersion>4.0.0</modelVersion>\n  <groupId>board</groupId>\n  <artifactId>board</artifactId>\n  <version>1</version>\n</project>\n"
}

// buildMarkers are the files that make a folder a build root.
var buildMarkers = []string{"go.mod", "package.json", "pyproject.toml", "setup.py", "setup.cfg", "build.gradle.kts", "build.gradle", "pom.xml"}

// nothingElse is whether the project's folder holds no build root's marker
// but its own and no Makefile or package.json.
func (c *coverageState) nothingElse() error {
	n := 0
	for _, marker := range append(buildMarkers, "Makefile") {
		if c.w.d.Exists(c.abs(path.Join(c.project, marker))) {
			n++
		}
	}
	if n != 1 {
		return fmt.Errorf("the project's folder holds %d build files, not its one marker", n)
	}
	return nil
}

// packageJSON writes the TypeScript project's package.json with scripts
// and devDependencies.
func (c *coverageState) packageJSON(dir string, scripts, deps map[string]string) error {
	object := map[string]any{"name": "board"}
	if scripts != nil {
		object["scripts"] = scripts
	}
	if deps != nil {
		object["devDependencies"] = deps
	}
	data, err := json.Marshal(object)
	if err != nil {
		return err
	}
	return c.w.write(path.Join(dir, "package.json"), string(data)+"\n")
}

// coverageScript gives the project a coverage script, which writes
// coverage/lcov.info: place's statements all run but the if's arm.
func (c *coverageState) coverageScript() error {
	lcov := lcovRecord("src/board.ts", lcovLines([2]int{2, 1}, [2]int{3, 1}, [2]int{4, 0}, [2]int{6, 1}, [2]int{7, 1}), nil)
	script := "const fs = require(\"fs\");\nfs.mkdirSync(\"coverage\", { recursive: true });\nfs.writeFileSync(\"coverage/lcov.info\", " +
		strconv.Quote(lcov) + ");\n"
	if err := c.packageJSON(".", map[string]string{"coverage": "node write-lcov.cjs"}, nil); err != nil {
		return err
	}
	if err := c.w.write("write-lcov.cjs", script); err != nil {
		return err
	}
	c.want[covFunction{"src/board.ts", "place"}] = 80
	return c.declare(".", Coverage{Language: "typescript", Report: lcov})
}

// vitestDependency makes Vitest of the pinned version, major or later, the
// project's, with its v8 provider and a test of place.
func (c *coverageState) vitestDependency(major int) error {
	const pinned = "5.0.2"
	if n, _ := strconv.Atoi(strings.Split(pinned, ".")[0]); n < major {
		return fmt.Errorf("the pinned Vitest %s is older than %d", pinned, major)
	}
	return c.vitestProject(".", true)
}

// placeholder writes an installed package name of version at dir's
// node_modules, its executable a file that cannot run: what itos-cc finds
// installed, where no pinned tool set has it.
func (c *coverageState) placeholder(dir, name, version string) error {
	return c.writeIn(path.Join(dir, "node_modules"), map[string]string{
		name + "/package.json":  fmt.Sprintf(`{"name": %q, "version": %q}`, name, version) + "\n",
		".bin/" + name:          "placeholder: no " + name + " runs here\n",
		".bin/" + name + ".cmd": "placeholder: no " + name + " runs here\n",
	})
}

// jestDependency makes Jest the project's, installed as a placeholder.
func (c *coverageState) jestDependency() error {
	if err := c.packageJSON(".", nil, map[string]string{"jest": "30.0.0"}); err != nil {
		return err
	}
	return c.placeholder(".", "jest", "30.0.0")
}

// neitherVitestNorJest gives the project a test script and no test runner,
// and declares the pinned tools, c8 among them.
func (c *coverageState) neitherVitestNorJest() error {
	if err := c.packageJSON(".", map[string]string{"test": "node -e \"\""}, nil); err != nil {
		return err
	}
	data, err := c.w.readIn(c.w.dir, "package.json")
	if err != nil {
		return err
	}
	if strings.Contains(string(data), "vitest") || strings.Contains(string(data), "jest") {
		return fmt.Errorf("package.json names vitest or jest: %s", data)
	}
	return c.declare(".", Coverage{Language: "typescript"})
}

// pytest declares python3's pytest and coverage, and where pytest is not
// importable, writes the project a pytest.py that cannot be imported.
func (c *coverageState) pytest(which string) error {
	if which == "not importable" {
		if err := c.w.write("pytest.py", "raise ImportError(\"pytest is not importable in this project\")\n"); err != nil {
			return err
		}
	}
	return c.declare(".", Coverage{Language: "python"})
}

// kover adds the Kover plugin to the Gradle build, or checks it is not
// there.
func (c *coverageState) kover(which string) error {
	marker := "build.gradle.kts"
	if !c.w.d.Exists(c.abs(marker)) {
		marker = "build.gradle"
	}
	data, err := c.w.readIn(c.w.dir, marker)
	if err != nil {
		return err
	}
	mentions := strings.Contains(string(data), "kover")
	switch {
	case which == "does not mention" && mentions:
		return fmt.Errorf("%s mentions kover", marker)
	case which == "does not mention":
		return nil
	}
	return c.w.write(marker, strings.Replace(string(data), "\n}", "\n    id(\"org.jetbrains.kotlinx.kover\") version \"0.9.1\"\n}", 1))
}

// jacocoPlugin declares jacoco-maven-plugin in the project's pom.xml.
func (c *coverageState) jacocoPlugin() error {
	return c.w.write("pom.xml", strings.Replace(kotlinBuild("pom.xml"), "</project>",
		"  <build>\n    <plugins>\n      <plugin>\n        <groupId>org.jacoco</groupId>\n"+
			"        <artifactId>jacoco-maven-plugin</artifactId>\n        <version>0.8.13</version>\n"+
			"      </plugin>\n    </plugins>\n  </build>\n</project>", 1))
}

// packageManagerWorkspace writes a workspace whose root package.json
// declares manager, and in packages/app a project with a coverage script.
// Its runs find no manager, so its script never runs.
func (c *coverageState) packageManagerWorkspace(manager string) error {
	name, _, _ := strings.Cut(manager, "@")
	root, err := json.Marshal(map[string]any{"name": "workspace", "private": true, "packageManager": manager, "workspaces": []string{"packages/*"}})
	if err != nil {
		return err
	}
	c.project, c.w.without = "packages/app", []string{name}
	if err := c.writeIn(".", map[string]string{"package.json": string(root) + "\n", "packages/app/src/board.ts": tsSource}); err != nil {
		return err
	}
	c.roots[c.project] = "typescript"
	return c.packageJSON(c.project, map[string]string{"coverage": "node -e \"\""}, nil)
}

// toolsAtWorkspaceRoot writes a workspace whose root holds the pinned
// node_modules, and in packages/app a Vitest project without its own.
func (c *coverageState) toolsAtWorkspaceRoot() error {
	if err := c.w.write("package.json", `{"name": "workspace", "private": true, "workspaces": ["packages/*"]}`+"\n"); err != nil {
		return err
	}
	c.project = "packages/app"
	if err := c.vitestProject(c.project, false); err != nil {
		return err
	}
	if err := c.w.d.Coverage(c.w.dir, Coverage{Language: "typescript"}); err != nil {
		return err
	}
	if c.w.d.Exists(c.abs(c.project + "/node_modules")) {
		return errors.New("the project holds a node_modules of its own")
	}
	return nil
}

// nodeToolMissing writes a TypeScript project using runner, "" for no
// runner, without missing, the tool itos-cc names as not installed. A
// version installs runner as a placeholder, its own provider missing.
func (c *coverageState) nodeToolMissing(runner, missing, version string) error {
	c.missing, c.roots["."] = missing, "typescript"
	if missing == "" {
		c.missing = runner
	}
	deps := map[string]string{}
	if runner != "" {
		deps[runner] = orElse(version, "5.0.2")
	}
	if err := c.packageJSON(".", nil, deps); err != nil {
		return err
	}
	if err := c.w.write("src/board.ts", tsSource); err != nil {
		return err
	}
	if version == "" {
		return nil
	}
	return c.placeholder(".", runner, version)
}

// orElse is s, or otherwise when s is "".
func orElse(s, otherwise string) string {
	if s == "" {
		return otherwise
	}
	return s
}

// pythonWithoutCoverage writes a Python project whose own .venv's python
// cannot run, so it imports nothing.
func (c *coverageState) pythonWithoutCoverage() error {
	c.missing, c.roots["."] = "coverage", "python"
	return c.writeIn(".", map[string]string{
		"pyproject.toml": pythonMarker("pyproject.toml"), "board.py": pySource, "test_board.py": pythonTest,
		".venv/bin/python": "placeholder: this python cannot run\n",
	})
}

// mavenWithoutJacoco writes a Kotlin module built by Maven whose pom.xml
// declares no JaCoCo; its runs find neither Gradle nor Maven.
func (c *coverageState) mavenWithoutJacoco() error {
	c.missing, c.roots["."], c.w.without = "jacoco-maven-plugin", "kotlin", []string{"gradle", "mvn"}
	return c.writeIn(".", map[string]string{"pom.xml": kotlinBuild("pom.xml"), "src/main/kotlin/board/Board.kt": ktSource})
}

// pythonVenv writes a Python project and declares its .venv's python.
func (c *coverageState) pythonVenv() error {
	if err := c.writeIn(".", map[string]string{"pyproject.toml": pythonMarker("pyproject.toml"), "board.py": pySource, "test_board.py": pythonTest}); err != nil {
		return err
	}
	return c.declare(".", Coverage{Language: "python", Venv: true})
}

// gradleWrapper writes a build root holding gradlew, a placeholder, and the
// Kotlin Gradle module app under it; its runs find neither Gradle nor
// Maven.
func (c *coverageState) gradleWrapper() error {
	c.project, c.w.without = "app", []string{"gradle", "mvn"}
	c.roots["app"] = "kotlin"
	return c.writeIn(".", map[string]string{
		"gradlew":                            "placeholder: no Gradle runs here\n",
		"app/build.gradle.kts":               kotlinBuild("build.gradle.kts"),
		"app/src/main/kotlin/board/Board.kt": ktSource,
	})
}

// goAndTypeScript writes a git repository holding the Go module api in
// api/ and a Vitest package in web/, and declares each one's tools.
func (c *coverageState) goAndTypeScript() error {
	if err := c.w.git("init"); err != nil {
		return err
	}
	if err := c.writeIn("api", map[string]string{"go.mod": "module example.com/api\n\ngo 1.22\n", "api.go": goSource("api"), "api_test.go": markerTest("api", 5)}); err != nil {
		return err
	}
	if err := c.declare("api", Coverage{Language: "go"}); err != nil {
		return err
	}
	return c.vitestProject("web", true)
}

// failingTests writes a Go module whose test runs Place, all but its if's
// arm, and then fails.
func (c *coverageState) failingTests() error {
	if err := c.writeIn(".", map[string]string{
		"go.mod":   "module example.com/m\n\ngo 1.22\n",
		"board.go": goSource("board"),
		"board_test.go": "package board\n\nimport \"testing\"\n\nfunc TestPlace(t *testing.T) {\n" +
			"\tif Place(0) != 2 {\n\t\tt.Fatal(\"Place\")\n\t}\n\tt.Fatal(\"this test fails after it ran Place\")\n}\n",
	}); err != nil {
		return err
	}
	return c.declare(".", Coverage{Language: "go"})
}

// earlierRun writes a Go project of two modules, each with reports from an
// earlier run that differ from what its tests measure, which write
// test-ran when they run: one/ holds coverage.out and cover.out, two/
// cover.out alone.
func (c *coverageState) earlierRun() error {
	for _, m := range []struct {
		dir     string
		reports map[string][]bool
		first   string
	}{
		{"one", map[string][]bool{"coverage.out": {true, false, false}, "cover.out": {true, true, true}}, "coverage.out"},
		{"two", map[string][]bool{"cover.out": {true, true, false}}, "cover.out"},
	} {
		files := map[string]string{
			"go.mod": "module example.com/" + m.dir + "\n\ngo 1.22\n", m.dir + ".go": goSource(m.dir), m.dir + "_test.go": markerTest(m.dir, 5),
		}
		for name, ran := range m.reports {
			files[name] = "mode: set\n" + goRecords("example.com/"+m.dir+"/"+m.dir+".go", goBlocks(ran...))
		}
		if err := c.writeIn(m.dir, files); err != nil {
			return err
		}
		c.want[covFunction{m.dir + "/" + m.dir + ".go", "Place"}], _ = share(goBlocks(m.reports[m.first]...), byUnit)
		c.markers = append(c.markers, m.dir+"/test-ran")
		if err := c.declare(m.dir, Coverage{Language: "go"}); err != nil {
			return err
		}
	}
	return nil
}

// Reading what itos-cc said and printed.

// covAnnouncement is a command itos-cc announced it runs to measure
// coverage: the folder it runs in, slash-separated from the scratch folder,
// "" for --coverage-command's, and its words.
type covAnnouncement struct {
	dir, command string
	words        []string
}

// announced are the coverage commands stderr announces.
func (c *coverageState) announced(stderr string) []covAnnouncement {
	var out []covAnnouncement
	for _, line := range strings.Split(strings.ReplaceAll(stderr, "\r\n", "\n"), "\n") {
		rest, ok := strings.CutPrefix(line, "itos-cc: coverage ")
		if !ok {
			continue
		}
		dir, command, ok := strings.Cut(rest, "$ ")
		if !ok {
			continue
		}
		a := covAnnouncement{command: command, words: strings.Fields(command)}
		if dir = strings.TrimSpace(dir); dir != "" {
			if a.dir, ok = c.rel(dir); !ok {
				a.dir = "?" + dir
			}
		}
		out = append(out, a)
	}
	return out
}

// rel is p, a path itos-cc printed, slash-separated from the scratch folder:
// found by the scratch folder's name, as the system may name the folder
// through another path (/private/var for /var, a short name on windows);
// false when p is not in it.
func (c *coverageState) rel(p string) (string, bool) {
	p = filepath.ToSlash(strings.Trim(p, `"`))
	base := "/" + filepath.Base(c.w.dir)
	i := strings.LastIndex(p, base)
	if i < 0 {
		return "", false
	}
	switch rest := p[i+len(base):]; {
	case rest == "":
		return ".", true
	case strings.HasPrefix(rest, "/"):
		return rest[1:], true
	}
	return "", false
}

// program is a command's program as its first word names it, without the
// extension windows runs it by.
func program(word string) string {
	base := path.Base(filepath.ToSlash(strings.Trim(word, `"`)))
	for _, ext := range []string{".cmd", ".exe", ".bat"} {
		if strings.HasSuffix(strings.ToLower(base), ext) {
			return base[:len(base)-len(ext)]
		}
	}
	return base
}

// commandPattern is a command as a scenario writes it, "..." standing for
// any words, or any text within a word, as a regular expression over the
// whole command.
func commandPattern(text string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i, token := range strings.Fields(text) {
		if token == "..." {
			b.WriteString(`(?: \S+)*`)
			continue
		}
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(strings.ReplaceAll(regexp.QuoteMeta(token), `\.\.\.`, ".*"))
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// wordPattern is a word as a scenario writes it, "..." standing for any
// text.
func wordPattern(word string) *regexp.Regexp {
	return regexp.MustCompile("^" + strings.ReplaceAll(regexp.QuoteMeta(word), `\.\.\.`, ".*") + "$")
}

// listed is what the last run listed, from its object or its table.
func (c *coverageState) listed() ([]crapEntry, error) {
	return (&crapState{w: c.w}).listed()
}

// coverageOf is the coverage the entries give f, nil for none; an error
// when they list no such function.
func coverageOf(entries []crapEntry, f covFunction) (*float64, error) {
	for _, e := range entries {
		if filepath.ToSlash(e.File) == f.file && e.Name == f.name {
			return e.Coverage, nil
		}
	}
	return nil, fmt.Errorf("%s of %s is not listed", f.name, f.file)
}

// hasCoverage is whether the entries give f want, as crap shows it.
func (c *coverageState) hasCoverage(entries []crapEntry, f covFunction, want float64) error {
	got, err := coverageOf(entries, f)
	if err != nil {
		return fmt.Errorf("%v\n%s", err, c.w.report())
	}
	if shown(got) != shown(&want) {
		return fmt.Errorf("%s of %s has %s%% coverage, not %s%%\n%s", f.name, f.file, shown(got), shown(&want), c.w.report())
	}
	return nil
}

// Then steps of the commands run.

// runs is whether a coverage command of the last run is pattern, as the
// scenario writes it, with no word beginning without when that is not "".
func (c *coverageState) runs(pattern, without string) error {
	re := commandPattern(pattern)
	for _, a := range c.announced(c.w.stderr) {
		if !re.MatchString(a.command) {
			continue
		}
		if without != "" && slices.ContainsFunc(a.words, func(w string) bool { return strings.HasPrefix(w, without) }) {
			return fmt.Errorf("coverage runs %q, with %s\n%s", a.command, without, c.w.report())
		}
		return nil
	}
	return fmt.Errorf("no coverage command is %q\n%s", pattern, c.w.report())
}

// sameAsWholeModule is whether the functions of a/a.go, of the coverage
// the last run gave them, have the same in a run of the whole module's
// tests, which covers them.
func (c *coverageState) sameAsWholeModule() error {
	entries, err := c.listed()
	if err != nil {
		return err
	}
	code, stdout, stderr, err := c.w.capture(c.w.dir, c.w.env(), "crap", "--json", "--all-tests", "a/a.go")
	if err != nil {
		return err
	}
	report := fmt.Sprintf("%s\n--- with --all-tests, exit %d\n--- stdout\n%s--- stderr\n%s", c.w.report(), code, stdout, stderr)
	whole := false
	for _, a := range c.announced(stderr) {
		whole = whole || slices.Equal(a.words[len(a.words)-2:], []string{"-coverpkg=./...", "./..."})
	}
	var object crapObject
	if err := json.Unmarshal([]byte(stdout), &object); err != nil || code != 0 || !whole {
		return fmt.Errorf("the whole module's tests did not measure a/a.go\n%s", report)
	}
	n := 0
	for _, e := range object.Entries {
		got, err := coverageOf(entries, covFunction{filepath.ToSlash(e.File), e.Name})
		switch {
		case err != nil:
			return fmt.Errorf("%v\n%s", err, report)
		case e.Coverage == nil:
			return fmt.Errorf("the whole module's tests do not measure %s\n%s", e.id(), report)
		case shown(got) != shown(e.Coverage):
			return fmt.Errorf("%s has %s%% coverage, and %s%% with the whole module's tests\n%s", e.id(), shown(got), shown(e.Coverage), report)
		}
		n++
	}
	if n == 0 {
		return fmt.Errorf("no function of a/a.go is scored\n%s", report)
	}
	return nil
}

// runsEither is whether a coverage command of the last run is one tool's
// with those words, or the other's: its program is the tool, and the words
// follow it in a row.
func (c *coverageState) runsEither(tool, words, other, otherWords string) error {
	for _, a := range c.announced(c.w.stderr) {
		for _, want := range [][2]string{{tool, words}, {other, otherWords}} {
			if program(a.words[0]) == want[0] && strings.Contains(strings.Join(a.words[1:], " ")+" ", want[1]+" ") {
				return nil
			}
		}
	}
	return fmt.Errorf("coverage runs neither %s %s nor %s %s\n%s", tool, words, other, otherWords, c.w.report())
}

// selectedRoots are the build roots of the sources the last run named, or
// every root when it named none.
func (c *coverageState) selectedRoots() ([]string, error) {
	if c.w.last == nil {
		return nil, errors.New("no run of itos-cc to read")
	}
	var paths []string
	for _, a := range c.w.last.args {
		if !strings.HasPrefix(a, "-") && !slices.Contains([]string{"crap", "mutation", "run"}, a) {
			paths = append(paths, filepath.ToSlash(a))
		}
	}
	var roots []string
	for root := range c.roots {
		if len(paths) == 0 {
			roots = append(roots, root)
		}
	}
	for _, p := range paths {
		best := "."
		for root := range c.roots {
			if root != "." && strings.HasPrefix(p, root+"/") && len(root) > len(best) {
				best = root
			}
		}
		if !slices.Contains(roots, best) {
			roots = append(roots, best)
		}
	}
	slices.Sort(roots)
	return roots, nil
}

// wholeSuite is whether the last run's coverage commands are one for each
// build root of the sources it named, each running that root's every
// package: go test over ./....
func (c *coverageState) wholeSuite() error {
	roots, err := c.selectedRoots()
	if err != nil {
		return err
	}
	var dirs []string
	for _, a := range c.announced(c.w.stderr) {
		if c.roots[a.dir] != "go" || !slices.Equal(a.words[:2], []string{"go", "test"}) {
			return fmt.Errorf("no whole suite is known for %q in %s\n%s", a.command, a.dir, c.w.report())
		}
		if !slices.Equal(a.words[len(a.words)-2:], []string{"-coverpkg=./...", "./..."}) {
			return fmt.Errorf("coverage in %s runs %q, not every package\n%s", a.dir, a.command, c.w.report())
		}
		dirs = append(dirs, a.dir)
	}
	slices.Sort(dirs)
	if !slices.Equal(dirs, roots) {
		return fmt.Errorf("coverage runs in %v, not once in each build root of %v\n%s", dirs, roots, c.w.report())
	}
	return nil
}

// commands are the coverage commands of the last run announced for the
// project's folder.
func (c *coverageState) commands() []covAnnouncement {
	var out []covAnnouncement
	for _, a := range c.announced(c.w.stderr) {
		if a.dir == c.project {
			out = append(out, a)
		}
	}
	return out
}

// commandIs is whether the project's coverage commands are what spec says:
// commands joined by ", then ", each held by a command after the one
// holding the one before. A command holds every word of its spec, <pm>
// standing for the project's package manager, but where it names the
// installed tool, which its program must be, from a node_modules/.bin: then
// run is the tool's next word, "with the v8 provider" Vitest's own
// announcement of it, around the words it ends with, and the rest words it
// holds. Writing LCOV, it asks for an LCOV reporter; read from a path, that
// report is there after the run and gives each function what the
// project's script wrote in it.
func (c *coverageState) commandIs(spec string) error {
	commands := c.commands()
	if len(commands) == 0 {
		return fmt.Errorf("no coverage command runs for the project\n%s", c.w.report())
	}
	next := 0
	for _, part := range strings.Split(spec, ", then ") {
		part, readFrom, _ := strings.Cut(part, ", read from ")
		part, lcov := strings.CutSuffix(strings.TrimSuffix(part, ","), " writing LCOV")
		part = strings.TrimSuffix(strings.ReplaceAll(part, "<pm>", c.pm), ",")
		i := slices.IndexFunc(commands[next:], func(a covAnnouncement) bool { return c.holds(a, part, lcov) == nil })
		if i < 0 {
			reason := errors.New("no command")
			if next < len(commands) {
				reason = c.holds(commands[next], part, lcov)
			}
			return fmt.Errorf("no coverage command is %s: %v\n%s", part, reason, c.w.report())
		}
		next += i + 1
		if readFrom != "" {
			if err := c.readFromReport(readFrom); err != nil {
				return err
			}
		}
	}
	return nil
}

// holds is why a is not the command spec says, nil when it is.
func (c *coverageState) holds(a covAnnouncement, spec string, lcov bool) error {
	words := a.words
	if lcov && !slices.ContainsFunc(words, func(w string) bool { return regexp.MustCompile(`[Rr]eporters?=lcov$`).MatchString(w) }) {
		return fmt.Errorf("%q asks for no LCOV reporter", a.command)
	}
	if rest, ok := strings.CutPrefix(spec, "the installed "); ok {
		tool, rest, _ := strings.Cut(rest, " ")
		bin, inModules := c.rel(path.Dir(filepath.ToSlash(words[0])))
		if program(words[0]) != tool || !inModules || path.Base(bin) != ".bin" || path.Base(path.Dir(bin)) != "node_modules" {
			return fmt.Errorf("%q does not run the installed %s", a.command, tool)
		}
		words = words[1:]
		if after, ok := strings.CutPrefix(rest, "run "); ok {
			if len(words) == 0 || words[0] != "run" {
				return fmt.Errorf("%q does not run %s run", a.command, tool)
			}
			rest = after
		}
		if provider, ok := strings.CutPrefix(rest, "with the "); ok {
			provider = strings.TrimSuffix(provider, " provider")
			if !strings.Contains(c.w.stderr, "Coverage enabled with "+provider) {
				return fmt.Errorf("%s does not say coverage is enabled with %s", tool, provider)
			}
			return nil
		}
		if around, ok := strings.CutPrefix(rest, "around "); ok {
			tail := strings.Fields(around)
			if len(words) < len(tail) || !slices.Equal(words[len(words)-len(tail):], tail) {
				return fmt.Errorf("%q does not run around %s", a.command, around)
			}
			return nil
		}
		spec = rest
	}
	for _, want := range strings.Fields(spec) {
		re := wordPattern(want)
		if !slices.ContainsFunc(words, re.MatchString) {
			return fmt.Errorf("%q does not hold %s", a.command, want)
		}
	}
	return nil
}

// readFromReport is whether the report the project's script wrote is at
// file, of the project's folder, and each function has the coverage the
// script wrote in it.
func (c *coverageState) readFromReport(file string) error {
	if !c.w.d.Exists(c.abs(path.Join(c.project, file))) {
		return fmt.Errorf("no report is at %s\n%s", file, c.w.report())
	}
	entries, err := c.listed()
	if err != nil {
		return err
	}
	if len(c.want) == 0 {
		return errors.New("the project's script wrote no report")
	}
	for f, want := range c.want {
		if err := c.hasCoverage(entries, f, want); err != nil {
			return err
		}
	}
	return nil
}

// pmIs is whether the project's coverage script runs with manager: the
// command that runs it is manager's.
func (c *coverageState) pmIs(manager string) error {
	for _, a := range c.commands() {
		if len(a.words) >= 3 && slices.Equal(a.words[1:3], []string{"run", "coverage"}) {
			if program(a.words[0]) != manager {
				return fmt.Errorf("the project's coverage script runs with %s, not %s\n%s", program(a.words[0]), manager, c.w.report())
			}
			return nil
		}
	}
	return fmt.Errorf("no command runs the project's coverage script\n%s", c.w.report())
}

// nearestNodeBin is whether the project's coverage command is Vitest's,
// Jest's or c8's from the node_modules/.bin nearest the project that
// holds it.
func (c *coverageState) nearestNodeBin() error {
	for _, a := range c.commands() {
		tool := program(a.words[0])
		if !slices.Contains([]string{"vitest", "jest", "c8"}, tool) {
			continue
		}
		nearest := ""
		for dir := c.project; nearest == ""; dir = path.Dir(dir) {
			if bin := path.Join(dir, "node_modules/.bin"); c.w.d.Exists(c.abs(path.Join(bin, tool))) {
				nearest = bin
			}
			if dir == "." {
				break
			}
		}
		got, _ := c.rel(path.Dir(filepath.ToSlash(a.words[0])))
		if nearest == "" || got != nearest {
			return fmt.Errorf("%s runs from %s, not the nearest node_modules/.bin, %s\n%s", tool, got, nearest, c.w.report())
		}
		return nil
	}
	return fmt.Errorf("neither vitest, jest nor c8 runs for the project\n%s", c.w.report())
}

// noNpx is whether no coverage command runs npx.
func (c *coverageState) noNpx() error {
	announced := c.announced(c.w.stderr)
	if len(announced) == 0 {
		return fmt.Errorf("no coverage command runs\n%s", c.w.report())
	}
	for _, a := range announced {
		if slices.ContainsFunc(a.words, func(w string) bool { return program(w) == "npx" }) {
			return fmt.Errorf("%q runs npx\n%s", a.command, c.w.report())
		}
	}
	return nil
}

// nothingRuns is whether no coverage command runs for the project.
func (c *coverageState) nothingRuns() error {
	if commands := c.commands(); len(commands) > 0 {
		return fmt.Errorf("coverage runs %q for the project\n%s", commands[0].command, c.w.report())
	}
	return nil
}

// saysWhatToInstall is whether stderr says, of the project's folder, that
// the tool the Given left out is missing: a line naming the folder and the
// tool that says to install or add it, or to measure with
// --coverage-command.
func (c *coverageState) saysWhatToInstall() error {
	if c.missing == "" {
		return errors.New("no Given left a tool out")
	}
	for _, line := range strings.Split(c.w.stderr, "\n") {
		rest, ok := strings.CutPrefix(line, "itos-cc: coverage: ")
		if !ok {
			continue
		}
		dir, reason, ok := strings.Cut(rest, ": ")
		if rel, in := c.rel(dir); !ok || !in || rel != c.project {
			continue
		}
		says := strings.Contains(reason, "install") || strings.Contains(reason, "add ") || strings.Contains(reason, "--coverage-command")
		if strings.Contains(strings.ToLower(reason), strings.ToLower(c.missing)) && says {
			return nil
		}
	}
	return fmt.Errorf("stderr does not say to install %s for the project, or to use --coverage-command\n%s", c.missing, c.w.report())
}

// onlyComplexity is whether the last run lists functions, of the project's
// folder for it, each with a complexity and neither coverage nor a score.
func (c *coverageState) onlyComplexity(forIt string) error {
	entries, err := c.listed()
	if err != nil {
		return err
	}
	n := 0
	for _, e := range entries {
		if forIt != "" && c.project != "." && !strings.HasPrefix(filepath.ToSlash(e.File), c.project+"/") {
			continue
		}
		n++
		if e.Complexity < 1 || e.Coverage != nil || e.CRAP != nil {
			return fmt.Errorf("%s has complexity %d, coverage %s and CRAP %s\n%s", e.id(), e.Complexity, shown(e.Coverage), shown(e.CRAP), c.w.report())
		}
	}
	if n == 0 {
		return fmt.Errorf("no function is listed\n%s", c.w.report())
	}
	return nil
}

// venvRunsTests is whether the project's .venv's python runs its tests,
// which measure place.
func (c *coverageState) venvRunsTests() error {
	for _, a := range c.commands() {
		python, _ := c.rel(a.words[0])
		i := slices.Index(a.words, "-m")
		if python != ".venv/bin/python" || !slices.ContainsFunc(a.words, func(w string) bool { return w == "pytest" || w == "unittest" }) || i < 0 {
			continue
		}
		entries, err := c.listed()
		if err != nil {
			return err
		}
		if got, err := coverageOf(entries, covFunction{"board.py", "place"}); err != nil || got == nil {
			return fmt.Errorf("the tests the .venv's python ran do not measure place\n%s", c.w.report())
		}
		return nil
	}
	return fmt.Errorf("the .venv's python runs no tests\n%s", c.w.report())
}

// wrapperUsed is whether the module's coverage command is the build root's
// gradlew, and no coverage command is gradle.
func (c *coverageState) wrapperUsed() error {
	commands := c.commands()
	if len(commands) == 0 {
		return fmt.Errorf("no coverage command runs for the module\n%s", c.w.report())
	}
	for _, a := range c.announced(c.w.stderr) {
		if program(a.words[0]) == "gradle" {
			return fmt.Errorf("coverage runs %q\n%s", a.command, c.w.report())
		}
	}
	if wrapper, _ := c.rel(commands[0].words[0]); wrapper != "gradlew" {
		return fmt.Errorf("the module's coverage runs %s, not the build root's gradlew\n%s", commands[0].words[0], c.w.report())
	}
	return nil
}

// toolPrograms are the programs a language's coverage command may start
// with.
var toolPrograms = map[string][]string{"go": {"go"}, "typescript": {"vitest", "jest", "c8", "npm", "pnpm", "yarn", "bun"}}

// eachRootRuns is whether each build root's coverage command is its
// language's, run in that root.
func (c *coverageState) eachRootRuns() error {
	for root, language := range c.roots {
		i := slices.IndexFunc(c.announced(c.w.stderr), func(a covAnnouncement) bool { return a.dir == root })
		if i < 0 {
			return fmt.Errorf("no coverage command runs in %s\n%s", root, c.w.report())
		}
		if a := c.announced(c.w.stderr)[i]; !slices.Contains(toolPrograms[language], program(a.words[0])) {
			return fmt.Errorf("%s runs %q, no %s coverage command\n%s", root, a.command, language, c.w.report())
		}
	}
	return nil
}

// mergedIntoOne is whether the one run gave a function of each build root
// coverage.
func (c *coverageState) mergedIntoOne() error {
	entries, err := c.listed()
	if err != nil {
		return err
	}
	for root := range c.roots {
		if !slices.ContainsFunc(entries, func(e crapEntry) bool {
			return strings.HasPrefix(filepath.ToSlash(e.File), root+"/") && e.Coverage != nil
		}) {
			return fmt.Errorf("no function of %s has coverage\n%s", root, c.w.report())
		}
	}
	return nil
}

// failureLogged is whether stderr shows the test failing and itos-cc
// saying the coverage command failed with its exit status.
func (c *coverageState) failureLogged() error {
	if !strings.Contains(c.w.stderr, "--- FAIL: TestPlace") ||
		!regexp.MustCompile(`(?m)^itos-cc: coverage: go: exit status [1-9]\d*$`).MatchString(c.w.stderr) {
		return fmt.Errorf("stderr does not log the failing tests\n%s", c.w.report())
	}
	return nil
}

// reportStillUsed is whether Place has the coverage the failing test gave
// it: every statement but its if's arm.
func (c *coverageState) reportStillUsed() error {
	entries, err := c.listed()
	if err != nil {
		return err
	}
	want, _ := share(goBlocks(true, false, true), byUnit)
	return c.hasCoverage(entries, covFunction{"board.go", "Place"}, want)
}

// reportsUnderMetrics is whether each coverage command of the last run, one
// at least, writes its report under .metrics/coverage/.
func (c *coverageState) reportsUnderMetrics() error {
	announced := c.announced(c.w.stderr)
	if len(announced) == 0 {
		return fmt.Errorf("no coverage command runs\n%s", c.w.report())
	}
	for _, a := range announced {
		i := slices.IndexFunc(a.words, func(w string) bool { return strings.HasPrefix(w, "-coverprofile=") })
		if i < 0 {
			return fmt.Errorf("%q names no report it writes\n%s", a.command, c.w.report())
		}
		report, in := c.rel(strings.TrimPrefix(a.words[i], "-coverprofile="))
		if !in || !strings.HasPrefix(report, ".metrics/coverage/") {
			return fmt.Errorf("%q writes its report outside .metrics/coverage/\n%s", a.command, c.w.report())
		}
	}
	return nil
}

// gitignoreIgnoresAll is whether git, in a repository of the scratch
// folder, ignores whatever is in .metrics/coverage/, its .gitignore
// included, and nothing beside it in .metrics.
func (c *coverageState) gitignoreIgnoresAll() error {
	if err := c.w.git("init"); err != nil {
		return err
	}
	for _, p := range []string{".metrics/coverage/.gitignore", ".metrics/coverage/go-1/coverage.out", ".metrics/coverage/run-1/lcov.info"} {
		if err := c.w.git("check-ignore", "-q", p); err != nil {
			return fmt.Errorf("git does not ignore %s: %v", p, err)
		}
	}
	if c.w.git("check-ignore", "-q", ".metrics/crap.json") == nil {
		return errors.New("git ignores .metrics/crap.json, which is not in .metrics/coverage/")
	}
	return nil
}

// noTestsRun is whether the last run announced no coverage command and no
// test of the project left its mark.
func (c *coverageState) noTestsRun() error {
	if announced := c.announced(c.w.stderr); len(announced) > 0 {
		return fmt.Errorf("coverage runs %q\n%s", announced[0].command, c.w.report())
	}
	if len(c.markers) == 0 {
		return errors.New("the project has no tests that would leave a mark")
	}
	for _, m := range c.markers {
		if c.w.d.Exists(c.abs(m)) {
			return fmt.Errorf("a test ran: %s is there\n%s", m, c.w.report())
		}
	}
	return nil
}

// firstExistingReport is whether each build root's function has the
// coverage of the first of its reports itos-cc looks for that is there.
func (c *coverageState) firstExistingReport() error {
	entries, err := c.listed()
	if err != nil {
		return err
	}
	if len(c.want) < 2 {
		return errors.New("no project of build roots with reports was written")
	}
	for f, want := range c.want {
		if err := c.hasCoverage(entries, f, want); err != nil {
			return err
		}
	}
	return nil
}

// bothReportsRead is whether each --coverage-report of the last run, named
// relative to the folder it ran in and there, gave the function it measures
// its coverage.
func (c *coverageState) bothReportsRead() error {
	entries, err := c.listed()
	if err != nil {
		return err
	}
	byReport := map[string]covFunction{"a/lcov.info": {"a/a.go", "A"}, "b/coverage.out": {"b/b.go", "B"}}
	args := c.w.last.args
	n := 0
	for i, a := range args {
		if a != "--coverage-report" || i+1 == len(args) {
			continue
		}
		report := args[i+1]
		f, ok := byReport[filepath.ToSlash(report)]
		switch {
		case filepath.IsAbs(report) || !c.w.d.Exists(filepath.Join(c.w.last.dir, report)):
			return fmt.Errorf("%s is not a report relative to the working directory", report)
		case !ok:
			return fmt.Errorf("the sample holds no report %s", report)
		}
		if err := c.hasCoverage(entries, f, c.want[f]); err != nil {
			return err
		}
		n++
	}
	if n < 2 {
		return fmt.Errorf("the run read %d reports, not both\n%s", n, c.w.report())
	}
	return nil
}

// runsThroughShell is whether itos-cc announced command as a shell command
// line, and the shell ran it: make echoed the cover target's recipe.
func (c *coverageState) runsThroughShell(command string) error {
	if !slices.Contains(strings.Split(strings.ReplaceAll(c.w.stderr, "\r\n", "\n"), "\n"), "itos-cc: coverage $ "+command) {
		return fmt.Errorf("itos-cc does not announce %q as a shell command\n%s", command, c.w.report())
	}
	if !strings.Contains(c.w.stderr, sampleRecipe) {
		return fmt.Errorf("%q did not run the Makefile's recipe\n%s", command, c.w.report())
	}
	return nil
}

// readAfterwards is whether report, which the sample did not hold, is
// there after the run, and gave a/a.go's A its coverage.
func (c *coverageState) readAfterwards(report string) error {
	if report != "cover.out" {
		return fmt.Errorf("the sample does not leave %s to be written", report)
	}
	if !c.w.d.Exists(c.abs(report)) {
		return fmt.Errorf("no %s was written\n%s", report, c.w.report())
	}
	entries, err := c.listed()
	if err != nil {
		return err
	}
	return c.hasCoverage(entries, covFunction{"a/a.go", "A"}, 100*2.0/3)
}

// Fixtures written by hand.

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

// Matching report paths to sources.

// source writes the source rel, slash-separated from the folder the run
// goes from, of the language of its extension, which the run selects.
func (c *coverageState) source(rel string) error {
	text, _, _, err := shapeOf(rel)
	if err != nil {
		return err
	}
	c.sources = append(c.sources, rel)
	return c.w.write(path.Join(c.runDir, rel), text)
}

// under is a path a scenario writes from the root, /p/a.go, as a path of
// the scratch folder, p/a.go.
func under(p string) string { return strings.TrimPrefix(p, "/") }

// exactEntries writes a/x/board.go and b/x/board.go, which share a tail,
// and names the one relative to the folder the run goes from and the
// other by its absolute path.
func (c *coverageState) exactEntries() error {
	for _, s := range []string{"a/x/board.go", "b/x/board.go"} {
		if err := c.source(s); err != nil {
			return err
		}
	}
	c.entries = []string{"a/x/board.go", c.abs("b/x/board.go")}
	return nil
}

// sourceFile writes the source p names from the root.
func (c *coverageState) sourceFile(p string) error { return c.source(under(p)) }

// sourcesUnder writes the sources the paths name from the root, which share
// their first folder: the folder the run goes from, which selects them.
func (c *coverageState) sourcesUnder(paths ...string) error {
	root, _, _ := strings.Cut(under(paths[0]), "/")
	c.runDir = root
	for _, p := range paths {
		rel, ok := strings.CutPrefix(under(p), root+"/")
		if !ok {
			return fmt.Errorf("%s is not in %s with %s", p, root, paths[0])
		}
		if err := c.source(rel); err != nil {
			return err
		}
	}
	if len(paths) == 1 {
		c.paths = c.sources
	}
	return nil
}

// existingEntries names a file by both entries, the one relative to the
// folder the run goes from and the other from the root, and writes it
// there, a source the run does not select.
func (c *coverageState) existingEntries(relative, fromRoot string) error {
	text, _, _, err := shapeOf(relative)
	if err != nil {
		return err
	}
	if path.Join(c.runDir, relative) != under(fromRoot) {
		return fmt.Errorf("%s and %s name different files from %s", relative, fromRoot, c.runDir)
	}
	if err := c.w.write(under(fromRoot), text); err != nil {
		return err
	}
	c.entries = append(c.entries, relative, c.abs(under(fromRoot)))
	for _, e := range []string{c.abs(path.Join(c.runDir, relative)), c.entries[1]} {
		if !c.w.d.Exists(e) {
			return fmt.Errorf("no file is at %s", e)
		}
	}
	return nil
}

// sourceBesideProjectFile writes the source the run selects and a project
// file beside it, in a module whose path the entries name.
func (c *coverageState) sourceBesideProjectFile(source, file string) error {
	if err := c.w.write("go.mod", "module example.com/m\n\ngo 1.22\n"); err != nil {
		return err
	}
	if err := c.source(source); err != nil {
		return err
	}
	c.paths = []string{source}
	text, _, _, err := shapeOf(file)
	if err != nil {
		return err
	}
	return c.w.write(file, text)
}

// symlinkedSource writes real/board.go and links src to real, the run
// selecting src/board.go.
func (c *coverageState) symlinkedSource() error {
	text, _, _, err := shapeOf("real/board.go")
	if err != nil {
		return err
	}
	if err := c.w.write("real/board.go", text); err != nil {
		return err
	}
	if err := c.w.d.Link(c.abs("real"), c.abs("src")); err != nil {
		return err
	}
	c.sources, c.paths = []string{"src/board.go"}, []string{"src/board.go"}
	return nil
}

// entryFromBase names entry, which is no file of the folder the run goes
// from, and neither source is in that folder itself.
func (c *coverageState) entryFromBase(entry string) error {
	if c.w.d.Exists(c.abs(path.Join(c.runDir, entry))) {
		return fmt.Errorf("the base directory holds %s", entry)
	}
	for _, s := range c.sources {
		if !strings.Contains(s, "/") {
			return fmt.Errorf("the base directory holds %s", s)
		}
	}
	c.entries = append(c.entries, entry)
	return nil
}

// separators writes src/demo/board.go, and two reports of it: one naming
// it with written separators, the other with the reverse, each read in a
// run of its own.
func (c *coverageState) separators(written, reverse string) error {
	if err := c.source("src/demo/board.go"); err != nil {
		return err
	}
	parts := []string{"src", "demo", "board.go"}
	c.cases = [][]string{{strings.Join(parts, written)}, {strings.Join(parts, reverse)}}
	return nil
}

// matchToSources writes each case's report, LCOV or a Go profile of its
// entries, each measuring its file as the source of that name is
// measured, and runs crap --json over it from the folder the run goes
// from, naming the paths a Given selected.
func (c *coverageState) matchToSources() error {
	cases := c.cases
	if len(cases) == 0 {
		cases = [][]string{c.entries}
	}
	if len(cases[0]) == 0 || len(c.sources) == 0 {
		return errors.New("no report entry and sources were written to match")
	}
	c.matched = nil
	for i, entries := range cases {
		name := fmt.Sprintf("report-%d.info", i)
		var b strings.Builder
		if c.goFormat {
			name = fmt.Sprintf("report-%d.out", i)
			b.WriteString("mode: set\n")
		}
		for _, e := range entries {
			_, _, pieces, err := shapeOf(filepath.ToSlash(e))
			if err != nil {
				return err
			}
			if c.goFormat {
				b.WriteString(goRecords(e, goBlocks(true, false, true)))
				continue
			}
			b.WriteString(lcovRecord(e, pieces, nil))
		}
		if err := c.w.write(path.Join(c.runDir, name), b.String()); err != nil {
			return err
		}
		if err := c.w.runEnv(c.abs(c.runDir), c.w.env(), append([]string{"crap", "--json", "--coverage-report", name}, c.paths...)...); err != nil {
			return err
		}
		entries, err := c.listed()
		if err != nil {
			return err
		}
		c.matched = append(c.matched, entries)
	}
	return nil
}

// matchedCoverage is what the report gives a source it matches: its shape's
// share, by a Go profile's statements or by LCOV's lines.
func (c *coverageState) matchedCoverage(source string) (float64, error) {
	_, _, pieces, err := shapeOf(source)
	if c.goFormat {
		pieces = goBlocks(true, false, true)
	}
	want, _ := share(pieces, byUnit)
	return want, err
}

// matches is whether, in every case's run, each source selected has the
// coverage its entry gives it.
func (c *coverageState) matches() error {
	if len(c.matched) == 0 {
		return errors.New("nothing was matched")
	}
	for _, entries := range c.matched {
		for _, s := range c.sources {
			_, name, _, err := shapeOf(s)
			if err != nil {
				return err
			}
			want, err := c.matchedCoverage(s)
			if err != nil {
				return err
			}
			if err := c.hasCoverage(entries, covFunction{s, name}, want); err != nil {
				return err
			}
		}
	}
	return nil
}

// matchesNeither is whether each source selected is listed with no
// coverage.
func (c *coverageState) matchesNeither() error {
	if len(c.matched) == 0 {
		return errors.New("nothing was matched")
	}
	for _, entries := range c.matched {
		for _, s := range c.sources {
			_, name, _, err := shapeOf(s)
			if err != nil {
				return err
			}
			got, err := coverageOf(entries, covFunction{s, name})
			if err != nil {
				return fmt.Errorf("%v\n%s", err, c.w.report())
			}
			if got != nil {
				return fmt.Errorf("%s of %s has %s%% coverage: an entry matched it\n%s", name, s, shown(got), c.w.report())
			}
		}
	}
	return nil
}

// ignored are the entries of COV-33's report that match no selected
// source: a test file beside the source and a dependency's file.
var ignored = []string{"board_test.go", "vendor/example.com/dep/dep.go"}

// unmatchedEntry writes board.go and its test, and a report of board.go
// and of the files ignored names.
func (c *coverageState) unmatchedEntry() error {
	if err := c.w.write("board_test.go", markerTest("board", 5)); err != nil {
		return err
	}
	f := covFixture{format: "LCOV", report: "lcov.info", source: "board.go", function: "Place", entry: "board.go", unit: "line", pieces: goLines}
	report := lcovRecord(f.entry, f.pieces, nil)
	for _, e := range ignored {
		report += lcovRecord(e, lcovLines([2]int{4, 1}, [2]int{5, 1}), nil)
	}
	return c.add(f, goSource("board"), report)
}

// entryIgnored is whether the run succeeded, listing board.go's functions
// alone with their coverage, and said nothing of the ignored entries.
func (c *coverageState) entryIgnored() error {
	f, err := c.fixture()
	if err != nil {
		return err
	}
	object, report, err := (&crapState{w: c.w}).lastJSON()
	if err != nil {
		return err
	}
	if c.w.exit != 0 || len(object.Problems) > 0 || len(object.Unmeasured) > 0 {
		return fmt.Errorf("the run did not ignore the entries\n%s", report)
	}
	for _, e := range object.Entries {
		if filepath.ToSlash(e.File) != f.source {
			return fmt.Errorf("%s of %s is listed\n%s", e.Name, e.File, report)
		}
	}
	for _, e := range ignored {
		if strings.Contains(filepath.ToSlash(c.w.stderr), e) {
			return fmt.Errorf("stderr names %s\n%s", e, report)
		}
	}
	want, _ := share(f.pieces, byUnit)
	return c.hasCoverage(object.Entries, covFunction{f.source, f.function}, want)
}

// Pieces listed more than once.

// threePackages writes the module example.com/m with the packages named,
// each of goSource's Place and a test of it.
func (c *coverageState) threePackages(a, b, cc string) error {
	files := map[string]string{"go.mod": "module example.com/m\n\ngo 1.22\n"}
	for _, p := range []string{a, b, cc} {
		files[p+"/"+p+".go"], files[p+"/"+p+"_test.go"] = goSource(p), markerTest(p, 5)
		c.fixtures = append(c.fixtures, covFixture{format: "Go cover profile", report: "cover.out", source: p + "/" + p + ".go", function: "Place",
			entry: "example.com/m/" + p + "/" + p + ".go", unit: "statement", pieces: goBlocks(true, true, true)})
	}
	return c.writeIn(".", files)
}

// everyBinaryProfile writes cover.out as go test -coverpkg=./... writes it
// for the packages: each test binary lists every block of every package,
// covered in its own package alone.
func (c *coverageState) everyBinaryProfile() error {
	if len(c.fixtures) == 0 {
		return errors.New("no packages were written")
	}
	var b strings.Builder
	b.WriteString("mode: set\n")
	for _, binary := range c.fixtures {
		for _, f := range c.fixtures {
			b.WriteString(goRecords(f.entry, goBlocks(f.entry == binary.entry, f.entry == binary.entry, f.entry == binary.entry)))
		}
	}
	return c.w.write("cover.out", b.String())
}

// goToolCover is a function's coverage as go tool cover -func gives it from
// profile: its blocks, each once, covered when any copy of it ran.
func goToolCover(profile, entry string) float64 {
	covered := map[string]bool{}
	statements := map[string]float64{}
	for _, line := range strings.Split(profile, "\n") {
		block, ok := strings.CutPrefix(line, entry+":")
		if !ok {
			continue
		}
		fields := strings.Fields(block)
		n, _ := strconv.ParseFloat(fields[1], 64)
		statements[fields[0]] = n
		covered[fields[0]] = covered[fields[0]] || fields[2] != "0"
	}
	var all, ran float64
	for span, n := range statements {
		all += n
		if covered[span] {
			ran += n
		}
	}
	if all == 0 {
		return 0
	}
	return 100 * ran / all
}

// asGoToolCover is whether the function scored has percent coverage, what
// go tool cover -func gives it from the profile.
func (c *coverageState) asGoToolCover(percent int) error {
	if len(c.fixtures) == 0 {
		return errors.New("no profile was written")
	}
	f := c.fixtures[0]
	data, err := c.w.readIn(c.w.dir, "cover.out")
	if err != nil {
		return err
	}
	if tool := goToolCover(string(data), f.entry); tool != float64(percent) {
		return fmt.Errorf("go tool cover -func gives %s %v%%, not %d%%", f.function, tool, percent)
	}
	entries, err := c.listed()
	if err != nil {
		return err
	}
	return c.hasCoverage(entries, covFunction{f.source, f.function}, float64(percent))
}

// splitSource is board.go of F, of no decision, on lines 3 to 7, and G,
// whose switch on line 10 decides between three branches.
const splitSource = "package board\n\nfunc F(a int) int {\n\tn := a\n\tn++\n\treturn n\n}\n\n" +
	"func G(a int) int {\n\tswitch {\n\tcase a > 1:\n\t\treturn 1\n\tcase a < -1:\n\t\treturn 2\n\t}\n\treturn 3\n}\n"

// twoReports writes board.go, which two reports are to measure.
func (c *coverageState) twoReports() error {
	c.split = &covSplit{reports: [2]string{"unit.info", "integration.info"}}
	return c.w.write("board.go", splitSource)
}

// eachCoversALine writes the two reports: each runs a line of F the other
// does not, and takes branches of G's decision the other does not, the
// second more of them.
func (c *coverageState) eachCoversALine() error {
	s := c.split
	if s == nil {
		return errors.New("no two reports were named")
	}
	s.f = [2][]covPiece{
		lcovLines([2]int{4, 1}, [2]int{5, 1}, [2]int{6, 0}),
		lcovLines([2]int{4, 1}, [2]int{5, 0}, [2]int{6, 1}),
	}
	s.g = [2]covDecision{{line: 10, branches: 3, taken: []int{0}}, {line: 10, branches: 3, taken: []int{1, 2}}}
	for i, name := range s.reports {
		gLines := lcovLines([2]int{10, 1}, [2]int{11, 1}, [2]int{12, 1 - i}, [2]int{13, i}, [2]int{14, i}, [2]int{16, 0})
		if err := c.w.write(name, lcovRecord("board.go", append(slices.Clone(s.f[i]), gLines...), []covDecision{s.g[i]})); err != nil {
			return err
		}
	}
	return nil
}

// anyReportCovers is whether F has the share of its lines either report
// ran, more than either alone ran.
func (c *coverageState) anyReportCovers() error {
	s := c.split
	if s == nil || s.f[0] == nil {
		return errors.New("no two reports were written")
	}
	union := slices.Clone(s.f[0])
	for i, p := range s.f[1] {
		union[i].covered = max(union[i].covered, p.covered)
	}
	want, _ := share(union, byUnit)
	for _, alone := range s.f {
		if got, _ := share(alone, byUnit); got >= want {
			return errors.New("one report alone covers every line of F the two do")
		}
	}
	entries, err := c.listed()
	if err != nil {
		return err
	}
	return c.hasCoverage(entries, covFunction{"board.go", "F"}, want)
}

// mostBranches is whether G has the share of its decision's branches the
// report that took most of them took, less than the two took together.
func (c *coverageState) mostBranches() error {
	s := c.split
	if s == nil || s.f[0] == nil {
		return errors.New("no two reports were written")
	}
	most, both := 0, map[int]bool{}
	for _, d := range s.g {
		most = max(most, len(d.taken))
		for _, b := range d.taken {
			both[b] = true
		}
	}
	if most == len(both) {
		return errors.New("one report took every branch the two took")
	}
	entries, err := c.listed()
	if err != nil {
		return err
	}
	return c.hasCoverage(entries, covFunction{"board.go", "G"}, 100*float64(most)/float64(s.g[0].branches))
}

// Where coverage starts.

// startsAt is whether the function "the <language> function:" wrote, its
// first line first when that is given, is measured from line from: a
// report that ran that line alone of every line of it gives it the share
// of one line of those from there to its end.
func (c *coverageState) startsAt(from int, firstLine string) error {
	first, _ := strconv.Atoi(firstLine)
	file := c.w.wrote
	if file == "" {
		return errors.New("no function was written")
	}
	code, stdout, stderr, err := c.w.capture(c.w.dir, c.w.env(), "crap", "--no-coverage", "--json", file)
	if err != nil {
		return err
	}
	var object crapObject
	if err := json.Unmarshal([]byte(stdout), &object); err != nil || code != 0 || len(object.Entries) != 1 {
		return fmt.Errorf("the function could not be measured: exit %d, %v\n%s%s", code, err, stdout, stderr)
	}
	e := object.Entries[0]
	switch {
	case first != 0 && e.StartLine != first:
		return fmt.Errorf("its first line is %d, not %d", e.StartLine, first)
	case from < e.StartLine || from > e.EndLine:
		return fmt.Errorf("line %d is not of the function, lines %d to %d", from, e.StartLine, e.EndLine)
	}
	var lines [][2]int
	for line := e.StartLine; line <= e.EndLine; line++ {
		lines = append(lines, [2]int{line, hits(line == from)})
	}
	if err := c.w.write("probe.info", lcovRecord(file, lcovLines(lines...), nil)); err != nil {
		return err
	}
	if err := c.w.runEnv(c.w.dir, c.w.env(), "crap", "--json", "--coverage-report", "probe.info", file); err != nil {
		return err
	}
	entries, err := c.listed()
	if err != nil {
		return err
	}
	return c.hasCoverage(entries, covFunction{file, e.Name}, 100/float64(e.EndLine-from+1))
}

// What itos-cc says while it measures.

// announcesCommand is whether stderr has a line of text, <dir> standing for
// the build root of the sources the run named and … for the rest of the
// line, the command it names the one that ran: its output follows.
func (c *coverageState) announcesCommand(text string) error {
	before, after, ok := strings.Cut(text, "<dir>")
	if !ok {
		return fmt.Errorf("%q names no <dir>", text)
	}
	after, rest := strings.CutSuffix(after, "…")
	re := regexp.MustCompile("^" + regexp.QuoteMeta(before) + "(.+?)" + regexp.QuoteMeta(after) + map[bool]string{true: ".*", false: ""}[rest] + "$")
	roots, err := c.selectedRoots()
	if err != nil {
		return err
	}
	lines := strings.Split(strings.ReplaceAll(c.w.stderr, "\r\n", "\n"), "\n")
	for i, line := range lines {
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if dir, in := c.rel(m[1]); !in || !slices.Contains(roots, dir) {
			return fmt.Errorf("%q names %s, not the build root of the run's sources, %v\n%s", line, m[1], roots, c.w.report())
		}
		if i+1 == len(lines) || !strings.HasPrefix(lines[i+1], "ok ") {
			return fmt.Errorf("the command %q names did not run\n%s", line, c.w.report())
		}
		return nil
	}
	return fmt.Errorf("stderr has no line %q\n%s", text, c.w.report())
}

// noLineBegins is whether no line of stderr begins with text.
func (c *coverageState) noLineBegins(text string) error {
	for _, line := range strings.Split(c.w.stderr, "\n") {
		if strings.HasPrefix(line, text) {
			return fmt.Errorf("stderr has the line %q\n%s", line, c.w.report())
		}
	}
	return nil
}

// exitStatusLine is whether a line of stderr begins with text and gives the
// exit status the failing command exited with.
func (c *coverageState) exitStatusLine(text string) error {
	re := regexp.MustCompile(`exit status [1-9]\d*$`)
	for _, line := range strings.Split(strings.ReplaceAll(c.w.stderr, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, text) && re.MatchString(line) {
			return nil
		}
	}
	return fmt.Errorf("no line of stderr begins %q and gives an exit status\n%s", text, c.w.report())
}
