package domain_test

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	aggregate "github.com/donvargax/itos-cc/internal/coverage/port/porttest"
	covtest "github.com/donvargax/itos-cc/internal/coverage/porttest"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// ownCacheFixture is listedCacheFixture's Go module, its tools measuring
// a.go's line 3 and b.go's nothing.
type ownCacheFixture struct {
	*listedCacheFixture
	measurer *covtest.Measurer
}

func newOwnCacheFixture(t *testing.T) *ownCacheFixture {
	f := newListedCacheFixture(t)
	m := &covtest.Measurer{Aggregate: aggregate.Measurer{Dir: f.root, Files: f.inputs.Files},
		Declarations: map[coverage.Unloaded]covtest.Measurement{{Language: "go", Dir: f.root}: {Language: "go",
			Report: "mode: set\nexample.com/w/a.go:3.14,3.24 1 1\nexample.com/w/b.go:3.14,3.24 1 0\n"}}}
	return &ownCacheFixture{listedCacheFixture: f, measurer: m}
}

func (f *ownCacheFixture) request(sources ...string) covport.Request {
	req := covport.Request{Scope: covport.OwnTests}
	for _, s := range sources {
		req.Sources = append(req.Sources, filepath.Join(f.root, s))
	}
	return req
}

// measure measures sources as a complete run does, through reuse, and
// gives what the run said and whether a coverage command ran.
func (f *ownCacheFixture) measure(t *testing.T, cache *listedCache, write bool, sources ...string) (*coverage.Report, []mutants.RunEvent, bool) {
	t.Helper()
	events := &runEvents{}
	reuse := &mutants.ListedReuse{Cache: cache, Write: write, Progress: events}
	req := f.request(sources...)
	plans := reuse.KeyPlans(t.Context(), f.measurer, req, f.inputs, f.inputs, f.root, nil)
	entered := len(f.measurer.Entered)
	r, failure := mutants.MeasureReusing(t.Context(), f.measurer, req, nil, reuse, nil, plans)
	if failure != nil {
		t.Fatal(failure)
	}
	return r, events.values, len(f.measurer.Entered) > entered
}

// A plan's own coverage a complete run measured is kept, and a later run
// of the same inputs and sources takes it from the cache, running no
// coverage command, its report answering what the measured one answered
// (slice-70).
func TestAPlansOwnCoverageIsKeptAndServedWhileItsInputsHold(t *testing.T) {
	f := newOwnCacheFixture(t)
	cache := &listedCache{}
	a := filepath.Join(f.root, "a.go")
	measured, said, ran := f.measure(t, cache, true, "a.go")
	if !ran || len(said) != 0 || len(cache.ownSaved) != 1 {
		t.Fatalf("an empty cache: ran %v, said %v, saved %v", ran, said, cache.ownSaved)
	}
	entry := cache.own[cache.ownSaved[0]]
	if entry.Language != "go" || entry.Dir != "." || !slices.Equal(entry.Sources, []string{"a.go"}) || len(entry.Files["a.go"]) != 1 ||
		!slices.Equal(entry.Proven, []string{"a.go"}) || entry.Inputs["a.go"] == "" {
		t.Fatalf("kept %+v, want a.go's coverage, its paths from the root", entry)
	}
	served, said, ran := f.measure(t, cache, true, "a.go")
	if ran || len(said) != 1 || said[0].Kind != mutants.OwnCoverageReused || said[0].File != f.root {
		t.Fatalf("an unchanged plan: ran %v, said %+v", ran, said)
	}
	if !served.Measures("go") || !served.Proves(a) || !slices.Equal(served.GoBlocks(a), measured.GoBlocks(a)) {
		t.Fatalf("the served report lost what the measured one said of a.go: %+v", served.GoBlocks(a))
	}
	if len(cache.ownSaved) != 1 {
		t.Errorf("a served plan was kept again: %v", cache.ownSaved)
	}
}

