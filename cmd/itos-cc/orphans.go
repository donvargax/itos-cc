package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/mutate"
	"github.com/donvargax/itos-cc/project"
)

// orphanEntry is an orphaned snapshot in the --json output of mutation run
// and check: "file", the gone source's path as "files" names a file, from
// the working directory, and "snapshot", the snapshot's path from the
// project root, slash-separated.
type orphanEntry struct {
	File     string `json:"file"`
	Snapshot string `json:"snapshot"`
}

// orphansCovered are the snapshots whose source file is gone (mutate.Orphans)
// that the paths of mutation run or check cover: those under a directory
// given, or whose path from the working directory holds a fragment given,
// as a file's must to be chosen. With no paths, whatever --changed or
// --since chooses, they cover the working directory, the whole project
// when run at its root, as a run without paths discovers its files there.
func orphansCovered(in *invocation) ([]mutate.Orphan, error) {
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
	return mutate.Orphans(project.Root(), func(path string) bool {
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
	})
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
		out = append(out, orphanEntry{File: project.Rel(o.Path), Snapshot: o.Snapshot})
	}
	return out
}

// printOrphans prints, in plain output, a line for each orphan:
// "<file>: gone, its snapshot <snapshot> <what>".
func printOrphans(in *invocation, orphans []orphanEntry, what string) {
	if in.json {
		return
	}
	for _, o := range orphans {
		fmt.Printf("%s: gone, its snapshot %s %s\n", o.File, o.Snapshot, what)
	}
}

// removeOrphans removes the orphans the paths of a mutation run cover
// (orphansCovered), printing a line for each in plain output, and returns
// them as the --json output lists them.
func removeOrphans(in *invocation) ([]orphanEntry, error) {
	orphans, err := orphansCovered(in)
	if err != nil {
		return nil, err
	}
	if err := mutate.RemoveOrphans(project.Root(), orphans); err != nil {
		return nil, err
	}
	out := orphansJSON(orphans)
	printOrphans(in, out, "was an orphan, and is removed")
	return out, nil
}
