package main

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/internal/mutants"
)

// memoryExceptions rereads current fixture bytes on EVERY update. Initial
// heldConfig text selects entries only; it is never a stale document writer.
type memoryExceptions struct {
	memory *memory
	root   string
}

func (s memoryExceptions) Except(ctx context.Context, e mutants.Exception) (bool, mutants.ExceptionFailure) {
	if ctx.Err() != nil {
		return false, mutants.ExceptionCancelled{Cause: context.Cause(ctx)}
	}
	path := filepath.Join(s.root, config.File)
	data, err := s.memory.Read(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, memoryExceptionFailure(err)
	}
	updated, replaced, err := config.ExceptDocument(data, e)
	if err != nil {
		return false, memoryExceptionFailure(err)
	}
	if ctx.Err() != nil {
		return false, mutants.ExceptionCancelled{Cause: context.Cause(ctx)}
	}
	return replaced, memoryExceptionFailure(s.memory.Write(path, updated))
}
func (s memoryExceptions) Renew(ctx context.Context, renewals []mutants.Renewal) mutants.ExceptionFailure {
	if ctx.Err() != nil {
		return mutants.ExceptionCancelled{Cause: context.Cause(ctx)}
	}
	if len(renewals) == 0 {
		return nil
	}
	path := filepath.Join(s.root, config.File)
	data, err := s.memory.Read(path)
	if err != nil {
		return memoryExceptionFailure(err)
	}
	updated, err := config.RenewDocument(data, renewals)
	if err != nil {
		return memoryExceptionFailure(err)
	}
	if ctx.Err() != nil {
		return mutants.ExceptionCancelled{Cause: context.Cause(ctx)}
	}
	return memoryExceptionFailure(s.memory.Write(path, updated))
}
func memoryExceptionFailure(err error) mutants.ExceptionFailure {
	if err == nil {
		return nil
	}
	var invalid *config.InvalidError
	if errors.As(err, &invalid) {
		return mutants.ExceptionInvalid{Reason: invalid.Reason, Key: invalid.Key}
	}
	return mutants.ExceptionFailed{Cause: err}
}
