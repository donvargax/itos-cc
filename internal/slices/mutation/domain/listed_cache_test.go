package domain_test

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// listedCache is a stub of the cache: the entry it holds, or the failure
// reading it gives, and each entry saved.
type listedCache struct {
	entry   *mutants.ListedCoverage
	failure snapport.Failure
	saved   []mutants.ListedCoverage
}

func (c *listedCache) Listed(context.Context) (*mutants.ListedCoverage, snapport.Failure) {
	return c.entry, c.failure
}

func (c *listedCache) SaveListed(_ context.Context, entry mutants.ListedCoverage) snapport.Failure {
	c.saved = append(c.saved, entry)
	c.entry = &entry
	return nil
}

// listedCacheFixture is a Go module of two files at root, its listing of
// one test defined in a.feature.
type listedCacheFixture struct {
	root   string
	inputs *porttest.Inputs
	inv    mutants.ListedInventory
}

func newListedCacheFixture(t *testing.T) *listedCacheFixture {
	root := t.TempDir()
	files := map[string]string{
		filepath.Join(root, "go.mod"):    "module example.com/w\n\ngo 1.22\n",
		filepath.Join(root, "a.go"):      "package w\n\nfunc A() int { return 1 }\n",
		filepath.Join(root, "b.go"):      "package w\n\nfunc B() int { return 2 }\n",
		filepath.Join(root, "a.feature"): "Feature: a\n",
	}
	return &listedCacheFixture{root: root, inputs: &porttest.Inputs{Dir: root, Files: files},
		inv: mutants.ParseInventory("ID-A-01\ta.feature\tfp1\nID-A-02\n")}
}

func (f *listedCacheFixture) key(t *testing.T, sources ...string) *mutants.ListedKey {
	t.Helper()
	var paths []string
	for _, s := range sources {
		paths = append(paths, filepath.Join(f.root, s))
	}
	key := mutants.KeyListed(t.Context(), f.inputs, f.inputs, f.root, paths, f.inv, map[string]string{"support/x.txt": "s1"})
	if key == nil {
		t.Fatal("no key for a supported Go module")
	}
	return key
}

// measured is a report of the listed tests' coverage of sources: ID-A-01
// covering line 3 of each, ID-A-02 nothing.
func (f *listedCacheFixture) measured(sources ...string) *coverage.Report {
	r := coverage.FromPieces(nil)
	files := map[string][]coverage.Piece{}
	for _, s := range sources {
		files[filepath.Join(f.root, s)] = []coverage.Piece{{Start: 3, End: 3, Total: 1, Covered: 1, Key: "3.14,3.24", InProcess: true}}
	}
	tests := &coverage.TestCoverage{}
	tests.Add("ID-A-01", coverage.FromPieces(files))
	r.SetTests(tests)
	return r
}

func kinds(events *runEvents) []string {
	var out []string
	for _, e := range events.values {
		out = append(out, e.Kind)
	}
	return out
}

func TestTheListedTestsCoverageIsKeptAndServedWhileItsInputsHold(t *testing.T) {
	f := newListedCacheFixture(t)
	cache := &listedCache{}
	events := &runEvents{}
	reuse := &mutants.ListedReuse{Cache: cache, Write: true, Progress: events}
	key := f.key(t, "a.go")
	if tc := reuse.Serve(t.Context(), key); tc != nil || len(events.values) != 0 {
		t.Fatalf("an empty cache served %v, said %v", tc, kinds(events))
	}
	measured := f.measured("a.go")
	// A file outside the root, which the cache keeps by its absolute path.
	outside := filepath.Join(t.TempDir(), "o.go")
	tests := &coverage.TestCoverage{}
	tests.Add("ID-A-01", coverage.Merge(measured.Tests().Of("ID-A-01"), coverage.FromPieces(map[string][]coverage.Piece{outside: {{Start: 1, End: 1, Total: 1, Covered: 1}}})))
	measured.SetTests(tests)
	reuse.Keep(t.Context(), key, measured)
	if len(cache.saved) != 1 {
		t.Fatalf("saved %d entries, want one", len(cache.saved))
	}
	entry := cache.saved[0]
	if !slices.Equal(entry.Measured, []string{"a.go"}) || len(entry.Tests) != 1 || entry.Tests[0].ID != "ID-A-01" || len(entry.Tests[0].Files["a.go"]) != 1 || len(entry.Tests[0].Files) != 2 {
		t.Fatalf("kept %+v, want a.go's coverage by ID-A-01, its path from the root", entry)
	}
	if entry.ListedFiles["a.feature"] == "" || len(entry.Listed) != 2 || entry.Listed[0].Fingerprint != "fp1" || entry.Inputs["support:support/x.txt"] != "s1" {
		t.Fatalf("kept inputs %+v", entry)
	}

	events.values = nil
	tc := (&mutants.ListedReuse{Cache: cache, Progress: events}).Serve(t.Context(), f.key(t, "a.go"))
	if tc == nil || !slices.Equal(kinds(events), []string{mutants.ListedCoverageReused}) {
		t.Fatalf("an unchanged key was not served: %v", kinds(events))
	}
	if covered, _ := tc.Of("ID-A-01").LineCovered(filepath.Join(f.root, "a.go"), 3); !covered {
		t.Error("the served coverage lost a.go's line 3, at its path under the root")
	}
	if covered, _ := tc.Of("ID-A-01").LineCovered(outside, 1); !covered {
		t.Error("the served coverage lost the file outside the root, at its own path")
	}
}

