// Package graph builds the architecture of one or more repositories as a
// tree with dependency edges: repositories contain directories, directories
// contain modules, modules contain functions. A module is a file in
// TypeScript, Python, and Kotlin, and a package in Go. Every level of the tree
// is the same kind of thing to a viewer, so the same drill-down works from a
// system of repositories down to one function.
package graph

import (
	"path"
	"path/filepath"
	"sort"
	"strings"

	"itos-cc/lang"
)

// Node is one box: a repository, a directory, or a module.
type Node struct {
	ID       string   `json:"id"`
	Parent   string   `json:"parent,omitempty"`
	Kind     string   `json:"kind"` // repo, dir, or module
	Label    string   `json:"label"`
	Language string   `json:"language,omitempty"`
	Files    []string `json:"files,omitempty"` // repository-relative
	Units    []Unit   `json:"units,omitempty"`
	External []string `json:"external,omitempty"` // packages imported from outside the repository
	Metrics  *Metrics `json:"metrics,omitempty"`
	children []*Node
}

// Unit is one function or method of a module with what is known about it.
type Unit struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	File       string   `json:"file"`
	Line       int      `json:"line"`
	EndLine    int      `json:"end_line"`
	Complexity int      `json:"complexity"`
	Coverage   *float64 `json:"coverage"` // percent, from the last coverage run
	CRAP       *float64 `json:"crap"`     // live complexity with the last coverage
	Killed     int      `json:"killed"`
	Survived   int      `json:"survived"`
	Uncovered  int      `json:"uncovered"`
	Mutated    bool     `json:"mutated"`
	Stale      bool     `json:"stale"` // the function changed since it was mutated
	Duplicates int      `json:"duplicates"`

	hash string
}

// Edge is a dependency between modules: From imports To, Count times.
type Edge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int    `json:"count"`
}

// Graph is every repository's tree and the module dependencies.
type Graph struct {
	Version int     `json:"version"`
	Nodes   []*Node `json:"nodes"`
	Edges   []Edge  `json:"edges"`
}

// fileInfo is what the graph needs from one parsed source file.
type fileInfo struct {
	rel       string // repository-relative, slash-separated
	abs       string
	language  string
	namespace string
	imports   []lang.Import
	topLevel  []string // Kotlin declarations other files import
	units     []Unit
}

// repoGraph builds one repository's nodes and edges. Node ids are prefixed
// with the repository's name so several repositories share one graph.
type repoGraph struct {
	name    string
	root    string
	files   []*fileInfo
	nodes   map[string]*Node
	modules map[string]*Node // by module id
	edges   map[[2]string]int
}

func newRepoGraph(name, root string, files []*fileInfo) *repoGraph {
	g := &repoGraph{name: name, root: root, files: files,
		nodes: map[string]*Node{}, modules: map[string]*Node{}, edges: map[[2]string]int{}}
	g.nodes[name] = &Node{ID: name, Kind: "repo", Label: name}
	for _, f := range files {
		m := g.module(f)
		m.Files = appendUnique(m.Files, f.rel)
		m.Units = append(m.Units, f.units...)
	}
	g.resolveAll()
	return g
}

// moduleID is the module a file belongs to: its package directory in Go and
// for a Python __init__.py, the file without its extension elsewhere.
func (g *repoGraph) moduleID(f *fileInfo) string {
	if f.language == "go" || path.Base(f.rel) == "__init__.py" {
		dir := path.Dir(f.rel)
		if dir == "." {
			return g.name + "/(root)"
		}
		return g.name + "/" + dir
	}
	return g.name + "/" + strings.TrimSuffix(f.rel, path.Ext(f.rel))
}

// module returns the file's module node. A package directory that also
// holds subpackages is one node: a module that is also a container.
func (g *repoGraph) module(f *fileInfo) *Node {
	id := g.moduleID(f)
	if m, ok := g.modules[id]; ok {
		return m
	}
	m, exists := g.nodes[id]
	if !exists {
		m = &Node{ID: id}
	}
	m.Kind, m.Label, m.Language = "module", path.Base(id), f.language
	if strings.HasSuffix(id, "/(root)") {
		m.Label = f.namespace
	}
	g.modules[id] = m
	if !exists {
		g.attach(m)
	}
	return m
}

// attach links n under its parent directory, creating directories as needed.
func (g *repoGraph) attach(n *Node) {
	g.nodes[n.ID] = n
	parentID := path.Dir(n.ID)
	if strings.HasSuffix(n.ID, "/(root)") || parentID == "." {
		parentID = g.name
	}
	n.Parent = parentID
	parent, ok := g.nodes[parentID]
	if !ok {
		// A directory, unless a package file turns it into a module later.
		parent = &Node{ID: parentID, Kind: "dir", Label: path.Base(parentID)}
		g.attach(parent)
	}
	parent.children = append(parent.children, n)
}

func (g *repoGraph) resolveAll() {
	idx := g.index()
	for _, f := range g.files {
		from := g.moduleID(f)
		for _, imp := range f.imports {
			targets, external := idx.resolve(f, imp)
			for _, to := range targets {
				if to != from {
					g.edges[[2]string{from, to}]++
				}
			}
			if external != "" {
				m := g.modules[from]
				m.External = appendUnique(m.External, external)
			}
		}
	}
}

