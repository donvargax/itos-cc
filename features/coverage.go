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
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"slices"
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

// coverageFeature declares coverage.feature's steps, coverageSteps, which
// tools/bin/assemble lists in steps_gen.go.
var coverageFeature = featureFile{Name: "coverage.feature", Steps: coverageSteps}

func coverageSteps(sc *godog.ScenarioContext, w *world) {
	c := &coverageState{w: w}
	sc.Before(func(ctx context.Context, s *godog.Scenario) (context.Context, error) {
		*c = coverageState{w: w, project: ".", runDir: ".", want: map[covFunction]float64{}, roots: map[string]string{}}
		if filepath.Base(s.Uri) != "coverage.feature" || len(s.Steps) == 0 {
			return ctx, nil
		}
		// no tests are run (steps.go) looks for their marks.
		w.noTests = c.noTestsRun
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

	c.floorSteps(sc)

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
	sc.Step(`^a (Go|TypeScript|Python|Kotlin|Dart) project whose build root holds (\S+)$`, c.projectWithMarker)
	sc.Step(`^nothing else$`, c.nothingElse)
	sc.Step(`^a "coverage" script$`, c.coverageScript)
	sc.Step(`^vitest (\d+) or later as a dependency$`, c.vitestDependency)
	sc.Step(`^vitest (\d+) or later installed through another package, named in neither its dependencies nor its devDependencies, its test script "([^"]*)"$`, c.vitestThroughAnotherPackage)
	sc.Step(`^jest as a dependency$`, c.jestDependency)
	sc.Step(`^neither vitest nor jest$`, c.neitherVitestNorJest)
	sc.Step(`^pytest (importable|not importable)$`, c.pytest)
	sc.Step(`^the build (mentions|does not mention) kover$`, c.kover)
	sc.Step(`^it or a parent declares jacoco-maven-plugin$`, c.jacocoPlugin)
	sc.Step(`^a dependency on the flutter SDK$`, c.flutterDependency)
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
	sc.Step(`^a Dart project whose flutter is not installed$`, c.flutterMissing)
	sc.Step(`^a Dart project with no \.dart_tool/package_config\.json, never resolved$`, c.unresolvedDart)
	sc.Step(`^nothing runs for it$`, c.nothingRuns)
	sc.Step(`^stderr says what to install, or to use --coverage-command$`, c.saysWhatToInstall)
	sc.Step(`^only complexity is reported( for it)?$`, c.onlyComplexity)
	sc.Step(`^a Python project with a \.venv directory$`, c.pythonVenv)
	sc.Step(`^the \.venv's python runs the tests$`, c.venvRunsTests)
	sc.Step(`^a Kotlin Gradle module under a build root that holds gradlew$`, c.gradleWrapper)
	sc.Step(`^the gradlew wrapper is used instead of gradle$`, c.wrapperUsed)
	sc.Step(`^a Flutter app whose \.fvmrc pins the SDK that \.fvm/flutter_sdk links to$`, c.fvmApp)
	sc.Step(`^\.fvm/flutter_sdk/bin/flutter runs the tests instead of the flutter on PATH$`, c.fvmRunsTests)
	sc.Step(`^a Dart package whose pubspec\.yaml does not depend on the flutter SDK$`, c.pureDart)
	sc.Step(`^stderr says only Flutter apps and packages are measured$`, c.onlyFlutterMeasured)
	sc.Step(`^a pub workspace whose root pubspec\.yaml lists the Flutter packages (\w+) and (\w+)$`, c.pubWorkspace)
	sc.Step(`^flutter test runs in (\w+) and in (\w+), and nothing runs at the workspace root$`, c.flutterRunsIn)
	sc.Step(`^each report's lib/ paths are read against its member's directory$`, c.membersRead)
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
	sc.Step(`^a Flutter app whose test loads (\S+) and no test loads (\S+) or (\S+)$`, c.unloadedApp)
	// A Dart source's, which this Given writes; itos-cc.yaml's first line is
	// a Then of file_selection.feature (slice-53).
	sc.Step(`^(\S+\.dart)'s first line is "([^"]*)"$`, c.firstLine)
	sc.Step(`^(\S+)'s functions are at (\d+)% coverage$`, c.functionsAt)
	sc.Step(`^(\S+)'s functions have no CRAP score$`, c.functionsUnscored)

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
