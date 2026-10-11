package mutate

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/donvargax/itos-cc/metrics"
)

// Orphan is a snapshot under .metrics/mutate whose source file is gone:
// nothing is at the path of the file it records (recordedFile), as when
// the file was renamed or deleted and no run moved its snapshot.
type Orphan struct {
	// Path is the source file's path, absolute, where nothing is now: under
	// the root, or the absolute path of a file outside it.
	Path string
	// Snapshot is the snapshot's path from the project root,
	// slash-separated, as .metrics/mutate/<source path>.json.
	Snapshot string
}

// Orphans lists the orphans among the snapshots of the project whose root
// is root, sorted by snapshot, whose source path covers says a command's
// paths cover. A snapshot's source is the file it records (recordedFile):
// one outside the root, which a snapshot names by its absolute path, is
// that path, not the path its name maps to under the root. A file is gone
// when nothing is at its path in the working tree, as mutation run and
// check find files: one deleted there but still committed is gone, and one
// that sits at the path again, whatever it is, is that path's file. Only
// regular files whose names end in .json are snapshots; a staging file of
// a write, or anything else, is never one.
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
		mapped := filepath.Join(root, strings.TrimSuffix(rel, ".json"))
		if present(mapped) {
			return nil
		}
		source := recordedFile(root, rel, mapped)
		if source != mapped && present(source) {
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

// recordedFile is the file the snapshot at rel under .metrics/mutate
// records, absolute: the one its "file" names, when that is a file outside
// the root (recordedPath). Otherwise, a path from the root, a snapshot that
// cannot be read or one that names another file (which LoadSnapshotOf
// takes for none), it is mapped, the path the name maps to under the root.
func recordedFile(root, rel, mapped string) string {
	var s struct {
		File string `json:"file"`
	}
	name := filepath.Join("mutate", rel)
	if ok, err := metrics.ReadIn(metrics.DirOf(root), name, &s); err == nil && ok {
		if path, ok := recordedPath(name, s.File); ok {
			return path
		}
	}
	return mapped
}

// recordedPath is the path of file, as the snapshot named name under
// .metrics records it, when it is absolute, as project.FromRootOf leaves a
// file outside the root, in this platform's form (a drive's letter or a
// share's name included), and name is where that file's snapshot lives
// (SnapshotName).
func recordedPath(name, file string) (string, bool) {
	path := filepath.FromSlash(file)
	if file == "" || !filepath.IsAbs(path) || SnapshotName(file) != name {
		return "", false
	}
	return path, true
}

// present reports whether anything is at path, as Orphans finds files:
// only nothing there at all is gone.
func present(path string) bool {
	_, err := os.Lstat(path)
	return !os.IsNotExist(err)
}

// underDir is path from dir, when path is beneath dir.
func underDir(dir, path string) (string, bool) {
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return rel, true
}
