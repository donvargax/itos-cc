// The steps of mutate.feature: mutation list, run, check, sample and except
// over projects the Givens write. Like every step here, they read the
// binary's output and the files it leaves alone, never its code. Its
// scenarios gain steps a piece at a time (Unstepped, steps.go); those of
// the pieces not done yet are held by cmd/itos-cc's Go tests.
package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// mutateState is one mutate.feature scenario's state beside the world's.
type mutateState struct {
	w *world

	// file is the source the scenario's mutation list listed, as the run
	// named it, and sites what it listed.
	file  string
	sites []listedSite
	// setup is src/setup.go's text as the previous run left it.
	setup string
	// snapshot is the snapshot a Given recorded of an excluded source,
	// slash-separated from the scratch folder, and source that source.
	snapshot, source string

	// language is the language of the project a Given wrote, as the
	// sentence names it, and has what it says the project has.
	language, has string
	// pm is the package manager a TypeScript project declares.
	pm string
	// records is the folder, outside the scratch folder, where the
	// project's tests record what they saw when they ran, "" for none.
	records string
	// original is the text of the file the tests check, as the Given wrote
	// it; recorded is what it held after a Given's run, "" for none.
	original, recorded string
	// baseline is how long the Given says the project's tests take.
	baseline time.Duration
	// command is the --test-command a Given's project runs its tests
	// with, "" for its own, and factor the timeout factor a When says is
	// the default, 0 where it says none.
	command string
	factor  float64
}

func mutateSteps(sc *godog.ScenarioContext, w *world) {
	m := &mutateState{w: w}
	sc.Before(func(ctx context.Context, s *godog.Scenario) (context.Context, error) {
		*m = mutateState{w: w}
		if filepath.Base(s.Uri) != "mutate.feature" {
			return ctx, nil
		}
		// no tests are run (steps.go) looks for a mark a test would leave,
		// .metrics and a baseline, as cli.feature's no test command runs.
		w.noTests = (&cliState{w: w}).noTestRuns
		for _, tag := range s.Tags {
			if setUp := mutateSamples[tag.Name]; setUp != nil {
				return ctx, setUp(m)
			}
		}
		return ctx, nil
	})

	// Mutation sites.
	sc.Step(`^the sites are:$`, m.sitesAre)
	sc.Step(`^code outside functions, comments, strings, and type arguments hold no sites$`, m.noSitesOutsideCode)
	sc.Step(`^a\?\.\(\) becomes a\(\), as a\?\.\[0\] becomes a\[0\]$`, m.optionalChainsLoseTheirMark)
	sc.Step(`^(\.\w+(?:, \.\w+)*,? and \.\w+) files have the same sites$`, m.sameSitesIn)
	sc.Step(`^each site is printed as "([^"]*)"$`, m.eachSitePrintedAs)
	sc.Step(`^src/app\.ts has a function with a site before, inside and after an inline callback$`, m.appWithCallback)
	sc.Step(`^its sites are listed in line and column order$`, m.inLineOrder)
	sc.Step(`^in the order "itos-cc ([^"]*)" lists the file's "([^"]*)"$`, m.inTheOrderOf)

	// Functions sharing a name.
	sc.Step(`^src/setup\.go declares two init functions with different bodies, each with a mutant the tests kill$`, m.setupProject)
	sc.Step(`^a previous run recorded both$`, m.previousRun)
	sc.Step(`^a previous run recorded both init functions of src/setup\.go, all killed$`, func() error {
		if err := m.setupProject(); err != nil {
			return err
		}
		return m.previousRun()
	})
	sc.Step(`^I run "itos-cc ?([^"]*)" with nothing changed$`, m.runUnchanged)
	sc.Step(`^the mutants of both init functions are reused, and none runs$`, m.bothReused)
	sc.Step(`^the second init function has changed since$`, m.secondChanged)
	sc.Step(`^the two have swapped places since, unchanged$`, m.initsSwapped)
	sc.Step(`^the only problem is "([^"]*)", for the second init function$`, m.onlyProblemOfSecond)
	sc.Step(`^"itos-cc ([^"]*)" runs only the second init function's mutants$`, m.runsOnlySecond)
	sc.Step(`^"itos-cc ([^"]*)" reuses every mutant$`, m.reusesEvery)

	// Sources itos-cc.yaml excludes.
	sc.Step(`^the only commit since main changes (\S+), whose mutants no test reaches$`, m.onlyCommitChanges)
	sc.Step(`^stderr says "(.*)", and the exit code is (\d+)$`, func(text string, code int) error {
		if err := w.stderrSays(text); err != nil {
			return err
		}
		return w.exitsWith(code)
	})
	sc.Step(`^"itos-cc ([^"]*)" exits (\d+)$`, func(args string, code int) error {
		if err := w.runWith(w.env(), args); err != nil {
			return err
		}
		return w.exitsWith(code)
	})
	sc.Step(`^neither "itos-cc ([^"]*)" nor "itos-cc ([^"]*)" names a site in (\S+)$`, m.neitherNames)
	sc.Step(`^a snapshot under \.metrics/mutate for (\S+), and fresh results for every other source$`, m.excludedSnapshot)
	sc.Step(`^itos-cc\.yaml then excludes "([^"]*)"$`, func(glob string) error {
		return w.write("itos-cc.yaml", "exclude: ["+strconv.Quote(glob)+"]\n")
	})
	sc.Step(`^its "([^"]*)" list (\S+) with "([^"]*)" "([^"]*)", and the exit code is (\d+)$`, func(key, file, name, value string, code int) error {
		if err := m.lists(key, file, name, value); err != nil {
			return err
		}
		return w.exitsWith(code)
	})
	sc.Step(`^the snapshot is removed, and its "([^"]*)" list it with "([^"]*)" "([^"]*)"$`, m.snapshotRemoved)
	sc.Step(`^itos-cc\.yaml excepts a survivor in (\S+) and excludes "([^"]*)"$`, m.exceptsAndExcludes)
	sc.Step(`^it reports "([^"]*)" with "([^"]*)" "([^"]*)"$`, m.reports)
	sc.Step(`^the itos-cc\.yaml HEAD commits excludes "([^"]*)"$`, m.headExcludes)
	sc.Step(`^no selected site is in (\S+)$`, m.noSelectedSiteIn)

	// Running mutants.
	sc.Step(`^a source file in a (Go|TypeScript|Python|Kotlin) project with (.+)$`, m.sourceInProject)
	sc.Step(`^its mutants run$`, m.itsMutantsRun)
	sc.Step(`^the test command is (.+)$`, m.testCommandIs)
	sc.Step(`^a/a\.go is tested only by e2e/e2e_test\.go, which imports package a$`, m.e2eModule)
	sc.Step(`^its mutants are uncovered and none runs$`, func() error { return m.allUncovered(m.file, true) })
	sc.Step(`^coverage and every mutant run the whole suite of its build root: (.+)$`, m.wholeSuite)
	sc.Step(`^the e2e test kills them$`, m.e2eKills)
	sc.Step(`^a later run without --all-tests reuses those kills for functions that have not changed while the Go module tests and support files remain unchanged$`, m.laterRunReuses)
	sc.Step(`^"([^"]*)" runs through the platform shell from the file's build root$`, m.shellFromBuildRoot)
	sc.Step(`^each worker runs mutants in its own private copy of the project$`, m.privateCopies)
	sc.Step(`^dependency directories such as (\S+) and (\S+) are linked, not copied$`, m.dependenciesLinked)
	sc.Step(`^the source files in the working tree are unchanged while tests run$`, m.treeUnchanged)
	sc.Step(`^a Python project installed in editable mode$`, m.editableProject)
	sc.Step(`^PYTHONPATH puts the worker's copy of "([^"]*)" and "([^"]*)" first$`, m.pythonPathFirst)
	sc.Step(`^mutants are about to run for a test command$`, m.aboutToRun)
	sc.Step(`^that command first runs unmutated inside a worker's copy$`, m.baselineFirst)
	sc.Step(`^its duration sets the mutant timeout$`, m.durationSetsTimeout)
	sc.Step(`^the baseline took (\d+) (seconds|milliseconds)$`, m.baselineTook)
	sc.Step(`^mutants run with the default timeout factor of (\d+)$`, m.runWithDefaultFactor)
	sc.Step(`^a mutant times out after ([\d.]+) seconds(?:: (\w+) times the baseline, plus (\d+) seconds)?$`, m.timesOutAfter)
	sc.Step(`^a timed-out mutant counts as killed$`, m.timeoutCountsKilled)
	sc.Step(`^the tests of (\S+) fail without any mutation$`, m.failingBaseline)
	sc.Step(`^the last (\d+) lines of the test output follow$`, m.lastLinesFollow)
	sc.Step(`^the snapshot of (\S+) is not changed$`, m.snapshotUnchanged)
	sc.Step(`^up to (\d+) mutants run at the same time$`, m.upToAtOnce)
	sc.Step(`^the default is half the CPUs, at least 1$`, m.defaultWorkers)

	// Coverage decides which mutants run.
	sc.Step(`^coverage shows line (\d+) of src/board\.ts is never executed$`, m.lineNeverExecuted)
	sc.Step(`^the mutants on line (\d+) are reported as uncovered without running$`, m.lineUncovered)
	sc.Step(`^coverage measured other TypeScript files but never (\S+)$`, m.unusedProject)
	sc.Step(`^a TypeScript project whose tests import src/board\.ts but never (\S+)$`, m.unusedProject)
	sc.Step(`^every mutant in (\S+) is uncovered$`, func(file string) error { return m.allUncovered(file, false) })
	sc.Step(`^every mutant in (\S+) is uncovered, and none runs$`, func(file string) error { return m.allUncovered(file, true) })
	sc.Step(`^stderr does not say "([^"]*)"$`, func(text string) error {
		if strings.Contains(w.stderr, text) {
			return fmt.Errorf("stderr says %q\n%s", text, w.report())
		}
		return nil
	})
	sc.Step(`^"itos-cc ([^"]*)" lists them as uncovered and exits (\d+)$`, m.listsThemUncovered)
	sc.Step(`^coverage measured no Kotlin file$`, m.kotlinUnmeasured)
	sc.Step(`^every mutant runs regardless of coverage$`, m.everyMutantRuns)
	sc.Step(`^every mutant can be reused from the previous snapshot$`, m.everyReusable)
	sc.Step(`^no coverage or test command runs$`, m.nothingRuns)
	sc.Step(`^no coverage command or mutant runs$`, m.nothingRuns)
	sc.Step(`^src/board\.ts has a mutant that is killed and one on a line no test executes$`, func() error { return m.lineNeverExecuted(12) })
	sc.Step(`^the lines itos-cc writes to stderr about the baseline and each mutant begin "([^"]*)"$`, m.progressLinesBegin)

	// Coverage from tests that run the built binary.
	sc.Step(`^a Go module whose only test builds its binary with go build -cover when GOCOVERDIR is set, and runs it$`, func() error { return m.greetModule(true, false) })
	sc.Step(`^the test checks one branch of the binary's output and not another$`, m.greetChecksOneBranch)
	sc.Step(`^the lines the binary ran are covered, and their mutants run$`, m.greetRanCovered)
	sc.Step(`^a mutant the test notices is killed, and one it does not notice survives$`, m.greetNotices)
	sc.Step(`^only the mutants on lines the binary never ran are uncovered$`, m.greetNeverRanUncovered)
	sc.Step(`^a Go module whose only test builds its binary without -cover and runs it$`, func() error { return m.greetModule(false, false) })
	sc.Step(`^every mutant the binary's code holds is uncovered, as before$`, m.greetAllUncovered)
	sc.Step(`^the module of ID-MUT-117, with one function also called by an in-process test$`, func() error { return m.greetModule(true, true) })
	sc.Step(`^each covered mutant has "coverage", listing "([^"]*)", "([^"]*)" or both, as the data that covers its line$`, m.greetCoverageKinds)
	sc.Step(`^an uncovered mutant has no "coverage"$`, m.uncoveredHasNoCoverage)
	sc.Step(`^the coverage of ID-MUT-117 runs$`, func() error { return w.runWith(w.env(), "mutation run --all-tests --fail-uncovered") })
	sc.Step(`^GOCOVERDIR names a directory under the run's own run-\* directory in \.metrics/coverage/$`, m.coverDirUnderRun)
	sc.Step(`^nothing of it is left once the run ends$`, m.coverDirGone)

	// Uncovered mutants as failures.
	sc.Step(`^every mutant on the lines the tests execute is killed$`, m.executedKilled)
	sc.Step(`^each mutant on line (\d+) is listed as "([^"]*)"$`, m.eachListedAs)
	sc.Step(`^every mutant in (\S+) is listed as uncovered$`, m.everyListedUncovered)
	sc.Step(`^a commit after "([^"]*)" changed "([^"]*)", which no test executes$`, m.commitChanged)
	sc.Step(`^"([^"]*)", unchanged, is not executed by any test either$`, m.unchangedUnexecuted)
	sc.Step(`^the mutants of "([^"]*)" are listed as uncovered$`, m.functionListedUncovered)
	sc.Step(`^none of "([^"]*)" is$`, m.noneOfFunctionListed)
	sc.Step(`^each uncovered mutant is a "([^"]*)" problem with file, line, column, function, original, and replacement$`, m.uncoveredProblems)
	sc.Step(`^"ok" is (true|false)$`, func(want string) error {
		object, err := w.json()
		if err != nil {
			return err
		}
		if ok, _ := object["ok"].(bool); fmt.Sprint(ok) != want {
			return fmt.Errorf("\"ok\" is %v, not %s\n%s", object["ok"], want, w.report())
		}
		return nil
	})
	sc.Step(`^the problem is "([^"]*)", naming "([^"]*)"$`, func(rule, flag string) error { return w.problemIs(rule, "flag", flag) })

	// The folder the tests of a scenario record what they saw in, outside
	// the scratch folder, so no worker copies it.
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if m.records != "" {
			_ = w.d.Remove(m.records)
		}
		return ctx, nil
	})
}

// The projects of the scenarios that only say When I run, by their ID.
var mutateSamples = map[string]func(*mutateState) error{
	"@ID-MUT-06":  (*mutateState).pythonSample,
	"@ID-MUT-09":  (*mutateState).e2eModule,
	"@ID-MUT-10":  (*mutateState).makeProject,
	"@ID-MUT-11":  (*mutateState).copiedProject,
	"@ID-MUT-17":  (*mutateState).workersProject,
	"@ID-MUT-21":  (*mutateState).halfCovered,
	"@ID-MUT-50":  func(m *mutateState) error { return m.lineNeverExecuted(12) },
	"@ID-MUT-51":  (*mutateState).untestedBoard,
	"@ID-MUT-120": func(m *mutateState) error { return m.greetModule(true, false) },
}

// Samples.

// The Python project of ID-MUT-06: src/board.py, its sites none a
// deletion, and a test that leaves test-ran behind if anything runs it.
var (
	pythonSample = map[string]string{
		"pyproject.toml":      "[project]\nname = \"board\"\nversion = \"0\"\n",
		"src/board.py":        "def place(row, col):\n    return row > 0 and col < 9\n",
		"tests/test_board.py": "open(\"test-ran\", \"w\").write(\"ran\")\n\nfrom src.board import place\n\n\ndef test_place():\n    assert place(1, 2)\n",
	}
	// pythonSites are src/board.py's sites as mutation list prints them.
	pythonSites = []listedSite{
		{file: "src/board.py", line: 2, column: 16, original: ">", replacement: ">=", function: "board#place"},
		{file: "src/board.py", line: 2, column: 18, original: "0", replacement: "1", function: "board#place"},
		{file: "src/board.py", line: 2, column: 20, original: "and", replacement: "or", function: "board#place"},
		{file: "src/board.py", line: 2, column: 28, original: "<", replacement: "<=", function: "board#place"},
	}
)

