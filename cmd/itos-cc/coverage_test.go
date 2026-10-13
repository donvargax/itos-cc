package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
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
	problems, ok := classifyCoverage(coverage.Unmeasured{Dir: "web", Language: "go", Cause: covport.Cancelled{}, Reason: "stopped"})
	if !ok || len(problems) != 1 || problems[0].Code != cli.CodeInterrupted || problems[0].Rule != "interrupted" {
		t.Errorf("a cancelled root is %+v, %t; want the interrupt", problems, ok)
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
		{covport.Cancelled{}, cli.CodeInterrupted, "interrupted", "interrupted before it finished"},
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

// unmeasured is a problem for each root or report a report did not
// measure, in its order, through classifyCoverage, each root's directory
// from the working directory.
func TestUnmeasuredClassifiesEachGap(t *testing.T) {
	problems := unmeasured([]coverage.Unmeasured{
		{Dir: absOrSame("web"), Language: "typescript", Cause: covport.ToolMissing{}, Reason: "no vitest"},
		{Report: "lcov.info", Cause: covport.Unreadable{}, Reason: "gone"},
	})
	if len(problems) != 2 || problems[0].Rule != "coverage.tool-missing" || problems[0].Subject("dir") != "web" ||
		problems[1].Rule != "coverage.report-unreadable" {
		t.Errorf("unmeasured = %+v, want tool-missing for web, then report-unreadable", problems)
	}
}
