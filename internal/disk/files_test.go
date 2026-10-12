package disk

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/internal/counting/port"
	"github.com/donvargax/itos-cc/internal/counting/port/porttest"
)

// Files keep the contract every Files keeps, the fake's included, reading
// a temporary working directory.
func TestFilesKeepTheContract(t *testing.T) {
	porttest.FilesContract(t, func(t *testing.T, files map[string][]byte) (port.Files, string) {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range slices.Sorted(maps.Keys(files)) {
			path := filepath.Join(dir, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, files[name], 0o600); err != nil {
				t.Fatal(err)
			}
		}
		t.Chdir(dir)
		return Files{}, dir
	})
}

// Reading a file leaves it as it was.
func TestFilesReadLeavesTheInputAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.txt")
	want := []byte("read only\n")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (Files{}).Read(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	contents, readErr := os.ReadFile(path)
	if readErr != nil || string(contents) != string(want) {
		t.Errorf("Read() changed the input to %q, %v", contents, readErr)
	}
}

func TestClassifyReadErrorUsesPortFailures(t *testing.T) {
	tests := []struct {
		name        string
		cause       error
		wantMissing bool
	}{
		{name: "missing", cause: fmt.Errorf("open failed: %w", fs.ErrNotExist), wantMissing: true},
		{name: "unreadable", cause: errors.New("read failed")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := classifyReadError("input", test.cause)
			var missing *port.Missing
			var unreadable *port.Unreadable
			if test.wantMissing && !errors.As(err, &missing) {
				t.Errorf("classification %T = %v, want Missing", err, err)
			}
			if test.wantMissing && missing != nil && missing.Path != "input" {
				t.Errorf("missing path = %q, want input", missing.Path)
			}
			if !test.wantMissing && !errors.As(err, &unreadable) {
				t.Errorf("classification %T = %v, want Unreadable", err, err)
			}
			if !test.wantMissing && unreadable != nil && unreadable.Path != "input" {
				t.Errorf("unreadable path = %q, want input", unreadable.Path)
			}
		})
	}
}
