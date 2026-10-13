package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/mutate"
	"github.com/donvargax/itos-cc/project"
)

// orphanEntry is an orphaned snapshot in the --json output of mutation run
// and check: "file", the source's path as "files" names a file, from the
// working directory, "snapshot", the snapshot's path from the project
// root, slash-separated, and "why", gone or excluded.
type orphanEntry struct {
	File     string `json:"file"`
	Snapshot string `json:"snapshot"`
	Why      string `json:"why"`
}

// orphansCovered are the snapshots whose source file is gone, or there but
// excluded by itos-cc.yaml (mutate.Orphans), that the paths of mutation run
// or check cover: those under a directory given, or whose path from the
// working directory holds a fragment given, as a file's must to be chosen.
// With no paths, whatever --changed or --since chooses, they cover the
// working directory, the whole project when run at its root, as a run
// without paths discovers its files there. The snapshots are those of the
// store the invocation's adapters hold, read under ctx.
func orphansCovered(ctx context.Context, in *invocation) ([]mutate.Orphan, error) {
	exclude, err := exclusion()
	if err != nil {
		return nil, err
	}
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	var dirs, fragments []string
	for _, p := range in.args {
		info, err := os.Stat(p)
		switch {
		case err != nil:
			fragments = append(fragments, filepath.ToSlash(p))
		case info.IsDir():
			abs, err := filepath.Abs(p)
			if err != nil {
				return nil, err
			}
			dirs = append(dirs, abs)
		}
	}
	if len(in.args) == 0 {
		dirs = []string{wd}
	}
	orphans, err := mutate.Orphans(ctx, in.adapters.Store, func(path string) bool {
		for _, dir := range dirs {
			if beneath(dir, path) {
				return true
			}
		}
		if !beneath(wd, path) {
			return false
		}
		rel, err := filepath.Rel(wd, path)
		if err != nil {
			return false
		}
		for _, frag := range fragments {
			if strings.Contains(filepath.ToSlash(rel), frag) {
				return true
			}
		}
		return false
	}, exclude.Excludes)
	if err != nil || len(orphans) == 0 {
		return orphans, err
	}
	return notRenamed(in, orphans), nil
}

// notRenamed are the orphans git shows no rename of to a file that
// exists, so a run with --since over the new path still moves their
// snapshots: renamed in the --since range, when there is one, or in the
// working tree against HEAD, staged as git mv leaves it or added. A rename
// to a path where nothing is, a deletion, a plain mv git sees as a
// deletion and an untracked file, or a file git does not track keeps an
// orphan one, and so does an excluded source, which is there. Outside a
// git repository, or when git fails, no orphan can be judged, and none is:
// each is kept, never named nor removed.
func notRenamed(in *invocation, orphans []mutate.Orphan) []mutate.Orphan {
	uncommitted, err := project.RenamesUncommitted()
	if err != nil {
		return nil
	}
	var since map[string]string
	if in.set("since") {
		if since, err = project.RenamesSince(in.str("since")); err != nil {
			return nil
		}
	}
	var out []mutate.Orphan
	for _, o := range orphans {
		if o.Why == mutate.OrphanExcluded || !renamedToAFile(o.Path, since, uncommitted) {
			out = append(out, o)
		}
	}
	return out
}

// renamedToAFile reports whether the renames of path, in the range (since)
// and then in the working tree (uncommitted), each map of an old absolute
// path to a new one, lead to a path where something is.
func renamedToAFile(path string, since, uncommitted map[string]string) bool {
	seen := map[string]bool{path: true}
	for {
		next, ok := since[path]
		if !ok {
			if next, ok = uncommitted[path]; !ok {
				return false
			}
		}
		if seen[next] {
			return false
		}
		if _, err := os.Lstat(next); err == nil {
			return true
		}
		seen[next], path = true, next
	}
}

// beneath reports whether path is inside dir.
func beneath(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// orphansJSON is each orphan as the --json output lists it.
func orphansJSON(orphans []mutate.Orphan) []orphanEntry {
	var out []orphanEntry
	for _, o := range orphans {
		out = append(out, orphanEntry{File: project.Rel(o.Path), Snapshot: o.Snapshot, Why: o.Why})
	}
	return out
}

// printOrphans prints, in plain output, a line for each orphan:
// "<file>: <why>, its snapshot <snapshot> <what>", why gone or excluded.
func printOrphans(in *invocation, orphans []orphanEntry, what string) {
	if in.json {
		return
	}
	for _, o := range orphans {
		fmt.Printf("%s: %s, its snapshot %s %s\n", o.File, o.Why, o.Snapshot, what)
	}
}

// removeOrphans removes the orphans the paths of a mutation run cover
// (orphansCovered), printing a line for each in plain output, and returns
// them as the --json output lists them, under ctx.
func removeOrphans(ctx context.Context, in *invocation) ([]orphanEntry, error) {
	orphans, err := orphansCovered(ctx, in)
	if err != nil {
		return nil, err
	}
	if err := mutate.RemoveOrphans(ctx, in.adapters.Store, orphans); err != nil {
		return nil, err
	}
	out := orphansJSON(orphans)
	printOrphans(in, out, "was an orphan, and is removed")
	return out, nil
}
