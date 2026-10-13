package porttest

import (
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage/port"
)

// FilesContract holds a Files to what every one, the fake and the disk's,
// must do: a regular file is the first of among that is the same file, by
// a path relative to the working directory, absolute or not clean, or no
// source when none of among is; a path that is not there, or a directory,
// is no regular file. open lays out files, by slash-separated paths
// relative to a directory, and returns the Files over them and that
// directory, absolute, the working directory; it may make that the test's,
// so no case runs in parallel. No case has a done context: the port's
// SameFile does not act on it. What only a disk can do (a symlinked
// directory, permissions) is the adapter's own tests'; the fake's links,
// the fake's.
func FilesContract(t *testing.T, open func(t *testing.T, files []string) (port.Files, string)) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, open func(t *testing.T, files []string) (port.Files, string))
	}{
		{"a regular file is the source among that names it, however its path is written", namesTheSource},
		{"the first of among that is the file is the one named", namesTheFirst},
		{"a regular file none of among names is no source", namesNoOtherFile},
		{"a path that is not there, or a directory, is no regular file", refusesWhatIsNoFile},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { c.run(t, open) })
	}
}

// sources are two files of one name in different directories, and one of
// another language.
var sources = []string{"src/a.go", "src/b/a.go", "lib/x.py"}

func namesTheSource(t *testing.T, open func(t *testing.T, files []string) (port.Files, string)) {
	files, dir := open(t, sources)
	abs := filepath.Join(dir, "src", "a.go")
	other := filepath.Join(dir, "src", "b", "a.go")
	for _, c := range []struct {
		path  string
		among []string
		want  string
	}{
		{"src/a.go", []string{other, abs}, abs},
		{filepath.Join("src", "a.go"), []string{abs}, abs},
		{abs, []string{other, filepath.Join("src", "a.go")}, filepath.Join("src", "a.go")},
		{filepath.Join(dir, "src", "b", "..", "a.go"), []string{other, abs}, abs},
		{"src/b/a.go", []string{abs, other}, other},
		{"lib/x.py", []string{filepath.Join(dir, "lib", "x.py")}, filepath.Join(dir, "lib", "x.py")},
	} {
		if same, regular := files.SameFile(t.Context(), c.path, c.among); same != c.want || !regular {
			t.Errorf("SameFile(%q, %q) = %q, %t; want %q, true", c.path, c.among, same, regular, c.want)
		}
	}
}

func namesTheFirst(t *testing.T, open func(t *testing.T, files []string) (port.Files, string)) {
	files, dir := open(t, sources)
	abs := filepath.Join(dir, "src", "a.go")
	among := []string{filepath.Join(dir, "absent", "a.go"), "src/a.go", abs}
	if same, regular := files.SameFile(t.Context(), abs, among); same != "src/a.go" || !regular {
		t.Errorf("SameFile(%q, %q) = %q, %t; want src/a.go, true: the first that is the file", abs, among, same, regular)
	}
}

func namesNoOtherFile(t *testing.T, open func(t *testing.T, files []string) (port.Files, string)) {
	files, dir := open(t, sources)
	for _, among := range [][]string{nil, {filepath.Join(dir, "src", "a.go")}, {filepath.Join(dir, "absent.go")}} {
		if same, regular := files.SameFile(t.Context(), "src/b/a.go", among); same != "" || !regular {
			t.Errorf("SameFile(src/b/a.go, %q) = %q, %t; want no source of a regular file", among, same, regular)
		}
	}
}

func refusesWhatIsNoFile(t *testing.T, open func(t *testing.T, files []string) (port.Files, string)) {
	files, dir := open(t, sources)
	for _, path := range []string{"absent.go", filepath.Join(dir, "src", "absent.go"), "src", "src/b", dir, ".", "src/a.go/x", "a.go"} {
		among := []string{filepath.Join(dir, "src", "a.go"), path}
		if same, regular := files.SameFile(t.Context(), path, among); same != "" || regular {
			t.Errorf("SameFile(%q, %q) = %q, %t; want no regular file", path, among, same, regular)
		}
	}
}
