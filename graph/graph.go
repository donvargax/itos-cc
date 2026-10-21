// Package graph builds the architecture of one or more repositories as a
// tree with dependency edges: repositories contain directories, directories
// contain modules, modules contain functions. A module is a file in
// TypeScript, Python, and Kotlin, and a package in Go. Every level of the tree
// is the same kind of thing to a viewer, so the same drill-down works from a
// system of repositories down to one function.
package graph

import (
	"context"
	"path"
	"sort"
	"strings"

	"github.com/donvargax/itos-cc/internal/imports"
	"github.com/donvargax/itos-cc/internal/lang"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
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
	Serves   []string `json:"serves,omitempty"`   // HTTP routes the module handles
	Calls    []string `json:"calls,omitempty"`    // HTTP requests the module makes
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
	Stale      bool     `json:"stale"` // its mutation results are stale, as mutation check calls them
	Duplicates int      `json:"duplicates"`

	hash  string
	sites []mutants.Site // its mutation sites, which a fresh entry records
}

// Edge is a dependency between modules. An import edge counts From's import
// statements of To; an http edge counts From's requests that match a route
// To serves, and Via lists those routes.
type Edge struct {
	From  string   `json:"from"`
	To    string   `json:"to"`
	Count int      `json:"count"`
	Kind  string   `json:"kind"` // import or http
	Via   []string `json:"via,omitempty"`
}

// Graph is every repository's tree and the module dependencies.
type Graph struct {
	Version int     `json:"version"`
	Nodes   []*Node `json:"nodes"`
	Edges   []Edge  `json:"edges"`
}

// fileInfo is what the graph needs from one parsed source file: what import
// resolution needs of it, and its units and endpoints.
type fileInfo struct {
	imports.File
	units  []Unit
	serves []lang.Endpoint
	calls  []lang.Endpoint
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
	res     imports.Resolution
}

// newRepoGraph is the graph of files, the repository name's under root, its
// imports resolved under ctx.
func newRepoGraph(ctx context.Context, name, root string, files []*fileInfo) *repoGraph {
	g := &repoGraph{name: name, root: root, files: files,
		nodes: map[string]*Node{}, modules: map[string]*Node{}, edges: map[[2]string]int{}}
	g.res = imports.Resolve(ctx, root, importFiles(files))
	g.nodes[name] = &Node{ID: name, Kind: "repo", Label: name}
	for _, f := range files {
		m := g.module(f)
		m.Files = appendUnique(m.Files, f.Rel)
		m.Units = append(m.Units, f.units...)
		for _, e := range f.serves {
			m.Serves = appendUnique(m.Serves, e.String())
		}
		for _, e := range f.calls {
			m.Calls = appendUnique(m.Calls, e.String())
		}
	}
	for e, count := range g.res.Edges {
		g.edges[[2]string{name + "/" + e[0], name + "/" + e[1]}] += count
	}
	for module, external := range g.res.External {
		m := g.modules[name+"/"+module]
		for _, p := range external {
			m.External = appendUnique(m.External, p)
		}
	}
	return g
}

// importFiles is what import resolution needs of each of files.
func importFiles(files []*fileInfo) []imports.File {
	out := make([]imports.File, 0, len(files))
	for _, f := range files {
		out = append(out, f.File)
	}
	return out
}

// moduleID is the module a file belongs to, prefixed with the repository's
// name, as import resolution names it (imports.Resolution.Module).
func (g *repoGraph) moduleID(f *fileInfo) string {
	return g.name + "/" + g.res.Module(f.File)
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
	m.Kind, m.Label, m.Language = "module", path.Base(id), f.Language
	if strings.HasSuffix(id, "/(root)") {
		m.Label = f.Namespace
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
		sort.Strings(n.Serves)
		sort.Strings(n.Calls)
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
		edges = append(edges, Edge{From: k[0], To: k[1], Count: c, Kind: "import"})
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})
	out.Edges = append(out.Edges, edges...)
}
