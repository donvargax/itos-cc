package graph

import (
	"sort"

	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/project"
)

// TestsImporting finds the tests of each source file of files, as the graph
// resolves imports. In TypeScript, Python and Kotlin they are the test files
// that reach its module through the transitive closure of their imports
// (Kotlin's same-package references included), as vitest related and jest
// --findRelatedTests select them. In Go, where a module is a package, they
// are the test files of its own package and of the packages whose sources
// import it, one hop: a Go test that reaches a package only through another
// is not among them. root is the project root, which tsconfig aliases
// resolve against. Paths are those of files, and each list is sorted.
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
	idx := g.index()
	// importers are the modules whose sources import each module, and
	// imported the modules each module's sources import.
	importers := map[string]map[string]bool{}
	imported := map[string][]string{}
	for e := range g.edges {
		if importers[e[1]] == nil {
			importers[e[1]] = map[string]bool{}
		}
		importers[e[1]][e[0]] = true
		imported[e[0]] = append(imported[e[0]], e[1])
	}
	// reaches is each test's modules: for Go its own package and the
	// packages that one imports, elsewhere those it imports and every module
	// they import in turn.
	byModule := map[string][]string{}
	for _, t := range tests {
		reaches := map[string]bool{}
		for _, to := range idx.samePackage(t) {
			reaches[to] = true
		}
		for _, imp := range t.imports {
			targets, _ := idx.resolve(t, imp)
			for _, to := range targets {
				reaches[to] = true
			}
		}
		if t.language == "go" {
			own := g.moduleID(t)
			reaches[own] = true
			for to, from := range importers {
				if from[own] {
					reaches[to] = true
				}
			}
		} else {
			var queue []string
			for m := range reaches {
				queue = append(queue, m)
			}
			for len(queue) > 0 {
				m := queue[len(queue)-1]
				queue = queue[:len(queue)-1]
				for _, to := range imported[m] {
					if !reaches[to] {
						reaches[to] = true
						queue = append(queue, to)
					}
				}
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
		out[s.abs] = list
	}
	return out
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
