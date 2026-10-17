package main

import (
	"errors"
	"fmt"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
)

// classifyCoverage is the coverage domain's classifier, which cmd hands to
// internal/cli: a build root or report coverage did not measure, a
// coverage.Unmeasured, is its gap's problem, and a measurement the port
// failed whole, its Failure's; any other error is not the coverage
// domain's to claim. crap and mutation share the domain, so cmd holds it,
// as it holds selection's, and each maps every gap through it.
func classifyCoverage(err error) ([]*cli.Problem, bool) {
	var m coverage.Unmeasured
	if errors.As(err, &m) {
		return []*cli.Problem{gapProblem(m)}, true
	}
	var failure covport.Failure
	if errors.As(err, &failure) {
		return []*cli.Problem{measureProblem(failure)}, true
	}
	return nil, false
}

// gapProblem is m's problem, by its gap: a missing tool or report is the
// environment, tests that measured nothing say no, an unreadable
// --coverage-report is a usage error, and a measurement an interrupt
// stopped is the stop (decision 31). Each names the build root, language
// and report m names. None has a default; a gap with no case here is the
// 70 of a bug.
func gapProblem(m coverage.Unmeasured) *cli.Problem {
	p := cli.Internal(fmt.Errorf("coverage gap %v: %s", m.Cause, m))
	switch m.Cause.(type) {
	case covport.ToolMissing:
		p = cli.NewProblem(cli.CodeEnvironment, "coverage.tool-missing", "no coverage for "+m.String(), "Install what it names, or measure with --coverage-command.")
	case covport.MeasuredNothing:
		p = cli.NewProblem(cli.CodeNo, "coverage.measured-nothing", "no coverage for "+m.String(), "Run the tests and fix them, then run itos-cc again.")
	case covport.NoReport:
		p = cli.NewProblem(cli.CodeEnvironment, "coverage.no-report", "no coverage for "+m.String(), "Run without --use-existing-coverage, or write the report first.")
	case covport.Unreadable:
		p = cli.NewProblem(cli.CodeUsage, "coverage.report-unreadable", "no coverage for "+m.String(), "Check the --coverage-report path and its format.")
	case covport.Cancelled:
		p = cli.Interrupted()
	}
	if m.Dir != "" {
		p.With("dir", m.Dir)
	}
	if m.Language != "" {
		p.With("language", m.Language)
	}
	if m.Report != "" {
		p.With("report", m.Report)
	}
	return p
}

// measureProblem is failure's problem: a stopped measurement is its gap's,
// Cancelled, the stop; a run folder that could not be made is the 70 it
// has always been, the operating system's text whole. None has a default;
// a kind with no case here is the 70 of a bug.
func measureProblem(failure covport.Failure) *cli.Problem {
	p := cli.Internal(failure)
	switch e := failure.(type) {
	case covport.Cancelled:
		p = gapProblem(coverage.Unmeasured{Cause: e})
	case covport.Unwritable:
		p = cli.Internal(e)
	}
	return p
}
