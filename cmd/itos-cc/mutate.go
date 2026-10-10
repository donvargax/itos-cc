package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/graph"
	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/mutate"
	"github.com/donvargax/itos-cc/project"
)

// mutationGroup is mutation testing: mutation run mutates and runs the
// tests, mutation list lists the sites without running any, mutation check
// reads the cached results without running any, mutation sample runs a few
// cached mutants again to see that their results hold, and mutation except
// excepts an equivalent mutant in itos-cc.yaml.
var mutationGroup = &command{
	name:    "mutation",
	summary: "mutation testing: do the tests notice small changes?",
	about: `
Mutation testing: would the tests notice if this code were wrong? mutation run
changes one operator, boolean, or 0/1 at a time inside each function and runs
the tests; mutation list lists those changes, the mutation sites, without
running any test; mutation check says whether the results mutation run
cached are there, fresh, and passing, without running any test either;
mutation sample runs a few of the cached mutants again and fails when an
outcome differs from the one recorded; and mutation except excepts an
equivalent mutant, a survivor no test can kill, in itos-cc.yaml, with the
reason no test can.`,
	examples: []string{
		"itos-cc mutation run --changed",
		"itos-cc mutation list src/billing/invoice.ts",
		"itos-cc mutation check --since origin/main --fail-uncovered",
		"itos-cc mutation sample --count 20",
		"itos-cc mutation except src/board.ts:3:13 --reason 'c is set again before it is read'",
	},
	subs: []*command{mutationRunCommand, mutationListCommand, mutationCheckCommand, mutationSampleCommand, mutationExceptCommand},
}

