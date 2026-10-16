package mutants_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/mutants/porttest"
	selected "github.com/donvargax/itos-cc/internal/selection"
	seltest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
)

func exceptionOptions(f *runFixture) mutants.CheckOptions {
	o := f.options
	return mutants.CheckOptions{Root: o.Root, Dir: o.Dir, Tree: o.Tree, Surroundings: o.Surroundings, Store: o.Store, Toolchain: o.Toolchain, Fingerprints: o.Fingerprints, Tests: o.Tests}
}
func sampleOptions(f *runFixture) mutants.SampleOptions {
	return mutants.SampleOptions{CheckOptions: exceptionOptions(f), WorkerFactory: f.workers, Workers: 1, TimeoutFactor: 10}
}
func survivingFixture(t *testing.T) (*runFixture, mutants.Site) {
	t.Helper()
	f := newRunFixture(t)
	parsed, err := selected.Parse(t.Context(), f.options.Tree, f.inputs, f.path)
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
	f.run(t)
	return f, sites[0]
}
func TestDomainSampleUsesRecordedScopesDrawAndActualWorkerFacts(t *testing.T) {
	f, site := survivingFixture(t)
	opt := sampleOptions(f)
	before := f.snapshots.Values["board.go"]
	writes := len(f.snapshots.Written)
	a, err := mutants.Sample(t.Context(), []string{f.path}, 1, "seed", opt)
	if err != nil || a.Cached < 1 || len(a.Files) != 1 || len(a.Files[0].Mutants) != 1 || !a.Files[0].Mutants[0].Agrees() {
		t.Fatalf("sample: %+v %v", a, err)
	}
	b, err := mutants.Sample(t.Context(), []string{f.path}, 1, "seed", opt)
	if err != nil || !reflect.DeepEqual(a, b) {
		t.Fatalf("seed draw: %+v %v", b, err)
	}
	if !reflect.DeepEqual(before, f.snapshots.Values["board.go"]) || writes != len(f.snapshots.Written) || len(f.edits.Writes) != 0 {
		t.Fatal("sample published")
	}
	// Same actual source and stored outcome, but independent command verdict changes.
	for command := range f.workers.Tests[f.root] {
		f.workers.Tests[f.root][command] = porttest.Tests{}
	}
	changed, err := mutants.Sample(t.Context(), []string{f.path}, 100, "seed", opt)
	if err != nil || len(changed.Files) == 0 {
		t.Fatalf("counterexample: %+v %v", changed, err)
	}
	for _, m := range changed.Files[0].Mutants {
		if m.Agrees() {
			t.Fatal("actual changed command outcome ignored")
		}
	}
	// Listed scope admits only its stored test IDs, retaining its own baseline.
	snap := f.snapshots.Values["board.go"]
	for i := range snap.Units {
		for j := range snap.Units[i].Mutants {
			m := &snap.Units[i].Mutants[j]
			m.Scope = mutants.ScopeListed
			m.Tests = []string{"selected"}
			m.Outcome = mutants.Survived
		}
	}
	f.snapshots.Values["board.go"] = snap
	opt.Listed = &mutants.Listed{Root: f.root, Select: func([]string) string { return "listed selection" }}
	f.workers.Tests[f.root]["listed selection"] = porttest.Tests{}
	// Own tests must survive first to reach the actual independent listed command.
	for command, facts := range f.workers.Tests[f.root] {
		if command != "listed selection" {
			facts.Verdicts = []porttest.Verdict{{Path: "board.go", Site: site, Outcome: mutants.Survived}}
			f.workers.Tests[f.root][command] = facts
		}
	}
	listed, err := mutants.Sample(t.Context(), []string{f.path}, 100, "seed", opt)
	if err != nil || len(listed.Files) != 1 {
		t.Fatalf("listed: %+v %v", listed, err)
	}
	saw := false
	for _, m := range listed.Files[0].Mutants {
		if m.Site.Key() == site.Key() {
			saw = true
			if m.Scope != mutants.ScopeListed || m.Outcome != mutants.Killed {
				t.Fatalf("listed outcome: %+v", m)
			}
		}
	}
	if !saw {
		t.Fatal("listed sample missing site")
	}
}
func TestDomainSampleRetainsBaselineFailureAndUnusedWorkersBypass(t *testing.T) {
	f, _ := survivingFixture(t)
	opt := sampleOptions(f)
	for command := range f.workers.Tests[f.root] {
		f.workers.Tests[f.root][command] = porttest.Tests{Baseline: "actual baseline failed"}
	}
	sampled, err := mutants.Sample(t.Context(), []string{f.path}, 100, "seed", opt)
	if err != nil || len(sampled.Files) != 1 || !sampled.Files[0].BaselineFailed || len(sampled.Files[0].Mutants) != 0 {
		t.Fatalf("baseline: %+v %v", sampled, err)
	}
	opt.WorkerFactory = nil
	f.snapshots.Values = nil
	sampled, err = mutants.Sample(t.Context(), []string{f.path}, 100, "seed", opt)
	if err != nil || sampled.Cached != 0 {
		t.Fatalf("no candidates need workers: %+v %v", sampled, err)
	}
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("sample stopped")
	cancel(cause)
	if _, err := mutants.Sample(ctx, []string{f.path}, 1, "seed", opt); !errors.Is(err, cause) {
		t.Fatalf("sample cancellation: %v", err)
	}
}
func TestDomainSurvivorAndRenewUseActualParsedEvidenceAndDistinctStatusPolicies(t *testing.T) {
	f, site := survivingFixture(t)
	opt := exceptionOptions(f)
	entry, err := mutants.Survivor(t.Context(), f.path, site.Line, site.Column, opt, f.edits)
	if err != nil || entry.File != "board.go" || entry.Original != site.Original {
		t.Fatalf("survivor: %+v %v", entry, err)
	}
	entry.Reason = "independent review"
	renewed, left, err := mutants.Renew(t.Context(), []mutants.Exception{entry}, []string{f.path}, opt, f.edits)
	if err != nil || len(renewed) != 0 || len(left) != 0 {
		t.Fatalf("holding entry: %+v %+v %v", renewed, left, err)
	}
	old := entry
	old.Hash = "old hash"
	renewed, left, err = mutants.Renew(t.Context(), []mutants.Exception{old}, []string{f.path}, opt, f.edits)
	if err != nil || len(renewed) != 1 || len(left) != 0 || renewed[0].New != entry || renewed[0].Match != mutants.MatchLine {
		t.Fatalf("renewed: %+v %+v %v", renewed, left, err)
	}
	denied := errors.New("actual Stat denied")
	f.edits.StatErrors = map[string]error{f.path: denied}
	_, err = mutants.Survivor(t.Context(), f.path, site.Line, site.Column, opt, f.edits)
	var none *mutants.NoSurvivorError
	if !errors.As(err, &none) || none.Reason != "there is no such file" {
		t.Fatalf("survivor ordinary error: %v", err)
	}
	if _, _, err = mutants.Renew(t.Context(), []mutants.Exception{old}, []string{f.path}, opt, f.edits); !errors.Is(err, denied) {
		t.Fatalf("renew ordinary error: %v", err)
	}
	if _, err = mutants.Survivor(t.Context(), filepath.Join(f.root, "unsupported.txt"), 1, 1, mutants.CheckOptions{}, nil); !errors.As(err, &none) || !strings.Contains(none.Reason, "no source") {
		t.Fatalf("syntax admission: %v", err)
	}
	f.edits.StatErrors = nil
	// A supported source missing in independent Stat state cannot use the host.
	f.edits.Files = map[string]string{}
	if _, err = mutants.Survivor(t.Context(), f.path, site.Line, site.Column, opt, f.edits); !errors.As(err, &none) {
		t.Fatalf("absent source accepted: %v", err)
	}
	// The old namespace alone may differ after a module rename; unique name still pairs.
	old = entry
	old.Function = "old-module#Value"
	f.edits.Files = f.files
	renewed, left, err = mutants.Renew(t.Context(), []mutants.Exception{old}, []string{f.path}, opt, f.edits)
	if err != nil || len(renewed) != 1 || len(left) != 0 {
		t.Fatalf("namespace renewal: %+v %+v %v", renewed, left, err)
	}
}
func TestDomainElsewhereUsesLexicalRootsAndLazyParse(t *testing.T) {
	f, _ := survivingFixture(t)
	opt := exceptionOptions(f)
	parsed, err := selected.Parse(t.Context(), opt.Tree, opt.Surroundings, f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer parsed.Close()
	unit := parsed.Units[0]
	entry := mutants.Exception{File: "old.go", Function: "old#" + unit.Name, Hash: mutants.UnitHash(parsed, unit)}
	yes := func(string) bool { return true }
	no := func(string) bool { return false }
	stale, moved, err := mutants.Elsewhere(t.Context(), []mutants.Exception{entry}, []string{f.path}, yes, no, opt)
	if err != nil || len(stale) != 1 || stale[0].Why != mutants.ExceptionMoved || len(moved) != 1 || moved[0].File != "board.go" {
		t.Fatalf("moved: %+v %+v %v", stale, moved, err)
	}
	opt.Tree = nil
	opt.Surroundings = nil
	stale, _, err = mutants.Elsewhere(t.Context(), []mutants.Exception{entry}, []string{f.path}, no, no, opt)
	if err != nil || len(stale) != 0 {
		t.Fatalf("unused parser: %+v %v", stale, err)
	}
	stale, _, err = mutants.Elsewhere(t.Context(), []mutants.Exception{entry}, []string{f.path}, yes, yes, opt)
	if err != nil || len(stale) != 1 || stale[0].Why != mutants.ExceptionExcluded {
		t.Fatalf("excluded: %+v %v", stale, err)
	}
	if _, _, err = mutants.Elsewhere(t.Context(), []mutants.Exception{entry}, []string{f.path}, yes, no, opt); err == nil {
		t.Fatal("missing parser accepted")
	}
	outside := filepath.Join(filepath.Dir(f.root), "outside.go")
	// Key naming outside the root must stay absolute, without Git/cwd discovery.
	opt.Tree = seltest.TreeOf(filepath.Dir(f.root), map[string]string{"outside.go": runSource}, nil)
	opt.Surroundings = &porttest.Inputs{Dir: f.root, Files: map[string]string{outside: runSource}}
	entry.File = outside
	stale, _, err = mutants.Elsewhere(t.Context(), []mutants.Exception{entry}, []string{outside}, yes, no, opt)
	if err != nil || len(stale) != 0 {
		t.Fatalf("outside selection: %+v %v", stale, err)
	}
}
