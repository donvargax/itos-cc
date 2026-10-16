package mutate

import (
	"fmt"
	"io"
	"strings"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/project"
)

// RunProgress presents domain observations in the existing progress format.
type RunProgress struct{ Log io.Writer }

func (p RunProgress) Report(e mutants.RunEvent) {
	log := p.Log
	if log == nil {
		return
	}
	switch e.Kind {
	case "nothing":
		fmt.Fprintln(log, "itos-cc: no mutations to test")
	case "stop":
		fmt.Fprintf(log, "itos-cc: --fail-fast: stopping at %s in %s; no further command starts\n", e.Stop.Rule, e.Stop.File)
	case "no-coverage":
		fmt.Fprintf(log, "itos-cc: no coverage for %s; running every mutant\n", e.File)
	case "baseline":
		fmt.Fprintf(log, "itos-cc: baseline %s$ %s\n", project.Rel(e.Command.Dir), e.Command)
	case "listing":
		fmt.Fprintf(log, "itos-cc: tests %s$ %s\n", project.Rel(e.Command.Dir), e.Command)
	case "coverage-failed":
		failure := e.Failure
		if _, ok := failure.(mutants.CoverageReportRequired); ok {
			failure = cli.CommandNeedsReport()
		}
		fmt.Fprintln(log, "itos-cc: coverage:", failure)
	case "selection-failed":
		fmt.Fprintf(log, "itos-cc: baseline of %s failed: no mutant runs it\n", strings.Join(e.IDs, " "))
	case "selection-passed":
		fmt.Fprintf(log, "itos-cc: baseline of %s passed in %.1fs: a mutant running it times out after %.1fs\n", strings.Join(e.IDs, " "), e.Elapsed.Seconds(), e.Timeout.Seconds())
	case "cancelled":
		fmt.Fprintf(log, "itos-cc: %s:%d %s → %s cancelled: %v (%.1fs)\n", e.File, e.Site.Line, show(e.Site.Original), show(e.Site.Replacement), e.Failure, e.Elapsed.Seconds())
	case "judged", "selection-blocked":
		said, took := e.Outcome, fmt.Sprintf("%.1fs", e.Elapsed.Seconds())
		if e.Kind == "selection-blocked" {
			said = fmt.Sprintf("survived its own tests, then not judged: %s fail without any mutant", strings.Join(e.IDs, " "))
		} else if e.IDs != nil {
			said = fmt.Sprintf("survived its own tests, then %s with %s", e.Outcome, strings.Join(e.IDs, " "))
			took += fmt.Sprintf(", the listed tests %.1fs", e.Listed.Seconds())
		}
		fmt.Fprintf(log, "itos-cc: [%d/%d] %s:%d %s → %s %s (%s)\n", e.Done, e.Total, e.File, e.Site.Line, show(e.Site.Original), show(e.Site.Replacement), said, took)
	}
}