func (m *mutateState) pythonSample() error {
	return m.writeAll(pythonSample)
}

// writeAll writes each file of files, by its slash-separated path from
// the scratch folder.
func (m *mutateState) writeAll(files map[string]string) error {
	for name, text := range files {
		if err := m.w.write(name, text); err != nil {
			return err
		}
	}
	return nil
}

// Sites.

// listedSite is a mutation site as mutation list names it: its file as
// the run names it, slash-separated, line and column, the text a mutant
// replaces and what replaces it, "" for a deletion, and its function.
type listedSite struct {
	file                  string
	line, column          int
	original, replacement string
	function              string
}

func (s listedSite) String() string {
	replacement := "`" + s.replacement + "`"
	if s.replacement == "" {
		replacement = "(deleted)"
	}
	return fmt.Sprintf("%s:%d:%d `%s` → %s in %s", s.file, s.line, s.column, s.original, replacement, s.function)
}

// listedLine is one site of mutation list's plain output.
var listedLine = regexp.MustCompile("^(\\S+):(\\d+):(\\d+) `([^`]+)` → (?:`([^`]*)`|\\(deleted\\)) in (.+)$")

// listedSites are the sites stdout, mutation list's plain output, lists,
// every line one.
func listedSites(stdout string) ([]listedSite, error) {
	var sites []listedSite
	for _, line := range outputLines(strings.ReplaceAll(stdout, "\r\n", "\n")) {
		match := listedLine.FindStringSubmatch(line)
		if match == nil {
			return nil, fmt.Errorf("%q is no site's line", line)
		}
		lineNo, _ := strconv.Atoi(match[2])
		column, _ := strconv.Atoi(match[3])
		sites = append(sites, listedSite{file: filepath.ToSlash(match[1]), line: lineNo, column: column,
			original: match[4], replacement: match[5], function: match[6]})
	}
	return sites, nil
}

// sitesOf is the sites mutation list --json lists of file, a path from the
// scratch folder.
func (m *mutateState) sitesOf(file string) ([]listedSite, error) {
	code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), "mutation", "list", "--json", filepath.FromSlash(file))
	if err != nil {
		return nil, err
	}
	var object struct {
		Sites []struct {
			File, Function, Original, Replacement string
			Line, Column                          int
		}
	}
	if err := json.Unmarshal([]byte(stdout), &object); err != nil || code != 0 {
		return nil, fmt.Errorf("mutation list --json %s exited %d, its output no object of sites (%v)\n%s%s", file, code, err, stdout, stderr)
	}
	var sites []listedSite
	for _, s := range object.Sites {
		sites = append(sites, listedSite{file: filepath.ToSlash(s.File), line: s.Line, column: s.Column,
			original: s.Original, replacement: s.Replacement, function: s.Function})
	}
	return sites, nil
}

// sitesAre is whether the last run, mutation list of one file, listed
// exactly the table's sites, in its order: their line, the text replaced
// and what replaces it, "(deleted)" for nothing.
func (m *mutateState) sitesAre(table *godog.Table) error {
	if err := m.listed(); err != nil {
		return err
	}
	var want, got []string
	for _, row := range table.Rows[1:] {
		want = append(want, strings.Join([]string{row.Cells[0].Value, row.Cells[1].Value, row.Cells[2].Value}, " "))
	}
	for _, s := range m.sites {
		replacement := s.replacement
		if replacement == "" {
			replacement = "(deleted)"
		}
		got = append(got, strings.Join([]string{strconv.Itoa(s.line), s.original, replacement}, " "))
	}
	if !slices.Equal(got, want) {
		return fmt.Errorf("the sites listed are\n%s\nnot\n%s\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"), m.w.report())
	}
	return nil
}

// listed reads the last run's sites: a mutation list of one file, which
// exited 0.
func (m *mutateState) listed() error {
	w := m.w
	if w.last == nil || len(w.last.args) == 0 || w.exit != 0 {
		return fmt.Errorf("no mutation list of a file ran and exited 0\n%s", w.report())
	}
	sites, err := listedSites(w.stdout)
	if err != nil {
		return fmt.Errorf("%v\n%s", err, w.report())
	}
	m.file, m.sites = filepath.ToSlash(w.last.args[len(w.last.args)-1]), sites
	return nil
}

// span is a piece of a line of a source: what it is, its line, and the
// columns it starts at and ends before, counted from 1.
type span struct {
	kind           string
	line, from, to int
}

// The pieces of a TypeScript source that are no code of a function, as
// MUT-01's file writes each.
var (
	commentText  = regexp.MustCompile(`//(.*)`)
	stringText   = regexp.MustCompile(`"[^"]*"`)
	typeArgument = regexp.MustCompile(`\w(<[\w, ]+>)`)
	// wouldBeSite is a text that would be a site in a function's code: a
	// comparison, a logical or arithmetic operator, a 0 or 1, or a boolean;
	// an assignment's = is none.
	wouldBeSite = regexp.MustCompile(`[<>]|[=!]==?|&&|\|\||[!+*/-]|\b[01]\b|\btrue\b|\bfalse\b`)
)

// noSitesOutsideCode is whether the file the last run listed holds code
// outside every function, a comment, a string and a type argument, each
// with a text that would be a site in a function's code, and no site the
// run listed is in one.
func (m *mutateState) noSitesOutsideCode() error {
	if m.file == "" {
		if err := m.listed(); err != nil {
			return err
		}
	}
	data, err := m.w.readIn(m.w.dir, m.file)
	if err != nil {
		return err
	}
	var spans []span
	depth := 0
	for i, line := range strings.Split(string(data), "\n") {
		n := i + 1
		trimmed := strings.TrimSpace(line)
		if depth == 0 && trimmed != "" && trimmed != "}" && !strings.Contains(line, "function ") {
			spans = append(spans, span{"code outside functions", n, 1, len(line) + 1})
		}
		depth += strings.Count(line, "{") - strings.Count(line, "}")
		for _, piece := range []struct {
			kind string
			text *regexp.Regexp
			sub  int
		}{{"comment", commentText, 1}, {"string", stringText, 0}, {"type argument", typeArgument, 1}} {
			for _, at := range piece.text.FindAllStringSubmatchIndex(line, -1) {
				spans = append(spans, span{piece.kind, n, at[2*piece.sub] + 1, at[2*piece.sub+1] + 1})
			}
		}
	}
	lines := strings.Split(string(data), "\n")
	for _, kind := range []string{"code outside functions", "comment", "string", "type argument"} {
		if !slices.ContainsFunc(spans, func(s span) bool {
			return s.kind == kind && wouldBeSite.MatchString(lines[s.line-1][s.from-1:s.to-1])
		}) {
			return fmt.Errorf("%s holds no %s with a text that would be a site in a function's code", m.file, kind)
		}
	}
	for _, site := range m.sites {
		for _, s := range spans {
			if site.line == s.line && site.column >= s.from && site.column < s.to {
				return fmt.Errorf("the site %s is in a %s of %s\n%s", site, s.kind, m.file, m.w.report())
			}
		}
	}
	return nil
}

// chains is a TypeScript file whose functions call a function and index a
// list through an optional chain, and what each line is, its ?. deleted.
const chains = "export function call(a) {\n  return a?.();\n}\n\nexport function index(a) {\n  return a?.[0];\n}\n"

var chainsMutated = map[int]string{2: "  return a();", 6: "  return a[0];"}

// optionalChainsLoseTheirMark is whether mutation list gives a?.() one
// site, the ?. deleted, making it a(), and a?.[0] one, making it a[0].
func (m *mutateState) optionalChainsLoseTheirMark() error {
	if err := m.w.write("chains.ts", chains); err != nil {
		return err
	}
	sites, err := m.sitesOf("chains.ts")
	if err != nil {
		return err
	}
	lines := strings.Split(chains, "\n")
	for line, want := range chainsMutated {
		var got []string
		for _, s := range sites {
			if s.line == line && s.original == "?." {
				text := lines[line-1]
				got = append(got, text[:s.column-1]+s.replacement+text[s.column-1+len(s.original):])
			}
		}
		if !slices.Equal(got, []string{want}) {
			return fmt.Errorf("the ?. of %q mutates to %q, not %q alone; the sites are %v", lines[line-1], got, want, sites)
		}
	}
	return nil
}

// sameSitesIn is whether a copy of the listed file under each extension
// the sentence names has the sites it has, at the same places, in the same
// functions.
func (m *mutateState) sameSitesIn(extensions string) error {
	if m.file == "" {
		return errors.New("no file's sites were listed to compare")
	}
	data, err := m.w.readIn(m.w.dir, m.file)
	if err != nil {
		return err
	}
	want, err := m.sitesOf(m.file)
	if err != nil {
		return err
	}
	stem := strings.TrimSuffix(m.file, path.Ext(m.file))
	for _, ext := range regexp.MustCompile(`\.\w+`).FindAllString(extensions, -1) {
		name := stem + ext
		if err := m.w.write(name, string(data)); err != nil {
			return err
		}
		got, err := m.sitesOf(name)
		if err != nil {
			return err
		}
		if len(got) == 0 || !slices.EqualFunc(got, want, func(a, b listedSite) bool {
			a.file, b.file = "", ""
			return a == b
		}) {
			return fmt.Errorf("%s has the sites %v, not %s's %v", name, got, m.file, want)
		}
	}
	return nil
}

// siteParts are the words of a site's format, each with what it matches.
var siteParts = map[string]string{
	"file": `(\S+?)`, "line": `(\d+)`, "column": `(\d+)`, "original": "([^`]+)", "replacement": "([^`]*)",
	"namespace": `([\w.$/-]+)`, "name": `(\w+)`,
}

// eachSitePrintedAs is whether every line of the last run's stdout is a
// site of ID-MUT-06's src/board.py in the format, its words file, line,
// column, original, replacement, namespace and name standing for the
// site's, and every site of the file is printed so, in order.
func (m *mutateState) eachSitePrintedAs(format string) error {
	var order []string
	words := regexp.MustCompile(`\b(?:file|line|column|original|replacement|namespace|name)\b`)
	pattern := words.ReplaceAllStringFunc(regexp.QuoteMeta(format), func(word string) string {
		order = append(order, word)
		return siteParts[word]
	})
	line := regexp.MustCompile("^" + pattern + "$")
	var got, want []string
	for _, text := range outputLines(strings.ReplaceAll(m.w.stdout, "\r\n", "\n")) {
		match := line.FindStringSubmatch(text)
		if match == nil {
			return fmt.Errorf("%q is not printed as %q\n%s", text, format, m.w.report())
		}
		fields := map[string]string{}
		for i, word := range order {
			fields[word] = match[i+1]
		}
		got = append(got, fmt.Sprintf("%s:%s:%s %s %s %s#%s", filepath.ToSlash(fields["file"]), fields["line"], fields["column"],
			fields["original"], fields["replacement"], fields["namespace"], fields["name"]))
	}
	for _, s := range pythonSites {
		want = append(want, fmt.Sprintf("%s:%d:%d %s %s %s", s.file, s.line, s.column, s.original, s.replacement, s.function))
	}
	if !slices.Equal(got, want) {
		return fmt.Errorf("the sites printed are\n%s\nnot\n%s\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"), m.w.report())
	}
	return nil
}

// The project of ID-MUT-101: src/app.ts's routes, of a site on line 2,
// before the route callback app.get("/u", …), and of two on line 7, after
// it, the callback a function of its own with a site on line 4; its test
// script passes whatever is mutated, so every mutant survives.
const appTS = `export function routes(app, a) {
  if (a > 1) {
    app.get("/u", (req, res) => {
      return req.x < 2;
    });
  }
  return a === 0;
}
`

var appSites = []Site{
	{"src/app.ts", 2, ">", ">="}, {"src/app.ts", 2, "1", "0"}, {"src/app.ts", 4, "<", "<="},
	{"src/app.ts", 7, "===", "!=="}, {"src/app.ts", 7, "0", "1"},
}

// appWithCallback writes the project, declares its tests, and checks the
// function has sites on the lines before, inside and after the callback,
// those inside in a function of their own.
func (m *mutateState) appWithCallback() error {
	if err := m.writeAll(map[string]string{
		"package.json": `{"name": "m", "scripts": {"test": "exit 0"}}` + "\n",
		"src/app.ts":   appTS,
	}); err != nil {
		return err
	}
	if err := m.w.d.Tests(m.w.dir, Tests{Language: "typescript", Command: "npm run test --silent", Surviving: appSites}); err != nil {
		return err
	}
	sites, err := m.sitesOf("src/app.ts")
	if err != nil {
		return err
	}
	byLine := map[int]string{}
	var got []Site
	for _, s := range sites {
		if f, ok := byLine[s.line]; ok && f != s.function {
			return fmt.Errorf("the sites of line %d are in two functions: %v", s.line, sites)
		}
		byLine[s.line] = s.function
		got = append(got, Site{s.file, s.line, s.original, s.replacement})
	}
	if !slices.Equal(got, appSites) || byLine[2] != byLine[7] || byLine[4] == byLine[2] {
		return fmt.Errorf("src/app.ts's sites are %v, not those of lines 2 and 7 in one function and line 4's in the callback", sites)
	}
	return nil
}

// inLineOrder is whether the last run listed every site of its file, by
// line and then column.
func (m *mutateState) inLineOrder() error {
	if err := m.listed(); err != nil {
		return err
	}
	want, err := m.sitesOf(m.file)
	if err != nil {
		return err
	}
	if len(m.sites) == 0 || len(m.sites) != len(want) {
		return fmt.Errorf("the run listed %d sites of %s's %d\n%s", len(m.sites), m.file, len(want), m.w.report())
	}
	if !slices.IsSortedFunc(m.sites, func(a, b listedSite) int {
		if a.line != b.line {
			return a.line - b.line
		}
		return a.column - b.column
	}) {
		return fmt.Errorf("the sites are not listed in line and column order\n%s", m.w.report())
	}
	return nil
}

// inTheOrderOf is whether the command's --json object lists the listed
// file's key, its mutants, as the last run listed its sites: the same
// places, texts and functions, in the same order.
func (m *mutateState) inTheOrderOf(args, key string) error {
	words, err := commandWords(args)
	if err != nil {
		return err
	}
	code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), words...)
	if err != nil {
		return err
	}
	var object struct {
		Files []map[string]json.RawMessage
	}
	if err := json.Unmarshal([]byte(stdout), &object); err != nil {
		return fmt.Errorf("itos-cc %s printed no JSON object: %v\nexit %d\n%s%s", args, err, code, stdout, stderr)
	}
	for _, file := range object.Files {
		var name string
		_ = json.Unmarshal(file["file"], &name)
		if filepath.ToSlash(name) != m.file {
			continue
		}
		var mutants []struct {
			Line, Column                    int
			Original, Replacement, Function string
		}
		if err := json.Unmarshal(file[key], &mutants); err != nil {
			return fmt.Errorf("%s's %q is no list of mutants: %v\n%s", m.file, key, err, stdout)
		}
		var got []listedSite
		for _, mu := range mutants {
			got = append(got, listedSite{file: m.file, line: mu.Line, column: mu.Column, original: mu.Original,
				replacement: mu.Replacement, function: mu.Function})
		}
		if len(got) == 0 || !slices.Equal(got, m.sites) {
			return fmt.Errorf("itos-cc %s lists %s's %s\n%v\nnot as mutation list did\n%v\n%s", args, m.file, key, got, m.sites, stderr)
		}
		return nil
	}
	return fmt.Errorf("itos-cc %s lists no file %s\nexit %d\n%s%s", args, m.file, code, stdout, stderr)
}

