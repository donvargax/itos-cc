package covrun

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/coverage/port"
	"github.com/donvargax/itos-cc/internal/coverage/port/porttest"
)

// Disk keeps the contract every Files keeps, the fake's included, over a
// temporary working directory.
func TestDiskKeepsTheContract(t *testing.T) {
	porttest.FilesContract(t, func(t *testing.T, files []string) (port.Files, string) {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		writeFiles(t, dir, func() map[string]string {
			out := map[string]string{}
			for _, name := range files {
				out[filepath.FromSlash(name)] = ""
			}
			return out
		}())
		t.Chdir(dir)
		return Disk{}, dir
	})
}

// A report that resolved a symlinked directory names the source reached
// through it, as os.SameFile tells.
func TestDiskFollowsSymlinkedDirectories(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	writeFiles(t, real, map[string]string{"lib.py": ""})
	src := filepath.Join(link, "lib.py")
	if same, regular := (Disk{}).SameFile(t.Context(), filepath.Join(real, "lib.py"), []string{src}); same != src || !regular {
		t.Errorf("SameFile = %q, %t; want %s through the link", same, regular, src)
	}
	entries := []coverage.Entry{{Path: filepath.Join(real, "lib.py"), Segments: []coverage.Segment{{Start: 1, End: 1, Total: 1, Covered: 1}}}}
	if r := coverage.Build(t.Context(), Disk{}, []string{src}, link, entries); !r.Has(src) {
		t.Error("a report that resolved the symlink still names the source")
	}
}

// Load reads a report in the format its content has, and fails as the
// open fails.
func TestLoadReadsAReportInItsFormat(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"go.out": "mode: set\nexample.com/m/a.go:1.1,2.2 1 1\n", "lcov.info": "SF:a.ts\nDA:1,1\nend_of_record\n"})
	for name, want := range map[string]string{"go.out": "example.com/m/a.go", "lcov.info": "a.ts"} {
		if entries, err := Load(filepath.Join(dir, name)); err != nil || len(entries) != 1 || entries[0].Path != want {
			t.Errorf("Load(%s) = %+v, %v; want one entry for %s", name, entries, err, want)
		}
	}
	if _, err := Load(filepath.Join(dir, "absent.info")); !os.IsNotExist(err) {
		t.Errorf("Load(absent.info) = %v, want the open's not-exist error", err)
	}
}
