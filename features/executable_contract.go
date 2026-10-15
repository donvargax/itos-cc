package features

import (
	"bytes"
	"path/filepath"
	"testing"
)

// ExecutableContract holds actual fixture bytes for a fresh file and an overwrite.
func ExecutableContract(t *testing.T, d Driver) {
	t.Helper()
	dir, err := d.Folder()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Remove(dir) })
	file := filepath.Join(dir, "nested", "runner")
	first := []byte("first executable bytes\n")
	next := []byte("replacement executable bytes\n")
	if err := d.WriteExecutable(file, first); err != nil {
		t.Fatal(err)
	}
	got, err := d.Read(file)
	if err != nil || !bytes.Equal(got, first) {
		t.Fatalf("fresh executable bytes %q %v", got, err)
	}
	if err := d.Write(file, []byte("ordinary")); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteExecutable(file, next); err != nil {
		t.Fatal(err)
	}
	got, err = d.Read(file)
	if err != nil || !bytes.Equal(got, next) {
		t.Fatalf("replacement executable bytes %q %v", got, err)
	}
}
