package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	mutationslice "github.com/donvargax/itos-cc/internal/slices/mutation"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// Every gap of the coverage port's sealed set has its exit code, rule and
// fix, the message saying what went unmeasured, and the build root,
// language and report it names; an interrupt is 130 and names none.
func TestEachCoverageGapHasItsProblem(t *testing.T) {
	root := coverage.Unmeasured{Dir: "web", Language: "typescript", Reason: "no vitest"}
	report := coverage.Unmeasured{Report: "lcov.info", Reason: "open lcov.info: no such file or directory"}
	for _, c := range []struct {
		gap      covport.Gap
		of       coverage.Unmeasured
		code     int
		rule     string
		fix      string
		subjects map[string]any
	}{
		{covport.ToolMissing{}, root, cli.CodeEnvironment, "coverage.tool-missing", "Install what it names, or measure with --coverage-command.",
			map[string]any{"dir": "web", "language": "typescript"}},
		{covport.MeasuredNothing{}, root, cli.CodeNo, "coverage.measured-nothing", "Run the tests and fix them, then run itos-cc again.",
			map[string]any{"dir": "web", "language": "typescript"}},
		{covport.NoReport{}, root, cli.CodeEnvironment, "coverage.no-report", "Run without --use-existing-coverage, or write the report first.",
			map[string]any{"dir": "web", "language": "typescript"}},
		{covport.Unreadable{}, report, cli.CodeUsage, "coverage.report-unreadable", "Check the --coverage-report path and its format.",
			map[string]any{"report": "lcov.info"}},
	} {
		m := c.of
		m.Cause = c.gap
		problems, ok := classifyCoverage(fmt.Errorf("measuring: %w", m))
		if !ok || len(problems) != 1 {
			t.Fatalf("%v: claimed %t, %d problems; want its one", c.gap, ok, len(problems))
		}
		p := problems[0]
		if p.Code != c.code || p.Rule != c.rule || p.Message != "no coverage for "+m.String() || p.Fix != c.fix {
			t.Errorf("%v: %+v, want %d %s %q", c.gap, p, c.code, c.rule, c.fix)
		}
		for _, key := range []string{"dir", "language", "report"} {
			if got := p.Subject(key); got != c.subjects[key] {
				t.Errorf("%v: %s is %v, want %v", c.gap, key, got, c.subjects[key])
			}
		}
	}
	problems, ok := classifyCoverage(coverage.Unmeasured{Dir: "web", Language: "go", Cause: covport.Cancelled{Cause: cli.ErrInterrupted}, Reason: "stopped"})
	if !ok || len(problems) != 1 || problems[0].Code != cli.CodeInterrupted || problems[0].Rule != "interrupted" {
		t.Errorf("a cancelled root is %+v, %t; want the interrupt", problems, ok)
	}
	// A cancellation no interrupt caused is no interrupt (decision 38).
	problems, ok = classifyCoverage(coverage.Unmeasured{Dir: "web", Language: "go", Cause: covport.Cancelled{Cause: context.Canceled}, Reason: "stopped"})
	if !ok || len(problems) != 1 || problems[0].Code != cli.CodeInternal {
		t.Errorf("a root another stop cancelled is %+v, %t; want the internal failure", problems, ok)
	}
	// A coverage command whose owned group did not end is
	// coverage.cleanup-failed, the internal exit, naming the group and the
	// command: crap's coverage commands own their groups as mutation's do
	// (ADR-0021), and no scenario can make a cleanup fail in crap's run.
	failed := covport.CleanupFailed{ProcessGroup: 4242, Command: "go test -coverprofile cover.out ./...", Reason: "owned process group 4242 remained active at cleanup deadline"}
	problems, ok = classifyCoverage(fmt.Errorf("measuring: %w", coverage.Unmeasured{Dir: "web", Language: "go", Cause: failed, Reason: "its coverage command's owned process group did not end"}))
	if !ok || len(problems) != 1 {
		t.Fatalf("a failed cleanup: claimed %t, %d problems; want its one", ok, len(problems))
	}
	if p := problems[0]; p.Code != cli.CodeInternal || p.Rule != "coverage.cleanup-failed" || p.Subject("process_group") != 4242 ||
		p.Subject("command") != failed.Command || p.Subject("dir") != "web" || p.Subject("language") != "go" {
		t.Errorf("a failed cleanup is %+v, want coverage.cleanup-failed naming group 4242 and its command", p)
	}
}