// An entry that does not serve the run says why, naming the plan's build
// root, and the run measures and replaces it: an input changed, a source
// it did not measure, one it measured besides the run's, or one it cannot
// read; a stop says nothing.
func TestAPlansCacheThatDoesNotServeTheRunSaysWhy(t *testing.T) {
	f := newOwnCacheFixture(t)
	cache := &listedCache{}
	unserved := func(sources ...string) string {
		t.Helper()
		_, said, ran := f.measure(t, cache, true, sources...)
		if !ran || len(said) != 1 || said[0].Kind != mutants.OwnCoverageUnserved || said[0].File != f.root {
			t.Fatalf("ran %v, said %+v, want why the plan's entry does not serve", ran, said)
		}
		return said[0].Failure.Error()
	}
	f.measure(t, cache, true, "a.go")
	if got := unserved("a.go", "b.go"); got != "it measured no b.go" {
		t.Errorf("a source it lacks: %q", got)
	}
	if got := unserved("a.go"); got != "it also measured b.go" {
		t.Errorf("a source besides the run's: %q", got)
	}
	f.inputs.Files[filepath.Join(f.root, "b.go")] = "package w\n\nfunc B() int { return 3 }\n"
	if got := unserved("a.go"); got != "b.go changed" {
		t.Errorf("a changed input: %q", got)
	}
	cache.ownFailure = &snapport.Unreadable{Reason: "it does not decode"}
	if got := unserved("a.go"); got != "it cannot be read" {
		t.Errorf("an unreadable entry: %q", got)
	}
	cache.ownFailure = &snapport.Cancelled{Cause: context.Canceled}
	if _, said, ran := f.measure(t, cache, true, "a.go"); !ran || len(said) != 0 {
		t.Errorf("a stopped read: ran %v, said %+v", ran, said)
	}
}

// Only a run that writes keeps what it measured, and only a measurement
// that succeeded whole.
func TestARunKeepsOnlyACompletePlanMeasurement(t *testing.T) {
	f := newOwnCacheFixture(t)
	cache := &listedCache{}
	f.measure(t, cache, false, "a.go")
	if len(cache.ownSaved) != 0 {
		t.Fatalf("a run that writes nothing kept %v", cache.ownSaved)
	}
	key := mutants.KeyPlan(t.Context(), f.inputs, f.inputs, f.root, coverage.Plan{Language: "go", Dir: f.root, Sources: []string{filepath.Join(f.root, "a.go")}}, false, nil)
	reuse := &mutants.ListedReuse{Cache: cache, Write: true}
	for _, gap := range []func(*coverage.Report){
		func(r *coverage.Report) {
			r.SetMissing(coverage.Unmeasured{Dir: f.root, Language: "go", Cause: covport.MeasuredNothing{}})
		},
		func(r *coverage.Report) {
			r.SetFailed(coverage.Unmeasured{Dir: f.root, Language: "go", Cause: covport.MeasuredNothing{}})
		},
		func(r *coverage.Report) {
			r.SetIntegrationMissing(coverage.Unmeasured{Dir: f.root, Language: "go", Cause: covport.ToolMissing{}})
		},
	} {
		r := coverage.FromPieces(nil)
		gap(r)
		reuse.KeepPlans(t.Context(), []*mutants.PlanKey{key}, r)
	}
	(*mutants.ListedReuse)(nil).KeepPlans(t.Context(), []*mutants.PlanKey{key}, coverage.FromPieces(nil))
	if len(cache.ownSaved) != 0 {
		t.Fatalf("kept a measurement with a gap: %v", cache.ownSaved)
	}
	reuse.KeepPlans(t.Context(), []*mutants.PlanKey{key}, coverage.FromPlan("go", f.root, nil, nil, []string{filepath.Join(f.root, "a.go")}))
	if entry := cache.own[cache.ownSaved[0]]; !slices.Equal(entry.Unloaded, []string{"a.go"}) || len(entry.Files) != 0 {
		t.Fatalf("kept %+v, want a.go unloaded", entry)
	}
}

