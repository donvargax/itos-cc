package mutation

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/cli"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
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

// baselineFailed is the problem of the file rel whose baseline failed, so
// none of its mutants was done, judged or run, before command runs again:
// its tests fail, or, when copied is the tree a worker's copy held and
// they pass in the project's own folder, they fail only in that copy,
// which holds copied alone and left out leftOut, what git ignores and
// the checkouts of other repositories it does not track, by their paths
// from the project root (slice-43): the first five of them,
// and the first the example of mutation.copy that brings it back.
func (p *Presentation) baselineFailed(rel, copied string, leftOut []string, done, command string) *cli.Problem {
	if copied == "" {
		return cli.NewProblem(cli.CodeNo, "mutation.baseline-failed", rel+": its tests fail before any mutant, so none was "+done,
			"Make its tests pass, then run "+command+" again.").With("file", rel)
	}
	name := p.UI.Rel(copied)
	if name == "." {
		name = filepath.Base(copied)
	}
	message := rel + ": its tests pass in the project but fail in the worker's copy of " + name + ", so none was " + done
	if len(leftOut) == 0 {
		return cli.NewProblem(cli.CodeNo, "mutation.baseline-failed", message,
			"The copy holds "+name+" alone: its tests read something outside it.").With("file", rel)
	}
	const named = 5
	listed := strings.Join(leftOut[:min(named, len(leftOut))], ", ")
	if len(leftOut) > named {
		listed += fmt.Sprintf(" and %d more", len(leftOut)-named)
	}
	example := leftOut[0]
	if strings.HasSuffix(example, "/") {
		example += "**"
	}
	return cli.NewProblem(cli.CodeNo, "mutation.baseline-failed", message+"; the copy left out what git ignores and other repositories' checkouts: "+listed,
		"If its tests read what it left out, name it in mutation.copy in itos-cc.yaml, such as mutation: {copy: ["+example+"]}; else they read something outside "+name+", which the copy holds alone.").With("file", rel)
}

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
			fix = pruneKilledFix
		case mutants.ExceptionChanged:
			message = fmt.Sprintf("%s: the entry in %s for %s was written for another version of the function", at, "itos-cc.yaml", change)
			fix = "Run 'itos-cc mutation except --renew' after mutation run: it carries the entry over, keeping its reason, when its mutant is unchanged. " +
				"If it is not renewed and the mutant still survives and changes no behaviour, except it at its site now with 'itos-cc mutation except " + rel + ":<line>:<column>', " +
				"which replaces this entry and keeps its reason unless --reason gives another; otherwise remove its entry from " + "itos-cc.yaml" + "."
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
			fix = pruneFix
		case mutants.ExceptionExcluded:
			message = fmt.Sprintf("%s: the entry in %s for %s names a source that its exclude leaves out, which no command mutates", at, "itos-cc.yaml", change)
			fix = pruneFix
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

// reportedLines is how many lines of each end of a failing baseline's
// output its report gives.
const reportedLines = 20

// HeadAndTail is o, a failing baseline's output, as its report gives it
// (issue #47): whole, its trailing newlines dropped, when it holds no more
// than twice n lines, else its first n lines and its last n, with one
// line between them saying how many lines were left out, those its run
// did not keep among them. A cause printed first, as Vitest prints it,
// and the bulk after both show.
func HeadAndTail(o mutants.Output, n int) string {
	if !o.Cut {
		lines := strings.Split(strings.TrimRight(o.Text, "\n"), "\n")
		if len(lines) <= 2*n {
			return strings.Join(lines, "\n")
		}
		return leftOutBetween(lines[:n], len(lines)-2*n, lines[len(lines)-n:])
	}
	head := strings.Split(o.Text[:o.At], "\n")
	rest := strings.TrimRight(o.Text[o.At:], "\n")
	tail := strings.Split(rest, "\n")
	total := strings.Count(o.Text[:o.At], "\n") + o.LeftOut + strings.Count(rest, "\n") + 1
	head, tail = head[:min(n, len(head))], tail[max(0, len(tail)-n):]
	return leftOutBetween(head, max(0, total-len(head)-len(tail)), tail)
}

// leftOutBetween is head, a line saying left lines were left out, then
// tail.
func leftOutBetween(head []string, left int, tail []string) string {
	said := fmt.Sprintf("… %d lines left out …", left)
	if left == 1 {
		said = "… 1 line left out …"
	}
	return strings.Join(slices.Concat(head, []string{said}, tail), "\n")
}

// Tail is the last lines of s, its trailing newlines dropped.
func Tail(s string, lines int) string {
	parts := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, "\n")
}
