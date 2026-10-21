package porttest

import (
	"context"
	"errors"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
	"io/fs"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestMemoryMutationCatalogContract(t *testing.T) {
	MutationCatalogContract(t, func(t *testing.T, files map[string]string) (mutants.MutationCatalog, string) {
		root := t.TempDir()
		return &Snapshots{RootDir: root, CatalogFiles: files}, root
	})
}
func TestMemorySourcePresenceContract(t *testing.T) {
	SourcePresenceContract(t, func(t *testing.T) (mutants.SourcePresence, string) {
		root := t.TempDir()
		return &SourceEdits{Files: map[string]string{filepath.Join(root, "file.go"): "actual bytes"}, Directories: map[string]bool{filepath.Join(root, "directory"): true}}, root
	})
}
func TestMemoryLstatFactsAreIndependentFromFollowStat(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "dangling")
	source := &SourceEdits{Links: map[string]string{link: "missing-target"}, StatErrors: map[string]error{link: fs.ErrNotExist}}
	if missing, failure := source.Missing(t.Context(), link); failure != nil || !missing {
		t.Fatalf("follow missing: %v %v", missing, failure)
	}
	for _, observe := range []func() (bool, mutants.InputFailure){func() (bool, mutants.InputFailure) { return source.Present(t.Context(), link) }, func() (bool, mutants.InputFailure) { return source.RenameTarget(t.Context(), link) }} {
		if got, failure := observe(); failure != nil || !got {
			t.Fatalf("dangling no-follow fact: %v %v", got, failure)
		}
	}
	denied := filepath.Join(root, "denied")
	source.LstatErrors = map[string]error{denied: errors.New("independent permission failure")}
	if got, failure := source.Present(t.Context(), denied); failure != nil || !got {
		t.Fatalf("error cannot prove gone: %v %v", got, failure)
	}
	if got, failure := source.RenameTarget(t.Context(), denied); failure != nil || got {
		t.Fatalf("error cannot prove target: %v %v", got, failure)
	}
	source.LstatErrors[denied] = fs.ErrNotExist
	if got, failure := source.Present(t.Context(), denied); failure != nil || got {
		t.Fatalf("not-exist: %v %v", got, failure)
	}
}

func TestMemoryMutationCatalogLifecycleContract(t *testing.T) {
	MutationCatalogLifecycleContract(t, func(t *testing.T, initial mutants.Snapshot) (MutationCatalogStore, string) {
		root := t.TempDir()
		return &Snapshots{RootDir: root, Values: map[string]mutants.Snapshot{"a.go": cloneSnapshot(initial)}}, root
	})
}

func TestMemoryCatalogPublicationPreservesRawFactsAndFailures(t *testing.T) {
	raw := `mutate/literal\name.go.json`
	malformed := &snapport.Unreadable{Reason: "independent malformed metadata"}
	store := &Snapshots{RootDir: t.TempDir(), Values: map[string]mutants.Snapshot{"a.go": {File: "old.go"}}, CatalogFiles: map[string]string{raw: "literal-file", "mutate/a.go.json": "old.go"}, CatalogFailures: map[string]snapport.Failure{raw: malformed, "mutate/a.go.json": malformed}}
	before, failure := store.List(t.Context())
	if failure != nil || !slices.Contains(before, raw) {
		t.Fatalf("raw catalog setup: %v %v", before, failure)
	}
	if file, failure := store.RecordedFile(t.Context(), raw); file != "" || failure != malformed {
		t.Fatalf("raw malformed state: %q %v", file, failure)
	}
	old := cloneSnapshot(store.Values["a.go"])
	store.WriteFailures = map[string]snapport.Failure{"a.go": &snapport.Unwritable{Reason: "actual publication unavailable"}}
	next := mutants.Snapshot{File: "new.go"}
	if failure := store.Write(t.Context(), "a.go", next); failure == nil {
		t.Fatal("unavailable publication succeeded")
	}
	if !reflect.DeepEqual(store.Values["a.go"], old) || store.CatalogFiles["mutate/a.go.json"] != "old.go" || store.CatalogFailures["mutate/a.go.json"] != malformed {
		t.Fatal("failed publication changed state")
	}
	store.Failures = map[string]snapport.Failure{"a.go": &snapport.Unremovable{Reason: "actual removal unavailable"}}
	if failure := store.Remove(t.Context(), "a.go"); failure == nil || !reflect.DeepEqual(store.Values["a.go"], old) {
		t.Fatalf("failed removal changed state: %v", failure)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if failure := store.Write(ctx, "a.go", next); failure == nil {
		t.Fatal("cancelled publication succeeded")
	}
	if failure := store.Remove(ctx, "a.go"); failure == nil {
		t.Fatal("cancelled removal succeeded")
	}
	if store.CatalogFiles["mutate/a.go.json"] != "old.go" || store.CatalogFailures["mutate/a.go.json"] != malformed {
		t.Fatal("cancelled operation changed metadata")
	}
	delete(store.WriteFailures, "a.go")
	delete(store.Failures, "a.go")
	if failure := store.Write(t.Context(), "a.go", next); failure != nil {
		t.Fatal(failure)
	}
	if file, failure := store.RecordedFile(t.Context(), "mutate/a.go.json"); failure != nil || file != "new.go" {
		t.Fatalf("overwrite retained old metadata: %q %v", file, failure)
	}
	if failure := store.Remove(t.Context(), "a.go"); failure != nil {
		t.Fatal(failure)
	}
	names, failure := store.List(t.Context())
	if failure != nil || !reflect.DeepEqual(names, []string{raw}) {
		t.Fatalf("remove changed raw names: %v %v", names, failure)
	}
	if _, failure := store.RecordedFile(t.Context(), raw); failure != malformed {
		t.Fatalf("unrelated malformed record erased: %v", failure)
	}
}