// Functions sharing a name.

// The Go module of "Rule: Functions sharing a name": src/setup.go's two
// init functions, both example.com/m/src#init, each setting a variable its
// test checks, so every mutant of both is killed.
const (
	mutateModule = "module example.com/m\n\ngo 1.22\n"
	initID       = "example.com/m/src#init"
	firstInit    = "func init() {\n\ta = 1 + 1\n}\n"
	secondInit   = "func init() {\n\tb = 3 - 1\n}\n"
	setupTest    = "package board\n\nimport \"testing\"\n\nfunc TestSetup(t *testing.T) {\n\tif a != 2 || b != 2 {\n\t\tt.Fatal(\"setup\")\n\t}\n}\n"
	setupSource  = "src/setup.go"
)

// goTests are the tests of the Go modules here, as mutation run runs them
// over src: every mutant they run is killed.
var goTests = Tests{Language: "go", Command: "go test -count=1 -failfast ./src"}

// setupProject writes the module, declares what its tests do and cover,
// and checks each init function has a site.
func (m *mutateState) setupProject() error {
	if err := m.writeAll(map[string]string{
		"go.mod":            mutateModule,
		setupSource:         "package board\n\nvar a, b int\n\n" + firstInit + "\n" + secondInit,
		"src/setup_test.go": setupTest,
	}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go", Report: "SF:src/setup.go\nDA:6,1\nDA:10,1\nend_of_record\n"}); err != nil {
		return err
	}
	if err := m.w.d.Tests(m.w.dir, goTests); err != nil {
		return err
	}
	inits, sites, err := m.initSites()
	if err != nil {
		return err
	}
	if bodies := strings.Split(m.read(setupSource), "func init() {"); len(inits) != 2 || len(bodies) != 3 || bodies[1] == bodies[2] {
		return fmt.Errorf("src/setup.go declares no two init functions with different bodies:\n%s", m.read(setupSource))
	}
	for i, in := range sites {
		if len(in) == 0 {
			return fmt.Errorf("init function %d of src/setup.go has no site: %v", i+1, sites)
		}
	}
	return nil
}

// read is the text of name, slash-separated from the scratch folder, ""
// when there is none.
func (m *mutateState) read(name string) string {
	data, _ := m.w.readIn(m.w.dir, name)
	return string(data)
}

// lineRange is the lines a function spans, its first and its last.
type lineRange struct{ first, last int }

func (l lineRange) holds(line int) bool { return line >= l.first && line <= l.last }

// initSites are the lines each init function of src/setup.go spans now, in
// file order, and the sites mutation list gives each, every one in
// example.com/m/src#init.
func (m *mutateState) initSites() ([]lineRange, [][]listedSite, error) {
	var inits []lineRange
	for i, line := range strings.Split(m.read(setupSource), "\n") {
		switch {
		case line == "func init() {":
			inits = append(inits, lineRange{first: i + 1})
		case line == "}" && len(inits) > 0 && inits[len(inits)-1].last == 0:
			inits[len(inits)-1].last = i + 1
		}
	}
	sites, err := m.sitesOf(setupSource)
	if err != nil {
		return nil, nil, err
	}
	each := make([][]listedSite, len(inits))
	for _, s := range sites {
		i := slices.IndexFunc(inits, func(l lineRange) bool { return l.holds(s.line) })
		if i < 0 || s.function != initID {
			return nil, nil, fmt.Errorf("the site %s is in no init function", s)
		}
		each[i] = append(each[i], s)
	}
	return inits, each, nil
}

// summary is what mutation run says of a file on stdout.
type summary struct{ killed, survived, uncovered, ran, reused int }

var summaryLine = regexp.MustCompile(`(?m)^(\S+): (\d+) killed, (\d+) survived, (\d+) uncovered \(ran (\d+), reused (\d+)\)\r?$`)

// summaryOf is what stdout, a mutation run's, says of file, and whether it
// says anything.
func summaryOf(stdout, file string) (summary, bool) {
	for _, match := range summaryLine.FindAllStringSubmatch(stdout, -1) {
		if filepath.ToSlash(match[1]) != file {
			continue
		}
		n := make([]int, 5)
		for i := range n {
			n[i], _ = strconv.Atoi(match[i+2])
		}
		return summary{n[0], n[1], n[2], n[3], n[4]}, true
	}
	return summary{}, false
}

// trialLine is mutation run's line on stderr for a mutant it ran.
var trialLine = regexp.MustCompile(`(?m)^itos-cc: \[\d+/\d+\] (\S+):(\d+) `)

// trials are the lines of the mutants of file a mutation run's stderr says
// it ran, one for each.
func trials(stderr, file string) []int {
	var out []int
	for _, match := range trialLine.FindAllStringSubmatch(stderr, -1) {
		if filepath.ToSlash(match[1]) == file {
			line, _ := strconv.Atoi(match[2])
			out = append(out, line)
		}
	}
	return out
}

// previousRun runs mutation run over src/setup.go, which must kill every
// mutant of both init functions, and keeps the text the run leaves.
func (m *mutateState) previousRun() error {
	_, each, err := m.initSites()
	if err != nil {
		return err
	}
	all := len(each[0]) + len(each[1])
	code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), "mutation", "run", filepath.FromSlash(setupSource))
	if err != nil {
		return err
	}
	if s, ok := summaryOf(stdout, setupSource); code != 0 || !ok || s != (summary{killed: all, ran: all}) {
		return fmt.Errorf("the previous run did not kill the %d mutants of both init functions: exit %d\n%s%s", all, code, stdout, stderr)
	}
	m.setup = m.read(setupSource)
	return nil
}

// runUnchanged runs itos-cc with args once src/setup.go is as the previous
// run left it.
func (m *mutateState) runUnchanged(args string) error {
	if m.setup == "" || m.read(setupSource) != m.setup {
		return errors.New("src/setup.go is not as a previous run left it")
	}
	return m.w.runWith(m.w.env(), args)
}

// bothReused is whether the last run reused every mutant of both init
// functions and ran none.
func (m *mutateState) bothReused() error {
	_, each, err := m.initSites()
	if err != nil {
		return err
	}
	s, ok := summaryOf(m.w.stdout, setupSource)
	if !ok || len(each[0]) == 0 || len(each[1]) == 0 || s.ran != 0 || s.reused != len(each[0])+len(each[1]) || len(trials(m.w.stderr, setupSource)) != 0 {
		return fmt.Errorf("the run did not reuse the %d and %d mutants of the two init functions alone\n%s", len(each[0]), len(each[1]), m.w.report())
	}
	return nil
}

// secondChanged changes the second init function's body.
func (m *mutateState) secondChanged() error {
	text := m.read(setupSource)
	if !strings.Contains(text, secondInit) {
		return fmt.Errorf("src/setup.go holds no second init function to change:\n%s", text)
	}
	return m.w.write(setupSource, strings.Replace(text, "b = 3 - 1", "b = 4 - 2", 1))
}

// initsSwapped swaps the two init functions, each unchanged.
func (m *mutateState) initsSwapped() error {
	text := m.read(setupSource)
	both := firstInit + "\n" + secondInit
	if !strings.Contains(text, both) {
		return fmt.Errorf("src/setup.go does not hold the two init functions in order:\n%s", text)
	}
	return m.w.write(setupSource, strings.Replace(text, both, secondInit+"\n"+firstInit, 1))
}

// onlyProblemOfSecond is whether the last run's one problem is of rule,
// naming the second init function of src/setup.go by its name and line.
func (m *mutateState) onlyProblemOfSecond(rule string) error {
	object, report, err := m.w.lastObject()
	if err != nil {
		return err
	}
	inits, _, err := m.initSites()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	if len(problems) != 1 {
		return fmt.Errorf("the run reported %d problems, not one\n%s", len(problems), report)
	}
	p, _ := problems[0].(map[string]any)
	file, _ := p["file"].(string)
	if p["rule"] != rule || filepath.ToSlash(file) != setupSource || p["function"] != initID || p["line"] != float64(inits[1].first) {
		return fmt.Errorf("the problem is %v, not %s of %s at line %d\n%s", p, rule, initID, inits[1].first, report)
	}
	return nil
}

// runsOnlySecond is whether a run of args runs the second init function's
// mutants, every one, and no other.
func (m *mutateState) runsOnlySecond(args string) error {
	inits, each, err := m.initSites()
	if err != nil {
		return err
	}
	if err := m.w.runWith(m.w.env(), args); err != nil {
		return err
	}
	ran := trials(m.w.stderr, setupSource)
	s, _ := summaryOf(m.w.stdout, setupSource)
	if len(ran) == 0 || len(ran) != len(each[1]) || s.ran != len(ran) || slices.ContainsFunc(ran, func(l int) bool { return !inits[1].holds(l) }) {
		return fmt.Errorf("the run ran mutants on lines %v, not the %d of the second init function, lines %d to %d\n%s",
			ran, len(each[1]), inits[1].first, inits[1].last, m.w.report())
	}
	return nil
}

// reusesEvery is whether a run of args reuses every mutant of src/setup.go
// and runs none.
func (m *mutateState) reusesEvery(args string) error {
	_, each, err := m.initSites()
	if err != nil {
		return err
	}
	if err := m.w.runWith(m.w.env(), args); err != nil {
		return err
	}
	all := len(each[0]) + len(each[1])
	if s, ok := summaryOf(m.w.stdout, setupSource); !ok || s.ran != 0 || s.reused != all || len(trials(m.w.stderr, setupSource)) != 0 {
		return fmt.Errorf("the run did not reuse the %d mutants of src/setup.go alone\n%s", all, m.w.report())
	}
	return nil
}

// Sources itos-cc.yaml excludes.

// The Go module of "Rule: Sources itos-cc.yaml excludes are not mutated":
// src/board.go's place, which TestPlace kills, and clear, which only
// TestClear, where a scenario has it, reaches, beside a fake in a package
// of its own, which no test reaches. Line 7 of the fake holds three sites,
// the first the 0 at column 9.
const (
	boardGo = `package board

type Board struct{}

func (b *Board) place(i int) bool {
	// past five
	return i > 5
}

func (b *Board) clear(i int) bool {
	return i < 3
}
`
	placeTest = `package board

import "testing"

func TestPlace(t *testing.T) {
	var b Board
	if b.place(5) || !b.place(6) {
		t.Fatal("place")
	}
}
`
	clearTest = `
func TestClear(t *testing.T) {
	var b Board
	if !b.clear(2) || b.clear(3) {
		t.Fatal("clear")
	}
}
`
	fakeGo = `package porttest

// Fake stands in for a board in tests.
type Fake struct{ n int }

func (f *Fake) Over(i int) bool {
	return 0 < i-f.n
}
`
	// fakeException excepts the fake's survivor at line 7, column 9, as
	// a person or mutation except writes one.
	fakeException = `mutation:
  exceptions:
    - file: %s
      function: example.com/m/src/port/porttest.Fake#Over
      hash: "0000000000000000"
      line_in_function: 2
      column: 9
      original: "0"
      replacement: "1"
      reason: the fake's bound is never read
`
)

// boardModule writes the module with the fake at fake, its tests killing
// clear's mutant too where both, and declares what they do and cover.
func (m *mutateState) boardModule(fake string, both bool) error {
	tests, report := placeTest, "SF:src/board.go\nDA:7,1\nDA:11,0\nend_of_record\n"
	if both {
		tests, report = placeTest+clearTest, "SF:src/board.go\nDA:7,1\nDA:11,1\nend_of_record\n"
	}
	if !strings.HasSuffix(fake, "/porttest/fake.go") {
		return fmt.Errorf("the fake is %s, not a porttest package's fake.go", fake)
	}
	if err := m.writeAll(map[string]string{
		"go.mod": mutateModule, "src/board.go": boardGo, "src/board_test.go": tests, fake: fakeGo,
	}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go", Report: report}); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, goTests)
}

// onlyCommitChanges writes the module beside the itos-cc.yaml a Given
// wrote, with the results of a run on main, the fake's mutants reached by
// no test, then commits a change to the fake alone on a branch of main.
func (m *mutateState) onlyCommitChanges(fake string) error {
	if m.read("itos-cc.yaml") == "" {
		return errors.New("no itos-cc.yaml was written before the module")
	}
	if err := m.boardModule(fake, false); err != nil {
		return err
	}
	if strings.Contains(m.read("src/board_test.go"), "porttest") {
		return errors.New("a test reaches the fake")
	}
	if err := m.w.git("init", "-q", "-b", "main"); err != nil {
		return err
	}
	if code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), "mutation", "run", "--no-annotate"); err != nil || code != 0 {
		return fmt.Errorf("the run on main failed: exit %d, %v\n%s%s", code, err, stdout, stderr)
	}
	for _, args := range [][]string{
		{"add", "-A"}, {"commit", "-qm", "base"}, {"checkout", "-qb", "work"},
	} {
		if err := m.w.git(args...); err != nil {
			return err
		}
	}
	if err := m.w.write(fake, strings.Replace(fakeGo, "0 < i-f.n", "1 < i-f.n", 1)); err != nil {
		return err
	}
	if err := m.w.git("add", "-A"); err != nil {
		return err
	}
	return m.w.git("commit", "-qm", "change the fake")
}

// siteMention is a site a command names: its file and line, and its column
// where it gives one, before the text a mutant replaces.
var siteMention = regexp.MustCompile("(\\S+):\\d+(?::\\d+)? `")

// neitherNames is whether each command names a site, and none in file,
// whose name neither says at all.
func (m *mutateState) neitherNames(first, second, file string) error {
	for _, args := range []string{first, second} {
		words, err := commandWords(args)
		if err != nil {
			return err
		}
		code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), words...)
		if err != nil {
			return err
		}
		output := filepath.ToSlash(stdout + stderr)
		mentions := siteMention.FindAllStringSubmatch(output, -1)
		if len(mentions) == 0 || strings.Contains(output, path.Base(file)) {
			return fmt.Errorf("itos-cc %s names no site, or names %s: exit %d\n%s", args, path.Base(file), code, output)
		}
	}
	return nil
}

// excludedSnapshot writes the module in a git repository and records a
// run's results of every source, the fake's at fake among them, all fresh.
func (m *mutateState) excludedSnapshot(fake string) error {
	if err := m.boardModule(fake, true); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-qm", "base"}} {
		if err := m.w.git(args...); err != nil {
			return err
		}
	}
	for _, args := range [][]string{{"mutation", "run"}, {"mutation", "check"}} {
		if code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), args...); err != nil || code != 0 {
			return fmt.Errorf("itos-cc %s: exit %d, %v\n%s%s", strings.Join(args, " "), code, err, stdout, stderr)
		}
	}
	m.source, m.snapshot = fake, ".metrics/mutate/"+fake+".json"
	if !m.w.d.Exists(filepath.Join(m.w.dir, filepath.FromSlash(m.snapshot))) {
		return fmt.Errorf("the run recorded no %s", m.snapshot)
	}
	return nil
}

