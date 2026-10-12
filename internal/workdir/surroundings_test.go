package workdir

import (
	"os"
	"path/filepath"
	"testing"
)

// Surroundings answer lang's questions from the disk of the working
// directory: a path is there when it is, a file holds what it holds, and a
// relative path is made absolute from the working directory.
func TestSurroundingsAnswerFromTheWorkingDirectory(t *testing.T) {
	dir := tree(t)
	s := Surroundings{}
	if !s.Exists(t.Context(), "pkg") || s.Exists(t.Context(), "absent") {
		t.Errorf("Exists(pkg), Exists(absent) = %t, %t; want true, false", s.Exists(t.Context(), "pkg"), s.Exists(t.Context(), "absent"))
	}
	if text, ok := s.Read(t.Context(), filepath.Join("pkg", "sum.go")); !ok || string(text) != "package pkg\n" {
		t.Errorf("Read(pkg/sum.go) = %q, %t", text, ok)
	}
	if text, ok := s.Read(cancelled(t), filepath.Join("pkg", "sum.go")); ok || text != nil {
		t.Errorf("Read() with a done context = %q, %t; want nothing", text, ok)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := s.WorkingDir(t.Context()); !ok || got != wd {
		t.Errorf("WorkingDir() = %q, %t; want %q", got, ok, wd)
	}
	if got, ok := s.Abs(t.Context(), "pkg"); !ok || got != filepath.Join(wd, "pkg") {
		t.Errorf("Abs(pkg) = %q, %t; want under %q", got, ok, dir)
	}
}
