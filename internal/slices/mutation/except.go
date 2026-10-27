package mutation

import (
	"context"
	"errors"
	"path/filepath"
	"slices"

	"github.com/donvargax/itos-cc/internal/selection"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// ExceptQuery carries except's domain values once: the survivor's site, as
// the person named it, by its file, line and column or by its Identity, as
// a counted run reports it, and the reason no test can kill it, nil when
// none was given.
type ExceptQuery struct {
	Path         string
	Line, Column int
	Identity     *mutants.Identity
	Reason       *string
}

// ExceptFound is the entry except wrote, and whether it replaced one, and
// the site excepted, as the source is now: the path of its file, its line
// and column. File is the identity's file, slash-separated from the project root on
// every system as the identity names it, which names the file in place of
// Path when an identity named the site, "" when a site did.
type ExceptFound struct {
	Exception    mutants.Exception
	Replaced     bool
	Path         string
	Line, Column int
	File         string
}

// RenewFound is what --renew renewed and left, the settings' root naming
// their files.
type RenewFound struct {
	Root    string
	Renewed []mutants.Renewal
	Left    []mutants.Unrenewed
}

// PruneFound is what --prune removed, the settings' root naming their
// files.
type PruneFound struct {
	Root   string
	Pruned []mutants.StaleException
}

// RehashFound is what --rehash carried over and left, the settings' root
// naming their files.
type RehashFound struct {
	Root     string
	Rehashed []mutants.Renewal
	Left     []mutants.StaleException
}

// errNoExceptions is the missing Exceptions binding, worded as cmd's was.
var errNoExceptions = errors.New("mutation Exceptions binding is missing")

// Except observes in the cmd handler's order: settings, then the site's
// exclusion, then the bindings, the listed tests' support files and the
// tests that reach each source, then the domain's survivor at the site, or
// the one an identity names, its file from the settings' root, running
// nothing, whose entry, with the reason, goes through the Exceptions writer
// made for the settings' root. When no entry is for the site and one stale
// entry names its file, function and change (Repointed), the survivor's
// entry replaces it, keeping its reason when none is given. Given no
// reason and re-pointing none, every refusal is ReasonMissing. A refusal is
// a Failure the UI words.
func (h Handler) Except(ctx context.Context, q ExceptQuery) (ExceptFound, error) {
	root, cfg, err := h.settings(ctx)
	if err != nil {
		return ExceptFound{}, err
	}
	path, identity, file := sourcePath(h.Dir, q.Path), "", ""
	if q.Identity != nil {
		path, identity, file = sourcePath(root, filepath.FromSlash(q.Identity.File)), q.Identity.String(), q.Identity.File
	}
	if selection.Exclusion(cfg.Selection).Excludes(path) {
		if q.Reason == nil {
			return ExceptFound{}, ReasonMissing{}
		}
		return ExceptFound{}, SiteExcluded{Path: path, Line: q.Line, Column: q.Column, Identity: identity, File: file}
	}
	support, tests, err := h.judging(ctx, root, cfg)
	if err != nil {
		return ExceptFound{}, err
	}
	opt := h.options(root, nil, tests, support, nil, h.listing(root, cfg))
	found := ExceptFound{Path: path, Line: q.Line, Column: q.Column, File: file}
	var e mutants.Exception
	if q.Identity != nil {
		e, found.Line, err = mutants.IdentifiedSurvivor(ctx, *q.Identity, opt, h.Status)
		found.Column = e.Column
	} else {
		e, err = mutants.Survivor(ctx, q.Path, q.Line, q.Column, opt, h.Status)
	}
	if refused := survivorRefused(err, q, path); refused != nil {
		return ExceptFound{}, refused
	}
	if err != nil {
		return ExceptFound{}, err
	}
	stale := -1
	if !slices.ContainsFunc(cfg.Exceptions, e.SameSite) {
		if stale, err = mutants.Repointed(ctx, cfg.Exceptions, e, path, opt); err != nil {
			return ExceptFound{}, err
		}
	}
	switch {
	case q.Reason != nil:
		e.Reason = *q.Reason
	case stale >= 0:
		e.Reason = cfg.Exceptions[stale].Reason
	default:
		return ExceptFound{}, ReasonMissing{}
	}
	writer, err := h.exceptions(root)
	if err != nil {
		return ExceptFound{}, err
	}
	if stale >= 0 {
		// The stale entry is replaced whole, as --renew replaces one.
		if failure := writer.Renew(ctx, []mutants.Renewal{{Old: cfg.Exceptions[stale], New: e}}); failure != nil {
			return ExceptFound{}, failure
		}
		found.Exception, found.Replaced = e, true
		return found, nil
	}
	replaced, failure := writer.Except(ctx, e)
	if failure != nil {
		return ExceptFound{}, failure
	}
	found.Exception, found.Replaced = e, replaced
	return found, nil
}

// survivorRefused is the Failure of err, the domain's refusal of q's
// survivor in the file at path, nil for any other error: ReasonMissing
// when q gives no reason, else NoSurvivor, at the site the identity named
// where it did, or IdentityStale.
func survivorRefused(err error, q ExceptQuery, path string) Failure {
	var none *mutants.NoSurvivorError
	var stale *mutants.IdentityStaleError
	switch {
	case !errors.As(err, &none) && !errors.As(err, &stale):
		return nil
	case q.Reason == nil:
		return ReasonMissing{}
	case stale != nil:
		return IdentityStale{Path: path, Identity: q.Identity.String(), Function: q.Identity.Function(), Why: stale.Why, Hash: stale.Hash, File: q.Identity.File}
	case q.Identity != nil:
		return NoSurvivor{Path: path, Line: none.Line, Column: none.Column, Reason: none.Reason, Uncovered: none.Uncovered, File: q.Identity.File}
	}
	return NoSurvivor{Path: path, Line: q.Line, Column: q.Column, Reason: none.Reason, Uncovered: none.Uncovered}
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
	renewed, left, err := mutants.Renew(ctx, cfg.Exceptions, sources, h.options(root, nil, tests, support, nil, h.listing(root, cfg)), h.Status)
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

// Rehash reads the settings, returning at once when they hold no
// exception, then judges each entry that no longer holds by its own file
// alone, running nothing and reading no snapshot (mutants.Rehash): those
// written under itos-cc's previous hash, whose function changed only in
// what the hash now leaves out, alone go through the Exceptions writer made
// for the settings' root, which gives each its function's hash now.
func (h Handler) Rehash(ctx context.Context) (RehashFound, error) {
	root, cfg, err := h.settings(ctx)
	if err != nil {
		return RehashFound{}, err
	}
	if len(cfg.Exceptions) == 0 {
		return RehashFound{Root: root}, nil
	}
	rehashed, left, err := mutants.Rehash(ctx, cfg.Exceptions, h.options(root, nil, nil, nil, nil, nil), h.Status)
	if err != nil {
		return RehashFound{}, err
	}
	writer, err := h.exceptions(root)
	if err != nil {
		return RehashFound{}, err
	}
	if failure := writer.Rehash(ctx, rehashed); failure != nil {
		return RehashFound{}, failure
	}
	return RehashFound{Root: root, Rehashed: rehashed, Left: left}, nil
}

// Prune observes as Renew does, then judges every entry as a check of the
// whole project does: those whose file is no source it selects, under the
// root less what exclude leaves out, as gone, moved or excluded; the rest
// by their file's sources and fresh results. The entries stale as gone,
// killed or excluded alone go through the Exceptions writer made for the
// settings' root, which removes them.
func (h Handler) Prune(ctx context.Context) (PruneFound, error) {
	root, cfg, err := h.settings(ctx)
	if err != nil {
		return PruneFound{}, err
	}
	if len(cfg.Exceptions) == 0 {
		return PruneFound{Root: root}, nil
	}
	support, tests, err := h.judging(ctx, root, cfg)
	if err != nil {
		return PruneFound{}, err
	}
	files, err := h.files(ctx, SelectionQuery{Paths: []string{root}})
	if err != nil {
		return PruneFound{}, err
	}
	exclude, err := h.exclusion(ctx)
	if err != nil {
		return PruneFound{}, err
	}
	every := func(string) bool { return true }
	excluded := func(file string) bool { return exclude.Excludes(sourcePath(root, file)) }
	stale, moved, err := mutants.Elsewhere(ctx, cfg.Exceptions, files.Sources, every, excluded, h.options(root, nil, nil, nil, nil, nil))
	if err != nil {
		return PruneFound{}, err
	}
	checks, err := mutants.Check(ctx, files.Sources, h.options(root, nil, tests, support, append(slices.Clone(cfg.Exceptions), moved...), h.listing(root, cfg)))
	if err != nil {
		return PruneFound{}, err
	}
	for _, c := range checks {
		stale = append(stale, c.StaleExceptions...)
	}
	pruned := mutants.Pruned(cfg.Exceptions, stale)
	writer, err := h.exceptions(root)
	if err != nil {
		return PruneFound{}, err
	}
	entries := make([]mutants.Exception, len(pruned))
	for i, s := range pruned {
		entries[i] = s.Exception
	}
	if failure := writer.Prune(ctx, entries); failure != nil {
		return PruneFound{}, failure
	}
	return PruneFound{Root: root, Pruned: pruned}, nil
}

// judging is what except, renew and prune judge a survivor with, in cmd's order:
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