var mutationRunCommand = &command{
	name:     "mutation run",
	summary:  "mutate each function and run its tests: are the mutants killed?",
	synopsis: "[options] [path ...]",
	about: `
Changes one operator, boolean, or 0/1 at a time inside each function and runs
the file's own tests: its Go package, the Vitest or Jest tests that import it,
or the whole suite where nothing narrower exists. A mutant is killed when the
tests fail or time out and survives when they pass. Mutants on lines those
tests never execute are uncovered and are not run.

--all-tests runs the whole suite for coverage and for every mutant, so
integration and end-to-end tests can kill mutants too. It is slow: run it
nightly. In Go, a test that runs the built binary shows up in coverage when
it builds the binary with go build -cover while GOCOVERDIR is set: itos-cc
sets it, and merges what the binary wrote with go test's own coverage.
Elsewhere coverage does not see such tests: add --no-coverage for them.

Results are cached in .metrics/mutate/<file>.json, which is meant to be
committed: later runs reuse killed mutants of unchanged functions, and the
survivors itos-cc.yaml excepts, and retry only other survivors and changed
functions. Each snapshot also records the SHA-256
of every test file that imports its file (in Go, its package's tests and
those of the packages that import it); when one of them is added, changed,
or removed, every mutant of the file runs again. Each outcome records the
scope of the tests that decided it: "own" (the file's own tests, left out of
the file), "all-tests", or the --test-command line; a reused outcome keeps
the scope it was decided with, and mutation sample re-runs it there. A summary comment is kept at
the end of each source file; it lists each survivor, and each one itos-cc.yaml
excepts apart, with its reason.

For Go outcomes recorded with --all-tests or --test-command, freshness also
depends on every _test.go file beneath the source's nearest go.mod (nested
modules excluded, build-tagged tests included) and on files matched by
mutation.tests.support. Name feature files and other command inputs there;
inputs outside those files cannot be inferred. These checks use saved hashes
and never run a test or list command. Reusing a broad-scope result keeps its
scope and evidence; a partial run does not refresh evidence for functions it
did not judge. Legacy broad-scope Go results without this evidence rerun once.

--since REF judges only the functions the commits since REF changed, as a
gate on a branch's own work: git diff REF...HEAD, committed changes only.
Paths narrow it to the files under them. Functions not judged neither run nor
change in the snapshot, and only those judged count in the summary, the
problems, and the exit code. A file with no function judged is left as it
was: neither its snapshot nor its summary comment is written. Renames are
followed: a move is no change, and a renamed file's snapshot moves with it.

--count N judges at most N mutation sites freshly, as a bounded check
that needs no cache: the cache neither saves a trial nor is written. It
resolves the Git repository and its HEAD commit once and judges that
commit's tracked files, frozen in a private copy, so staged, unstaged and
untracked changes play no part; tools, dependencies and the environment are
used as installed, and nothing is installed. The sites of the selection,
paths and --since REF's changed functions, are ranked by SHA-256 over
--seed TEXT (the HEAD commit's id unless given) and each site's identity,
and the first N are selected across every file, function and worker.
Coverage, listed reach and a clean baseline of each selected file's own
tests are then measured on the frozen copy; any command failing fails the
run before any mutant. A selected site no test reaches is reported
uncovered, never run and never redrawn. Every other selected site runs its
own tests once, even when the cache holds a kill for it, and one that
survives them runs the listed tests that reach its line, after their
selection's clean baseline, as a complete run does: both stages are one
trial. A selection whose baseline fails is tests.selection-failed, and a
site that needs it is blocked, with no outcome. Exceptions of the functions
with a selected site apply as in a complete run: a valid one excepts its
survivor, which is still drawn and run, and a stale one fails, as
mutation.exception-stale. --count bounds mutant trials only, not discovery,
coverage, listing, baselines or total time. A counted run writes no
snapshot, summary comment or coverage cache, and its pass proves only the
judgments it reports, never a complete result: mutation check reports the
cache as it was. --fail-uncovered keeps its meaning, strict Go included,
for every admitted function. A range with no site is not applicable, which
is not a pass of a range whose tests or measurement failed. --count runs
on Linux only (Windows is #29); it refuses --changed, --no-coverage,
existing or raw coverage, --test-command and --mutate-all, and --seed
needs it. With --all-tests the own stage runs the whole suite, and a
survivor still runs the listed tests that reach it.

--fail-uncovered makes each uncovered mutant a failure, listed like a
survivor. For Go it also requires fresh measured executable coverage for
every judged function, including functions with no mutation sites, and fails
each uncovered positive-weight coverage block. Strict Go runs reject
--no-coverage, --coverage-report, --use-existing-coverage, and
--coverage-command. A matching independent cache can avoid remeasurement;
otherwise built-in or listed coverage must measure successfully. Empty or
comment-only function bodies have no executable coverage obligation. For
other languages the existing uncovered-mutant behavior is unchanged. With
--since, only the judged functions' uncovered mutants and executable blocks
count.

A survivor that itos-cc.yaml excepts (see mutation except) fails nothing: it
is counted excepted, not survived, and is reused without running, as a kill
is, while its function and the tests that import its file are unchanged.
An exception no longer holds, and fails as mutation.exception-stale, when
its function changed (its mutant is then judged as if it had none), when
its function or its site is gone, or when the mutant, run again after its
tests changed, is killed. An exception never excuses an uncovered mutant.
With --since, only the judged functions' exceptions count. --mutate-all
runs excepted mutants too.

An entry for a file the run does not select is judged too by a run of the
whole project (no paths, no --changed, no --since), and by a --since run
whose range deleted or renamed its file. When exactly one source selected
holds a function of the entry's name and hash, the entry has moved: it
still excepts the mutant at its site there, and fails as stale with why
moved and new_file, the file to name in the entry. Otherwise it is gone.
A run never writes itos-cc.yaml. An itos-cc.yaml that cannot be read is
config.invalid, and nothing runs.

Tests that only run the built program, such as an end-to-end suite that a
single test function runs, can be listed in itos-cc.yaml under
mutation.tests, so each mutant runs only those that reach its line:
  mutation:
    tests:
      list: <command printing a test a line: its ID, then a tab and its file, or not>
      run: <command running the tests {pattern} selects>
      ids_pattern: <the pattern, with {ids}>
      join: {each: <each ID, with {id}>, sep: <between them>}
      whole: <command running every test; optional, else run with every ID>
      support: [<globs of files every test depends on, optional>]
Commands run through the platform shell at the project root. When a mutant
has to run, the list command runs once, then every listed test, for
coverage, with ITOS_CC_TEST_COVERDIR set to a directory of the run's own:
a harness that gives the processes each test starts
GOCOVERDIR=<that directory>/<test ID> splits the coverage by test in one
run; otherwise each test runs alone, with a GOCOVERDIR of its own. A
mutant runs its file's own tests first and, only if it survives them, the
listed tests that reach its line, in one run: its outcome then has scope
"listed" and records their IDs. A line a listed test reaches is never
uncovered. The first time a mutant needs a selection of listed tests,
that selection runs once without any mutant, in the mutant's worker's
copy: its time, times --timeout-factor, plus 5s, is the timeout of
every mutant run of it. A selection that fails without a mutant is
tests.selection-failed: no mutant that would run it is judged, and the
files holding them keep their snapshots, as with a failing baseline
("baseline": "failed" in --json). Such
an outcome holds while the files its tests are defined
in, and the support files, are as it recorded them; mutation check calls
its function stale when one changed. A list command that fails is
tests.list-failed: nothing is
judged and no snapshot is written. Listed tests are not run with
--no-coverage, --all-tests, --test-command, --use-existing-coverage,
--coverage-command, or --coverage-report.`,
	flags: append(append(append([]flagSpec{}, selectionFlags...), coverageFlags...),
		opt("workers", intFlag, "N", fmt.Sprint(max(1, runtime.NumCPU()/2)), "mutants run at the same time"),
		sw("mutate-all", "rerun killed mutants of unchanged functions too"),
		opt("timeout-factor", floatFlag, "N", "10", "a mutant times out after N times the baseline duration, plus 5s"),
		opt("test-command", stringFlag, "CMD", "", "shell command that runs the tests, instead of the per-language default"),
		sw("no-annotate", "do not write the summary comment into source files"),
		opt("since", stringFlag, "REF", "", "judge only the functions the commits since REF changed (git diff REF...HEAD)"),
		sw("fail-uncovered", "fail on uncovered mutants and executable Go coverage blocks"),
		opt("count", intFlag, "N", "", "judge at most N committed mutation sites freshly, drawn across the whole selection; Linux only"),
		opt("seed", stringFlag, "TEXT", "", "with --count, seed the draw with TEXT instead of the HEAD commit's id")),
	json: `"files": [{"file", "killed", "survived", "excepted", "uncovered", "ran",
   "reused", "baseline": "passed"|"failed", "mutants": [{"line", "column",
   "function", "original", "replacement",
   "outcome": "killed"|"survived"|"timeout"|"uncovered", "reused",
   "scope": "own"|"all-tests"|"listed"|"<test command>", for an
   excepted survivor "excepted": "its reason", where this run's coverage
   executed its line "coverage": ["in-process", "integration"], either or
   both, and with scope "listed" "tests": ["<test ID>"]}], and with
   --since
   "judged": ["namespace#name"]}]; with --count, instead of "files":
   "sampling": {"budget", "eligible", "selected", "executed", "omitted",
   "seed", "algorithm", "commit", "since", "since_base",
   "assurance": "sampled"|"not-applicable",
   "completion": "completed"|"stopped"|"not-applicable", "stop", "bounds"},
   "selected": [{"identity", "file", "line", "column", "function",
   "original", "replacement",
   "state": "judged"|"uncovered"|"blocked"|"failed"|"unattempted",
   "outcome" (judged and uncovered sites only), "scope", with scope
   "listed" "tests": ["<test ID>"], for an excepted survivor "excepted":
   "its reason", "reason", and the stages its trial ran, "stages":
   [{"name": "own"|"listed-baseline"|"listed",
   "state": "complete"|"failed", "outcome", "tests", "error"}]}],
   "subjects": {"judged": [{"file", "function"}], "omitted": [...]},
   "stages": [{"name", "state": "complete"|"failed"|"skipped"|"aborted", "error"}]`,
	rules: []string{
		"mutation.survived         a mutant survived: file, line, column, function, original, replacement",
		"mutation.uncovered        with --fail-uncovered, no test executes a mutant: file, line, column, function, original, replacement",
		"mutation.uncovered-statement with --fail-uncovered, a measured executable Go coverage block is uncovered: file, function, line",
		"mutation.coverage-missing  with --fail-uncovered, a Go function lacks complete measured coverage evidence: file, function, line",
		"mutation.coverage-unsupported with --fail-uncovered, strict coverage reaches beyond the inventoried Go module: file, function, line",
		"mutation.exception-stale  an exception in itos-cc.yaml no longer holds: file, function, line (none when the function or its file is gone), column, original, replacement, why: killed|changed|gone|moved, and with moved new_file",
		"mutation.baseline-failed  the tests fail before any mutant: file",
		"config.invalid            itos-cc.yaml cannot be read: file",
		"tests.list-failed         the list command of mutation.tests failed: command, exit_code",
		"tests.selection-failed    a selection of listed tests fails without any mutant: ids, command, exit_code",
		"since.bad-ref             --since names no commit: ref",
		"since.no-git              --since outside a git repository",
		"flags.conflict            --since with --changed: flag",
		"flags.conflict            --fail-uncovered with --no-coverage, or strict Go coverage with raw coverage flags: flag",
		"flags.conflict            --seed without --count, or --count with a flag it refuses: flag",
		"count.platform            --count on a platform other than Linux: platform",
		"count.no-git              --count outside a Git repository, or before its first commit",
		"count.unsupported-scope   --count over committed content it cannot judge, such as a symlink or a submodule",
		"count.preparation-failed  with --count, a runtime Git, tool, listing, coverage, conversion or baseline step failed before any mutant: stage",
		"count.trial-failed        with --count, a selected mutant's trial could not run: file, line, column, function, original, replacement, identity",
	},
	exits: []exitDoc{
		{0, "every mutant that ran was killed"},
		{1, "a mutant survived, a mutant is uncovered with --fail-uncovered, an exception is stale, a file's tests fail before any mutant, the list command of mutation.tests failed, a selection of listed tests fails without any mutant, or with --count a preparation step failed or a selected mutant is not judged"},
		{2, "a usage or config error: a bad flag or path, a --since ref that is no commit, --since with --changed, a --count below 1, --seed without --count, committed content --count cannot judge, or an itos-cc.yaml that cannot be read"},
		{3, "--changed or --since outside a git repository; --count outside a Git repository with a commit, on a platform other than Linux, or with a required tool missing"},
	},
	examples: []string{
		"itos-cc mutation run --changed",
		"itos-cc mutation run --since origin/main --fail-uncovered  # a branch's own commits, as a gate",
		"itos-cc mutation run --all-tests --json                    # nightly",
		"itos-cc mutation run --count 20 --since origin/main        # a bounded fresh check of committed work (Linux)",
	},
	run: runMutate,
}

