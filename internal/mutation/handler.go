package mutation

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	langport "github.com/donvargax/itos-cc/internal/lang/port"
	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/selection"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
)

// Handler holds only actual domain capabilities. Cmd selects the adapters;
// settings are reread at their existing admission points, never cached here.
type Handler struct {
	Settings     mutants.SettingsReader
	Tree         selport.Tree
	Surroundings langport.Surroundings
	Store        mutants.MutationSnapshotReader
	Catalog      mutants.MutationCatalog
	Presence     mutants.SourcePresence
	Status       mutants.SourceStatus
	History      mutants.History
	Toolchain    mutants.Toolchain
	Fingerprints mutants.Fingerprints
	Reach        mutants.Reach
	Workers      mutants.Workers
	// Exceptions makes the writer of the settings at a root, which except
	// and renew update; cmd selects it.
	Exceptions func(root string) mutants.Exceptions
	Dir        string
}

// SelectionQuery retains the selection's explicit optional ref, including empty.
type SelectionQuery struct {
	Paths   []string
	Changed bool
	Since   *string
}

func (h Handler) settings(ctx context.Context) (string, *mutants.Config, error) {
	if h.Settings == nil {
		return "", nil, errors.New("mutation settings binding is missing")
	}
	root, config, failure := h.Settings.Read(ctx)
	if failure != nil {
		return root, nil, failure
	}
	if config == nil {
		return root, nil, errors.New("mutation settings binding returned no value")
	}
	return root, config, nil
}
func (h Handler) exclusion(ctx context.Context) (selection.Exclusion, error) {
	root, config, err := h.settings(ctx)
	if err != nil {
		return selection.Exclusion{}, err
	}
	return selection.Exclusion{Root: root, Globs: config.Exclude}, nil
}
func (h Handler) files(ctx context.Context, q SelectionQuery) (selection.Files, error) {
	exclude, err := h.exclusion(ctx)
	if err != nil {
		return selection.Files{}, err
	}
	if h.Tree == nil {
		return selection.Files{}, errors.New("mutation source tree binding is missing")
	}
	return selection.Select(ctx, h.Tree, q.Paths, q.Changed, exclude)
}
func sourcePath(dir, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(dir, path)
}
func (h Handler) options(root string, judge func(string, string, string) bool, tests func(string) []string, support map[string]string, exceptions []mutants.Exception) mutants.CheckOptions {
	return mutants.CheckOptions{Store: h.Store, Tree: h.Tree, Surroundings: h.Surroundings, Toolchain: h.Toolchain, Fingerprints: h.Fingerprints, Root: root, Dir: h.Dir, Judge: judge, Tests: tests, Support: support, Exceptions: exceptions}
}
func beneath(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
