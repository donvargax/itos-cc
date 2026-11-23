package project

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
)

// WalkFunc is what WalkDir calls for each file and directory, as
// filepath.WalkDir calls its fs.WalkDirFunc, under the walk's context.
type WalkFunc func(ctx context.Context, path string, d fs.DirEntry, err error) error

// FS is the files a walk and a discovery see, by absolute path as the system
// names it: the disk (OS), or a commit's tree read from git's objects. An
// entry's kind is what Lstat tells, so a link is not followed.
type FS interface {
	Lstat(path string) (fs.FileInfo, error)
	// Stat is Lstat, but follows a link; a tree with no links answers as
	// Lstat does.
	Stat(path string) (fs.FileInfo, error)
	ReadDir(path string) ([]fs.DirEntry, error)
	ReadFile(path string) ([]byte, error)
}

// OS is the disk, the FS of every walk but a past commit's.
type OS struct{}

var _ FS = OS{}

func (OS) Lstat(path string) (fs.FileInfo, error)     { return os.Lstat(path) }
func (OS) Stat(path string) (fs.FileInfo, error)      { return os.Stat(path) }
func (OS) ReadDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }
func (OS) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }

// WalkDir walks the tree at root as filepath.WalkDir does, root first, then
// each directory's entries in lexical order, calling fn under ctx; fn may
// return filepath.SkipDir or filepath.SkipAll as it may there. It hands fn
// the context, so a callback that does I/O takes it first rather than
// capturing it. Once ctx is done it visits nothing more and returns its
// cause.
func WalkDir(ctx context.Context, root string, fn WalkFunc) error {
	return WalkDirIn(ctx, OS{}, root, fn)
}

// WalkDirIn is WalkDir over the files of fsys.
func WalkDirIn(ctx context.Context, fsys FS, root string, fn WalkFunc) error {
	info, err := fsys.Lstat(root)
	if err != nil {
		err = fn(ctx, root, nil, err)
	} else {
		err = walkDir(ctx, fsys, root, fs.FileInfoToDirEntry(info), fn)
	}
	if err == filepath.SkipDir || err == filepath.SkipAll {
		return nil
	}
	return err
}

// walkDir visits path, whose entry is d, then what it holds when it is a
// directory fn did not skip.
func walkDir(ctx context.Context, fsys FS, path string, d fs.DirEntry, fn WalkFunc) error {
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	if err := fn(ctx, path, d, nil); err != nil || !d.IsDir() {
		if err == filepath.SkipDir && d.IsDir() {
			return nil
		}
		return err
	}
	entries, err := fsys.ReadDir(path)
	if err != nil {
		// As filepath.WalkDir does, fn hears of the directory again with
		// the error, and its answer decides.
		if err = fn(ctx, path, d, err); err != nil {
			if err == filepath.SkipDir && d.IsDir() {
				return nil
			}
			return err
		}
	}
	for _, entry := range entries {
		if err := walkDir(ctx, fsys, filepath.Join(path, entry.Name()), entry, fn); err != nil {
			if err == filepath.SkipDir {
				break
			}
			return err
		}
	}
	return nil
}