// lists is whether the last run's --json object's key lists an entry of
// file with name value.
func (m *mutateState) lists(key, file, name, value string) error {
	object, err := m.w.json()
	if err != nil {
		return err
	}
	entries, _ := object[key].([]any)
	for _, one := range entries {
		entry, _ := one.(map[string]any)
		if got, _ := entry["file"].(string); filepath.ToSlash(got) == file {
			if entry[name] != value {
				return fmt.Errorf("%q lists %s with %q %v, not %q\n%s", key, file, name, entry[name], value, m.w.report())
			}
			return nil
		}
	}
	return fmt.Errorf("%q does not list %s\n%s", key, file, m.w.report())
}

// snapshotRemoved is whether the snapshot the Given recorded is gone, and
// the last run's key lists its source with name value.
func (m *mutateState) snapshotRemoved(key, name, value string) error {
	if m.snapshot == "" {
		return errors.New("no Given recorded a snapshot")
	}
	if m.w.d.Exists(filepath.Join(m.w.dir, filepath.FromSlash(m.snapshot))) {
		return fmt.Errorf("%s is still there\n%s", m.snapshot, m.w.report())
	}
	return m.lists(key, m.source, name, value)
}

// exceptsAndExcludes writes the module with the fake at fake, and an
// itos-cc.yaml excluding glob and excepting a survivor of the fake.
func (m *mutateState) exceptsAndExcludes(fake, glob string) error {
	if err := m.boardModule(fake, true); err != nil {
		return err
	}
	return m.w.write("itos-cc.yaml", "exclude: ["+strconv.Quote(glob)+"]\n"+fmt.Sprintf(fakeException, fake))
}

// reports is whether the last run's --json object reports a problem of
// rule with name value.
func (m *mutateState) reports(rule, name, value string) error {
	object, err := m.w.json()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	for _, one := range problems {
		if p, _ := one.(map[string]any); p["rule"] == rule && p[name] == value {
			return nil
		}
	}
	return fmt.Errorf("no problem is %s with %q %q\n%s", rule, name, value, m.w.report())
}

// headExcludes commits the module with an itos-cc.yaml excluding glob,
// then leaves one excluding nothing in the working tree, where mutation
// list lists the fake's sites: only the file HEAD commits leaves them out.
func (m *mutateState) headExcludes(glob string) error {
	if err := m.w.d.Counted(); err != nil {
		return err
	}
	fake := "src/port/porttest/fake.go"
	if err := m.boardModule(fake, false); err != nil {
		return err
	}
	if err := m.w.write("itos-cc.yaml", "exclude: ["+strconv.Quote(glob)+"]\n"); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-qm", "base"}} {
		if err := m.w.git(args...); err != nil {
			return err
		}
	}
	if err := m.w.write("itos-cc.yaml", "exclude: []\n"); err != nil {
		return err
	}
	sites, err := m.sitesOf(".")
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(sites, func(s listedSite) bool { return s.file == fake }) {
		return fmt.Errorf("mutation list lists no site of %s: %v", fake, sites)
	}
	return nil
}

// noSelectedSiteIn is whether the last run's --json object selects sites,
// none in file.
func (m *mutateState) noSelectedSiteIn(file string) error {
	object, err := m.w.json()
	if err != nil {
		return err
	}
	selected, _ := object["selected"].([]any)
	if len(selected) == 0 {
		return fmt.Errorf("the run selected no site\n%s", m.w.report())
	}
	for _, one := range selected {
		site, _ := one.(map[string]any)
		if got, _ := site["file"].(string); filepath.ToSlash(got) == file {
			return fmt.Errorf("the run selected %v, in %s\n%s", site, file, m.w.report())
		}
	}
	return nil
}

// Running mutants.

// announced is a command mutation run's stderr says it runs unmutated: the
// folder it runs in, as itos-cc names it, slash-separated, and its words.
type announced struct {
	dir, command string
	words        []string
}

var baselineLine = regexp.MustCompile(`^itos-cc: baseline (.*?)\$ (.+)$`)

// baselines are the baselines stderr, a mutation run's, announces.
func baselines(stderr string) []announced {
	var out []announced
	for _, line := range strings.Split(strings.ReplaceAll(stderr, "\r\n", "\n"), "\n") {
		if match := baselineLine.FindStringSubmatch(line); match != nil {
			out = append(out, announced{dir: filepath.ToSlash(match[1]), command: match[2], words: strings.Fields(match[2])})
		}
	}
	return out
}

// trial is a mutant a mutation run's stderr says it ran: its file,
// slash-separated, line, texts, what the run said of it and the seconds it
// took.
type trial struct {
	file                           string
	line                           int
	original, replacement, outcome string
	elapsed                        float64
}

var trialOutcome = regexp.MustCompile("^itos-cc: \\[\\d+/\\d+\\] (\\S+):(\\d+) `([^`]*)` → (?:`([^`]*)`|\\(deleted\\)) (.+?) \\(([\\d.]+)s(?:, [^)]*)?\\)$")

// trialsOf are the mutants of file stderr says the run ran.
func trialsOf(stderr, file string) []trial {
	var out []trial
	for _, line := range strings.Split(strings.ReplaceAll(stderr, "\r\n", "\n"), "\n") {
		match := trialOutcome.FindStringSubmatch(line)
		if match == nil || filepath.ToSlash(match[1]) != file {
			continue
		}
		n, _ := strconv.Atoi(match[2])
		took, _ := strconv.ParseFloat(match[6], 64)
		out = append(out, trial{file: file, line: n, original: match[3], replacement: match[4], outcome: match[5], elapsed: took})
	}
	return out
}

// uncoveredListed are the mutants of file stdout lists as uncovered, as
// --fail-uncovered lists them: "uncovered <file>:<line>:<column> `<original>` → …".
func uncoveredListed(stdout, file string) []listedSite {
	var out []listedSite
	for _, line := range strings.Split(strings.ReplaceAll(stdout, "\r\n", "\n"), "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "uncovered ")
		if !ok {
			continue
		}
		match := listedLine.FindStringSubmatch(rest)
		if match == nil || filepath.ToSlash(match[1]) != file {
			continue
		}
		n, _ := strconv.Atoi(match[2])
		column, _ := strconv.Atoi(match[3])
		out = append(out, listedSite{file: file, line: n, column: column, original: match[4], replacement: match[5], function: match[6]})
	}
	return out
}

// sameSites is whether got and want hold the same sites, in any order.
func sameSites(got, want []listedSite) bool {
	key := func(s listedSite) string { return s.String() }
	a, b := make([]string, 0, len(got)), make([]string, 0, len(want))
	for _, s := range got {
		a = append(a, key(s))
	}
	for _, s := range want {
		b = append(b, key(s))
	}
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

// sitesWhere are the sites of file mutation list gives on whose line keep
// holds.
func (m *mutateState) sitesWhere(file string, keep func(listedSite) bool) ([]listedSite, error) {
	sites, err := m.sitesOf(file)
	if err != nil {
		return nil, err
	}
	var out []listedSite
	for _, s := range sites {
		if keep(s) {
			out = append(out, s)
		}
	}
	return out, nil
}

// subsequence is whether want's words are in words, in order.
func subsequence(words, want []string) bool {
	i := 0
	for _, word := range words {
		if i < len(want) && word == want[i] {
			i++
		}
	}
	return i == len(want)
}

// contiguous is whether want's words are in words, one after another.
func contiguous(words, want []string) bool {
	for i := 0; i+len(want) <= len(words); i++ {
		if slices.Equal(words[i:i+len(want)], want) {
			return true
		}
	}
	return false
}

// testRecords makes the folder, outside the scratch folder, the project's
// tests record what they saw in, and gives its path.
func (m *mutateState) testRecords() (string, error) {
	if m.records != "" {
		return m.records, nil
	}
	dir, err := m.w.d.Folder()
	if err != nil {
		return "", err
	}
	m.records = dir
	return dir, nil
}

// testRecord is what one run of a project's tests recorded: where it ran,
// the real tree's folder or a copy, when its test began and ended, in
// milliseconds, and what else the project's test says.
type testRecord struct {
	Cwd       string   `json:"cwd"`
	Real      bool     `json:"real"`
	Start     int64    `json:"start"`
	End       int64    `json:"end"`
	Mutated   bool     `json:"mutated"`
	Make      bool     `json:"make"`
	Links     []string `json:"links"`
	Unchanged bool     `json:"unchanged"`
	Path      []string `json:"path"`
	File      string   `json:"file"`
}

// recorded are the runs of the project's tests, in the order they
// recorded them.
func (m *mutateState) runsRecorded() ([]testRecord, error) {
	if m.records == "" {
		return nil, errors.New("the project's tests record nothing")
	}
	data, err := m.w.d.Read(filepath.Join(m.records, "runs"))
	if err != nil {
		return nil, fmt.Errorf("the project's tests recorded no run: %v\n%s", err, m.w.report())
	}
	var out []testRecord
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var r testRecord
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, fmt.Errorf("the record %q is no run's: %v", line, err)
		}
		out = append(out, r)
	}
	return out, nil
}

// goRecorder is a Go test file's code that records a run: record(entry)
// writes the entry, with where the test runs, to the records folder.
func goRecorder(pkg, records, real string, imports ...string) string {
	all := append([]string{"encoding/json", "os", "path/filepath", "testing"}, imports...)
	slices.Sort(all)
	var b strings.Builder
	fmt.Fprintf(&b, "package %s\n\nimport (\n", pkg)
	for _, name := range all {
		fmt.Fprintf(&b, "\t%q\n", name)
	}
	fmt.Fprintf(&b, ")\n\nconst records, real = %s, %s\n\n", strconv.Quote(records), strconv.Quote(real))
	b.WriteString(`// record writes entry, with the folder the test runs in and whether it
// is the real tree's, to the records folder.
func record(t *testing.T, entry map[string]any) {
	cwd, _ := os.Getwd()
	here, _ := filepath.EvalSymlinks(cwd)
	there, _ := filepath.EvalSymlinks(real)
	entry["cwd"], entry["real"] = here, here == there
	data, _ := json.Marshal(entry)
	f, err := os.OpenFile(filepath.Join(records, "runs"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		t.Fatal(err)
	}
}
`)
	return b.String()
}

// The projects of "Each file runs its narrowest test command", each a
// source file and a test that reaches it.
const (
	pyBoard     = "def place(row, col):\n    return row > 0\n"
	pytestBoard = "from board import place\n\n\ndef test_place():\n    assert place(1, 2)\n    assert not place(0, 2)\n"
	unitBoard   = "import unittest\n\nfrom board import place\n\n\nclass TestPlace(unittest.TestCase):\n" +
		"    def test_place(self):\n        self.assertTrue(place(1, 2))\n        self.assertFalse(place(0, 2))\n"
	ktTest = "package board\n\nimport kotlin.test.Test\nimport kotlin.test.assertEquals\n\nclass BoardTest {\n" +
		"    @Test\n    fun placesPastOne() {\n        assertEquals(1, place(5))\n    }\n}\n"
	pyproject = "[project]\nname = \"board\"\nversion = \"0\"\n"
	// noPytest shadows pytest in the project's folder, where itos-cc asks
	// python whether it can import it.
	noPytest = "raise ImportError(\"pytest is not importable in this project\")\n"
)

// sourceInProject writes a project of language with what has says, its
// source file and a test that reaches it, and declares what its tests do.
func (m *mutateState) sourceInProject(language, has string) error {
	m.language, m.has = language, has
	w := m.w
	files := map[string]string{}
	var tests *Tests
	switch language + "/" + has {
	case "Go/go.mod":
		m.file = "src/board.go"
		files["go.mod"], files[m.file], files["src/board_test.go"] = mutateModule, boardGo, placeTest+clearTest
		tests = &Tests{Language: "go", Command: "go test -count=1 -failfast ./src"}
	case "TypeScript/vitest":
		m.file = "src/board.ts"
		files["package.json"] = `{"name": "board", "type": "module", "devDependencies": {"vitest": "5.0.2", "@vitest/coverage-v8": "5.0.2"}}` + "\n"
		files[m.file] = tsSource
		files["src/board.test.ts"] = "import { expect, test } from \"vitest\";\nimport { place } from \"./board\";\n\n" +
			"test(\"place\", () => {\n  expect(place(5)).toBe(3);\n});\n"
		tests = &Tests{Language: "typescript", Command: "node_modules/.bin/vitest related --run --bail=1 src/board.ts"}
	case "TypeScript/jest":
		// Jest is installed as a placeholder, which cannot run: the
		// scenario reads which command itos-cc chose.
		m.file = "src/board.ts"
		files["package.json"] = `{"name": "board", "devDependencies": {"jest": "30.0.0"}}` + "\n"
		files[m.file] = tsSource
		if err := (&coverageState{w: w}).placeholder(".", "jest", "30.0.0"); err != nil {
			return err
		}
	case "TypeScript/neither installed":
		// The project declares pnpm, which no run finds, so its script
		// never runs: the scenario reads which command itos-cc chose.
		m.file, m.pm = "src/board.ts", "pnpm"
		files["package.json"] = `{"name": "board", "packageManager": "pnpm@9.12.0", "scripts": {"test": "node -e \"\""}}` + "\n"
		files[m.file] = tsSource
		w.without = append(w.without, "pnpm")
	case "Python/pytest importable":
		m.file = "src/board.py"
		files["pyproject.toml"], files[m.file], files["tests/test_board.py"] = pyproject, pyBoard, pytestBoard
		tests = &Tests{Language: "python", Command: "python3 -m pytest -q -x -p no:cacheprovider tests/test_board.py"}
	case "Python/no pytest":
		m.file = "src/board.py"
		files["pyproject.toml"], files[m.file], files["tests/test_board.py"], files["pytest.py"] = pyproject, pyBoard, unitBoard, noPytest
		tests = &Tests{Language: "python", Command: "python3 -m unittest -f tests.test_board"}
	case "Kotlin/Gradle", "Kotlin/Maven":
		// Gradle and Maven are off the PATH, so neither runs: the scenario
		// reads which command itos-cc chose.
		m.file = "src/main/kotlin/board/Board.kt"
		files[m.file], files["src/test/kotlin/board/BoardTest.kt"] = ktSource, ktTest
		if has == "Gradle" {
			files["build.gradle.kts"], files["settings.gradle.kts"] = kotlinBuild("build.gradle.kts"), "rootProject.name = \"board\"\n"
		} else {
			files["pom.xml"] = kotlinBuild("pom.xml")
		}
		w.without = append(w.without, "gradle", "mvn")
	default:
		return fmt.Errorf("no %s project is written with %s", language, has)
	}
	if err := m.writeAll(files); err != nil {
		return err
	}
	if language == "TypeScript" && has == "vitest" {
		if err := w.d.Coverage(w.dir, Coverage{Language: "typescript"}); err != nil {
			return err
		}
	}
	if tests != nil {
		return w.d.Tests(w.dir, *tests)
	}
	return nil
}

// itsMutantsRun runs mutation run over the file the Given wrote, without
// coverage, which decides nothing of the command.
func (m *mutateState) itsMutantsRun() error {
	if m.file == "" {
		return errors.New("no Given wrote a source file")
	}
	return m.w.runEnv(m.w.dir, m.w.env(), "mutation", "run", "--no-coverage", filepath.FromSlash(m.file))
}

