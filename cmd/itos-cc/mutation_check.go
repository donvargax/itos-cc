package main

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/mutate"
	"github.com/donvargax/itos-cc/project"
)

var mutationCheckCommand = &command{
	name:     "mutation check",
	summary:  "check the cached mutation results without running tests",
	synopsis: "[options] [path ...]",
	about: `
Says, from the results mutation run cached in .metrics/mutate/ alone, whether
each function has passing results for its code as it is now. It runs no test
and no coverage command, and writes nothing, so a gate such as a commit hook
can prove mutation run was run on the code being committed.

For each chosen function with at least one mutation site, its entry in its
file's snapshot is compared with its source now. No entry is missing; an
entry whose function has changed since is stale, while a function that only
moved is fresh; an entry that lacks a site the function has now, as when a
newer itos-cc adds a mutation operator or a mutation run --fail-fast stop
left the site undecided, is stale, naming each such site, and
mutation run runs only those; and a fresh entry fails on each survivor it records, and
with --fail-uncovered on each uncovered mutant it records. A function with no
mutation site needs no mutation results, but with --fail-uncovered every
judged Go function also needs fresh independent executable coverage evidence,
including zero-site functions. Uncovered executable blocks and missing or
stale evidence fail without running tests, coverage or list commands. The
snapshot also records the hash of each test
file that imports its file: when one was added, changed, or removed since,
every function of the file is stale, as it is when the snapshot predates
recording tests. A run with --since that finds them changed keeps the
entries of the functions it does not judge marked stale, until a run judges
them again.

A survivor that itos-cc.yaml excepts (see mutation except) fails nothing.
An exception that no longer holds fails as mutation.exception-stale, as in
mutation run: its function changed, its function or site is gone, or a fresh
entry records its mutant killed. A check of the whole project, or with
--since one whose range deleted or renamed an entry's file, judges the
entries of files it does not select as mutation run does: moved, when one
source checked holds the function unchanged, or gone. An itos-cc.yaml that
cannot be read is config.invalid.

It chooses as mutation run does: paths, --changed, and --since REF, which
checks only the functions the commits since REF changed (git diff
REF...HEAD), as mutation run judges them.

Plain output is a line per file, "<file>: N fresh, N stale, N missing",
which names the test files that changed when they made functions stale, then
each problem, a line each:
  missing <file>:<line> in <namespace#name>
  stale <file>:<line> in <namespace#name>
  survived <file>:<line>:<column> ` + "`original` → `replacement`" + ` in <namespace#name>
  uncovered <file>:<line>:<column> ` + "`original` → `replacement`" + ` in <namespace#name>
  exception-stale <file>:<line>:<column> ` + "`original` → `replacement`" + ` in <namespace#name> (<why>)
the place being the file alone when the function or the file is gone, and
the why "moved to <file>" when the function is in another file now. The line of a missing or stale function is where it starts now. With --json,
"functions" holds the functions checked, in the file's order, with the
counts their entry records, zero when missing; an excepted survivor counts
in "excepted", not "survived".`,
	flags: append(append([]flagSpec{}, selectionFlags...),
		opt("since", stringFlag, "REF", "", "check only the functions the commits since REF changed (git diff REF...HEAD)"),
		sw("fail-uncovered", "fail on uncovered mutants and executable Go coverage blocks")),
	json: `"files": [{"file", "functions": [{"function",
   "state": "fresh"|"stale"|"missing", "killed", "survived", "excepted",
   "uncovered"}]}]`,
	rules: []string{
		"mutation.missing          a function with a mutation site has no results: file, line, function",
		"mutation.stale            a function, or a test that imports its file, changed since its results, or they never recorded one of its sites: file, line, function",
		"mutation.survived         its results record a survivor: file, line, column, function, original, replacement",
		"mutation.uncovered        with --fail-uncovered, its results record an uncovered mutant: file, line, column, function, original, replacement",
		"mutation.uncovered-statement with --fail-uncovered, a measured Go coverage block is uncovered: file, function, line",
		"mutation.coverage-missing  with --fail-uncovered, a Go function lacks complete measured coverage evidence: file, function, line",
		"mutation.coverage-stale    with --fail-uncovered, Go coverage inputs changed since measurement: file, function, line",
		"mutation.coverage-unsupported with --fail-uncovered, strict coverage reaches beyond the inventoried Go module: file, function, line",
		"mutation.exception-stale  an exception in itos-cc.yaml no longer holds: file, function, line (none when the function or its file is gone), column, original, replacement, why: killed|changed|gone|moved, and with moved new_file",
		"config.invalid            itos-cc.yaml cannot be read: file",
		"since.bad-ref             --since names no commit: ref",
		"since.no-git              --since outside a git repository",
		"flags.conflict            --since with --changed: flag",
	},
	exits: []exitDoc{
		{0, "every function checked has fresh results, and none records a survivor"},
		{1, "a function's results are missing or stale, or record a survivor itos-cc.yaml does not except, or an uncovered mutant, Go executable block, or missing/stale Go coverage evidence with --fail-uncovered; or an exception is stale"},
		{2, "a usage or config error: a bad flag or path, a --since ref that is no commit, --since with --changed, or an itos-cc.yaml that cannot be read"},
		{3, "--changed or --since outside a git repository"},
	},
	examples: []string{
		"itos-cc mutation check --since origin/main --fail-uncovered  # a commit hook",
		"itos-cc mutation check --json src/billing",
	},
	run: runMutationCheck,
}

