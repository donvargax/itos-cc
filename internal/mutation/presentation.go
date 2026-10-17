package mutation

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/mutants"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
)

// Presentation owns shared mutation wording. Remaining cmd handlers delegate
// here until they move; caller context reaches any actual file observation.
type Presentation struct {
	UI       *cli.UI
	JSON     bool
	Root     string
	Tree     selport.Tree
	Problems []*cli.Problem
}

func (p *Presentation) report(problem *cli.Problem) { p.Problems = append(p.Problems, problem) }

func (p *Presentation) Failed(rel, function string, items []mutants.Mutant, failUncovered func(line int) bool) {
	for _, m := range items {
		switch {
		case m.Outcome == mutants.Survived && m.Excepted == "":
			p.Mutant(rel, function, m, "mutation.survived", "survived", "survived",
				fmt.Sprintf("Add a test that fails with this change. If no test can, as the change changes no behaviour, except it with 'itos-cc mutation except %s:%d:%d --reason …'.",
					rel, m.Line, m.Column))
		case m.Outcome == mutants.Uncovered && failUncovered != nil && failUncovered(m.Line):
			p.Mutant(rel, function, m, "mutation.uncovered", "uncovered", "is uncovered: no test executes its line",
				"Add a test that executes this line and fails with this change.")
		}
	}
}

func (p *Presentation) Mutant(rel, function string, m mutants.Mutant, rule, what, verdict, fix string) {
	if !p.JSON {
		_, _ = fmt.Fprintf(p.UI.Stdout, "  %s %s:%d:%d %s → %s in %s\n", what, rel, m.Line, m.Column,
			Quote(m.Original), Quote(m.Replacement), function)
	}
	problem := cli.NewProblem(cli.CodeNo, rule,
		fmt.Sprintf("%s:%d:%d: %s → %s in %s %s", rel, m.Line, m.Column, Quote(m.Original), Quote(m.Replacement), function, verdict),
		fix).
		With("file", rel).With("line", m.Line).With("column", m.Column).With("function", function).
		With("original", m.Original).With("replacement", m.Replacement)
	problem.Shown = true
	p.report(problem)
}

func (p *Presentation) CoverageProblem(language, file, function string, line int, rule, message string) {
	if !p.JSON {
		_, _ = fmt.Fprintf(p.UI.Stdout, "  %s %s:%d in %s\n", strings.TrimPrefix(rule, "mutation."), file, line, function)
	}
	fix := "Add a test that executes the uncovered " + LanguageName(language) + " code, then run mutation run again."
	if rule == "mutation.coverage-unsupported" {
		fix = "Strict coverage cannot prove inputs beyond the inventoried root: keep them inside it, or judge this code without --fail-uncovered."
	}
	problem := cli.NewProblem(cli.CodeNo, rule, fmt.Sprintf("%s:%d in %s %s", file, line, function, message), fix).
		With("file", file).With("function", function).With("line", line)
	problem.Shown = true
	p.report(problem)
}

