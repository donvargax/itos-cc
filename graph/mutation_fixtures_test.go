package graph

import (
	"context"
	"os"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/internal/metricsdir"
	"github.com/donvargax/itos-cc/internal/project"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/snapshots"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/testinputs"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/testtools"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
	"github.com/donvargax/itos-cc/internal/workdir"
)

// Retained graph/check parity guards compose the real observations explicitly.
func checkGraphSnapshot(ctx context.Context, store snapport.Store, files []string, judge func(string, string, string) bool, tests func(string) []string, support map[string]string, exceptions []config.Exception) ([]mutants.FileCheck, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return mutants.Check(ctx, files, mutants.CheckOptions{Root: project.Root(), Dir: dir, Store: snapshots.Mutations{Store: store}, Tree: workdir.Tree{}, Surroundings: workdir.Surroundings{}, Toolchain: testtools.Toolchain{}, Fingerprints: testinputs.Fingerprints{Runtime: testinputs.CurrentRuntime()}, Judge: judge, Tests: tests, Support: support, Exceptions: exceptions})
}

func realMutationInputs() MutationInputs {
	dir, _ := os.Getwd()
	return MutationInputs{Snapshots: func(root string) mutants.MutationSnapshotReader {
		return snapshots.Mutations{Store: metricsdir.Store{RootDir: root}}
	}, Toolchain: testtools.Toolchain{Dir: dir, Env: os.Environ()}, Fingerprints: testinputs.Fingerprints{Runtime: testinputs.CurrentRuntime()}}
}
