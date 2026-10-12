// Package snapshot owns the snapshots under .metrics/ that the commands
// share and that projects commit: the format version every snapshot
// records, and how a snapshot names a file, from the project root, so the
// same file has the same name wherever a command runs. It reads and writes
// snapshots only through its port (decision 41).
package snapshot

import (
	"path/filepath"
	"strings"
)

// Version is the snapshot format version every snapshot records.
const Version = 1

// Key is abs, an absolute path, as a snapshot records it: relative to root,
// the project root, and slash-separated. A path outside root, or one root
// cannot be made relative to, stays absolute, slash-separated. The paths a
// selection discovers are absolute already, so naming one needs no working
// directory.
func Key(root, abs string) string {
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(abs)
	}
	return filepath.ToSlash(rel)
}
