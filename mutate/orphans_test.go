package mutate

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func writeIn(t *testing.T, root, name, text string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func there(t *testing.T, root, name string) bool {
	t.Helper()
	_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(name)))
	return err == nil
}

func snapshotsOf(orphans []Orphan) []string {
	var out []string
	for _, o := range orphans {
		out = append(out, o.Snapshot)
	}
	return out
}

// A snapshot is an orphan when nothing is at its source's path in the
// working tree: a file at the path again, whatever it is, keeps it, and
// so does anything under .metrics/mutate that is no snapshot.
func TestOrphansAreSnapshotsWhoseSourceIsGone(t *testing.T) {
	root := t.TempDir()
	writeIn(t, root, "src/kept.ts", "export {};\n")
	writeIn(t, root, "src/again.ts", "export const x = 1;\n")
	for _, name := range []string{"src/kept.ts", "src/gone.ts", "src/again.ts", "src/with space.ts", "lib/gone.ts"} {
		writeIn(t, root, ".metrics/mutate/"+name+".json", "{}\n")
	}
	writeIn(t, root, ".metrics/mutate/src/gone.ts.json.123.tmp", "{")
	writeIn(t, root, ".metrics/mutate/src/notes.txt", "")
	writeIn(t, root, ".metrics/mutate/.json", "{}\n")

	all, err := Orphans(root, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".metrics/mutate/lib/gone.ts.json", ".metrics/mutate/src/gone.ts.json", ".metrics/mutate/src/with space.ts.json"}
	if got := snapshotsOf(all); !slices.Equal(got, want) {
		t.Fatalf("orphans %v, want %v", got, want)
	}
	if all[1].Path != filepath.Join(root, "src", "gone.ts") {
		t.Errorf("orphan's path %s, want the source's, %s", all[1].Path, filepath.Join(root, "src", "gone.ts"))
	}

	src := filepath.Join(root, "src")
	under, err := Orphans(root, func(path string) bool { return filepath.Dir(path) == src })
	if err != nil {
		t.Fatal(err)
	}
	if err := RemoveOrphans(root, under); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"src/gone.ts", "src/with space.ts"} {
		if there(t, root, ".metrics/mutate/"+name+".json") {
			t.Errorf("the snapshot of %s is still there, want it removed", name)
		}
	}
	for _, name := range []string{".metrics/mutate/src/kept.ts.json", ".metrics/mutate/src/again.ts.json", ".metrics/mutate/lib/gone.ts.json",
		".metrics/mutate/src/gone.ts.json.123.tmp", ".metrics/mutate/src/notes.txt", "src/kept.ts"} {
		if !there(t, root, name) {
			t.Errorf("%s is gone, want it kept", name)
		}
	}
}

// Removing orphans empties no directory but those it emptied under
// .metrics/mutate, and never touches anything outside it, whatever an
// Orphan names.
func TestRemovingOrphansStaysInsideTheSnapshots(t *testing.T) {
	root := t.TempDir()
	writeIn(t, root, ".metrics/mutate/a/b/gone.ts.json", "{}\n")
	writeIn(t, root, "outside.json", "{}\n")
	writeIn(t, root, ".metrics/crap.json", "{}\n")
	orphans, err := Orphans(root, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	orphans = append(orphans, Orphan{Snapshot: "outside.json"}, Orphan{Snapshot: ".metrics/crap.json"},
		Orphan{Snapshot: ".metrics/mutate/../crap.json"}, Orphan{Snapshot: ".metrics/mutate"})
	if err := RemoveOrphans(root, orphans); err != nil {
		t.Fatal(err)
	}
	if there(t, root, ".metrics/mutate/a") {
		t.Errorf(".metrics/mutate/a is still there, want the directories the removal emptied removed")
	}
	for _, name := range []string{".metrics/mutate", "outside.json", ".metrics/crap.json"} {
		if !there(t, root, name) {
			t.Errorf("%s is gone, want everything outside the orphans kept", name)
		}
	}
	if got, err := Orphans(t.TempDir(), func(string) bool { return true }); got != nil || err != nil {
		t.Errorf("orphans of a project with no snapshots %v, %v, want none", got, err)
	}
}

// A snapshot's name maps back to its source's path as it is: off Windows a
// backslash is part of a name, not a separator, and a symbolic link under
// .metrics/mutate is no snapshot, so what it points to stays.
func TestOrphanNamesMapBackAsTheyAre(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a backslash is a separator on Windows, and symbolic links need privileges")
	}
	root := t.TempDir()
	writeIn(t, root, `src/a\b.ts`, "export {};\n")
	writeIn(t, root, `.metrics/mutate/src/a\b.ts.json`, "{}\n")
	writeIn(t, root, `.metrics/mutate/src\c.ts.json`, "{}\n")
	writeIn(t, root, "target.json", "{}\n")
	if err := os.Symlink(filepath.Join(root, "target.json"), filepath.Join(root, ".metrics", "mutate", "link.ts.json")); err != nil {
		t.Fatal(err)
	}
	orphans, err := Orphans(root, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if got, want := snapshotsOf(orphans), []string{`.metrics/mutate/src\c.ts.json`}; !slices.Equal(got, want) {
		t.Fatalf("orphans %v, want %v", got, want)
	}
	if err := RemoveOrphans(root, orphans); err != nil {
		t.Fatal(err)
	}
	if there(t, root, `.metrics/mutate/src\c.ts.json`) || !there(t, root, `.metrics/mutate/src/a\b.ts.json`) || !there(t, root, "target.json") {
		t.Errorf(`want only .metrics/mutate/src\c.ts.json removed`)
	}
}
