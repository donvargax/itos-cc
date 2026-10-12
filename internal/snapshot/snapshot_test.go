package snapshot_test

import (
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/snapshot"
)

// abs is a slash-separated path as this system writes an absolute one
// rooted at its separator.
func abs(path string) string {
	return filepath.FromSlash(path)
}

// A snapshot names a file from the project root, slash-separated, so it is
// the same name from any directory a command runs in and on every system;
// one the root cannot reach keeps its whole path.
func TestKeyNamesAFileFromTheRoot(t *testing.T) {
	root := abs("/work/project")
	for _, c := range []struct {
		name, root, abs, want string
	}{
		{"a file under the root", root, abs("/work/project/src/board.go"), "src/board.go"},
		{"a file at the root", root, abs("/work/project/go.mod"), "go.mod"},
		{"the root itself", root, root, "."},
		{"a name starting with two dots stays under the root", root, abs("/work/project/..notes/a.go"), "..notes/a.go"},
		{"a file beside the root stays absolute", root, abs("/work/other/a.go"), filepath.ToSlash(abs("/work/other/a.go"))},
		{"the root's parent stays absolute", root, abs("/work"), filepath.ToSlash(abs("/work"))},
		{"a root that cannot reach it keeps it whole", "project", abs("/work/project/a.go"), filepath.ToSlash(abs("/work/project/a.go"))},
	} {
		if got := snapshot.Key(c.root, c.abs); got != c.want {
			t.Errorf("%s: Key(%q, %q) = %q, want %q", c.name, c.root, c.abs, got, c.want)
		}
	}
}
