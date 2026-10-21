package domain_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	aggregate "github.com/donvargax/itos-cc/internal/coverage/port/porttest"
	covtest "github.com/donvargax/itos-cc/internal/coverage/porttest"
	"github.com/donvargax/itos-cc/internal/selection"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

type runEvents struct{ values []mutants.RunEvent }

func (e *runEvents) Report(value mutants.RunEvent) { e.values = append(e.values, value) }
func listingFixture(t *testing.T) (*runFixture, *porttest.Lister, *covtest.Measurer, *runEvents) {
	t.Helper()
	f := newRunFixture(t)
	lister := &porttest.Lister{Listings: map[string]map[string]porttest.Listing{f.root: {"fixture-list": {Stdout: "first\tboard_test.go\nfirst\twrong.go\nsecond\n"}}}}
	measurer := &covtest.Measurer{Aggregate: aggregate.Measurer{Dir: f.root, Files: f.files}}
	events := &runEvents{}
	f.options.Progress = events
	f.options.Listed = &mutants.Listed{Root: f.root, List: "fixture-list", Select: func(ids []string) string { return "select " + strings.Join(ids, " ") }, All: func(ids []string) string { return "all " + strings.Join(ids, " ") }}
	f.options.Measurement = &mutants.RunMeasurement{Measurer: measurer, Lister: lister}
	return f, lister, measurer, events
}
func TestRunListsBeforeCoverageAdmissionAndKeepsFailurePolicies(t *testing.T) {
	t.Run("failed listing precedes missing report", func(t *testing.T) {
		f, lister, m, events := listingFixture(t)
		lister.Listings[f.root]["fixture-list"] = porttest.Listing{ExitCode: 7, Reason: "exit status 7"}
		f.options.Measurement.Request.Command = "actual coverage override"
		_, err := mutants.Run(t.Context(), []string{f.path}, f.options)
		var failed mutants.ListingFailed
		if !errors.As(err, &failed) || failed.ExitCode != 7 || failed.Command != "fixture-list" {
			t.Fatalf("listing precedence: %v", err)
		}
		if len(f.workers.Opened) != 0 || len(f.snapshots.Written) != 0 || len(m.Entered) != 0 || len(events.values) != 1 || events.values[0].Kind != "listing" {
			t.Fatalf("work after list refusal: %v %+v", m.Entered, events.values)
		}
	})
	t.Run("successful listing precedes ordinary missing-report fallback", func(t *testing.T) {
		f, _, m, events := listingFixture(t)
		f.options.Measurement.Request.Command = "override"
		r := f.run(t)
		if r[0].Ran == 0 || len(f.snapshots.Written) != 1 || !reflect.DeepEqual(f.options.Listed.Files, map[string]string{"first": "board_test.go", "second": ""}) {
			t.Fatalf("ordinary fallback/list inventory: %+v %v", r, f.options.Listed.Files)
		}
		if len(m.Entered) != 0 || len(events.values) < 2 || events.values[0].Kind != "listing" || events.values[1].Kind != "coverage-failed" {
			t.Fatalf("actual lazy ordering: %+v", events.values)
		}
		var required mutants.CoverageReportRequired
		if !errors.As(events.values[1].Failure, &required) {
			t.Fatal("fallback lost original admission kind")
		}
		before := len(events.values)
		// Reused/no-pending results need neither measurement nor the list binding.
		f.options.Measurement.Lister = nil
		f.options.Measurement.Measurer = nil
		cached := f.run(t)
		if cached[0].Ran != 0 || cached[0].Reused != r[0].Ran {
			t.Fatalf("cache: %+v", cached)
		}
		for _, event := range events.values[before:] {
			if event.Kind == "listing" || event.Kind == "coverage-failed" {
				t.Fatalf("unnecessary cached observation: %+v", event)
			}
		}
	})
	t.Run("strict admission keeps list first and returns usage kind", func(t *testing.T) {
		f, lister, _, _ := listingFixture(t)
		f.options.Measurement.Request.Command = "override"
		lister.Listings[f.root]["fixture-list"] = porttest.Listing{ExitCode: 7, Reason: "exit status 7"}
		_, err := f.options.Measure(t.Context(), []string{f.path}, true)
		var listing mutants.ListingFailed
		if !errors.As(err, &listing) {
			t.Fatalf("strict list precedence: %v", err)
		}
		lister.Listings[f.root]["fixture-list"] = porttest.Listing{Stdout: "actual\tboard_test.go\n"}
		_, err = f.options.Measure(t.Context(), []string{f.path}, true)
		var required mutants.CoverageReportRequired
		if !errors.As(err, &required) || f.options.Listed.Files["actual"] != "board_test.go" {
			t.Fatalf("strict report admission: %v %v", err, f.options.Listed.Files)
		}
	})
	t.Run("collection failure is ordinary fallback but strict fatal", func(t *testing.T) {
		f, _, m, events := listingFixture(t)
		m.Declarations = map[coverage.Unloaded]covtest.Measurement{}
		m.RunDirectoryFailure = &covport.Unwritable{Reason: "read-only directory"}
		r := f.run(t)
		if r[0].Ran == 0 || len(f.snapshots.Written) != 1 {
			t.Fatalf("ordinary error became fatal: %+v", r)
		}
		if len(events.values) < 2 || events.values[1].Kind != "coverage-failed" {
			t.Fatalf("error observation lost: %+v", events.values)
		}
		_, err := f.options.Measure(t.Context(), []string{f.path}, true)
		var failed covport.Unwritable
		if !errors.As(err, &failed) || failed.Reason != "read-only directory" {
			t.Fatalf("strict error: %v", err)
		}
	})
}
func TestRunUsesActualListedCoverageAndSelectionBaseline(t *testing.T) {
	f, _, m, _ := listingFixture(t)
	own := coverage.FromLines("go", map[string][]coverage.Line{f.path: {{Line: 2, Covered: true}}})
	m.Declarations = map[coverage.Unloaded]covtest.Measurement{{Language: "go", Dir: f.root}: {Language: "go", Report: "mode: set\nexample.test/board/board.go:2.25,2.40 1 1\n", Listed: map[string]map[string]*coverage.Report{"go": {"first": own}}}}
	// Independent own tests survive each actual site; listed selection kills it.
	parsed, err := selection.Parse(t.Context(), f.options.Tree, f.inputs, f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer parsed.Close()
	sites := mutants.Sites(parsed)
	for command, facts := range f.workers.Tests[f.root] {
		for _, site := range sites {
			facts.Verdicts = append(facts.Verdicts, porttest.Verdict{Path: "board.go", Site: site, Outcome: mutants.Survived})
		}
		f.workers.Tests[f.root][command] = facts
	}
	f.workers.Tests[f.root]["select first"] = porttest.Tests{}
	r := f.run(t)
	if r[0].Ran != len(sites) || len(r[0].Mutants) == 0 {
		t.Fatalf("listed judgments: %+v", r)
	}
	for _, mu := range r[0].Mutants {
		if mu.Scope != mutants.ScopeListed || mu.Outcome != mutants.Killed || !reflect.DeepEqual(mu.Tests, []string{"first"}) {
			t.Fatalf("actual listed reach: %+v", mu)
		}
	}
	if r[0].Snapshot.Listed["first"] != "board_test.go" || r[0].Snapshot.ListedFiles["board_test.go"] == "" {
		t.Fatalf("actual listed evidence: %+v", r[0].Snapshot)
	}
	if !reflect.DeepEqual(m.Commands, []string{"all first second", "select first", "select second"}) {
		t.Fatalf("actual measurement requested IDs: %v", m.Commands)
	}
}
func TestRunListingBindingIsRequiredOnlyWhenActuallyNeeded(t *testing.T) {
	for _, wrong := range []bool{false, true} {
		f, lister, _, _ := listingFixture(t)
		if wrong {
			delete(lister.Listings, f.root)
		} else {
			f.options.Measurement.Lister = nil
		}
		if _, err := mutants.Run(t.Context(), []string{f.path}, f.options); err == nil || len(f.workers.Opened) != 0 || len(f.snapshots.Written) != 0 {
			t.Fatalf("wrong/missing actual list binding: %v", err)
		}
	}
	f, _, _, _ := listingFixture(t)
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("actual measurement caller stopped")
	cancel(cause)
	if _, err := f.options.Measure(ctx, []string{f.path}, false); !errors.Is(err, cause) {
		t.Fatalf("cancelled lazy measurement: %v", err)
	}
}

// Listed tests whose coverage run failed judge nothing, in ordinary
// measurement as in strict: unlike any other failure, the ordinary branch
// does not go on without listed coverage.
func TestRunJudgesNothingWhenTheListedTestsFailTheirCoverageRun(t *testing.T) {
	for _, strict := range []bool{false, true} {
		f, _, m, events := listingFixture(t)
		m.Declarations = map[coverage.Unloaded]covtest.Measurement{{Language: "go", Dir: f.root}: {Language: "go", Report: "mode: set\nexample.test/board/board.go:2.25,2.40 1 1\n",
			Listed: map[string]map[string]*coverage.Report{"go": {}}, ListedFailed: true, ListedExit: 3}}
		_, err := f.options.Measure(t.Context(), []string{f.path}, strict)
		var failed mutants.CoverageRunFailed
		if !errors.As(err, &failed) || failed.Command != "all first second" || failed.ExitCode != 3 || failed.Reason != "exit status 3" {
			t.Fatalf("strict %v: %v", strict, err)
		}
		for _, event := range events.values {
			if event.Kind == "coverage-failed" {
				t.Fatalf("strict %v went on without listed coverage: %+v", strict, events.values)
			}
		}
	}
	f, _, m, _ := listingFixture(t)
	m.Declarations = map[coverage.Unloaded]covtest.Measurement{{Language: "go", Dir: f.root}: {Language: "go", Report: "mode: set\n",
		Listed: map[string]map[string]*coverage.Report{"go": {}}, ListedFailed: true}}
	_, err := mutants.Run(t.Context(), []string{f.path}, f.options)
	var failed mutants.CoverageRunFailed
	if !errors.As(err, &failed) || failed.ExitCode != 1 || len(f.workers.Opened) != 0 || len(f.snapshots.Written) != 0 {
		t.Fatalf("run after a failed coverage run: %v, opened %v, written %v", err, f.workers.Opened, f.snapshots.Written)
	}
	if other := mutants.MeasureFailure(covport.Unwritable{Reason: "read-only"}); !errors.Is(other, covport.Unwritable{Reason: "read-only"}) {
		t.Fatalf("another failure changed: %v", other)
	}
}
