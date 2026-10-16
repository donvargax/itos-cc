package mutation

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/internal/mutants"
)

func (h Handler) orphans(ctx context.Context, q SelectionQuery) ([]mutants.Orphan, error) {
	exclude, err := h.exclusion(ctx)
	if err != nil {
		return nil, err
	}
	var dirs, fragments []string
	for _, path := range q.Paths {
		if h.Status == nil {
			return nil, errors.New("mutation source status binding is missing")
		}
		kind, failure := h.Status.Kind(ctx, sourcePath(h.Dir, path))
		var canceled mutants.InputCancelled
		if errors.As(failure, &canceled) {
			return nil, canceled.Cause
		}
		switch {
		case failure != nil || kind == mutants.SourceMissing:
			fragments = append(fragments, filepath.ToSlash(path))
		case kind == mutants.SourceDirectory:
			dirs = append(dirs, sourcePath(h.Dir, path))
		}
	}
	if len(q.Paths) == 0 {
		dirs = []string{h.Dir}
	}
	wd := h.Dir
	found, err := mutants.Orphans(ctx, h.Catalog, h.Presence, func(path string) bool {
		for _, dir := range dirs {
			if beneath(dir, path) {
				return true
			}
		}
		if !beneath(wd, path) {
			return false
		}
		rel, err := filepath.Rel(wd, path)
		if err != nil {
			return false
		}
		for _, fragment := range fragments {
			if strings.Contains(filepath.ToSlash(rel), fragment) {
				return true
			}
		}
		return false
	}, exclude.Excludes)
	if err != nil || len(found) == 0 {
		return found, err
	}
	ref := ""
	if q.Since != nil {
		ref = *q.Since
	}
	return mutants.NotRenamed(ctx, found, h.History, h.Presence, ref, q.Since != nil)
}

// Orphan is the existing read-only command representation of an orphan.
type Orphan struct {
	File     string `json:"file"`
	Snapshot string `json:"snapshot"`
	Why      string `json:"why"`
}
