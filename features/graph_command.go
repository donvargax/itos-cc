// The steps of graph_command.feature: the architecture graph itos-cc graph
// writes as .metrics/graph.json, judged from the file it leaves, what it
// prints and, where a scenario compares them, what mutation check prints.
// Like every step here, they read the binary's output alone, never its
// code.
package features

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

// graphFeature declares graph_command.feature's steps, graphSteps, which
// tools/bin/assemble lists in steps_gen.go.
var graphFeature = featureFile{Name: "graph_command.feature", Steps: graphSteps}

// The Go module graph_command.feature's scenarios run in: package app,
// whose Total calls app/store's Count and whose Empty calls nothing, and
// app's test, which kills every mutant of both.
const (
	graphModule = "module example.com/shop\n\ngo 1.22\n"
	graphApp    = "package app\n\nimport \"example.com/shop/app/store\"\n\n" +
		"func Total(n int) bool {\n\treturn store.Count(n) > 1\n}\n\n" +
		"func Empty(n int) bool {\n\treturn n == 0\n}\n"
	graphAppTest = "package app\n\nimport \"testing\"\n\n" +
		"func TestTotal(t *testing.T) {\n\tif !Total(2) || Total(1) {\n\t\tt.Fatal(\"Total\")\n\t}\n}\n\n" +
		"func TestEmpty(t *testing.T) {\n\tif !Empty(0) || Empty(1) {\n\t\tt.Fatal(\"Empty\")\n\t}\n}\n"
	graphStore = "package store\n\nfunc Count(n int) int {\n\treturn n\n}\n"
)

// graphState is one graph_command.feature scenario's state beside the
// world's.
type graphState struct {
	w *world
	// root is the folder the module is in, which itos-cc runs in: the
	// scratch folder, or a folder of the name a Given chose under it.
	root string
	// written is .metrics/graph.json as a run wrote it before the When.
	written string
}

func graphSteps(sc *godog.ScenarioContext, w *world) {
	g := &graphState{w: w}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*g = graphState{w: w}
		return ctx, nil
	})

	sc.Step(`^a Go module with the packages app and app/store, app importing app/store$`, func() error {
		g.root = w.dir
		return g.writeModule()
	})
	sc.Step(`^its git remote origin is "([^"]*)" while its folder is named "([^"]*)"$`, g.remoteAndFolder)
	sc.Step(`^\.metrics/mutate holds a fresh snapshot for one function of app and a stale one for another$`, g.freshAndStale)
	sc.Step(`^"itos-cc graph" has written \.metrics/graph\.json$`, g.hasWritten)
	sc.Step(`^I run "itos-cc graph" again$`, func() error { return w.runWith(w.env(), "graph") })
	sc.Step(`^\.metrics/graph\.json holds "version": 1, nodes and edges$`, g.holdsVersionNodesAndEdges)
	sc.Step(`^in \.metrics/graph\.json the repository is a repo node, app and app/store are module nodes, and app imports app/store$`, g.repoModulesAndImport)
	sc.Step(`^in \.metrics/graph\.json the repo node is named "([^"]*)"$`, g.repoNamed)
	sc.Step(`^in \.metrics/graph\.json app/store is a module node$`, func() error {
		graph, repo, err := g.graph()
		if err != nil {
			return err
		}
		return graph.isModule(repo + "/app/store")
	})
	sc.Step(`^each function in \.metrics/graph\.json is marked stale exactly when "itos-cc mutation check" calls it stale$`, g.staleAsCheck)
	sc.Step(`^\.metrics/graph\.json is byte for byte as it was$`, func() error {
		now, err := g.read()
		if err != nil {
			return err
		}
		if now != g.written {
			return fmt.Errorf(".metrics/graph.json changed:\n%s\nwas:\n%s\n%s", now, g.written, w.report())
		}
		return nil
	})
}

