package metricsdir

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/internal/snapshot/port"
	"github.com/donvargax/itos-cc/internal/snapshot/port/porttest"
	"github.com/donvargax/itos-cc/metrics"
)

// inProject is a temporary project with no git above it, the test's working
// directory for its duration, so its root is the directory itself.
func inProject(t *testing.T) string {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	t.Chdir(dir)
	return dir
}

// open lays snapshots, raw text by name, out under .metrics in a project
// of its own, as inProject makes one.
func open(t *testing.T, snapshots map[string]string) (port.Store, string) {
	root := inProject(t)
	for _, name := range slices.Sorted(maps.Keys(snapshots)) {
		path := filepath.Join(root, metrics.Name, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(snapshots[name]), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return Store{}, root
}

// The adapter keeps the contract every Store keeps, the fake's included.
func TestStoreKeepsTheContract(t *testing.T) {
	porttest.StoreContract(t, open)
}

func cancelled(t *testing.T) context.Context {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return ctx
}

type sample struct {
	Version int      `json:"version"`
	Files   []string `json:"files"`
}

// A snapshot written through the store is the file the metrics library
// writes, byte for byte, under .metrics at the root, and reads back.
func TestStoreWritesWhatTheLibraryWrites(t *testing.T) {
	inProject(t)
	store := Store{}
	root, failure := store.Root(t.Context())
	if failure != nil {
		t.Fatalf("Root() = %v", failure)
	}
	v := sample{Version: 1, Files: []string{"src/a<b>.go"}}
	if failure := store.Write(t.Context(), "store.json", v); failure != nil {
		t.Fatalf("Write(store.json) = %v", failure)
	}
	if err := metrics.Write("library.json", v); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(filepath.Join(root, ".metrics", "store.json"))
	if err != nil {
		t.Fatalf("no snapshot under the root: %v", err)
	}
	if want, _ := os.ReadFile(filepath.Join(root, ".metrics", "library.json")); !bytes.Equal(written, want) {
		t.Errorf("the store wrote\n%s\nthe library writes\n%s", written, want)
	}
	var got sample
	if found, failure := store.Read(t.Context(), "store.json", &got); !found || failure != nil || got.Files[0] != "src/a<b>.go" {
		t.Errorf("Read(store.json) = %t, %v, %+v", found, failure, got)
	}
	if found, failure := store.Read(t.Context(), "none.json", &got); found || failure != nil {
		t.Errorf("Read(none.json) = %t, %v, want not there", found, failure)
	}
}

// A snapshot that does not decode is Unreadable, and one that cannot be
// stored Unwritable, each with the library's own text whole.
func TestStoreTranslatesTheLibrarysFailures(t *testing.T) {
	root := inProject(t)
	if err := os.MkdirAll(filepath.Join(root, ".metrics"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".metrics", "bad.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	var v sample
	_, want := metrics.Read("bad.json", &v)
	found, failure := Store{}.Read(t.Context(), "bad.json", &v)
	var unreadable *port.Unreadable
	if want == nil || !found || !errors.As(failure, &unreadable) || unreadable.Name != "bad.json" || unreadable.Reason != want.Error() {
		t.Errorf("Read(bad.json) = %t, %T %v, want Unreadable %v", found, failure, failure, want)
	}

	// A file where the snapshots' directory should be leaves nowhere to
	// write one.
	if err := os.WriteFile(filepath.Join(root, ".metrics", "sub"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	want = metrics.Write("sub/x.json", v)
	failure = Store{}.Write(t.Context(), "sub/x.json", v)
	var unwritable *port.Unwritable
	if want == nil || !errors.As(failure, &unwritable) || unwritable.Name != "sub/x.json" || unwritable.Reason != want.Error() {
		t.Errorf("Write(sub/x.json) = %T %v, want Unwritable %v", failure, failure, want)
	}
}

// Once the context is done, the store asks, reads and writes nothing.
func TestStoreStopsOnceCancelled(t *testing.T) {
	root := inProject(t)
	ctx := cancelled(t)
	var c *port.Cancelled
	if _, failure := (Store{}).Root(ctx); !errors.As(failure, &c) {
		t.Errorf("Root() = %v, want Cancelled", failure)
	}
	var v sample
	if _, failure := (Store{}).Read(ctx, "x.json", &v); !errors.As(failure, &c) {
		t.Errorf("Read(x.json) = %v, want Cancelled", failure)
	}
	if failure := (Store{}).Write(ctx, "x.json", v); !errors.As(failure, &c) {
		t.Errorf("Write(x.json) = %v, want Cancelled", failure)
	}
	if _, err := os.Stat(filepath.Join(root, ".metrics")); err == nil {
		t.Errorf("a cancelled write made .metrics")
	}
}
