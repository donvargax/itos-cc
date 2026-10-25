// The steps of mutate.feature: mutation list, run, check, sample and except
// over projects the Givens write. Like every step here, they read the
// binary's output and the files it leaves alone, never its code. Its
// scenarios gain steps a piece at a time (Unstepped, steps.go); those of
// the pieces not done yet are held by cmd/itos-cc's Go tests.
package features

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// mutateState is one mutate.feature scenario's state beside the world's.
type mutateState struct {
	w *world
	// execution is the state of the ownership and fail-fast steps, which
	// the interrupt's steps share.
	execution *mutationExecution

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
	// launched is when the last run whose baseline the tests record was
	// launched.
	launched time.Time
	// command is the --test-command a Given's project runs its tests
	// with, "" for its own, and factor the timeout factor a When says is
	// the default, 0 where it says none.
	command       string
	factor        float64
	quietCheck    func() error
	held          map[string]string
	failingSample func() error
	samplePlain   func() error
	forFile       func(string) error
}

// mutateFeature declares mutate.feature's steps, mutateSteps, which
// tools/bin/assemble lists in steps_gen.go.
var mutateFeature = featureFile{Name: "mutate.feature", Steps: mutateSteps}

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

	since := sinceMutationSteps(sc, m)
	checkMutationSteps(sc, m, since)
	commentFreeSteps(sc, m, since)
	listedMutationSteps(sc, m)
	sample := sampleMutationSteps(sc, m, since)
	resultMutationSteps(sc, m, since, sample)
	exceptionMutationSteps(sc, m, since)
	unjudgedEvidenceSteps(sc, m, since)
	languageMutationSteps(sc, m, since)
	strictMutationSteps(sc, m, since)
	executionMutationSteps(sc, m, since)
	countedMutationSteps(sc, m, since)
	emptyCaseSteps(sc, m)
	pristineSteps(sc, m)
	vitestThroughSteps(sc, m)
	workspaceCopySteps(sc, m)
	ignoredSteps(sc, m)
	capsSteps(sc, m)
	notBuiltSteps(sc, m)
	plainMatcherSteps(sc, m)

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
	sc.Step(`^the itos-cc\.yaml HEAD commits holds '([^']*)'$`, m.headHolds)
	sc.Step(`^its fix says "([^"]*)", then "([^"]*)"$`, m.fixSaysThen)

	// Running mutants.
	sc.Step(`^a source file in a (Go|TypeScript|Python|Kotlin|Dart) project with (.+)$`, m.sourceInProject)
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
	sc.Step(`^the test output follows whole, as it is under 40 lines$`, m.wholeOutputFollows)
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
	return m.writeProject(map[string][]string{"tests/test_board.py": {"src/board.py"}}, pythonSample)
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

// pythonProbe declares whether python3 imports pytest in the scratch
// folder, as the run asks it before choosing a Python test command.
func (m *mutateState) pythonProbe(importable bool) error {
	return m.w.d.Tools(context.Background(), m.w.dir, ToolSetup{Env: m.w.env(), Python: &PythonProbe{Interpreter: "python3", Importable: importable}})
}

// writeProject writes files, as writeAll does, and declares the import
// edges among them (imports).
func (m *mutateState) writeProject(edges map[string][]string, files map[string]string) error {
	if err := m.writeAll(files); err != nil {
		return err
	}
	m.w.imports(edges)
	return nil
}
