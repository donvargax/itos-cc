package main

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/exceptions"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/settings"
	mutation "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// memoryExceptions rereads current fixture bytes on EVERY update. Initial
// heldConfig text selects entries only; it is never a stale document writer.
type memoryExceptions struct {
	memory *memory
	root   string
}

func (s memoryExceptions) Except(ctx context.Context, e mutation.Exception) (bool, mutation.ExceptionFailure) {
	if ctx.Err() != nil {
		return false, mutation.ExceptionCancelled{Cause: context.Cause(ctx)}
	}
	path := filepath.Join(s.root, settings.File)
	data, err := s.memory.Read(path)
	if errors.Is(err, fs.ErrNotExist) {
		data = exceptions.Created()
	} else if err != nil {
		return false, memoryExceptionFailure(err)
	}
	updated, replaced, err := exceptions.ExceptDocument(data, e)
	if err != nil {
		return false, memoryExceptionFailure(err)
	}
	if ctx.Err() != nil {
		return false, mutation.ExceptionCancelled{Cause: context.Cause(ctx)}
	}
	return replaced, memoryExceptionFailure(s.memory.Write(path, updated))
}
func (s memoryExceptions) Renew(ctx context.Context, renewals []mutation.Renewal) mutation.ExceptionFailure {
	return s.rewrite(ctx, len(renewals), func(data []byte) ([]byte, error) { return exceptions.RenewDocument(data, renewals) })
}
func (s memoryExceptions) Rehash(ctx context.Context, renewals []mutation.Renewal) mutation.ExceptionFailure {
	return s.rewrite(ctx, len(renewals), func(data []byte) ([]byte, error) { return exceptions.RehashDocument(data, renewals) })
}

// rewrite writes itos-cc.yaml as update makes it from the bytes it holds
// now, when n, the entries it updates, is not 0.
func (s memoryExceptions) rewrite(ctx context.Context, n int, update func([]byte) ([]byte, error)) mutation.ExceptionFailure {
	if ctx.Err() != nil {
		return mutation.ExceptionCancelled{Cause: context.Cause(ctx)}
	}
	if n == 0 {
		return nil
	}
	path := filepath.Join(s.root, settings.File)
	data, err := s.memory.Read(path)
	if err != nil {
		return memoryExceptionFailure(err)
	}
	updated, err := update(data)
	if err != nil {
		return memoryExceptionFailure(err)
	}
	if ctx.Err() != nil {
		return mutation.ExceptionCancelled{Cause: context.Cause(ctx)}
	}
	return memoryExceptionFailure(s.memory.Write(path, updated))
}
func (s memoryExceptions) Prune(ctx context.Context, entries []mutation.Exception) mutation.ExceptionFailure {
	if ctx.Err() != nil {
		return mutation.ExceptionCancelled{Cause: context.Cause(ctx)}
	}
	if len(entries) == 0 {
		return nil
	}
	path := filepath.Join(s.root, settings.File)
	data, err := s.memory.Read(path)
	if err != nil {
		return memoryExceptionFailure(err)
	}
	updated, err := exceptions.PruneDocument(data, entries)
	if err != nil {
		return memoryExceptionFailure(err)
	}
	if ctx.Err() != nil {
		return mutation.ExceptionCancelled{Cause: context.Cause(ctx)}
	}
	return memoryExceptionFailure(s.memory.Write(path, updated))
}
func memoryExceptionFailure(err error) mutation.ExceptionFailure {
	if err == nil {
		return nil
	}
	var invalid *settings.InvalidError
	if errors.As(err, &invalid) {
		return mutation.ExceptionInvalid{Reason: invalid.Reason, Key: invalid.Key}
	}
	return mutation.ExceptionFailed{Cause: err}
}
