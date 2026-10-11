// Package porttest provides in-memory fakes for selection-domain tests.
package porttest

import (
	"context"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/selection/port"
)

// Tree is a working tree held in memory: its files, by slash-separated
// paths relative to ".", the changed ones, and configured failures.
type Tree struct {
	// Sources and Tests are every file of the tree.
	Sources []string
	Tests   []string
	// ChangedFiles is what Changed returns, unless NoGit is set.
	ChangedFiles []string
	// NoGit, when set, is Changed's failure.
	NoGit *port.NoGit
	// Failures are Discover's, by root: a walk that takes a root here fails
	// with its failure.
	Failures map[string]port.Failure
}

var _ port.Tree = Tree{}

// Discover is the files under roots: "." is every file, a file's path is
// that file, and a directory's path every file below it. Each list is
// sorted and holds a file once, as a real walk gives them.
func (t Tree) Discover(ctx context.Context, roots []string) (sources, tests []string, failure port.Failure) {
	for _, root := range roots {
		if failure, ok := t.Failures[root]; ok {
			return nil, nil, failure
		}
		sources = append(sources, under(t.Sources, root)...)
		tests = append(tests, under(t.Tests, root)...)
	}
	return sorted(sources), sorted(tests), nil
}

// Changed is ChangedFiles, or NoGit when it is set.
func (t Tree) Changed(ctx context.Context) ([]string, port.Failure) {
	if t.NoGit != nil {
		return nil, t.NoGit
	}
	return append([]string(nil), t.ChangedFiles...), nil
}

// Exists says whether path is ".", a file of the tree, or a directory
// above one.
func (t Tree) Exists(ctx context.Context, path string) bool {
	return path == "." || len(under(t.Sources, path)) > 0 || len(under(t.Tests, path)) > 0
}

// under is the files of list at or below root.
func under(list []string, root string) []string {
	var out []string
	for _, f := range list {
		if root == "." || f == root || strings.HasPrefix(f, strings.TrimSuffix(root, "/")+"/") {
			out = append(out, f)
		}
	}
	return out
}

// sorted is list sorted, each file once.
func sorted(list []string) []string {
	slices.Sort(list)
	return slices.Compact(list)
}
