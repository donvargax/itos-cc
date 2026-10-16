// Package porttest provides in-memory fakes for selection-domain tests,
// and the contract every Tree, fake or adapter, is held to.
package porttest

import (
	"context"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/selection/port"
)

// Tree is a working tree held in memory: the directory it is at, its
// files, by slash-separated paths relative to that directory, one outside
// it starting with ../, as the disk holds files outside the working
// directory too, the changed ones, and configured failures. It walks, names and skips as
// internal/workdir's Tree does on the disk, which TreeContract holds.
type Tree struct {
	// Dir is the working directory, absolute: a relative path resolves
	// against it, and every path Discover gives is under the root it was
	// given. TreeOf sets it.
	Dir string
	// Sources and Tests are the tree's supported files, wherever they are,
	// a skipped directory included.
	Sources []string
	Tests   []string
	// Contents are the text of the tree's files: what Read gives. A file
	// here in neither Sources nor Tests is one of no supported language. A
	// source or test with none here cannot be read.
	Contents map[string]string
	// ChangedFiles are what git reports as changed; Changed keeps the
	// sources and tests among them, unless NoGit is set.
	ChangedFiles []string
	// NoGit, when set, is Changed's failure: there is no repository.
	NoGit *port.NoGit
	// Failures are Discover's, by root as given: a walk that takes a root
	// here fails with its failure.
	Failures map[string]port.Failure
}

var _ port.Tree = Tree{}

// TreeOf is the tree at dir, an absolute path it names and never touches,
// holding files, by slash-separated paths relative to dir with their
// text, and changed, what git reports changed. Each file lang supports is
// a source or a test as lang classifies its absolute path, as the disk
// adapter classifies it.
func TreeOf(dir string, files map[string]string, changed []string) Tree {
	tree := Tree{Dir: filepath.Clean(dir), Contents: maps.Clone(files), ChangedFiles: slices.Clone(changed)}
	for _, rel := range slices.Sorted(maps.Keys(files)) {
		path := tree.path(rel)
		spec := lang.Detect(path)
		switch {
		case spec == nil:
		case spec.IsTest(path):
			tree.Tests = append(tree.Tests, rel)
		default:
			tree.Sources = append(tree.Sources, rel)
		}
	}
	return tree
}

// skipDirs are never walked into: a copy of project's list, which
// TreeContract keeps honest. A directory whose name starts with a dot is
// skipped too.
var skipDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true, ".idea": true, ".vscode": true,
	"node_modules": true, "vendor": true, ".gradle": true, ".venv": true, "venv": true,
	"__pycache__": true, ".mypy_cache": true, ".pytest_cache": true, ".tox": true,
	".metrics": true, "testdata": true,
}

// buildDirs and buildManifests are project's too: a directory of one of
// these names beside one of these files is build output, never walked
// into. What git ignores is skipped by the disk adapter alone.
var (
	buildDirs      = map[string]bool{"target": true, "build": true, "dist": true, "out": true, "coverage": true}
	buildManifests = []string{
		"package.json", "pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle",
		"settings.gradle.kts", "Cargo.toml", "pyproject.toml", "setup.py", "setup.cfg",
	}
)

// Discover is the files under roots, absolute: a file's path is that file,
// wherever it is, and a directory's every file below it but those under a
// hidden, skipped or build output directory beneath it. Each list is
// sorted and holds a file once. A root that is not there is Unwalkable,
// and a done ctx Cancelled.
func (t Tree) Discover(ctx context.Context, roots []string) (sources, tests []string, failure port.Failure) {
	if ctx.Err() != nil {
		return nil, nil, &port.Cancelled{}
	}
	for _, root := range roots {
		if failure, ok := t.Failures[root]; ok {
			return nil, nil, failure
		}
		abs := t.abs(root)
		if !t.Exists(ctx, root) {
			return nil, nil, &port.Unwalkable{Reason: "stat " + abs + ": no such file or directory in the fake"}
		}
		sources = append(sources, t.walk(abs, t.Sources)...)
		tests = append(tests, t.walk(abs, t.Tests)...)
	}
	return sorted(sources), sorted(tests), nil
}

// Changed is the sources and tests among ChangedFiles, the caller's own
// copy, NoGit when it is set, or Cancelled once ctx is done.
func (t Tree) Changed(ctx context.Context) ([]string, port.Failure) {
	if ctx.Err() != nil {
		return nil, &port.Cancelled{}
	}
	if t.NoGit != nil {
		return nil, t.NoGit
	}
	var changed []string
	for _, rel := range t.ChangedFiles {
		if slices.Contains(t.Sources, rel) || slices.Contains(t.Tests, rel) {
			changed = append(changed, rel)
		}
	}
	return changed, nil
}

