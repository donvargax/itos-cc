package features

import (
	"context"
	"os"
	"time"
)

func (binary) SetModTime(ctx context.Context, path string, when time.Time) (time.Time, error) {
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return time.Time{}, err
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return time.Time{}, &os.PathError{Op: "chtimes", Path: path, Err: os.ErrInvalid}
	}
	if err = os.Chtimes(path, time.Now(), when); err != nil {
		return time.Time{}, err
	}
	info, err = os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}
