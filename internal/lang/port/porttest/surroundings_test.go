package porttest

import (
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/lang/port"
)

// The fake keeps the contract every Surroundings keeps, the disk's
// included. Its directory is a name only: the fake never touches it, and a
// temporary directory's is absolute on every system.
func TestSurroundingsKeepTheContract(t *testing.T) {
	SurroundingsContract(t, func(t *testing.T, files map[string]string) (port.Surroundings, string) {
		dir := t.TempDir()
		return Surroundings{Files: files, Dir: dir}, dir
	})
}

// The fake answers as a disk does: a file and the directories above it are
// there, a file's bytes are what it holds, and a relative path is made
// absolute from the working directory.
func TestSurroundingsAnswerAsADiskDoes(t *testing.T) {
	s := Surroundings{Files: map[string]string{"/work/mod/go.mod": "module m\n"}, Dir: "/work"}
	for p, want := range map[string]bool{
		"/work/mod/go.mod": true, "/work/mod": true, "/work": true, "/": true,
		"/work/mo": false, "/work/mod/go.mod/x": false, "/other": false,
		// A relative path is from Dir.
		"mod/go.mod": true, "mod": true, ".": true, "go.mod": false,
	} {
		if got := s.Exists(t.Context(), filepath.FromSlash(p)); got != want {
			t.Errorf("Exists(%q) = %t, want %t", p, got, want)
		}
	}
	if text, ok := s.Read(t.Context(), filepath.FromSlash("/work/mod/go.mod")); !ok || string(text) != "module m\n" {
		t.Errorf("Read(go.mod) = %q, %t", text, ok)
	}
	if text, ok := s.Read(t.Context(), filepath.FromSlash("/work/mod")); ok {
		t.Errorf("Read(a directory) = %q, want no answer", text)
	}
	if wd, ok := s.WorkingDir(t.Context()); !ok || wd != filepath.FromSlash("/work") {
		t.Errorf("WorkingDir() = %q, %t", wd, ok)
	}
	for p, want := range map[string]string{
		"x.ts":          "/work/x.ts",
		"../other/y.ts": "/other/y.ts",
		".":             "/work",
		"/abs/./z.ts":   "/abs/z.ts",
	} {
		if got, ok := s.Abs(t.Context(), filepath.FromSlash(p)); !ok || got != filepath.FromSlash(want) {
			t.Errorf("Abs(%q) = %q, %t; want %q", p, got, ok, want)
		}
	}
}

// With no working directory, it cannot be told and no relative path can be
// made absolute, as when os.Getwd fails; an absolute one still can.
func TestSurroundingsWithNoWorkingDirectory(t *testing.T) {
	s := Surroundings{}
	if wd, ok := s.WorkingDir(t.Context()); ok {
		t.Errorf("WorkingDir() = %q, want no answer", wd)
	}
	if got, ok := s.Abs(t.Context(), "x.ts"); ok {
		t.Errorf("Abs(x.ts) = %q, want no answer", got)
	}
	if got, ok := s.Abs(t.Context(), filepath.FromSlash("/a/x.ts")); !ok || got != filepath.FromSlash("/a/x.ts") {
		t.Errorf("Abs(/a/x.ts) = %q, %t", got, ok)
	}
}
