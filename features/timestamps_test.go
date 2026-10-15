package features

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBinaryTimestampObservedEntryAndCancellation(t *testing.T) {
	d := binary{}
	root := t.TempDir()
	file := filepath.Join(root, "marker")
	if err := d.Write(file, []byte("unchanged")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{root, file} {
		want := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
		got, err := d.SetModTime(t.Context(), path, want)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !got.Equal(info.ModTime()) || got.Sub(want).Abs() > time.Second {
			t.Fatalf("observed %v stat %v want %v", got, info.ModTime(), want)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	before, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.SetModTime(ctx, file, time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled existing: %v", err)
	}
	after, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("cancelled call changed entry")
	}
	if _, err = d.SetModTime(ctx, filepath.Join(root, "missing"), time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation priority: %v", err)
	}
	if _, err = d.SetModTime(t.Context(), filepath.Join(root, "missing"), time.Now()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing cause: %v", err)
	}
	data, err := d.Read(file)
	if err != nil || string(data) != "unchanged" {
		t.Fatalf("fixture bytes %q: %v", data, err)
	}
}
