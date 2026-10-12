package porttest

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/donvargax/itos-cc/internal/snapshot/port"
)

func cancelled(t *testing.T) context.Context {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return ctx
}

// What the fake writes it reads back, a snapshot it does not hold is not
// there, and one holding what does not decode is Unreadable, as a real
// store's are.
func TestStoreReadsWhatItWrote(t *testing.T) {
	store := &Store{RootDir: "/project", Snapshots: map[string]string{"bad.json": "{"}}
	if root, failure := store.Root(t.Context()); root != "/project" || failure != nil {
		t.Errorf("Root() = %q, %v", root, failure)
	}
	if failure := store.Write(t.Context(), "dry.json", map[string]int{"version": 1}); failure != nil {
		t.Fatalf("Write(dry.json) = %v", failure)
	}
	var got map[string]int
	if found, failure := store.Read(t.Context(), "dry.json", &got); !found || failure != nil || got["version"] != 1 {
		t.Errorf("Read(dry.json) = %t, %v, %v", found, failure, got)
	}
	if found, failure := store.Read(t.Context(), "scrap.json", &got); found || failure != nil {
		t.Errorf("Read(scrap.json) = %t, %v, want not there", found, failure)
	}
	found, failure := store.Read(t.Context(), "bad.json", &got)
	var unreadable *port.Unreadable
	if !found || !errors.As(failure, &unreadable) || unreadable.Name != "bad.json" || unreadable.Reason != "unexpected end of JSON input" {
		t.Errorf("Read(bad.json) = %t, %T %v, want Unreadable", found, failure, failure)
	}
}

// A store with no snapshots yet takes one, a value that does not encode is
// Unwritable, and a configured failure is what a read, a write or the
// root returns.
func TestStoreFailsAsConfigured(t *testing.T) {
	store := &Store{}
	if failure := store.Write(t.Context(), "dry.json", []int{}); failure != nil || store.Snapshots["dry.json"] != "[]" {
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

// Once the context is done, the fake does nothing and says Cancelled.
func TestStoreStopsOnceCancelled(t *testing.T) {
	store := &Store{Snapshots: map[string]string{"dry.json": "1"}}
	ctx := cancelled(t)
	var v int
	var c *port.Cancelled
	if _, failure := store.Root(ctx); !errors.As(failure, &c) {
		t.Errorf("Root() = %v, want Cancelled", failure)
	}
	if _, failure := store.Read(ctx, "dry.json", &v); !errors.As(failure, &c) || v != 0 {
		t.Errorf("Read(dry.json) = %v, %d, want Cancelled", failure, v)
	}
	if failure := store.Write(ctx, "dry.json", 2); !errors.As(failure, &c) || store.Snapshots["dry.json"] != "1" {
		t.Errorf("Write(dry.json) = %v, holding %q, want Cancelled", failure, store.Snapshots["dry.json"])
	}
}