var mutationListCommand = &command{
	name:     "mutation list",
	summary:  "list the mutation sites without running tests",
	synopsis: "[options] [path ...]",
	about: `
Lists each mutation site of the production sources chosen, the changes
mutation run makes one at a time, without running any test, as
"file:line:column ` + "`original` → `replacement`" + ` in namespace#name",
each file's sites in line and column order.`,
	flags: append([]flagSpec{}, selectionFlags...),
	json:  `"sites": [{"file", "line", "column", "function", "original", "replacement"}]`,
	exits: []exitDoc{
		{0, "success"},
		{2, "a usage error: a bad flag or path"},
		{3, "--changed outside a git repository"},
	},
	examples: []string{
		"itos-cc mutation list src/billing/invoice.ts",
		"itos-cc mutation list --changed --json",
	},
	run: runMutationList,
}

type mutateFile struct {
	File     string `json:"file"`
	Killed   int    `json:"killed"`
	Survived int    `json:"survived"`
	// Excepted is the survivors itos-cc.yaml excepts, not in Survived.
	Excepted  int    `json:"excepted"`
	Uncovered int    `json:"uncovered"`
	Ran       int    `json:"ran"`
	Reused    int    `json:"reused"`
	Baseline  string `json:"baseline"`
	// Judged is there only with --since, empty when no function changed.
	Judged []string `json:"judged,omitzero"`
	// Mutants is every mutant of the functions judged, in site order;
	// empty, never null, when the baseline or a selection of listed tests
	// failed.
	Mutants []mutateMutant `json:"mutants"`
}

// mutateMutant is a site, less its file, and how it was decided: a
// timed-out mutant is "timeout" here and counts in "killed".
type mutateMutant struct {
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Function    string `json:"function"`
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
	Outcome     string `json:"outcome"`
	Reused      bool   `json:"reused"`
	// Scope is the scope of the tests that decided the outcome: "own",
	// "all-tests", or the --test-command line.
	Scope string `json:"scope"`
	// Excepted is the reason itos-cc.yaml gives, on an excepted survivor
	// only; its outcome stays survived.
	Excepted string `json:"excepted,omitempty"`
	// Coverage names the coverage that executed the mutant's line,
	// "in-process", "integration", or both; there only when this run's
	// coverage executed it.
	Coverage []string `json:"coverage,omitempty"`
	// Tests is, with scope "listed", the IDs of the listed tests that
	// decided the outcome.
	Tests []string `json:"tests,omitempty"`
}

type mutateResult struct {
	Files []mutateFile `json:"files"`
}

