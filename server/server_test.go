package server

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// viewer is a built viewer's files: its index.html saying says, and an
// asset.
func viewer(says string) fstest.MapFS {
	return fstest.MapFS{
		"index.html":    {Data: []byte("<p>" + says + "</p>\n")},
		"assets/app.js": {Data: []byte("console.log(\"" + says + "\");\n")},
	}
}

// withBuiltIn makes ui the viewer the binary was built with for the test.
func withBuiltIn(t *testing.T, ui fs.FS) {
	t.Helper()
	before := builtIn
	builtIn = ui
	t.Cleanup(func() { builtIn = before })
}

// get is the status and body s's handler answers GET path with.
func get(t *testing.T, s *Server, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	body, err := io.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	return rec.Code, string(body)
}

// The folder a release copies viewer/dist into is the viewer when it
// holds an index.html, and no viewer when it holds only its .gitignore.
func TestCarried(t *testing.T) {
	ui := carried(fstest.MapFS{"ui/.gitignore": {Data: []byte("*\n")}, "ui/index.html": {Data: []byte("<p>built in</p>\n")}})
	if ui == nil {
		t.Fatal("carried = nil for a folder holding index.html")
	}
	if page, err := fs.ReadFile(ui, "index.html"); err != nil || string(page) != "<p>built in</p>\n" {
		t.Errorf("the carried viewer's index.html = %q, %v", page, err)
	}
	if ui := carried(fstest.MapFS{"ui/.gitignore": {Data: []byte("*\n")}}); ui != nil {
		t.Errorf("carried = %v for a folder holding only its .gitignore, want nil", ui)
	}
}

// / serves the viewer the binary was built with, its files at their paths
// and its index.html for any other route.
func TestHandlerServesTheBuiltInViewer(t *testing.T) {
	withBuiltIn(t, viewer("built-in viewer"))
	for path, want := range map[string]string{
		"/":              "<p>built-in viewer</p>\n",
		"/assets/app.js": "console.log(\"built-in viewer\");\n",
		"/shop/web":      "<p>built-in viewer</p>\n",
	} {
		if code, body := get(t, &Server{}, path); code != http.StatusOK || body != want {
			t.Errorf("GET %s = %d %q, want 200 %q", path, code, body, want)
		}
	}
}

// --ui's directory replaces the built-in viewer.
func TestHandlerServesTheUIDirectory(t *testing.T) {
	withBuiltIn(t, viewer("built-in viewer"))
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<p>other viewer</p>\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/shop/web"} {
		if code, body := get(t, &Server{ui: dir}, path); code != http.StatusOK || body != "<p>other viewer</p>\n" {
			t.Errorf("GET %s = %d %q, want --ui's index.html", path, code, body)
		}
	}
}

// A binary built without a viewer, run without --ui, says at / how to get
// one, and has nothing at any other route.
func TestHandlerWithoutAViewer(t *testing.T) {
	withBuiltIn(t, nil)
	if code, body := get(t, &Server{}, "/"); code != http.StatusOK || !strings.HasPrefix(body, "itos-cc API: /api/graph, /api/events, /api/source. Start the viewer, or pass --ui viewer/dist.") {
		t.Errorf("GET / = %d %q, want the API's text line", code, body)
	}
	if code, _ := get(t, &Server{}, "/shop/web"); code != http.StatusNotFound {
		t.Errorf("GET /shop/web = %d, want 404", code)
	}
}

// built is a fake Builder: graph is what each build gives, changed
// whether it changed, err its failure, and roots the repositories' roots.
type built struct {
	graph   Graph
	changed bool
	err     error
	roots   map[string]string
}

func (b *built) Build(context.Context) (Graph, bool, error) { return b.graph, b.changed, b.err }

func (b *built) Roots() map[string]string { return b.roots }

// The server publishes its builder's graph as JSON, and serves the files of
// each repository the graph lists, from the repository's root, and no other.
func TestTheServerPublishesItsBuildersGraph(t *testing.T) {
	root := t.TempDir()
	for name, text := range map[string]string{"a.go": "package a\n", "b.go": "package b\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	b := &built{graph: Graph{Version: 1, Body: map[string]int{"version": 1}, Files: map[string][]string{"shop": {"a.go"}}}, changed: true, roots: map[string]string{"shop": root}}
	s, err := New(t.Context(), b, "", log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	if code, body := get(t, s, "/api/graph"); code != http.StatusOK || body != `{"version":1}` {
		t.Errorf("GET /api/graph = %d %q, want the builder's graph", code, body)
	}
	if code, body := get(t, s, "/api/source?repo=shop&file=a.go"); code != http.StatusOK || body != "package a\n" {
		t.Errorf("GET a.go = %d %q, want its text", code, body)
	}
	for _, query := range []string{"repo=shop&file=b.go", "repo=other&file=a.go"} {
		if code, _ := get(t, s, "/api/source?"+query); code != http.StatusNotFound {
			t.Errorf("GET /api/source?%s = %d, want 404", query, code)
		}
	}

	// A build that changed nothing, or failed, keeps the graph published.
	b.graph, b.changed = Graph{Version: 2, Body: map[string]int{"version": 2}}, false
	if err := s.rebuild(t.Context()); err != nil {
		t.Fatal(err)
	}
	b.err = errors.New("mid-save")
	if err := s.rebuild(t.Context()); !errors.Is(err, b.err) {
		t.Errorf("rebuild: %v, want the build's failure", err)
	}
	if code, body := get(t, s, "/api/graph"); code != http.StatusOK || body != `{"version":1}` {
		t.Errorf("GET /api/graph = %d %q, want the first graph kept", code, body)
	}
	b.err = errors.New("unreadable")
	if _, err := New(t.Context(), b, "", log.New(io.Discard, "", 0)); !errors.Is(err, b.err) {
		t.Errorf("New: %v, want the first build's failure", err)
	}
}
