package metricsdir_test

import (
	"github.com/donvargax/itos-cc/internal/metricsdir"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestRealMutationCatalogContract(t *testing.T) {
	porttest.MutationCatalogContract(t, func(t *testing.T, files map[string]string) (mutants.MutationCatalog, string) {
		root := t.TempDir()
		for name, file := range files {
			path := filepath.Join(root, ".metrics", filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(`{"file":`+strconv.Quote(file)+`}`), 0600); err != nil {
				t.Fatal(err)
			}
		}
		// A JSON-named directory is not a regular snapshot.
		if err := os.MkdirAll(filepath.Join(root, ".metrics", "mutate", "directory.json"), 0700); err != nil {
			t.Fatal(err)
		}
		return metricsdir.Mutations{Store: metricsdir.Store{RootDir: root}}, root
	})
}
func TestRecordedFileIsPermissiveAndSeparateFromFullSnapshotDecode(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".metrics", "mutate")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store := metricsdir.Mutations{Store: metricsdir.Store{RootDir: root}}
	// The wrong version type invalidates the full schema without invalidating file metadata.
	for _, text := range []string{`{"file":"outside.go","version":"invalid"}`, `{"file":"outside.go","units":"invalid"}`} {
		if err := os.WriteFile(filepath.Join(dir, "a.go.json"), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		if file, failure := store.RecordedFile(t.Context(), "mutate/a.go.json"); failure != nil || file != "outside.go" {
			t.Fatalf("metadata rejected: %q %v", file, failure)
		}
		if _, failure := store.Read(t.Context(), "a.go"); failure == nil {
			t.Fatal("strict snapshot accepted malformed full schema")
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go.json"), []byte(`{"file":17}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, failure := store.RecordedFile(t.Context(), "mutate/a.go.json"); failure == nil {
		t.Fatal("malformed file metadata accepted")
	}
}

func TestRealMutationCatalogLifecycleContract(t *testing.T) {
	porttest.MutationCatalogLifecycleContract(t, func(t *testing.T, initial mutants.Snapshot) (porttest.MutationCatalogStore, string) {
		root := t.TempDir()
		store := metricsdir.Mutations{Store: metricsdir.Store{RootDir: root}}
		if failure := store.Write(t.Context(), "a.go", initial); failure != nil {
			t.Fatal(failure)
		}
		return store, root
	})
}
