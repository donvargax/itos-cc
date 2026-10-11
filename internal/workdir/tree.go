// Package workdir adapts the working directory, as the disk and git hold
// it, to the selection domain's Tree port: it walks it with project's
// discovery, asks git for its changes, stats paths and reads files,
// translating what fails into the port's failures.
package workdir

import (
	"context"
	"os"

	"github.com/donvargax/itos-cc/internal/selection/port"
	"github.com/donvargax/itos-cc/project"
)

// Tree is the working directory of this process.
type Tree struct{}

var _ port.Tree = Tree{}

// Discover walks roots with project.Discover, its paths absolute. A done
// ctx walks nothing; a walk under way runs to its end, which a large tree's
// takes seconds at most.
func (Tree) Discover(ctx context.Context, roots []string) (sources, tests []string, failure port.Failure) {
	if ctx.Err() != nil {
		return nil, nil, &port.Cancelled{}
	}
	files, err := project.Discover(roots)
	if err != nil {
		return nil, nil, &port.Unwalkable{Reason: err.Error()}
	}
	return files.Sources, files.Tests, nil
}

// Changed asks git, through project.Changed, for the changed files. Any
// failure to is NoGit, with what it said: git missing, or no repository.
// A done ctx asks nothing.
func (Tree) Changed(ctx context.Context) ([]string, port.Failure) {
	if ctx.Err() != nil {
		return nil, &port.Cancelled{}
	}
	changed, err := project.Changed()
	if err != nil {
		return nil, &port.NoGit{Reason: err.Error()}
	}
	return changed, nil
}

// Exists says whether path can be stat'ed: a path that cannot, for any
// reason, is not there. A stat ends at once, so ctx has nothing to cut
// short.
func (Tree) Exists(ctx context.Context, path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Read reads the file at path, any failure to Unreadable with the read's
// own message, as the command line has always printed it. A done ctx reads
// nothing; a read under way runs to its end.
func (Tree) Read(ctx context.Context, path string) ([]byte, port.Failure) {
	if ctx.Err() != nil {
		return nil, &port.Cancelled{}
	}
	text, err := os.ReadFile(path) //nolint:gosec // the person selected the path, reading it is the point
	if err != nil {
		return nil, &port.Unreadable{Path: path, Reason: err.Error()}
	}
	return text, nil
}
