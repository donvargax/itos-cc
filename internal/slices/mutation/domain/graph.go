package domain

import (
	"context"
	"errors"

	"github.com/donvargax/itos-cc/internal/lang"
)

// Graph is mutation check's judgment of a file's functions as the
// architecture graph shows it (GRAPH-38, q-95 D1): the graph slice reaches
// it only through a port of its own, which cmd binds to this, so check's
// rule stays this slice's and the two cannot drift. Snapshots makes the
// reader of a project's mutation snapshots from its root, with no I/O;
// Toolchain and Fingerprints observe the tools and hash the inputs a
// snapshot recorded, as check observes and hashes them.
type Graph struct {
	Snapshots    func(root string) MutationSnapshotReader
	Toolchain    Toolchain
	Fingerprints Fingerprints
}

// Bound is a refusal when any of g's observations is missing, so a graph
// never silently reads the disk instead.
func (g Graph) Bound() error {
	if g.Snapshots == nil || g.Toolchain == nil || g.Fingerprints == nil {
		return errors.New("graph mutation observation bindings are missing")
	}
	return nil
}

// GraphFacts is what the judgment needs of a parsed source's functions, in
// their order: each one's namespace#name and hash, and its mutation sites,
// by its index. The graph keeps it with the file, so an unchanged file is
// never parsed again.
type GraphFacts struct {
	IDs, Hashes []string
	Sites       map[int][]Site
}

// GraphFactsOf is f's facts.
func GraphFactsOf(f *lang.File) GraphFacts {
	ids, hashes := FileKeys(f)
	sites := map[int][]Site{}
	for _, s := range Sites(f) {
		sites[s.Unit] = append(sites[s.Unit], s)
	}
	return GraphFacts{IDs: ids, Hashes: hashes, Sites: sites}
}

// Snapshot is the snapshot of the source whose key is key, its path from
// the project root top, read where mutation check reads it (LoadSnapshot),
// nil for none. One that cannot be read is none too, so the graph still
// builds; the error is a missing reader.
func (g Graph) Snapshot(ctx context.Context, top, key string) (*Snapshot, error) {
	reader := g.Snapshots(top)
	if reader == nil {
		return nil, errors.New("graph mutation snapshot reader is missing")
	}
	snap, _ := LoadSnapshot(ctx, reader, key, g.Toolchain)
	return snap, nil
}

// Support is the hashes of the files the support globs name under the
// project root top, hashed as a snapshot records them; nil when they cannot
// be.
func (g Graph) Support(ctx context.Context, top string, globs []string) map[string]string {
	support, failed := g.Fingerprints.Support(ctx, top, globs)
	if failed != nil {
		return nil
	}
	return support
}

// GraphVerdict is what mutation check knows of one function: whether a
// snapshot entry records it, its counts, and whether its results are stale.
type GraphVerdict struct {
	Mutated                     bool
	Killed, Survived, Uncovered int
	Stale                       bool
}

// Judge is the verdict on each function facts names of the source file at
// path in the project whose root is top, snap being its snapshot (nil for
// none), tests the hashes of the test files that import it now and support
// those of its project's listed support files. A function's entry is paired
// as check pairs it, by name and hash (EntriesOf), so of functions sharing
// a name, reordering them changes nothing and editing one makes only that
// one stale; it is stale exactly when check calls it stale (FreshnessOf),
// listed being how the files its listed outcomes rest on changed and broad
// how its module's tests and support did; a function with no entry, which
// check calls missing, carries none.
func (g Graph) Judge(ctx context.Context, top, path string, snap *Snapshot, facts GraphFacts, tests, support map[string]string) []GraphVerdict {
	out := make([]GraphVerdict, len(facts.IDs))
	if snap == nil {
		return out
	}
	listed := ListedChangedWithInputs(ctx, snap, top, support, g.Fingerprints)
	var moduleTests map[string]string
	if HasBroadOutcome(snap.Units) {
		moduleTests, _ = g.Fingerprints.Suite(ctx, path, top)
	}
	for i, j := range EntriesOf(facts.IDs, facts.Hashes, snap.Units) {
		if j < 0 {
			continue
		}
		hash, sites := facts.Hashes[i], facts.Sites[i]
		m := OnSites(&snap.Units[j], hash, sites)
		out[i] = GraphVerdict{Mutated: true, Killed: m.Killed, Survived: m.Survived, Uncovered: m.Uncovered,
			Stale: FreshnessOf(m, hash, snap.TestsChanged(tests), listed, BroadChangesForUnit(m.Mutants, moduleTests, support), sites).State == Stale}
	}
	return out
}
