package mutants

import (
	"context"

	"github.com/donvargax/itos-cc/internal/lang"
)

// History reads committed mutation inputs from one caller's repository.
// ChangedSince is caller-relative ref...HEAD; rename queries are repository-wide.
// Implementations return owned maps and bytes, never scratch paths or handles.
type History interface {
	Head(ctx context.Context) (string, HistoryFailure)
	Resolve(ctx context.Context, ref string) (string, HistoryFailure)
	ReadCommitted(ctx context.Context, ref, path string) ([]byte, HistoryFailure)
	ChangedSince(ctx context.Context, ref string) (map[string]ChangedFunctions, map[string]string, HistoryFailure)
	DeletedSince(ctx context.Context, ref string) ([]string, HistoryFailure)
	RenamesSince(ctx context.Context, ref string) (map[string]string, HistoryFailure)
	RenamesUncommitted(ctx context.Context) (map[string]string, HistoryFailure)
}

// ChangedFunctions identifies changed functions separately from changed new lines.
type ChangedFunctions struct {
	// Functions names each namespace#name whose lines the range touched.
	Functions map[string]bool
	// Kept holds untouched HEAD hashes when several units share a changed name.
	Kept map[string]map[string]bool
	// Lines is only each hunk's new-side added/changed range. A pure deletion
	// admits adjacent functions but contributes no executable-line obligation.
	Lines []LineRange
}

func (c ChangedFunctions) Changes(start, end int) bool { return OverlapsRanges(c.Lines, start, end) }
func (c ChangedFunctions) Judges(function, hash string) bool {
	return c.Functions[function] && !c.Kept[function][hash]
}

// HistoryFailure is a repository observation failure, not a mutation verdict.
//
//sumtype:decl
type HistoryFailure interface {
	error
	historyFailure()
}

type NoRepository struct{ Reason string }

func (NoRepository) historyFailure() {}
func (f NoRepository) Error() string { return "not a git repository: " + f.Reason }

type BadReference struct{ Ref string }

func (BadReference) historyFailure() {}
func (BadReference) Error() string   { return "not a commit in this repository" }

type HistoryCancelled struct{ Cause error }

func (HistoryCancelled) historyFailure() {}
func (f HistoryCancelled) Error() string { return f.Cause.Error() }
func (f HistoryCancelled) Unwrap() error { return f.Cause }

type HistoryFailed struct{ Cause error }

func (HistoryFailed) historyFailure() {}
func (f HistoryFailed) Error() string { return f.Cause.Error() }
func (f HistoryFailed) Unwrap() error { return f.Cause }

// ChangedFunctionsOf pairs HEAD units with independently observed diff ranges.
// Pure deletion ranges admit neighboring functions but never changed new lines.
func ChangedFunctionsOf(file *lang.File, ranges []LineRange, hash func(*lang.File, lang.Unit) string) ChangedFunctions {
	c := ChangedFunctions{Functions: map[string]bool{}}
	for _, r := range ranges {
		if !r.Deletion {
			c.Lines = append(c.Lines, r)
		}
	}
	named := map[string]int{}
	for _, u := range file.Units {
		id := u.Namespace + "#" + u.Name
		named[id]++
		if OverlapsRanges(ranges, u.StartLine, u.EndLine) {
			c.Functions[id] = true
		}
	}
	for _, u := range file.Units {
		id := u.Namespace + "#" + u.Name
		if hash == nil || named[id] < 2 || !c.Functions[id] || OverlapsRanges(ranges, u.StartLine, u.EndLine) {
			continue
		}
		if c.Kept == nil {
			c.Kept = map[string]map[string]bool{}
		}
		if c.Kept[id] == nil {
			c.Kept[id] = map[string]bool{}
		}
		c.Kept[id][hash(file, u)] = true
	}
	return c
}
