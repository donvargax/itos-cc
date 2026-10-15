package features

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestBinaryReadObservation(t *testing.T) {
	d := binary{}
	dir := t.TempDir()
	folder := filepath.Join(dir, "outside")
	file := filepath.Join(folder, "manifest")
	if err := d.Write(file, []byte("owned marker")); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	observe, err := d.ObserveReads(file, folder)
	if errors.Is(err, ErrReadObservationUnsupported) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if got, err := observe(); err != nil || len(got) != 0 {
		t.Fatalf("unread targets %v: %v", got, err)
	}
	if _, err := d.Read(file); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Files(t.Context(), folder); err != nil {
		t.Fatal(err)
	}
	want := []string{file, folder}
	slices.Sort(want)
	if got, err := observe(); err != nil || !slices.Equal(got, want) {
		t.Fatalf("positive read/list %v, want %v: %v", got, want, err)
	}
	after, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("observation changed modification time")
	}
	if err := d.Remove(file); err != nil {
		t.Fatal(err)
	}
	if _, err := observe(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed target must fail, got %v", err)
	}
}

func TestBinaryReadObservationRejectsInvalidTargets(t *testing.T) {
	d := binary{}
	if _, err := d.ObserveReads(); err == nil || errors.Is(err, ErrReadObservationUnsupported) {
		t.Fatalf("empty targets: %v", err)
	}
	if _, err := d.ObserveReads(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing target: %v", err)
	}
}
