package mutation

import (
	"context"
	"errors"

	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/selection"
)

// ExceptQuery carries except's domain values once: the survivor's site, as
// the person named it, and the reason no test can kill it.
type ExceptQuery struct {
	Path         string
	Line, Column int
	Reason       string
}

// ExceptFound is the entry except wrote, and whether it replaced one.
type ExceptFound struct {
	Exception mutants.Exception
	Replaced  bool
}

// RenewFound is what --renew renewed and left, the settings' root naming
// their files.
type RenewFound struct {
	Root    string
	Renewed []mutants.Renewal
	Left    []mutants.Unrenewed
}

// errNoExceptions is the missing Exceptions binding, worded as cmd's was.
var errNoExceptions = errors.New("mutation Exceptions binding is missing")

// Except observes in the cmd handler's order: settings, then the site's
// exclusion, then the bindings, the listed tests' support files and the
// tests that reach each source, then the domain's survivor at the site,
// whose entry, with the reason, goes through the Exceptions writer made for
// the settings' root. A refusal is a Failure the UI words.
func (h Handler) Except(ctx context.Context, q ExceptQuery) (ExceptFound, error) {
	root, cfg, err := h.settings(ctx)
	if err != nil {
		return ExceptFound{}, err
	}
	path := sourcePath(h.Dir, q.Path)
	if (selection.Exclusion{Root: root, Globs: cfg.Exclude}).Excludes(path) {
		return ExceptFound{}, SiteExcluded{Path: path, Line: q.Line, Column: q.Column}
	}
	support, tests, err := h.judging(ctx, root, cfg)
	if err != nil {
		return ExceptFound{}, err
	}
	e, err := mutants.Survivor(ctx, q.Path, q.Line, q.Column, h.options(root, nil, tests, support, nil), h.Status)
	var none *mutants.NoSurvivorError
	if errors.As(err, &none) {
		return ExceptFound{}, NoSurvivor{Path: path, Line: q.Line, Column: q.Column, Reason: none.Reason}
	}
	if err != nil {
		return ExceptFound{}, err
	}
	e.Reason = q.Reason
	writer, err := h.exceptions(root)
	if err != nil {
		return ExceptFound{}, err
	}
	replaced, failure := writer.Except(ctx, e)
	if failure != nil {
		return ExceptFound{}, failure
	}
	return ExceptFound{Exception: e, Replaced: replaced}, nil
}

// Renew observes in the cmd handler's order: settings, returning at once
// when they hold no exception, then the bindings, the listed tests' support
// files and the tests that reach each source, then every source under the
// root through the Tree, then the domain's renewals, which alone go through
// the Exceptions writer made for the settings' root.
func (h Handler) Renew(ctx context.Context) (RenewFound, error) {
	root, cfg, err := h.settings(ctx)
	if err != nil {
		return RenewFound{}, err
	}
	if len(cfg.Exceptions) == 0 {
		return RenewFound{Root: root}, nil
	}
	support, tests, err := h.judging(ctx, root, cfg)
	if err != nil {
		return RenewFound{}, err
	}
	sources, _, failure := h.Tree.Discover(ctx, []string{root})
	if failure != nil {
		return RenewFound{}, failure
	}
	renewed, left, err := mutants.Renew(ctx, cfg.Exceptions, sources, h.options(root, nil, tests, support, nil), h.Status)
	if err != nil {
		return RenewFound{}, err
	}
	writer, err := h.exceptions(root)
	if err != nil {
		return RenewFound{}, err
	}
	if failure := writer.Renew(ctx, renewed); failure != nil {
		return RenewFound{}, failure
	}
	return RenewFound{Root: root, Renewed: renewed, Left: left}, nil
}

// judging is what except and renew judge a survivor with, in cmd's order:
// the Toolchain and Fingerprints bindings, the listed tests' support files,
// then the tests that reach each source. importingTests has checked the
// Tree binding once it returns.
func (h Handler) judging(ctx context.Context, root string, cfg *mutants.Config) (map[string]string, func(string) []string, error) {
	if h.Toolchain == nil {
		return nil, nil, errors.New("mutation Toolchain binding is missing")
	}
	if h.Fingerprints == nil {
		return nil, nil, errors.New("mutation Fingerprints binding is missing")
	}
	var support map[string]string
	if cfg.Tests != nil {
		var failure mutants.InputFailure
		if support, failure = h.Fingerprints.Support(ctx, root, cfg.Tests.Support); failure != nil {
			return nil, nil, failure
		}
	}
	tests, err := h.importingTests(ctx, root)
	if err != nil {
		return nil, nil, err
	}
	return support, tests, nil
}

// exceptions is the writer cmd's binding makes for root.
func (h Handler) exceptions(root string) (mutants.Exceptions, error) {
	if h.Exceptions == nil {
		return nil, errNoExceptions
	}
	writer := h.Exceptions(root)
	if writer == nil {
		return nil, errNoExceptions
	}
	return writer, nil
}
