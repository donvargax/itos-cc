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

// WalkDir walks the tree at root as filepath.WalkDir does, root first, then
// each directory's entries in lexical order, calling fn under ctx; fn may
// return filepath.SkipDir or filepath.SkipAll as it may there. It hands fn
// the context, so a callback that does I/O takes it first rather than
// capturing it. Once ctx is done it visits nothing more and returns its
// cause.
func WalkDir(ctx context.Context, root string, fn WalkFunc) error {
	info, err := os.Lstat(root)
	if err != nil {
		err = fn(ctx, root, nil, err)
	} else {
		err = walkDir(ctx, root, fs.FileInfoToDirEntry(info), fn)
	}
	if err == filepath.SkipDir || err == filepath.SkipAll {
		return nil
	}
	return err
}

// walkDir visits path, whose entry is d, then what it holds when it is a
// directory fn did not skip.
func walkDir(ctx context.Context, path string, d fs.DirEntry, fn WalkFunc) error {
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	if err := fn(ctx, path, d, nil); err != nil || !d.IsDir() {
		if err == filepath.SkipDir && d.IsDir() {
			return nil
		}
		return err
	}
	entries, err := os.ReadDir(path)
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
		if err := walkDir(ctx, filepath.Join(path, entry.Name()), entry, fn); err != nil {
			if err == filepath.SkipDir {
				break
			}
			return err
		}
	}
	return nil
}
