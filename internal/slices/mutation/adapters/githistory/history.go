// Package githistory adapts the existing Git library to mutation's History.
package githistory

import (
	"context"
	"errors"

	"github.com/donvargax/itos-cc/internal/project"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
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
	changed, renamed, err := project.ChangedSinceAt(ctx, h.Dir, ref, judged(mutants.ChangedFunctionsOf, mutants.UnitHash))
	return changed, renamed, failed(ctx, ref, err)
}

// judged is the judge ChangedSince hands each file the range changed: of,
// mutation's mutants.ChangedFunctionsOf, with hash, given project's line
// ranges converted to mutation's field for field. It is generic in the
// parsed file and the hash so this adapter names no type of internal/lang,
// which its import rule keeps out: the call infers them.
func judged[F, H, T any](of func(file, base F, ranges []mutants.LineRange, hash H) T, hash H) func(file, base F, lines []project.LineRange) T {
	return func(file, base F, lines []project.LineRange) T {
		ranges := make([]mutants.LineRange, 0, len(lines))
		for _, r := range lines {
			ranges = append(ranges, mutants.LineRange{Start: r.Start, End: r.End, Deletion: r.Deletion})
		}
		return of(file, base, ranges, hash)
	}
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
