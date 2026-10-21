package mutation

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/snapshot"
)

func (h Handler) elsewhere(ctx context.Context, q SelectionQuery, root string, exceptions []mutants.Exception, sources []string, renamed func(string) string) ([]mutants.StaleException, []mutants.Exception, error) {
	if len(exceptions) == 0 {
		return nil, nil, nil
	}
	key := func(path string) string { return snapshot.Key(root, sourcePath(h.Dir, path)) }
	var judge func(string) bool
	switch {
	case q.Since != nil:
		removed := map[string]bool{}
		for _, path := range sources {
			if old := renamed(path); old != "" {
				removed[key(old)] = true
			}
		}
		if len(q.Paths) == 0 {
			if h.History == nil {
				return nil, nil, errors.New("mutation history binding is missing")
			}
			deleted, err := h.History.DeletedSince(ctx, *q.Since)
			if err != nil {
				return nil, nil, err
			}
			for _, path := range deleted {
				removed[key(path)] = true
			}
		}
		// Stored identity is intentionally not newly normalized.
		judge = func(file string) bool { return removed[file] }
	case len(q.Paths) == 0 && !q.Changed:
		dir := h.Dir
		judge = func(file string) bool {
			rel, err := filepath.Rel(dir, sourcePath(root, file))
			return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
		}
	default:
		return nil, nil, nil
	}
	exclude, err := h.exclusion(ctx)
	if err != nil {
		return nil, nil, err
	}
	return mutants.Elsewhere(ctx, exceptions, sources, judge, func(file string) bool { return exclude.Excludes(sourcePath(root, file)) }, h.options(root, nil, nil, nil, nil))
}
