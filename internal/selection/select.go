// Package selection owns how a command chooses its files: the paths it is
// given, fragments of a path, the files git reports as changed, and the
// sources the project's settings exclude. It reaches the working tree only
// through its port.
package selection

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/donvargax/itos-cc/internal/selection/port"
)

// Files are the selected files, split into production sources and tests,
// and the count of the sources the selection left out by its Exclusion.
type Files struct {
	Sources  []string
	Tests    []string
	Excluded int
}

// Exclusion is the sources a project leaves out of what every command
// measures and mutates: those whose path from Root, slash-separated,
// matches one of Globs, doublestar patterns (itos-cc.yaml's exclude). It
// never names a test, and the zero Exclusion excludes nothing.
type Exclusion struct {
	Root  string
	Globs []string
}

// Excludes reports whether the source at path, absolute, is left out: a
// path the root cannot reach never is. The globs are taken as valid, as
// the settings that hold them checked them.
func (e Exclusion) Excludes(path string) bool {
	if len(e.Globs) == 0 {
		return false
	}
	rel, err := filepath.Rel(e.Root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	rel = filepath.ToSlash(rel)
	for _, glob := range e.Globs {
		if doublestar.MatchUnvalidated(glob, rel) {
			return true
		}
	}
	return false
}

// from is files less the sources e excludes, counted in Excluded; the
// tests stay whole.
func (e Exclusion) from(files Files) Files {
	var kept []string
	for _, f := range files.Sources {
		if e.Excludes(f) {
			files.Excluded++
			continue
		}
		kept = append(kept, f)
	}
	files.Sources = kept
	return files
}

// Select chooses the files of paths, through tree, passing ctx on to it,
// and leaves out the sources exclude names.
//
// With changed, the files git reports as changed join paths, and when there
// are none nothing is selected, whatever paths say. Without any path the
// whole working directory, ".", is selected. A path that exists is a root,
// its files selected; one that does not is a fragment, selecting every file
// under "." whose path contains it, so "billing" finds src/billing/*. A
// fragment that no file's path contains is refused as port.Unmatched, the
// first such in argument order. A failure selects nothing.
//
// Exclusion comes last and always wins: a source exclude names is left out
// however it was chosen, a path, a fragment or changed naming it, and a
// fragment that only such sources contain still matched, so it is no
// port.Unmatched.
func Select(ctx context.Context, tree port.Tree, paths []string, changed bool, exclude Exclusion) (Files, error) {
	files, err := chosen(ctx, tree, paths, changed)
	if err != nil {
		return Files{}, err
	}
	return exclude.from(files), nil
}

// chosen is the files of paths and changed, before any is excluded.
func chosen(ctx context.Context, tree port.Tree, paths []string, changed bool) (Files, error) {
	paths = append([]string(nil), paths...)
	if changed {
		found, failure := tree.Changed(ctx)
		if failure != nil {
			return Files{}, failure
		}
		if len(found) == 0 {
			return Files{}, nil
		}
		paths = append(paths, found...)
	}
	if len(paths) == 0 {
		return discover(ctx, tree, []string{"."})
	}
	var roots, fragments []string
	for _, p := range paths {
		if tree.Exists(ctx, p) {
			roots = append(roots, p)
		} else {
			fragments = append(fragments, filepath.ToSlash(p))
		}
	}
	files, err := discover(ctx, tree, roots)
	if err != nil || len(fragments) == 0 {
		return files, err
	}
	all, err := discover(ctx, tree, []string{"."})
	if err != nil {
		return Files{}, err
	}
	matched := map[string]bool{}
	files.Sources = append(files.Sources, matching(all.Sources, fragments, matched)...)
	files.Tests = append(files.Tests, matching(all.Tests, fragments, matched)...)
	for _, fragment := range fragments {
		if !matched[fragment] {
			return Files{}, &port.Unmatched{Fragment: fragment}
		}
	}
	return files, nil
}

// discover is the files under roots, as tree finds them.
func discover(ctx context.Context, tree port.Tree, roots []string) (Files, error) {
	sources, tests, failure := tree.Discover(ctx, roots)
	if failure != nil {
		return Files{}, failure
	}
	return Files{Sources: sources, Tests: tests}, nil
}

// matching is the files of list whose path contains one of fragments, in
// list's order, marking in matched each fragment that selected one. A file
// marks only the first fragment it contains.
func matching(list, fragments []string, matched map[string]bool) []string {
	var out []string
	for _, f := range list {
		for _, fragment := range fragments {
			if strings.Contains(filepath.ToSlash(f), fragment) {
				out = append(out, f)
				matched[fragment] = true
				break
			}
		}
	}
	return out
}
