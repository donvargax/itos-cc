package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	"github.com/donvargax/itos-cc/internal/covrun"
	"github.com/donvargax/itos-cc/internal/processes"
)

// coverageCommand runs a coverage command crap or mutation measures with in
// a process group owner owns, as a mutant's command runs (ADR-0021), which
// an interrupt stops and kills (decision 38). It gives the command's end
// as exec.Cmd.Run gives it, so the runner words a failure as it always
// has, and a group whose cleanup did not finish as the coverage domain's
// CleanupFailed, naming the group and the command.
func coverageCommand(owner *processes.OwnedExecutor) covrun.CommandExecutor {
	return func(ctx context.Context, cmd *exec.Cmd) error {
		r, err := owner.Command(ctx, cmd)
		var cleanup *processes.CleanupFailed
		switch {
		case errors.As(err, &cleanup):
			return covport.CleanupFailed{ProcessGroup: cleanup.ProcessGroup, Command: cleanup.Command, Reason: cleanup.Error()}
		case err != nil:
			return err
		case r.Cancelled || ctx.Err() != nil && !r.Passed:
			return fmt.Errorf("%s stopped: %w", cmd.Path, context.Cause(ctx))
		case !r.Passed && cmd.ProcessState != nil:
			return &exec.ExitError{ProcessState: cmd.ProcessState}
		case !r.Passed:
			return fmt.Errorf("%s exited %d", cmd.Path, r.ExitCode)
		}
		return nil
	}
}

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
		return measureProblem(failure), true
	}
	return nil, false
}

// gapProblem is m's problem, by its gap: a missing tool or report is the
// environment, tests that measured nothing say no, an unreadable
// --coverage-report is a usage error, a command whose owned process group
// did not end is coverage.cleanup-failed, the internal exit naming the group
// and the command (ADR-0021), and a measurement an interrupt stopped, its
// cause cli.ErrInterrupted, is the stop (decision 38); a measurement
// anything else stopped is the internal failure it would be. Each names the build root, language
// and report m names. None has a default; a gap with no case here is the
// 100 of a bug.
func gapProblem(m coverage.Unmeasured) *cli.Problem {
	p := cli.Internal(fmt.Errorf("coverage gap %v: %s", m.Cause, m))
	switch cause := m.Cause.(type) {
	case covport.ToolMissing:
		p = cli.NewProblem(cli.CodeEnvironment, "coverage.tool-missing", "no coverage for "+m.String(), "Install what it names, or measure with --coverage-command.")
	case covport.MeasuredNothing:
		p = cli.NewProblem(cli.CodeNo, "coverage.measured-nothing", "no coverage for "+m.String(), "Run the tests and fix them, then run itos-cc again.")
	case covport.NoReport:
		p = cli.NewProblem(cli.CodeEnvironment, "coverage.no-report", "no coverage for "+m.String(), "Run without --use-existing-coverage, or write the report first.")
	case covport.Unreadable:
		p = cli.NewProblem(cli.CodeUsage, "coverage.report-unreadable", "no coverage for "+m.String(), "Check the --coverage-report path and its format.")
	case covport.CleanupFailed:
		p = cleanupProblem(cause)
	case covport.Cancelled:
		if errors.Is(cause.Cause, cli.ErrInterrupted) {
			p = cli.Interrupted()
		}
	}
	if m.Dir != "" {
		p = p.With("dir", m.Dir)
	}
	if m.Language != "" {
		p = p.With("language", m.Language)
	}
	if m.Report != "" {
		p = p.With("report", m.Report)
	}
	return p
}

// measureProblem is failure's problems: a stopped measurement is its
// gap's, Cancelled, the stop, and beside it the cleanup of the command it
// stopped, when that did not finish; a run folder that could not be made is
// the internal 100, the operating system's text whole, as is listed tests'
// failed coverage run, which mutation words before it reaches here. None has a default;
// a kind with no case here is the 100 of a bug.
func measureProblem(failure covport.Failure) []*cli.Problem {
	p := []*cli.Problem{cli.Internal(failure)}
	switch e := failure.(type) {
	case covport.Cancelled:
		p = []*cli.Problem{gapProblem(coverage.Unmeasured{Cause: e})}
		if e.Cleanup != nil {
			p = append(p, cleanupProblem(*e.Cleanup))
		}
	case covport.Unwritable:
		p = []*cli.Problem{cli.Internal(e)}
	case covport.TestsFailed:
		// Only mutation's listed tests fail so, and its slice words them
		// as tests.coverage-failed before any classifier sees them.
		p = []*cli.Problem{cli.Internal(e)}
	}
	return p
}

// cleanupProblem is coverage.cleanup-failed for c, a coverage command's
// owned process group whose cleanup did not finish.
func cleanupProblem(c covport.CleanupFailed) *cli.Problem {
	return cli.NewProblem(cli.CodeInternal, "coverage.cleanup-failed",
		fmt.Sprintf("the owned process group %d of %q did not end by its cleanup deadline: %s", c.ProcessGroup, c.Command, c.Reason),
		"Stop what it left running; a command that detaches from its process group or session is not supported.").
		With("process_group", c.ProcessGroup).With("command", c.Command)
}
