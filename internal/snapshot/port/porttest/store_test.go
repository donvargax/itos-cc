package porttest

import (
	"errors"
	"maps"
	"math"
	"testing"

	"github.com/donvargax/itos-cc/internal/snapshot/port"
)

// The fake keeps the contract every Store keeps, the disk's included. Its
// root is a name only: the fake never touches it.
func TestStoreKeepsTheContract(t *testing.T) {
	StoreContract(t, func(t *testing.T, snapshots map[string]string) (port.Store, string) {
		dir := t.TempDir()
		return &Store{RootDir: dir, Snapshots: maps.Clone(snapshots)}, dir
	})
}

// The fake writes the metrics library's bytes: indented by two spaces,
// nothing escaped for HTML, ending in a newline, which no Read can show.
func TestStoreWritesTheLibrarysBytes(t *testing.T) {
	store := &Store{}
	if failure := store.Write(t.Context(), "dry.json", map[string]any{"files": []string{"a<b>.go"}, "version": 1}); failure != nil {
		t.Fatal(failure)
	}
	if want := "{\n  \"files\": [\n    \"a<b>.go\"\n  ],\n  \"version\": 1\n}\n"; store.Snapshots["dry.json"] != want {
		t.Errorf("Write(dry.json) holds %q, want %q", store.Snapshots["dry.json"], want)
	}
}

// A store with no snapshots yet takes one, a value that does not encode is
// Unwritable, and a configured failure is what a read, a write or the
// root returns.
func TestStoreFailsAsConfigured(t *testing.T) {
	store := &Store{}
	if failure := store.Write(t.Context(), "dry.json", []int{}); failure != nil || store.Snapshots["dry.json"] != "[]\n" {
		t.Errorf("Write(dry.json) = %v, holding %q", failure, store.Snapshots["dry.json"])
	}
	var unwritable *port.Unwritable
	if failure := store.Write(t.Context(), "nan.json", math.NaN()); !errors.As(failure, &unwritable) || unwritable.Name != "nan.json" {
		t.Errorf("Write(nan.json) = %T %v, want Unwritable", failure, failure)
	}
	full := &port.Unwritable{Name: "full.json", Reason: "no space left on device"}
	store.Failures = map[string]port.Failure{"full.json": full}
	if failure := store.Write(t.Context(), "full.json", 1); failure != full {
		t.Errorf("Write(full.json) = %v, want %v", failure, full)
	}
	var v int
	if found, failure := store.Read(t.Context(), "full.json", &v); found || failure != full {
		t.Errorf("Read(full.json) = %t, %v, want %v", found, failure, full)
	}
	stopped := &port.Cancelled{}
	store.RootFailure = stopped
	if root, failure := store.Root(t.Context()); root != "" || failure != stopped {
		t.Errorf("Root() = %q, %v, want %v", root, failure, stopped)
	}
}
