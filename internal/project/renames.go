package project

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// RenamesSince maps each file the commits since ref renamed, by the
// absolute path it had at ref, to the absolute path it has at HEAD, across
// the whole repository of the working directory, both under the project
// root (Root): git diff -M ref...HEAD, the rename detection ChangedSince
// follows renames by. ref must be one ChangedSince accepted.
func RenamesSince(ref string) (map[string]string, error) {
	return renames(ref + "...HEAD")
}

// RenamesUncommitted maps each file the working tree has renamed against
// HEAD, as RenamesSince maps a range's: git diff -M HEAD, which sees a
// rename once the new file is tracked, staged as git mv leaves it or added
// with git add. A plain mv with nothing staged is a deletion and an
// untracked file to git, so no rename. It returns a *NoGitError outside a
// git repository, or in one with no commit yet.
func RenamesUncommitted() (map[string]string, error) {
	return renames("HEAD")
}

// renames maps the files git diff -M rev shows renamed, from their old
// absolute path to their new one.
func renames(rev string) (map[string]string, error) {
	if out, err := gitCommand("", "rev-parse", "--verify", "HEAD^{commit}").CombinedOutput(); err != nil {
		return nil, &NoGitError{gitError(out, err)}
	}
	root := Root()
	// -z prints each path as it is, unquoted; --name-status a rename as
	// R<score>, its old path and its new one, from the repository's top.
	out, err := gitCommand("", "diff", "--name-status", "-z", "-M", "--no-color", "--no-relative", "--diff-filter=R", rev, "--").Output()
	if err != nil {
		return nil, fmt.Errorf("git diff %s: %s", rev, gitError(nil, err))
	}
	return renamedPaths(out, rev, root)
}

func renamedPaths(out []byte, rev, root string) (map[string]string, error) {
	moves := map[string]string{}
	fields := strings.Split(string(out), "\x00")
	for i := 0; i+2 < len(fields); i += 3 {
		if !strings.HasPrefix(fields[i], "R") {
			return nil, fmt.Errorf("git diff %s: %q is no rename", rev, fields[i])
		}
		moves[filepath.Join(root, filepath.FromSlash(fields[i+1]))] = filepath.Join(root, filepath.FromSlash(fields[i+2]))
	}
	return moves, nil
}

// RenamesSinceAt and RenamesUncommittedAt use the same repository-wide map
// under a caller directory/context, independently of --since file narrowing.
func RenamesSinceAt(ctx context.Context, dir, ref string) (map[string]string, error) {
	return renamesAt(ctx, dir, ref+"...HEAD")
}
func RenamesUncommittedAt(ctx context.Context, dir string) (map[string]string, error) {
	return renamesAt(ctx, dir, "HEAD")
}
func renamesAt(ctx context.Context, dir, rev string) (map[string]string, error) {
	if _, err := HeadAt(ctx, dir); err != nil {
		return nil, err
	}
	rootOut, err := historyCommand(ctx, dir, "rev-parse", "--show-cdup").Output()
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	if err != nil {
		return nil, err
	}
	root, err := historyAbs(dir, filepath.FromSlash(strings.TrimSpace(string(rootOut))))
	if err != nil {
		return nil, err
	}
	out, err := historyCommand(ctx, dir, "diff", "--name-status", "-z", "-M", "--no-color", "--no-relative", "--diff-filter=R", rev, "--").Output()
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("git diff %s: %s", rev, gitError(nil, err))
	}
	return renamedPaths(out, rev, root)
}
