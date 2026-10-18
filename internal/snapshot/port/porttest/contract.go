package porttest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/snapshot/port"
)

// StoreContract holds a Store to what every one, the fake and the disk's,
// must do: the root, what it reads and writes and the bytes it writes, what
// it removes and lists, the failures of snapshots both can hold, and a done
// context. open lays out
// snapshots, raw text by name, in a project and returns the Store over it
// and the project's root, absolute; it may make that the test's working
// directory, so no case runs in parallel. What only a disk can do (the git
// top level as the root, the atomic rename, a file where a folder should
// be, the folders a removal empties, a link) is the adapter's own tests'; a
// failure the fake is configured with, the fake's.
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
		{"a removed snapshot is gone and the others stay", removesTheSnapshot},
		{"removing an absent snapshot is no failure", removesNothingAbsent},
		{"removing the last snapshot of nested folders leaves the others, and they take one again", removesFromNestedFolders},
		{"a folder lists its snapshots at any depth, named from .metrics, sorted", listsTheFolder},
		{"what is no json file is not listed, and a folder named like one lists what it holds", listsOnlyJSONFiles},
		{"an absent folder lists none", listsNoAbsentFolder},
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
	store, _ := open(t, map[string]string{
		"bad.json":           "{",
		"mutate/bad.go.json": `{"version": "one"}`,
		"files.json":         `{"version": 1, "files": ["a.go", 7]}`,
		"list.json":          `[1]`,
	})
	// A value of the wrong kind is said in the snapshot's own terms, its
	// key and the JSON kinds, never the Go types the reader decodes into
	// (bug-3); JSON that does not parse keeps the parser's text.
	reasons := map[string]string{
		"bad.json":           "unexpected end of JSON input",
		"mutate/bad.go.json": "version should be a number, not a string",
		"files.json":         "files[1] should be a string, not a number",
		"list.json":          "the snapshot should be an object, not an array",
	}
	for _, name := range []string{"bad.json", "mutate/bad.go.json", "files.json", "list.json"} {
		var got record
		found, failure := store.Read(t.Context(), name, &got)
		var unreadable *port.Unreadable
		if !found || !errors.As(failure, &unreadable) || unreadable.Name != name || unreadable.Reason == "" {
			t.Errorf("Read(%s) = %t, %T %v; want there and Unreadable naming it", name, found, failure, failure)
			continue
		}
		if unreadable.Reason != reasons[name] {
			t.Errorf("Read(%s) Reason = %q; want %q", name, unreadable.Reason, reasons[name])
		}
		for _, internal := range []string{"Go ", "of type", "record", "porttest", "[]string"} {
			if strings.Contains(unreadable.Reason, internal) {
				t.Errorf("Read(%s) Reason = %q names %q, a Go type or package", name, unreadable.Reason, internal)
			}
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
	if failure := store.Remove(ctx, "dry.json"); !errors.As(failure, &cancelled) {
		t.Errorf("Remove(dry.json) = %T; want Cancelled", failure)
	}
	if names, failure := store.List(ctx, "."); !errors.As(failure, &cancelled) || names != nil {
		t.Errorf("List(.) = %q, %T; want Cancelled and no names", names, failure)
	}
	var after record
	if found, failure := store.Read(t.Context(), "dry.json", &after); !found || failure != nil || after.Version != 1 {
		t.Errorf("Read(dry.json) after = %t, %v, %+v; want the snapshot as it was", found, failure, after)
	}
	if found, failure := store.Read(t.Context(), "scrap.json", &after); found || failure != nil {
		t.Errorf("Read(scrap.json) after = %t, %v; want nothing written", found, failure)
	}
}

func removesTheSnapshot(t *testing.T, open func(t *testing.T, snapshots map[string]string) (port.Store, string)) {
	store, _ := open(t, map[string]string{"dry.json": `{"version": 1}`, "mutate/a.go.json": `{"version": 2}`, "mutate/b.go.json": `{"version": 3}`})
	for _, name := range []string{"mutate/a.go.json", "dry.json"} {
		if failure := store.Remove(t.Context(), name); failure != nil {
			t.Fatalf("Remove(%s) = %v", name, failure)
		}
		var got record
		if found, failure := store.Read(t.Context(), name, &got); found || failure != nil {
			t.Errorf("Read(%s) after its removal = %t, %v; want not there", name, found, failure)
		}
	}
	var kept record
	if found, failure := store.Read(t.Context(), "mutate/b.go.json", &kept); !found || failure != nil || kept.Version != 3 {
		t.Errorf("Read(mutate/b.go.json) = %t, %v, %+v; want it kept", found, failure, kept)
	}
	if names, failure := store.List(t.Context(), "mutate"); failure != nil || !slices.Equal(names, []string{"mutate/b.go.json"}) {
		t.Errorf("List(mutate) = %q, %v; want only mutate/b.go.json", names, failure)
	}
}

func removesNothingAbsent(t *testing.T, open func(t *testing.T, snapshots map[string]string) (port.Store, string)) {
	store, _ := open(t, map[string]string{"mutate/a.go.json": `{"version": 1}`})
	for _, name := range []string{"scrap.json", "mutate/absent.go.json", "absent/folder/x.json"} {
		if failure := store.Remove(t.Context(), name); failure != nil {
			t.Errorf("Remove(%s) = %T %v; want no failure", name, failure, failure)
		}
	}
	if names, failure := store.List(t.Context(), "."); failure != nil || !slices.Equal(names, []string{"mutate/a.go.json"}) {
		t.Errorf("List(.) = %q, %v; want mutate/a.go.json kept", names, failure)
	}
}

func removesFromNestedFolders(t *testing.T, open func(t *testing.T, snapshots map[string]string) (port.Store, string)) {
	store, _ := open(t, map[string]string{
		"mutate/src/deep/er/a.go.json": `{"version": 1}`,
		"mutate/src/b.go.json":         `{"version": 2}`,
		"mutate/lib/c.go.json":         `{"version": 3}`,
	})
	for _, name := range []string{"mutate/src/deep/er/a.go.json", "mutate/lib/c.go.json"} {
		if failure := store.Remove(t.Context(), name); failure != nil {
			t.Fatalf("Remove(%s) = %v", name, failure)
		}
	}
	if names, failure := store.List(t.Context(), "mutate"); failure != nil || !slices.Equal(names, []string{"mutate/src/b.go.json"}) {
		t.Errorf("List(mutate) = %q, %v; want only mutate/src/b.go.json", names, failure)
	}
	if names, failure := store.List(t.Context(), "mutate/src/deep"); failure != nil || len(names) != 0 {
		t.Errorf("List(mutate/src/deep) = %q, %v; want none", names, failure)
	}
	if failure := store.Remove(t.Context(), "mutate/src/b.go.json"); failure != nil {
		t.Fatalf("Remove(mutate/src/b.go.json) = %v", failure)
	}
	if names, failure := store.List(t.Context(), "mutate"); failure != nil || len(names) != 0 {
		t.Errorf("List(mutate) = %q, %v; want none once the last is removed", names, failure)
	}
	for _, name := range []string{"mutate/src/deep/er/a.go.json", "mutate/lib/c.go.json", "dry.json"} {
		if failure := store.Write(t.Context(), name, written); failure != nil {
			t.Errorf("Write(%s) after the removals = %v", name, failure)
		}
		var got record
		if found, failure := store.Read(t.Context(), name, &got); !found || failure != nil || !equal(got, written) {
			t.Errorf("Read(%s) = %t, %v, %+v; want what was written", name, found, failure, got)
		}
	}
}

func listsTheFolder(t *testing.T, open func(t *testing.T, snapshots map[string]string) (port.Store, string)) {
	store, _ := open(t, map[string]string{
		"dry.json":               `{}`,
		"mutate.json":            `{}`,
		"mutated/x.go.json":      `{}`,
		"mutate/b.go.json":       `{}`,
		"mutate/a/c.ts.json":     `{}`,
		"mutate/a/d/e.py.json":   `{}`,
		"mutate/with space.json": `{}`,
		"coverage/run/lcov.json": `{}`,
	})
	if names, failure := store.List(t.Context(), "mutate"); failure != nil ||
		!slices.Equal(names, []string{"mutate/a/c.ts.json", "mutate/a/d/e.py.json", "mutate/b.go.json", "mutate/with space.json"}) {
		t.Errorf("List(mutate) = %q, %v; want the four under mutate, sorted", names, failure)
	}
	if names, failure := store.List(t.Context(), "mutate/a"); failure != nil || !slices.Equal(names, []string{"mutate/a/c.ts.json", "mutate/a/d/e.py.json"}) {
		t.Errorf("List(mutate/a) = %q, %v; want the two under mutate/a", names, failure)
	}
	var got record
	for _, name := range []string{"mutate/a/d/e.py.json", "mutate/with space.json"} {
		if found, failure := store.Read(t.Context(), name, &got); !found || failure != nil {
			t.Errorf("Read(%s) of a listed name = %t, %v; want it there", name, found, failure)
		}
	}
}

func listsOnlyJSONFiles(t *testing.T, open func(t *testing.T, snapshots map[string]string) (port.Store, string)) {
	store, _ := open(t, map[string]string{
		"mutate/a.go.json":             `{}`,
		"mutate/a.go.json.123.tmp":     `{`,
		"mutate/notes.txt":             ``,
		"mutate/b.go.JSON.bak":         `{}`,
		"mutate/folder.json/c.go.json": `{}`,
		"mutate/folder.json/readme.md": ``,
	})
	if names, failure := store.List(t.Context(), "mutate"); failure != nil || !slices.Equal(names, []string{"mutate/a.go.json", "mutate/folder.json/c.go.json"}) {
		t.Errorf("List(mutate) = %q, %v; want only the json files", names, failure)
	}
}

func listsNoAbsentFolder(t *testing.T, open func(t *testing.T, snapshots map[string]string) (port.Store, string)) {
	for _, snapshots := range []map[string]string{nil, {"dry.json": `{}`}} {
		store, _ := open(t, snapshots)
		for _, dir := range []string{"mutate", "mutate/src"} {
			if names, failure := store.List(t.Context(), dir); failure != nil || len(names) != 0 {
				t.Errorf("List(%s) of %q = %q, %v; want none", dir, slices.Sorted(maps.Keys(snapshots)), names, failure)
			}
		}
	}
}

// equal says whether two records hold the same.
func equal(a, b record) bool {
	return a.Version == b.Version && slices.Equal(a.Files, b.Files)
}
