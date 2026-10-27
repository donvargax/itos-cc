package domain

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	fileselection "github.com/donvargax/itos-cc/internal/selection"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
)

// FreshNamedAlgorithm is the algorithm of a plan whose sites are named,
// not drawn (mutation run --site): it ranks nothing.
const FreshNamedAlgorithm = "named"

// FreshSite is a mutant a --site run names, Argument as written: a
// position, File, slash-separated from the repository's top, Line and
// Column, naming every mutant there, or, with Identity, the one mutant a
// counted run's identity names, File the identity's file.
type FreshSite struct {
	Argument     string
	File         string
	Line, Column int
	Identity     *Identity
}

// FreshNamed is an argument of a --site run and the identities of the
// mutants it named, in their order.
type FreshNamed struct {
	Argument   string
	Identities []string
}

// Why a named site names no mutant (UnmatchedSite).
const (
	// UnmatchedNoSite: no mutation site is at the position, as in a test
	// file.
	UnmatchedNoSite = "no-site"
	// UnmatchedGone: the file is not in the commit, or the identity's
	// function or site is gone.
	UnmatchedGone = ExceptionGone
	// UnmatchedChanged: a function of the identity's name is there with
	// another hash.
	UnmatchedChanged = ExceptionChanged
	// UnmatchedExcluded: the settings' exclude leaves the source out.
	UnmatchedExcluded = "excluded"
	// UnmatchedNotBuilt: only another system builds the Go file.
	UnmatchedNotBuilt = "not-built"
)

// UnmatchedSite is an argument of a --site run that names no mutant of
// the commit, Why saying why: with UnmatchedChanged, Hash is the
// function's hash now, and with UnmatchedNotBuilt, Platform is this
// system's.
type UnmatchedSite struct {
	Argument, Why, Hash string
	Platform            Platform
}

// FreshUnmatched is the plan of a --site run refused because arguments
// name no mutant: every such argument, in the order given.
type FreshUnmatched struct{ Sites []UnmatchedSite }

func (FreshUnmatched) freshFailure() {}
func (f FreshUnmatched) Error() string {
	var parts []string
	for _, s := range f.Sites {
		parts = append(parts, s.Argument+" ("+s.Why+")")
	}
	return "named sites match no mutant: " + strings.Join(parts, ", ")
}

// namedFile is a committed file a named site resolves in: parsed, or why
// none of its sites can be named.
type namedFile struct {
	file     *lang.File
	why      string
	platform Platform
}

// AdmitNamed admits the mutants sites name, each argument resolved in the
// frozen commit before anything runs, as AdmitFresh admits a drawn
// selection: the named mutants are Eligible and Selected, in the order
// first named, a mutant named twice once; Units are their functions alone;
// nothing is ranked or omitted, and Named records each argument with the
// identities it named. An argument that names no mutant is an
// UnmatchedSite, and any makes the error a FreshUnmatched naming every
// one.
func AdmitNamed(ctx context.Context, plan *FreshPlan, tree selport.Tree, around langport.Surroundings, tools Toolchain, sources, tracked []string, exclude fileselection.Exclusion, sites []FreshSite) error {
	committed := make(map[string]bool, len(tracked))
	for _, rel := range tracked {
		committed[rel] = true
	}
	// The discovered sources, by their paths in the frozen root.
	source := make(map[string]bool, len(sources))
	for _, discovered := range sources {
		source[filepath.Clean(discovered)] = true
	}
	plan.Seed, plan.Algorithm, plan.NotBuilt = "", FreshNamedAlgorithm, nil
	files := map[string]*namedFile{}
	defer func() {
		for _, f := range files {
			if f.file != nil {
				f.file.Close()
			}
		}
	}()
	var unmatched []UnmatchedSite
	var units []FreshUnit
	seen, unitSeen := map[string]bool{}, map[string]bool{}
	plan.Eligible, plan.Selected, plan.Omitted, plan.Named = nil, nil, nil, nil
	for _, named := range sites {
		rel := path.Clean(named.File)
		f, opened := files[rel]
		if !opened {
			// A cancelled context stops the read of its first file.
			var err error
			f, err = openNamed(ctx, plan, tree, around, tools, rel, committed[rel], exclude, source)
			files[rel] = f
			if err != nil {
				return err
			}
		}
		found, miss := namedCandidates(f, named)
		if miss != nil {
			miss.Argument = named.Argument
			unmatched = append(unmatched, *miss)
			continue
		}
		entry := FreshNamed{Argument: named.Argument, Identities: []string{}}
		for _, s := range found {
			unit := f.file.Units[s.Unit]
			function := stableFunction(f.file, unit, rel, committed)
			hash := UnitHash(f.file, unit)
			unitIdentity := fmt.Sprintf("%s@%d:%d:%s", function, unit.StartLine, unit.EndLine, hash)
			identity := fmt.Sprintf("%s:%s:%s", rel, unitIdentity, s.Key())
			entry.Identities = append(entry.Identities, identity)
			if !unitSeen[unitIdentity] {
				unitSeen[unitIdentity] = true
				units = append(units, FreshUnit{Path: rel, Function: function, Identity: unitIdentity,
					StartLine: unit.StartLine, EndLine: unit.EndLine, Hash: hash})
			}
			if !seen[identity] {
				seen[identity] = true
				plan.Selected = append(plan.Selected, FreshCandidate{Identity: identity, Path: rel, Function: function,
					UnitIdentity: unitIdentity, Site: s})
			}
		}
		plan.Named = append(plan.Named, entry)
	}
	if len(unmatched) > 0 {
		return FreshUnmatched{Sites: unmatched}
	}
	plan.Eligible = slices.Clone(plan.Selected)
	slices.SortFunc(units, func(a, b FreshUnit) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), cmp.Compare(a.StartLine, b.StartLine))
	})
	plan.Units = units
	return nil
}

