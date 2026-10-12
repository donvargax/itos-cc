package project

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Surroundings answer as the disk does: a stat that succeeds is there, a
// file's bytes are what os.ReadFile gives, and the working directory and an
// absolute path are os.Getwd's and filepath.Abs's.
func TestSurroundingsAnswerAsTheDiskDoes(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "mod", "go.mod"), "module m\n")
	t.Chdir(dir)
	s := Surroundings{}
	for path, want := range map[string]bool{"mod": true, filepath.Join("mod", "go.mod"): true, "absent": false} {
		if got := s.Exists(t.Context(), path); got != want {
			t.Errorf("Exists(%q) = %t, want %t", path, got, want)
		}
	}
	if text, ok := s.Read(t.Context(), filepath.Join("mod", "go.mod")); !ok || string(text) != "module m\n" {
		t.Errorf("Read(go.mod) = %q, %t", text, ok)
	}
	if text, ok := s.Read(t.Context(), "mod"); ok {
		t.Errorf("Read(a directory) = %q, want no answer", text)
	}
	if text, ok := s.Read(t.Context(), "absent"); ok {
		t.Errorf("Read(absent) = %q, want no answer", text)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := s.WorkingDir(t.Context()); !ok || got != wd {
		t.Errorf("WorkingDir() = %q, %t; want %q", got, ok, wd)
	}
	if got, ok := s.Abs(t.Context(), "x.ts"); !ok || got != filepath.Join(wd, "x.ts") {
		t.Errorf("Abs(x.ts) = %q, %t", got, ok)
	}
}

// A done context reads nothing.
func TestSurroundingsReadNothingOnceTheContextIsDone(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module m\n")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if text, ok := (Surroundings{}).Read(ctx, filepath.Join(dir, "go.mod")); ok || text != nil {
		t.Errorf("Read() = %q, %t; want nothing", text, ok)
	}
}
