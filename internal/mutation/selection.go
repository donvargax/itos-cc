package mutation

import (
	"context"
	"errors"
	"fmt"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/mutants"
)

func (h Handler) selectSources(ctx context.Context, q SelectionQuery) (sources []string, judge func(string, string, string) bool, renamed func(string) string, lines mutants.LineScope, excluded int, err error) {
	var since map[string]mutants.ChangedFunctions
	var moves map[string]string
	if q.Since != nil {
		if q.Changed {
			return nil, nil, nil, nil, 0, cli.NewProblem(cli.CodeUsage, "flags.conflict", "--since and --changed cannot be combined: --since judges the functions of commits, --changed whole files of the working tree", "Drop --changed; commit the work to judge it with --since.").With("flag", "--changed")
		}
		if h.History == nil {
			return nil, nil, nil, nil, 0, errors.New("mutation history binding is missing")
		}
		var failure mutants.HistoryFailure
		since, moves, failure = h.History.ChangedSince(ctx, *q.Since)
		if failure != nil {
			return nil, nil, nil, nil, 0, sinceError(failure, *q.Since)
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
func sinceError(failure mutants.HistoryFailure, ref string) error {
	switch failure := failure.(type) {
	case mutants.BadReference:
		return cli.NewProblem(cli.CodeUsage, "since.bad-ref", fmt.Sprintf("--since %s: not a commit in this repository", ref), "Name a branch, tag, or commit, such as origin/main; fetch a remote one first.").With("ref", ref)
	case mutants.NoRepository:
		return cli.NewProblem(cli.CodeEnvironment, "since.no-git", "--since needs a git repository: "+failure.Reason, "Run it inside a git repository, or name the paths instead.")
	case mutants.HistoryCancelled:
		return failure.Cause
	case mutants.HistoryFailed:
		return failure.Cause
	}
	return failure
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
