package domain

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"

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
	// Reuse, when set, serves the listed tests' coverage from the cache,
	// and keeps what the run measures of them there (MeasureReusing).
	Reuse *ListedReuse
}

// ListingFailed is the actual unsuccessful configured command, before trials.
// The command-line boundary owns tests.list-failed's wording and exit policy.
type ListingFailed struct {
	Command, Reason string
	ExitCode        int
}

func (f ListingFailed) Error() string { return f.Reason }

// CoverageRunFailed is the listed tests' coverage run that failed without
// any mutant, so no mutant is judged: Command is the line that failed,
// ExitCode its exit code, -1 for none, Reason the runner's text, and
// Cleanup the owned process group of the command whose cleanup did not
// finish, nil for none. The command-line boundary owns
// tests.coverage-failed's wording and exit policy.
type CoverageRunFailed struct {
	Command, Reason string
	ExitCode        int
	Cleanup         *covport.CleanupFailed
}

func (f CoverageRunFailed) Error() string { return f.Reason }

// MeasureFailure is failure as the run fails with it: the listed tests'
// failed coverage run is CoverageRunFailed, in strict and ordinary
// measurement alike; any other failure is itself.
func MeasureFailure(failure covport.Failure) error {
	if failed, ok := failure.(covport.TestsFailed); ok {
		return CoverageRunFailed{Command: failed.Command, Reason: failed.Reason, ExitCode: failed.ExitCode, Cleanup: failed.Cleanup}
	}
	return failure
}

// ParseListing retains first-ID-wins, optional filenames and CRLF trimming.
func ParseListing(stdout string) []coverage.Test {
	return ParseInventory(stdout).Tests
}

// ListedInventory is what the list command printed: each test, by its ID,
// with its file, "" for none, and the fingerprint of each test listed with
// one, by its ID.
type ListedInventory struct {
	Tests []coverage.Test
	// Fingerprints is the third column of a test's line, opaque text the
	// lister computes from the test's own text, by the test's ID; a test
	// listed without one has none.
	Fingerprints map[string]string
}

// fingerprint is the fingerprint the inventory lists id with, "" when it
// lists id without one or does not list it.
func (inv ListedInventory) fingerprint(id string) string {
	return inv.Fingerprints[id]
}

// ParseInventory reads the list command's output: a test a line, its ID,
// then, after a tab, optionally its file, and after another tab, optionally
// its fingerprint. The first line of an ID wins, and CR line ends are
// trimmed.
func ParseInventory(stdout string) ListedInventory {
	var inv ListedInventory
	seen := map[string]bool{}
	for _, line := range strings.Split(stdout, "\n") {
		id, rest, _ := strings.Cut(strings.TrimRight(line, "\r"), "\t")
		file, fingerprint, _ := strings.Cut(rest, "\t")
		id, file, fingerprint = strings.TrimSpace(id), filepath.ToSlash(strings.TrimSpace(file)), strings.TrimSpace(fingerprint)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		inv.Tests = append(inv.Tests, coverage.Test{ID: id, File: file})
		if fingerprint != "" {
			if inv.Fingerprints == nil {
				inv.Fingerprints = map[string]string{}
			}
			inv.Fingerprints[id] = fingerprint
		}
	}
	return inv
}

// ListedTests runs command through lister and reads the tests it prints
// (ParseInventory). A command that fails is ListingFailed.
func ListedTests(ctx context.Context, lister TestLister, command Command) (ListedInventory, error) {
	if ctx.Err() != nil {
		return ListedInventory{}, context.Cause(ctx)
	}
	if lister == nil {
		return ListedInventory{}, errors.New("mutation TestLister binding is missing")
	}
	listed, failure := lister.List(ctx, command)
	if failure != nil {
		return ListedInventory{}, failure
	}
	if listed.Reason != "" || listed.ExitCode != 0 {
		return ListedInventory{}, ListingFailed{Command: command.String(), Reason: listed.Reason, ExitCode: listed.ExitCode}
	}
	return ParseInventory(listed.Stdout), nil
}

// ListOnce runs one command's list command at most once, when first asked,
// so a run's coverage and the freshness of kills by tests listed with a
// fingerprint read the same listing. A failure is kept as the listing. It
// is safe for concurrent use.
type ListOnce struct {
	Lister  TestLister
	Command Command
	mu      sync.Mutex
	ran     bool
	value   ListedInventory
	err     error
}

// NewListOnce lists by running the shell line list at root through lister.
func NewListOnce(lister TestLister, root, list string) *ListOnce {
	return &ListOnce{Lister: lister, Command: Command{Root: root, Dir: root, Shell: list}}
}

// List is the listing, running the list command, and reporting it to
// progress, the first time it is asked for.
func (l *ListOnce) List(ctx context.Context, progress Progress) (ListedInventory, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.ran {
		report(progress, RunEvent{Kind: "listing", Command: l.Command})
		l.value, l.err = ListedTests(ctx, l.Lister, l.Command)
		l.ran = true
	}
	return l.value, l.err
}

// Listed is the listing when the list command ran and succeeded, and
// whether it did.
func (l *ListOnce) Listed() (ListedInventory, bool) {
	if l == nil {
		return ListedInventory{}, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.value, l.ran && l.err == nil
}

// Measure performs the bound request. Strict callers keep actual errors;
// ordinary lazy measurement retains the existing best-effort fallback, but
// for listed tests whose coverage run failed, which judge nothing.
func (opt Options) Measure(ctx context.Context, sources []string, strict bool) (*coverage.Report, error) {
	if opt.Coverage != nil {
		return opt.Coverage, nil
	}
	m := opt.Measurement
	if m == nil {
		return nil, nil
	}
	var perTest *coverage.PerTest
	var key *ListedKey
	if opt.Listed != nil && opt.Listed.List != "" {
		listing := opt.Listing
		if listing == nil {
			listing = NewListOnce(m.Lister, opt.Listed.Root, opt.Listed.List)
		}
		inv, err := listing.List(ctx, opt.Progress)
		if err != nil {
			return nil, err
		}
		perTest = &coverage.PerTest{Root: opt.Listed.Root, Tests: inv.Tests, Select: opt.Listed.Select}
		var ids []string
		opt.Listed.Files = map[string]string{}
		for _, test := range inv.Tests {
			ids = append(ids, test.ID)
			opt.Listed.Files[test.ID] = test.File
		}
		perTest.All = opt.Listed.All(ids)
		if m.Reuse != nil {
			key = KeyListed(ctx, opt.Toolchain, opt.Fingerprints, opt.Root, sources, inv, opt.Support)
		}
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
	value, failure := MeasureReusing(ctx, m.Measurer, req, perTest, m.Reuse, key)
	if failure != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		if failed, ok := MeasureFailure(failure).(CoverageRunFailed); ok {
			return nil, failed
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
