package main

import (
	"context"
	"os"

	"github.com/donvargax/itos-cc/internal/metricsdir"
	"github.com/donvargax/itos-cc/internal/project"
	"github.com/donvargax/itos-cc/internal/slices/graph"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/settings"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/snapshots"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/testinputs"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/testtools"
	mutation "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
	"github.com/donvargax/itos-cc/internal/workdir"
)

// Retained graph/check parity guards compose the real observations explicitly.
func checkGraphSnapshot(ctx context.Context, store snapport.Store, files []string, judge func(string, string, string) bool, tests func(string) []string, support map[string]string, exceptions []settings.Exception) ([]mutation.FileCheck, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return mutation.Check(ctx, files, mutation.CheckOptions{Root: project.Root(), Dir: dir, Store: snapshots.Mutations{Store: store}, Tree: workdir.Tree{}, Surroundings: workdir.Surroundings{}, Toolchain: testtools.Toolchain{}, Fingerprints: testinputs.Fingerprints{Runtime: testinputs.CurrentRuntime()}, Judge: judge, Tests: tests, Support: support, Exceptions: exceptions})
}

// graphInputs are the mutation observations a graph builder's Mutation port
// is bound over (newGraphMutation).
type graphInputs struct {
	Snapshots    func(root string) mutation.MutationSnapshotReader
	Toolchain    mutation.Toolchain
	Fingerprints mutation.Fingerprints
}

// newGraphBuilder is the graph builder of roots over inputs, as serve binds
// it (newServer).
func newGraphBuilder(roots []string, inputs graphInputs) (*graph.Builder, error) {
	m, err := newGraphMutation(inputs.Snapshots, inputs.Toolchain, inputs.Fingerprints)
	if err != nil {
		return nil, err
	}
	return graph.NewBuilder(roots, servedFrom(m))
}

func realMutationInputs() graphInputs {
	dir, _ := os.Getwd()
	return graphInputs{Snapshots: func(root string) mutation.MutationSnapshotReader {
		return snapshots.Mutations{Store: metricsdir.Store{RootDir: root}}
	}, Toolchain: testtools.Toolchain{Dir: dir, Env: os.Environ()}, Fingerprints: testinputs.Fingerprints{Runtime: testinputs.CurrentRuntime()}}
}