func TestACacheThatDoesNotServeTheRunSaysWhy(t *testing.T) {
	f := newListedCacheFixture(t)
	cache := &listedCache{}
	(&mutants.ListedReuse{Cache: cache, Write: true}).Keep(t.Context(), f.key(t, "a.go"), f.measured("a.go"))
	kept := *cache.entry
	serve := func(key *mutants.ListedKey) string {
		t.Helper()
		events := &runEvents{}
		if tc := (&mutants.ListedReuse{Cache: cache, Progress: events}).Serve(t.Context(), key); tc != nil {
			t.Fatal("served a key it does not serve")
		}
		if len(events.values) != 1 || events.values[0].Kind != mutants.ListedCoverageUnserved {
			t.Fatalf("said %v, want why it does not serve", kinds(events))
		}
		return events.values[0].Failure.Error()
	}
	if got := serve(f.key(t, "a.go", "b.go")); got != "it measured no b.go" {
		t.Errorf("a file it lacks: %q", got)
	}
	f.inputs.Files[filepath.Join(f.root, "a.feature")] = "Feature: a, changed\n"
	f.inv = mutants.ParseInventory("ID-A-01\ta.feature\tfp2\nID-A-03\n")
	if got := serve(f.key(t, "a.go")); got != "ID-A-01 changed, ID-A-02 changed, ID-A-03 changed and 1 more" {
		t.Errorf("changed inputs: %q", got)
	}
	cache.entry, cache.failure = nil, &snapport.Unreadable{Reason: "it does not decode"}
	if got := serve(f.key(t, "a.go")); got != "it cannot be read" {
		t.Errorf("an unreadable entry: %q", got)
	}
	cache.failure = &snapport.Cancelled{Cause: context.Canceled}
	events := &runEvents{}
	if tc := (&mutants.ListedReuse{Cache: cache, Progress: events}).Serve(t.Context(), f.key(t, "a.go")); tc != nil || len(events.values) != 0 {
		t.Errorf("a cancelled read served or said %v", kinds(events))
	}
	cache.failure, cache.entry = nil, &kept
	if tc := (*mutants.ListedReuse)(nil).Serve(t.Context(), f.key(t, "a.go")); tc != nil {
		t.Error("no reuse serves nothing")
	}
}

func TestARunKeepsOnlyACompleteMeasurementAndAddsItsFiles(t *testing.T) {
	f := newListedCacheFixture(t)
	cache := &listedCache{}
	(&mutants.ListedReuse{Cache: cache, Write: true}).Keep(t.Context(), f.key(t, "a.go"), f.measured("a.go"))
	reuse := &mutants.ListedReuse{Cache: cache, Write: true}
	key := f.key(t, "b.go")
	if reuse.Serve(t.Context(), key) != nil {
		t.Fatal("served b.go, which the entry lacks")
	}
	missing := f.measured("b.go")
	missing.SetMissing(coverage.Unmeasured{Dir: f.root, Language: "go", Cause: covport.MeasuredNothing{}, Reason: "no"})
	reuse.Keep(t.Context(), key, missing)
	(&mutants.ListedReuse{Cache: cache}).Keep(t.Context(), key, f.measured("b.go"))
	if len(cache.saved) != 1 {
		t.Fatalf("kept a measurement with a gap, or a run that writes nothing: %d saves", len(cache.saved))
	}
	reuse.Keep(t.Context(), key, f.measured("b.go"))
	added := cache.saved[len(cache.saved)-1]
	if !slices.Equal(added.Measured, []string{"a.go", "b.go"}) || len(added.Tests) != 1 || len(added.Tests[0].Files) != 2 {
		t.Fatalf("the entry of the same inputs did not gain b.go: %+v", added)
	}
	f.inputs.Files[filepath.Join(f.root, "b.go")] = "package w\n\nfunc B() int { return 3 }\n"
	replacing := &mutants.ListedReuse{Cache: cache, Write: true}
	key = f.key(t, "b.go")
	replacing.Serve(t.Context(), key)
	replacing.Keep(t.Context(), key, f.measured("b.go"))
	if replaced := cache.saved[len(cache.saved)-1]; !slices.Equal(replaced.Measured, []string{"b.go"}) {
		t.Fatalf("an entry of other inputs was not replaced: %+v", replaced.Measured)
	}
}

func TestNoKeyServesAFileStrictCoverageRefuses(t *testing.T) {
	f := newListedCacheFixture(t)
	if mutants.KeyListed(t.Context(), f.inputs, nil, f.root, []string{filepath.Join(f.root, "a.go")}, f.inv, nil) != nil {
		t.Error("a key without fingerprints")
	}
	if mutants.KeyListed(t.Context(), f.inputs, f.inputs, f.root, []string{filepath.Join(f.root, "notes.txt")}, f.inv, nil) != nil {
		t.Error("a key of a file of no language")
	}
	f.inputs.Files[filepath.Join(f.root, "package.json")] = "{}\n"
	f.inputs.Files[filepath.Join(f.root, "src", "a.ts")] = "export const a = 1\n"
	if mutants.KeyListed(t.Context(), nil, f.inputs, f.root, []string{filepath.Join(f.root, "src", "a.ts")}, f.inv, nil) != nil {
		t.Error("a key of a TypeScript file whose coverage runner cannot be told")
	}
	elsewhere := t.TempDir()
	f.inputs.Files[filepath.Join(elsewhere, "x.go")] = "package x\n"
	if mutants.KeyListed(t.Context(), f.inputs, f.inputs, f.root, []string{filepath.Join(elsewhere, "x.go")}, f.inv, nil) != nil {
		t.Error("a key of a Go file with no go.mod above it")
	}
	f.inputs.Files[filepath.Join(f.root, "go.work")] = "go 1.22\n\nuse .\n"
	if mutants.KeyListed(t.Context(), f.inputs, f.inputs, f.root, []string{filepath.Join(f.root, "a.go")}, f.inv, nil) != nil {
		t.Error("a key of a module an active go.work holds")
	}
}
