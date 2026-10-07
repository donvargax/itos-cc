package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var roots sync.Map // directory → its root, as RootOf finds it

// Root is the project root of the working directory: where .metrics/ and
// itos-cc.yaml live, and what every path a snapshot records is relative
// to. It is the git top level of the working directory, or the working
// directory itself outside a git repository. It is absolute, in the form
// os.Getwd gives.
func Root() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return RootOf(wd)
}

// RootOf is the project root of the directory dir, absolute: the git top
// level of dir, or dir itself outside a git repository. It is found from
// dir as given, so a root reached through a symbolic link keeps its form.
func RootOf(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	if r, ok := roots.Load(abs); ok {
		return r.(string)
	}
	root := abs
	cmd := exec.Command("git", "rev-parse", "--show-cdup")
	cmd.Dir = abs
	if out, err := cmd.Output(); err == nil {
		root = filepath.Join(abs, filepath.FromSlash(strings.TrimSpace(string(out))))
	}
	roots.Store(abs, root)
	return root
}

// FromRoot is path, absolute or relative to the working directory, as a
// snapshot records it: relative to the project root and slash-separated.
// A path outside the root stays absolute.
func FromRoot(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	rel, err := filepath.Rel(Root(), abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(abs)
	}
	return filepath.ToSlash(rel)
}

// AtRoot is the absolute path of rel, a path a snapshot records.
func AtRoot(rel string) string {
	if filepath.IsAbs(filepath.FromSlash(rel)) {
		return filepath.FromSlash(rel)
	}
	return filepath.Join(Root(), filepath.FromSlash(rel))
}
