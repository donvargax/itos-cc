package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
)

// loadCoverage produces coverage for sources under ctx, or nil with
// --no-coverage, through the invocation's mutation coverage capability.
// scope is the tests that run without --all-tests. With perTest, when the
// coverage commands run, it also measures each listed test's coverage, in
// the run's own folder. The bound adapter owns progress and test output.
// reach, in OwnTests, is the test files that reach each
// source, which measure Python's and Kotlin's; nil measures
// them by the whole suite. A stopped run is reported as ctx's cause, as
// mutation reports a stop.
func loadCoverage(ctx context.Context, in *invocation, sources []string, scope covport.Scope, perTest *coverage.PerTest, reach covport.Reach) (*coverage.Report, error) {
	req, measure, err := coverageRequest(in, sources, scope)
	if !measure || err != nil {
		return nil, err
	}
	req.Reach = reach
	if in.adapters.MutationCoverage == nil {
		return nil, fmt.Errorf("mutation coverage capability is not bound")
	}
	report, failure := in.adapters.MutationCoverage.MeasureListed(ctx, req, perTest)
	if failure != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		return nil, failure
	}
	return report, nil
}

// coverageRequest is what in's coverage flags ask the coverage port to
// measure of sources, scope the tests that run without --all-tests, and
// whether to measure at all: not with --no-coverage. --coverage-command
// without --coverage-report, which says where its report lands, is a usage
// problem.
func coverageRequest(in *invocation, sources []string, scope covport.Scope) (covport.Request, bool, error) {
	command, reports := in.str("coverage-command"), in.strs("coverage-report")
	switch {
	case in.set("no-coverage"):
		return covport.Request{}, false, nil
	case command != "" && len(reports) == 0:
		return covport.Request{}, false, cli.CommandNeedsReport()
	}
	return covport.Request{Sources: sources, Reports: reports, Command: command, Existing: in.set("use-existing-coverage"),
		AllTests: in.set("all-tests"), Scope: scope}, true, nil
}

func absOrSame(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