type mutateSite struct {
	File        string `json:"file"`
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Function    string `json:"function"`
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
}

// mutationSelection is the sources mutation run, check, and sample take:
// the paths and --changed, and with --since the files the range changed or
// renamed, with judge saying which of their functions it changed and renamed
// the path each renamed file had at the ref, "" for the others. judge and
// renamed are nil without --since.
func mutationSelection(in *invocation) (sources []string, judge func(path, function, hash string) bool, renamed func(path string) string, err error) {
	var since map[string]project.ChangedFunctions
	var moves map[string]string
	if in.set("since") {
		if since, moves, err = changedSince(in); err != nil {
			return nil, nil, nil, err
		}
	}
	files, err := files(in)
	if err != nil {
		return nil, nil, nil, err
	}
	if since == nil {
		return files.Sources, nil, nil, nil
	}
	// The range selects the files; paths only narrow it, by the path a
	// renamed file has now.
	for _, f := range files.Sources {
		if _, ok := since[f]; ok {
			sources = append(sources, f)
		}
	}
	return sources, func(path, function, hash string) bool { return since[path].Judges(function, hash) },
		func(path string) string { return moves[path] }, nil
}

// importingTests lists the test files that import each source, as the graph
// of the project under the project root resolves imports, whatever command
// runs the mutants and wherever it runs: what a snapshot records the hashes
// of.
func importingTests() (func(path string) []string, error) {
	root := project.Root()
	all, err := project.Discover([]string{root})
	if err != nil {
		return nil, err
	}
	tests, err := graph.TestsImporting(root, all)
	if err != nil {
		return nil, err
	}
	return func(path string) []string {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		return tests[path]
	}, nil
}

