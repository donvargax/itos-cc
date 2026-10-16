package mutate

import (
	"context"
	"path/filepath"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/project"
)

// FreshGoCoverageEvidence binds measured blocks to one frozen admitted
// function without consulting persistent snapshots or the live project root.
func FreshGoCoverageEvidence(ctx context.Context, root string, admitted FreshUnit, blocks []coverage.GoBlock, producer string, inputs map[string]string) (*GoCoverageEvidence, error) {
	source := filepath.Join(root, filepath.FromSlash(admitted.Path))
	file, err := project.ParseFile(ctx, source)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return mutants.FreshGoCoverageEvidence(file, admitted, blocks, producer, inputs)
}
