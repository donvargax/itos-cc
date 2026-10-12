package porttest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/internal/snapshot/port"
)

// StoreContract holds a Store to what every one, the fake and the disk's,
// must do: the root, what it reads and writes and the bytes it writes, the
// failures of snapshots both can hold, and a done context. open lays out
// snapshots, raw text by name, in a project and returns the Store over it
// and the project's root, absolute; it may make that the test's working
// directory, so no case runs in parallel. What only a disk can do (the git
// top level as the root, the atomic rename, a file where a folder should
// be) is the adapter's own tests'; a failure the fake is configured with,
// the fake's.
func StoreContract(t *testing.T, open func(t *testing.T, snapshots map[string]string) (port.Store, string)) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, open func(t *testing.T, snapshots map[string]string) (port.Store, string))
	}{
		{"the root is the project's", rootsAtTheProject},
		{"a written snapshot reads back, a nested one too, as the metrics library writes it", readsWhatItWrote},
		{"a snapshot laid out reads as its text decodes", readsWhatIsThere},
		{"an absent snapshot is not there and leaves the value alone", findsNoAbsentSnapshot},
		{"a snapshot that does not decode is unreadable", refusesMalformedText},
		{"a value that does not encode is unwritable and writes nothing", refusesWhatDoesNotEncode},
		{"a write replaces the snapshot whole", replacesWhole},
		{"a done context is cancelled and writes nothing", aDoneContextIsCancelled},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { c.run(t, open) })
	}
}

// record is a snapshot's value, with text JSON would escape for HTML.
type record struct {
	Version int      `json:"version"`
	Files   []string `json:"files"`
}

var written = record{Version: 1, Files: []string{"src/a<b>.go", "lib/c&d.ts"}}

func rootsAtTheProject(t *testing.T, open func(t *testing.T, snapshots map[string]string) (port.Store, string)) {
	store, dir := open(t, nil)
	if root, failure := store.Root(t.Context()); failure != nil || root != dir {
		t.Errorf("Root() = %q, %v; want %q", root, failure, dir)
	}
}

func readsWhatItWrote(t *testing.T, open func(t *testing.T, snapshots map[string]string) (port.Store, string)) {
	store, _ := open(t, nil)
	// The metrics library's bytes: indented by two spaces, nothing escaped
	// for HTML. A Read into a json.RawMessage gives them back but for the
	// newline that ends them.
	var want bytes.Buffer
	enc := json.NewEncoder(&want)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(written); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dry.json", "mutate/a.go.json"} {
		if failure := store.Write(t.Context(), name, written); failure != nil {
			t.Fatalf("Write(%s) = %v", name, failure)
		}
		var got record
		if found, failure := store.Read(t.Context(), name, &got); !found || failure != nil || !equal(got, written) {
			t.Errorf("Read(%s) = %t, %v, %+v; want %+v", name, found, failure, got, written)
		}
		var raw json.RawMessage
		if _, failure := store.Read(t.Context(), name, &raw); failure != nil || !bytes.Equal(raw, bytes.TrimSuffix(want.Bytes(), []byte("\n"))) {
			t.Errorf("Read(%s) holds\n%s\nwant\n%s", name, raw, want.Bytes())
		}
	}
}

func readsWhatIsThere(t *testing.T, open func(t *testing.T, snapshots map[string]string) (port.Store, string)) {
	store, _ := open(t, map[string]string{"scrap.json": `{"version": 1, "files": ["a.go"]}`, "mutate/b.go.json": `{"version": 2}`})
	var got record
	if found, failure := store.Read(t.Context(), "scrap.json", &got); !found || failure != nil || !equal(got, record{Version: 1, Files: []string{"a.go"}}) {
		t.Errorf("Read(scrap.json) = %t, %v, %+v", found, failure, got)
	}
	var nested record
	if found, failure := store.Read(t.Context(), "mutate/b.go.json", &nested); !found || failure != nil || nested.Version != 2 {
		t.Errorf("Read(mutate/b.go.json) = %t, %v, %+v", found, failure, nested)
	}
}

