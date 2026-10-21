// Package importgraph adapts the import-resolution engine, internal/imports,
// to mutation's Reach contract (T-162): it calls the engine's TestsImporting
// and turns the context's cause into InputCancelled and the engine's error
// into InputFailed, since the engine imports no slice and cannot return the
// domain's failures. cmd composes it; it imports neither graph nor project.
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
