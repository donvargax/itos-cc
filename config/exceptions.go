package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// Exceptions updates the current configuration at an explicit project root.
// It writes through ExceptDocument and RenewDocument, the codec the in-process
// scenario driver's memory fixtures write through too.
type Exceptions struct{ RootDir string }

func (e Exceptions) Except(ctx context.Context, exception mutants.Exception) (bool, mutants.ExceptionFailure) {
	if ctx.Err() != nil {
		return false, mutants.ExceptionCancelled{Cause: context.Cause(ctx)}
	}
	path := filepath.Join(e.RootDir, File)
	data, err := os.ReadFile(path) //nolint:gosec // itos-cc.yaml at the project root this adapter was given
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, exceptionFailure(err)
	}
	updated, replaced, err := ExceptDocument(data, exception)
	if err != nil {
		return false, exceptionFailure(err)
	}
	if ctx.Err() != nil {
		return false, mutants.ExceptionCancelled{Cause: context.Cause(ctx)}
	}
	return replaced, exceptionFailure(writeDocument(path, updated))
}
func (e Exceptions) Renew(ctx context.Context, renewals []mutants.Renewal) mutants.ExceptionFailure {
	if ctx.Err() != nil {
		return mutants.ExceptionCancelled{Cause: context.Cause(ctx)}
	}
	if len(renewals) == 0 {
		return nil
	}
	path := filepath.Join(e.RootDir, File)
	data, err := os.ReadFile(path) //nolint:gosec // itos-cc.yaml at the project root this adapter was given
	if err != nil {
		return exceptionFailure(err)
	}
	updated, err := RenewDocument(data, renewals)
	if err != nil {
		return exceptionFailure(err)
	}
	if ctx.Err() != nil {
		return mutants.ExceptionCancelled{Cause: context.Cause(ctx)}
	}
	return exceptionFailure(writeDocument(path, updated))
}
func exceptionFailure(err error) mutants.ExceptionFailure {
	if err == nil {
		return nil
	}
	var invalid *InvalidError
	if errors.As(err, &invalid) {
		return mutants.ExceptionInvalid{Reason: invalid.Reason, Key: invalid.Key}
	}
	return mutants.ExceptionFailed{Cause: err}
}

// writeDocument rewrites itos-cc.yaml at path with data, keeping the mode
// it has, or 0644 for a file it creates.
func writeDocument(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return os.WriteFile(path, data, mode) //nolint:gosec // path is itos-cc.yaml at the project root, rewritten keeping its own mode
}