// pythonName is a Python interpreter's name.
var pythonName = regexp.MustCompile(`^python(?:3(?:\.\d+)?)?(?:\.exe)?$`)

// testCommandIs is whether the baseline the last run announced is the
// command spec says: its words, in order, among the command's, the first
// the command's program; "the installed <tool>" the project's
// node_modules/.bin/<tool>, "python" any Python interpreter, <pm> the
// package manager the project declares and <the file's test modules> the
// modules of the tests that reach the file. "on the file's package" says
// the command names the file's package folder, "on the file" the file,
// and "for the module" Gradle's -p and the module's folder, anywhere among
// its words; "via ./gradlew if present" checks
// a run with a gradlew in the build root runs it instead.
func (m *mutateState) testCommandIs(spec string) error {
	var after []string
	gradlew := false
	for _, q := range []struct {
		suffix string
		words  []string
	}{
		{" on the file's package", []string{"./" + path.Dir(m.file)}},
		{" on the file", []string{m.file}},
		{", the package manager the project declares", nil},
		{" for the module, via ./gradlew if present", []string{"-p", "."}},
	} {
		if rest, ok := strings.CutSuffix(spec, q.suffix); ok {
			spec, after = rest, q.words
			gradlew = strings.Contains(q.suffix, "gradlew")
			break
		}
	}
	installed := ""
	if rest, ok := strings.CutPrefix(spec, "the installed "); ok {
		spec = rest
		installed, _, _ = strings.Cut(rest, " ")
	}
	spec = strings.ReplaceAll(spec, "<pm>", m.pm)
	spec = strings.ReplaceAll(spec, "<the file's test modules>", "tests.test_board")
	want := strings.Fields(spec)
	if err := m.commandHolds(want, after, installed); err != nil {
		return err
	}
	if !gradlew {
		return nil
	}
	if err := m.w.write("gradlew", "#!/bin/sh\nexit 1\n"); err != nil {
		return err
	}
	if err := m.itsMutantsRun(); err != nil {
		return err
	}
	want[0] = "./gradlew"
	return m.commandHolds(want, after, "")
}

// commandHolds is whether the last run announced one baseline, whose
// words hold want, in order, and after, one after another, its program
// want's first, or the project's installed tool of that name.
func (m *mutateState) commandHolds(want, after []string, installed string) error {
	runs := baselines(m.w.stderr)
	if len(runs) != 1 || len(want) == 0 {
		return fmt.Errorf("the run announced %d baselines, not one\n%s", len(runs), m.w.report())
	}
	words := slices.Clone(runs[0].words)
	program := filepath.ToSlash(words[0])
	switch {
	case installed != "":
		if !strings.HasSuffix(program, "/node_modules/.bin/"+installed) && !strings.HasSuffix(program, "/node_modules/.bin/"+installed+".cmd") {
			return fmt.Errorf("the test command %q runs no installed %s\n%s", runs[0].command, installed, m.w.report())
		}
		words[0] = installed
	case want[0] == "python" && pythonName.MatchString(path.Base(program)):
		words[0] = "python"
	}
	if words[0] != want[0] || !subsequence(words, want) || !contiguous(words[1:], after) {
		return fmt.Errorf("the test command is %q, not %s with %s\n%s", runs[0].command, strings.Join(want, " "), strings.Join(after, " "), m.w.report())
	}
	return nil
}

// The module of "By default a file is measured and mutated by its own
// tests": a/a.go, whose package has no test of its own, and e2e/e2e_test.go,
// which imports it and kills every mutant of it.
const (
	e2eSource = "package a\n\n// Half is whether i is past two.\nfunc Half(i int) bool {\n\treturn i > 2\n}\n\n" +
		"// Zero is whether i is zero.\nfunc Zero(i int) bool {\n\treturn i == 0\n}\n"
	e2eTest = "package e2e\n\nimport (\n\t\"testing\"\n\n\t\"example.com/m/a\"\n)\n\nfunc TestE2E(t *testing.T) {\n" +
		"\tif a.Half(2) || !a.Half(3) || !a.Zero(0) || a.Zero(1) {\n\t\tt.Fatal(\"a\")\n\t}\n}\n"
)

// e2eModule writes the module and declares what its tests do and cover.
func (m *mutateState) e2eModule() error {
	m.file = "a/a.go"
	if err := m.writeAll(map[string]string{"go.mod": mutateModule, m.file: e2eSource, "e2e/e2e_test.go": e2eTest}); err != nil {
		return err
	}
	if m.w.d.Exists(filepath.Join(m.w.dir, "a", "a_test.go")) || !strings.Contains(e2eTest, "\"example.com/m/a\"") {
		return errors.New("a/a.go is not tested by e2e/e2e_test.go alone")
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go", Report: "SF:a/a.go\nDA:5,0\nDA:10,0\nend_of_record\n"}); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, Tests{Language: "go", Command: "go test -count=1 -failfast ./..."})
}

// allUncovered is whether the last run left every mutant of file
// uncovered, and with none, ran none.
func (m *mutateState) allUncovered(file string, none bool) error {
	sites, err := m.sitesOf(file)
	if err != nil {
		return err
	}
	s, ok := summaryOf(m.w.stdout, file)
	if !ok || len(sites) == 0 || s.uncovered != len(sites) || s.killed+s.survived != 0 {
		return fmt.Errorf("the run did not leave the %d mutants of %s uncovered\n%s", len(sites), file, m.w.report())
	}
	if none && (s.ran != 0 || len(trials(m.w.stderr, file)) != 0) {
		return fmt.Errorf("the run ran mutants of %s\n%s", file, m.w.report())
	}
	return nil
}

// wholeSuite is whether the last run measured coverage and ran the
// mutants' baseline with commands each of which is one of the list's,
// its words in order among the command's, its program the list's first.
func (m *mutateState) wholeSuite(list string) error {
	var suites [][]string
	for _, one := range regexp.MustCompile(`, or |, `).Split(list, -1) {
		suites = append(suites, strings.Fields(one))
	}
	coverage := (&coverageState{w: m.w}).announced(m.w.stderr)
	runs := baselines(m.w.stderr)
	if len(coverage) == 0 || len(runs) == 0 || len(trials(m.w.stderr, m.file)) == 0 {
		return fmt.Errorf("the run announced no coverage command, baseline or mutant\n%s", m.w.report())
	}
	var commands [][]string
	for _, a := range coverage {
		commands = append(commands, a.words)
	}
	for _, r := range runs {
		commands = append(commands, r.words)
	}
	for _, words := range commands {
		if !slices.ContainsFunc(suites, func(suite []string) bool {
			return path.Base(filepath.ToSlash(words[0])) == suite[0] && subsequence(words, suite)
		}) {
			return fmt.Errorf("%q runs none of %s\n%s", strings.Join(words, " "), list, m.w.report())
		}
	}
	return nil
}

// e2eKills is whether the last run killed every mutant of the file, each
// run.
func (m *mutateState) e2eKills() error {
	sites, err := m.sitesOf(m.file)
	if err != nil {
		return err
	}
	if s, ok := summaryOf(m.w.stdout, m.file); !ok || len(sites) == 0 || s != (summary{killed: len(sites), ran: len(sites)}) {
		return fmt.Errorf("the run did not run and kill the %d mutants of %s\n%s", len(sites), m.file, m.w.report())
	}
	return nil
}

// laterRunReuses runs the last run again without --all-tests, which must
// reuse every kill and run nothing.
func (m *mutateState) laterRunReuses() error {
	if m.w.last == nil || !slices.Contains(m.w.last.args, "--all-tests") {
		return errors.New("no run with --all-tests came before")
	}
	sites, err := m.sitesOf(m.file)
	if err != nil {
		return err
	}
	args := slices.DeleteFunc(slices.Clone(m.w.last.args), func(a string) bool { return a == "--all-tests" })
	if err := m.w.runEnv(m.w.dir, m.w.env(), args...); err != nil {
		return err
	}
	if s, ok := summaryOf(m.w.stdout, m.file); !ok || s != (summary{killed: len(sites), reused: len(sites)}) || len(trials(m.w.stderr, m.file)) != 0 {
		return fmt.Errorf("the run without --all-tests did not reuse the %d kills of %s\n%s", len(sites), m.file, m.w.report())
	}
	return nil
}

// makeProject writes a module in src/, its build root, with a Makefile
// whose test target runs its tests, passing them a word that says make ran
// them, and a test that records where it ran.
func (m *mutateState) makeProject() error {
	records, err := m.testRecords()
	if err != nil {
		return err
	}
	m.file = "src/board.go"
	test := goRecorder("board", records, filepath.Join(m.w.dir, "src"), "slices") + `
func TestPlace(t *testing.T) {
	record(t, map[string]any{"make": slices.Contains(os.Args, "make-ran")})
	var b Board
	if b.place(5) || !b.place(6) {
		t.Fatal("place")
	}
}
`
	if err := m.writeAll(map[string]string{
		"src/go.mod": mutateModule, m.file: boardGo, "src/board_test.go": test,
		"src/Makefile": "test:\n\tgo test -count=1 ./... -args make-ran\n",
	}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(filepath.Join(m.w.dir, "src"), Coverage{Language: "go", Report: "SF:board.go\nDA:7,1\nDA:11,0\nend_of_record\n"}); err != nil {
		return err
	}
	return m.w.d.Tests(filepath.Join(m.w.dir, "src"), Tests{Language: "go", Command: "make test",
		Surviving: []Site{{"board.go", 11, "<", "<="}}})
}

// shellFromBuildRoot is whether the last run announced command as its
// baseline, from the file's build root, and the command, make's recipe,
// ran the tests in a copy of that folder.
func (m *mutateState) shellFromBuildRoot(command string) error {
	root := path.Dir(m.file)
	if !slices.ContainsFunc(baselines(m.w.stderr), func(a announced) bool { return a.command == command && a.dir == root }) {
		return fmt.Errorf("the run announced no baseline %q from %s\n%s", command, root, m.w.report())
	}
	runs, err := m.runsRecorded()
	if err != nil {
		return err
	}
	ran := 0
	for _, r := range runs {
		if r.Real {
			// coverage, measured in the real tree
			continue
		}
		ran++
		if !r.Make || filepath.Base(r.Cwd) != path.Base(root) {
			return fmt.Errorf("a run of the tests in %s did not come from make's recipe in a copy of %s: %+v\n%s", r.Cwd, root, r, m.w.report())
		}
	}
	if ran == 0 {
		return fmt.Errorf("no run of the tests came from %q\n%s", command, m.w.report())
	}
	return nil
}

// The project of "The real tree is never modified while tests run": a
// TypeScript source whose test script, check.js, records where it ran,
// which of node_modules and .venv are links there, and whether the real
// tree's src/board.ts was as written before and after it, then fails when
// its own copy of src/board.ts is mutated.
const (
	copiedTS = "export function place(i: number, j: number): boolean {\n  return i > 1 && j < 5;\n}\n"
	checkJS  = `const fs = require("fs");
const path = require("path");
const records = %s;
const real = %s;
const original = %s;
const unchanged = () => fs.readFileSync(path.join(real, "src", "board.ts"), "utf8") === original;
const here = fs.realpathSync(process.cwd());
const entry = {
  cwd: here,
  real: here === fs.realpathSync(real),
  start: Date.now(),
  links: ["node_modules", ".venv"].filter((dir) => fs.lstatSync(dir).isSymbolicLink()),
  unchanged: unchanged(),
};
const until = Date.now() + 300;
while (Date.now() < until) {}
entry.end = Date.now();
entry.unchanged = entry.unchanged && unchanged();
fs.appendFileSync(path.join(records, "runs"), JSON.stringify(entry) + "\n");
process.exit(fs.readFileSync(path.join("src", "board.ts"), "utf8") === original ? 0 : 1);
`
)

// copiedProject writes the project, with a node_modules and a .venv of
// its own.
func (m *mutateState) copiedProject() error {
	records, err := m.testRecords()
	if err != nil {
		return err
	}
	m.file, m.original = "src/board.ts", copiedTS
	if err := m.writeAll(map[string]string{
		"package.json":                  `{"name": "board", "scripts": {"test": "node check.js"}}` + "\n",
		m.file:                          copiedTS,
		"check.js":                      fmt.Sprintf(checkJS, strconv.Quote(records), strconv.Quote(m.w.dir), strconv.Quote(copiedTS)),
		"node_modules/dep/index.js":     "module.exports = 1;\n",
		"node_modules/dep/package.json": `{"name": "dep", "version": "1.0.0"}` + "\n",
		".venv/pyvenv.cfg":              "include-system-site-packages = false\n",
	}); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, Tests{Language: "typescript", Command: "npm run test --silent"})
}

// mutantRuns are the runs the project's tests recorded in a worker's
// copy, at least one a mutant's beside the baseline.
func (m *mutateState) mutantRuns() ([]testRecord, error) {
	runs, err := m.runsRecorded()
	if err != nil {
		return nil, err
	}
	copies := slices.DeleteFunc(slices.Clone(runs), func(r testRecord) bool { return r.Real })
	if len(copies) < 2 {
		return nil, fmt.Errorf("the tests ran %d times outside the real tree, not a baseline and a mutant's: %+v\n%s", len(copies), runs, m.w.report())
	}
	return copies, nil
}

// privateCopies is whether every run of the tests ran in a worker's copy,
// none in the real tree, and no two at once in one copy.
func (m *mutateState) privateCopies() error {
	runs, err := m.runsRecorded()
	if err != nil {
		return err
	}
	if len(runs) < 2 {
		return fmt.Errorf("the tests ran %d times, not a baseline and a mutant's\n%s", len(runs), m.w.report())
	}
	for i, r := range runs {
		if r.Real {
			return fmt.Errorf("a run of the tests ran in the real tree, %s\n%s", r.Cwd, m.w.report())
		}
		for _, other := range runs[i+1:] {
			if other.Cwd == r.Cwd && other.Start < r.End && r.Start < other.End {
				return fmt.Errorf("two runs of the tests ran in %s at once\n%s", r.Cwd, m.w.report())
			}
		}
	}
	return nil
}

// dependenciesLinked is whether every run of the tests found each folder a
// link in its copy.
func (m *mutateState) dependenciesLinked(first, second string) error {
	runs, err := m.mutantRuns()
	if err != nil {
		return err
	}
	for _, r := range runs {
		for _, dir := range []string{first, second} {
			if !slices.Contains(r.Links, dir) {
				return fmt.Errorf("%s is no link in the copy %s: the links are %v", dir, r.Cwd, r.Links)
			}
		}
	}
	return nil
}

// treeUnchanged is whether every run of the tests found the real tree's
// source as written before and after it ran. The run annotates it once
// the tests are done.
func (m *mutateState) treeUnchanged() error {
	runs, err := m.mutantRuns()
	if err != nil {
		return err
	}
	for _, r := range runs {
		if !r.Unchanged {
			return fmt.Errorf("the run of the tests in %s found %s changed in the real tree\n%s", r.Cwd, m.file, m.w.report())
		}
	}
	return nil
}

// editableTest records where it ran, PYTHONPATH and the file board was
// imported from, each as the system resolves it.
const editableTest = `import json
import os

import board

RECORDS = %s
REAL = %s


def test_place():
    here = os.path.realpath(os.getcwd())
    entry = {
        "cwd": here,
        "real": here == os.path.realpath(REAL),
        "path": [os.path.realpath(p) for p in os.environ.get("PYTHONPATH", "").split(os.pathsep) if p],
        "file": os.path.realpath(board.__file__),
    }
    with open(os.path.join(RECORDS, "runs"), "a") as f:
        f.write(json.dumps(entry) + "\n")
    assert board.place(1, 2) and not board.place(0, 2)
`

// editableProject writes a Python project in src/ layout, and makes it
// installed in editable mode in its own virtualenv: a .pth file in the
// virtualenv's site-packages names the real tree's src/, as pip install -e
// writes one, for each Python 3 a virtualenv may hold.
func (m *mutateState) editableProject() error {
	records, err := m.testRecords()
	if err != nil {
		return err
	}
	m.file = "src/board.py"
	files := map[string]string{
		"pyproject.toml": pyproject, m.file: pyBoard,
		"tests/test_board.py": fmt.Sprintf(editableTest, strconv.Quote(records), strconv.Quote(m.w.dir)),
	}
	if err := m.writeAll(files); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "python", Venv: true}); err != nil {
		return err
	}
	pth := map[string]string{".venv/pyvenv.cfg": "include-system-site-packages = true\n"}
	for minor := 9; minor <= 20; minor++ {
		pth[fmt.Sprintf(".venv/lib/python3.%d/site-packages/__editable__.board-0.pth", minor)] = filepath.Join(m.w.dir, "src") + "\n"
	}
	if err := m.writeAll(pth); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, Tests{Language: "python", Command: ".venv/bin/python -m pytest -q -x -p no:cacheprovider tests/test_board.py"})
}

