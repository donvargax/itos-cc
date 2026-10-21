package main

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"testing"

	"github.com/donvargax/itos-cc/internal/metricsdir"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/testtools"
	mutation "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

func loadMutationSnapshot(ctx context.Context, store snapport.Store, key string) (*mutation.Snapshot, error) {
	return mutation.LoadSnapshot(ctx, metricsdir.Mutations{Store: store}, key, testtools.Toolchain{})
}
func planFreshForTest(ctx context.Context, root string, paths []string, since string, count int, seed string) (*mutation.FreshPlan, error) {
	return diskAdapters(io.Discard, nil).Fresh.Plan(ctx, mutation.FreshRequest{Root: root, Paths: paths, Since: since, Count: count, Seed: seed})
}
func releaseFreshPlan(ctx context.Context, p *mutation.FreshPlan) {
	diskAdapters(io.Discard, nil).Fresh.Release(ctx, p)
}

// removeFixture removes path, a fixture file the test may or may not have
// written by now, failing t on any error but its absence.
func removeFixture(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}
