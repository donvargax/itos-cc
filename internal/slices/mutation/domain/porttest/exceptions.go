package porttest

import (
	"context"
	"fmt"
	"io/fs"
	"slices"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// Exceptions owns independent current entries. It contains no configuration codec.
// Failure is an actual unavailable/invalid document state, not a call script.
type Exceptions struct {
	Entries []mutants.Exception
	Missing bool
	Failure mutants.ExceptionFailure
	Writes  int
}

func (s *Exceptions) Except(ctx context.Context, e mutants.Exception) (bool, mutants.ExceptionFailure) {
	if ctx.Err() != nil {
		return false, mutants.ExceptionCancelled{Cause: context.Cause(ctx)}
	}
	if s.Failure != nil {
		return false, s.Failure
	}
	entries := slices.Clone(s.Entries)
	for i, old := range entries {
		if old.SameSite(e) {
			entries[i] = e
			s.Entries = entries
			s.Missing = false
			s.Writes++
			return true, nil
		}
	}
	s.Entries = append(entries, e)
	s.Missing = false
	s.Writes++
	return false, nil
}
func (s *Exceptions) Prune(ctx context.Context, entries []mutants.Exception) mutants.ExceptionFailure {
	if ctx.Err() != nil {
		return mutants.ExceptionCancelled{Cause: context.Cause(ctx)}
	}
	if len(entries) == 0 {
		return nil
	}
	if s.Missing {
		return mutants.ExceptionFailed{Cause: fs.ErrNotExist}
	}
	if s.Failure != nil {
		return s.Failure
	}
	done := make([]bool, len(s.Entries))
	for _, e := range entries {
		at := slices.IndexFunc(s.Entries, func(old mutants.Exception) bool { return old == e })
		for at >= 0 && done[at] {
			next := slices.IndexFunc(s.Entries[at+1:], func(old mutants.Exception) bool { return old == e })
			if next < 0 {
				at = -1
				break
			}
			at += 1 + next
		}
		if at < 0 {
			return mutants.ExceptionFailed{Cause: fmt.Errorf("no entry for %s in %s to remove", e.Function, e.File)}
		}
		done[at] = true
	}
	var kept []mutants.Exception
	for i, e := range s.Entries {
		if !done[i] {
			kept = append(kept, e)
		}
	}
	s.Entries = kept
	s.Writes++
	return nil
}
func (s *Exceptions) Renew(ctx context.Context, renewals []mutants.Renewal) mutants.ExceptionFailure {
	if ctx.Err() != nil {
		return mutants.ExceptionCancelled{Cause: context.Cause(ctx)}
	}
	if len(renewals) == 0 {
		return nil
	}
	if s.Missing {
		return mutants.ExceptionFailed{Cause: fs.ErrNotExist}
	}
	if s.Failure != nil {
		return s.Failure
	}
	entries := slices.Clone(s.Entries)
	done := make([]bool, len(entries))
	for _, r := range renewals {
		at := -1
		for i, old := range s.Entries {
			if !done[i] && old == r.Old {
				at = i
				break
			}
		}
		if at < 0 {
			return mutants.ExceptionFailed{Cause: fmt.Errorf("no entry for %s in %s to renew", r.Old.Function, r.Old.File)}
		}
		entries[at] = r.New
		done[at] = true
	}
	s.Entries = entries
	s.Writes++
	return nil
}
