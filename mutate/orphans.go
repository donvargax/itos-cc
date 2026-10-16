package mutate

import (
	"context"
	"github.com/donvargax/itos-cc/internal/metricsdir"
	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/snapshot"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
	"github.com/donvargax/itos-cc/internal/workdir"
	"path"
	"path/filepath"
	"strings"
)

const (
	OrphanGone     = mutants.OrphanGone
	OrphanExcluded = mutants.OrphanExcluded
)

type Orphan = mutants.Orphan

// Orphans retains concrete composition for direct legacy callers only.
func Orphans(ctx context.Context, store snapport.Store, covers, excluded func(path string) bool) ([]Orphan, error) {
	return mutants.Orphans(ctx, metricsdir.Mutations{Store: store}, workdir.SourceEdits{}, covers, excluded)
}
func recordedPath(name, file string) (string, bool) { return mutants.RecordedPath(name, file) }

// RemoveOrphans removes the snapshot of each orphan from store, and each
// folder under .metrics/mutate that removing them left empty (store's
// Remove). It removes nothing outside .metrics/mutate, whatever an Orphan
// names.
func RemoveOrphans(ctx context.Context, store snapport.Store, orphans []Orphan) error {
	for _, o := range orphans {
		name, ok := strings.CutPrefix(o.Snapshot, snapshot.Folder+"/mutate/")
		if !ok || !filepath.IsLocal(filepath.FromSlash(name)) {
			continue
		}
		if failure := store.Remove(ctx, path.Join("mutate", name)); failure != nil {
			return failure
		}
	}
	return nil
}