// pythonPathFirst is whether every run of the tests in a worker's copy had
// PYTHONPATH begin with the copy's first and second folders, and imported
// the source from the copy.
func (m *mutateState) pythonPathFirst(first, second string) error {
	runs, err := m.mutantRuns()
	if err != nil {
		return err
	}
	for _, r := range runs {
		want := []string{filepath.Join(r.Cwd, first), filepath.Join(r.Cwd, second)}
		if len(r.Path) < 2 || !slices.Equal(r.Path[:2], want) {
			return fmt.Errorf("PYTHONPATH in the copy %s is %v, not %v first", r.Cwd, r.Path, want)
		}
		if !strings.HasPrefix(r.File, r.Cwd+string(filepath.Separator)) {
			return fmt.Errorf("the tests in the copy %s imported %s", r.Cwd, r.File)
		}
	}
	return nil
}

// The module of "The baseline proves the copy works": Count, whose
// mutants of i - 1 never end, its every other mutant killed by its test,
// which takes half a second and records where it ran and whether its
// copy of board.go is mutated.
const countGo = `package board

// Count is how many steps take i down to zero.
func Count(i int) int {
	n := 0
	for i > 0 {
		i = i - 1
		n++
	}
	return n
}
`

// aboutToRun writes the module and runs its mutants.
func (m *mutateState) aboutToRun() error {
	records, err := m.testRecords()
	if err != nil {
		return err
	}
	m.file, m.original = "src/board.go", countGo
	test := goRecorder("board", records, filepath.Join(m.w.dir, "src"), "time") + fmt.Sprintf(`
func TestCount(t *testing.T) {
	start := time.Now()
	time.Sleep(500 * time.Millisecond)
	got := Count(3)
	text, _ := os.ReadFile("board.go")
	record(t, map[string]any{"start": start.UnixMilli(), "end": time.Now().UnixMilli(), "mutated": string(text) != %s})
	if got != 3 {
		t.Fatal("Count")
	}
}
`, strconv.Quote(countGo))
	if err := m.writeAll(map[string]string{"go.mod": mutateModule, m.file: countGo, "src/board_test.go": test}); err != nil {
		return err
	}
	if err := m.w.d.Tests(m.w.dir, Tests{Language: "go", Command: goTests.Command, Elapsed: 500 * time.Millisecond,
		TimingOut: []Site{{m.file, 7, "-", "+"}, {m.file, 7, "1", "0"}}}); err != nil {
		return err
	}
	return m.w.runEnv(m.w.dir, m.w.env(), "mutation", "run", "--no-coverage", filepath.FromSlash(m.file))
}

// baselineFirst is whether the first run of the tests was unmutated, in a
// worker's copy, announced before any mutant's, and mutants' runs came
// after it.
func (m *mutateState) baselineFirst() error {
	runs, err := m.mutantRuns()
	if err != nil {
		return err
	}
	slices.SortStableFunc(runs, func(a, b testRecord) int { return int(a.Start - b.Start) })
	if runs[0].Mutated || !slices.ContainsFunc(runs[1:], func(r testRecord) bool { return r.Mutated }) {
		return fmt.Errorf("the first run of the tests was not the unmutated one before the mutants': %+v\n%s", runs, m.w.report())
	}
	lines := strings.Split(strings.ReplaceAll(m.w.stderr, "\r\n", "\n"), "\n")
	base := slices.IndexFunc(lines, func(l string) bool { return baselineLine.MatchString(l) })
	first := slices.IndexFunc(lines, func(l string) bool { return trialOutcome.MatchString(l) })
	if base < 0 || first < 0 || first < base {
		return fmt.Errorf("the run did not announce its baseline before its first mutant\n%s", m.w.report())
	}
	return nil
}

// timeoutAllowance is the time beyond the factor times its baseline a
// mutant's run takes before it times out, as the feature says it.
const timeoutAllowance = 5.0

// timedOut are the last run's mutants of the file that timed out, and the
// longest of those that did not, in seconds.
func (m *mutateState) timedOut() ([]trial, float64, error) {
	var out []trial
	longest := -1.0
	for _, t := range trialsOf(m.w.stderr, m.file) {
		if t.outcome == "timeout" {
			out = append(out, t)
		} else {
			longest = max(longest, t.elapsed)
		}
	}
	if len(out) == 0 || longest < 0 {
		return nil, 0, fmt.Errorf("the run timed out no mutant of %s beside one it judged\n%s", m.file, m.w.report())
	}
	return out, longest, nil
}

// durationSetsTimeout is whether the mutant that timed out took the
// factor times the baseline's time, plus the allowance: no less than its
// test took, and no more than the longest other mutant's run took.
func (m *mutateState) durationSetsTimeout() error {
	runs, err := m.mutantRuns()
	if err != nil {
		return err
	}
	slices.SortStableFunc(runs, func(a, b testRecord) int { return int(a.Start - b.Start) })
	baseline := float64(runs[0].End-runs[0].Start) / 1000
	out, longest, err := m.timedOut()
	if err != nil {
		return err
	}
	factor := 10.0
	low, high := factor*baseline+timeoutAllowance, factor*longest+timeoutAllowance+1
	for _, t := range out {
		if t.elapsed < low || t.elapsed > high {
			return fmt.Errorf("the mutant at line %d timed out after %.1fs, not between %.1fs and %.1fs, as a baseline of %.2fs to %.1fs sets\n%s",
				t.line, t.elapsed, low, high, baseline, longest, m.w.report())
		}
	}
	return nil
}

// The Python project of a baseline that takes seconds: wait, whose
// mutant > → >= never ends, and past, whose mutant its test kills, which
// sleeps the baseline's time first. pytest is shadowed, so its tests run
// with unittest, as quick to start as Python is.
const (
	waitPy   = "def wait(i):\n    while i > 2:\n        pass\n    return i\n\n\ndef past(i):\n    return i > 5\n"
	waitTest = "import time\nimport unittest\n\nfrom board import past, wait\n\n\nclass TestBoard(unittest.TestCase):\n" +
		"    def test_board(self):\n        time.sleep(%s)\n        self.assertEqual(wait(2), 2)\n" +
		"        self.assertFalse(past(5))\n        self.assertTrue(past(6))\n"
	// waitGo is the Go source of a baseline under a second: past, whose
	// mutant the shell command waitCommand lets run 30 seconds, and low,
	// whose mutant it passes.
	waitGo = "package board\n\n// Past is whether i is past five.\nfunc Past(i int) bool {\n\treturn i > 5\n}\n\n" +
		"// Low is whether i is under three.\nfunc Low(i int) bool {\n\treturn i < 3\n}\n"
	waitCommand = `grep -q "i > 5" src/board.go && sleep %s || sleep 30`
)

// baselineTook writes a project whose tests take the time said: Python's,
// for a time of seconds, else a Go source tested by a shell command, quick
// to start, its run a sleep of that time.
func (m *mutateState) baselineTook(n int, unit string) error {
	took := time.Duration(n) * time.Millisecond
	if unit == "seconds" {
		took = time.Duration(n) * time.Second
	}
	m.baseline = took
	seconds := strconv.FormatFloat(took.Seconds(), 'f', -1, 64)
	if took >= time.Second {
		// board.py is at the root, where coverage, which sets no
		// PYTHONPATH, imports it.
		m.file = "board.py"
		if err := m.writeAll(map[string]string{
			"pyproject.toml": pyproject, m.file: waitPy, "pytest.py": noPytest,
			"tests/test_board.py": fmt.Sprintf(waitTest, seconds),
		}); err != nil {
			return err
		}
		if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "python", Report: "SF:board.py\nDA:2,1\nDA:4,1\nDA:8,1\nend_of_record\n"}); err != nil {
			return err
		}
		return m.w.d.Tests(m.w.dir, Tests{Language: "python", Command: "python3 -m unittest -f tests.test_board", Elapsed: took,
			TimingOut: []Site{{m.file, 2, ">", ">="}}})
	}
	m.file, m.command = "src/board.go", fmt.Sprintf(waitCommand, seconds)
	if err := m.writeAll(map[string]string{"go.mod": mutateModule, m.file: waitGo}); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, Tests{Language: "go", Command: m.command, Elapsed: took,
		TimingOut: []Site{{m.file, 5, ">", ">="}}, Surviving: []Site{{m.file, 10, "<", "<="}}})
}

// runWithDefaultFactor runs the Given's project's mutants with no
// --timeout-factor, which the sentence says is factor.
func (m *mutateState) runWithDefaultFactor(factor int) error {
	if m.command == "" {
		return errors.New("no Given wrote a project whose tests a command runs")
	}
	m.factor = float64(factor)
	return m.w.runEnv(m.w.dir, m.w.env(), "mutation", "run", "--no-coverage", "--test-command", m.command, filepath.FromSlash(m.file))
}

// factorOf is the timeout factor of the last run: its --timeout-factor,
// else the default the When said.
func (m *mutateState) factorOf() (float64, error) {
	if i := slices.Index(m.w.last.args, "--timeout-factor"); i >= 0 && i+1 < len(m.w.last.args) {
		return strconv.ParseFloat(m.w.last.args[i+1], 64)
	}
	if m.factor == 0 {
		return 0, errors.New("the run gave no --timeout-factor, and no When said its default")
	}
	return m.factor, nil
}

// numbers are the factors a sentence writes as a word.
var factorWords = map[string]float64{"two": 2, "five": 5, "ten": 10, "twenty": 20}

// timesOutAfter is whether every mutant the last run timed out took the
// seconds said: no less, and no more than the factor times what the
// longest other mutant's run took past the Given's baseline, and a second.
// Where the sentence says how, it is the factor times the baseline, plus
// the allowance.
func (m *mutateState) timesOutAfter(text, times, allowance string) error {
	after, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return err
	}
	factor, err := m.factorOf()
	if err != nil {
		return err
	}
	if m.baseline == 0 {
		return errors.New("no Given said how long the baseline took")
	}
	if times != "" {
		plus, _ := strconv.ParseFloat(allowance, 64)
		if factorWords[times] != factor || math.Abs(factor*m.baseline.Seconds()+plus-after) > 1e-9 {
			return fmt.Errorf("%s seconds is not %s times the baseline of %s, plus %s seconds, at the run's factor of %g", text, times, m.baseline, allowance, factor)
		}
	}
	out, longest, err := m.timedOut()
	if err != nil {
		return err
	}
	high := after + factor*max(0, longest-m.baseline.Seconds()) + 1
	for _, t := range out {
		if t.elapsed < after || t.elapsed > high {
			return fmt.Errorf("the mutant at line %d timed out after %.1fs, not %ss, its baseline %s and the longest other run %.1fs\n%s",
				t.line, t.elapsed, text, m.baseline, longest, m.w.report())
		}
	}
	return nil
}

// timeoutCountsKilled is whether the run's summary counts every mutant
// that timed out among those it killed.
func (m *mutateState) timeoutCountsKilled() error {
	out, _, err := m.timedOut()
	if err != nil {
		return err
	}
	killed := 0
	for _, t := range trialsOf(m.w.stderr, m.file) {
		if t.outcome == "killed" {
			killed++
		}
	}
	if s, ok := summaryOf(m.w.stdout, m.file); !ok || s.killed != killed+len(out) {
		return fmt.Errorf("the summary does not count the %d mutants that timed out among the %d killed\n%s", len(out), killed+len(out), m.w.report())
	}
	return nil
}

// failingTest prints thirty numbered lines, then fails.
const failingTest = "from board import place\n\n\ndef test_place():\n    for i in range(1, 31):\n" +
	"        print(\"line %02d\" % i)\n    assert not place(1, 2)\n"

// failingBaseline writes a Python project whose tests pass, records a
// run's results of the file, then makes its tests fail without any
// mutation.
func (m *mutateState) failingBaseline(file string) error {
	if file != "src/board.py" {
		return fmt.Errorf("no project is written for %s", file)
	}
	m.file = file
	if err := m.writeAll(map[string]string{"pyproject.toml": pyproject, file: pyBoard, "tests/test_board.py": pytestBoard}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "python", Report: "SF:src/board.py\nDA:2,1\nend_of_record\n"}); err != nil {
		return err
	}
	command := "python3 -m pytest -q -x -p no:cacheprovider tests/test_board.py"
	if err := m.w.d.Tests(m.w.dir, Tests{Language: "python", Command: command}); err != nil {
		return err
	}
	if code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), "mutation", "run", filepath.FromSlash(file)); err != nil || code != 0 {
		return fmt.Errorf("the run that records %s: exit %d, %v\n%s%s", file, code, err, stdout, stderr)
	}
	m.snapshot = ".metrics/mutate/" + file + ".json"
	if m.recorded = m.read(m.snapshot); m.recorded == "" {
		return fmt.Errorf("the run recorded no %s", m.snapshot)
	}
	if err := m.w.write("tests/test_board.py", failingTest); err != nil {
		return err
	}
	var printed strings.Builder
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&printed, "line %02d\n", i)
	}
	return m.w.d.Tests(m.w.dir, Tests{Language: "python", Command: command, Baseline: printed.String()})
}

// lastLinesFollow is whether n lines follow stdout's line saying the
// baseline fails, before the next file's summary, the end of what the
// failing test printed: its last line, and not its first.
func (m *mutateState) lastLinesFollow(n int) error {
	lines := outputLines(strings.ReplaceAll(m.w.stdout, "\r\n", "\n"))
	at := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, "baseline tests fail") })
	if at < 0 {
		return fmt.Errorf("stdout says no baseline fails\n%s", m.w.report())
	}
	follow := lines[at+1:]
	if end := slices.IndexFunc(follow, func(l string) bool { return summaryLine.MatchString(l) }); end >= 0 {
		follow = follow[:end]
	}
	if len(follow) != n || !slices.Contains(follow, "line 30") || slices.Contains(follow, "line 01") {
		return fmt.Errorf("%d lines follow, not the last %d of the test output\n%s", len(follow), n, m.w.report())
	}
	return nil
}

