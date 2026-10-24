package imports

import (
	"context"
	"slices"
	"sort"

	"github.com/donvargax/itos-cc/internal/project"
)

// TestsImporting finds the tests of each source file of sources, as the graph
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
// sources and tests, and each list is sorted. It reads under ctx, and once
// ctx is done returns its cause.
func TestsImporting(ctx context.Context, root string, sources, tests []string) (map[string][]string, error) {
	parsedSources, err := parseDependencies(ctx, root, sources)
	if err != nil {
		return nil, stopped(ctx, err)
	}
	parsedTests, err := parseDependencies(ctx, root, tests)
	if err != nil {
		return nil, stopped(ctx, err)
	}
	importing := Reach(ctx, root, parsedSources, parsedTests)
	if err := stopped(ctx, nil); err != nil {
		return nil, err
	}
	return importing, nil
}

// Reach is TestsImporting of sources and tests already parsed, by each
// source's Abs.
func Reach(ctx context.Context, root string, sources, tests []File) map[string][]string {
	g := Resolve(ctx, root, sources)
	// importers are the modules whose sources import each module.
	importers := map[string]map[string]bool{}
	for e := range g.Edges {
		if importers[e[1]] == nil {
			importers[e[1]] = map[string]bool{}
		}
		importers[e[1]][e[0]] = true
	}
	// all is the graph of the sources and every test but those reaching one
	// hop, Go's, whose imports resolve to test files as well as sources:
	// imported is the modules each module imports there.
	all := append([]File{}, sources...)
	transitive := map[string][]*File{} // by language
	for i := range tests {
		t := &tests[i]
		if languageOf(t.Language).oneHop() {
			continue
		}
		all = append(all, *t)
		transitive[t.Language] = append(transitive[t.Language], t)
	}
	supporters := map[string]supporter{} // by language
	for name, ts := range transitive {
		if s := languageOf(name).support(ts); s != nil {
			supporters[name] = s
		}
	}
	whole := Resolve(ctx, root, all)
	imported := map[string][]string{}
	for e := range whole.Edges {
		imported[e[0]] = append(imported[e[0]], e[1])
	}
	byModule := map[string][]string{}
	for i := range tests {
		t := &tests[i]
		reaches := map[string]bool{}
		if languageOf(t.Language).oneHop() {
			// Its own module and the modules that import it, one hop.
			res := g.resolver(t.Language)
			for _, to := range res.unimported(t) {
				reaches[to] = true
			}
			for _, imp := range t.imports {
				targets, _ := res.resolve(ctx, t, imp)
				for _, to := range targets {
					reaches[to] = true
				}
			}
			own := moduleID(*t)
			reaches[own] = true
			for to, from := range importers {
				if from[own] {
					reaches[to] = true
				}
			}
		} else {
			// Every module the test's imports reach, transitively, through
			// sources and test files alike, from the test and the support
			// files that apply to it, such as pytest's conftest.py.
			queue := []string{moduleID(*t)}
			if s := supporters[t.Language]; s != nil {
				for _, c := range s(ctx, root, t) {
					queue = append(queue, moduleID(*c))
				}
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
			byModule[m] = append(byModule[m], t.Abs)
		}
	}
	out := map[string][]string{}
	for _, s := range sources {
		list := append([]string{}, byModule[moduleID(s)]...)
		sort.Strings(list)
		out[s.Abs] = slices.Compact(list)
	}
	return out
}

// parseDependencies parses each of paths under ctx for what the graph needs
// to resolve its imports.
func parseDependencies(ctx context.Context, root string, paths []string) ([]File, error) {
	var out []File
	for _, path := range paths {
		f, err := project.ParseFile(ctx, path)
		if err != nil {
			return nil, err
		}
		out = append(out, Of(root, f))
		f.Close()
	}
	return out, nil
}

// stopped is the cause of ctx once it is done, or else err, so a call cut
// short by a stopped run reports the stop, never what it left undone.
func stopped(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	return err
}
