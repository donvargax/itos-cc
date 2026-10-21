package worktrees

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/donvargax/itos-cc/project"
)

// Each run in a worker starts from its copy as it was made (slice-13,
// issue #35): before a command runs in a copy that an earlier command ran
// in, the worker removes what the runs added, puts back what they changed
// and makes again what they removed, so a file one run of the tests leaves
// cannot fail every later run. It restores rather than copying afresh,
// which would cost a whole copy, and Dart's pub get, for every mutant.
//
// What the copy leaves out stays as the runs leave it: skipDirs, build
// output and Dart's build/ and coverage/, so incremental builds and
// .dart_tool, Ready's resolution, persist. The linked dependency folders
// are the project's own and shared: the restore never walks into them, and
// makes a link a run removed again.

// copied is what a copy's record holds of one of its entries, as made: its
// type and permissions, a file's size and modification time, a link's
// target. ready says Ready wrote the file, which is then copied again from
// the worker's keep of it, never from the project.
type copied struct {
	mode  fs.FileMode
	size  int64
	mtime time.Time
	link  string
	ready bool
}

// workerCopy is a worker's copy of one tree: its folder, dir, the tree it
// copies, kept, where it keeps the files Ready wrote, and the record of
// each entry it held once made and readied, by its path from dir.
type workerCopy struct {
	dir, tree, kept string
	record          map[string]copied
}

// leftOut says whether a copy leaves out the folder dir, as copyTree
// does, but for build output git ignores: a copy has no repository to ask.
func leftOut(ctx context.Context, dir string) bool {
	return skipDirs[filepath.Base(dir)] || dartLeftOut(dir) || project.IsBuildOutputWithIgnore(ctx, dir, nil)
}

