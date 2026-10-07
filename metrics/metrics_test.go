package metrics

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestWritesAtTheSameTimeLeaveOneWholeSnapshot(t *testing.T) {
	t.Chdir(t.TempDir())
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := Write("x.json", map[string]string{"text": strings.Repeat(string(rune('a'+i)), 1<<16)}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var got map[string]string
	if ok, err := Read("x.json", &got); !ok || err != nil || len(got["text"]) != 1<<16 {
		t.Fatalf("read %v %v, %d bytes: want one whole snapshot", ok, err, len(got["text"]))
	}
	if left, _ := filepath.Glob(filepath.Join(Dir, "*.tmp")); len(left) > 0 {
		t.Errorf("staging files left behind: %v", left)
	}
	// Windows has no Unix permission bits: Go reports 0666 for a writable file.
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(filepath.Join(Dir, "x.json")); info.Mode().Perm() != 0o644 {
			t.Errorf("mode %v, want 0644", info.Mode().Perm())
		}
	}
}
