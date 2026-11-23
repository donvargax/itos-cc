// Package past is what a command that reads sources alone reaches to
// measure a past commit (--at): the commit's tree as its Tree and
// Surroundings, its own itos-cc.yaml as its settings, a Store that keeps
// nothing, and the results kept for it, per commit, in the git common
// directory. A slice gives itself the same ports a run of the working tree
// has and never learns where the files come from; this package holds what
// is the same for every one of them: the keeping of a result, and the
// failures an unknown revision or no repository is.
package past

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang/port"
	"github.com/donvargax/itos-cc/internal/selection"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// Failure is why a past commit could not be opened, without exposing git's
// own errors to the slices.
//
//sumtype:decl
type Failure interface {
	error
	pastFailure()
}

// BadRev is a revision git cannot name as a commit of the repository.
type BadRev struct{ Rev string }

func (*BadRev) pastFailure()    {}
func (e *BadRev) Error() string { return e.Rev + " names no commit of this repository" }

// NoRepository is no git repository to ask, or no git. Reason is what git
// said, or why it could not be run.
type NoRepository struct{ Reason string }

func (*NoRepository) pastFailure()    {}
func (e *NoRepository) Error() string { return "no git repository: " + e.Reason }

// Source opens the commits of the repository holding the working directory.
type Source interface {
	// Open is the commit rev names, any revision git rev-parse takes, or a
	// Failure; once ctx is done, its cause.
	Open(ctx context.Context, rev string) (Commit, error)
}

// Results keeps what a command measured of a commit, so the next run of
// the same question, in any worktree of the repository, reads it back. A
// result is written once, whole, never half.
type Results interface {
	// Get is what Put kept under commit, command and key, false when none
	// is there or it cannot be read.
	Get(ctx context.Context, commit, command, key string) ([]byte, bool)
	// Put keeps data. Keeping is best effort: one that cannot be written
	// is no failure of the run that measured.
	Put(ctx context.Context, commit, command, key string, data []byte)
}

// Commit is one commit, opened: the ports a command measures it through.
type Commit struct {
	// ID is the commit's full id, which names it for good.
	ID string
	// Root is the project root of the working directory, absolute: the
	// commit's files are named as they would be there, so a result names
	// its files from the working directory as a run of the checked-out
	// commit would.
	Root string
	// Dir is the working directory, absolute, which the files are named
	// from, relative to Root.
	Dir          string
	Tree         selport.Tree
	Surroundings port.Surroundings
	// Exclude is the sources the commit's own itos-cc.yaml leaves out.
	Exclude selection.Exclusion
	// Store keeps nothing: the working tree is never changed.
	Store snapport.Store
	// Results are the commit's kept results.
	Results Results
}

// Rel is path, absolute, named from Root, slash-separated, as a kept
// result names its files in any worktree.
func (c Commit) Rel(path string) string {
	rel, err := filepath.Rel(c.Root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// Abs is rel, as Rel names a path, absolute again under Root.
func (c Commit) Abs(rel string) string {
	if filepath.IsAbs(filepath.FromSlash(rel)) {
		return filepath.FromSlash(rel)
	}
	return filepath.Join(c.Root, filepath.FromSlash(rel))
}

// Key is the name of one question asked of a commit, from what changes its
// answer beyond the commit and the command: query, whatever the command
// line asked (its flags and paths), the working directory's place in the
// project, which the paths are taken from, and version, itos-cc's own. The
// same question gets the same key in every worktree.
func (c Commit) Key(query any, version string) string {
	place, err := filepath.Rel(c.Root, c.Dir)
	if err != nil {
		place = c.Dir
	}
	text, err := json.Marshal(struct {
		Query   any    `json:"query"`
		Place   string `json:"place"`
		Version string `json:"version"`
	}{query, filepath.ToSlash(place), version})
	if err != nil {
		// A query that cannot be encoded has no key worth keeping under.
		text = fmt.Appendf(nil, "%v", query)
	}
	sum := sha256.Sum256(text)
	return hex.EncodeToString(sum[:8])
}

// Kept is the answer to a question about c: what an earlier run kept
// under key for command, thawed, else what compute measures, frozen and
// kept before it is returned. A kept result that cannot be thawed is
// measured again, as is one nothing kept; keeping never fails the run.
// freeze and thaw name the files of the result from Root and back, so one
// kept in a worktree reads in another.
func Kept[T any](ctx context.Context, c Commit, command, key string, compute func() (T, error), freeze func(T) ([]byte, error), thaw func([]byte) (T, error)) (T, error) {
	if c.Results != nil {
		if data, ok := c.Results.Get(ctx, c.ID, command, key); ok {
			if value, err := thaw(data); err == nil {
				return value, nil
			}
		}
	}
	value, err := compute()
	if err != nil {
		return value, err
	}
	if c.Results != nil {
		if data, err := freeze(value); err == nil {
			c.Results.Put(ctx, c.ID, command, key, data)
		}
	}
	return value, nil
}

// IsFailure is whether err is a Failure of this package.
func IsFailure(err error) (Failure, bool) {
	var f Failure
	ok := errors.As(err, &f)
	return f, ok
}

// Nothing is the Store of a commit: it holds no snapshot and keeps none, so
// a command measures the commit alone, compared with nothing, and the
// working tree's .metrics/ is neither read nor written.
type Nothing struct{ RootDir string }

var _ snapport.Store = Nothing{}

// Root is the project root.
func (n Nothing) Root(context.Context) (string, snapport.Failure) { return n.RootDir, nil }

// Read finds no snapshot.
func (Nothing) Read(context.Context, string, any) (bool, snapport.Failure) { return false, nil }

// Write keeps nothing.
func (Nothing) Write(context.Context, string, any) snapport.Failure { return nil }

// Remove has nothing to remove.
func (Nothing) Remove(context.Context, string) snapport.Failure { return nil }

// List holds no snapshot.
func (Nothing) List(context.Context, string) ([]string, snapport.Failure) { return nil, nil }