// writeModule writes the module in g.root.
func (g *graphState) writeModule() error {
	for name, text := range map[string]string{"go.mod": graphModule, "app/app.go": graphApp, "app/app_test.go": graphAppTest, "app/store/store.go": graphStore} {
		if err := g.w.writeIn(g.root, name, text); err != nil {
			return err
		}
	}
	return nil
}

// remoteAndFolder writes the module again in a folder named folder under
// the scratch folder, makes it a git repository whose remote origin is url,
// and has itos-cc run there.
func (g *graphState) remoteAndFolder(url, folder string) error {
	g.root = filepath.Join(g.w.dir, folder)
	if err := g.writeModule(); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-q"}, {"config", "maintenance.auto", "false"}, {"config", "gc.auto", "0"}, {"config", "remote.origin.url", url}} {
		if err := g.w.d.Git(g.root, args...); err != nil {
			return err
		}
	}
	g.w.runIn = g.root
	return nil
}

// freshAndStale has mutation run record every function of app/app.go,
// with the tests declared to kill every mutant, then changes Empty, so its
// results are stale while Total's stay fresh.
func (g *graphState) freshAndStale() error {
	report := "SF:app/app.go\n"
	for i, line := range strings.Split(graphApp, "\n") {
		if strings.Contains(line, "return ") {
			report += fmt.Sprintf("DA:%d,1\n", i+1)
		}
	}
	report += "end_of_record\n"
	if err := g.w.d.Coverage(g.root, Coverage{Language: "go", Report: report}); err != nil {
		return err
	}
	if err := g.w.d.Tests(g.root, Tests{Language: "go", Command: "go test -count=1 -failfast ./app"}); err != nil {
		return err
	}
	if err := g.w.runEnv(g.root, g.w.env(), "mutation", "run", "app/app.go", "--no-annotate", "--json"); err != nil {
		return err
	}
	if _, err := g.w.readIn(g.root, ".metrics/mutate/app/app.go.json"); err != nil {
		return fmt.Errorf("mutation run recorded no snapshot of app/app.go: %v\n%s", err, g.w.report())
	}
	return g.w.writeIn(g.root, "app/app.go", strings.Replace(graphApp, "return n == 0", "return 0 == n", 1))
}

// hasWritten runs itos-cc graph, which must exit 0, and keeps the file it
// writes.
func (g *graphState) hasWritten() error {
	if err := g.w.runWith(g.w.env(), "graph"); err != nil {
		return err
	}
	if err := g.w.exitsWith(0); err != nil {
		return err
	}
	written, err := g.read()
	g.written = written
	return err
}

// read is .metrics/graph.json as it is now.
func (g *graphState) read() (string, error) {
	data, err := g.w.readIn(g.root, ".metrics/graph.json")
	if err != nil {
		return "", fmt.Errorf("no .metrics/graph.json: %v\n%s", err, g.w.report())
	}
	return string(data), nil
}

// writtenGraph is .metrics/graph.json as a viewer reads it.
type writtenGraph struct {
	Version *int          `json:"version"`
	Nodes   []writtenNode `json:"nodes"`
	Edges   []struct {
		From string `json:"from"`
		To   string `json:"to"`
		Kind string `json:"kind"`
	} `json:"edges"`
}

// writtenNode is a node of the written graph.
type writtenNode struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Units []struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		File      string `json:"file"`
		Stale     bool   `json:"stale"`
	} `json:"units"`
}

// graph is the written graph and its repository node's id.
func (g *graphState) graph() (writtenGraph, string, error) {
	text, err := g.read()
	if err != nil {
		return writtenGraph{}, "", err
	}
	var graph writtenGraph
	if err := json.Unmarshal([]byte(text), &graph); err != nil {
		return writtenGraph{}, "", fmt.Errorf(".metrics/graph.json is no graph: %v\n%s", err, text)
	}
	var repos []string
	for _, n := range graph.Nodes {
		if n.Kind == "repo" {
			repos = append(repos, n.ID)
		}
	}
	if len(repos) != 1 {
		return graph, "", fmt.Errorf(".metrics/graph.json holds the repository nodes %v, not one\n%s", repos, text)
	}
	return graph, repos[0], nil
}