// No plan is keyed where the cache cannot serve it: no reuse, a request
// of reports, a plan that runs nothing, one of no fingerprints, of an
// input it cannot read, or of a scope strict coverage refuses. A plan of
// its own tests is named apart from its build root's whole suite.
func TestNoKeyServesAPlanTheCacheCannotServe(t *testing.T) {
	f := newOwnCacheFixture(t)
	reuse := &mutants.ListedReuse{Cache: &listedCache{}}
	req := f.request("a.go")
	if (*mutants.ListedReuse)(nil).KeyPlans(t.Context(), f.measurer, req, f.inputs, f.inputs, f.root, nil) != nil {
		t.Error("no reuse keyed a plan")
	}
	for _, raw := range []covport.Request{{Sources: req.Sources, Reports: []string{"c.out"}}, {Sources: req.Sources, Existing: true}} {
		if reuse.KeyPlans(t.Context(), f.measurer, raw, f.inputs, f.inputs, f.root, nil) != nil {
			t.Errorf("a request of reports %+v keyed a plan", raw)
		}
	}
	plan := coverage.Plan{Language: "go", Dir: f.root, Sources: req.Sources}
	if mutants.KeyPlan(t.Context(), f.inputs, nil, f.root, plan, false, nil) != nil {
		t.Error("a key without fingerprints")
	}
	key := mutants.KeyPlan(t.Context(), f.inputs, f.inputs, f.root, plan, false, nil)
	if (*mutants.ListedReuse)(nil).ServePlan(t.Context(), key) != nil || reuse.ServePlan(t.Context(), nil) != nil {
		t.Error("no reuse, or no key, served a plan")
	}
	unreached := plan
	unreached.Unreached = true
	if mutants.KeyPlan(t.Context(), f.inputs, f.inputs, f.root, unreached, false, nil) != nil {
		t.Error("a key of a plan that runs nothing")
	}
	f.inputs.Files[filepath.Join(f.root, "package.json")] = "{}\n"
	ts := filepath.Join(f.root, "src", "a.ts")
	f.inputs.Files[ts] = "export const a = 1\n"
	if mutants.KeyPlan(t.Context(), nil, f.inputs, f.root, coverage.Plan{Language: "typescript", Dir: f.root, Sources: []string{ts}}, false, nil) != nil {
		t.Error("a key of a TypeScript plan whose coverage runner cannot be told")
	}
	f.inputs.Files[filepath.Join(f.root, "go.work")] = "go 1.22\n\nuse .\n"
	if mutants.KeyPlan(t.Context(), f.inputs, f.inputs, f.root, plan, false, nil) != nil {
		t.Error("a key of a module an active go.work holds")
	}
	delete(f.inputs.Files, filepath.Join(f.root, "go.work"))
	cache := &listedCache{}
	own := &mutants.ListedReuse{Cache: cache, Write: true}
	reports := coverage.FromPlan("python", f.root, nil, nil, nil)
	for _, p := range []coverage.Plan{plan, {Language: "go", Dir: f.root, Sources: req.Sources, Tests: []string{"a_test.go"}}} {
		own.KeepPlans(t.Context(), []*mutants.PlanKey{mutants.KeyPlan(t.Context(), f.inputs, f.inputs, f.root, p, false, nil)}, reports)
	}
	if len(cache.ownSaved) != 2 || cache.ownSaved[0] == cache.ownSaved[1] {
		t.Errorf("a plan of its own tests is kept as its build root's: %v", cache.ownSaved)
	}
}

// A run whose every plan the cache serves runs no measurement at all; one
// it does not serve measures, and gives the measurement's failure.
func TestARunTheCacheServesWholeMeasuresNothing(t *testing.T) {
	f := newOwnCacheFixture(t)
	cache := &listedCache{}
	f.measure(t, cache, true, "a.go")
	f.measurer.RunDirectoryFailure = &covport.Unwritable{Reason: "no run folder"}
	if _, said, ran := f.measure(t, cache, true, "a.go"); ran || len(said) != 1 {
		t.Fatalf("a served run measured: ran %v, said %+v", ran, said)
	}
	reuse := &mutants.ListedReuse{Cache: &listedCache{}, Write: true}
	req := f.request("a.go")
	plans := reuse.KeyPlans(t.Context(), f.measurer, req, f.inputs, f.inputs, f.root, nil)
	if r, failure := mutants.MeasureReusing(t.Context(), f.measurer, req, nil, reuse, nil, plans); r != nil || failure == nil {
		t.Fatalf("a failed measurement gave %v, %v", r, failure)
	}
}
