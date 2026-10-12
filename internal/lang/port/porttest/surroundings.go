// Package porttest provides an in-memory fake of lang's Surroundings, so a
// test parses a file without a disk.
package porttest

import (
	"context"
	"path"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang/port"
)

// Surroundings are files and a working directory held in memory. Paths are
// slash-separated and rooted at "/", which stands for an absolute path on
// every system, so a test reads the same on each; the fake gives them back
// with the system's separators, as the disk would.
type Surroundings struct {
	// Files are what Read gives, by absolute path, such as "/work/go.mod".
	// A directory is there when a file is below it.
	Files map[string]string
	// Dir is the working directory, such as "/work"; "" when it cannot be
	// told, as when os.Getwd fails.
	Dir string
}

var _ port.Surroundings = Surroundings{}

// Exists says whether path is a file of Files or a directory above one.
func (s Surroundings) Exists(_ context.Context, p string) bool {
	key := slashed(p)
	if _, ok := s.Files[key]; ok {
		return true
	}
	below := strings.TrimSuffix(key, "/") + "/"
	for f := range s.Files {
		if strings.HasPrefix(f, below) {
			return true
		}
	}
	return false
}

// Read is path's text in Files, false when it has none.
func (s Surroundings) Read(_ context.Context, p string) ([]byte, bool) {
	text, ok := s.Files[slashed(p)]
	if !ok {
		return nil, false
	}
	return []byte(text), true
}

// WorkingDir is Dir, false when it is "".
func (s Surroundings) WorkingDir(context.Context) (string, bool) {
	if s.Dir == "" {
		return "", false
	}
	return filepath.FromSlash(s.Dir), true
}

// Abs is p cleaned when it is absolute, and joined to Dir when it is not;
// false for a relative path with no Dir, as filepath.Abs fails when
// os.Getwd does.
func (s Surroundings) Abs(_ context.Context, p string) (string, bool) {
	q := filepath.ToSlash(p)
	if !strings.HasPrefix(q, "/") && !filepath.IsAbs(p) {
		if s.Dir == "" {
			return "", false
		}
		q = path.Join(s.Dir, q)
	}
	return filepath.FromSlash(path.Clean(q)), true
}

// slashed is p as Files keys it.
func slashed(p string) string { return path.Clean(filepath.ToSlash(p)) }
