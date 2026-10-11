package mutate

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/donvargax/itos-cc/metrics"
)

// Orphan is a snapshot under .metrics/mutate whose source file is gone: no
// file is at the path its name gives (SnapshotName), as when the file was
// renamed or deleted and no run moved its snapshot.
type Orphan struct {
	// Path is the source file's path, absolute, where the working tree
	// holds nothing now.
	Path string
	// Snapshot is the snapshot's path from the project root,
	// slash-separated, as .metrics/mutate/<source path>.json.
	Snapshot string
}

// Orphans lists the orphans among the snapshots of the project whose root
// is root, sorted by snapshot, whose source path covers says a command's
// paths cover. A file is gone when nothing is at its path in the working
// tree, as mutation run and check find files: one deleted there but still
// committed is gone, and one that sits at the path again, whatever it is,
// is that path's file. Only regular files whose names end in .json are
// snapshots; a staging file of a write, or anything else, is never one.
func Orphans(root string, covers func(path string) bool) ([]Orphan, error) {
	dir := filepath.Join(metrics.DirOf(root), "mutate")
	if _, err := os.Lstat(dir); os.IsNotExist(err) {
		return nil, nil
	}
	var out []Orphan
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() || !strings.HasSuffix(d.Name(), ".json") || d.Name() == ".json" {
			return nil
		}
		rel, ok := underDir(dir, path)
		if !ok {
			return nil
		}
		source := filepath.Join(root, strings.TrimSuffix(rel, ".json"))
		if _, err := os.Lstat(source); !os.IsNotExist(err) {
			return nil
		}
		if covers(source) {
			out = append(out, Orphan{Path: source, Snapshot: filepath.ToSlash(filepath.Join(metrics.Name, "mutate", rel))})
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Snapshot < out[j].Snapshot })
	return out, err
}

// RemoveOrphans removes the snapshot of each orphan of the project whose
// root is root, then each directory under .metrics/mutate that removing
// them left empty. It removes nothing outside .metrics/mutate.
func RemoveOrphans(root string, orphans []Orphan) error {
	dir := filepath.Join(metrics.DirOf(root), "mutate")
	for _, o := range orphans {
		path := filepath.Join(root, filepath.FromSlash(o.Snapshot))
		if _, ok := underDir(dir, path); !ok {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		for parent := filepath.Dir(path); parent != dir; parent = filepath.Dir(parent) {
			if _, ok := underDir(dir, parent); !ok || os.Remove(parent) != nil {
				break
			}
		}
	}
	return nil
}

// underDir is path from dir, when path is beneath dir.
func underDir(dir, path string) (string, bool) {
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return rel, true
}
