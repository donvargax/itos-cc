package worktrees

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"go.yaml.in/yaml/v3"
)

// What a worker copies of a TypeScript package that belongs to a workspace
// (slice-19, issue #44): the workspace's root, whose files the package may
// read from above its own folder, such as the tsconfig.base.json its
// tsconfig.json extends, and every member beside it. Two members of one
// workspace share one copy per worker. A workspace root above the project
// root is never copied, and nothing outside a workspace is followed.

// nodeWorkspace is the tree a worker copies for the TypeScript package at
// root: the root of the workspace it belongs to, the nearest folder above
// it and within bound, the project root, that is a workspace whose globs
// match it; root itself for a package in none. A workspace is a
// pnpm-workspace.yaml with a packages key, or a package.json with a
// workspaces field, as npm and yarn read it.
func nodeWorkspace(root, bound string) string {
	if !regular(filepath.Join(root, "package.json")) {
		return root
	}
	for dir := filepath.Dir(root); dir != filepath.Dir(dir) && (dir == bound || within(bound, dir)); dir = filepath.Dir(dir) {
		globs, ok := workspaceGlobs(dir)
		if !ok {
			continue
		}
		rel, err := filepath.Rel(dir, root)
		if err == nil && member(filepath.ToSlash(rel), globs) {
			return dir
		}
	}
	return root
}

// workspaceGlobs are the member globs of the workspace whose root is dir,
// and whether dir is one: pnpm-workspace.yaml's packages, else
// package.json's workspaces, a list or yarn's object with packages.
func workspaceGlobs(dir string) ([]string, bool) {
	if data, err := os.ReadFile(filepath.Join(dir, "pnpm-workspace.yaml")); err == nil { //nolint:gosec // a workspace file above the package the person selected, within the project root
		var pnpm struct {
			Packages []string `yaml:"packages"`
		}
		if yaml.Unmarshal(data, &pnpm) == nil && len(pnpm.Packages) > 0 {
			return pnpm.Packages, true
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "package.json")) //nolint:gosec // a manifest above the package the person selected, within the project root
	if err != nil {
		return nil, false
	}
	var pkg struct {
		Workspaces json.RawMessage `json:"workspaces"`
	}
	if json.Unmarshal(data, &pkg) != nil || len(pkg.Workspaces) == 0 {
		return nil, false
	}
	var globs []string
	if json.Unmarshal(pkg.Workspaces, &globs) == nil {
		return globs, len(globs) > 0
	}
	var yarn struct {
		Packages []string `json:"packages"`
	}
	if json.Unmarshal(pkg.Workspaces, &yarn) == nil {
		return yarn.Packages, len(yarn.Packages) > 0
	}
	return nil, false
}

// member says whether rel, a folder's slash-separated path from a
// workspace's root, matches its globs: one that matches, after a leading
// ./ and a trailing / are dropped, and no later one that excludes it with
// a leading !, as pnpm reads them.
func member(rel string, globs []string) bool {
	in := false
	for _, glob := range globs {
		exclude := strings.HasPrefix(glob, "!")
		glob = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(glob, "!"), "./"), "/")
		if ok, err := doublestar.Match(glob, rel); err == nil && ok {
			in = !exclude
		}
	}
	return in
}

// regular says whether path is a regular file.
func regular(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