func findsNoAbsentSnapshot(t *testing.T, open func(t *testing.T, snapshots map[string]string) (port.Store, string)) {
	store, _ := open(t, map[string]string{"dry.json": `{"version": 1}`})
	for _, name := range []string{"scrap.json", "mutate/absent.go.json"} {
		got := record{Version: 7}
		if found, failure := store.Read(t.Context(), name, &got); found || failure != nil || got.Version != 7 {
			t.Errorf("Read(%s) = %t, %v, %+v; want not there and the value untouched", name, found, failure, got)
		}
	}
}

func refusesMalformedText(t *testing.T, open func(t *testing.T, snapshots map[string]string) (port.Store, string)) {
	store, _ := open(t, map[string]string{"bad.json": "{", "mutate/bad.go.json": `{"version": "one"}`})
	for _, name := range []string{"bad.json", "mutate/bad.go.json"} {
		var got record
		found, failure := store.Read(t.Context(), name, &got)
		var unreadable *port.Unreadable
		if !found || !errors.As(failure, &unreadable) || unreadable.Name != name || unreadable.Reason == "" {
			t.Errorf("Read(%s) = %t, %T %v; want there and Unreadable naming it", name, found, failure, failure)
		}
	}
}

func refusesWhatDoesNotEncode(t *testing.T, open func(t *testing.T, snapshots map[string]string) (port.Store, string)) {
	store, _ := open(t, nil)
	failure := store.Write(t.Context(), "nan.json", math.NaN())
	var unwritable *port.Unwritable
	if !errors.As(failure, &unwritable) || unwritable.Name != "nan.json" || unwritable.Reason == "" {
		t.Errorf("Write(nan.json) = %T %v; want Unwritable naming it", failure, failure)
	}
	var got float64
	if found, failure := store.Read(t.Context(), "nan.json", &got); found || failure != nil {
		t.Errorf("Read(nan.json) = %t, %v; want nothing written", found, failure)
	}
}

func replacesWhole(t *testing.T, open func(t *testing.T, snapshots map[string]string) (port.Store, string)) {
	store, _ := open(t, map[string]string{"dry.json": `{"version": 1, "files": ["a.go", "b.go"], "threshold": 0.8}`})
	if failure := store.Write(t.Context(), "dry.json", record{Version: 2}); failure != nil {
		t.Fatalf("Write(dry.json) = %v", failure)
	}
	var got map[string]any
	if found, failure := store.Read(t.Context(), "dry.json", &got); !found || failure != nil || len(got) != 2 || got["version"] != 2.0 || got["files"] != nil {
		t.Errorf("Read(dry.json) = %t, %v, %v; want only the version 2 written", found, failure, got)
	}
}

func aDoneContextIsCancelled(t *testing.T, open func(t *testing.T, snapshots map[string]string) (port.Store, string)) {
	store, _ := open(t, map[string]string{"dry.json": `{"version": 1}`})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var cancelled *port.Cancelled
	if root, failure := store.Root(ctx); !errors.As(failure, &cancelled) || root != "" {
		t.Errorf("Root() = %q, %T; want Cancelled and no root", root, failure)
	}
	got := record{Version: 7}
	if found, failure := store.Read(ctx, "dry.json", &got); !errors.As(failure, &cancelled) || found || got.Version != 7 {
		t.Errorf("Read(dry.json) = %t, %T, %+v; want Cancelled and the value untouched", found, failure, got)
	}
	for _, name := range []string{"dry.json", "scrap.json"} {
		if failure := store.Write(ctx, name, record{Version: 2}); !errors.As(failure, &cancelled) {
			t.Errorf("Write(%s) = %T; want Cancelled", name, failure)
		}
	}
	var after record
	if found, failure := store.Read(t.Context(), "dry.json", &after); !found || failure != nil || after.Version != 1 {
		t.Errorf("Read(dry.json) after = %t, %v, %+v; want the snapshot as it was", found, failure, after)
	}
	if found, failure := store.Read(t.Context(), "scrap.json", &after); found || failure != nil {
		t.Errorf("Read(scrap.json) after = %t, %v; want nothing written", found, failure)
	}
}

// equal says whether two records hold the same.
func equal(a, b record) bool {
	return a.Version == b.Version && slices.Equal(a.Files, b.Files)
}
