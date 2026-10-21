package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/config"
	seltest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/exceptions"
	mutation "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	mutantstest "github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

func exceptionBindingFixture(t *testing.T) (*mutationRunFixture, adapters, mutation.Exception) {
	t.Helper()
	f := newMutationRunFixture(t)
	if code, out := f.invoke(t, f.adapters, "--no-coverage", "--no-annotate"); code != 0 {
		t.Fatalf("actual recorded setup: %d %s", code, out)
	}
	snap := f.snapshots.Values["board.go"]
	for i := range snap.Units {
		for j := range snap.Units[i].Mutants {
			snap.Units[i].Mutants[j].Outcome = mutation.Survived
		}
	}
	f.snapshots.Values["board.go"] = snap
	a := f.adapters
	a.SnapshotReader = f.snapshots
	a.SourceStatus = f.edits
	path := filepath.Join(f.root, "board.go")
	reach, failure := a.Reach.TestsImporting(t.Context(), f.root, []string{path}, []string{filepath.Join(f.root, "board_test.go")})
	if failure != nil {
		t.Fatal(failure)
	}
	opt := mutation.CheckOptions{Tests: func(path string) []string { return reach[path] }, Root: f.root, Dir: f.root, Store: f.snapshots, Tree: a.Tree, Surroundings: a.Surroundings, Toolchain: a.Toolchain, Fingerprints: a.Fingerprints}
	m := snap.Units[0].Mutants[0]
	e, err := mutation.Survivor(t.Context(), path, m.Line, m.Column, opt, f.edits)
	if err != nil {
		t.Fatal(err)
	}
	e.Reason = "independently reviewed"
	store := &mutantstest.Exceptions{}
	a.Exceptions = func(string) mutation.Exceptions { return store }
	return f, a, e
}
func TestMutationExceptionConsumersRequireActualBindings(t *testing.T) {
	f, base, entry := exceptionBindingFixture(t)
	site := fmt.Sprintf("board.go:%d:%d", f.snapshots.Values["board.go"].Units[0].Mutants[0].Line, entry.Column)
	invoke := func(a adapters, args ...string) (int, string) { return invokeRead(t, f, a, args...) }
	except := []string{"mutation", "except", site, "--reason", entry.Reason, "--json"}
	store := base.Exceptions(f.root).(*mutantstest.Exceptions)
	if code, out := invoke(base, except...); code != 0 || !reflect.DeepEqual(store.Entries, []mutation.Exception{entry}) {
		t.Fatalf("bound except: %d %s %+v", code, out, store.Entries)
	}
	if _, err := os.Stat(filepath.Join(f.root, config.File)); !os.IsNotExist(err) {
		t.Fatalf("except fell back to host: %v", err)
	}
	// Initial selected entries live on the disposable legacy admission boundary.
	// Updates must use the independent injected CURRENT store, leaving this decoy.
	old := entry
	old.Hash = "earlier function hash"
	data, _, err := exceptions.ExceptDocument([]byte("# physical decoy\n"), old)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.root, config.File), data, 0600); err != nil {
		t.Fatal(err)
	}
	store.Entries = []mutation.Exception{old}
	renew := []string{"mutation", "except", "--renew", "--json"}
	if code, out := invoke(base, renew...); code != 0 || len(store.Entries) != 1 || store.Entries[0] != entry {
		t.Fatalf("bound renew: %d %s %+v", code, out, store.Entries)
	}
	disk, err := os.ReadFile(filepath.Join(f.root, config.File))
	if err != nil || !bytes.Equal(disk, data) {
		t.Fatalf("renew fell back to host: %v", err)
	}
	for _, consumer := range []struct {
		name string
		args []string
	}{{"except", except}, {"renew", renew}} {
		t.Run(consumer.name, func(t *testing.T) {
			for _, c := range []struct {
				name, token string
				code        int
				change      func(*adapters)
			}{
				{"missing exceptions", "Exceptions binding is missing", 100, func(a *adapters) { a.Exceptions = nil }},
				{"wrong exceptions", "actual document read denied", 100, func(a *adapters) {
					a.Exceptions = func(string) mutation.Exceptions {
						return &mutantstest.Exceptions{Failure: mutation.ExceptionFailed{Cause: errors.New("actual document read denied")}}
					}
				}},
				{"invalid exceptions", "config.invalid", 80, func(a *adapters) {
					a.Exceptions = func(string) mutation.Exceptions {
						return &mutantstest.Exceptions{Failure: mutation.ExceptionInvalid{Reason: "wrong independent document shape", Key: "mutation"}}
					}
				}},
				{"missing status", "source status binding is missing", 100, func(a *adapters) { a.SourceStatus = nil }},
				{"missing reader", "snapshots reader binding is missing", 100, func(a *adapters) { a.SnapshotReader = nil }},
				{"wrong reader", "actual read-only snapshot denied", 100, func(a *adapters) {
					a.SnapshotReader = &mutantstest.Snapshots{RootDir: f.root, Failures: map[string]snapport.Failure{"board.go": &snapport.Unreadable{Reason: "actual read-only snapshot denied"}}}
				}},
				{"missing tree", "source tree binding is missing", 100, func(a *adapters) { a.Tree = nil }},
				{"wrong tree", "outside the requested inventory", 100, func(a *adapters) { a.Tree = seltest.TreeOf(f.root, map[string]string{}, nil) }},
			} {
				t.Run(c.name, func(t *testing.T) {
					a := base
					c.change(&a)
					if code, out := invoke(a, consumer.args...); code != c.code || !strings.Contains(compactRead(out), c.token) {
						t.Fatalf("binding escaped: %d %s", code, out)
					}
				})
			}
		})
	}
	wrong := base
	wrong.SourceStatus = &mutantstest.SourceEdits{}
	if code, out := invoke(wrong, except...); code != 80 || !strings.Contains(out, "exception.no-survivor") {
		t.Fatalf("missing actual source ignored: %d %s", code, out)
	}
	wrong.SourceStatus = &mutantstest.SourceEdits{StatErrors: map[string]error{filepath.Join(f.root, "board.go"): errors.New("actual following Stat denied")}}
	if code, out := invoke(wrong, renew...); code != 100 || !strings.Contains(out, "actual following Stat denied") {
		t.Fatalf("wrong renewal Stat ignored: %d %s", code, out)
	}
}
func TestMutationSampleUsesActualReadAndExecutionBindings(t *testing.T) {
	f, a, _ := exceptionBindingFixture(t)
	args := []string{"mutation", "sample", "--seed", "fixed", "--count", "2", "--json", "board.go"}
	writes := len(f.snapshots.Written)
	if code, out := invokeRead(t, f, a, args...); code != 1 || !strings.Contains(out, "mutation.mismatch") {
		t.Fatalf("actual changed worker outcome: %d %s", code, out)
	}
	if len(f.snapshots.Written) != writes || len(f.edits.Writes) != 0 {
		t.Fatal("sample wrote")
	}
	for _, c := range []struct {
		name, token string
		change      func(*adapters)
	}{
		{"missing reader", "snapshots reader binding is missing", func(a *adapters) { a.SnapshotReader = nil }},
		{"wrong reader", "actual sample reader denied", func(a *adapters) {
			a.SnapshotReader = &mutantstest.Snapshots{RootDir: f.root, RootFailure: &snapport.Unreadable{Reason: "actual sample reader denied"}}
		}},
		{"missing settings", "settings binding is missing", func(a *adapters) { a.Settings = nil }},
		{"wrong settings", "independent settings unavailable", func(a *adapters) {
			a.Settings = &mutantstest.Settings{Failure: mutation.InputFailed{Cause: errors.New("independent settings unavailable")}}
		}},
		{"missing tree", "source tree binding is missing", func(a *adapters) { a.Tree = nil }},
		// The selection reads the bound tree too: it lists board.go, whose
		// bytes that tree does not hold, while the disk does.
		{"wrong tree", "fake", func(a *adapters) {
			tree := seltest.TreeOf(f.root, map[string]string{"board.go": runSourceForRead, "board_test.go": "package board\n", "go.mod": "module example.test/board\n"}, nil)
			delete(tree.Contents, "board.go")
			a.Tree = tree
		}},
		{"missing workers", "Workers binding is missing", func(a *adapters) { a.Workers = nil }},
		{"wrong workers", "no tests declared", func(a *adapters) { a.Workers = &mutantstest.Workers{Files: f.memory.files} }},
	} {
		t.Run(c.name, func(t *testing.T) {
			bad := a
			c.change(&bad)
			if code, out := invokeRead(t, f, bad, args...); code != 100 || !strings.Contains(compactRead(out), c.token) {
				t.Fatalf("sample binding escaped: %d %s", code, out)
			}
		})
	}
}
func TestMemoryExceptionWriterUsesCurrentDocumentBytes(t *testing.T) {
	m := newMemory(t)
	root := t.TempDir()
	path := filepath.Join(root, config.File)
	old := mutation.Exception{File: "a.go", Function: "p#F", Hash: "old", LineInFunction: 1, Column: 2, Original: ">", Replacement: ">=", Reason: "reviewed"}
	original, _, err := exceptions.ExceptDocument(nil, old)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Write(path, original); err != nil {
		t.Fatal(err)
	}
	store := memoryExceptions{memory: m, root: root}
	current := append([]byte("# added AFTER initial selection\nexclude: [vendor/**]\n"), original...)
	if err := m.Write(path, current); err != nil {
		t.Fatal(err)
	}
	next := old
	next.Hash = "new"
	if failure := store.Renew(t.Context(), []mutation.Renewal{{Old: old, New: next}}); failure != nil {
		t.Fatal(failure)
	}
	actual, err := m.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Parse(actual)
	if err != nil || !reflect.DeepEqual(cfg.Exceptions, []mutation.Exception{next}) || !strings.Contains(string(actual), "AFTER initial selection") || !reflect.DeepEqual(cfg.Selection.Exclude, []string{"vendor/**"}) {
		t.Fatalf("current memory document lost: %s %v", actual, err)
	}
	// A failed complete-Old batch never publishes an earlier renewal.
	before := string(actual)
	failure := store.Renew(t.Context(), []mutation.Renewal{{Old: next, New: old}, {Old: old, New: next}})
	after, err := m.Read(path)
	if failure == nil || err != nil || string(after) != before {
		t.Fatalf("partial memory batch: %s %v %v", after, failure, err)
	}
	updated := next
	updated.Reason = "reviewed again"
	replaced, failure := store.Except(t.Context(), updated)
	actual, err = m.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Parse(actual)
	if failure != nil || !replaced || err != nil || !reflect.DeepEqual(cfg.Exceptions, []mutation.Exception{updated}) || !strings.Contains(string(actual), "AFTER initial selection") {
		t.Fatalf("current memory replacement: %s %v %v", actual, failure, err)
	}
	missing := memoryExceptions{memory: m, root: filepath.Join(root, "new-project")}
	replaced, failure = missing.Except(t.Context(), old)
	actual, err = m.Read(filepath.Join(missing.root, config.File))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Parse(actual)
	if failure != nil || replaced || err != nil || !reflect.DeepEqual(cfg.Exceptions, []mutation.Exception{old}) {
		t.Fatalf("missing memory creation: %s %v %v", actual, failure, err)
	}
}
