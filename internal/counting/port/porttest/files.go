// Package porttest provides in-memory fakes for counting-domain tests, and
// the contract every Files, fake or adapter, is held to.
package porttest

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/internal/counting/port"
)

// Files holds test inputs and configured read failures in memory. It reads
// as internal/disk's Files reads the disk, which FilesContract holds.
type Files struct {
	// Dir is the working directory, absolute: a relative path, given to
	// Read or keying Data, is from it. A path rooted with no volume, such
	// as "/work", is on Dir's volume, so a test reads the same on every
	// system. With no Dir, paths are compared as given, cleaned.
	Dir string
	// Data are the files' bytes, by path, absolute or relative to Dir. A
	// directory is there when a file is below it.
	Data map[string][]byte
	// Failures are Read's, by path as given: a read of a path here fails
	// with its failure.
	Failures map[string]port.ReadFailure
}

var _ port.Files = Files{}

// Read returns a copy of the file at path, its configured failure, Missing
// when nothing is there, or Unreadable for a directory, as the disk cannot
// read one. It does not act on ctx, since nothing here waits: a test that
// wants a cancelled read configures port.Cancelled as the path's failure.
func (f Files) Read(ctx context.Context, path string) ([]byte, port.ReadFailure) {
	if failure, ok := f.Failures[path]; ok {
		return nil, failure
	}
	key := f.abs(path)
	below := strings.TrimSuffix(key, string(filepath.Separator)) + string(filepath.Separator)
	directory := false
	for name, data := range f.Data {
		file := f.abs(name)
		if file == key {
			return append([]byte(nil), data...), nil
		}
		directory = directory || strings.HasPrefix(file, below)
	}
	if directory {
		return nil, &port.Unreadable{Path: path}
	}
	return nil, &port.Missing{Path: path}
}

// abs is path made absolute against Dir and cleaned. A path rooted with no
// volume, as Windows writes one, is on Dir's volume.
func (f Files) abs(path string) string {
	switch {
	case filepath.IsAbs(path):
		return filepath.Clean(path)
	case filepath.VolumeName(path) == "" && strings.HasPrefix(filepath.ToSlash(path), "/"):
		return filepath.Clean(filepath.VolumeName(f.Dir) + filepath.FromSlash(path))
	default:
		return filepath.Join(f.Dir, path)
	}
}

// Unreadable returns a configured unreadable-file failure.
func Unreadable(path string) port.ReadFailure { return &port.Unreadable{Path: path} }