func (p *Presentation) StaleExceptions(ctx context.Context, rel string, stale []mutants.StaleException) {
	for _, e := range stale {
		at := fmt.Sprintf("%s:%d:%d", rel, e.Line, e.Column)
		if e.Line == 0 {
			at = rel
		}
		change := fmt.Sprintf("%s → %s in %s", Quote(e.Original), Quote(e.Replacement), e.Function)
		var newFile string
		if e.Why == mutants.ExceptionMoved {
			newFile = p.UI.Rel(sourcePath(p.Root, e.NewFile))
		}
		if !p.JSON {
			why := e.Why
			if newFile != "" {
				why += " to " + newFile
			}
			_, _ = fmt.Fprintf(p.UI.Stdout, "  exception-stale %s %s (%s)\n", at, change, why)
		}
		var message, fix string
		switch e.Why {
		case mutants.ExceptionKilled:
			message = fmt.Sprintf("%s: %s is excepted in %s, but the tests now kill it", at, change, "itos-cc.yaml")
			fix = "Remove its entry from " + "itos-cc.yaml" + ": the tests show that the mutant changes behaviour."
		case mutants.ExceptionChanged:
			message = fmt.Sprintf("%s: the entry in %s for %s was written for another version of the function", at, "itos-cc.yaml", change)
			fix = fmt.Sprintf("Run 'itos-cc mutation except --renew' after mutation run: it carries the entry over, keeping its reason, when its mutant is unchanged. "+
				"If it is not renewed and the mutant still survives and changes no behaviour, except it again with 'itos-cc mutation except %s --reason …'; otherwise remove its entry from %s.", at, "itos-cc.yaml")
		case mutants.ExceptionGone:
			message = fmt.Sprintf("%s: the entry in %s for %s names a site the function no longer has", at, "itos-cc.yaml", change)
			if e.Line == 0 {
				message = fmt.Sprintf("%s: the entry in %s for %s names a function the file no longer has", at, "itos-cc.yaml", change)
				if p.Tree == nil {
					p.report(cli.Internal(errors.New("mutation source tree binding is missing")))
					continue
				}
				if !p.Tree.Exists(ctx, sourcePath(p.Root, e.File)) {
					message = fmt.Sprintf("%s: the entry in %s for %s names a file that is gone", at, "itos-cc.yaml", change)
				}
			}
			fix = "Remove its entry from " + "itos-cc.yaml" + "."
		case mutants.ExceptionExcluded:
			message = fmt.Sprintf("%s: the entry in %s for %s names a source that its exclude leaves out, which no command mutates", at, "itos-cc.yaml", change)
			fix = "Remove its entry from " + "itos-cc.yaml" + "."
		case mutants.ExceptionMoved:
			message = fmt.Sprintf("%s: the entry in %s for %s names a file that is gone; its function is in %s now, where the entry still excepts the mutant",
				at, "itos-cc.yaml", change, newFile)
			fix = fmt.Sprintf("Change the entry's file in %s from %s to %s, where its function is now.", "itos-cc.yaml", e.File, e.NewFile)
		}
		problem := cli.NewProblem(cli.CodeNo, "mutation.exception-stale", message, fix)
		for key, value := range p.StaleSubject(rel, e) {
			problem = problem.With(key, value)
		}
		problem.Shown = true
		p.report(problem)
	}
}

func (p *Presentation) StaleSubject(rel string, e mutants.StaleException) map[string]any {
	subject := map[string]any{"file": rel, "function": e.Function, "column": e.Column,
		"original": e.Original, "replacement": e.Replacement, "why": e.Why}
	if e.Line != 0 {
		subject["line"] = e.Line
	}
	if e.Why == mutants.ExceptionMoved {
		subject["new_file"] = p.UI.Rel(sourcePath(p.Root, e.NewFile))
	}
	return subject
}

// FailedSelections reports each selection of listed tests in failed that
// reported does not hold yet as tests.selection-failed, showing its output
// in plain output, and adds it to reported. command is the one to run again.
func (p *Presentation) FailedSelections(failed []mutants.FailedSelection, reported map[string]bool, command string) {
	for _, f := range failed {
		key := strings.Join(slices.Sorted(slices.Values(f.IDs)), " ")
		if reported[key] {
			continue
		}
		reported[key] = true
		ids := strings.Join(f.IDs, " ")
		if !p.JSON {
			_, _ = fmt.Fprintf(p.UI.Stdout, "listed tests %s fail without any mutant\n%s\n", ids, Tail(f.Output, 20))
		}
		p.report(cli.NewProblem(cli.CodeNo, "tests.selection-failed",
			fmt.Sprintf("the listed tests %s fail without any mutant, so no mutant that would run them was judged: %s", ids, f.Command),
			fmt.Sprintf("Make them pass when they run alone, as that command runs them, then run %s again.", command)).
			With("ids", f.IDs).With("command", f.Command).With("exit_code", f.ExitCode))
	}
}

// Tail is the last lines of s, its trailing newlines dropped.
func Tail(s string, lines int) string {
	parts := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, "\n")
}
