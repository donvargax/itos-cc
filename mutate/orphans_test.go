package mutate

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
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

// A snapshot is judged by the file it records: one of a file outside the
// root, named by that file's absolute path, is no orphan while the file
// exists, and is one, at that path, once it is gone. A snapshot that names
// another file, or none, is judged by the path its name maps to.
func TestOrphansAreJudgedByTheFileTheSnapshotRecords(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	kept, gone, other := filepath.Join(outside, "kept.go"), filepath.Join(outside, "gone.go"), filepath.Join(outside, "other.go")
	writeIn(t, outside, "kept.go", "package x\n")
	writeIn(t, outside, "other.go", "package x\n")
	snapshotOf := func(path string) string {
		return filepath.ToSlash(filepath.Join(".metrics", SnapshotName(filepath.ToSlash(path))))
	}
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(filepath.FromSlash(snapshotOf(kept)))), 0o755); err != nil {
		t.Skipf("no snapshot can be named %s here: %v", snapshotOf(kept), err)
	}
	for _, path := range []string{kept, gone} {
		writeIn(t, root, snapshotOf(path), `{"version": 1, "file": "`+filepath.ToSlash(path)+`", "units": []}`+"\n")
	}
	// other.go exists, but its snapshot records a file that is no longer
	// there, as when it was copied from one: it is that file's, which
	// LoadSnapshotOf takes for none, and judged by its name.
	writeIn(t, root, snapshotOf(other), `{"version": 1, "file": "src/old.go", "units": []}`+"\n")

	var asked []string
	orphans, err := Orphans(root, func(path string) bool { asked = append(asked, path); return true })
	if err != nil {
		t.Fatal(err)
	}
	mapped := filepath.Join(root, strings.TrimSuffix(filepath.FromSlash(strings.TrimPrefix(snapshotOf(other), ".metrics/mutate/")), ".json"))
	want := []Orphan{{Path: gone, Snapshot: snapshotOf(gone)}, {Path: mapped, Snapshot: snapshotOf(other)}}
	slices.SortFunc(want, func(a, b Orphan) int { return strings.Compare(a.Snapshot, b.Snapshot) })
	if !slices.Equal(orphans, want) {
		t.Errorf("orphans %v, want %v: the snapshot of %s, whose file exists, kept", orphans, want, kept)
	}
	if !slices.Contains(asked, gone) {
		t.Errorf("covers was asked of %v, want of the gone file's own path, %s", asked, gone)
	}
}

// A snapshot's recorded file is the absolute path it names in this
// platform's form, a drive's letter included, only where the snapshot is
// that file's.
func TestARecordedFileIsAnAbsolutePathWhereItsSnapshotLives(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "src", "board.go")
	cases := []struct {
		name, file string
		want       string
	}{
		{SnapshotName(filepath.ToSlash(abs)), filepath.ToSlash(abs), abs},
		{SnapshotName("src/board.go"), "src/board.go", ""},
		{SnapshotName("src/board.go"), filepath.ToSlash(abs), ""},
		{SnapshotName(filepath.ToSlash(abs)), "", ""},
	}
	if runtime.GOOS == "windows" {
		cases = append(cases,
			struct{ name, file, want string }{SnapshotName("D:/work/board.go"), "D:/work/board.go", `D:\work\board.go`},
			struct{ name, file, want string }{SnapshotName("//host/share/board.go"), "//host/share/board.go", `\\host\share\board.go`},
			struct{ name, file, want string }{SnapshotName("/work/board.go"), "/work/board.go", ""})
	}
	for _, c := range cases {
		got, ok := recordedPath(c.name, c.file)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("recordedPath(%q, %q) = %q, %v, want %q", c.name, c.file, got, ok, c.want)
		}
	}
}