type checkFile struct {
	File      string          `json:"file"`
	Functions []checkFunction `json:"functions"`
}

type checkFunction struct {
	Function  string `json:"function"`
	State     string `json:"state"`
	Killed    int    `json:"killed"`
	Survived  int    `json:"survived"`
	Excepted  int    `json:"excepted"`
	Uncovered int    `json:"uncovered"`
}

type checkResult struct {
	Files []checkFile `json:"files"`
}

// runMutationCheck checks the chosen functions' cached results.
func runMutationCheck(in *invocation) (any, error) {
	result := checkResult{Files: []checkFile{}}
	cfg, err := loadConfig()
	if err != nil {
		return result, err
	}
	support, err := supportNow(cfg)
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
		fmt.Fprintln(os.Stderr, "itos-cc: no source files to check")
		reportElsewhere(in, elsewhere)
		return result, nil
	}
	tests, err := importingTests()
	if err != nil {
		return result, err
	}
	checks, err := mutate.Check(sources, judge, tests, support, append(slices.Clone(cfg.Exceptions), moved...))
	if err != nil {
		return result, err
	}
	failUncovered := in.set("fail-uncovered")
	if failUncovered && hasGoSource(sources) {
		producer := "go test -count=1 -covermode=set -coverprofile=coverage.out; scope=own"
		coverageChecks, err := mutate.CheckGoCoverage(sources, judge, func(path string) (string, map[string]string, error) {
			inputs, err := mutate.GoCoverageInputs(path, project.Root(), producer, support)
			return producer, inputs, err
		})
		if err != nil {
			return result, err
		}
		for _, coverageCheck := range coverageChecks {
			switch coverageCheck.State {
			case "unsupported":
				reportCoverageProblem(in, coverageCheck.File, coverageCheck.Function, coverageCheck.Line,
					"mutation.coverage-unsupported", "uses unsupported Go coverage scope "+strings.Join(coverageCheck.Changed, ", "))
			case "missing":
				reportCoverageProblem(in, coverageCheck.File, coverageCheck.Function, coverageCheck.Line,
					"mutation.coverage-missing", "has no complete fresh measured Go coverage inventory")
			case "stale":
				reportCoverageProblem(in, coverageCheck.File, coverageCheck.Function, coverageCheck.Line,
					"mutation.coverage-stale", "has Go coverage made before inputs changed: "+strings.Join(coverageCheck.Changed, ", "))
			default:
				for _, block := range coverageCheck.Blocks {
					if !block.Covered {
						reportCoverageProblem(in, coverageCheck.File, coverageCheck.Function, block.Line,
							"mutation.uncovered-statement", "has an uncovered executable Go coverage block")
					}
				}
			}
		}
	}
	for _, c := range checks {
		f := checkFile{File: c.Rel, Functions: []checkFunction{}}
		count := map[string]int{}
		for _, fn := range c.Functions {
			count[fn.State]++
			excepted := exceptedIn(fn.Mutants)
			f.Functions = append(f.Functions, checkFunction{Function: fn.Function, State: fn.State,
				Killed: fn.Entry.Killed, Survived: fn.Entry.Survived - excepted, Excepted: excepted, Uncovered: fn.Entry.Uncovered})
		}
		result.Files = append(result.Files, f)
		if !in.json {
			fmt.Printf("%s: %d fresh, %d stale, %d missing%s\n", c.Rel, count[mutate.Fresh], count[mutate.Stale], count[mutate.Missing],
				testsNote(c.Functions))
		}
		for _, fn := range c.Functions {
			switch fn.State {
			case mutate.Missing:
				reportFunction(in, c.Rel, fn, "mutation.missing", "has no mutation results")
			case mutate.Stale:
				reportFunction(in, c.Rel, fn, "mutation.stale", staleBecause(fn))
			default:
				reportFailed(in, c.Rel, fn.Function, fn.Mutants, failUncovered)
			}
		}
		reportStaleExceptions(in, c.Rel, c.StaleExceptions)
	}
	reportElsewhere(in, elsewhere)
	return result, nil
}