func runMutate(in *invocation) (any, error) {
	if in.set("count") || in.set("seed") {
		return runCountedMutate(in)
	}
	result := mutateResult{Files: []mutateFile{}}
	if in.set("fail-uncovered") && in.set("no-coverage") {
		return result, flagConflict("--no-coverage", "--no-coverage conflicts with --fail-uncovered, which requires measured Go executable coverage")
	}
	cfg, err := loadConfig()
	if err != nil {
		return result, err
	}
	sources, judge, renamed, err := mutationSelection(in)
	if err != nil {
		return result, err
	}
	elsewhere, moved, err := exceptionsElsewhere(in, cfg.Exceptions, sources, renamed)
	if err != nil {
		return result, err
	}
	if len(sources) == 0 {
		fmt.Fprintln(os.Stderr, "itos-cc: no source files to mutate")
		reportElsewhere(in, elsewhere)
		return result, nil
	}
	strictGo := in.set("fail-uncovered") && hasGoSource(sources)
	if strictGo {
		for _, flag := range []string{"--coverage-report", "--use-existing-coverage", "--coverage-command"} {
			if flagPresent(in, flag) {
				return result, flagConflict(flag, flag+" is not admitted with strict Go coverage; use a fresh built-in or listed measurement")
			}
		}
	}
	tests, err := importingTests()
	if err != nil {
		return result, err
	}
	support, err := supportNow(cfg)
	if err != nil {
		return result, err
	}
	opt := mutate.Options{
		Support:       support,
		Tests:         tests,
		Workers:       in.integer("workers"),
		MutateAll:     in.set("mutate-all"),
		TimeoutFactor: in.float("timeout-factor"),
		TestCommand:   in.str("test-command"),
		AllTests:      in.set("all-tests"),
		Annotate:      !in.set("no-annotate"),
		Log:           os.Stderr,
		Judge:         judge,
		Renamed:       renamed,
		// An entry whose function moved to another file still excepts
		// its mutant there.
		Exceptions: append(slices.Clone(cfg.Exceptions), moved...),
	}
	suite := listedConfig(in, cfg)
	if suite != nil {
		opt.Listed = &mutate.Listed{Root: project.Root(), Select: suite.Select}
	}
	if strictGo {
		producer := "go test -count=1 -covermode=set -coverprofile=coverage.out; scope=own"
		fingerprint := func(source string) (string, map[string]string, error) {
			inputs, err := mutate.GoCoverageInputs(source, project.Root(), producer, support)
			return producer, inputs, err
		}
		var cached []mutate.GoCoverageCheck
		mutationChecks, err := mutate.Check(sources, judge, tests, support, append(slices.Clone(cfg.Exceptions), moved...))
		if err != nil {
			return result, err
		}
		cached, err = mutate.CheckGoCoverage(sources, judge, fingerprint)
		if err != nil {
			return result, err
		}
		unsupported := false
		for _, check := range cached {
			if check.State == "unsupported" {
				unsupported = true
				reportCoverageProblem(in, check.File, check.Function, check.Line,
					"mutation.coverage-unsupported", "uses unsupported Go coverage scope "+strings.Join(check.Changed, ", "))
			}
		}
		if unsupported {
			return result, nil
		}
		canReuse := !mutate.NeedsMutationCoverage(mutationChecks, in.set("mutate-all"))
		allFresh := canReuse
		for _, check := range cached {
			if check.State != "fresh" {
				allFresh = false
			}
		}
		if allFresh {
			byFunction := map[string]*mutate.GoCoverageEvidence{}
			byFile := map[string][]coverage.GoBlock{}
			for _, check := range cached {
				byFunction[check.File+"\x00"+check.Function] = check.Evidence
				for _, block := range check.Blocks {
					byFile[filepath.Join(project.Root(), filepath.FromSlash(check.File))] = append(byFile[filepath.Join(project.Root(), filepath.FromSlash(check.File))], coverage.GoBlock{
						Span: block.Span, Line: block.Line, Column: block.Column, Weight: block.Weight, Covered: block.Covered,
					})
				}
			}
			opt.CachedCoverage = func(path, function, _ string) *mutate.GoCoverageEvidence {
				return byFunction[project.Rel(path)+"\x00"+function]
			}
			report := coverage.FromGoBlocks(byFile)
			opt.Coverage = func([]string) (*coverage.Report, error) { return report, nil }
		} else {
			report, measuredProducer, currentInputs, listedFiles, err := strictGoCoverage(in, sources, suite, support)
			if err != nil {
				return result, err
			}
			if report == nil {
				return result, fmt.Errorf("strict Go coverage measurement produced no report")
			}
			var missingGo bool
			for _, missing := range report.Missing() {
				missingGo = missingGo || missing.Language == "go"
			}
			if missingGo || !report.Measures("go") {
				for _, p := range unmeasured(report) {
					if p.subject["language"] == "go" {
						in.report(p)
					}
				}
				if !missingGo {
					in.report(fail(kindMissing, "coverage.no-report", "Go coverage measurement did not produce executable coverage",
						"Run the built-in Go coverage command successfully, then retry.").with("language", "go"))
				}
				return result, nil
			}
			opt.StatementCoverage = report
			opt.CoverageProducer = measuredProducer
			opt.CoverageInputs = currentInputs
			opt.Coverage = func([]string) (*coverage.Report, error) { return report, nil }
			if opt.Listed != nil {
				opt.Listed.Files = listedFiles
			}
		}
	} else if !in.set("no-coverage") {
		opt.Coverage = func(sources []string) (*coverage.Report, error) {
			var perTest *coverage.PerTest
			if suite != nil {
				listed, err := listTests(suite)
				if err != nil {
					return nil, err
				}
				perTest = &coverage.PerTest{Root: project.Root(), Tests: listed, Select: suite.Select}
				var ids []string
				opt.Listed.Files = map[string]string{}
				for _, t := range listed {
					ids = append(ids, t.ID)
					opt.Listed.Files[t.ID] = t.File
				}
				perTest.All = suite.All(ids)
			}
			// Coverage comes from the tests that kill mutants, so a line only
			// other tests reach is uncovered rather than a survivor, unless
			// a listed test reaches it.
			report, err := loadCoverage(in, sources, coverage.OwnTests, os.Stderr, perTest)
			if err != nil {
				fmt.Fprintln(os.Stderr, "itos-cc: coverage:", err)
				return nil, nil
			}
			return report, nil
		}
	}

	// Uncovered mutants never run, so without it a function no test
	// executes passes.
	failUncovered := in.set("fail-uncovered")
	results, err := mutate.Run(sources, opt)
	if err != nil {
		return result, err
	}
	if strictGo {
		reportStrictGoCoverage(in, results)
	}
	reported := map[string]bool{}
	for _, r := range results {
		if len(r.FailedSelections) > 0 && !r.BaselineFailed {
			// A mutant the file holds needed listed tests that fail
			// without it: it is not decided, so neither is the file.
			result.Files = append(result.Files, mutateFile{File: r.Rel, Baseline: "failed", Judged: r.Judged, Mutants: []mutateMutant{}})
			if !in.json {
				fmt.Printf("%s: listed tests its mutants run fail without any mutant; snapshot not updated\n", r.Rel)
			}
			reportFailedSelections(in, r.FailedSelections, reported, "mutation run")
			reportStaleExceptions(in, r.Rel, r.StaleExceptions)
			continue
		}
		if r.BaselineFailed {
			result.Files = append(result.Files, mutateFile{File: r.Rel, Baseline: "failed", Judged: r.Judged, Mutants: []mutateMutant{}})
			if !in.json {
				fmt.Printf("%s: baseline tests fail; snapshot not updated\n%s\n", r.Rel, tail(r.BaselineOutput, 20))
			}
			in.report(fail(kindNo, "mutation.baseline-failed", r.Rel+": its tests fail before any mutant, so none was judged",
				"Make its tests pass, then run mutation run again.").with("file", r.Rel))
			reportStaleExceptions(in, r.Rel, r.StaleExceptions)
			continue
		}
		f := mutateFile{File: r.Rel, Ran: r.Ran, Reused: r.Reused, Baseline: "passed", Judged: r.Judged, Mutants: []mutateMutant{}}
		for _, m := range r.Mutants {
			f.Mutants = append(f.Mutants, mutateMutant{Line: m.Line, Column: m.Column, Function: m.Function,
				Original: m.Original, Replacement: m.Replacement, Outcome: m.Outcome, Reused: m.Reused, Scope: m.Scope, Excepted: m.Excepted,
				Coverage: m.Coverage, Tests: m.Tests})
		}
		// Only the functions judged count: the others keep outcomes no
		// change in the range is to blame for.
		judged := map[string]bool{}
		for _, id := range r.Judged {
			judged[id] = true
		}
		var units []mutate.UnitResult
		for _, u := range r.Snapshot.Units {
			if r.Judged == nil || judged[u.Namespace+"#"+u.Name] {
				units = append(units, u)
			}
		}
		for _, u := range units {
			excepted := exceptedIn(u.Mutants)
			f.Killed += u.Killed
			f.Survived += u.Survived - excepted
			f.Excepted += excepted
			f.Uncovered += u.Uncovered
		}
		result.Files = append(result.Files, f)
		if !in.json {
			// "excepted" only where there is one, so a project with no
			// exceptions reads as it always did.
			counts := fmt.Sprintf("%d killed, %d survived, %d uncovered", f.Killed, f.Survived, f.Uncovered)
			if f.Excepted > 0 {
				counts = fmt.Sprintf("%d killed, %d survived, %d excepted, %d uncovered", f.Killed, f.Survived, f.Excepted, f.Uncovered)
			}
			line := fmt.Sprintf("%s: %s (ran %d, reused %d)", r.Rel, counts, f.Ran, f.Reused)
			if r.Judged != nil {
				line += fmt.Sprintf(" (judged %d of %d functions)", len(r.Judged), r.Functions)
			}
			fmt.Println(line)
		}
		for _, u := range units {
			reportFailed(in, r.Rel, u.Namespace+"#"+u.Name, u.Mutants, failUncovered)
		}
		reportStaleExceptions(in, r.Rel, r.StaleExceptions)
	}
	reportElsewhere(in, elsewhere)
	return result, nil
}

