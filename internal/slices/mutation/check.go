package mutation

import (
	"context"
	"errors"
	"slices"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// CheckQuery carries the domain-valued selection and coverage judgment mode.
type CheckQuery struct {
	SelectionQuery
	FailUncovered, LinesOnly bool
}

// CheckFound retains admitted domain results, never formatted partial checks.
type CheckFound struct {
	Root      string
	Files     []mutants.FileCheck
	Coverage  []mutants.CoverageCheck
	Elsewhere []mutants.StaleException
	Orphans   []mutants.Orphan
	Lines     mutants.LineScope
	Excluded  int
	NoSources bool
}

// Check observes in the cmd handler's order: settings, then test support,
// then the lines-without-since refusal, then selection. A refusal is a
// Failure the UI words; the handler builds no problem and picks no exit code.
func (h Handler) Check(ctx context.Context, q CheckQuery) (CheckFound, error) {
	found := CheckFound{}
	root, cfg, err := h.settings(ctx)
	if err != nil {
		return found, err
	}
	found.Root = root
	var support map[string]string
	if cfg.Tests != nil {
		if h.Fingerprints == nil {
			return found, errors.New("mutation Fingerprints binding is missing")
		}
		var failure mutants.InputFailure
		support, failure = h.Fingerprints.Support(ctx, root, cfg.Tests.Support)
		if failure != nil {
			return found, failure
		}
	}
	if q.LinesOnly && q.Since == nil {
		return found, LinesWithoutSince{}
	}
	sources, judge, renamed, lines, excluded, err := h.selectSources(ctx, q.SelectionQuery)
	if err != nil {
		return found, err
	}
	found.Excluded = excluded
	if q.LinesOnly {
		found.Lines = lines
	}
	elsewhere, moved, err := h.elsewhere(ctx, q.SelectionQuery, root, cfg.Exceptions, sources, renamed)
	if err != nil {
		return found, err
	}
	found.Elsewhere = elsewhere
	found.Orphans, err = h.orphans(ctx, q.SelectionQuery)
	if err != nil {
		return found, err
	}
	if len(sources) == 0 {
		found.NoSources = true
		return found, nil
	}
	if h.Toolchain == nil {
		return found, errors.New("mutation Toolchain binding is missing")
	}
	if h.Fingerprints == nil {
		return found, errors.New("mutation Fingerprints binding is missing")
	}
	if h.Tree == nil || h.Surroundings == nil {
		return found, errors.New("mutation source tree binding is missing")
	}
	tests, err := h.importingTests(ctx, root)
	if err != nil {
		return found, err
	}
	checks, err := mutants.Check(ctx, sources, h.options(root, judge, tests, support, append(slices.Clone(cfg.Exceptions), moved...), h.listing(root, cfg)))
	if err != nil {
		return found, err
	}
	if languages := StrictLanguages(sources); q.FailUncovered && len(languages) > 0 {
		enabled := map[string]bool{}
		for _, language := range languages {
			enabled[language] = true
		}
		found.Coverage, err = mutants.CheckCoverage(ctx, sources, h.options(root, judge, nil, support, nil, nil), enabled, false, false)
		if err != nil {
			return found, err
		}
	}
	found.Files = checks
	return found, nil
}
