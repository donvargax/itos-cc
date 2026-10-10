package graph

import (
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/project"
)

// TestsImporting finds the tests of each source file of files, as the graph
// resolves imports. In TypeScript, Python and Kotlin they are the test files
// that reach its module through the transitive closure of their imports
// (Kotlin's same-package references included), as vitest related and jest
// --findRelatedTests select them, through the test-support files they use
// too: a test reaches what the test files it imports reach, such as a helper
// under tests/ or __tests__/ or a Kotlin helper in src/test, and a Python
// test also reaches what every conftest.py in its directory and those above
// it, up to its build root, reaches, as pytest loads them without an import.
// The support files that reach a module are among its tests too. In Go,
// where a module is a package, they are the test files of its own package
// and of the packages whose sources import it, one hop: a Go test that
// reaches a package only through another is not among them. root is the
// project root, which tsconfig aliases resolve against. Paths are those of
// files, and each list is sorted.
func TestsImporting(root string, files project.Files) (map[string][]string, error) {
	sources, err := parseDependencies(root, files.Sources)
	if err != nil {
		return nil, err
	}
	tests, err := parseDependencies(root, files.Tests)
	if err != nil {
		return nil, err
	}
	return importingTests(root, sources, tests), nil
}

// importingTests is TestsImporting of sources and tests already parsed.
func importingTests(root string, sources, tests []*fileInfo) map[string][]string {
	g := newRepoGraph("project", root, sources)
	// importers are the modules whose sources import each module.
	importers := map[string]map[string]bool{}
	for e := range g.edges {
		if importers[e[1]] == nil {
			importers[e[1]] = map[string]bool{}
		}
		importers[e[1]][e[0]] = true
	}
	// all is the graph of the sources and every test but Go's, whose
	// imports resolve to test files as well as sources: imported is the
	// modules each module imports there.
	all := append([]*fileInfo{}, sources...)
	conftests := map[string]*fileInfo{} // by directory
	for _, t := range tests {
		if t.language == "go" {
			continue
		}
		all = append(all, t)
		if t.language == "python" && filepath.Base(t.abs) == "conftest.py" {
			conftests[filepath.Dir(t.abs)] = t
		}
	}
	whole := newRepoGraph("project", root, all)
	imported := map[string][]string{}
	for e := range whole.edges {
		imported[e[0]] = append(imported[e[0]], e[1])
	}
	idx := g.index()
	byModule := map[string][]string{}
	for _, t := range tests {
		reaches := map[string]bool{}
		if t.language == "go" {
			// Its own package and the packages that import it, one hop.
			for _, imp := range t.imports {
				targets, _ := idx.resolve(t, imp)
				for _, to := range targets {
					reaches[to] = true
				}
			}
			own := g.moduleID(t)
			reaches[own] = true
			for to, from := range importers {
				if from[own] {
					reaches[to] = true
				}
			}
		} else {
			// Every module the test's imports reach, transitively, through
			// sources and test files alike, from the test and the
			// conftest.py files that apply to it.
			queue := []string{whole.moduleID(t)}
			for _, c := range applyingConftests(t, conftests, root) {
				queue = append(queue, whole.moduleID(c))
			}
			seen := map[string]bool{}
			for len(queue) > 0 {
				m := queue[len(queue)-1]
				queue = queue[:len(queue)-1]
				if seen[m] {
					continue
				}
				seen[m] = true
				reaches[m] = true
				queue = append(queue, imported[m]...)
			}
		}
		for m := range reaches {
			byModule[m] = append(byModule[m], t.abs)
		}
	}
	out := map[string][]string{}
	for _, s := range sources {
		list := append([]string{}, byModule[g.moduleID(s)]...)
		sort.Strings(list)
		out[s.abs] = slices.Compact(list)
	}
	return out
}

// applyingConftests is the conftest.py files pytest loads for t, a Python
// test file it runs or a conftest.py: those in its directory and each
// directory above it, up to its build root, or root when it has none; none
// for a helper or another language.
func applyingConftests(t *fileInfo, conftests map[string]*fileInfo, root string) []*fileInfo {
	if t.language != "python" || len(conftests) == 0 || !coverage.PythonTestFile(t.abs) && filepath.Base(t.abs) != "conftest.py" {
		return nil
	}
	top := lang.FindUp(t.abs, "pyproject.toml", "setup.py", "setup.cfg")
	if top == "" {
		top = root
	}
	var out []*fileInfo
	for dir := filepath.Dir(t.abs); ; dir = filepath.Dir(dir) {
		if c := conftests[dir]; c != nil && c != t {
			out = append(out, c)
		}
		if rel, err := filepath.Rel(top, dir); err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.Dir(dir) == dir {
			return out
		}
	}
}

func parseDependencies(root string, paths []string) ([]*fileInfo, error) {
	var out []*fileInfo
	for _, path := range paths {
		f, err := lang.ParseFile(path)
		if err != nil {
			return nil, err
		}
		out = append(out, dependencies(root, f))
		f.Close()
	}
	return out, nil
}