// snapshotUnchanged is whether file's snapshot is as the Given's run
// recorded it.
func (m *mutateState) snapshotUnchanged(file string) error {
	if m.recorded == "" || m.snapshot != ".metrics/mutate/"+file+".json" {
		return fmt.Errorf("no Given recorded a snapshot of %s", file)
	}
	if now := m.read(m.snapshot); now != m.recorded {
		return fmt.Errorf("the snapshot of %s changed:\n%s\nwas\n%s", file, now, m.recorded)
	}
	return nil
}

// workersProject writes a module of as many functions as mutants may run
// at once, each with one mutant, and a test of them all that sleeps three
// seconds, recording when.
func (m *mutateState) workersProject() error {
	records, err := m.testRecords()
	if err != nil {
		return err
	}
	n := max(4, runtime.NumCPU()/2)
	var source, checks strings.Builder
	source.WriteString("package board\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&source, "\n// F%02d is whether i is past %d.\nfunc F%02d(i int) bool {\n\treturn i > %d\n}\n", i, i+2, i, i+2)
		fmt.Fprintf(&checks, "\tif F%02d(%d) || !F%02d(%d) {\n\t\tt.Fatal(\"F%02d\")\n\t}\n", i, i+2, i, i+3, i)
	}
	m.file = "src/board.go"
	test := goRecorder("board", records, filepath.Join(m.w.dir, "src"), "time") + `
func TestAll(t *testing.T) {
	start := time.Now()
	time.Sleep(3 * time.Second)
	record(t, map[string]any{"start": start.UnixMilli(), "end": time.Now().UnixMilli()})
` + checks.String() + "}\n"
	if err := m.writeAll(map[string]string{"go.mod": mutateModule, m.file: source.String(), "src/board_test.go": test}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go"}); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, Tests{Language: "go", Command: goTests.Command, Elapsed: 3 * time.Second})
}

// atOnce is how many runs of the tests in workers' copies ran at the same
// time, at most.
func (m *mutateState) atOnce() (int, error) {
	runs, err := m.mutantRuns()
	if err != nil {
		return 0, err
	}
	most := 0
	for _, r := range runs {
		n := 0
		for _, other := range runs {
			if other.Start <= r.Start && r.Start < other.End {
				n++
			}
		}
		most = max(most, n)
	}
	return most, nil
}

// upToAtOnce is whether n runs of the tests ran at once, and never more.
func (m *mutateState) upToAtOnce(n int) error {
	got, err := m.atOnce()
	if err != nil {
		return err
	}
	if got != n {
		return fmt.Errorf("%d mutants ran at once, at most, not %d\n%s", got, n, m.w.report())
	}
	return nil
}

// defaultWorkers runs the last run again without --workers, its results
// and records gone, and is whether half the CPUs, at least one, ran at
// once.
func (m *mutateState) defaultWorkers() error {
	args := slices.Clone(m.w.last.args)
	if i := slices.Index(args, "--workers"); i >= 0 {
		args = slices.Delete(args, i, i+2)
	}
	for _, gone := range []string{filepath.Join(m.w.dir, ".metrics", "mutate"), filepath.Join(m.records, "runs")} {
		if err := m.w.d.Remove(gone); err != nil {
			return err
		}
	}
	if err := m.w.runEnv(m.w.dir, m.w.env(), args...); err != nil {
		return err
	}
	return m.upToAtOnce(max(1, runtime.NumCPU()/2))
}

// Coverage decides which mutants run.

// The TypeScript project of "Mutants on lines no test executes are not
// run": score, whose mutant its test kills, and place, which no test
// calls, its one site on line 12, the > at column 9.
const (
	placeBoardTS = `// The board's rules.

// score is whether a row is past the second.
export function score(row: number): boolean {
  return row > 2;
}

// place moves a piece past the fifth column,
// which no test reaches.
export function place(i: number): number {
  let n = i;
  if (i > 5) {
    n = i;
  }
  return n;
}
`
	placeBoardTestTS = "import { expect, test } from \"vitest\";\nimport { score } from \"./board\";\n\n" +
		"test(\"score\", () => {\n  expect(score(2)).toBe(false);\n  expect(score(3)).toBe(true);\n});\n"
	vitestPackage = `{"name": "board", "type": "module", "devDependencies": {"vitest": "5.0.2", "@vitest/coverage-v8": "5.0.2"}}` + "\n"
	vitestCommand = "node_modules/.bin/vitest related --run --bail=1 "
)

// lineNeverExecuted writes the project and declares what its tests do and
// cover, and checks the line holds a site in a function no test calls.
func (m *mutateState) lineNeverExecuted(line int) error {
	m.file = "src/board.ts"
	if err := m.writeAll(map[string]string{"package.json": vitestPackage, m.file: placeBoardTS, "src/board.test.ts": placeBoardTestTS}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "typescript",
		Report: "SF:src/board.ts\nDA:5,1\nDA:11,0\nDA:12,0\nDA:13,0\nDA:15,0\nend_of_record\n"}); err != nil {
		return err
	}
	if err := m.w.d.Tests(m.w.dir, Tests{Language: "typescript", Command: vitestCommand + m.file}); err != nil {
		return err
	}
	on, err := m.sitesWhere(m.file, func(s listedSite) bool { return s.line == line })
	if err != nil {
		return err
	}
	if len(on) == 0 || slices.ContainsFunc(on, func(s listedSite) bool {
		return strings.Contains(placeBoardTestTS, strings.TrimPrefix(s.function, "board#"))
	}) {
		return fmt.Errorf("line %d of %s holds no site in a function no test calls: %v", line, m.file, on)
	}
	return nil
}

// untestedBoard writes the project with no tools declared: no run may run
// its tests.
func (m *mutateState) untestedBoard() error {
	m.file = "src/board.ts"
	return m.writeAll(map[string]string{"package.json": vitestPackage, m.file: placeBoardTS, "src/board.test.ts": placeBoardTestTS})
}

// executedKilled checks every site of the file but those of functions no
// test calls is in one the test calls, every mutant of which it kills.
func (m *mutateState) executedKilled() error {
	sites, err := m.sitesOf(m.file)
	if err != nil {
		return err
	}
	for _, s := range sites {
		if s.function != "board#place" && !strings.Contains(placeBoardTestTS, strings.TrimPrefix(s.function, "board#")+"(") {
			return fmt.Errorf("the site %s is in a function the test does not call", s)
		}
	}
	return nil
}

// lineUncovered is whether the last run left every mutant on the line
// uncovered without running it, and ran the rest.
func (m *mutateState) lineUncovered(line int) error {
	on, err := m.sitesWhere(m.file, func(s listedSite) bool { return s.line == line })
	if err != nil {
		return err
	}
	all, err := m.sitesOf(m.file)
	if err != nil {
		return err
	}
	ran := trials(m.w.stderr, m.file)
	s, ok := summaryOf(m.w.stdout, m.file)
	if !ok || len(on) == 0 || s.uncovered != len(on) || s.ran != len(all)-len(on) || slices.Contains(ran, line) {
		return fmt.Errorf("the run did not leave the %d mutants on line %d uncovered, running the rest\n%s", len(on), line, m.w.report())
	}
	return nil
}

// unusedTS is a source no test imports.
const unusedTS = "export function half(i: number): boolean {\n  return i > 2;\n}\n\nexport function zero(i: number): boolean {\n  return i === 0;\n}\n"

// unusedProject writes a TypeScript project whose test imports
// src/board.ts and never file, and declares what its tests do and cover.
func (m *mutateState) unusedProject(file string) error {
	if file != "src/unused.ts" {
		return fmt.Errorf("no project is written that never loads %s", file)
	}
	m.file = file
	test := "import { expect, test } from \"vitest\";\nimport { place } from \"./board\";\n\n" +
		"test(\"place\", () => {\n  expect(place(5)).toBe(false);\n  expect(place(6)).toBe(true);\n});\n"
	if err := m.writeAll(map[string]string{
		"package.json": vitestPackage, "src/board.ts": "export function place(i: number): boolean {\n  return i > 5;\n}\n",
		file: unusedTS, "src/board.test.ts": test,
	}); err != nil {
		return err
	}
	if strings.Contains(test, "unused") {
		return errors.New("the test loads src/unused.ts")
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "typescript", Report: "SF:src/board.ts\nDA:2,1\nend_of_record\n"}); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, Tests{Language: "typescript", Command: vitestCommand + file})
}

// listsThemUncovered runs itos-cc with args, and is whether it lists
// every mutant of the file as uncovered and exits with code.
func (m *mutateState) listsThemUncovered(args string, code int) error {
	if err := m.w.runWith(m.w.env(), args); err != nil {
		return err
	}
	if err := m.everyListedUncovered(m.file); err != nil {
		return err
	}
	return m.w.exitsWith(code)
}

// everyListedUncovered is whether stdout lists every mutant of file as
// uncovered, and no other of it.
func (m *mutateState) everyListedUncovered(file string) error {
	sites, err := m.sitesOf(file)
	if err != nil {
		return err
	}
	if listed := uncoveredListed(m.w.stdout, file); len(sites) == 0 || !sameSites(listed, sites) {
		return fmt.Errorf("stdout lists %v as uncovered, not every mutant of %s, %v\n%s", listed, file, sites, m.w.report())
	}
	return nil
}

// kotlinUnmeasured writes a Kotlin module built with Maven and no JaCoCo,
// so coverage measures none of its files, a test class reaching its
// source, with Gradle and Maven off the PATH.
func (m *mutateState) kotlinUnmeasured() error {
	m.file = "src/Board.kt"
	pom := kotlinBuild("pom.xml")
	if strings.Contains(pom, "jacoco") {
		return errors.New("the pom declares JaCoCo")
	}
	m.w.without = append(m.w.without, "gradle", "mvn")
	return m.writeAll(map[string]string{"pom.xml": pom, m.file: ktSource, "src/test/kotlin/board/BoardTest.kt": ktTest})
}

// halfCovered writes the board module whose test calls place alone, so
// coverage would leave clear's mutant uncovered, and it survives a run.
func (m *mutateState) halfCovered() error {
	m.file = "src/board.go"
	if err := m.writeAll(map[string]string{"go.mod": mutateModule, m.file: boardGo, "src/board_test.go": placeTest}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go", Report: "SF:src/board.go\nDA:7,1\nDA:11,0\nend_of_record\n"}); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, Tests{Language: "go", Command: goTests.Command, Surviving: []Site{{m.file, 11, "<", "<="}}})
}

// everyMutantRuns is whether the last run ran every mutant of the file,
// none uncovered, and measured no coverage.
func (m *mutateState) everyMutantRuns() error {
	sites, err := m.sitesOf(m.file)
	if err != nil {
		return err
	}
	s, ok := summaryOf(m.w.stdout, m.file)
	if !ok || len(sites) == 0 || s.ran != len(sites) || s.uncovered != 0 || len(trials(m.w.stderr, m.file)) != len(sites) {
		return fmt.Errorf("the run did not run the %d mutants of %s\n%s", len(sites), m.file, m.w.report())
	}
	if announced := (&coverageState{w: m.w}).announced(m.w.stderr); len(announced) != 0 {
		return fmt.Errorf("the run measured coverage with %q\n%s", announced[0].command, m.w.report())
	}
	return nil
}

// markedTest is the board module's test of both functions, which leaves
// test-ran in its folder when it runs.
const markedTest = "package board\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestPlace(t *testing.T) {\n" +
	"\t_ = os.WriteFile(\"test-ran\", nil, 0o644)\n\tvar b Board\n\tif b.place(5) || !b.place(6) || !b.clear(2) || b.clear(3) {\n" +
	"\t\tt.Fatal(\"board\")\n\t}\n}\n"

// everyReusable writes the board module, its tests killing every mutant,
// and records a run of it, then removes the mark its tests left.
func (m *mutateState) everyReusable() error {
	m.file = "src/board.go"
	if err := m.writeAll(map[string]string{"go.mod": mutateModule, m.file: boardGo, "src/board_test.go": markedTest}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go", Report: "SF:src/board.go\nDA:7,1\nDA:11,1\nend_of_record\n"}); err != nil {
		return err
	}
	if err := m.w.d.Tests(m.w.dir, goTests); err != nil {
		return err
	}
	code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), "mutation", "run")
	if s, ok := summaryOf(stdout, m.file); err != nil || code != 0 || !ok || s.killed != 2 {
		return fmt.Errorf("the previous run did not kill every mutant: exit %d, %v\n%s%s", code, err, stdout, stderr)
	}
	m.snapshot = ".metrics/mutate/" + m.file + ".json"
	m.recorded = m.read(m.snapshot)
	return m.w.d.Remove(filepath.Join(m.w.dir, "src", "test-ran"))
}

// nothingRuns is whether the last run announced no coverage command,
// baseline or mutant, the project's tests left no mark, and a project no
// run recorded holds no .metrics.
func (m *mutateState) nothingRuns() error {
	for _, line := range strings.Split(strings.ReplaceAll(m.w.stderr, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "itos-cc: coverage") || baselineLine.MatchString(line) || trialLine.MatchString(line) {
			return fmt.Errorf("stderr says %q\n%s", line, m.w.report())
		}
	}
	if m.w.d.Exists(filepath.Join(m.w.dir, "src", "test-ran")) {
		return fmt.Errorf("the tests ran: they left src/test-ran\n%s", m.w.report())
	}
	if m.recorded == "" && m.w.d.Exists(filepath.Join(m.w.dir, ".metrics")) {
		return fmt.Errorf("the run left .metrics: a coverage or test command ran\n%s", m.w.report())
	}
	return nil
}

// progressLinesBegin is whether the last run's stderr announces its
// baseline and each mutant it ran on lines beginning prefix, and every
// line naming a baseline or a mutant's outcome begins so.
func (m *mutateState) progressLinesBegin(prefix string) error {
	about := regexp.MustCompile("baseline .*\\$ |\\[\\d+/\\d+\\] \\S+:\\d+ `")
	base, mutants := 0, 0
	for _, line := range strings.Split(strings.ReplaceAll(m.w.stderr, "\r\n", "\n"), "\n") {
		if !about.MatchString(line) {
			continue
		}
		if !strings.HasPrefix(line, prefix) {
			return fmt.Errorf("the line %q does not begin %q\n%s", line, prefix, m.w.report())
		}
		if strings.Contains(line, "baseline ") {
			base++
		} else {
			mutants++
		}
	}
	if base == 0 || mutants == 0 {
		return fmt.Errorf("stderr announces %d baselines and %d mutants\n%s", base, mutants, m.w.report())
	}
	return nil
}

// Coverage from tests that run the built binary.