// exceptionsElsewhere is each exception of itos-cc.yaml for a file that is
// none of sources and that the command judges all the same, stale, and the
// entries of those whose function moved to one of sources, as they read
// there (mutate.Elsewhere). With no paths, no --changed, and no --since, it
// judges every entry for a file under the working directory; with --since,
// each whose file the range deleted, unless paths narrow it, or renamed to
// one of sources, renamed giving the path each had at the ref; with paths
// or --changed alone, none.
func exceptionsElsewhere(in *invocation, exceptions []config.Exception, sources []string, renamed func(path string) string) ([]mutate.StaleException, []config.Exception, error) {
	if len(exceptions) == 0 {
		return nil, nil, nil
	}
	var judge func(file string) bool
	switch {
	case in.set("since"):
		removed := map[string]bool{}
		for _, path := range sources {
			if old := renamed(path); old != "" {
				removed[project.FromRoot(old)] = true
			}
		}
		if len(in.args) == 0 {
			deleted, err := project.DeletedSince(in.str("since"))
			if err != nil {
				return nil, nil, err
			}
			for _, path := range deleted {
				removed[project.FromRoot(path)] = true
			}
		}
		judge = func(file string) bool { return removed[file] }
	case len(in.args) == 0 && !in.set("changed"):
		wd, err := os.Getwd()
		if err != nil {
			return nil, nil, err
		}
		judge = func(file string) bool {
			rel, err := filepath.Rel(wd, project.AtRoot(file))
			return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
		}
	default:
		return nil, nil, nil
	}
	return mutate.Elsewhere(exceptions, sources, judge)
}

// reportElsewhere reports each exception of exceptionsElsewhere, each under
// the file it names.
func reportElsewhere(in *invocation, stale []mutate.StaleException) {
	for _, e := range stale {
		reportStaleExceptions(in, project.Rel(project.AtRoot(e.File)), []mutate.StaleException{e})
	}
}

// reportFailedSelections reports each selection of listed tests in failed
// that reported does not hold yet as tests.selection-failed, showing its
// output in plain output, and adds it to reported. cmd is the command to
// run again.
func reportFailedSelections(in *invocation, failed []mutate.FailedSelection, reported map[string]bool, cmd string) {
	for _, f := range failed {
		key := strings.Join(slices.Sorted(slices.Values(f.IDs)), " ")
		if reported[key] {
			continue
		}
		reported[key] = true
		ids := strings.Join(f.IDs, " ")
		if !in.json {
			fmt.Printf("listed tests %s fail without any mutant\n%s\n", ids, tail(f.Output, 20))
		}
		in.report(fail(kindNo, "tests.selection-failed",
			fmt.Sprintf("the listed tests %s fail without any mutant, so no mutant that would run them was judged: %s", ids, f.Command),
			fmt.Sprintf("Make them pass when they run alone, as that command runs them, then run %s again.", cmd)).
			with("ids", f.IDs).with("command", f.Command).with("exit_code", f.ExitCode))
	}
}

// exceptedIn counts the survivors of mutants that itos-cc.yaml excepts.
func exceptedIn(mutants []mutate.Mutant) int {
	n := 0
	for _, m := range mutants {
		if m.Excepted != "" {
			n++
		}
	}
	return n
}

// loadExceptions is the survivors itos-cc.yaml excepts. A file that cannot
// be read is a config error.
func loadExceptions() ([]config.Exception, error) {
	c, err := loadConfig()
	if err != nil {
		return nil, err
	}
	return c.Exceptions, nil
}

// loadConfig reads itos-cc.yaml. A file that cannot be read is a config
// error.
func loadConfig() (*config.Config, error) {
	c, err := config.Load()
	var invalid *config.InvalidError
	if errors.As(err, &invalid) {
		return nil, fail(kindUsage, "config.invalid", invalid.Error(),
			"Fix it: each entry under mutation.exceptions needs file, function, hash, line_in_function, column, original, replacement, and reason, and 'itos-cc mutation except' writes one; mutation.tests needs list, run with {pattern}, ids_pattern with {ids}, and join with each, holding {id}, and sep.").
			with("file", config.File)
	}
	return c, err
}

// supportNow is the hashes of the support files of the tests cfg lists,
// now: what a kill by listed tests rests on, with the files of its tests.
func supportNow(cfg *config.Config) (map[string]string, error) {
	if cfg.Tests == nil {
		return nil, nil
	}
	return mutate.SupportHashes(project.Root(), cfg.Tests.Support)
}

func flagConflict(flag, message string) error {
	return fail(kindUsage, "flags.conflict", message, "Drop the incompatible flag and run again.").with("flag", flag)
}

func flagPresent(in *invocation, flag string) bool {
	switch flag {
	case "--coverage-report":
		return len(in.strs("coverage-report")) > 0
	case "--coverage-command":
		return in.set("coverage-command")
	case "--use-existing-coverage":
		return in.set("use-existing-coverage")
	}
	return false
}

func hasGoSource(sources []string) bool {
	for _, source := range sources {
		if spec := lang.Detect(source); spec != nil && spec.Name == "go" {
			return true
		}
	}
	return false
}

