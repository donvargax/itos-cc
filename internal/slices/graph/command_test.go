package graph

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	"github.com/donvargax/itos-cc/internal/project"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// written is a store of one project's snapshots: its root, the snapshots
// it holds, by name, which Read decodes nothing of, and what Write was
// given, by name. rootFailure and writeFailure are its failures when set.
type written struct {
	root         string
	values       map[string]any
	rootFailure  snapport.Failure
	writeFailure snapport.Failure
}

func (s *written) Root(context.Context) (string, snapport.Failure) { return s.root, s.rootFailure }

func (s *written) Read(context.Context, string, any) (bool, snapport.Failure) { return false, nil }

func (s *written) Write(_ context.Context, name string, v any) snapport.Failure {
	if s.writeFailure != nil {
		return s.writeFailure
	}
	s.values[name] = v
	return nil
}

func (s *written) Remove(context.Context, string) snapport.Failure { return nil }

func (s *written) List(context.Context, string) ([]string, snapport.Failure) { return nil, nil }

// shopModule is a Go module in a folder named checkout: package app,
// importing app/store, each with one function.
func shopModule(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "checkout")
	for name, text := range map[string]string{
		"go.mod":             "module example.com/shop\n\ngo 1.22\n",
		"app/app.go":         "package app\n\nimport \"example.com/shop/app/store\"\n\nfunc Total(n int) bool { return store.Count(n) > 1 }\n",
		"app/store/store.go": "package store\n\nfunc Count(n int) int { return n }\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// shopHandler is the handler of the module at root, its snapshots in
// store, its repository's remote origin url.
func shopHandler(store *written, url string) Handler {
	return Handler{Tree: diskTree{}, Surroundings: project.Surroundings{}, Store: store, Mutation: judging{},
		Origin: func(context.Context, string) string { return url }}
}

// The repository is named as a forge names it, by the last segment of its
// remote origin's path without .git, and by its folder with none.
func TestARepositoryIsNamedByItsRemoteOrigin(t *testing.T) {
	for url, want := range map[string]string{
		"https://example.com/acme/shop.git": "shop",
		"git@example.com:acme/shop.git":     "shop",
		"git@example.com:shop":              "shop",
		"/srv/git/shop.git/":                "shop",
		`C:\git\shop`:                       "shop",
		"shop":                              "shop",
		"":                                  "checkout",
		"https://example.com/":              "example.com",
		".git":                              "checkout",
	} {
		if got := RepositoryName(url, "checkout"); got != want {
			t.Errorf("RepositoryName(%q) = %q, want %q", url, got, want)
		}
	}
}

// graph writes the project's graph to its store as graph.json, its version
// the snapshot format's and its repository named by its remote origin, and
// counts what it holds.
func TestTheGraphIsWrittenToTheStore(t *testing.T) {
	root := shopModule(t)
	store := &written{root: root, values: map[string]any{}}
	got, err := shopHandler(store, "https://example.com/acme/shop.git").Handle(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if want := (Written{Repositories: 1, Modules: 2, Functions: 2, Edges: 1}); got != want {
		t.Errorf("written %+v, want %+v", got, want)
	}
	g, ok := store.values["graph.json"].(*Graph)
	if !ok || g.Version != 1 || node(g, "shop") == nil || node(g, "shop/app/store") == nil {
		t.Fatalf("graph.json: %+v", store.values["graph.json"])
	}
}

// What graph cannot read or write is its failure: the store's root, a
// project that cannot be walked, a missing Mutation port, and a graph the
// store cannot write, beside the counts it built.
func TestWhatTheGraphCannotReadOrWriteIsItsFailure(t *testing.T) {
	root := shopModule(t)
	unwritable := &snapport.Unwritable{Name: "graph.json", Reason: "read-only"}
	store := &written{root: root, values: map[string]any{}, writeFailure: unwritable}
	got, err := shopHandler(store, "").Handle(t.Context())
	if !errors.Is(err, unwritable) || got.Modules != 2 {
		t.Errorf("unwritable: %+v %v, want the counts and the failure", got, err)
	}
	unreadable := &snapport.Unreadable{Name: "", Reason: "no root"}
	if _, err := shopHandler(&written{rootFailure: unreadable}, "").Handle(t.Context()); !errors.Is(err, unreadable) {
		t.Errorf("no root: %v, want the store's failure", err)
	}
	gone := &written{root: filepath.Join(root, "gone"), values: map[string]any{}}
	if _, err := shopHandler(gone, "").Handle(t.Context()); err == nil || len(gone.values) != 0 {
		t.Errorf("a project not there: %v, wrote %v", err, gone.values)
	}
	h := shopHandler(&written{root: root, values: map[string]any{}}, "")
	h.Mutation = nil
	if _, err := h.Handle(t.Context()); err == nil {
		t.Error("a graph was built without its Mutation port")
	}
}

// ports gives a handler what a test chose: the Mutation port or its
// failure.
type ports struct {
	mutation Mutation
	err      error
}

func (ports) SelectionTree() selport.Tree             { return diskTree{} }
func (ports) LangSurroundings() langport.Surroundings { return project.Surroundings{} }
func (ports) SnapshotStore() snapport.Store           { return &written{} }
func (p ports) GraphMutation() (Mutation, error)      { return p.mutation, p.err }
func (ports) RepositoryOrigin(context.Context, string) string {
	return "https://example.com/acme/shop.git"
}

// The handler is built over its ports; a Mutation port that cannot be bound
// is the failure, as the ports gave it.
func TestTheHandlerIsBuiltOverItsPorts(t *testing.T) {
	h, err := NewHandler(t.Context(), ports{mutation: judging{}})
	if err != nil || h.Tree == nil || h.Surroundings == nil || h.Store == nil || h.Mutation == nil || h.Origin(t.Context(), "") != "https://example.com/acme/shop.git" {
		t.Errorf("handler %+v %v", h, err)
	}
	unbound := errors.New("unbound")
	if _, err := NewHandler(t.Context(), ports{err: unbound}); !errors.Is(err, unbound) {
		t.Errorf("unbound: %v, want the ports' failure", err)
	}
}

// graph prints the counts a person reads, or the object a script reads, and
// a failure as a problem.
func TestGraphPrintsWhatItWrote(t *testing.T) {
	root := shopModule(t)
	run := func(c CLI, store *written) (int, string) {
		var out, errs bytes.Buffer
		code := c.Run(t.Context(), &cli.UI{Stdout: &out, Stderr: &errs}, shopHandler(store, ""))
		return code, out.String()
	}
	if code, out := run(CLI{}, &written{root: root, values: map[string]any{}}); code != 0 || out != "2 modules, 2 functions and 1 edges in .metrics/graph.json\n" {
		t.Errorf("text: %d %q", code, out)
	}
	if code, out := run(CLI{JSON: true}, &written{root: root, values: map[string]any{}}); code != 0 || !strings.Contains(out, `"modules": 2`) {
		t.Errorf("json: %d %q", code, out)
	}
	unwritable := &written{root: root, values: map[string]any{}, writeFailure: &snapport.Unwritable{Name: "graph.json", Reason: "read-only"}}
	if code, out := run(CLI{JSON: true}, unwritable); code == 0 || !strings.Contains(out, `"ok": false`) {
		t.Errorf("unwritable: %d %q", code, out)
	}
	if contract := (CLI{}).Contract(); !strings.Contains(contract.About, ".metrics/graph.json") {
		t.Errorf("contract: %+v", contract)
	}
}

// A graph's files are listed by repository, the name its nodes' ids begin
// with, as serve's source endpoint may serve them.
func TestAGraphsFilesAreListedByRepository(t *testing.T) {
	g := &Graph{Nodes: []*Node{
		{ID: "shop", Kind: "repo"},
		{ID: "shop/app", Kind: "module", Files: []string{"app/app.go", "app/more.go"}},
		{ID: "web/src", Kind: "module", Files: []string{"src/index.ts"}},
	}}
	files := g.Files()
	if len(files) != 2 || strings.Join(files["shop"], ",") != "app/app.go,app/more.go" || strings.Join(files["web"], ",") != "src/index.ts" {
		t.Errorf("files %v", files)
	}
}
