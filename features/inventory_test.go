package features

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/cucumber/godog"
)

func TestBinaryInventory(t *testing.T) { InventoryContract(t, binary{}) }
func TestBinaryInventoryDoesNotFollowDirectoryLinks(t *testing.T) {
	d := binary{}
	dir := t.TempDir()
	outside := t.TempDir()
	if err := d.Write(filepath.Join(outside, ".coverage.leak"), []byte("outside")); err != nil {
		t.Fatal(err)
	}
	if err := d.Link(outside, filepath.Join(dir, "linked")); err != nil {
		if errors.Is(err, godog.ErrSkip) {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	files, err := d.Files(t.Context(), dir)
	if err != nil || !slices.Equal(files, []string{}) {
		t.Fatalf("inventory crossed directory link: %v, %v", files, err)
	}
}

// Lock ownership is a driver boundary contract, independent of which
// mutation scenarios a particular CI selection executes.
func TestBinaryLockOwnership(t *testing.T) {
	d := binary{}
	file := filepath.Join(t.TempDir(), "owner.lock")
	marker := []byte("itos-cc pid 1\n")
	if err := d.Write(file, marker); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if release, err := d.Lock(ctx, file); !errors.Is(err, context.Canceled) {
		if release != nil {
			_ = release()
		}
		t.Fatalf("cancelled lock: %v", err)
	}
	release, err := d.Lock(t.Context(), file)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if release != nil {
			_ = release()
		}
	})
	other, err := d.Lock(t.Context(), file)
	if err == nil {
		_ = other()
		t.Fatal("a second descriptor acquired the held lock")
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	release = nil
	release, err = d.Lock(t.Context(), file)
	if err != nil {
		t.Fatalf("released lock cannot be reacquired: %v", err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	release = nil
	got, err := d.Read(file)
	if err != nil || string(got) != string(marker) {
		t.Fatalf("marker changed: %q, %v", got, err)
	}
}
