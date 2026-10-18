// Package coveragetools is the coverage port's adapter (decision 46): it
// measures coverage with the project's own tools, through the top-level
// coverage runner, its process groups and the run folders under
// .metrics/coverage they write their reports in, and reads the reports a
// request names. It returns the coverage domain's Report, built through the
// domain's API, with no mapper between them (decision 47).
package coveragetools

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	covrun "github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/coverage/port"
	"github.com/donvargax/itos-cc/metrics"
)

// Measurer is the coverage port's Measurer over the disk and the project's
// own tools. porttest.MeasurerContract holds it to what the fake does.
type Measurer struct {
	// Log is where it says what it runs and the tools write their output,
	// the writer main gives it, so stdout stays the report.
	Log io.Writer
	// Execute runs each coverage command the tests' own measurement runs,
	// in a process group it owns, which an interrupt stops (ADR-0021,
	// decision 38); nil runs it as it is.
	Execute covrun.CommandExecutor
}

var _ port.Measurer[*coverage.Report] = Measurer{}
var _ coverage.MutationMeasurer = Measurer{}

// Measure gives the report of req, as the port says: the reports it names,
// after its Command when it has one; with none, those an earlier run left
// when it says Existing, and otherwise the tests' own, run now.
func (m Measurer) Measure(ctx context.Context, req port.Request) (*coverage.Report, port.Failure) {
	return m.MeasureListed(ctx, req, nil)
}

// MeasureListed is Measure, and with perTest, when it runs the tests, each
// listed test's coverage too, the report's LineTests, measured in its run
// folder through the unchanged runner.
func (m Measurer) MeasureListed(ctx context.Context, req port.Request, perTest *coverage.PerTest) (*coverage.Report, port.Failure) {
	if ctx.Err() != nil {
		return nil, port.Cancelled{Cause: context.Cause(ctx)}
	}
	if len(req.Reports) > 0 {
		return m.named(ctx, req)
	}
	scope := req.Scope
	if req.AllTests {
		scope = port.AllTests
	}
	out := filepath.Join(metrics.Dir(), "coverage")
	if req.Existing {
		return covrun.Existing(ctx, covrun.Plans(ctx, req.Sources, out, scope, req.Reach), req.Sources, m.Log), nil
	}
	return m.fresh(ctx, req, out, scope, perTest)
}

// named is the report of the reports req names, read once its Command, if
// it has one, has run: a report the command was to write and did not, or
// wrote one that cannot be read, measured nothing.
func (m Measurer) named(ctx context.Context, req port.Request) (*coverage.Report, port.Failure) {
	if req.Command == "" {
		return covrun.Files(ctx, req.Reports, req.Sources, m.Log), nil
	}
	_, _ = fmt.Fprintf(m.Log, "itos-cc: coverage $ %s\n", req.Command)
	if err := covrun.Shell(req.Command, m.Log); err != nil {
		_, _ = fmt.Fprintf(m.Log, "itos-cc: coverage: %v\n", err)
	}
	report := covrun.Files(ctx, req.Reports, req.Sources, m.Log)
	missing := slices.Clone(report.Missing())
	for i := range missing {
		if _, unreadable := missing[i].Cause.(port.Unreadable); unreadable {
			missing[i].Cause, missing[i].Reason = port.MeasuredNothing{}, "--coverage-command wrote no readable report: "+missing[i].Reason
		}
	}
	report.SetMissing(missing...)
	return report, nil
}

// fresh is the report of req's sources, measured now by their tests in a
// run folder of its own under out, so runs at the same time, such as an
// agent's and a commit hook's, never read each other's; it then keeps the
// run's reports in out for Existing, in place of the last run's. A run the
// context stopped measured what it measured for that reason, not for a
// tool's: it is Cancelled and keeps nothing. A run folder that cannot be
// made is Unwritable.
func (m Measurer) fresh(ctx context.Context, req port.Request, out string, scope port.Scope, perTest *coverage.PerTest) (*coverage.Report, port.Failure) {
	if err := covrun.IgnoreDir(out); err != nil {
		return nil, port.Unwritable{Reason: err.Error()}
	}
	removeAbandoned(out)
	run, err := os.MkdirTemp(out, "run-")
	if err != nil {
		return nil, port.Unwritable{Reason: err.Error()}
	}
	defer func() { _ = os.RemoveAll(run) }()
	report := covrun.Run(ctx, covrun.Plans(ctx, req.Sources, run, scope, req.Reach), req.Sources, m.Log, m.Execute)
	if perTest != nil {
		report.SetTests(covrun.MeasureTests(ctx, *perTest, filepath.Join(run, "tests"), req.Sources, m.Log, m.Execute))
	}
	if ctx.Err() != nil {
		// The command it stopped may have left its owned group's cleanup
		// unfinished, which the stop reports beside itself.
		cancelled := port.Cancelled{Cause: context.Cause(ctx)}
		for _, missing := range report.Missing() {
			if c, ok := missing.Cause.(port.CleanupFailed); ok {
				cancelled.Cleanup = &c
			}
		}
		return nil, cancelled
	}
	keepLatest(run, out)
	return report, nil
}

// removeAbandoned deletes run folders a killed run left behind: any a day
// old, longer than a run takes.
func removeAbandoned(out string) {
	runs, _ := filepath.Glob(filepath.Join(out, "run-*"))
	for _, r := range runs {
		if info, err := os.Stat(r); err == nil && time.Since(info.ModTime()) > 24*time.Hour {
			_ = os.RemoveAll(r)
		}
	}
}

// keepLatest moves each report folder a run wrote into out, replacing the
// one an earlier run left. It is a cache, so failures are ignored.
func keepLatest(run, out string) {
	entries, _ := os.ReadDir(run)
	for _, e := range entries {
		target := filepath.Join(out, e.Name())
		_ = os.RemoveAll(target)
		_ = os.Rename(filepath.Join(run, e.Name()), target)
	}
}

// Inventory preserves the runner's best-effort policy: ordinary collector
// failures are logged, while cancellation stops the caller with its cause.
func (m Measurer) Inventory(ctx context.Context, report *coverage.Report, language string) port.Failure {
	if ctx.Err() != nil {
		return port.Cancelled{Cause: context.Cause(ctx)}
	}
	_, _ = covrun.Inventory(ctx, report, language, m.Log, nil)
	if ctx.Err() != nil {
		return port.Cancelled{Cause: context.Cause(ctx)}
	}
	return nil
}
