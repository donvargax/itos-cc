package features

import (
	"context"
	"path"
	"slices"
	"strings"
)

// sourceExtensions are the extensions of the files itos-cc reads as a
// language's sources or tests, as lang.Detect knows them.
var sourceExtensions = []string{".go", ".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs", ".py", ".kt", ".kts"}

// unwalked are the folders no selection walks into, beside any whose name
// starts with a dot, as the selection's tree skips them.
var unwalked = map[string]bool{"node_modules": true, "vendor": true, "venv": true, "__pycache__": true, "testdata": true}

// imports adds edges to the import adjacency the scenario's fixture
// declares: each file, by slash path from the scratch folder, imports
// the files it names, as its text does. A file of a supported language no
// fixture names an edge of imports nothing.
func (w *world) imports(edges map[string][]string) {
	if w.edges == nil {
		w.edges = map[string][]string{}
	}
	for from, targets := range edges {
		w.edges[from] = slices.Clone(targets)
	}
}

// declareAdjacency declares, before a run, the resolved import adjacency
// of every source and test the scratch folder holds outside the folders a
// selection never walks into: the edges the fixture named (imports), and
// none of any other. The real resolver reads the same from the files'
// text, which the binary driver runs; the in-process driver's Reach is
// handed it (Tests.Imports). An edge of a file no longer there is dropped,
// as no import resolves to it.
func (w *world) declareAdjacency(ctx context.Context) error {
	if w.dir == "" || !w.d.Exists(w.dir) {
		return nil
	}
	files, err := w.d.Files(ctx, w.dir)
	if err != nil {
		return err
	}
	adjacency := map[string][]string{}
	for _, name := range files {
		if slices.Contains(sourceExtensions, path.Ext(name)) && walked(name) {
			adjacency[name] = []string{}
		}
	}
	for from, targets := range w.edges {
		if _, ok := adjacency[from]; !ok {
			continue
		}
		var kept []string
		for _, target := range targets {
			if _, ok := adjacency[target]; ok {
				kept = append(kept, target)
			}
		}
		adjacency[from] = append([]string{}, kept...)
	}
	if len(adjacency) == 0 {
		return nil
	}
	return w.d.Tests(w.dir, Tests{Imports: adjacency})
}

// walked is whether a selection walks to name, a slash path: no folder
// above it is hidden or one it never walks into.
func walked(name string) bool {
	folders := strings.Split(name, "/")
	for _, folder := range folders[:len(folders)-1] {
		if strings.HasPrefix(folder, ".") || unwalked[folder] {
			return false
		}
	}
	return true
}
