package porttest

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/counting/port"
)

// FilesContract holds a Files to what every one, the fake and the disk's,
// must do: a present file's bytes, by a path relative to the working
// directory or absolute, the caller's own copy, Missing for an absent path
// and Unreadable for a directory. open lays out files, by slash-separated
// paths relative to a directory with their bytes, and returns the Files
// over them and that directory, absolute, the working directory; it may
// make that the test's, so no case runs in parallel. No case has a done
// context: the port's Read acts on it only where a read waits, and neither
// a fake's nor a file's read does. What only a disk can do (permissions, a
// path through a file, whose failure differs by system) is the adapter's
// own tests'; standard input is internal/stdin's, and a failure the fake
// is configured with, the fake's.
func FilesContract(t *testing.T, open func(t *testing.T, files map[string][]byte) (port.Files, string)) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, open func(t *testing.T, files map[string][]byte) (port.Files, string))
	}{
		{"a present file's bytes, by a relative or an absolute path", readsAPresentFile},
		{"what read gives is the caller's own copy", givesItsOwnCopy},
		{"an absent path is missing", refusesAnAbsentPath},
		{"a directory is unreadable", refusesADirectory},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { c.run(t, open) })
	}
}

// inputs are files a count reads: text, bytes that are no text, and none.
var inputs = map[string][]byte{
	"notes.txt":      []byte("one two\nthree\n"),
	"data/blob.bin":  {0, 1, 0xff, '\r', '\n'},
	"data/empty.txt": {},
}

func readsAPresentFile(t *testing.T, open func(t *testing.T, files map[string][]byte) (port.Files, string)) {
	files, dir := open(t, inputs)
	for path, name := range map[string]string{
		"notes.txt": "notes.txt", filepath.Join(dir, "notes.txt"): "notes.txt",
		"data/blob.bin": "data/blob.bin", filepath.Join("data", "blob.bin"): "data/blob.bin",
		"data/../notes.txt": "notes.txt", filepath.Join(dir, "data", "empty.txt"): "data/empty.txt",
	} {
		if got, failure := files.Read(t.Context(), path); failure != nil || !bytes.Equal(got, inputs[name]) {
			t.Errorf("Read(%q) = %q, %v; want %q", path, got, failure, inputs[name])
		}
	}
}

func givesItsOwnCopy(t *testing.T, open func(t *testing.T, files map[string][]byte) (port.Files, string)) {
	files, _ := open(t, inputs)
	first, failure := files.Read(t.Context(), "notes.txt")
	if failure != nil {
		t.Fatal(failure)
	}
	first[0] = 'O'
	if again, failure := files.Read(t.Context(), "notes.txt"); failure != nil || !bytes.Equal(again, inputs["notes.txt"]) {
		t.Errorf("Read(notes.txt) = %q, %v after the caller wrote to what it gave; want %q", again, failure, inputs["notes.txt"])
	}
}

func refusesAnAbsentPath(t *testing.T, open func(t *testing.T, files map[string][]byte) (port.Files, string)) {
	files, dir := open(t, inputs)
	for _, path := range []string{"absent.txt", filepath.Join(dir, "data", "absent.txt"), "absent/notes.txt", "note"} {
		got, failure := files.Read(t.Context(), path)
		var missing *port.Missing
		if !errors.As(failure, &missing) || missing.Path != path || got != nil {
			t.Errorf("Read(%q) = %q, %T %v; want Missing naming it", path, got, failure, failure)
		}
	}
}

func refusesADirectory(t *testing.T, open func(t *testing.T, files map[string][]byte) (port.Files, string)) {
	files, dir := open(t, inputs)
	for _, path := range []string{"data", filepath.Join(dir, "data"), ".", dir} {
		got, failure := files.Read(t.Context(), path)
		var unreadable *port.Unreadable
		if !errors.As(failure, &unreadable) || unreadable.Path != path || got != nil {
			t.Errorf("Read(%q) = %q, %T %v; want Unreadable naming it", path, got, failure, failure)
		}
	}
}
