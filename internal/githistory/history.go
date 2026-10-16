// Package githistory adapts the existing Git library to mutation's History.
package githistory

import (
	"context"
	"errors"

	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/project"
)

// History observes one actual caller directory without changing process cwd.
type History struct{ Dir string }

var _ mutants.History = History{}

func (h History) Head(ctx context.Context) (string, mutants.HistoryFailure) {
	value, err := project.HeadAt(ctx, h.Dir)
	return value, failed(ctx, "HEAD", err)
}
func (h History) Resolve(ctx context.Context, ref string) (string, mutants.HistoryFailure) {
	value, err := project.ResolveAt(ctx, h.Dir, ref)
	return value, failed(ctx, ref, err)
}
func (h History) ReadCommitted(ctx context.Context, ref, path string) ([]byte, mutants.HistoryFailure) {
	value, err := project.ReadCommittedAt(ctx, h.Dir, ref, path)
	return value, failed(ctx, ref, err)
}
func (h History) ChangedSince(ctx context.Context, ref string) (map[string]mutants.ChangedFunctions, map[string]string, mutants.HistoryFailure) {
	changed, renamed, err := project.ChangedSinceAt(ctx, h.Dir, ref, mutants.UnitHash)
	return changed, renamed, failed(ctx, ref, err)
}
func (h History) DeletedSince(ctx context.Context, ref string) ([]string, mutants.HistoryFailure) {
	value, err := project.DeletedSinceAt(ctx, h.Dir, ref)
	return value, failed(ctx, ref, err)
}
func (h History) RenamesSince(ctx context.Context, ref string) (map[string]string, mutants.HistoryFailure) {
	value, err := project.RenamesSinceAt(ctx, h.Dir, ref)
	return value, failed(ctx, ref, err)
}
func (h History) RenamesUncommitted(ctx context.Context) (map[string]string, mutants.HistoryFailure) {
	value, err := project.RenamesUncommittedAt(ctx, h.Dir)
	return value, failed(ctx, "HEAD", err)
}
func failed(ctx context.Context, ref string, err error) mutants.HistoryFailure {
	if ctx.Err() != nil {
		return mutants.HistoryCancelled{Cause: context.Cause(ctx)}
	}
	if err == nil {
		return nil
	}
	var noGit *project.NoGitError
	switch {
	case errors.As(err, &noGit):
		return mutants.NoRepository{Reason: noGit.Reason}
	case errors.Is(err, project.ErrBadRef):
		return mutants.BadReference{Ref: ref}
	default:
		return mutants.HistoryFailed{Cause: err}
	}
}
