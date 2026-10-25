package main

import (
	"context"
	"path/filepath"

	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/metricsdir"
	"github.com/donvargax/itos-cc/internal/slices/graph"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/settings"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/snapshots"
	mutation "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// graphMutation binds graph's Mutation port to the judgment the mutation
// slice exports (mutation.Graph), so graph imports nothing of that slice
// and check's rule stays its own (q-95 D1): a snapshot is named as the
// slice's snapshots adapter names it, and a project's listed support files
// are those the settings at its root, read by the slice's settings adapter
// (Settings), list.
type graphMutation struct {
	judge    mutation.Graph
	settings func(root string) (*mutation.Config, error)
}

var _ graph.Mutation = graphMutation{}

// newGraphMutation is graph's Mutation port over the snapshot readers, the
// toolchain and the fingerprints cmd chose, reading the settings at each
// project's root from the disk; a missing one is refused.
func newGraphMutation(snapshotReaders func(root string) mutation.MutationSnapshotReader, toolchain mutation.Toolchain, fingerprints mutation.Fingerprints) (graphMutation, error) {
	judge := mutation.Graph{Snapshots: snapshotReaders, Toolchain: toolchain, Fingerprints: fingerprints}
	if err := judge.Bound(); err != nil {
		return graphMutation{}, err
	}
	return graphMutation{judge: judge, settings: settings.LoadFrom}, nil
}

// Facts is f's facts, as mutation judges them.
func (g graphMutation) Facts(f *lang.File) any { return mutation.GraphFactsOf(f) }

// SnapshotPath is the file under top's .metrics that the snapshots adapter
// reads key's snapshot from.
func (graphMutation) SnapshotPath(top, key string) string {
	return filepath.Join(metricsdir.DirOf(top), snapshots.MutationName(key))
}

// Snapshot is key's snapshot, nil for none.
func (g graphMutation) Snapshot(ctx context.Context, top, key string) (any, error) {
	snap, err := g.judge.Snapshot(ctx, top, key)
	if err != nil {
		return nil, err
	}
	return snap, nil
}

// Support is the hashes of the support files of the tests the settings at
// top list, nil when they list none or cannot be read.
func (g graphMutation) Support(ctx context.Context, top string) map[string]string {
	cfg, err := g.settings(top)
	if err != nil || cfg.Tests == nil {
		return nil
	}
	return g.judge.Support(ctx, top, cfg.Tests.Support)
}

// Tests is the hashes of tests, as a snapshot records them.
func (g graphMutation) Tests(ctx context.Context, top string, tests []string) (map[string]string, error) {
	hashes, failure := g.judge.Fingerprints.Tests(ctx, top, tests)
	if failure != nil {
		return nil, failure
	}
	return hashes, nil
}

// Judge is mutation check's verdict on each function of file.
func (g graphMutation) Judge(ctx context.Context, file graph.Judged) []graph.UnitMutation {
	snap, _ := file.Snapshot.(*mutation.Snapshot)
	facts, _ := file.Facts.(mutation.GraphFacts)
	verdicts := g.judge.Judge(ctx, file.Top, file.Path, snap, facts, file.Tests, file.Support)
	out := make([]graph.UnitMutation, len(verdicts))
	for i, v := range verdicts {
		out[i] = graph.UnitMutation(v)
	}
	return out
}