// staleBecause says why a stale function's results no longer hold: the
// function changed, or the tests that import its file did, named, or did
// before a run that did not judge it kept its entry marked stale, or it has
// sites they never recorded, each named as "<line>:<column> `original` →
// `replacement`".
func staleBecause(fn mutate.FunctionCheck) string {
	tests := fn.Tests
	switch {
	case fn.Marked:
		return "has mutation results from before the tests that import its file changed, kept by a mutation run that did not judge it"
	case len(fn.Listed) > 0:
		return "has kills by listed tests made before files they rest on changed: " + strings.Join(fn.Listed, ", ")
	case len(fn.Broad) > 0:
		return "has Go whole-suite results made before inputs changed: " + strings.Join(fn.Broad, ", ")
	case len(fn.Unrecorded) > 0:
		var sites []string
		for _, s := range fn.Unrecorded {
			sites = append(sites, fmt.Sprintf("%d:%d `%s` → `%s`", s.Line, s.Column, s.Original, s.Replacement))
		}
		return "has sites its mutation results never recorded, as when a newer itos-cc adds a mutation operator or a fail-fast stop left them undecided: " + strings.Join(sites, ", ")
	case tests == nil:
		return "changed since its mutation results"
	case tests.Unrecorded:
		return "has mutation results from before snapshots recorded the tests that import its file"
	}
	return "has mutation results from before the tests that import its file changed: " + testChanges(tests)
}

// testChanges names the test files changed, added, and removed, as
// "changed a, b; added c".
func testChanges(tests *mutate.TestChange) string {
	var changes []string
	for _, c := range []struct {
		what  string
		paths []string
	}{{"changed", tests.Changed}, {"added", tests.Added}, {"removed", tests.Removed}} {
		if len(c.paths) > 0 {
			changes = append(changes, c.what+" "+strings.Join(c.paths, ", "))
		}
	}
	return strings.Join(changes, "; ")
}

// testsNote ends a file's summary line when its tests made functions stale.
func testsNote(functions []mutate.FunctionCheck) string {
	for _, fn := range functions {
		switch {
		case fn.Tests == nil:
		case fn.Tests.Unrecorded:
			return " (its results predate snapshots recording tests)"
		default:
			return " (the tests that import it: " + testChanges(fn.Tests) + ")"
		}
	}
	return ""
}

// reportFunction lists function fn of file rel on stdout, as "<what>
// <file>:<line> in <function>", what being rule's last word, and reports it
// as a problem of rule whose message ends with verdict.
func reportFunction(in *invocation, rel string, fn mutate.FunctionCheck, rule, verdict string) {
	what := rule[len("mutation."):]
	if !in.json {
		fmt.Printf("  %s %s:%d in %s\n", what, rel, fn.StartLine, fn.Function)
	}
	p := fail(kindNo, rule, fmt.Sprintf("%s:%d: %s %s", rel, fn.StartLine, fn.Function, verdict),
		fmt.Sprintf("Run 'itos-cc mutation run %s', and commit .metrics/mutate/ with the code.", rel)).
		with("file", rel).with("line", fn.StartLine).with("function", fn.Function)
	p.shown = true
	in.report(p)
}
