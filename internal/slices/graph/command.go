package graph

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/internal/cli"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
	"github.com/donvargax/itos-cc/internal/snapshot"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// Command is the graph command as cmd assembles it (ADR-0051): its name,
// the line the top-level help lists it by, and a fresh command line for
// each parser.
var Command = cli.Declaration{
	Name: "graph",
	Help: "write the architecture graph as .metrics/graph.json, for a viewer",
	New:  func() any { return &CLI{} },
}

// CLI is the graph command's command line: whether it prints the line a
// person reads or the one object a script reads. It selects nothing, since
// the graph is the whole project's (q-95, D3).
type CLI struct {
	JSON bool `name:"json" help:"print one JSON object on stdout instead of text (shape below)"`
}

// Contract is what graph's help says beyond its flags: what it writes, the
// --json object a script reads, and every exit code it can return.
func (CLI) Contract() cli.Contract {
	return cli.Contract{
		About: `
Writes the architecture of the project as .metrics/graph.json, which a
viewer reads without asking itos-cc: its repository, directories, modules
and functions, the modules' import and HTTP edges, each function's
complexity and the coverage, CRAP, duplicates and mutation results the
other snapshots hold, its mutation results stale exactly when mutation
check calls them stale. The repository is named by its git remote origin's
repository name, or by its folder's without one. Every file of the project
is read, whatever itos-cc.yaml's exclude says. The file is sorted and holds
no time, so an unchanged project writes it byte for byte again.`,
		JSON: `"repositories", "modules", "functions", "edges"`,
		Exits: []cli.Exit{
			{Code: 0, Meaning: "success, whatever the graph holds"},
			{Code: cli.CodeUsage, Meaning: "a usage error: a bad flag or an argument"},
		},
		Examples: []string{
			"itos-cc graph",
			"itos-cc graph --json",
		},
	}
}

// Handler is the graph command's application: the tree it finds and reads
// the project's files in, what surrounds them, the store of the project's
// snapshots, whose root is the project's and which the graph is written
// to, mutation's judgment, and Origin, the URL of the git remote origin of
// the repository at a root, "" for none. cmd gives each through Ports
// (NewHandler).
type Handler struct {
	Tree         selport.Tree
	Surroundings langport.Surroundings
	Store        snapport.Store
	Mutation     Mutation
	Origin       func(ctx context.Context, root string) string
}

// Ports are what the graph command reaches the world through, which cmd's
// adapters give it: the tree, the surroundings and the snapshot store every
// command reads through, the Mutation port bound to mutation's judgment, or
// why it cannot be, and the URL of a repository's git remote origin.
type Ports interface {
	SelectionTree() selport.Tree
	LangSurroundings() langport.Surroundings
	SnapshotStore() snapport.Store
	GraphMutation() (Mutation, error)
	RepositoryOrigin(ctx context.Context, root string) string
}

// NewHandler is the handler over p's ports; the Mutation port's failure is
// returned as p gave it.
func NewHandler(_ context.Context, p Ports) (Handler, error) {
	mutation, err := p.GraphMutation()
	if err != nil {
		return Handler{}, err
	}
	return Handler{Tree: p.SelectionTree(), Surroundings: p.LangSurroundings(), Store: p.SnapshotStore(), Mutation: mutation, Origin: p.RepositoryOrigin}, nil
}

// Written is what Handle wrote: the counts of the graph's repositories,
// modules, functions and edges.
type Written struct {
	Repositories int `json:"repositories"`
	Modules      int `json:"modules"`
	Functions    int `json:"functions"`
	Edges        int `json:"edges"`
}

// Handle builds the graph of the project whose root is the store's, under
// ctx, its repository named by its git remote origin (RepositoryName), and
// writes it to the store as graph.json, its version the snapshot format's.
// A failure to read the project stops it with nothing written; a failure
// to write returns the counts beside it.
func (h Handler) Handle(ctx context.Context) (Written, error) {
	root, failure := h.Store.Root(ctx)
	if failure != nil {
		return Written{}, failure
	}
	name := RepositoryName(h.Origin(ctx, root), filepath.Base(root))
	b, err := NewBuilder([]string{root}, Reading{Tree: h.Tree, Surroundings: h.Surroundings, Store: func(string) snapport.Store { return h.Store },
		Mutation: h.Mutation, Top: func(string) string { return root }, Name: func(string) string { return name }})
	if err != nil {
		return Written{}, err
	}
	g, _, err := b.Build(ctx)
	if err != nil {
		return Written{}, err
	}
	g.Version = snapshot.Version
	written := Written{Edges: len(g.Edges)}
	for _, n := range g.Nodes {
		switch n.Kind {
		case "repo":
			written.Repositories++
		case "module":
			written.Modules++
		}
		written.Functions += len(n.Units)
	}
	if failure := h.Store.Write(ctx, "graph.json", g); failure != nil {
		return written, failure
	}
	return written, nil
}

// RepositoryName is the name of the repository whose git remote origin is
// url, as a forge names it: the last segment of its path, without .git;
// folder, its folder's name, when it has no remote or url names nothing.
func RepositoryName(url, folder string) string {
	name := strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(url), "/"), ".git")
	if i := strings.LastIndexAny(name, "/:\\"); i >= 0 {
		name = name[i+1:]
	}
	if name == "" {
		return folder
	}
	return name
}

// Run writes the graph through h with the context cmd made, and prints the
// line a person reads, or the object, or reports the failure it met, with
// the counts written before it in the object, and returns the exit code
// the UI gives it.
func (c CLI) Run(ctx context.Context, ui *cli.UI, h Handler) int {
	written, err := h.Handle(ctx)
	if err != nil {
		return ui.Emit(c.JSON, written, ui.Problems(err))
	}
	if !c.JSON {
		_, _ = fmt.Fprintf(ui.Stdout, "%d modules, %d functions and %d edges in .metrics/graph.json\n", written.Modules, written.Functions, written.Edges)
	}
	return ui.Emit(c.JSON, written, nil)
}
