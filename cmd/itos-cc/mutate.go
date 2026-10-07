package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/mutate"
	"github.com/donvargax/itos-cc/project"
)

// mutationGroup is mutation testing: mutation run mutates and runs the
// tests, mutation list lists the sites without running any.
var mutationGroup = &command{
	name:    "mutation",
	summary: "mutation testing: do the tests notice small changes?",
	about: `
Mutation testing: would the tests notice if this code were wrong? mutation run
changes one operator, boolean, or 0/1 at a time inside each function and runs
the tests; mutation list lists those changes, the mutation sites, without
running any test.`,
	examples: []string{
		"itos-cc mutation run --changed",
		"itos-cc mutation list src/billing/invoice.ts",
	},
	subs: []*command{mutationRunCommand, mutationListCommand},
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
nightly. Add --no-coverage for end-to-end tests that only run the built
binary, which coverage does not see.

Results are cached in .metrics/mutate/<file>.json, which is meant to be
committed: later runs reuse killed mutants of unchanged functions and retry
only survivors and changed functions. A summary comment is kept at the end of
each source file.

--since REF judges only the functions the commits since REF changed, as a
gate on a branch's own work: git diff REF...HEAD, committed changes only.
Paths narrow it to the files under them. Functions not judged neither run nor
change in the snapshot, and only those judged count in the summary, the
problems, and the exit code.

--fail-uncovered makes each uncovered mutant a failure, listed like a
survivor, so a gate fails a change no test executes. With --since, only the
judged functions' uncovered mutants count. With --no-coverage, or where
coverage measured nothing for the language, every mutant runs and none is
uncovered.`,
	flags: append(append(append([]flagSpec{}, selectionFlags...), coverageFlags...),
		opt("workers", intFlag, "N", fmt.Sprint(max(1, runtime.NumCPU()/2)), "mutants run at the same time"),
		sw("mutate-all", "rerun killed mutants of unchanged functions too"),
		opt("timeout-factor", floatFlag, "N", "10", "a mutant times out after N times the baseline duration, and at least 2s"),
		opt("test-command", stringFlag, "CMD", "", "shell command that runs the tests, instead of the per-language default"),
		sw("no-annotate", "do not write the summary comment into source files"),
		opt("since", stringFlag, "REF", "", "judge only the functions the commits since REF changed (git diff REF...HEAD)"),
		sw("fail-uncovered", "fail on each uncovered mutant, as on a survivor")),
	json: `"files": [{"file", "killed", "survived", "uncovered", "ran", "reused",
   "baseline": "passed"|"failed", "mutants": [{"line", "column", "function",
   "original", "replacement", "outcome": "killed"|"survived"|"timeout"|"uncovered",
   "reused"}], and with --since "judged": ["namespace#name"]}]`,
	rules: []string{
		"mutation.survived         a mutant survived: file, line, column, function, original, replacement",
		"mutation.uncovered        with --fail-uncovered, no test executes a mutant: file, line, column, function, original, replacement",
		"mutation.baseline-failed  the tests fail before any mutant: file",
		"since.bad-ref             --since names no commit: ref",
		"since.no-git              --since outside a git repository",
		"flags.conflict            --since with --changed: flag",
	},
	exits: []exitDoc{
		{0, "every mutant that ran was killed"},
		{1, "a mutant survived, a mutant is uncovered with --fail-uncovered, or a file's tests fail before any mutant"},
		{2, "a usage error: a bad flag or path, a --since ref that is no commit, or --since with --changed"},
		{3, "--changed or --since outside a git repository"},
	},
	examples: []string{
		"itos-cc mutation run --changed",
		"itos-cc mutation run --since origin/main --fail-uncovered  # a branch's own commits, as a gate",
		"itos-cc mutation run --all-tests --json                    # nightly",
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
"file:line:column ` + "`original` → `replacement`" + ` in namespace#name".`,
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
	File      string `json:"file"`
	Killed    int    `json:"killed"`
	Survived  int    `json:"survived"`
	Uncovered int    `json:"uncovered"`
	Ran       int    `json:"ran"`
	Reused    int    `json:"reused"`
	Baseline  string `json:"baseline"`
	// Judged is there only with --since, empty when no function changed.
	Judged []string `json:"judged,omitzero"`
	// Mutants is every mutant of the functions judged, in site order;
	// empty, never null, when the baseline failed.
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

func runMutate(in *invocation) (any, error) {
	result := mutateResult{Files: []mutateFile{}}
	var since map[string]map[string]bool
	if in.set("since") {
		var err error
		if since, err = changedSince(in); err != nil {
			return result, err
		}
	}
	files, err := files(in)
	if err != nil {
		return result, err
	}
	if since != nil {
		// The range selects the files; paths only narrow it.
		var changed []string
		for _, f := range files.Sources {
			if _, ok := since[f]; ok {
				changed = append(changed, f)
			}
		}
		files.Sources = changed
	}
	if len(files.Sources) == 0 {
		fmt.Fprintln(os.Stderr, "itos-cc: no source files to mutate")
		return result, nil
	}
	opt := mutate.Options{
		Workers:       in.integer("workers"),
		MutateAll:     in.set("mutate-all"),
		TimeoutFactor: in.float("timeout-factor"),
		TestCommand:   in.str("test-command"),
		AllTests:      in.set("all-tests"),
		Annotate:      !in.set("no-annotate"),
		Log:           os.Stderr,
	}
	if since != nil {
		opt.Judge = func(path, function string) bool { return since[path][function] }
	}
	if !in.set("no-coverage") {
		opt.Coverage = func(sources []string) *coverage.Report {
			// Coverage comes from the tests that kill mutants, so a line only
			// other tests reach is uncovered rather than a survivor.
			report, err := loadCoverage(in, sources, coverage.OwnTests, os.Stderr)
			if err != nil {
				fmt.Fprintln(os.Stderr, "itos-cc: coverage:", err)
				return nil
			}
			return report
		}
	}

	// Uncovered mutants never run, so without it a function no test
	// executes passes.
	failUncovered := in.set("fail-uncovered")
	results, err := mutate.Run(files.Sources, opt)
	if err != nil {
		return result, err
	}
	for _, r := range results {
		if r.BaselineFailed {
			result.Files = append(result.Files, mutateFile{File: r.Rel, Baseline: "failed", Judged: r.Judged, Mutants: []mutateMutant{}})
			if !in.json {
				fmt.Printf("%s: baseline tests fail; snapshot not updated\n%s\n", r.Rel, tail(r.BaselineOutput, 20))
			}
			in.report(fail(kindNo, "mutation.baseline-failed", r.Rel+": its tests fail before any mutant, so none was judged",
				"Make its tests pass, then run mutation run again.").with("file", r.Rel))
			continue
		}
		f := mutateFile{File: r.Rel, Ran: r.Ran, Reused: r.Reused, Baseline: "passed", Judged: r.Judged, Mutants: []mutateMutant{}}
		for _, m := range r.Mutants {
			f.Mutants = append(f.Mutants, mutateMutant{Line: m.Line, Column: m.Column, Function: m.Function,
				Original: m.Original, Replacement: m.Replacement, Outcome: m.Outcome, Reused: m.Reused})
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
			f.Killed += u.Killed
			f.Survived += u.Survived
			f.Uncovered += u.Uncovered
		}
		result.Files = append(result.Files, f)
		if !in.json {
			line := fmt.Sprintf("%s: %d killed, %d survived, %d uncovered (ran %d, reused %d)",
				r.Rel, f.Killed, f.Survived, f.Uncovered, f.Ran, f.Reused)
			if r.Judged != nil {
				line += fmt.Sprintf(" (judged %d of %d functions)", len(r.Judged), r.Functions)
			}
			fmt.Println(line)
		}
		for _, u := range units {
			for _, m := range u.Mutants {
				switch {
				case m.Outcome == mutate.Survived:
					reportMutant(in, r.Rel, u, m, "mutation.survived", "survived", "survived",
						"Add a test that fails with this change.")
				case m.Outcome == mutate.Uncovered && failUncovered:
					reportMutant(in, r.Rel, u, m, "mutation.uncovered", "uncovered", "is uncovered: no test executes its line",
						"Add a test that executes this line and fails with this change.")
				}
			}
		}
	}
	return result, nil
}

// reportMutant lists mutant m of unit u in file rel on stdout, as "<what>
// <file>:<line>:<column> <original> → <replacement> in <function>", and
// reports it as a problem of rule whose message ends with verdict.
func reportMutant(in *invocation, rel string, u mutate.UnitResult, m mutate.Mutant, rule, what, verdict, fix string) {
	function := u.Namespace + "#" + u.Name
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
func changedSince(in *invocation) (map[string]map[string]bool, error) {
	if in.set("changed") {
		return nil, fail(kindUsage, "flags.conflict", "--since and --changed cannot be combined: --since judges the functions of commits, --changed whole files of the working tree",
			"Drop --changed; commit the work to judge it with --since.").with("flag", "--changed")
	}
	ref := in.str("since")
	changed, err := project.ChangedSince(ref)
	var noGit *project.NoGitError
	switch {
	case errors.Is(err, project.ErrBadRef):
		return nil, fail(kindUsage, "since.bad-ref", fmt.Sprintf("--since %s: not a commit in this repository", ref),
			"Name a branch, tag, or commit, such as origin/main; fetch a remote one first.").with("ref", ref)
	case errors.As(err, &noGit):
		return nil, fail(kindMissing, "since.no-git", "--since needs a git repository: "+noGit.Reason,
			"Run it inside a git repository, or name the paths instead.")
	}
	return changed, err
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
		for _, s := range mutate.Sites(f) {
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
