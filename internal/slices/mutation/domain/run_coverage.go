package domain

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	"github.com/donvargax/itos-cc/internal/lang"
)

// CoverageReportRequired is the existing command/report admission failure.
// Presentation owns its usage problem and wording.
type CoverageReportRequired struct{}

func (CoverageReportRequired) Error() string { return "coverage command requires a report" }

// RunMeasurement is the actual coverage request of a mutation run. Configured
// listing is performed lazily before fresh measurement, only for pending work.
type RunMeasurement struct {
	Measurer coverage.MutationMeasurer
	Request  covport.Request
	Lister   TestLister
}

// ListingFailed is the actual unsuccessful configured command, before trials.
// The command-line boundary owns tests.list-failed's wording and exit policy.
type ListingFailed struct {
	Command, Reason string
	ExitCode        int
}

func (f ListingFailed) Error() string { return f.Reason }

// ParseListing retains first-ID-wins, optional filenames and CRLF trimming.
func ParseListing(stdout string) []coverage.Test {
	var listed []coverage.Test
	seen := map[string]bool{}
	for _, line := range strings.Split(stdout, "\n") {
		id, file, _ := strings.Cut(strings.TrimRight(line, "\r"), "\t")
		id, file = strings.TrimSpace(id), filepath.ToSlash(strings.TrimSpace(file))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		listed = append(listed, coverage.Test{ID: id, File: file})
	}
	return listed
}

func ListedTests(ctx context.Context, lister TestLister, command Command) ([]coverage.Test, error) {
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	if lister == nil {
		return nil, errors.New("mutation TestLister binding is missing")
	}
	listed, failure := lister.List(ctx, command)
	if failure != nil {
		return nil, failure
	}
	if listed.Reason != "" || listed.ExitCode != 0 {
		return nil, ListingFailed{Command: command.String(), Reason: listed.Reason, ExitCode: listed.ExitCode}
	}
	return ParseListing(listed.Stdout), nil
}

// Measure performs the bound request. Strict callers keep actual errors;
// ordinary lazy measurement retains the existing best-effort fallback.
func (opt Options) Measure(ctx context.Context, sources []string, strict bool) (*coverage.Report, error) {
	if opt.Coverage != nil {
		return opt.Coverage, nil
	}
	m := opt.Measurement
	if m == nil {
		return nil, nil
	}
	var perTest *coverage.PerTest
	if opt.Listed != nil && opt.Listed.List != "" {
		c := Command{Root: opt.Listed.Root, Dir: opt.Listed.Root, Shell: opt.Listed.List}
		report(opt.Progress, RunEvent{Kind: "listing", Command: c})
		tests, err := ListedTests(ctx, m.Lister, c)
		if err != nil {
			return nil, err
		}
		perTest = &coverage.PerTest{Root: opt.Listed.Root, Tests: tests, Select: opt.Listed.Select}
		var ids []string
		opt.Listed.Files = map[string]string{}
		for _, test := range tests {
			ids = append(ids, test.ID)
			opt.Listed.Files[test.ID] = test.File
		}
		perTest.All = opt.Listed.All(ids)
	}
	if m.Request.Command != "" && len(m.Request.Reports) == 0 {
		err := CoverageReportRequired{}
		if strict {
			return nil, err
		}
		report(opt.Progress, RunEvent{Kind: "coverage-failed", Failure: err})
		return nil, nil
	}
	if m.Measurer == nil {
		return nil, errors.New("mutation coverage capability is not bound")
	}
	req := m.Request
	req.Sources = sources
	value, failure := m.Measurer.MeasureListed(ctx, req, perTest)
	if failure != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		if strict {
			return nil, failure
		}
		report(opt.Progress, RunEvent{Kind: "coverage-failed", Failure: failure})
		return nil, nil
	}
	return value, nil
}

// CoverageInputsFor observes the existing producer/input policy through actual
// bindings, independently of which mutation sites the caller judges.
func CoverageInputsFor(ctx context.Context, tools Toolchain, inputs Fingerprints, source, root string, all, shell bool, support map[string]string) (string, map[string]string, error) {
	if ctx.Err() != nil {
		return "", nil, context.Cause(ctx)
	}
	if inputs == nil {
		return "", nil, errors.New("mutation Fingerprints binding is missing")
	}
	spec := lang.Detect(source)
	if spec == nil {
		return "", nil, errors.New("no strict coverage inputs for its language")
	}
	producer := CoverageProducer(spec.Name, all || shell && spec.Name != "go")
	if spec.Name == "typescript" || spec.Name == "kotlin" {
		if tools == nil {
			return "", nil, errors.New("mutation Toolchain binding is missing")
		}
		runner, failure := tools.CoverageRunner(ctx, source, !all)
		if failure != nil {
			return "", nil, failure
		}
		if spec.Name == "typescript" {
			producer = TypeScriptCoverageProducer(runner, all)
		} else {
			producer = KotlinCoverageProducer(runner, all || shell)
		}
	}
	hashes, failure := inputs.Coverage(ctx, source, root, producer, support)
	return producer, hashes, failure
}
