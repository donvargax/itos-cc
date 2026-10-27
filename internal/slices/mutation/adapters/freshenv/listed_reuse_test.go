package freshenv

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/covrun"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// listedCache is a stub of the cache of the live project: the entry it
// holds.
type listedCache struct{ entry *mutants.ListedCoverage }

func (c *listedCache) Listed(context.Context) (*mutants.ListedCoverage, snapport.Failure) {
	return c.entry, nil
}

func (c *listedCache) SaveListed(_ context.Context, entry mutants.ListedCoverage) snapport.Failure {
	c.entry = &entry
	return nil
}

// events records what preparation said of the cache.
type events struct{ kinds []string }

func (e *events) Report(event mutants.RunEvent) { e.kinds = append(e.kinds, event.Kind) }

// A preparation whose plan reads the cache measures the listed tests' reach
// while the cache holds nothing for its frozen inputs, and takes it from
// the cache, running none of them, once it does (slice-69). This is the
// real adapter's own path, which the in-process scenarios reach through
// the fake preparation.
func TestFreshPreparationServesTheListedReachTheCacheHolds(t *testing.T) {
	repo := freshPreparationFixture(t)
	plan, err := PlanFresh(t.Context(), repo, nil, "", 1, "reuse-test")
	if err != nil {
		t.Fatal(err)
	}
	cleanupFreshPlan(t, plan)
	cache := &listedCache{}
	said := &events{}
	plan.ListedReuse = &mutants.ListedReuse{Cache: cache, Progress: said}
	prepare := func() (*FreshPreparation, []string) {
		t.Helper()
		var calls []string
		prep, err := prepareFreshContext(t.Context(), plan, freshPreparationOptions{Scope: covrun.OwnTests, Executor: freshPreparationExecutor(t, &calls, nil), Log: coverageLog(t)})
		if err != nil {
			t.Fatalf("prepare: %v; stages=%+v calls=%v", err, prep.Stages, calls)
		}
		return prep, calls
	}
	ranListed := func(calls []string) bool {
		return slices.ContainsFunc(calls, func(call string) bool { return strings.Contains(call, "fresh-listed-") })
	}
	measured, calls := prepare()
	if !ranListed(calls) || cache.entry != nil || len(said.kinds) != 0 {
		t.Fatalf("an empty cache: listed ran %v, kept %+v, said %v", ranListed(calls), cache.entry, said.kinds)
	}
	// What a complete run of the same commit keeps: its key is the frozen
	// tree's, as preparation takes it.
	e := fixtureEnvironment(repo)
	key := mutants.KeyListed(t.Context(), e.Toolchain, e.Fingerprints, plan.FrozenRoot, mutants.FreshMeasurementSources(plan),
		mutants.ParseInventory("T-1\tmain_test.go\n"), nil)
	(&mutants.ListedReuse{Cache: cache, Write: true}).Keep(t.Context(), key, measured.Report)
	if cache.entry == nil {
		t.Fatal("the measured preparation's reach could not be kept")
	}
	served, calls := prepare()
	if ranListed(calls) || !slices.Equal(said.kinds, []string{mutants.ListedCoverageReused}) {
		t.Fatalf("a matching cache: listed ran %v (%v), said %v", ranListed(calls), calls, said.kinds)
	}
	if got := served.Report.LineTests(filepath.Join(plan.FrozenRoot, "main.go"), 2); !slices.Equal(got, []string{"T-1"}) {
		t.Fatalf("served listed reach = %v, want [T-1]", got)
	}
	if i := slices.IndexFunc(served.Stages, func(s PreparationStage) bool { return s.Name == "listed-coverage" }); i < 0 || served.Stages[i].State != "complete" {
		t.Fatalf("stages %+v, want listed-coverage complete", served.Stages)
	}
}
