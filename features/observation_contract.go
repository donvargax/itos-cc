package features

import (
	"path/filepath"
	"slices"
	"testing"
)

// WriteObservationContract holds both drivers to the same visible writes,
// including an otherwise invisible rewrite of exactly the previous bytes.
func WriteObservationContract(t *testing.T, d Driver) {
	t.Helper()
	dir, err := d.Folder()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Remove(dir) })
	file := filepath.Join(dir, "x.txt")
	if err := d.Write(file, []byte("unchanged\n")); err != nil {
		t.Fatal(err)
	}
	observe, err := d.ObserveWrites(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Read(file); err != nil {
		t.Fatal(err)
	}
	if got, err := observe(); err != nil || len(got) != 0 {
		t.Fatalf("read reported writes %v: %v", got, err)
	}
	if err := d.Write(file, []byte("unchanged\n")); err != nil {
		t.Fatal(err)
	}
	if got, err := observe(); err != nil || !slices.Equal(got, []string{file}) {
		t.Fatalf("identical rewrite reported %v: %v", got, err)
	}
	observe, err = d.ObserveWrites(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Remove(file); err != nil {
		t.Fatal(err)
	}
	if got, err := observe(); err != nil || !slices.Equal(got, []string{file}) {
		t.Fatalf("removal reported %v: %v", got, err)
	}
}
