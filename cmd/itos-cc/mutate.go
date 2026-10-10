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
used as installed, from the live project, and nothing is installed or
downloaded: TypeScript's node_modules and Python's .venv or venv (else the
virtualenv VIRTUAL_ENV names) at its build roots, and the Go module, Gradle
and Maven caches, with Go commands offline (GOPROXY=off), Gradle --offline
and Maven -o. A dependency missing offline fails the run, as
count.preparation-failed naming the stage. The sites of the selection,
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
on Linux and macOS, where every command it starts runs in a process group
it owns (a command that detaches from its group or session is not
followed); Windows is #29. It refuses --changed, --no-coverage, existing
or raw coverage, --test-command and --mutate-all, and --seed needs it. With --all-tests the own stage runs the whole suite, and a
survivor still runs the listed tests that reach it. SIGINT or SIGTERM
interrupts a counted run: no command starts after it, the judgment then
running is cancelled with no outcome, every process the run started is
stopped and joined within one shared five-second deadline before its
private copies are removed, a second signal included, and the partial
report, completion "interrupted", keeps the judgments completed before it.

--fail-fast stops a complete run at the first actionable failure it
observes, in the order judgments finish, for a quick fix-and-retry loop:
an unexcepted survivor, once the listed tests that reach it failed to kill
it too; with --fail-uncovered, an uncovered mutant or a strict Go coverage
finding; an exception that no longer holds; a file whose tests fail before
any mutant; or a selection of listed tests that fails without any mutant.
It reports that failure under the rule a run without it reports, and exits
as that rule says. Killed, timed-out, validly excepted and listed-killed
mutants never stop it. A failure planning or coverage already shows, such
as a stale exception or an uncovered mutant with --fail-uncovered, stops it
before any baseline or mutant runs; listing and coverage keep their scope,
so it bounds no total time. At the stop no further command starts, each
judgment still running is cancelled, with no outcome (never killed or timed
out), and every process the run started is stopped and joined within one
shared five-second deadline from the stop, before its worker copies are
removed. A file whose every selected mutant was decided is written as
without --fail-fast. A file the stop cut short, or whose mutant a failing
selection of listed tests blocked, gets no summary comment, its source
left as it was, but its snapshot keeps every judgment the run completed or
reused, with the scope and the freshness evidence that decided it; a
cancelled, blocked or unattempted mutant gets no outcome, and keeps what
the snapshot recorded for it that still holds, if anything, so a stopped
--mutate-all rerun never drops a valid result. mutation check still fails
a function with a mutant no valid result records, and a later run reuses
what was kept while its inputs hold and runs only the rest. A file whose
baseline failed, or which the run judged nothing anew in, is left as it
was. With --json, "stop" says whether the run stopped early and at which
rule and subject, "work" counts the selected mutants completed (run,
reused or measured uncovered), cancelled, unattempted and blocked,
disjointly, and each file has its "state" and "work", the run's own work
on it, its "cache", whether its snapshot as the run leaves it holds a
valid result for every mutant of its functions judged, its "baseline" as
it ran ("not-run" when it never did), and every mutant of its functions
judged with its "state", an outcome only when completed. --fail-fast runs on Linux and macOS, where every command it
starts runs in a process group it owns (a command that detaches from its
group or session is not followed); on Windows it is fail-fast.platform,
before any command runs (#29). It refuses --count, whose runs stay
aggregate. Without it, scheduling, output, snapshots and comments are as
they always were.

--fail-uncovered makes each uncovered mutant a failure, listed like a
survivor. For Go it also requires fresh measured executable coverage for
every judged function, including functions with no mutation sites, and fails
each uncovered positive-weight coverage block. Strict Go runs reject
--no-coverage, --coverage-report, --use-existing-coverage, and
--coverage-command. A matching independent cache can avoid remeasurement;
otherwise built-in or listed coverage must measure successfully. Empty or
comment-only function bodies have no executable coverage obligation. For
TypeScript, Python and Kotlin it fails closed as Go does: a build root with
mutants to judge whose coverage tool is missing, or whose coverage command
fails or writes no report, is coverage.tool-missing or
coverage.measured-nothing, naming its language and dir, and the run stops
before any mutant runs. Without --fail-uncovered such a file runs every
mutant, as it always did, and so it does with --coverage-report,
--use-existing-coverage or --coverage-command. With --since, only the judged
functions' uncovered mutants and executable blocks count.

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
		sw("fail-fast", "stop at the first actionable failure, cancelling the work still running; Linux and macOS"),
		opt("count", intFlag, "N", "", "judge at most N committed mutation sites freshly, drawn across the whole selection; Linux and macOS"),
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
   "judged": ["namespace#name"], and with --fail-fast "state":
   "completed"|"stopped"|"unattempted"|"blocked", "work",
   "cache": "complete"|"incomplete", "baseline" also "not-run", every
   mutant "state":
   "completed"|"cancelled"|"unattempted"|"blocked", and "outcome" and
   "scope" for completed ones only}]; with --fail-fast, "stop":
   {"stopped", "rule", "subject"}, the problem's rule and subject keys, and
   "work": {"completed", "cancelled", "unattempted", "blocked"}; with
   --count, instead of "files":
   "sampling": {"budget", "eligible", "selected", "executed", "omitted",
   "seed", "algorithm", "commit", "since", "since_base",
   "assurance": "sampled"|"not-applicable",
   "completion": "completed"|"stopped"|"interrupted"|"not-applicable",
   "stop", "bounds"},
   "selected": [{"identity", "file", "line", "column", "function",
   "original", "replacement",
   "state": "judged"|"uncovered"|"blocked"|"failed"|"cancelled"|"unattempted",
   "outcome" (judged and uncovered sites only), "scope", with scope
   "listed" "tests": ["<test ID>"], for an excepted survivor "excepted":
   "its reason", "reason", and the stages its trial ran, "stages":
   [{"name": "own"|"listed-baseline"|"listed",
   "state": "complete"|"failed"|"aborted", "outcome", "tests", "error"}]}],
   "subjects": {"judged": [{"file", "function"}], "omitted": [...]},
   "stages": [{"name", "state": "complete"|"failed"|"skipped"|"aborted", "error"}]`,
	rules: []string{
		"mutation.survived         a mutant survived: file, line, column, function, original, replacement",
		"mutation.uncovered        with --fail-uncovered, no test executes a mutant: file, line, column, function, original, replacement",
		"mutation.uncovered-statement with --fail-uncovered, a measured executable Go coverage block is uncovered: file, function, line",
		"mutation.coverage-missing  with --fail-uncovered, a Go function lacks complete measured coverage evidence: file, function, line",
		"mutation.coverage-unsupported with --fail-uncovered, strict coverage reaches beyond the inventoried Go module: file, function, line",
		"coverage.tool-missing     with --fail-uncovered, a language's coverage tool is missing where it has mutants to judge: language, dir",
		"coverage.measured-nothing with --fail-uncovered, a language's coverage command failed or wrote no report where it has mutants to judge: language, dir",
		"mutation.exception-stale  an exception in itos-cc.yaml no longer holds: file, function, line (none when the function or its file is gone), column, original, replacement, why: killed|changed|gone|moved, and with moved new_file",
		"mutation.baseline-failed  the tests fail before any mutant: file",
		"config.invalid            itos-cc.yaml cannot be read: file",
		"tests.list-failed         the list command of mutation.tests failed: command, exit_code",
		"tests.selection-failed    a selection of listed tests fails without any mutant: ids, command, exit_code",
		"since.bad-ref             --since names no commit: ref",
		"since.no-git              --since outside a git repository",
		"flags.conflict            --since with --changed: flag",
		"flags.conflict            --fail-uncovered with --no-coverage, or strict Go coverage with raw coverage flags: flag",
		"flags.conflict            --seed without --count, or --count with a flag it refuses, --fail-fast included: flag",
		"count.platform            --count on a platform other than Linux and macOS, that is Windows (#29): platform",
		"fail-fast.platform        --fail-fast on a platform other than Linux and macOS, that is Windows (#29), before any command runs: platform",
		"count.no-git              --count outside a Git repository, or before its first commit",
		"count.unsupported-scope   --count over committed content it cannot judge, such as a symlink or a submodule",
		"count.preparation-failed  with --count, a runtime Git, tool, listing, coverage, conversion or baseline step failed before any mutant: stage",
		"count.trial-failed        with --count, a selected mutant's trial could not run: file, line, column, function, original, replacement, identity",
		"count.interrupted         with --count, SIGINT or SIGTERM interrupted the run, whose report is partial",
	},
	exits: []exitDoc{
		{0, "every mutant that ran was killed"},
		{1, "a mutant survived, a mutant is uncovered with --fail-uncovered or coverage measured nothing where it has mutants to judge, an exception is stale, a file's tests fail before any mutant, the list command of mutation.tests failed, a selection of listed tests fails without any mutant, or with --count a preparation step failed or a selected mutant is not judged"},
		{2, "a usage or config error: a bad flag or path, a --since ref that is no commit, --since with --changed, a --count below 1, --seed without --count, committed content --count cannot judge, or an itos-cc.yaml that cannot be read"},
		{3, "--changed or --since outside a git repository; with --fail-uncovered, a coverage tool missing where it has mutants to judge; --count outside a Git repository with a commit, on Windows, or with a required tool missing; --fail-fast on Windows"},
		{75, "with --count, SIGINT or SIGTERM interrupted the run; its partial report is printed"},
	},
	examples: []string{
		"itos-cc mutation run --changed",
		"itos-cc mutation run --since origin/main --fail-uncovered  # a branch's own commits, as a gate",
		"itos-cc mutation run --all-tests --json                    # nightly",
		"itos-cc mutation run --count 20 --since origin/main        # a bounded fresh check of committed work (Linux and macOS)",
		"itos-cc mutation run --changed --fail-fast                 # stop at the first survivor while fixing (Linux and macOS)",
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
	// failed, except with --fail-fast, which lists them with their state.
	Mutants []mutateMutant `json:"mutants"`
	// State and Work are there with --fail-fast only: whether the run
	// completed the file, stopped inside it, never started its work, or
	// found it blocked by a failing baseline or selection, and how many of
	// its mutants ended in each state.
	State string      `json:"state,omitempty"`
	Work  *mutateWork `json:"work,omitempty"`
	// Cache is there with --fail-fast only: "complete" when the file's
	// snapshot, as the run leaves it, holds a valid result for every
	// mutant of its functions judged, else "incomplete". A stopped run's
	// own work, State, can be incomplete while the cache is complete.
	Cache string `json:"cache,omitempty"`
}

// mutateWork counts selected mutants by their fail-fast state, disjointly:
// completed (run, reused or measured uncovered, with an outcome),
// cancelled, unattempted and blocked (with none).
type mutateWork struct {
	Completed   int `json:"completed"`
	Cancelled   int `json:"cancelled"`
	Unattempted int `json:"unattempted"`
	Blocked     int `json:"blocked"`
}

func (w *mutateWork) add(o *mutateWork) {
	w.Completed += o.Completed
	w.Cancelled += o.Cancelled
	w.Unattempted += o.Unattempted
	w.Blocked += o.Blocked
}

// mutateStop is, with --fail-fast, whether the run stopped early, and at
// which failure: the rule and subject of its problem.
type mutateStop struct {
	Stopped bool           `json:"stopped"`
	Rule    string         `json:"rule,omitempty"`
	Subject map[string]any `json:"subject,omitempty"`
}

// mutateMutant is a site, less its file, and how it was decided: a
// timed-out mutant is "timeout" here and counts in "killed".
type mutateMutant struct {
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Function    string `json:"function"`
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
	// Outcome and Scope are there for every mutant but one a --fail-fast
	// stop left undecided.
	Outcome string `json:"outcome,omitempty"`
	Reused  bool   `json:"reused"`
	// Scope is the scope of the tests that decided the outcome: "own",
	// "all-tests", or the --test-command line.
	Scope string `json:"scope,omitempty"`
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
	// State is there with --fail-fast only: "completed", "cancelled",
	// "unattempted" or "blocked".
	State string `json:"state,omitempty"`
}

type mutateResult struct {
	Files []mutateFile `json:"files"`
	// Stop and Work are there with --fail-fast only.
	Stop *mutateStop `json:"stop,omitempty"`
	Work *mutateWork `json:"work,omitempty"`
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
	failFast := in.set("fail-fast")
	if failFast {
		if countedPlatform != "linux" && countedPlatform != "darwin" {
			return result, fail(kindMissing, "fail-fast.platform",
				fmt.Sprintf("mutation run --fail-fast supports Linux and macOS, where every command's process tree is owned and a stop can end it; this is %s", countedPlatform),
				"Run it on Linux or macOS, or run mutation run without --fail-fast; native Windows support is #29.").with("platform", countedPlatform)
		}
		result.Stop, result.Work = &mutateStop{}, &mutateWork{}
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
			// Another language that measured nothing fails closed as Go
			// does, and stops the run with it.
			others := unmeasuredToJudge(report, sources, mutationChecks, in.set("mutate-all"))
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
				for _, p := range others {
					in.report(p)
				}
				return result, nil
			}
			if len(others) > 0 {
				for _, p := range others {
					in.report(p)
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
		// measure measures sources; a strict run fails on a coverage error
		// rather than going without coverage.
		measure := func(sources []string, strict bool) (*coverage.Report, error) {
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
				if strict {
					return nil, err
				}
				fmt.Fprintln(os.Stderr, "itos-cc: coverage:", err)
				return nil, nil
			}
			return report, nil
		}
		opt.Coverage = func(sources []string) (*coverage.Report, error) { return measure(sources, false) }
		if in.set("fail-uncovered") && !rawCoverage(in) {
			// Strict: a language the per-language commands measured
			// nothing of fails closed, as Go does, instead of running
			// every mutant, and stops the run before any mutant runs.
			checks, err := mutate.Check(sources, judge, tests, support, append(slices.Clone(cfg.Exceptions), moved...))
			if err != nil {
				return result, err
			}
			if mutate.NeedsMutationCoverage(checks, in.set("mutate-all")) {
				report, err := measure(sources, true)
				if err != nil {
					return result, err
				}
				if others := unmeasuredToJudge(report, sources, checks, in.set("mutate-all")); len(others) > 0 {
					for _, p := range others {
						in.report(p)
					}
					return result, nil
				}
				opt.Coverage = func([]string) (*coverage.Report, error) { return report, nil }
			}
		}
	}

	// Uncovered mutants never run, so without it a function no test
	// executes passes.
	failUncovered := in.set("fail-uncovered")
	var results []mutate.FileResult
	var stop *mutate.Stop
	if failFast {
		ff := mutate.FailFast{FailUncovered: failUncovered}
		if len(elsewhere) > 0 {
			// Known before anything runs: an entry of a file the run does
			// not select that no longer holds.
			e := elsewhere[0]
			ff.Known = &mutate.Stop{Rule: "mutation.exception-stale", File: project.Rel(project.AtRoot(e.File)), Exception: &e}
		}
		results, stop, err = mutate.RunFailFast(sources, opt, ff)
	} else {
		results, err = mutate.Run(sources, opt)
	}
	if err != nil {
		return result, err
	}
	if strictGo {
		reportStrictGoCoverage(in, results)
	}
	reported := map[string]bool{}
	for _, r := range results {
		// With --fail-fast every file has its state, its work and the
		// baseline it actually ran, and a file not written still lists its
		// mutants, each with its state.
		baseline := func(aggregate string) string { return aggregate }
		var work *mutateWork
		state, cache := "", ""
		if failFast {
			work = workOf(r.Mutants)
			result.Work.add(work)
			state, cache = failFastState(r, work), "incomplete"
			if r.Cached {
				cache = "complete"
			}
			baseline = func(string) string {
				if r.Baseline == "" {
					return "not-run"
				}
				return r.Baseline
			}
		}
		if failFast && (r.BaselineFailed || len(r.FailedSelections) > 0) {
			f := mutateFile{File: r.Rel, Baseline: baseline(""), Judged: r.Judged, Mutants: jsonMutants(r.Mutants), State: state, Work: work, Cache: cache}
			countMutants(&f, r.Mutants)
			result.Files = append(result.Files, f)
			if !in.json {
				fmt.Printf("%s: blocked, %s: its tests, or the listed tests its mutants run, fail without any mutant (%d completed, %d cancelled, %d unattempted, %d blocked); cache %s\n",
					r.Rel, snapshotKept(r), work.Completed, work.Cancelled, work.Unattempted, work.Blocked, cache)
				if r.BaselineFailed {
					fmt.Println(tail(r.BaselineOutput, 20))
				}
			}
			if r.BaselineFailed {
				in.report(fail(kindNo, "mutation.baseline-failed", r.Rel+": its tests fail before any mutant, so none was judged",
					"Make its tests pass, then run mutation run again.").with("file", r.Rel))
			}
			reportFailedSelections(in, r.FailedSelections, reported, "mutation run")
			for _, m := range r.Mutants {
				reportFailed(in, r.Rel, m.Function, []mutate.Mutant{{Line: m.Line, Column: m.Column, Original: m.Original,
					Replacement: m.Replacement, Outcome: m.Outcome, Excepted: m.Excepted}}, failUncovered)
			}
			reportStaleExceptions(in, r.Rel, r.StaleExceptions)
			continue
		}
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
		f := mutateFile{File: r.Rel, Ran: r.Ran, Reused: r.Reused, Baseline: baseline("passed"), Judged: r.Judged, Mutants: jsonMutants(r.Mutants),
			State: state, Work: work, Cache: cache}
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
		if r.Incomplete {
			// Only this run's judgments count and fail: the snapshot also
			// keeps results of the snapshot before for sites the stop
			// left undecided.
			units = nil
			countMutants(&f, r.Mutants)
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
			if r.Incomplete {
				line = fmt.Sprintf("%s: stopped early, %s: %s (ran %d, reused %d); %d cancelled, %d unattempted; cache %s",
					r.Rel, snapshotKept(r), counts, f.Ran, f.Reused, work.Cancelled, work.Unattempted, cache)
			}
			fmt.Println(line)
		}
		for _, u := range units {
			reportFailed(in, r.Rel, u.Namespace+"#"+u.Name, u.Mutants, failUncovered)
		}
		if r.Incomplete {
			for _, m := range r.Mutants {
				reportFailed(in, r.Rel, m.Function, []mutate.Mutant{{Line: m.Line, Column: m.Column, Original: m.Original,
					Replacement: m.Replacement, Outcome: m.Outcome, Excepted: m.Excepted}}, failUncovered)
			}
		}
		reportStaleExceptions(in, r.Rel, r.StaleExceptions)
	}
	reportElsewhere(in, elsewhere)
	if stop != nil {
		result.Stop = &mutateStop{Stopped: true, Rule: stop.Rule, Subject: stopSubject(stop)}
		if !in.json {
			w := result.Work
			fmt.Printf("stopped early at the first actionable failure, %s %s: %d completed, %d cancelled, %d unattempted, %d blocked; a file it cut short keeps its source as it was, and its snapshot only what was judged\n",
				stop.Rule, describeStop(stop), w.Completed, w.Cancelled, w.Unattempted, w.Blocked)
		}
	}
	return result, nil
}

// snapshotKept says, in plain output, what became of the snapshot of a file
// a fail-fast stop left incomplete.
func snapshotKept(r mutate.FileResult) string {
	if r.Preserved {
		return "snapshot keeps what was judged, source not annotated"
	}
	return "snapshot not updated"
}

// jsonMutants is the --json form of mutants.
func jsonMutants(mutants []mutate.MutantResult) []mutateMutant {
	out := []mutateMutant{}
	for _, m := range mutants {
		out = append(out, mutateMutant{Line: m.Line, Column: m.Column, Function: m.Function,
			Original: m.Original, Replacement: m.Replacement, Outcome: m.Outcome, Reused: m.Reused, Scope: m.Scope, Excepted: m.Excepted,
			Coverage: m.Coverage, Tests: m.Tests, State: m.State})
	}
	return out
}

// countMutants counts the outcomes of mutants in f, as a file's units count
// them: a timed-out mutant is killed, and an excepted survivor excepted.
func countMutants(f *mutateFile, mutants []mutate.MutantResult) {
	for _, m := range mutants {
		switch {
		case m.Outcome == mutate.Killed || m.Outcome == mutate.Timeout:
			f.Killed++
		case m.Outcome == mutate.Survived && m.Excepted != "":
			f.Excepted++
		case m.Outcome == mutate.Survived:
			f.Survived++
		case m.Outcome == mutate.Uncovered:
			f.Uncovered++
		}
	}
}

// workOf counts mutants by their fail-fast state.
func workOf(mutants []mutate.MutantResult) *mutateWork {
	w := &mutateWork{}
	for _, m := range mutants {
		switch m.State {
		case mutate.StateCompleted:
			w.Completed++
		case mutate.StateCancelled:
			w.Cancelled++
		case mutate.StateUnattempted:
			w.Unattempted++
		case mutate.StateBlocked:
			w.Blocked++
		}
	}
	return w
}

// failFastState is a file's state in a --fail-fast run: "blocked" when its
// baseline or a selection of listed tests it needed failed, "completed"
// when every selected mutant has its outcome, else "stopped" when the stop
// came once its work began, and "unattempted" when it came before.
func failFastState(r mutate.FileResult, w *mutateWork) string {
	switch {
	case r.BaselineFailed || len(r.FailedSelections) > 0:
		return "blocked"
	case w.Cancelled == 0 && w.Unattempted == 0:
		return "completed"
	case w.Cancelled > 0 || r.Ran > 0:
		return "stopped"
	}
	return "unattempted"
}

// stopSubject is the subject of the problem the stop's failure reports.
func stopSubject(s *mutate.Stop) map[string]any {
	switch {
	case s.Site != nil:
		return map[string]any{"file": s.File, "line": s.Site.Line, "column": s.Site.Column, "function": s.Function,
			"original": s.Site.Original, "replacement": s.Site.Replacement}
	case s.Exception != nil:
		return staleSubject(s.File, *s.Exception)
	case s.Selection != nil:
		return map[string]any{"ids": s.Selection.IDs, "command": s.Selection.Command, "exit_code": s.Selection.ExitCode}
	case s.Function != "":
		return map[string]any{"file": s.File, "function": s.Function, "line": s.Line}
	}
	return map[string]any{"file": s.File}
}

// describeStop names the stop's failure in plain output.
func describeStop(s *mutate.Stop) string {
	switch {
	case s.Site != nil:
		return fmt.Sprintf("%s:%d:%d %s → %s in %s", s.File, s.Site.Line, s.Site.Column, quote(s.Site.Original), quote(s.Site.Replacement), s.Function)
	case s.Exception != nil && s.Exception.Line != 0:
		return fmt.Sprintf("%s:%d:%d in %s (%s)", s.File, s.Exception.Line, s.Exception.Column, s.Exception.Function, s.Exception.Why)
	case s.Exception != nil:
		return fmt.Sprintf("%s in %s (%s)", s.File, s.Exception.Function, s.Exception.Why)
	case s.Selection != nil:
		return "of the listed tests " + strings.Join(s.Selection.IDs, " ")
	case s.Function != "":
		return fmt.Sprintf("%s:%d in %s", s.File, s.Line, s.Function)
	}
	return "in " + s.File
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

// rawCoverage says whether coverage comes from reports the run is given or
// told to read, not from the per-language commands: strict non-Go coverage
// keeps their behaviour.
func rawCoverage(in *invocation) bool {
	return in.set("use-existing-coverage") || in.str("coverage-command") != "" || len(in.strs("coverage-report")) > 0
}

// unmeasuredToJudge is, under --fail-uncovered, a problem for each build
// root of a language other than Go that coverage measured nothing of while
// a file of it has mutants this run must judge: the files Run would
// otherwise fall back on, running every mutant. Each fails closed with the
// problem unmeasured gives it, as Go does (ADR-0017). A file of a language
// measured elsewhere in the run is uncovered instead, as before, and a file
// with nothing to judge needs no coverage.
func unmeasuredToJudge(report *coverage.Report, sources []string, checks []mutate.FileCheck, mutateAll bool) []*problem {
	var found []coverage.Unmeasured
	seen := map[int]bool{}
	for i, source := range sources {
		spec := lang.Detect(source)
		if spec == nil || spec.Name == "go" || i >= len(checks) {
			continue
		}
		if !mutate.NeedsMutationCoverage(checks[i:i+1], mutateAll) || report.Has(source) || report.Measures(spec.Name) {
			continue
		}
		abs, err := filepath.Abs(source)
		if err != nil {
			abs = source
		}
		matched := false
		for j, m := range report.Missing() {
			dir, err := filepath.Abs(m.Dir)
			if m.Language != spec.Name || m.Dir == "" || err != nil {
				continue
			}
			if rel, err := filepath.Rel(dir, abs); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			matched = true
			if !seen[j] {
				seen[j] = true
				found = append(found, m)
			}
		}
		if !matched {
			found = append(found, coverage.Unmeasured{Dir: filepath.Dir(abs), Language: spec.Name, Cause: coverage.MeasuredNothing,
				Reason: "its coverage run measured none of its files"})
		}
	}
	return unmeasuredProblems(relativeUnmeasured(found))
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
		p := fail(kindNo, "mutation.exception-stale", message, fix)
		p.subject = staleSubject(rel, e)
		p.shown = true
		in.report(p)
	}
}

// staleSubject is the subject of the mutation.exception-stale problem of e,
// an exception of the file rel.
func staleSubject(rel string, e mutate.StaleException) map[string]any {
	subject := map[string]any{"file": rel, "function": e.Function, "column": e.Column,
		"original": e.Original, "replacement": e.Replacement, "why": e.Why}
	if e.Line != 0 {
		subject["line"] = e.Line
	}
	if e.Why == mutate.ExceptionMoved {
		subject["new_file"] = project.Rel(project.AtRoot(e.NewFile))
	}
	return subject
}
