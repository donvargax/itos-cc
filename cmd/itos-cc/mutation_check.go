package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/donvargax/itos-cc/mutate"
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
moved is fresh; and a fresh entry fails on each survivor it records, and
with --fail-uncovered on each uncovered mutant it records. A function with no
mutation site needs no entry. The snapshot also records the hash of each test
file that imports its file: when one was added, changed, or removed since,
every function of the file is stale, as it is when the snapshot predates
recording tests.

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
The line of a missing or stale function is where it starts now. With --json,
"functions" holds the functions checked, in the file's order, with the
counts their entry records, zero when missing.`,
	flags: append(append([]flagSpec{}, selectionFlags...),
		opt("since", stringFlag, "REF", "", "check only the functions the commits since REF changed (git diff REF...HEAD)"),
		sw("fail-uncovered", "fail on each recorded uncovered mutant, as on a survivor")),
	json: `"files": [{"file", "functions": [{"function",
   "state": "fresh"|"stale"|"missing", "killed", "survived", "uncovered"}]}]`,
	rules: []string{
		"mutation.missing          a function with a mutation site has no results: file, line, function",
		"mutation.stale            a function, or a test that imports its file, changed since its results: file, line, function",
		"mutation.survived         its results record a survivor: file, line, column, function, original, replacement",
		"mutation.uncovered        with --fail-uncovered, its results record an uncovered mutant: file, line, column, function, original, replacement",
		"since.bad-ref             --since names no commit: ref",
		"since.no-git              --since outside a git repository",
		"flags.conflict            --since with --changed: flag",
	},
	exits: []exitDoc{
		{0, "every function checked has fresh results, and none records a survivor"},
		{1, "a function's results are missing or stale, or record a survivor, or an uncovered mutant with --fail-uncovered"},
		{2, "a usage error: a bad flag or path, a --since ref that is no commit, or --since with --changed"},
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
	Uncovered int    `json:"uncovered"`
}

type checkResult struct {
	Files []checkFile `json:"files"`
}

// runMutationCheck checks the chosen functions' cached results.
func runMutationCheck(in *invocation) (any, error) {
	result := checkResult{Files: []checkFile{}}
	sources, judge, err := mutationSelection(in)
	if err != nil {
		return result, err
	}
	if len(sources) == 0 {
		fmt.Fprintln(os.Stderr, "itos-cc: no source files to check")
		return result, nil
	}
	tests, err := importingTests()
	if err != nil {
		return result, err
	}
	checks, err := mutate.Check(sources, judge, tests)
	if err != nil {
		return result, err
	}
	failUncovered := in.set("fail-uncovered")
	for _, c := range checks {
		f := checkFile{File: c.Rel, Functions: []checkFunction{}}
		count := map[string]int{}
		for _, fn := range c.Functions {
			count[fn.State]++
			f.Functions = append(f.Functions, checkFunction{Function: fn.Function, State: fn.State,
				Killed: fn.Entry.Killed, Survived: fn.Entry.Survived, Uncovered: fn.Entry.Uncovered})
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
				reportFunction(in, c.Rel, fn, "mutation.stale", staleBecause(fn.Tests))
			default:
				reportFailed(in, c.Rel, fn.Function, fn.Mutants, failUncovered)
			}
		}
	}
	return result, nil
}

// staleBecause says why a stale function's results no longer hold: the
// function changed, or the tests that import its file did, named.
func staleBecause(tests *mutate.TestChange) string {
	switch {
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