// A measurement the port failed whole is its failure's problem: stopped,
// the interrupt, with no subject; a run folder that could not be made, the
// internal 70, the operating system's text whole.
func TestEachMeasureFailureHasItsProblem(t *testing.T) {
	for _, c := range []struct {
		failure covport.Failure
		code    int
		rule    string
		message string
	}{
		{covport.Cancelled{Cause: cli.ErrInterrupted}, cli.CodeInterrupted, "interrupted", "interrupted before it finished"},
		{covport.Unwritable{Reason: "mkdir .metrics/coverage: not a directory"}, cli.CodeInternal, "internal", "mkdir .metrics/coverage: not a directory"},
	} {
		problems, ok := classifyCoverage(fmt.Errorf("measuring: %w", c.failure))
		if !ok || len(problems) != 1 {
			t.Fatalf("%T: claimed %t, %d problems; want its one", c.failure, ok, len(problems))
		}
		if p := problems[0]; p.Code != c.code || p.Rule != c.rule || p.Message != c.message || p.Subject("dir") != nil {
			t.Errorf("%T: %+v, want %d %s %q", c.failure, p, c.code, c.rule, c.message)
		}
	}
	// An interrupt that stopped a command whose owned group then did not
	// end is reported beside that cleanup, and the run exits 130.
	cleanup := &covport.CleanupFailed{ProcessGroup: 77, Command: "go test", Reason: "owned command output did not close before cleanup deadline"}
	problems, ok := classifyCoverage(covport.Cancelled{Cause: cli.ErrInterrupted, Cleanup: cleanup})
	if !ok || len(problems) != 2 || problems[0].Rule != "interrupted" || problems[1].Rule != "coverage.cleanup-failed" ||
		problems[1].Subject("process_group") != 77 || cli.ExitCode(problems) != cli.CodeInterrupted {
		t.Errorf("an interrupt whose cleanup failed is %+v, %t; want interrupted and coverage.cleanup-failed, exit 130", problems, ok)
	}
}

// Neither another domain's failure nor a plain error is the coverage
// domain's to claim.
func TestAnErrorOfNoKindIsNotCoverages(t *testing.T) {
	for _, err := range []error{errors.New("no space"), &snapport.Unwritable{Name: "crap.json", Reason: "full"}} {
		if problems, ok := classifyCoverage(err); ok || problems != nil {
			t.Errorf("classifyCoverage(%v) claimed %v", err, problems)
		}
	}
}

// The slice's GapProblems is a problem for each root or report a report
// did not measure, in its order, through classifyCoverage, which cmd hands
// the UI, each root's directory from the working directory.
func TestUnmeasuredClassifiesEachGap(t *testing.T) {
	ui := stdUI()
	problems := mutationslice.GapProblems(ui, []coverage.Unmeasured{
		{Dir: absOrSame("web"), Language: "typescript", Cause: covport.ToolMissing{}, Reason: "no vitest"},
		{Report: "lcov.info", Cause: covport.Unreadable{}, Reason: "gone"},
	})
	if len(problems) != 2 || problems[0].Rule != "coverage.tool-missing" || problems[0].Subject("dir") != "web" ||
		problems[1].Rule != "coverage.report-unreadable" {
		t.Errorf("unmeasured = %+v, want tool-missing for web, then report-unreadable", problems)
	}
}
