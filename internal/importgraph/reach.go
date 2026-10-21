// Package importgraph adapts the current graph import resolver to mutation's
// Reach contract. The graph remains legacy until S8; cmd composes this adapter
// because graph itself still imports legacy mutation snapshot readers.
package importgraph

import (
	"context"
	"github.com/donvargax/itos-cc/graph"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/project"
)

type Reach struct{}

func (Reach) TestsImporting(ctx context.Context, root string, sources, tests []string) (map[string][]string, mutants.InputFailure) {
	if cause := context.Cause(ctx); cause != nil {
		return nil, mutants.InputCancelled{Cause: cause}
	}
	out, err := graph.TestsImporting(ctx, root, project.Files{Sources: sources, Tests: tests})
	if cause := context.Cause(ctx); cause != nil {
		return nil, mutants.InputCancelled{Cause: cause}
	}
	if err != nil {
		return nil, mutants.InputFailed{Cause: err}
	}
	return out, nil
}
