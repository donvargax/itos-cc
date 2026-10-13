// Package porttest provides in-memory fakes for coverage-domain tests, and
// the contract every Files, fake or adapter, is held to.
package porttest

import (
	"context"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/coverage/port"
)

// Files holds which paths are regular files, and the links that reach one
// path through another, in memory. It answers as the runner's Disk answers
// from the disk, which FilesContract holds.
type Files struct {
	// Dir is the working directory, absolute: a relative path, given to
	// SameFile or keying Regular and Links, is from it. A path rooted with
	// no volume, such as "/work", is on Dir's volume, so a test reads the
	// same on every system.
	Dir string
	// Regular are the regular files, by path, absolute or relative to Dir.
	// Any other path, a directory above one of them included, is none.
	Regular []string
	// Links reach the path each names another way, as a symlink does: a
	// path under the key is the same path under its value. Keys and values
	// are absolute or relative to Dir.
	Links map[string]string
}

var _ port.Files = Files{}

// SameFile says whether path, followed through Links, is one of Regular,
// and which of among, followed the same way, is the same file: the first
// that is, or "" when none is. It does not act on ctx, as the port says.
func (f Files) SameFile(_ context.Context, path string, among []string) (string, bool) {
	target := f.resolve(path)
	regular := false
	for _, r := range f.Regular {
		regular = regular || f.resolve(r) == target
	}
	if !regular {
		return "", false
	}
	for _, candidate := range among {
		if f.resolve(candidate) == target {
			return candidate, true
		}
	}
	return "", true
}

// resolve is path made absolute and clean, then followed through Links,
// in their keys' order, until none applies, as many times as there are
// links at most, so a loop of links ends.
func (f Files) resolve(path string) string {
	path = f.abs(path)
	links := slices.Sorted(maps.Keys(f.Links))
	for range links {
		followed := false
		for _, link := range links {
			from := f.abs(link)
			if path == from || strings.HasPrefix(path, from+string(filepath.Separator)) {
				path = filepath.Join(f.abs(f.Links[link]), strings.TrimPrefix(path, from))
				followed = true
				break
			}
		}
		if !followed {
			break
		}
	}
	return path
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