// The module of "Coverage from tests that run the built binary": greet,
// tested only by e2e/e2e_test.go, which builds the binary, with -cover
// when GOCOVERDIR is set, records where GOCOVERDIR points, and runs it with
// one argument, so it reaches "one" and never "many", and checks stdout
// alone, so what main writes to stderr is never checked.
const (
	greetGo = `package main

import (
	"fmt"
	"os"
)

func main() {
	n := len(os.Args) - 1
	fmt.Println(greet(n))
	fmt.Fprintln(os.Stderr, n*2)
}

// greet tells one from many.
func greet(n int) string {
	if n > 1 {
		return fmt.Sprint(n-1, " more")
	}
	return "one"
}

// half is never run.
func half(i int) bool {
	return i > 2
}
`
	greetE2E = `package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const records = %s

func TestOne(t *testing.T) {
	if dir := os.Getenv("GOCOVERDIR"); dir != "" {
		f, err := os.OpenFile(filepath.Join(records, "gocoverdir"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString(dir + "\n")
		f.Close()
	}
	bin := filepath.Join(t.TempDir(), "greet.exe")
	build := []string{"build", "-o", bin, ".."}
	%s
	if out, err := exec.Command("go", build...).CombinedOutput(); err != nil {
		t.Fatalf("go %%v: %%v\n%%s", build, err, out)
	}
	out, err := exec.Command(bin, "x").Output()
	if err != nil {
		t.Fatalf("greet x: %%v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "one" {
		t.Fatalf("greet x printed %%q, want \"one\"", got)
	}
}
`
	greetCover = "if os.Getenv(\"GOCOVERDIR\") != \"\" {\n\t\tbuild = []string{\"build\", \"-cover\", \"-o\", bin, \"..\"}\n\t}"
	greetMany  = "package main\n\nimport \"testing\"\n\nfunc TestMany(t *testing.T) {\n" +
		"\tif got := greet(3); got != \"2 more\" {\n\t\tt.Fatalf(\"greet(3) = %q\", got)\n\t}\n}\n"
)

// The lines of main.go: those the binary runs, the branch it never takes
// and half, which nothing runs.
var (
	greetRan   = []int{9, 11, 16}
	greetNever = []int{17, 24}
)

// greetModule writes the module, its test building the binary with -cover
// where cover, and with a test in package main calling greet(3) where
// inProcess, and declares what its tests do and cover.
func (m *mutateState) greetModule(cover, inProcess bool) error {
	records, err := m.testRecords()
	if err != nil {
		return err
	}
	m.file = "main.go"
	flag := ""
	if cover {
		flag = greetCover
	}
	files := map[string]string{
		"go.mod": "module example.com/greet\n\ngo 1.22\n", m.file: greetGo,
		"e2e/e2e_test.go": fmt.Sprintf(greetE2E, strconv.Quote(records), flag),
	}
	if inProcess {
		files["main_test.go"] = greetMany
	}
	if err := m.writeAll(files); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go"}); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, Tests{Language: "go", Command: "go test -count=1 -failfast ./...",
		Surviving: []Site{{m.file, 11, "*", "/"}, {m.file, 17, "-", "+"}, {m.file, 17, "1", "0"}, {m.file, 24, ">", ">="}}})
}

// greetChecksOneBranch checks the test reads the binary's "one", and never
// its other branch's "more".
func (m *mutateState) greetChecksOneBranch() error {
	test := m.read("e2e/e2e_test.go")
	if !strings.Contains(test, `got != "one"`) || strings.Contains(test, "more") {
		return errors.New("e2e/e2e_test.go does not check one branch of the binary's output alone")
	}
	return nil
}

// greetSites are the sites of main.go on lines, every one where lines is
// nil.
func (m *mutateState) greetSites(lines []int) ([]listedSite, error) {
	return m.sitesWhere(m.file, func(s listedSite) bool { return lines == nil || slices.Contains(lines, s.line) })
}

// greetRanCovered is whether the last run ran every mutant on the lines
// the binary ran, listing none of them uncovered.
func (m *mutateState) greetRanCovered() error {
	sites, err := m.greetSites(greetRan)
	if err != nil {
		return err
	}
	ran := trialsOf(m.w.stderr, m.file)
	for _, s := range sites {
		if !slices.ContainsFunc(ran, func(t trial) bool {
			return t.line == s.line && t.original == s.original && t.replacement == s.replacement
		}) {
			return fmt.Errorf("the mutant %s did not run\n%s", s, m.w.report())
		}
	}
	if listed := uncoveredListed(m.w.stdout, m.file); slices.ContainsFunc(listed, func(s listedSite) bool { return slices.Contains(greetRan, s.line) }) {
		return fmt.Errorf("a mutant on a line the binary ran is listed as uncovered: %v\n%s", listed, m.w.report())
	}
	return nil
}

// outcomeOf is what the last run said of the mutant of the file at line
// replacing original with replacement, "" when it ran none.
func (m *mutateState) outcomeOf(line int, original, replacement string) string {
	for _, t := range trialsOf(m.w.stderr, m.file) {
		if t.line == line && t.original == original && t.replacement == replacement {
			return t.outcome
		}
	}
	return ""
}

// greetNotices is whether the mutant that makes the binary print "0 more"
// was killed, and the one changing only what it writes to stderr survived.
func (m *mutateState) greetNotices() error {
	if got := m.outcomeOf(16, ">", ">="); got != "killed" {
		return fmt.Errorf("main.go:16 > → >= %q, not killed: the binary prints \"0 more\"\n%s", got, m.w.report())
	}
	if got := m.outcomeOf(11, "*", "/"); got != "survived" {
		return fmt.Errorf("main.go:11 * → / %q, not survived: the test never reads stderr\n%s", got, m.w.report())
	}
	return nil
}

// greetNeverRanUncovered is whether stdout lists as uncovered the mutants
// on the lines the binary never ran, and no other.
func (m *mutateState) greetNeverRanUncovered() error {
	never, err := m.greetSites(greetNever)
	if err != nil {
		return err
	}
	if listed := uncoveredListed(m.w.stdout, m.file); len(never) == 0 || !sameSites(listed, never) {
		return fmt.Errorf("stdout lists %v as uncovered, not %v\n%s", listed, never, m.w.report())
	}
	return nil
}

// greetAllUncovered is whether stdout lists every mutant of main.go as
// uncovered.
func (m *mutateState) greetAllUncovered() error {
	return m.everyListedUncovered(m.file)
}

// greetMutants are main.go's mutants in the last run's --json object.
func (m *mutateState) greetMutants() ([]map[string]any, error) {
	object, err := m.w.json()
	if err != nil {
		return nil, err
	}
	files, _ := object["files"].([]any)
	for _, one := range files {
		f, _ := one.(map[string]any)
		if name, _ := f["file"].(string); filepath.ToSlash(name) != m.file {
			continue
		}
		var out []map[string]any
		list, _ := f["mutants"].([]any)
		for _, mu := range list {
			if entry, ok := mu.(map[string]any); ok {
				out = append(out, entry)
			}
		}
		if len(out) > 0 {
			return out, nil
		}
	}
	return nil, fmt.Errorf("the object lists no mutant of %s\n%s", m.file, m.w.report())
}

// lineOf is a mutant's line in the --json object.
func lineOf(mu map[string]any) int {
	line, _ := mu["line"].(float64)
	return int(line)
}

// greetCoverageKinds is whether each mutant the last run covered lists,
// as "coverage", the data covering its line: integration on the lines
// only the binary runs, in-process on the branch only greet(3) takes, and
// both on the line both reach.
func (m *mutateState) greetCoverageKinds(inProcess, integration string) error {
	want := map[int][]any{9: {integration}, 11: {integration}, 16: {inProcess, integration}, 17: {inProcess}}
	mutants, err := m.greetMutants()
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, mu := range mutants {
		if mu["outcome"] == "uncovered" {
			continue
		}
		got, _ := mu["coverage"].([]any)
		if !slices.Equal(got, want[lineOf(mu)]) {
			return fmt.Errorf("the mutant at line %d has \"coverage\" %v, not %v\n%s", lineOf(mu), mu["coverage"], want[lineOf(mu)], m.w.report())
		}
		seen[fmt.Sprint(got)] = true
	}
	if len(seen) != 3 {
		return fmt.Errorf("the covered mutants list %d kinds of coverage, not in-process, integration and both\n%s", len(seen), m.w.report())
	}
	return nil
}

// uncoveredHasNoCoverage is whether the last run left a mutant uncovered,
// and no uncovered one has "coverage".
func (m *mutateState) uncoveredHasNoCoverage() error {
	mutants, err := m.greetMutants()
	if err != nil {
		return err
	}
	n := 0
	for _, mu := range mutants {
		if mu["outcome"] != "uncovered" {
			continue
		}
		n++
		if c, has := mu["coverage"]; has {
			return fmt.Errorf("the uncovered mutant at line %d has \"coverage\" %v\n%s", lineOf(mu), c, m.w.report())
		}
	}
	if n == 0 {
		return fmt.Errorf("no mutant is uncovered\n%s", m.w.report())
	}
	return nil
}

// coverDir is the GOCOVERDIR the binary was first given, slash-separated
// from the scratch folder, and the run-* folder it is under.
func (m *mutateState) coverDir() (dir, run string, err error) {
	if m.records == "" {
		return "", "", errors.New("the project's test records nothing")
	}
	data, err := m.w.d.Read(filepath.Join(m.records, "gocoverdir"))
	if err != nil {
		return "", "", fmt.Errorf("the test was given no GOCOVERDIR: %v\n%s", err, m.w.report())
	}
	first, _, _ := strings.Cut(strings.TrimSpace(string(data)), "\n")
	rel, in := (&coverageState{w: m.w}).rel(strings.TrimSpace(first))
	parts := strings.Split(rel, "/")
	if !in || len(parts) < 4 || parts[0] != ".metrics" || parts[1] != "coverage" || !strings.HasPrefix(parts[2], "run-") {
		return "", "", fmt.Errorf("GOCOVERDIR is %s, not a directory under a run-* directory of .metrics/coverage", first)
	}
	return rel, path.Join(parts[:3]...), nil
}

// coverDirUnderRun is whether the binary's GOCOVERDIR was under the run's
// own run-* directory in .metrics/coverage/.
func (m *mutateState) coverDirUnderRun() error {
	_, _, err := m.coverDir()
	return err
}

// coverDirGone is whether neither GOCOVERDIR nor its run-* directory is
// left after the run.
func (m *mutateState) coverDirGone() error {
	dir, run, err := m.coverDir()
	if err != nil {
		return err
	}
	for _, left := range []string{dir, run} {
		if m.w.d.Exists(filepath.Join(m.w.dir, filepath.FromSlash(left))) {
			return fmt.Errorf("%s is left after the run\n%s", left, m.w.report())
		}
	}
	return nil
}

// Uncovered mutants as failures.

// eachListedAs is whether stdout lists every mutant of the file on line
// as text says, and lists one.
func (m *mutateState) eachListedAs(line int, text string) error {
	on, err := m.sitesWhere(m.file, func(s listedSite) bool { return s.line == line })
	if err != nil {
		return err
	}
	var listed []listedSite
	for _, s := range uncoveredListed(m.w.stdout, m.file) {
		if s.line == line {
			listed = append(listed, s)
		}
	}
	if len(on) == 0 || len(listed) != len(on) {
		return fmt.Errorf("stdout lists %d of the %d mutants on line %d as uncovered\n%s", len(listed), len(on), line, m.w.report())
	}
	for _, s := range listed {
		if got := "uncovered " + s.String(); got != text {
			return fmt.Errorf("stdout lists %q, not %q\n%s", got, text, m.w.report())
		}
	}
	return nil
}

// commitChanged writes the board module, no test of which executes
// either function, commits it and tags the commit ref, records a run's
// results, place's mutant and clear's uncovered, then commits a change to
// the function named.
func (m *mutateState) commitChanged(ref, function string) error {
	if function != "Board#place" {
		return fmt.Errorf("no commit is made changing %s", function)
	}
	m.file = "src/board.go"
	if err := m.writeAll(map[string]string{"go.mod": mutateModule, m.file: boardGo,
		"src/board_test.go": "package board\n\nimport \"testing\"\n\nfunc TestNothing(t *testing.T) {}\n"}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go", Report: "SF:src/board.go\nDA:7,0\nDA:11,0\nend_of_record\n"}); err != nil {
		return err
	}
	if err := m.w.d.Tests(m.w.dir, goTests); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-qm", ref}, {"tag", ref}} {
		if err := m.w.git(args...); err != nil {
			return err
		}
	}
	if code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), "mutation", "run"); err != nil || code != 0 {
		return fmt.Errorf("the run after %s: exit %d, %v\n%s%s", ref, code, err, stdout, stderr)
	}
	if err := m.w.write(m.file, strings.Replace(boardGo, "// past five\n", "// past five, and not at five\n", 1)); err != nil {
		return err
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-qm", "change place"}} {
		if err := m.w.git(args...); err != nil {
			return err
		}
	}
	return nil
}

// unchangedUnexecuted checks the function is as committed and no test
// calls it.
func (m *mutateState) unchangedUnexecuted(function string) error {
	name := function[strings.LastIndex(function, "#")+1:]
	if !strings.Contains(m.read(m.file), "func (b *Board) "+name+"(i int) bool {\n\treturn i < 3\n}") ||
		strings.Contains(m.read("src/board_test.go"), name+"(") {
		return fmt.Errorf("%s changed, or a test calls it", function)
	}
	return nil
}

// ofFunction is whether s is a site of the function a scenario names, the
// type and method of its name.
func ofFunction(s listedSite, function string) bool {
	return strings.HasSuffix(s.function, "."+function) || strings.HasSuffix(s.function, "/"+function)
}

// functionListedUncovered is whether stdout lists every mutant of the
// function as uncovered.
func (m *mutateState) functionListedUncovered(function string) error {
	sites, err := m.sitesWhere(m.file, func(s listedSite) bool { return ofFunction(s, function) })
	if err != nil {
		return err
	}
	var listed []listedSite
	for _, s := range uncoveredListed(m.w.stdout, m.file) {
		if ofFunction(s, function) {
			listed = append(listed, s)
		}
	}
	if len(sites) == 0 || !sameSites(listed, sites) {
		return fmt.Errorf("stdout lists %v of %s as uncovered, not %v\n%s", listed, function, sites, m.w.report())
	}
	return nil
}

// noneOfFunctionListed is whether stdout lists no mutant of the function
// as uncovered.
func (m *mutateState) noneOfFunctionListed(function string) error {
	for _, s := range uncoveredListed(m.w.stdout, m.file) {
		if ofFunction(s, function) {
			return fmt.Errorf("stdout lists %s as uncovered\n%s", s, m.w.report())
		}
	}
	return nil
}

// uncoveredProblems is whether every mutant the last run's --json object
// lists as uncovered is a problem of rule naming its file, line, column,
// function and texts.
func (m *mutateState) uncoveredProblems(rule string) error {
	object, err := m.w.json()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	files, _ := object["files"].([]any)
	n := 0
	for _, one := range files {
		f, _ := one.(map[string]any)
		list, _ := f["mutants"].([]any)
		for _, mu := range list {
			mutant, _ := mu.(map[string]any)
			if mutant["outcome"] != "uncovered" {
				continue
			}
			n++
			if !slices.ContainsFunc(problems, func(p any) bool {
				problem, _ := p.(map[string]any)
				file, _ := problem["file"].(string)
				named, _ := f["file"].(string)
				if problem["rule"] != rule || filepath.ToSlash(file) != filepath.ToSlash(named) {
					return false
				}
				for _, key := range []string{"line", "column", "function", "original", "replacement"} {
					if problem[key] == nil || problem[key] != mutant[key] {
						return false
					}
				}
				return true
			}) {
				return fmt.Errorf("the uncovered mutant %v is no %s problem naming it\n%s", mutant, rule, m.w.report())
			}
		}
	}
	if n == 0 {
		return fmt.Errorf("the object lists no uncovered mutant\n%s", m.w.report())
	}
	return nil
}