func strictGoCoverage(in *invocation, sources []string, suite *config.Tests, support map[string]string) (*coverage.Report, string, func(string) (string, map[string]string, error), map[string]string, error) {
	perTest := (*coverage.PerTest)(nil)
	listedFiles := map[string]string{}
	if suite != nil {
		listed, err := listTests(suite)
		if err != nil {
			return nil, "", nil, nil, err
		}
		perTest = &coverage.PerTest{Root: project.Root(), Tests: listed, Select: suite.Select}
		var ids []string
		for _, test := range listed {
			ids = append(ids, test.ID)
			listedFiles[test.ID] = test.File
		}
		perTest.All = suite.All(ids)
	}
	scope := coverage.OwnTests
	producer := "go test -count=1 -covermode=set -coverprofile=coverage.out; scope=own"
	if in.set("all-tests") {
		scope = coverage.AllTests
		producer = "go test -count=1 -covermode=set -coverpkg=./... -coverprofile=coverage.out ./...; scope=all-tests"
	}
	report, err := loadCoverage(in, sources, scope, os.Stderr, perTest)
	if err != nil {
		return nil, "", nil, nil, err
	}
	fingerprint := func(source string) (string, map[string]string, error) {
		inputs, err := mutate.GoCoverageInputs(source, project.Root(), producer, support)
		return producer, inputs, err
	}
	return report, producer, fingerprint, listedFiles, nil
}

func reportStrictGoCoverage(in *invocation, results []mutate.FileResult) {
	for _, result := range results {
		if result.Snapshot.Language != "go" {
			continue
		}
		if result.BaselineFailed || len(result.FailedSelections) > 0 {
			continue
		}
		judged := map[string]bool{}
		for _, id := range result.Judged {
			judged[id] = true
		}
		for _, unit := range result.Snapshot.Units {
			id := unit.Namespace + "#" + unit.Name
			if result.Judged != nil && !judged[id] {
				continue
			}
			if unit.Coverage == nil || !unit.Coverage.Complete {
				reportCoverageProblem(in, result.Rel, id, unit.StartLine, "mutation.coverage-missing", "has no complete measured Go coverage inventory")
				continue
			}
			for _, block := range unit.Coverage.Blocks {
				if !block.Covered {
					reportCoverageProblem(in, result.Rel, id, block.Line, "mutation.uncovered-statement", "has an uncovered executable Go coverage block")
				}
			}
		}
	}
}

func reportCoverageProblem(in *invocation, file, function string, line int, rule, message string) {
	if !in.json {
		fmt.Printf("  %s %s:%d in %s\n", strings.TrimPrefix(rule, "mutation."), file, line, function)
	}
	p := fail(kindNo, rule, fmt.Sprintf("%s:%d in %s %s", file, line, function, message),
		"Add a test that executes the uncovered Go code, then run mutation run again.").
		with("file", file).with("function", function).with("line", line)
	p.shown = true
	in.report(p)
}

// listedConfig is the tests itos-cc.yaml lists, when the mutants of this run
// are to run them: coverage is measured by running the per-language
// commands, and the mutants run their own tests, not --all-tests or a
// --test-command. Otherwise nil.
func listedConfig(in *invocation, cfg *config.Config) *config.Tests {
	if cfg.Tests == nil || in.set("no-coverage") || in.set("all-tests") || in.str("test-command") != "" ||
		in.set("use-existing-coverage") || in.str("coverage-command") != "" || len(in.strs("coverage-report")) > 0 {
		return nil
	}
	return cfg.Tests
}

// listTests runs the list command of tests at the project root, through the
// platform shell, and reads the tests it prints: one a line, its ID, then,
// after a tab, optionally its file. A command that fails is
// tests.list-failed.
func listTests(tests *config.Tests) ([]coverage.Test, error) {
	root := project.Root()
	fmt.Fprintf(os.Stderr, "itos-cc: tests %s$ %s\n", project.Rel(root), tests.List)
	out, err := shellOutput(tests.List, root, os.Stderr)
	if err != nil {
		code := -1
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
		}
		return nil, fail(kindNo, "tests.list-failed",
			fmt.Sprintf("the list command of mutation.tests in %s failed (%v), so no mutant was judged: %s", config.File, err, tests.List),
			"Make the command print the tests, one a line, and exit 0, then run again.").
			with("command", tests.List).with("exit_code", code)
	}
	var listed []coverage.Test
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		id, file, _ := strings.Cut(strings.TrimRight(line, "\r"), "\t")
		id, file = strings.TrimSpace(id), filepath.ToSlash(strings.TrimSpace(file))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		listed = append(listed, coverage.Test{ID: id, File: file})
	}
	return listed, nil
}

// reportFailed reports each mutant of function that fails: a survivor that
// itos-cc.yaml does not except, and with failUncovered an uncovered mutant.
func reportFailed(in *invocation, rel, function string, mutants []mutate.Mutant, failUncovered bool) {
	for _, m := range mutants {
		switch {
		case m.Outcome == mutate.Survived && m.Excepted == "":
			reportMutant(in, rel, function, m, "mutation.survived", "survived", "survived",
				fmt.Sprintf("Add a test that fails with this change. If no test can, as the change changes no behaviour, except it with 'itos-cc mutation except %s:%d:%d --reason …'.",
					rel, m.Line, m.Column))
		case m.Outcome == mutate.Uncovered && failUncovered:
			reportMutant(in, rel, function, m, "mutation.uncovered", "uncovered", "is uncovered: no test executes its line",
				"Add a test that executes this line and fails with this change.")
		}
	}
}

// reportMutant lists mutant m of function in file rel on stdout, as "<what>
// <file>:<line>:<column> <original> → <replacement> in <function>", and
// reports it as a problem of rule whose message ends with verdict.
func reportMutant(in *invocation, rel, function string, m mutate.Mutant, rule, what, verdict, fix string) {
	if !in.json {
		fmt.Printf("  %s %s:%d:%d %s → %s in %s\n", what, rel, m.Line, m.Column,
			quote(m.Original), quote(m.Replacement), function)
	}
	p := fail(kindNo, rule,
		fmt.Sprintf("%s:%d:%d: %s → %s in %s %s", rel, m.Line, m.Column, quote(m.Original), quote(m.Replacement), function, verdict),
		fix).
		with("file", rel).with("line", m.Line).with("column", m.Column).with("function", function).
		with("original", m.Original).with("replacement", m.Replacement)
	p.shown = true
	in.report(p)
}

