package workdir

import (
	"context"
	"os"

	"github.com/donvargax/itos-cc/internal/slices/mutation/domain"
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

func (SourceEdits) Present(ctx context.Context, path string) (bool, mutants.InputFailure) {
	if ctx.Err() != nil {
		return false, mutants.InputCancelled{Cause: context.Cause(ctx)}
	}
	_, err := os.Lstat(path)
	return !os.IsNotExist(err), nil
}

func (SourceEdits) RenameTarget(ctx context.Context, path string) (bool, mutants.InputFailure) {
	if ctx.Err() != nil {
		return false, mutants.InputCancelled{Cause: context.Cause(ctx)}
	}
	_, err := os.Lstat(path)
	return err == nil, nil
}

// Kind follows links and accepts any non-directory, as exception renewal did.
func (SourceEdits) Kind(ctx context.Context, path string) (mutants.SourceKind, mutants.InputFailure) {
	if ctx.Err() != nil {
		return mutants.SourceMissing, mutants.InputCancelled{Cause: context.Cause(ctx)}
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return mutants.SourceMissing, nil
	}
	if err != nil {
		return mutants.SourceMissing, mutants.InputFailed{Cause: err}
	}
	if info.IsDir() {
		return mutants.SourceDirectory, nil
	}
	return mutants.SourceNonDirectory, nil
}