// openNamed is the committed file rel of plan's frozen root, parsed, or why
// no site of it can be named: gone when it is not committed, excluded when
// exclude leaves its source out, no-site when it is no source (a test, or
// a language with none), not-built when only another system builds it.
// A file that cannot be read, a platform that cannot be told, or source
// that cannot be parsed is an error, beside a namedFile holding nothing.
func openNamed(ctx context.Context, plan *FreshPlan, tree selport.Tree, around langport.Surroundings, tools Toolchain, rel string, committed bool, exclude fileselection.Exclusion, source map[string]bool) (*namedFile, error) {
	frozen := filepath.Join(plan.FrozenRoot, filepath.FromSlash(rel))
	switch {
	case !committed:
		return &namedFile{why: UnmatchedGone}, nil
	case exclude.Excludes(frozen):
		return &namedFile{why: UnmatchedExcluded}, nil
	case !source[frozen]:
		return &namedFile{why: UnmatchedNoSite}, nil
	}
	text, failure := tree.Read(ctx, frozen)
	if failure != nil {
		return &namedFile{}, fmt.Errorf("read committed %s: %w", rel, failure)
	}
	built, platform, err := builtHere(ctx, tools, map[string]Platform{}, frozen, text)
	if err != nil || !built {
		return &namedFile{why: UnmatchedNotBuilt, platform: platform}, err
	}
	file, err := lang.Parse(ctx, around, lang.Detect(rel), frozen, text)
	return &namedFile{file: file}, err
}

// namedCandidates are the sites of f named names: every site at its
// position, or the one site of its identity, found as mutation except
// finds it (identifiedUnit), its function by name and hash, its lines
// choosing among several. None is an UnmatchedSite saying why.
func namedCandidates(f *namedFile, named FreshSite) ([]Site, *UnmatchedSite) {
	if f.file == nil {
		why := f.why
		if why == UnmatchedNoSite && named.Identity != nil {
			// An identity's function is no unit of a file that is no
			// source: it is gone.
			why = UnmatchedGone
		}
		return nil, &UnmatchedSite{Why: why, Platform: f.platform}
	}
	var found []Site
	if named.Identity == nil {
		for _, s := range Sites(f.file) {
			if s.Line == named.Line && s.Column == named.Column {
				found = append(found, s)
			}
		}
		if len(found) == 0 {
			return nil, &UnmatchedSite{Why: UnmatchedNoSite}
		}
		return found, nil
	}
	unit, err := identifiedUnit(f.file, *named.Identity)
	var stale *IdentityStaleError
	if errors.As(err, &stale) {
		return nil, &UnmatchedSite{Why: stale.Why, Hash: stale.Hash}
	}
	for _, s := range Sites(f.file) {
		if s.Unit == unit && s.Key() == named.Identity.Key {
			return []Site{s}, nil
		}
	}
	return nil, &UnmatchedSite{Why: UnmatchedGone}
}