// changedSince is the functions the commits since --since's ref changed, by
// file. --changed is refused beside it: it judges whole files of the working
// tree, --since functions of commits, and together they would judge neither.
func changedSince(in *invocation) (map[string]project.ChangedFunctions, map[string]string, error) {
	if in.set("changed") {
		return nil, nil, fail(kindUsage, "flags.conflict", "--since and --changed cannot be combined: --since judges the functions of commits, --changed whole files of the working tree",
			"Drop --changed; commit the work to judge it with --since.").with("flag", "--changed")
	}
	ref := in.str("since")
	changed, renamed, err := project.ChangedSince(ref, mutate.UnitHash)
	var noGit *project.NoGitError
	switch {
	case errors.Is(err, project.ErrBadRef):
		return nil, nil, fail(kindUsage, "since.bad-ref", fmt.Sprintf("--since %s: not a commit in this repository", ref),
			"Name a branch, tag, or commit, such as origin/main; fetch a remote one first.").with("ref", ref)
	case errors.As(err, &noGit):
		return nil, nil, fail(kindMissing, "since.no-git", "--since needs a git repository: "+noGit.Reason,
			"Run it inside a git repository, or name the paths instead.")
	}
	return changed, renamed, err
}

type mutationListResult struct {
	Sites []mutateSite `json:"sites"`
}

// runMutationList lists the mutation sites of the chosen sources.
func runMutationList(in *invocation) (any, error) {
	result := mutationListResult{Sites: []mutateSite{}}
	files, err := files(in)
	if err != nil {
		return result, err
	}
	if len(files.Sources) == 0 {
		fmt.Fprintln(os.Stderr, "itos-cc: no source files to list")
		return result, nil
	}
	for _, path := range files.Sources {
		f, err := lang.ParseFile(path)
		if err != nil {
			return result, err
		}
		for _, s := range slices.SortedStableFunc(slices.Values(mutate.Sites(f)), mutate.LineOrder) {
			u := f.Units[s.Unit]
			site := mutateSite{File: project.Rel(path), Line: s.Line, Column: s.Column,
				Function: u.Namespace + "#" + u.Name, Original: s.Original, Replacement: s.Replacement}
			result.Sites = append(result.Sites, site)
			if !in.json {
				fmt.Printf("%s:%d:%d %s → %s in %s\n", site.File, site.Line, site.Column,
					quote(site.Original), quote(site.Replacement), site.Function)
			}
		}
		f.Close()
	}
	return result, nil
}

func quote(s string) string {
	if s == "" {
		return "(deleted)"
	}
	return "`" + s + "`"
}

func tail(s string, lines int) string {
	parts := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, "\n")
}

// reportStaleExceptions lists each exception of file rel that no longer
// holds on stdout, as "exception-stale <file>:<line>:<column> <original> →
// <replacement> in <function> (<why>)", the place left out when its
// function or its file is gone, and "(moved to <file>)" when its function
// is in another file now, and reports it as mutation.exception-stale.
func reportStaleExceptions(in *invocation, rel string, stale []mutate.StaleException) {
	for _, e := range stale {
		at := fmt.Sprintf("%s:%d:%d", rel, e.Line, e.Column)
		if e.Line == 0 {
			at = rel
		}
		change := fmt.Sprintf("%s → %s in %s", quote(e.Original), quote(e.Replacement), e.Function)
		var newFile string
		if e.Why == mutate.ExceptionMoved {
			newFile = project.Rel(project.AtRoot(e.NewFile))
		}
		if !in.json {
			why := e.Why
			if newFile != "" {
				why += " to " + newFile
			}
			fmt.Printf("  exception-stale %s %s (%s)\n", at, change, why)
		}
		var message, fix string
		switch e.Why {
		case mutate.ExceptionKilled:
			message = fmt.Sprintf("%s: %s is excepted in %s, but the tests now kill it", at, change, config.File)
			fix = "Remove its entry from " + config.File + ": the tests show that the mutant changes behaviour."
		case mutate.ExceptionChanged:
			message = fmt.Sprintf("%s: the entry in %s for %s was written for another version of the function", at, config.File, change)
			fix = fmt.Sprintf("If the mutant still survives and changes no behaviour, except it again with 'itos-cc mutation except %s --reason …' after mutation run; otherwise remove its entry from %s.", at, config.File)
		case mutate.ExceptionGone:
			message = fmt.Sprintf("%s: the entry in %s for %s names a site the function no longer has", at, config.File, change)
			if e.Line == 0 {
				message = fmt.Sprintf("%s: the entry in %s for %s names a function the file no longer has", at, config.File, change)
				if _, err := os.Stat(project.AtRoot(e.File)); err != nil {
					message = fmt.Sprintf("%s: the entry in %s for %s names a file that is gone", at, config.File, change)
				}
			}
			fix = "Remove its entry from " + config.File + "."
		case mutate.ExceptionMoved:
			message = fmt.Sprintf("%s: the entry in %s for %s names a file that is gone; its function is in %s now, where the entry still excepts the mutant",
				at, config.File, change, newFile)
			fix = fmt.Sprintf("Change the entry's file in %s from %s to %s, where its function is now.", config.File, e.File, e.NewFile)
		}
		p := fail(kindNo, "mutation.exception-stale", message, fix).
			with("file", rel).with("function", e.Function).with("column", e.Column).
			with("original", e.Original).with("replacement", e.Replacement).with("why", e.Why)
		if e.Line != 0 {
			p.with("line", e.Line)
		}
		if newFile != "" {
			p.with("new_file", newFile)
		}
		p.shown = true
		in.report(p)
	}
}
