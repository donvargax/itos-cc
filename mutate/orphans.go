package mutate

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/donvargax/itos-cc/internal/snapshot"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// Why a snapshot is an orphan.
const (
	OrphanGone     = "gone"     // nothing is at its source's path
	OrphanExcluded = "excluded" // its source is there, and itos-cc.yaml excludes it
)

// Orphan is a snapshot under .metrics/mutate whose source file is gone:
// nothing is at the path of the file it records (recordedFile), as when
// the file was renamed or deleted and no run moved its snapshot; or whose
// source is there but excluded, which no command measures any more.
type Orphan struct {
	// Path is the source file's path, absolute, where nothing is now: under
	// the root, or the absolute path of a file outside it.
	Path string
	// Snapshot is the snapshot's path from the project root,
	// slash-separated, as .metrics/mutate/<source path>.json.
	Snapshot string
	// Why is OrphanGone or OrphanExcluded.
	Why string
}

// Orphans lists the orphans among the snapshots store holds under mutate,
// of the project at its root, sorted by snapshot, whose source path covers says a command's
// paths cover: those whose source is gone, and those whose source is there
// and excluded says is left out of what is measured, an absolute path as
// covers takes. A snapshot's source is the file it records (recordedFile):
// one outside the root, which a snapshot names by its absolute path, is
// that path, not the path its name maps to under the root. A file is gone
// when nothing is at its path in the working tree, as mutation run and
// check find files: one deleted there but still committed is gone, and one
// that sits at the path again, whatever it is, is that path's file. Only
// regular files whose names end in .json are snapshots (store's List); a
// staging file of a write, or anything else, is never one, and neither is
// one named .json alone.
func Orphans(ctx context.Context, store snapport.Store, covers, excluded func(path string) bool) ([]Orphan, error) {
	root, failure := store.Root(ctx)
	if failure != nil {
		return nil, failure
	}
	names, failure := store.List(ctx, "mutate")
	if failure != nil {
		return nil, failure
	}
	var out []Orphan
	for _, name := range names {
		if path.Base(name) == ".json" {
			continue
		}
		rel := filepath.FromSlash(strings.TrimPrefix(name, "mutate/"))
		recorded := path.Join(snapshot.Folder, name)
		mapped := filepath.Join(root, strings.TrimSuffix(rel, ".json"))
		if present(mapped) {
			if excluded(mapped) && covers(mapped) {
				out = append(out, Orphan{Path: mapped, Snapshot: recorded, Why: OrphanExcluded})
			}
			continue
		}
		source := recordedFile(ctx, store, rel, mapped)
		if source != mapped && present(source) {
			if excluded(source) && covers(source) {
				out = append(out, Orphan{Path: source, Snapshot: recorded, Why: OrphanExcluded})
			}
			continue
		}
		if covers(source) {
			out = append(out, Orphan{Path: source, Snapshot: recorded, Why: OrphanGone})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Snapshot < out[j].Snapshot })
	return out, nil
}

// RemoveOrphans removes the snapshot of each orphan from store, and each
// folder under .metrics/mutate that removing them left empty (store's
// Remove). It removes nothing outside .metrics/mutate, whatever an Orphan
// names.
func RemoveOrphans(ctx context.Context, store snapport.Store, orphans []Orphan) error {
	for _, o := range orphans {
		name, ok := strings.CutPrefix(o.Snapshot, snapshot.Folder+"/mutate/")
		if !ok || !filepath.IsLocal(filepath.FromSlash(name)) {
			continue
		}
		if failure := store.Remove(ctx, path.Join("mutate", name)); failure != nil {
			return failure
		}
	}
	return nil
}

// recordedFile is the file the snapshot at rel under .metrics/mutate, in
// store, records, absolute: the one its "file" names, when that is a file outside
// the root (recordedPath). Otherwise, a path from the root, a snapshot that
// cannot be read or one that names another file (which LoadSnapshotOf
// takes for none), it is mapped, the path the name maps to under the root.
func recordedFile(ctx context.Context, store snapport.Store, rel, mapped string) string {
	var s struct {
		File string `json:"file"`
	}
	name := filepath.Join("mutate", rel)
	if ok, failure := store.Read(ctx, name, &s); failure == nil && ok {
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
