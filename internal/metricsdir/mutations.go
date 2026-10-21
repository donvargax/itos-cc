package metricsdir

import (
	"context"
	"path/filepath"

	"github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// Mutations supplies model-valued mutation snapshots through the existing Store
// and its concrete codec, preserving atomic publication and all stored bytes.
type Mutations struct{ Store snapport.Store }

func MutationName(key string) string                                    { return filepath.Join("mutate", filepath.ToSlash(key)+".json") }
func (m Mutations) Root(ctx context.Context) (string, snapport.Failure) { return m.Store.Root(ctx) }
func (m Mutations) Read(ctx context.Context, key string) (*mutants.Snapshot, snapport.Failure) {
	var wire snapshotWire
	found, failure := m.Store.Read(ctx, MutationName(key), &wire)
	if failure != nil || !found {
		return nil, failure
	}
	s := wire.domain()
	return &s, nil
}
func (m Mutations) Write(ctx context.Context, key string, s mutants.Snapshot) snapport.Failure {
	return m.Store.Write(ctx, MutationName(key), SnapshotJSON(s))
}
func (m Mutations) Remove(ctx context.Context, key string) snapport.Failure {
	return m.Store.Remove(ctx, MutationName(key))
}

func (m Mutations) List(ctx context.Context) ([]string, snapport.Failure) {
	return m.Store.List(ctx, "mutate")
}
func (m Mutations) RecordedFile(ctx context.Context, name string) (string, snapport.Failure) {
	var metadata struct {
		File string `json:"file"`
	}
	_, failure := m.Store.Read(ctx, name, &metadata)
	return metadata.File, failure
}
