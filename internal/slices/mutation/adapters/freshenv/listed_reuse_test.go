package freshenv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// The frozen listing refuses an empty command, a command that fails, an
// output it cannot read and a listed file the commit does not hold, and
// gives the output it read beside the tests.
func TestFrozenListingRefusesWhatItCannotList(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.feature"), []byte("Feature: a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	printing := func(out string, err error) covrun.CommandExecutor {
		return func(_ context.Context, cmd *exec.Cmd) error {
			_, _ = fmt.Fprint(cmd.Stdout, out)
			return err
		}
	}
	if _, _, _, err := listFreshTests(t.Context(), runtime.GOOS, root, "  ", io.Discard, printing("", nil)); err == nil {
		t.Error("an empty list command listed")
	}
	if _, _, calls, err := listFreshTests(t.Context(), runtime.GOOS, root, "list", io.Discard, printing("", errors.New("exit status 2"))); err == nil || len(calls) != 1 {
		t.Errorf("a failing list command listed: %v, %v", calls, err)
	}
	if _, _, _, err := listFreshTests(t.Context(), runtime.GOOS, root, "list", io.Discard, printing("T-1\nT-1\n", nil)); err == nil {
		t.Error("a duplicate ID listed")
	}
	if _, _, _, err := listFreshTests(t.Context(), runtime.GOOS, root, "list", io.Discard, printing("T-1\tgone.feature\n", nil)); err == nil {
		t.Error("a test of a file the commit lacks listed")
	}
	tests, output, _, err := listFreshTests(t.Context(), runtime.GOOS, root, "list", io.Discard, printing("T-1\ta.feature\tfp\n", nil))
	if err != nil || len(tests) != 1 || output != "T-1\ta.feature\tfp\n" {
		t.Errorf("listed %v, output %q, %v", tests, output, err)
	}
}

// Preparation that stops, or whose listed tests' coverage run fails, after
// the own coverage measured, reports it at that stage, whatever the cache
// holds of another commit.
func TestFreshPreparationStopsBeforeOrAtTheListedCoverage(t *testing.T) {
	repo := freshPreparationFixture(t)
	plan, err := PlanFresh(t.Context(), repo, nil, "", 1, "stop-test")
	if err != nil {
		t.Fatal(err)
	}
	cleanupFreshPlan(t, plan)
	plan.ListedReuse = &mutants.ListedReuse{Cache: &listedCache{}}
	stageOf := func(prep *FreshPreparation, name string) string {
		for _, s := range prep.Stages {
			if s.Name == name {
				return s.State
			}
		}
		return ""
	}
	var calls []string
	failing := freshPreparationExecutor(t, &calls, func(cmd *exec.Cmd) error {
		if len(cmd.Args) >= 3 && cmd.Args[2] == "fresh-listed-all" {
			return errors.New("the listed tests fail")
		}
		return nil
	})
	prep, err := prepareFreshContext(t.Context(), plan, freshPreparationOptions{Scope: covrun.OwnTests, Executor: failing, Log: coverageLog(t)})
	if err == nil || stageOf(prep, "listed-coverage") != "failed" {
		t.Fatalf("a failing listed coverage run: %v, stages %+v", err, prep.Stages)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls = nil
	// The stop comes while preparation reads the cache, which holds
	// nothing: no listed test may run after it.
	plan.ListedReuse = &mutants.ListedReuse{Cache: stoppingCache{stop: cancel}}
	stopping := freshPreparationExecutor(t, &calls, func(cmd *exec.Cmd) error {
		if len(cmd.Args) >= 3 && cmd.Args[2] == "fresh-listed-all" {
			return errors.New("the listed tests ran after the stop")
		}
		return nil
	})
	prep, err = prepareFreshContext(ctx, plan, freshPreparationOptions{Scope: covrun.OwnTests, Executor: stopping, Log: coverageLog(t)})
	if !errors.Is(err, context.Canceled) || stageOf(prep, "preparation") != "aborted" || stageOf(prep, "listed-coverage") != "" {
		t.Fatalf("a stop before the listed coverage: %v, stages %+v, calls %v", err, prep.Stages, calls)
	}
}

// stoppingCache holds nothing, and calls stop as it is read.
type stoppingCache struct{ stop context.CancelFunc }

func (c stoppingCache) Listed(context.Context) (*mutants.ListedCoverage, snapport.Failure) {
	c.stop()
	return nil, nil
}

func (stoppingCache) SaveListed(context.Context, mutants.ListedCoverage) snapport.Failure { return nil }
