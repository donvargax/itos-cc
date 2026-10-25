package graph

import (
	"context"
	"os"

	"github.com/donvargax/itos-cc/internal/metricsdir"
	"github.com/donvargax/itos-cc/internal/project"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// diskTree is the tree a test's fixtures are on: the disk, walked as the
// project engine walks it. graph's tests read their fixtures from the disk,
// as serve reads repositories, and cmd binds the real adapters.
type diskTree struct{}

func (diskTree) Discover(ctx context.Context, roots []string) (sources, tests []string, failure selport.Failure) {
	files, err := project.Discover(ctx, roots)
	if err != nil {
		return nil, nil, &selport.Unwalkable{Reason: err.Error()}
	}
	return files.Sources, files.Tests, nil
}

func (diskTree) Changed(context.Context) ([]string, selport.Failure) { return nil, nil }

func (diskTree) Exists(_ context.Context, path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (diskTree) Read(_ context.Context, path string) ([]byte, selport.Failure) {
	text, err := os.ReadFile(path) //nolint:gosec // a test's own fixture
	if err != nil {
		return nil, &selport.Unreadable{Path: path, Reason: err.Error()}
	}
	return text, nil
}

// onDisk is what a builder reads a test's fixtures through, asking m what
// mutation check knows of their functions.
func onDisk(m Mutation) Reading {
	return Reading{Tree: diskTree{}, Surroundings: project.Surroundings{}, Store: func(top string) snapport.Store {
		return metricsdir.Store{RootDir: top}
	}, Mutation: m, Top: project.RootOf}
}
