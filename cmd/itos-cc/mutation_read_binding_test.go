package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mutation "github.com/donvargax/itos-cc/internal/mutants"
	mutantstest "github.com/donvargax/itos-cc/internal/mutants/porttest"
	seltest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

const runSourceForRead = "package board\nfunc Value(x int) bool { return x > 1 }\n"

func compactRead(out string) string {
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(out)); err != nil {
		return out
	}
	return compact.String()
}

func invokeRead(t *testing.T, f *mutationRunFixture, a adapters, args ...string) (int, string) {
	t.Helper()
	f.stdout.Reset()
	f.stderr.Reset()
	code := 0
	out, log := captured(t, func() { code = run(t.Context(), args, strings.NewReader(""), &f.stdout, &f.stderr, a) })
	return code, out + log + f.stderr.String()
}

func TestMutationCheckUsesActualReadOnlyBindings(t *testing.T) {
	f := newMutationRunFixture(t)
	if code, out := f.invoke(t, f.adapters, "--no-coverage", "--no-annotate"); code != 0 {
		t.Fatalf("actual recorded setup: %d %s", code, out)
	}
	base := f.adapters
	base.SnapshotReader = f.snapshots
	beforeWrites := len(f.snapshots.Written)
	invoke := func(a adapters) (int, string) { return invokeRead(t, f, a, "mutation", "check", "--json", "board.go") }
	if code, out := invoke(base); code != 0 || !strings.Contains(compactRead(out), `"state":"fresh"`) {
		t.Fatalf("bound current check: %d %s", code, out)
	}
	for _, c := range []struct {
		name, token string
		change      func(*adapters)
	}{
		{"missing reader", "snapshots reader binding is missing", func(a *adapters) { a.SnapshotReader = nil }},
		{"wrong reader", "actual unreadable model", func(a *adapters) {
			a.SnapshotReader = &mutantstest.Snapshots{RootDir: f.root, Failures: map[string]snapport.Failure{"board.go": &snapport.Unreadable{Reason: "actual unreadable model"}}}
		}},
		{"missing tree", "source tree binding is missing", func(a *adapters) { a.Tree = nil }},
		{"missing surroundings", "source tree binding is missing", func(a *adapters) { a.Surroundings = nil }},
		{"wrong tree", "fake", func(a *adapters) {
			tree := seltest.TreeOf(f.root, map[string]string{"board.go": runSourceForRead, "board_test.go": "package board\n", "go.mod": "module example.test/board\n"}, nil)
			delete(tree.Contents, "board.go")
			a.Tree = tree
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := base
			c.change(&a)
			if code, out := invoke(a); code != 70 || !strings.Contains(compactRead(out), c.token) {
				t.Fatalf("binding escaped: %d %s", code, out)
			}
		})
	}
	a := base
	a.SnapshotReader = &mutantstest.Snapshots{RootDir: f.root}
	if code, out := invoke(a); code != 1 || !strings.Contains(compactRead(out), "mutation.missing") {
		t.Fatalf("different actual model not observed: %d %s", code, out)
	}
	if len(f.snapshots.Written) != beforeWrites || len(f.edits.Writes) != 0 {
		t.Fatal("check wrote")
	}
}

func TestMutationListUsesBoundTreeAndRendersPrefixBeforeLaterReadFailure(t *testing.T) {
	f := newMutationRunFixture(t)
	// Legacy selection still sees the physical fixture; the actual domain read
	// sees independently held bytes. The second selected source is absent there.
	if err := os.WriteFile(filepath.Join(f.root, "z.go"), []byte("package board\nfunc Z() int { return 2 }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"mutation", "list", "--json", "."}
	if code, out := invokeRead(t, f, f.adapters, args...); code != 70 || !strings.Contains(compactRead(out), `"file":"board.go"`) || !strings.Contains(compactRead(out), "fake") {
		t.Fatalf("ordered JSON prefix: %d %s", code, out)
	}
	if code, out := invokeRead(t, f, f.adapters, "mutation", "list", "."); code != 70 || !strings.Contains(compactRead(out), "board.go:2:") || !strings.Contains(compactRead(out), "fake") {
		t.Fatalf("ordered plain prefix: %d %s", code, out)
	}
	if code, out := invokeRead(t, f, f.adapters, "mutation", "check", "--json", "."); code != 70 || !strings.Contains(compactRead(out), `"files":[]`) || strings.Contains(compactRead(out), `"function":`) {
		t.Fatalf("check published a partial prefix: %d %s", code, out)
	}
	a := f.adapters
	a.Tree = nil
	if code, out := invokeRead(t, f, a, "mutation", "list", "board.go"); code != 70 || !strings.Contains(compactRead(out), "source tree binding is missing") {
		t.Fatalf("list disk fallback: %d %s", code, out)
	}
	a = f.adapters
	a.Tree = seltest.TreeOf(f.root, map[string]string{"board.go": "package board\nfunc NoSites() {}\n", "go.mod": "module example.test/board\n"}, nil)
	if code, out := invokeRead(t, f, a, "mutation", "list", "--json", "board.go"); code != 0 || !strings.Contains(compactRead(out), `"sites":[]`) {
		t.Fatalf("actual bound zero-site source ignored: %d %s", code, out)
	}
}

func TestMutationOrphansUseActualCatalogAndIndependentPresence(t *testing.T) {
	f := newMutationRunFixture(t)
	gone := filepath.Join(f.root, "gone.go")
	catalog := &mutantstest.Snapshots{RootDir: f.root, CatalogFiles: map[string]string{"mutate/gone.go.json": "gone.go"}}
	source := &mutantstest.SourceEdits{}
	repo := mutantstest.NewHistoryRepository()
	repo.Index = map[string]string{"board.go": f.memory.files[filepath.Join(f.root, "board.go")]}
	repo.Commit("actual independent history")
	a := f.adapters
	a.Catalog = catalog
	a.Presence = source
	a.History = mutantstest.History{Root: f.root, Dir: f.root, Repository: repo, Working: f.memory.files}
	args := []string{"mutation", "check", "--json", "."}
	if code, out := invokeRead(t, f, a, args...); code != 1 || !strings.Contains(compactRead(out), `"file":"gone.go","snapshot":".metrics/mutate/gone.go.json","why":"gone"`) {
		t.Fatalf("actual bound orphan: %d %s", code, out)
	}
	source.Directories = map[string]bool{gone: true}
	if _, out := invokeRead(t, f, a, args...); strings.Contains(compactRead(out), `"snapshot":".metrics/mutate/gone.go.json"`) {
		t.Fatalf("independent present directory ignored: %s", out)
	}
	for _, c := range []struct {
		name, token string
		change      func(*adapters)
	}{
		{"missing catalog", "catalog or source presence binding is missing", func(a *adapters) { a.Catalog = nil }},
		{"missing presence", "catalog or source presence binding is missing", func(a *adapters) { a.Presence = nil }},
		{"wrong catalog", "actual catalog unavailable", func(a *adapters) {
			a.Catalog = &mutantstest.Snapshots{RootDir: f.root, ListFailure: &snapport.Unlistable{Reason: "actual catalog unavailable"}}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			bad := a
			c.change(&bad)
			if code, out := invokeRead(t, f, bad, args...); code != 70 || !strings.Contains(compactRead(out), c.token) {
				t.Fatalf("binding escaped: %d %s", code, out)
			}
		})
	}
	if len(catalog.Written) != 0 || len(catalog.Removed) != 0 || len(source.Writes) != 0 {
		t.Fatal("orphan check wrote")
	}
}

var _ mutation.SourcePresence = (*mutantstest.SourceEdits)(nil)
