package mutation

import (
	"fmt"
	"io"
	"strings"

	"github.com/donvargax/itos-cc/internal/cli"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// Progress presents domain observations in the existing progress format on
// Log, naming each command's folder with Rel. Sample reports through it, and
// cmd's run delegates to it until run moves.
type Progress struct {
	Log io.Writer
	Rel func(path string) string
}

func (p Progress) Report(e mutants.RunEvent) {
	log := p.Log
	if log == nil {
		return
	}
	switch e.Kind {
	case mutants.MemoryUnmeasured:
		_, _ = fmt.Fprintln(log, "itos-cc: no memory cap holds on this system, as itos-cc cannot measure a test run's memory here")
	case "fresh-trial":
		r := e.FreshTrial
		said := overLimit(r.Outcome, r.Limit, r.Limits)
		own := "no own test reaches its file"
		if r.RanOwn() {
			own = "survived its own tests"
		}
		switch {
		case r.Err != nil:
			said = "not judged: " + r.Err.Error()
		case r.FailedSelection != nil:
			said = fmt.Sprintf("%s, then not judged: %s fail without any mutant", own, strings.Join(r.FailedSelection.IDs, " "))
		case r.Scope == mutants.ScopeListed:
			said = fmt.Sprintf("%s, then %s with %s", own, overLimit(r.Outcome, r.Limit, r.Limits), strings.Join(r.Tests, " "))
		}
		_, _ = fmt.Fprintf(log, "itos-cc: [%d/%d] %s:%d %s → %s %s (%.1fs)\n", e.Done, e.Total, e.File, e.Site.Line, mutants.Show(e.Site.Original), mutants.Show(e.Site.Replacement), said, r.Elapsed.Seconds())
	case "nothing":
		_, _ = fmt.Fprintln(log, "itos-cc: no mutations to test")
	case "reused":
		_, _ = fmt.Fprintln(log, "itos-cc: every mutant reused from .metrics/mutate, none to run")
	case "stop":
		_, _ = fmt.Fprintf(log, "itos-cc: --fail-fast: stopping at %s in %s; no further command starts\n", e.Stop.Rule, e.Stop.File)
	case "no-coverage":
		_, _ = fmt.Fprintf(log, "itos-cc: no coverage for %s; running every mutant\n", e.File)
	case "baseline":
		_, _ = fmt.Fprintf(log, "itos-cc: baseline %s$ %s\n", p.rel(e.Command.Dir), e.Command)
	case "baseline-project":
		_, _ = fmt.Fprintf(log, "itos-cc: its baseline failed in the worker's copy; again in the project %s$ %s\n", p.rel(e.Command.Dir), e.Command)
	case "listing":
		_, _ = fmt.Fprintf(log, "itos-cc: tests %s$ %s\n", p.rel(e.Command.Dir), e.Command)
	case "coverage-failed":
		failure := e.Failure
		if _, ok := failure.(mutants.CoverageReportRequired); ok {
			failure = cli.CommandNeedsReport()
		}
		_, _ = fmt.Fprintln(log, "itos-cc: coverage:", failure)
	case "selection-failed":
		_, _ = fmt.Fprintf(log, "itos-cc: baseline of %s failed: no mutant runs it\n", strings.Join(e.IDs, " "))
	case "selection-passed":
		_, _ = fmt.Fprintf(log, "itos-cc: baseline of %s passed in %.1fs: a mutant running it times out after %.1fs\n", strings.Join(e.IDs, " "), e.Elapsed.Seconds(), e.Timeout.Seconds())
	case "cancelled":
		_, _ = fmt.Fprintf(log, "itos-cc: %s:%d %s → %s cancelled: %v (%.1fs)\n", e.File, e.Site.Line, mutants.Show(e.Site.Original), mutants.Show(e.Site.Replacement), e.Failure, e.Elapsed.Seconds())
	case "judged", "selection-blocked":
		said, took := overLimit(e.Outcome, e.Limit, e.Limits), fmt.Sprintf("%.1fs", e.Elapsed.Seconds())
		if e.Kind == "selection-blocked" {
			said = fmt.Sprintf("survived its own tests, then not judged: %s fail without any mutant", strings.Join(e.IDs, " "))
		} else if e.IDs != nil {
			said = fmt.Sprintf("survived its own tests, then %s with %s", overLimit(e.Outcome, e.Limit, e.Limits), strings.Join(e.IDs, " "))
			took += fmt.Sprintf(", the listed tests %.1fs", e.Listed.Seconds())
		}
		_, _ = fmt.Fprintf(log, "itos-cc: [%d/%d] %s:%d %s → %s %s (%s)\n", e.Done, e.Total, e.File, e.Site.Line, mutants.Show(e.Site.Original), mutants.Show(e.Site.Replacement), said, took)
	}
}

// overLimit is outcome as a progress line says it: an OverLimit one names
// the cap, limit, its run went over, and its size, of limits.
func overLimit(outcome, limit string, limits mutants.Limits) string {
	if outcome != mutants.OverLimit {
		return outcome
	}
	size := limits.Output
	if limit == mutants.LimitMemory {
		size = limits.Memory
	}
	return fmt.Sprintf("%s: its %s went over its cap of %s", outcome, limit, mebibytes(size))
}

// mebibytes is size, in bytes, as a person reads it.
func mebibytes(size int64) string {
	return fmt.Sprintf("%.1f MiB", float64(size)/(1<<20))
}

func (p Progress) rel(path string) string {
	if p.Rel == nil {
		return path
	}
	return p.Rel(path)
}
