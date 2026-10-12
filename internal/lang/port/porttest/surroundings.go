// Package porttest provides an in-memory fake of lang's Surroundings, so a
// test parses a file without a disk, and the contract every Surroundings,
// fake or adapter, is held to.
package porttest

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang/port"
)

// Surroundings are files and a working directory held in memory. It answers
// as project's and internal/workdir's Surroundings answer from the disk,
// which SurroundingsContract holds: a relative path is from Dir, and every
// path it gives back has the system's separators. A path rooted with no
// volume, such as "/work", is on Dir's volume, so a test may write its
// paths that way and read the same on every system.
type Surroundings struct {
	// Files are what Read gives, by path: absolute, such as "/work/go.mod",
	// or relative to Dir. A directory is there when a file is below it.
	Files map[string]string
	// Dir is the working directory, absolute, such as "/work"; "" when it
	// cannot be told, as when os.Getwd fails, and then no relative path is
	// there.
	Dir string
}

var _ port.Surroundings = Surroundings{}

// Exists says whether path is a file of Files or a directory above one.
func (s Surroundings) Exists(_ context.Context, p string) bool {
	key, ok := s.abs(p)
	if !ok {
		return false
	}
	below := strings.TrimSuffix(key, string(filepath.Separator)) + string(filepath.Separator)
	for f := range s.Files {
		if file, ok := s.abs(f); ok && (file == key || strings.HasPrefix(file, below)) {
			return true
		}
	}
	return false
}

// Read is path's text in Files, false when it has none, as a directory
// has none.
func (s Surroundings) Read(_ context.Context, p string) ([]byte, bool) {
	key, ok := s.abs(p)
	if !ok {
		return nil, false
	}
	for f, text := range s.Files {
		if file, ok := s.abs(f); ok && file == key {
			return []byte(text), true
		}
	}
	return nil, false
}

// WorkingDir is Dir, false when it is "".
func (s Surroundings) WorkingDir(context.Context) (string, bool) {
	if s.Dir == "" {
		return "", false
	}
	return filepath.Clean(filepath.FromSlash(s.Dir)), true
}

// Abs is p cleaned when it is absolute, and joined to Dir when it is not;
// false for a relative path with no Dir, as filepath.Abs fails when
// os.Getwd does.
func (s Surroundings) Abs(_ context.Context, p string) (string, bool) {
	return s.abs(p)
}

// abs is p made absolute against Dir and cleaned, with the system's
// separators. A path rooted with no volume, as Windows writes one, is on
// Dir's volume.
func (s Surroundings) abs(p string) (string, bool) {
	dir := filepath.Clean(filepath.FromSlash(s.Dir))
	switch {
	case filepath.IsAbs(p):
		return filepath.Clean(p), true
	case filepath.VolumeName(p) == "" && strings.HasPrefix(filepath.ToSlash(p), "/"):
		return filepath.Clean(filepath.VolumeName(dir) + filepath.FromSlash(p)), true
	case s.Dir == "":
		return "", false
	default:
		return filepath.Join(dir, p), true
	}
}
