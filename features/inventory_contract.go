package features

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"testing"
)

// InventoryContract holds actual recursive state, ordering and common failures.
func InventoryContract(t *testing.T, d Driver) {
	t.Helper()
	dir, err := d.Folder()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Remove(dir) })
	for _, name := range []string{"z.txt", "nested/b.txt", "a.txt"} {
		if err := d.Write(filepath.Join(dir, name), []byte(name)); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"a.txt", "nested/b.txt", "z.txt"}
	got, err := d.Files(t.Context(), dir)
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("files %v, %v; want %v", got, err, want)
	}
	got, err = d.Files(t.Context(), dir+string(filepath.Separator))
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("directory with trailing separator: %v, %v", got, err)
	}
	if err := d.Remove(filepath.Join(dir, "nested/b.txt")); err != nil {
		t.Fatal(err)
	}
	got, err = d.Files(t.Context(), dir)
	if err != nil || !slices.Equal(got, []string{"a.txt", "z.txt"}) {
		t.Fatalf("after removal %v: %v", got, err)
	}
	if _, err := d.Files(t.Context(), filepath.Join(dir, "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.Files(ctx, dir); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
	if _, err := d.Files(ctx, filepath.Join(dir, "missing")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled missing path: %v", err)
	}
}
