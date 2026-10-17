package main

import (
	"context"
	"io"

	"github.com/donvargax/itos-cc/internal/metricsdir"
	mutation "github.com/donvargax/itos-cc/internal/mutants"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
	"github.com/donvargax/itos-cc/internal/testtools"
)

func loadMutationSnapshot(ctx context.Context, store snapport.Store, key string) (*mutation.Snapshot, error) {
	return mutation.LoadSnapshot(ctx, metricsdir.Mutations{Store: store}, key, testtools.Toolchain{})
}
func planFreshForTest(ctx context.Context, root string, paths []string, since string, count int, seed string) (*mutation.FreshPlan, error) {
	return diskAdapters(io.Discard).Fresh.Plan(ctx, mutation.FreshRequest{Root: root, Paths: paths, Since: since, Count: count, Seed: seed})
}
func releaseFreshPlan(ctx context.Context, p *mutation.FreshPlan) error {
	diskAdapters(io.Discard).Fresh.Release(ctx, p)
	return nil
}
