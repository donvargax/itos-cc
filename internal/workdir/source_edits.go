package workdir

import (
	"context"
	"os"

	"github.com/donvargax/itos-cc/internal/mutants"
)

// SourceEdits updates the live source, never a planning or worker copy.
type SourceEdits struct{}

func (SourceEdits) Missing(ctx context.Context, path string) (bool, mutants.InputFailure) {
	if ctx.Err() != nil {
		return false, mutants.InputCancelled{Cause: context.Cause(ctx)}
	}
	_, err := os.Stat(path)
	return os.IsNotExist(err), nil
}

func (SourceEdits) Annotate(ctx context.Context, path, comment, block string) mutants.InputFailure {
	if ctx.Err() != nil {
		return mutants.InputCancelled{Cause: context.Cause(ctx)}
	}
	src, err := os.ReadFile(path) //nolint:gosec // reread the caller-selected current source immediately before updating its annotation
	if err != nil {
		return mutants.InputFailed{Cause: err}
	}
	updated := mutants.Annotated(string(src), comment, block)
	if updated == string(src) {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return mutants.InputFailed{Cause: err}
	}
	// Preserve the current source's permissions, as the legacy annotation did.
	if err := os.WriteFile(path, []byte(updated), info.Mode().Perm()); err != nil { //nolint:gosec // preserve the current selected source permissions, as the legacy annotation boundary did
		return mutants.InputFailed{Cause: err}
	}
	return nil
}
