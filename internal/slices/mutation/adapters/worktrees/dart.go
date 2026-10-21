package worktrees

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// What a worker's copy of a Dart package holds (slice-35): no build output
// or coverage report beside its pubspec.yaml, and, for a Flutter package,
// none of the platform folders flutter create writes, which discovery never
// walks into either (slice-30, q-75): flutter test's tester uses none of
// them, and ios's Pods and .symlinks are large. Generated Dart stays, as
// tests compile against it (q-77).

// dartOutput are the folders flutter writes beside a pubspec.yaml: its
// build output, which build/test_cache's incremental compiles fill anew in
// each copy, and flutter test --coverage's default report.
var dartOutput = map[string]bool{"build": true, "coverage": true}

// flutterPlatforms are the platform folders of a Flutter package.
var flutterPlatforms = map[string]bool{"android": true, "ios": true, "macos": true, "linux": true, "windows": true, "web": true}

// pubspec is what a worker reads of a pubspec.yaml: whether it depends on
// the flutter SDK, whether it is a pub workspace's member, and the members
// a workspace's root lists.
type pubspec struct {
	Dependencies map[string]any `yaml:"dependencies"`
	Resolution   string         `yaml:"resolution"`
	Workspace    []string       `yaml:"workspace"`
}

// readPubspec reads the pubspec.yaml in dir; ok is false when there is
// none or it does not parse.
func readPubspec(dir string) (pubspec, bool) {
	var p pubspec
	data, err := os.ReadFile(filepath.Join(dir, "pubspec.yaml")) //nolint:gosec // the manifest of a package the person selected, or of the folders above it
	if err != nil || yaml.Unmarshal(data, &p) != nil {
		return pubspec{}, false
	}
	return p, true
}

// flutter says whether the package depends on the flutter SDK.
func (p pubspec) flutter() bool {
	dep, ok := p.Dependencies["flutter"].(map[string]any)
	return ok && dep["sdk"] == "flutter"
}

// dartLeftOut says whether a worker's copy leaves out the folder dir:
// build/ or coverage/ beside a pubspec.yaml, or a platform folder beside
// a Flutter package's.
func dartLeftOut(dir string) bool {
	name := filepath.Base(dir)
	if !dartOutput[name] && !flutterPlatforms[name] {
		return false
	}
	p, ok := readPubspec(filepath.Dir(dir))
	return ok && (dartOutput[name] || p.flutter())
}

// pubWorkspace is the tree a worker copies for the package at root, and,
// for a pub workspace's member, the members it copies: the member resolves
// only with its workspace's root pubspec.yaml and pubspec.lock above it and
// the members it may depend on at their places, so the tree is the
// workspace's root and members are every member it lists. Any other
// package's tree is root, with no members.
func pubWorkspace(root string) (string, []string) {
	p, ok := readPubspec(root)
	if !ok || p.Resolution != "workspace" {
		return root, nil
	}
	for dir := filepath.Dir(root); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		ws, ok := readPubspec(dir)
		if !ok || len(ws.Workspace) == 0 {
			continue
		}
		var members []string
		for _, pattern := range ws.Workspace {
			// Globs arrived in Dart 3.11; a plain path matches itself.
			matches, _ := filepath.Glob(filepath.Join(dir, filepath.FromSlash(pattern)))
			for _, m := range matches {
				if _, ok := readPubspec(m); ok && !slices.Contains(members, m) {
					members = append(members, m)
				}
			}
		}
		if slices.Contains(members, filepath.Clean(root)) {
			slices.Sort(members)
			// A member inside another is copied with it.
			var outer []string
			for _, m := range members {
				if !slices.ContainsFunc(members, func(o string) bool { return within(o, m) }) {
					outer = append(outer, m)
				}
			}
			return dir, outer
		}
	}
	return root, nil
}

// within says whether path is below dir, not dir itself.
func within(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
