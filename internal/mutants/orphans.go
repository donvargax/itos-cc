package mutants

import (
	"context"
	"errors"
	"path"
	"path/filepath"
	"sort"
	"strings"
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
func Orphans(ctx context.Context, store MutationCatalog, presence SourcePresence, covers, excluded func(path string) bool) ([]Orphan, error) {
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	if store == nil || presence == nil {
		return nil, errors.New("mutation catalog or source presence binding is missing")
	}
	root, failure := store.Root(ctx)
	if failure != nil {
		return nil, failure
	}
	names, failure := store.List(ctx)
	if failure != nil {
		return nil, failure
	}
	var out []Orphan
	for _, name := range names {
		if path.Base(name) == ".json" {
			continue
		}
		rel := filepath.FromSlash(strings.TrimPrefix(name, "mutate/"))
		recorded := path.Join(".metrics", name)
		mapped := filepath.Join(root, strings.TrimSuffix(rel, ".json"))
		exists, err := presence.Present(ctx, mapped)
		if err != nil {
			return nil, err
		}
		if exists {
			if OrphanReason(true, excluded(mapped), covers(mapped)) == OrphanExcluded {
				out = append(out, Orphan{Path: mapped, Snapshot: recorded, Why: OrphanExcluded})
			}
			continue
		}
		source := mapped
		file, metadataFailure := store.RecordedFile(ctx, name)
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		if metadataFailure == nil {
			if recorded, ok := RecordedPath(name, file); ok {
				source = recorded
			}
		}
		if source != mapped {
			exists, err = presence.Present(ctx, source)
			if err != nil {
				return nil, err
			}
		} else {
			exists = false
		}
		if exists {
			if OrphanReason(true, excluded(source), covers(source)) == OrphanExcluded {
				out = append(out, Orphan{Path: source, Snapshot: recorded, Why: OrphanExcluded})
			}
			continue
		}
		if OrphanReason(false, false, covers(source)) == OrphanGone {
			out = append(out, Orphan{Path: source, Snapshot: recorded, Why: OrphanGone})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Snapshot < out[j].Snapshot })
	return out, nil
}

// recordedPath is the path of file, as the snapshot named name under
// .metrics records it, when it is absolute, as project.FromRootOf leaves a
// file outside the root, in this platform's form (a drive's letter or a
// share's name included), and name is where that file's snapshot lives
// (SnapshotName).
func RecordedPath(name, file string) (string, bool) {
	path := filepath.FromSlash(file)
	if file == "" || !filepath.IsAbs(path) || filepath.Join("mutate", filepath.ToSlash(file)+".json") != filepath.FromSlash(name) {
		return "", false
	}
	return path, true
}

// NotRenamed keeps orphans without a successfully observed tracked rename target.
// Ordinary history failures conservatively judge no orphans; cancellation keeps
// its cause. Rename queries remain repository-wide, distinct from check ranges.
func NotRenamed(ctx context.Context, orphans []Orphan, history History, presence SourcePresence, ref string, sinceSet bool) ([]Orphan, error) {
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	if history == nil {
		return nil, errors.New("mutation history binding is missing")
	}
	uncommitted, failure := history.RenamesUncommitted(ctx)
	if failure != nil {
		if cancelled, ok := failure.(HistoryCancelled); ok {
			return nil, cancelled.Cause
		}
		return nil, nil
	}
	var since map[string]string
	if sinceSet {
		since, failure = history.RenamesSince(ctx, ref)
		if failure != nil {
			if cancelled, ok := failure.(HistoryCancelled); ok {
				return nil, cancelled.Cause
			}
			return nil, nil
		}
	}
	var out []Orphan
	for _, o := range orphans {
		if o.Why == OrphanExcluded {
			out = append(out, o)
			continue
		}
		renamed, err := RenamedToAFile(ctx, o.Path, since, uncommitted, presence)
		if err != nil {
			return nil, err
		}
		if !renamed {
			out = append(out, o)
		}
	}
	return out, nil
}

// RenamedToAFile follows since before working-tree edges, observing each target
// only as reached. An error is not successful target evidence; cycles terminate.
func RenamedToAFile(ctx context.Context, source string, since, uncommitted map[string]string, presence SourcePresence) (bool, error) {
	if ctx.Err() != nil {
		return false, context.Cause(ctx)
	}
	if presence == nil {
		return false, errors.New("mutation source presence binding is missing")
	}
	seen := map[string]bool{source: true}
	for {
		next, ok := since[source]
		if !ok {
			if next, ok = uncommitted[source]; !ok {
				return false, nil
			}
		}
		if seen[next] {
			return false, nil
		}
		present, failure := presence.RenameTarget(ctx, next)
		if failure != nil {
			return false, failure
		}
		if present {
			return true, nil
		}
		seen[next], source = true, next
	}
}
