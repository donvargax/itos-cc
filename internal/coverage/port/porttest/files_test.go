package porttest

import (
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage/port"
)

// The fake keeps the contract every Files keeps, the disk's included. Its
// directory is a name only: the fake never touches it, and a temporary
// directory's is absolute on every system.
func TestFilesKeepTheContract(t *testing.T) {
	FilesContract(t, func(t *testing.T, files []string) (port.Files, string) {
		dir := t.TempDir()
		return Files{Dir: dir, Regular: files}, dir
	})
}

// A link reaches the files below it by another path, as a symlinked
// directory does, a link through a link too, and a loop of links ends; a
// path rooted with no volume is on the volume of Dir.
func TestSameFileFollowsLinks(t *testing.T) {
	files := Files{Dir: "/work", Regular: []string{"/real/lib.py"}, Links: map[string]string{
		"/work/link": "/real", "/work/twice": "link", "/work/loop": "/work/pool", "/work/pool": "/work/loop",
	}}
	for path, want := range map[string]string{"/real/lib.py": "link/lib.py", "link/lib.py": "link/lib.py", "/work/twice/lib.py": "link/lib.py"} {
		if same, regular := files.SameFile(t.Context(), path, []string{"/real/other.py", "link/lib.py"}); same != want || !regular {
			t.Errorf("SameFile(%q) = %q, %t; want %q, true", path, same, regular, want)
		}
	}
	if same, regular := files.SameFile(t.Context(), "loop/lib.py", []string{"link/lib.py"}); same != "" || regular {
		t.Errorf("SameFile(loop/lib.py) = %q, %t; want no regular file", same, regular)
	}
	if same, regular := files.SameFile(t.Context(), "/link/lib.py", []string{"/work/link/lib.py"}); same != "" || regular {
		t.Errorf("SameFile(/link/lib.py) = %q, %t; want no regular file: the link is under /work", same, regular)
	}
}
