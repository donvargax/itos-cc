package features

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/cucumber/godog"
)

// A real syscall's ordinary I/O errors must not turn a bad fixture into an
// unsupported-host omission. Typed capability cases are held by the shared
// classifier's existing table; this holds the actual Link binding.
func TestBinaryLinkPreservesOrdinaryFailures(t *testing.T) {
	d := binary{}
	root := t.TempDir()
	target := filepath.Join(root, "source")
	if err := d.Write(target, []byte("fixture")); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(root, "existing")
	if err := d.Write(existing, []byte("keep")); err != nil {
		t.Fatal(err)
	}

	probe := filepath.Join(root, "supported-link")
	if err := d.Link(target, probe); err != nil {
		if errors.Is(err, godog.ErrSkip) {
			t.Skipf("actual symlink capability unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if err := d.Remove(probe); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"", "1"} {
		t.Setenv(languageToolsEnv, required)
		for _, test := range []struct {
			path  string
			cause error
		}{{existing, os.ErrExist}, {filepath.Join(root, "absent", "link"), os.ErrNotExist}} {
			err := d.Link(target, test.path)
			if !errors.Is(err, test.cause) || errors.Is(err, godog.ErrSkip) {
				t.Fatalf("required%q Link(%s): %v", required, test.path, err)
			}
		}
	}
	data, err := d.Read(existing)
	if err != nil || string(data) != "keep" {
		t.Fatalf("existing destination changed: %q %v", data, err)
	}
	data, err = d.Read(target)
	if err != nil || string(data) != "fixture" {
		t.Fatalf("source changed: %q %v", data, err)
	}
}