// Exists says whether path, relative to Dir or absolute, is a file of the
// tree, a directory above one, Dir itself or a directory Dir is under. It
// answers at once, so ctx has nothing to cut short.
func (t Tree) Exists(ctx context.Context, path string) bool {
	abs := t.abs(path)
	if above, err := filepath.Rel(abs, t.Dir); err == nil && (above == "." || filepath.IsLocal(above)) {
		return true
	}
	rel, ok := t.rel(abs)
	return ok && (t.isFile(rel) || t.isDir(rel))
}

// Read is the text in Contents of the file at path, relative to Dir or
// absolute, Unreadable when it has none, or Cancelled once ctx is done.
func (t Tree) Read(ctx context.Context, path string) ([]byte, port.Failure) {
	if ctx.Err() != nil {
		return nil, &port.Cancelled{}
	}
	if rel, ok := t.rel(t.abs(path)); ok {
		if text, ok := t.Contents[rel]; ok {
			return []byte(text), nil
		}
	}
	return nil, &port.Unreadable{Path: path, Reason: "open " + path + ": no such file in the fake"}
}

// abs is path made absolute against Dir, as the working directory makes a
// relative path absolute. A path rooted with no volume, as Windows writes
// one, is on Dir's volume.
func (t Tree) abs(path string) string {
	switch {
	case filepath.IsAbs(path):
		return filepath.Clean(path)
	case filepath.VolumeName(path) == "" && strings.HasPrefix(filepath.ToSlash(path), "/"):
		return filepath.Clean(filepath.VolumeName(t.Dir) + path)
	default:
		return filepath.Join(t.Dir, path)
	}
}

// rel is abs as the tree holds it, slash-separated from Dir, ../ leading
// it out of Dir, and whether Dir reaches it, as one on another volume it
// does not.
func (t Tree) rel(abs string) (string, bool) {
	rel, err := filepath.Rel(t.Dir, abs)
	if err != nil {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// path is the absolute path of rel, a file of the tree.
func (t Tree) path(rel string) string {
	return filepath.Join(t.Dir, filepath.FromSlash(rel))
}

// isFile says whether rel is one of the tree's files.
func (t Tree) isFile(rel string) bool {
	_, ok := t.Contents[rel]
	return ok || slices.Contains(t.Sources, rel) || slices.Contains(t.Tests, rel)
}

// isDir says whether rel is a directory above one of the tree's files.
func (t Tree) isDir(rel string) bool {
	below := func(f string) bool { return strings.HasPrefix(f, rel+"/") }
	return slices.ContainsFunc(t.Sources, below) || slices.ContainsFunc(t.Tests, below) ||
		slices.ContainsFunc(slices.Collect(maps.Keys(t.Contents)), below)
}

// walk is the files of list, absolute, a walk from root takes: root
// itself when it names one, else each below it with no hidden, skipped or
// build output directory between them.
func (t Tree) walk(root string, list []string) []string {
	var out []string
	for _, f := range list {
		path := t.path(f)
		rel, err := filepath.Rel(root, path)
		if path == root || err == nil && filepath.IsLocal(rel) && !t.skips(root, rel) {
			out = append(out, path)
		}
	}
	return out
}

// skips says whether a walk from root skips a directory on its way down to
// rel, a file below it.
func (t Tree) skips(root, rel string) bool {
	dir := root
	for name := range strings.SplitSeq(filepath.ToSlash(filepath.Dir(rel)), "/") {
		if name == "." {
			return false
		}
		dir = filepath.Join(dir, name)
		if skipDirs[name] || strings.HasPrefix(name, ".") || t.isBuildOutput(dir) {
			return true
		}
	}
	return false
}

// isBuildOutput says whether dir has a build output's name and a build
// tool's manifest of the tree beside it.
func (t Tree) isBuildOutput(dir string) bool {
	if !buildDirs[filepath.Base(dir)] {
		return false
	}
	for _, m := range buildManifests {
		if rel, ok := t.rel(filepath.Join(filepath.Dir(dir), m)); ok && t.isFile(rel) {
			return true
		}
	}
	return false
}

// sorted is list sorted, each file once.
func sorted(list []string) []string {
	slices.Sort(list)
	return slices.Compact(list)
}
