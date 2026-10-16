package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/cli"
	mutation "github.com/donvargax/itos-cc/internal/mutants"
)

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
	// LeftOut is how many mutants its fresh entry records at sites it no
	// longer holds, left out of the counts; left out when there are none.
	LeftOut int `json:"left_out,omitempty"`
}

type checkResult struct {
	Files []checkFile `json:"files"`
	// Orphans are the snapshots under the paths checked whose source file
	// is gone: named, never judged; left out when there are none.
	Orphans []orphanEntry `json:"orphans,omitempty"`
}

// runMutationCheck checks the chosen functions' cached results.
func runMutationCheck(ctx context.Context, in *invocation) (any, error) {
	result := checkResult{Files: []checkFile{}}
	cfg, err := loadConfig()
	if err != nil {
		return result, err
	}
	support, err := supportNow(ctx, in, cfg)
	if err != nil {
		return result, err
	}
	linesOnly, err := changedLinesOnly(in)
	if err != nil {
		return result, err
	}
	sources, judge, renamed, lines, err := mutationSelection(ctx, in)
	if err != nil {
		return result, err
	}
	// With --fail-uncovered=lines, only the lines the range changed are
	// judged: a function with none needs no evidence, and an uncovered
	// block, line or mutant elsewhere fails nothing.
	var scope mutation.LineScope
	if linesOnly {
		scope = lines
	}
	elsewhere, moved, err := exceptionsElsewhere(ctx, in, cfg.Exceptions, sources, renamed)
	if err != nil {
		return result, err
	}
	orphans, err := orphansCovered(ctx, in)
	if err != nil {
		return result, err
	}
	result.Orphans = orphansJSON(orphans)
	if len(sources) == 0 {
		fmt.Fprintln(os.Stderr, "itos-cc: no source files to check")
		reportElsewhere(in, elsewhere)
		printOrphans(in, result.Orphans, "is an orphan, which mutation run removes")
		return result, nil
	}
	if err := mutationInputsBound(in.adapters); err != nil {
		return result, err
	}
	if in.adapters.Tree == nil || in.adapters.Surroundings == nil {
		return result, errors.New("mutation source tree binding is missing")
	}
	tests, err := importingTests(ctx, in)
	if err != nil {
		return result, err
	}
	checks, err := checkMutations(ctx, in, sources, judge, tests, support, append(slices.Clone(cfg.Exceptions), moved...))
	if err != nil {
		return result, err
	}
	failUncovered := in.set("fail-uncovered")
	if languages := strictLanguagesOf(sources); failUncovered && len(languages) > 0 {
		coverageChecks, err := checkMutationCoverage(ctx, in, sources, judge, languages, support)
		if err != nil {
			return result, err
		}
		for _, coverageCheck := range coverageChecks {
			language := coverageCheck.Language
			if !scope.Judges(coverageCheck.File, coverageCheck.Line, coverageCheck.EndLine) {
				continue
			}
			switch coverageCheck.State {
			case "unsupported":
				reportLanguageCoverageProblem(in, language, coverageCheck.File, coverageCheck.Function, coverageCheck.Line,
					"mutation.coverage-unsupported", "uses unsupported "+languageName(language)+" coverage scope "+strings.Join(coverageCheck.Changed, ", "))
			case "missing":
				reportLanguageCoverageProblem(in, language, coverageCheck.File, coverageCheck.Function, coverageCheck.Line,
					"mutation.coverage-missing", "has no complete fresh measured "+coverageInventory(language))
			case "stale":
				reportLanguageCoverageProblem(in, language, coverageCheck.File, coverageCheck.Function, coverageCheck.Line,
					"mutation.coverage-stale", "has "+coverageEvidenceName(language)+" made before inputs changed: "+strings.Join(coverageCheck.Changed, ", "))
			default:
				for _, block := range coverageCheck.Blocks {
					if !block.Covered && scope.JudgesBlock(coverageCheck.File, block) {
						reportLanguageCoverageProblem(in, language, coverageCheck.File, coverageCheck.Function, block.Line,
							"mutation.uncovered-statement", "has an uncovered "+executableUnit(language))
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
				Killed: fn.Entry.Killed, Survived: fn.Entry.Survived - excepted, Excepted: excepted, Uncovered: fn.Entry.Uncovered,
				LeftOut: fn.LeftOut})
		}
		result.Files = append(result.Files, f)
		if !in.json {
			fmt.Printf("%s: %d fresh, %d stale, %d missing%s\n", c.Rel, count[mutation.SnapshotFresh], count[mutation.Stale], count[mutation.Missing],
				testsNote(c.Functions))
		}
		for _, fn := range c.Functions {
			switch fn.State {
			case mutation.Missing:
				reportFunction(in, c.Rel, fn, "mutation.missing", "has no mutation results")
			case mutation.Stale:
				reportFunction(in, c.Rel, fn, "mutation.stale", staleBecause(fn))
			default:
				reportFailed(in, c.Rel, fn.Function, fn.Mutants, uncoveredFails(failUncovered, scope, c.Rel))
				printLeftOut(in, c.Rel, fn)
			}
		}
		reportStaleExceptions(in, c.Rel, c.StaleExceptions)
	}
	reportElsewhere(in, elsewhere)
	printOrphans(in, result.Orphans, "is an orphan, which mutation run removes")
	return result, nil
}

// staleBecause says why a stale function's results no longer hold: the
// function changed, or the tests that import its file did, named, or did
// before a run that did not judge it kept its entry marked stale, or it has
// sites they never recorded, each named as "<line>:<column> `original` →
// `replacement`".
func staleBecause(fn mutation.FunctionCheck) string {
	tests := fn.Tests
	switch {
	case fn.Marked:
		return "has mutation results from before the tests that import its file changed, kept by a mutation run that did not judge it"
	case len(fn.Listed) > 0:
		return "has kills by listed tests made before files they rest on changed: " + strings.Join(fn.Listed, ", ")
	case len(fn.Broad) > 0:
		return "has whole-suite results made before inputs changed: " + strings.Join(fn.Broad, ", ")
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
func testChanges(tests *mutation.TestChange) string {
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
func testsNote(functions []mutation.FunctionCheck) string {
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

// printLeftOut notes, in plain output, the mutants fn's fresh entry records
// at sites it no longer holds, as "left out <file>:<line> in <function>: <n>
// recorded mutants at sites gone, which the next mutation run drops": a
// note, not a problem, so it fails nothing.
func printLeftOut(in *invocation, rel string, fn mutation.FunctionCheck) {
	if in.json || fn.LeftOut == 0 {
		return
	}
	mutants := "mutants"
	if fn.LeftOut == 1 {
		mutants = "mutant"
	}
	fmt.Printf("  left out %s:%d in %s: %d recorded %s at sites gone, which the next mutation run drops\n",
		rel, fn.StartLine, fn.Function, fn.LeftOut, mutants)
}

// reportFunction lists function fn of file rel on stdout, as "<what>
// <file>:<line> in <function>", what being rule's last word, and reports it
// as a problem of rule whose message ends with verdict.
func reportFunction(in *invocation, rel string, fn mutation.FunctionCheck, rule, verdict string) {
	what := rule[len("mutation."):]
	if !in.json {
		fmt.Printf("  %s %s:%d in %s\n", what, rel, fn.StartLine, fn.Function)
	}
	p := cli.NewProblem(cli.CodeNo, rule, fmt.Sprintf("%s:%d: %s %s", rel, fn.StartLine, fn.Function, verdict),
		fmt.Sprintf("Run 'itos-cc mutation run %s', and commit .metrics/mutate/ with the code.", rel)).
		With("file", rel).With("line", fn.StartLine).With("function", fn.Function)
	p.Shown = true
	in.report(p)
}
