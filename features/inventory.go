package features

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// Files inventories the fixture itself, never linked external directories.
func (binary) Files(ctx context.Context, dir string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var files []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	slices.Sort(files)
	return files, err
}

// Lock keeps an existing marker locked until release closes its descriptor.
func (binary) Lock(ctx context.Context, path string) (func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	if err := lockFixture(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f.Close, nil
}