// node is the node id of graph, nil for none.
func (graph writtenGraph) node(id string) *writtenNode {
	for i := range graph.Nodes {
		if graph.Nodes[i].ID == id {
			return &graph.Nodes[i]
		}
	}
	return nil
}

// isModule is whether id is a module node of graph.
func (graph writtenGraph) isModule(id string) error {
	if n := graph.node(id); n == nil || n.Kind != "module" {
		return fmt.Errorf("%s is no module node of .metrics/graph.json: %+v", id, n)
	}
	return nil
}

func (g *graphState) holdsVersionNodesAndEdges() error {
	text, err := g.read()
	if err != nil {
		return err
	}
	var graph struct {
		Version *int             `json:"version"`
		Nodes   []map[string]any `json:"nodes"`
		Edges   []map[string]any `json:"edges"`
	}
	if err := json.Unmarshal([]byte(text), &graph); err != nil {
		return fmt.Errorf(".metrics/graph.json is no graph: %v\n%s", err, text)
	}
	if graph.Version == nil || *graph.Version != 1 || len(graph.Nodes) == 0 || graph.Edges == nil {
		return fmt.Errorf(".metrics/graph.json holds no \"version\": 1, nodes and edges:\n%s", text)
	}
	return nil
}

func (g *graphState) repoModulesAndImport() error {
	graph, repo, err := g.graph()
	if err != nil {
		return err
	}
	for _, module := range []string{repo + "/app", repo + "/app/store"} {
		if err := graph.isModule(module); err != nil {
			return err
		}
	}
	for _, e := range graph.Edges {
		if e.From == repo+"/app" && e.To == repo+"/app/store" && e.Kind == "import" {
			return nil
		}
	}
	return fmt.Errorf("no import edge from %s/app to %s/app/store: %+v", repo, repo, graph.Edges)
}

func (g *graphState) repoNamed(name string) error {
	graph, repo, err := g.graph()
	if err != nil {
		return err
	}
	if n := graph.node(repo); repo != name || n.Label != name {
		return fmt.Errorf("the repo node is %s, labelled %s, not %s", repo, n.Label, name)
	}
	return nil
}

// staleAsCheck compares the graph's functions with what mutation check
// calls each, over every source of the module: a function check calls
// stale is stale in the graph and every other is not, and the scenario's
// Given left at least one of each.
func (g *graphState) staleAsCheck() error {
	graph, _, err := g.graph()
	if err != nil {
		return err
	}
	inGraph := map[string]bool{}
	for _, n := range graph.Nodes {
		for _, u := range n.Units {
			inGraph[u.Namespace+"#"+u.Name] = u.Stale
		}
	}
	code, stdout, _, err := g.w.capture(g.root, g.w.env(), "mutation", "check", "app/app.go", "app/store/store.go", "--json")
	if err != nil {
		return err
	}
	var check struct {
		Files []struct {
			Functions []struct {
				Function string `json:"function"`
				State    string `json:"state"`
			} `json:"functions"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(stdout), &check); err != nil {
		return fmt.Errorf("mutation check printed no object (exit %d): %v\n%s", code, err, stdout)
	}
	states := map[string]string{}
	for _, f := range check.Files {
		for _, fn := range f.Functions {
			states[fn.Function] = fn.State
		}
	}
	seen := map[string]bool{}
	for function, state := range states {
		seen[state] = true
		stale, ok := inGraph[function]
		if !ok {
			return fmt.Errorf("mutation check judges %s, which .metrics/graph.json does not hold: %v", function, inGraph)
		}
		if stale != (state == "stale") {
			return fmt.Errorf("%s: stale=%v in .metrics/graph.json, %q to mutation check", function, stale, state)
		}
	}
	if !seen["stale"] || !seen["fresh"] {
		return fmt.Errorf("mutation check calls the functions %v, not one fresh and one stale\n%s", states, stdout)
	}
	return nil
}
