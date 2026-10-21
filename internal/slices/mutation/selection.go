package mutation

import (
	"context"
	"errors"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

func (h Handler) selectSources(ctx context.Context, q SelectionQuery) (sources []string, judge func(string, string, string) bool, renamed func(string) string, lines mutants.LineScope, excluded int, err error) {
	var since map[string]mutants.ChangedFunctions
	var moves map[string]string
	if q.Since != nil {
		if q.Changed {
			return nil, nil, nil, nil, 0, SinceWithChanged{}
		}
		if h.History == nil {
			return nil, nil, nil, nil, 0, errors.New("mutation history binding is missing")
		}
		var failure mutants.HistoryFailure
		since, moves, failure = h.History.ChangedSince(ctx, *q.Since)
		if failure != nil {
			return nil, nil, nil, nil, 0, SinceUnavailable{Ref: *q.Since, Failure: failure}
		}
	}
	files, err := h.files(ctx, q)
	if err != nil {
		return nil, nil, nil, nil, 0, err
	}
	if since == nil {
		return files.Sources, nil, nil, nil, files.Excluded, nil
	}
	for _, file := range files.Sources {
		if _, ok := since[file]; ok {
			sources = append(sources, file)
		}
	}
	dir := h.Dir
	return sources, func(path, function, hash string) bool { return since[path].Judges(function, hash) }, func(path string) string { return moves[path] }, func(path string, start, end int) bool { return since[sourcePath(dir, path)].Changes(start, end) }, files.Excluded, nil
}
func (h Handler) importingTests(ctx context.Context, root string) (func(string) []string, error) {
	if h.Reach == nil {
		return nil, errors.New("mutation Reach binding is missing")
	}
	if h.Tree == nil {
		return nil, errors.New("mutation source tree binding is missing")
	}
	sources, tests, failure := h.Tree.Discover(ctx, []string{root})
	if failure != nil {
		return nil, failure
	}
	reach, err := h.Reach.TestsImporting(ctx, root, sources, tests)
	if err != nil {
		return nil, err
	}
	dir := h.Dir
	return func(path string) []string { return reach[sourcePath(dir, path)] }, nil
}
