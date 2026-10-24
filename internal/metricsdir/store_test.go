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
		path := filepath.Join(root, Name, filepath.FromSlash(name))
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

// A store given its root keeps it too, wherever the process runs: the
// working directory is another project, whose snapshots it never touches.
func TestAStoreOfAnotherRootKeepsTheContract(t *testing.T) {
	porttest.StoreContract(t, func(t *testing.T, snapshots map[string]string) (port.Store, string) {
		_, root := open(t, snapshots)
		elsewhere := inProject(t)
		t.Cleanup(func() {
			if _, err := os.Stat(filepath.Join(elsewhere, Name)); err == nil {
				t.Errorf("a store of %s touched the working directory's .metrics", root)
			}
		})
		return Store{RootDir: root}, root
	})
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
	if err := Write("library.json", v); err != nil {
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
	_, want := Read("bad.json", &v)
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
	want = Write("sub/x.json", v)
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
	if failure := (Store{}).Remove(ctx, "x.json"); !errors.As(failure, &c) {
		t.Errorf("Remove(x.json) = %v, want Cancelled", failure)
	}
	if _, failure := (Store{}).List(ctx, "mutate"); !errors.As(failure, &c) {
		t.Errorf("List(mutate) = %v, want Cancelled", failure)
	}
	if _, err := os.Stat(filepath.Join(root, ".metrics")); err == nil {
		t.Errorf("a cancelled write made .metrics")
	}
}

// A removal deletes each folder it left empty up to the one at the top of
// the snapshot's name, which stays, as .metrics does; a folder that holds
// anything else stays too.
func TestRemovingEmptiesNestedFoldersBelowTheTopOne(t *testing.T) {
	_, root := open(t, map[string]string{
		"mutate/src/deep/er/a.go.json": "{}",
		"mutate/src/kept/b.go.json":    "{}",
		"mutate/src/other/c.go.json":   "{}",
		"mutate/src/other/notes.txt":   "",
		"coverage/run/only.json":       "{}",
		"dry.json":                     "{}",
	})
	for _, name := range []string{"mutate/src/deep/er/a.go.json", "mutate/src/other/c.go.json", "coverage/run/only.json", "dry.json"} {
		if failure := (Store{}).Remove(t.Context(), name); failure != nil {
			t.Fatalf("Remove(%s) = %v", name, failure)
		}
	}
	at := func(name string) bool {
		_, err := os.Lstat(filepath.Join(root, Name, filepath.FromSlash(name)))
		return err == nil
	}
	for _, name := range []string{"mutate/src/deep/er", "mutate/src/deep", "coverage/run"} {
		if at(name) {
			t.Errorf("%s is still there, want the folders the removal emptied removed", name)
		}
	}
	for _, name := range []string{"mutate/src/kept/b.go.json", "mutate/src/other/notes.txt", "mutate/src", "coverage", "."} {
		if !at(name) {
			t.Errorf("%s is gone, want it kept", name)
		}
	}
	if failure := (Store{}).Remove(t.Context(), "mutate/src/kept/b.go.json"); failure != nil {
		t.Fatal(failure)
	}
	if at("mutate/src/kept") || !at("mutate") {
		t.Errorf("removing the last snapshot under mutate/src/kept left the folder, or removed mutate, the top one")
	}
}

// A link is no snapshot, whatever it points to and whatever its name, and
// a link to a folder is not followed.
func TestALinkIsNoSnapshot(t *testing.T) {
	_, root := open(t, map[string]string{"mutate/a.go.json": "{}", "elsewhere/b.go.json": "{}"})
	dir := filepath.Join(root, Name)
	if err := os.Symlink(filepath.Join(dir, "mutate", "a.go.json"), filepath.Join(dir, "mutate", "link.go.json")); err != nil {
		t.Skipf("no symbolic link can be made here: %v", err)
	}
	if err := os.Symlink(filepath.Join(dir, "elsewhere"), filepath.Join(dir, "mutate", "folder")); err != nil {
		t.Skipf("no symbolic link can be made here: %v", err)
	}
	if names, failure := (Store{}).List(t.Context(), "mutate"); failure != nil || !slices.Equal(names, []string{"mutate/a.go.json"}) {
		t.Errorf("List(mutate) = %q, %v; want only mutate/a.go.json", names, failure)
	}
}

// A snapshot that cannot be removed is Unremovable, and a folder that
// cannot be listed Unlistable, each with the library's own text whole. No
// system takes a name holding a NUL.
func TestStoreTranslatesTheLibrarysRemoveAndListFailures(t *testing.T) {
	_, root := open(t, map[string]string{"mutate/a.go.json": "{}"})
	dir := filepath.Join(root, Name)
	const bad = "mutate/a\x00.go.json"
	want := RemoveIn(dir, bad)
	failure := Store{}.Remove(t.Context(), bad)
	var unremovable *port.Unremovable
	if want == nil || !errors.As(failure, &unremovable) || unremovable.Name != bad || unremovable.Reason != want.Error() {
		t.Errorf("Remove(%q) = %T %v, want Unremovable %v", bad, failure, failure, want)
	}
	const folder = "mutate\x00"
	_, want = ListIn(dir, folder)
	names, failure := Store{}.List(t.Context(), folder)
	var unlistable *port.Unlistable
	if want == nil || names != nil || !errors.As(failure, &unlistable) || unlistable.Dir != folder || unlistable.Reason != want.Error() {
		t.Errorf("List(%q) = %q, %T %v, want Unlistable %v", folder, names, failure, failure, want)
	}
}
