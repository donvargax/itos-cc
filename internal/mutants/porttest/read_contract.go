package porttest

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/donvargax/itos-cc/internal/mutants"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// MutationCatalogContract checks actual store-relative names and permissive file
// metadata. The factory lays out independent records, not full snapshot models.
func MutationCatalogContract(t *testing.T, open func(*testing.T, map[string]string) (mutants.MutationCatalog, string)) {
	t.Helper()
	files := map[string]string{"mutate/a.go.json": "a.go", "mutate/nested/b.go.json": "nested/b.go", "mutate/.json": "", "mutate/not-json.tmp": "ignored", "other/out.json": "ignored"}
	catalog, root := open(t, files)
	got, failure := catalog.Root(t.Context())
	if failure != nil || got != root {
		t.Fatalf("actual root: %q %v", got, failure)
	}
	names, failure := catalog.List(t.Context())
	want := []string{"mutate/.json", "mutate/a.go.json", "mutate/nested/b.go.json"}
	if failure != nil || !reflect.DeepEqual(names, want) {
		t.Fatalf("regular JSON inventory: %v %v", names, failure)
	}
	for _, name := range want {
		file, failure := catalog.RecordedFile(t.Context(), name)
		if failure != nil || file != files[name] {
			t.Fatalf("metadata %q: %q %v", name, file, failure)
		}
	}
	if file, failure := catalog.RecordedFile(t.Context(), "mutate/absent.go.json"); failure != nil || file != "" {
		t.Fatalf("absent metadata: %q %v", file, failure)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var cancelled *snapport.Cancelled
	if _, failure := catalog.List(ctx); !errors.As(failure, &cancelled) {
		t.Fatalf("cancelled inventory: %v", failure)
	}
	if _, failure := catalog.RecordedFile(ctx, names[0]); !errors.As(failure, &cancelled) {
		t.Fatalf("cancelled metadata: %v", failure)
	}
	if _, failure := catalog.Root(ctx); !errors.As(failure, &cancelled) {
		t.Fatalf("cancelled root: %v", failure)
	}
}

// SourcePresenceContract keeps conservative presence separate from successful
// target evidence, and forwards the original caller cancellation cause.
func SourcePresenceContract(t *testing.T, open func(*testing.T) (mutants.SourcePresence, string)) {
	t.Helper()
	presence, root := open(t)
	for _, c := range []struct {
		name string
		want bool
	}{{"file.go", true}, {"directory", true}, {"missing", false}} {
		path := filepath.Join(root, c.name)
		got, failure := presence.Present(t.Context(), path)
		if failure != nil || got != c.want {
			t.Fatalf("presence %s: %v %v", c.name, got, failure)
		}
		got, failure = presence.RenameTarget(t.Context(), path)
		if failure != nil || got != c.want {
			t.Fatalf("target %s: %v %v", c.name, got, failure)
		}
	}
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("presence caller stopped")
	cancel(cause)
	if _, failure := presence.Present(ctx, filepath.Join(root, "file.go")); !errors.Is(failure, cause) {
		t.Fatalf("presence cause: %v", failure)
	}
	if _, failure := presence.RenameTarget(ctx, filepath.Join(root, "file.go")); !errors.Is(failure, cause) {
		t.Fatalf("target cause: %v", failure)
	}
}
