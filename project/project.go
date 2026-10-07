// Package project finds the source and test files the tools analyze.
package project

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/donvargax/itos-cc/lang"
)

// skipDirs are never walked: dependencies, caches, and fixtures that are
// not part of the program. Build output is skipped by IsBuildOutput.
var skipDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true, ".idea": true, ".vscode": true,
	"node_modules": true, "vendor": true, ".gradle": true, ".venv": true, "venv": true,
	"__pycache__": true, ".mypy_cache": true, ".pytest_cache": true, ".tox": true,
	".metrics": true, "testdata": true,
}

// buildDirs are the names build and test tools write their output to. They
// are also ordinary package names, such as Go's internal/out.
var buildDirs = map[string]bool{"target": true, "build": true, "dist": true, "out": true, "coverage": true}

// buildManifests mark a directory whose tool writes buildDirs beside them:
// npm and its bundlers, Maven, Gradle, Cargo, and Python packaging. Go
// writes no output into the module.
var buildManifests = []string{
	"package.json", "pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle",
	"settings.gradle.kts", "Cargo.toml", "pyproject.toml", "setup.py", "setup.cfg",
}

// IsBuildOutput reports whether dir is build or test output rather than
// source: it has a build output name, and it sits beside a manifest of a
// tool that writes there, or git ignores it.
func IsBuildOutput(dir string) bool {
	if !buildDirs[filepath.Base(dir)] {
		return false
	}
	for _, m := range buildManifests {
		if _, err := os.Stat(filepath.Join(filepath.Dir(dir), m)); err == nil {
			return true
		}
	}
	cmd := exec.Command("git", "check-ignore", "-q", dir)
	cmd.Dir = filepath.Dir(dir)
	return cmd.Run() == nil
}

// Files are the supported source files under some roots, split into
// production code and test code. Paths are absolute and sorted.
type Files struct {
	Sources []string
	Tests   []string
}

// Discover walks roots (files or directories) and classifies every file in a
// supported language. A root that names a file is taken even if it would be
// skipped while walking.
func Discover(roots []string) (Files, error) {
	seen := map[string]bool{}
	var files Files
	add := func(path string) {
		spec := lang.Detect(path)
		if spec == nil || seen[path] {
			return
		}
		seen[path] = true
		if spec.IsTest(path) {
			files.Tests = append(files.Tests, path)
		} else {
			files.Sources = append(files.Sources, path)
		}
	}
	for _, root := range roots {
		abs, err := filepath.Abs(root)
		if err != nil {
			return files, err
		}
		info, err := os.Stat(abs)
		if err != nil {
			return files, err
		}
		if !info.IsDir() {
			add(abs)
			continue
		}
		err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != abs && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".") || IsBuildOutput(path)) {
					return filepath.SkipDir
				}
				return nil
			}
			add(path)
			return nil
		})
		if err != nil {
			return files, err
		}
	}
	sort.Strings(files.Sources)
	sort.Strings(files.Tests)
	return files, nil
}

// Changed returns the supported files under the working directory that git
// reports as added or modified, staged or not, plus untracked files, as paths
// relative to the working directory. Git lists them NUL-terminated, so a name
// with spaces or non-ASCII letters arrives unquoted.
func Changed() ([]string, error) {
	var paths []string
	for _, args := range [][]string{
		{"diff", "--name-only", "-z", "--relative", "--diff-filter=AMR", "HEAD"},
		{"ls-files", "-z", "--others", "--exclude-standard"},
	} {
		out, err := exec.Command("git", args...).Output()
		if err != nil {
			// A repository without commits has no HEAD; fall back to the index.
			if args[0] == "diff" {
				out, err = exec.Command("git", "diff", "--name-only", "-z", "--relative", "--cached").Output()
			}
			if err != nil {
				return nil, err
			}
		}
		for _, name := range strings.Split(string(out), "\x00") {
			if name != "" && lang.Detect(name) != nil {
				if _, err := os.Stat(name); err == nil {
					paths = append(paths, name)
				}
			}
		}
	}
	return paths, nil
}

// Rel returns path relative to the working directory when it is inside it,
// and path unchanged otherwise. Reports use it so output reads like the
// command line that produced it.
func Rel(path string) string {
	wd, err := os.Getwd()
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(wd, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}