// index maps the names imports use to module ids.
type index struct {
	g          *repoGraph
	byFile     map[string]string   // repository-relative file without extension → module
	byDotted   map[string]string   // Python module and package names → module
	byImport   map[string]string   // Go import path → module
	byPackage  map[string][]string // Kotlin package → modules
	byKotlinFQ map[string]string   // Kotlin package.Name → module
}

func (g *repoGraph) index() *index {
	idx := &index{g: g, byFile: map[string]string{}, byDotted: map[string]string{}, byImport: map[string]string{},
		byPackage: map[string][]string{}, byKotlinFQ: map[string]string{}}
	for _, f := range g.files {
		id := g.moduleID(f)
		switch f.language {
		case "typescript":
			idx.byFile[strings.TrimSuffix(f.rel, path.Ext(f.rel))] = id
		case "python":
			idx.byDotted[f.namespace] = id
		case "go":
			idx.byImport[f.namespace] = id
		case "kotlin":
			idx.byPackage[f.namespace] = appendUnique(idx.byPackage[f.namespace], id)
			for _, name := range f.topLevel {
				idx.byKotlinFQ[f.namespace+"."+name] = id
			}
		}
	}
	return idx
}

// resolve returns the project modules an import names, or the external
// package it names.
func (idx *index) resolve(f *fileInfo, imp lang.Import) (targets []string, external string) {
	switch f.language {
	case "typescript":
		return idx.typescript(f, imp.Path)
	case "python":
		return idx.python(f, imp)
	case "go":
		if id, ok := idx.byImport[imp.Path]; ok {
			return []string{id}, ""
		}
		return nil, imp.Path
	case "kotlin":
		return idx.kotlin(imp)
	}
	return nil, ""
}

var tsExtensions = []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts"}

func (idx *index) typescript(f *fileInfo, spec string) ([]string, string) {
	if !strings.HasPrefix(spec, ".") {
		parts := strings.SplitN(spec, "/", 3)
		if strings.HasPrefix(spec, "@") && len(parts) > 1 {
			return nil, parts[0] + "/" + parts[1]
		}
		return nil, parts[0]
	}
	target := path.Join(path.Dir(f.rel), spec)
	// ESM TypeScript imports name the compiled file: ./a.js means a.ts.
	for _, ext := range tsExtensions {
		target = strings.TrimSuffix(target, ext)
	}
	for _, candidate := range []string{target, target + "/index"} {
		if id, ok := idx.byFile[candidate]; ok {
			return []string{id}, ""
		}
	}
	return nil, ""
}

func (idx *index) python(f *fileInfo, imp lang.Import) ([]string, string) {
	module := imp.Path
	if strings.HasPrefix(module, ".") {
		dots := len(module) - len(strings.TrimLeft(module, "."))
		pkg := strings.Split(f.namespace, ".")
		if !strings.HasSuffix(f.rel, "__init__.py") {
			pkg = pkg[:len(pkg)-1]
		}
		up := dots - 1
		if up > len(pkg) {
			return nil, ""
		}
		parts := append([]string{}, pkg[:len(pkg)-up]...)
		if rest := strings.TrimLeft(module, "."); rest != "" {
			parts = append(parts, rest)
		}
		module = strings.Join(parts, ".")
	}
	var targets []string
	// from pkg import mod, Name: a name that is a submodule depends on it;
	// otherwise the dependency is on pkg itself.
	onPackage := len(imp.Names) == 0
	for _, name := range imp.Names {
		if id, ok := idx.byDotted[join(module, name)]; ok {
			targets = append(targets, id)
		} else {
			onPackage = true
		}
	}
	if onPackage {
		for m := module; m != ""; m = parentModule(m) {
			if id, ok := idx.byDotted[m]; ok {
				targets = append(targets, id)
				break
			}
		}
	}
	if len(targets) == 0 && !strings.HasPrefix(imp.Path, ".") {
		return nil, strings.Split(module, ".")[0]
	}
	return targets, ""
}

func (idx *index) kotlin(imp lang.Import) ([]string, string) {
	if imp.Wildcard {
		if ids, ok := idx.byPackage[imp.Path]; ok {
			return ids, ""
		}
	}
	// a.b.Outer.Inner imports from the file that declares a.b.Outer.
	for name := imp.Path; name != ""; name = parentModule(name) {
		if id, ok := idx.byKotlinFQ[name]; ok {
			return []string{id}, ""
		}
	}
	parts := strings.Split(imp.Path, ".")
	if len(parts) > 2 {
		return nil, strings.Join(parts[:2], ".")
	}
	return nil, imp.Path
}

func join(module, name string) string {
	if module == "" {
		return name
	}
	return module + "." + name
}

func parentModule(m string) string {
	if i := strings.LastIndex(m, "."); i >= 0 {
		return m[:i]
	}
	return ""
}

func appendUnique(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}

// output appends the repository's nodes and edges in a stable order.
func (g *repoGraph) output(out *Graph) {
	var ids []string
	for id := range g.nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		n := g.nodes[id]
		sort.Strings(n.External)
		sort.Slice(n.Units, func(i, j int) bool {
			if n.Units[i].File != n.Units[j].File {
				return n.Units[i].File < n.Units[j].File
			}
			return n.Units[i].Line < n.Units[j].Line
		})
		out.Nodes = append(out.Nodes, n)
	}
	var edges []Edge
	for k, c := range g.edges {
		edges = append(edges, Edge{From: k[0], To: k[1], Count: c})
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})
	out.Edges = append(out.Edges, edges...)
}

// rel is path relative to root with forward slashes.
func rel(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(r)
}
