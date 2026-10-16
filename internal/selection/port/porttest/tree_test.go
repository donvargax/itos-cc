package porttest

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/selection/port"
)

// The fake does what internal/workdir's Tree does on the disk. Its
// directory is a name only: the fake never touches it, and a temporary
// directory's is absolute on every system.
func TestTreeKeepsTheContract(t *testing.T) {
	TreeContract(t, func(t *testing.T, f TreeFixture) (port.Tree, string) {
		dir := t.TempDir()
		tree := TreeOf(dir, f.Files, f.Changed)
		if !f.Git {
			tree.NoGit = &port.NoGit{Reason: "not a git repository"}
		}
		return tree, dir
	})
}

// A failure configured for a root is Discover's when a walk takes that
// root, before anything is found.
func TestTreeFailsAtAConfiguredRoot(t *testing.T) {
	walk := &port.Unwalkable{Reason: "open src: permission denied"}
	tree := TreeOf(t.TempDir(), map[string]string{"src/a.go": "package src\n", "lib/b.go": "package lib\n"}, nil)
	tree.Failures = map[string]port.Failure{"src": walk}
	if sources, _, failure := tree.Discover(t.Context(), []string{"lib", "src"}); failure != walk || sources != nil {
		t.Errorf("Discover(lib, src) = %q, %v; want the configured failure and nothing", sources, failure)
	}
	if sources, _, failure := tree.Discover(t.Context(), []string{"lib"}); failure != nil || len(sources) != 1 {
		t.Errorf("Discover(lib) = %q, %v; want lib/b.go", sources, failure)
	}
}

// A source with no text in Contents cannot be read, as a file the disk
// will not open, while it is still found and there.
func TestTreeCannotReadASourceWithNoText(t *testing.T) {
	dir := t.TempDir()
	tree := TreeOf(dir, map[string]string{"src/a.go": "package src\n"}, nil)
	delete(tree.Contents, "src/a.go")
	path := filepath.Join(dir, "src", "a.go")
	_, failure := tree.Read(t.Context(), path)
	var unreadable *port.Unreadable
	if !errors.As(failure, &unreadable) || unreadable.Path != path || !tree.Exists(t.Context(), "src/a.go") {
		t.Errorf("Read(%q) = %T %v; want Unreadable of a file that is there", path, failure, failure)
	}
}

// NoGit is Changed's failure whenever it is configured, the changed files
// set or not.
func TestTreeChangedIsTheConfiguredNoGit(t *testing.T) {
	noGit := &port.NoGit{Reason: "exit status 128"}
	tree := TreeOf(t.TempDir(), map[string]string{"a.go": "package a\n"}, []string{"a.go"})
	tree.NoGit = noGit
	if changed, failure := tree.Changed(t.Context()); failure != noGit || changed != nil {
		t.Errorf("Changed() = %q, %v; want the configured NoGit", changed, failure)
	}
}

// Paths rooted with no volume, as Windows writes them, are on the volume
// of the tree's directory, so a tree at such a path names its files under
// it on every system.
func TestTreeAtARootedPathNamesItsFilesUnderIt(t *testing.T) {
	dir := filepath.FromSlash("/work")
	tree := TreeOf(dir, map[string]string{"src/a.go": "package src\n"}, nil)
	path := filepath.Join(dir, "src", "a.go")
	sources, _, failure := tree.Discover(t.Context(), []string{filepath.Join(dir, "src")})
	if failure != nil || len(sources) != 1 || sources[0] != path {
		t.Errorf("Discover(%q) = %q, %v; want %q", filepath.Join(dir, "src"), sources, failure, path)
	}
	if text, failure := tree.Read(t.Context(), path); failure != nil || string(text) != "package src\n" {
		t.Errorf("Read(%q) = %q, %v", path, text, failure)
	}
}

// A file outside the working directory, ../ leading to it, is there, reads
// and is found by a walk from it, as the disk holds it, while a walk of the
// working directory never reaches it.
func TestTreeHoldsFilesOutsideItsDirectory(t *testing.T) {
	tree := TreeOf("/work/src", map[string]string{"a.go": "package src\n", "../lib/b.go": "package lib\n"}, nil)
	outside := filepath.FromSlash("/work/lib/b.go")
	if !tree.Exists(t.Context(), "../lib/b.go") || !tree.Exists(t.Context(), outside) || !tree.Exists(t.Context(), "../lib") {
		t.Error("the file outside the working directory, or its folder, is not there")
	}
	if text, failure := tree.Read(t.Context(), outside); failure != nil || string(text) != "package lib\n" {
		t.Errorf("Read(%s) = %q, %v", outside, text, failure)
	}
	if sources, _, failure := tree.Discover(t.Context(), []string{"."}); failure != nil || len(sources) != 1 {
		t.Errorf("Discover(.) = %q, %v; want a.go alone", sources, failure)
	}
	if sources, _, failure := tree.Discover(t.Context(), []string{"../lib"}); failure != nil || len(sources) != 1 || sources[0] != outside {
		t.Errorf("Discover(../lib) = %q, %v; want %s", sources, failure, outside)
	}
}
