// Package importgraph adapts the current graph import resolver to mutation's
// Reach contract. The graph remains legacy until S8; cmd composes this adapter
// because graph itself still imports legacy mutation snapshot readers.
package importgraph

import (
	"context"
	"github.com/donvargax/itos-cc/internal/imports"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

type Reach struct{}

func (Reach) TestsImporting(ctx context.Context, root string, sources, tests []string) (map[string][]string, mutants.InputFailure) {
	if cause := context.Cause(ctx); cause != nil {
		return nil, mutants.InputCancelled{Cause: cause}
	}
	out, err := imports.TestsImporting(ctx, root, sources, tests)
	if cause := context.Cause(ctx); cause != nil {
		return nil, mutants.InputCancelled{Cause: cause}
	}
	if err != nil {
		return nil, mutants.InputFailed{Cause: err}
	}
	return out, nil
}
