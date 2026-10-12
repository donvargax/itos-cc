package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/metrics"
	"github.com/donvargax/itos-cc/project"
)

// coverageFlags say how a command gets coverage.
var coverageFlags = []flagSpec{
	sw("no-coverage", "skip coverage; complexity only"),
	sw("use-existing-coverage", "read reports already on disk instead of running tests"),
	sw("all-tests", "run the whole test suite, integration and end-to-end tests included"),
	opt("coverage-command", stringFlag, "CMD", "", "run this shell command instead of the per-language commands (with --coverage-report)"),
	{name: "coverage-report", typ: stringFlag, arg: "FILE", repeat: true, help: "read this LCOV, Go cover profile, or JaCoCo XML report"},
}

// loadCoverage produces coverage for sources under ctx, or nil with
// --no-coverage.
// scope is the tests that run without --all-tests. With perTest, when the
// coverage commands run, it also measures each listed test's coverage, in
// the run's own directory. Progress and test output go to log so stdout
// stays the report. reach, in OwnTests, is the test files that reach each
// source, which measure Python's and Kotlin's (coverage.Plans); nil
// measures them by the whole suite.
func loadCoverage(ctx context.Context, in *invocation, sources []string, scope coverage.Scope, log io.Writer, perTest *coverage.PerTest, reach coverage.Reach) (*coverage.Report, error) {
	command, reports := in.str("coverage-command"), in.strs("coverage-report")
	switch {
	case in.set("no-coverage"):
		return nil, nil
	case command != "":
		if len(reports) == 0 {
			return nil, cli.NewProblem(cli.CodeUsage, "coverage.command-needs-report", "--coverage-command needs --coverage-report to say where its report lands",
				"Add --coverage-report FILE.")
		}
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		fmt.Fprintf(log, "itos-cc: coverage $ %s\n", command)
		if err := shell(command, log); err != nil {
			fmt.Fprintf(log, "itos-cc: coverage: %v\n", err)
		}
		report := coverage.Files(reports, sources, log)
		// The command was to write the report, so a report it did not
		// write means it measured nothing.
		missing := report.Missing()
		for i := range missing {
			if missing[i].Cause == coverage.Unreadable {
				missing[i].Cause, missing[i].Reason = coverage.MeasuredNothing, "--coverage-command wrote no readable report: "+missing[i].Reason
			}
		}
		return report, nil
	case len(reports) > 0:
		return coverage.Files(reports, sources, log), nil
	}
	out := filepath.Join(metrics.Dir(), "coverage")
	if in.set("all-tests") {
		scope = coverage.AllTests
	}
	if in.set("use-existing-coverage") {
		return coverage.Existing(coverage.Plans(ctx, sources, out, scope, reach), sources, log), nil
	}
	if err := coverage.IgnoreDir(out); err != nil {
		return nil, err
	}
	// Each run writes its reports in a directory of its own, so runs at the
	// same time, such as an agent's and a commit hook's, never read each
	// other's, then keeps only the latest for --use-existing-coverage.
	removeAbandoned(out)
	run, err := os.MkdirTemp(out, "run-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(run)
	report := coverage.Run(ctx, coverage.Plans(ctx, sources, run, scope, reach), sources, log)
	if perTest != nil {
		report.SetTests(coverage.MeasureTests(ctx, *perTest, filepath.Join(run, "tests"), sources, log))
	}
	// A stopped run measured what it measured for that reason, not for a
	// tool's: it is reported as the stop, and keeps no reports.
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	keepLatest(run, out)
	return report, nil
}

// unmeasured is a problem for each build root or report coverage did not
// measure: a missing tool or report is the environment, tests that measured
// nothing say no, and an unreadable --coverage-report is a usage error.
func unmeasured(report *coverage.Report) []*cli.Problem {
	return unmeasuredProblems(relativeUnmeasured(report.Missing()))
}

// unmeasuredProblems is unmeasured of the build roots and reports ms.
func unmeasuredProblems(ms []coverage.Unmeasured) []*cli.Problem {
	var out []*cli.Problem
	for _, m := range ms {
		var p *cli.Problem
		switch m.Cause {
		case coverage.ToolMissing:
			p = cli.NewProblem(cli.CodeEnvironment, "coverage.tool-missing", "no coverage for "+m.String(), "Install what it names, or measure with --coverage-command.")
		case coverage.MeasuredNothing:
			p = cli.NewProblem(cli.CodeNo, "coverage.measured-nothing", "no coverage for "+m.String(), "Run the tests and fix them, then run itos-cc again.")
		case coverage.NoReport:
			p = cli.NewProblem(cli.CodeEnvironment, "coverage.no-report", "no coverage for "+m.String(), "Run without --use-existing-coverage, or write the report first.")
		default:
			p = cli.NewProblem(cli.CodeUsage, "coverage.report-unreadable", "no coverage for "+m.String(), "Check the --coverage-report path and its format.")
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
		out = append(out, p)
	}
	return out
}

// relativeUnmeasured is ms, a report's Missing, with directories relative
// to the working directory, as every other path itos-cc prints is.
func relativeUnmeasured(ms []coverage.Unmeasured) []coverage.Unmeasured {
	out := []coverage.Unmeasured{}
	for _, m := range ms {
		if m.Dir != "" {
			m.Dir = project.Rel(m.Dir)
		}
		out = append(out, m)
	}
	return out
}

// removeAbandoned deletes run directories a killed run left behind: any a
// day old, longer than a run takes.
func removeAbandoned(out string) {
	runs, _ := filepath.Glob(filepath.Join(out, "run-*"))
	for _, r := range runs {
		if info, err := os.Stat(r); err == nil && time.Since(info.ModTime()) > 24*time.Hour {
			os.RemoveAll(r)
		}
	}
}

// keepLatest moves each report directory a run wrote into out, replacing the
// one an earlier run left. It is a cache, so failures are ignored.
func keepLatest(run, out string) {
	entries, _ := os.ReadDir(run)
	for _, e := range entries {
		target := filepath.Join(out, e.Name())
		os.RemoveAll(target)
		os.Rename(filepath.Join(run, e.Name()), target)
	}
}

func absOrSame(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