// recordOf is the record of the copy at dir as it is now: each folder,
// file and link it holds, by its path from dir, leaving out what leftOut
// leaves out and never walking into a link.
func recordOf(ctx context.Context, dir string) (map[string]copied, error) {
	record := map[string]copied{}
	err := project.WalkDir(ctx, dir, func(ctx context.Context, path string, d fs.DirEntry, err error) error {
		if err != nil || path == dir {
			return err
		}
		if d.IsDir() && leftOut(ctx, path) {
			return filepath.SkipDir
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		entry, ok, err := entryOf(path, info)
		if ok {
			rel, _ := filepath.Rel(dir, path)
			record[rel] = entry
		}
		return err
	})
	return record, err
}

// entryOf is what a record holds of the entry at path, whose information
// is info; ok is false for anything but a folder, a file or a link, which
// no copy makes.
func entryOf(path string, info fs.FileInfo) (copied, bool, error) {
	e := copied{mode: info.Mode()}
	switch {
	case info.IsDir():
	case info.Mode()&fs.ModeSymlink != 0:
		link, err := os.Readlink(path)
		e.link = link
		return e, err == nil, err
	case info.Mode().IsRegular():
		e.size, e.mtime = info.Size(), info.ModTime()
	default:
		return e, false, nil
	}
	return e, true, nil
}

// holds says whether the entry at path, whose information is info, is
// still the one recorded: the same type, a link to the same target, or a
// file of the same mode, size and modification time, as rsync's quick
// check decides. A folder's permissions are put back where it stands.
func (e copied) holds(path string, info fs.FileInfo) bool {
	switch {
	case info.Mode().Type() != e.mode.Type():
		return false
	case e.mode.IsDir():
		return true
	case e.mode&fs.ModeSymlink != 0:
		link, err := os.Readlink(path)
		return err == nil && link == e.link
	}
	return info.Mode() == e.mode && info.Size() == e.size && info.ModTime().Equal(e.mtime)
}

// same says whether e and o record a file alike.
func (e copied) same(o copied) bool {
	return e.mode == o.mode && e.size == o.size && e.mtime.Equal(o.mtime) && e.link == o.link
}

// restore puts the copy back as its record has it, under ctx: it removes
// what is not recorded, outside the folders the copy leaves out, puts back
// a folder's permissions, and makes again what is missing or changed, a
// changed file copied again from where it came from. A file it copies
// again is recorded anew, so the record follows its new modification time.
func (c *workerCopy) restore(ctx context.Context) error {
	seen := make(map[string]bool, len(c.record))
	err := project.WalkDir(ctx, c.dir, func(ctx context.Context, path string, d fs.DirEntry, err error) error {
		if err != nil || path == c.dir {
			return err
		}
		rel, _ := filepath.Rel(c.dir, path)
		want, known := c.record[rel]
		if !known && d.IsDir() && leftOut(ctx, path) {
			return filepath.SkipDir
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if known && want.holds(path, info) {
			seen[rel] = true
			if info.Mode() != want.mode {
				// Before the walk reads the folder, so one a run made
				// unreadable is read.
				return os.Chmod(path, want.mode)
			}
			return nil
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		if d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return err
	}
	return c.remake(ctx, seen)
}

// remake makes again each recorded entry the walk did not see, folders
// before what they hold.
func (c *workerCopy) remake(ctx context.Context, seen map[string]bool) error {
	var missing []string
	for rel := range c.record {
		if !seen[rel] {
			missing = append(missing, rel)
		}
	}
	slices.Sort(missing)
	for _, rel := range missing {
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		want, path := c.record[rel], filepath.Join(c.dir, rel)
		switch {
		case want.mode.IsDir():
			if err := os.Mkdir(path, want.mode.Perm()); err != nil {
				return err
			}
			if err := os.Chmod(path, want.mode); err != nil {
				return err
			}
		case want.mode&fs.ModeSymlink != 0:
			if err := os.Symlink(want.link, path); err != nil {
				return err
			}
		default:
			from := filepath.Join(c.tree, rel)
			if want.ready {
				from = filepath.Join(c.kept, rel)
			}
			if err := copyFile(from, path); err != nil {
				return fmt.Errorf("copy %s again: %w", rel, err)
			}
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			c.record[rel] = copied{mode: info.Mode(), size: info.Size(), mtime: info.ModTime(), ready: want.ready}
		}
	}
	return nil
}

// keepReady copies to kept each file made records that before, the record
// of the copy at dir before Ready, did not record alike, and marks it
// ready in made: those Ready wrote, which no project file gives back.
func keepReady(dir, kept string, before, made map[string]copied) error {
	for rel, e := range made {
		if !e.mode.IsRegular() {
			continue
		}
		if b, ok := before[rel]; ok && b.same(e) {
			continue
		}
		to := filepath.Join(kept, rel)
		if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
			return err
		}
		if err := copyFile(filepath.Join(dir, rel), to); err != nil {
			return err
		}
		e.ready = true
		made[rel] = e
	}
	return nil
}

// discard removes the copy and its keep as far as it can, first making
// each folder in them writable and readable, as a run may have left one
// that is not; a later command copies afresh.
func (c *workerCopy) discard() {
	for _, dir := range []string{c.dir, c.kept} {
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				_ = os.Chmod(path, 0o700) //nolint:gosec // a folder of the owner's private scratch, made traversable to be removed
			}
			return nil
		})
		_ = os.RemoveAll(dir)
	}
}

// fresh makes the worker's copy of c.Root ready for one of c's commands:
// the copy as made, before any command ran in it. A copy's first use makes
// it; any later one restores it, and a copy it cannot restore is discarded
// and made afresh, with Ready. The command's timeout has not started.
func (w *worker) fresh(ctx context.Context, c Command) error {
	tree, _ := pubWorkspace(c.Root)
	cp, ok := w.copies[tree]
	if !ok {
		_, err := w.copyOf(ctx, c)
		return err
	}
	err := cp.restore(ctx)
	if w.restored != nil {
		w.restored()
	}
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("restore the worker's copy: %w", ctx.Err())
	}
	cp.discard()
	delete(w.copies, tree)
	if _, again := w.copyOf(ctx, c); again != nil {
		return fmt.Errorf("restore the worker's copy: %v; copy it afresh: %w", err, again)
	}
	return nil
}
