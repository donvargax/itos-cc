package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/features"
)

func TestMemoryWriteObservation(t *testing.T) { features.WriteObservationContract(t, newMemory(t)) }

func TestMemoryObservesAnIdenticalSnapshotWrite(t *testing.T) {
	m := newMemory(t)
	dir, err := m.Folder()
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"go.mod": "module example.com/board\n\ngo 1.22\n", "board.go": "package board\n\nfunc Place(i int) bool { return i > 5 }\n"} {
		if err := m.Write(filepath.Join(dir, name), []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	r := features.Run{Dir: dir, Args: []string{"dry", "--json"}}
	first, err := m.Run(r)
	if err != nil || first.Exit != 0 {
		t.Fatalf("first dry: %+v, %v", first, err)
	}
	file := filepath.Join(dir, ".metrics", "dry.json")
	before, err := m.Read(file)
	if err != nil {
		t.Fatal(err)
	}
	observe, err := m.ObserveWrites(file)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Run(r)
	if err != nil || second.Exit != 0 {
		t.Fatalf("second dry: %+v, %v", second, err)
	}
	after, err := m.Read(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("fixture snapshot changed bytes:\n%s\n%s", before, after)
	}
	got, err := observe()
	if err != nil || !slices.Equal(got, []string{file}) {
		t.Fatalf("identical command snapshot write %v: %v", got, err)
	}
}

func TestMemoryReadObservationIsExplicitUntilPortsMove(t *testing.T) {
	_, err := newMemory(t).ObserveReads("/outside/marker")
	if !errors.Is(err, features.ErrReadObservationUnsupported) {
		t.Fatalf("unbound observation: %v", err)
	}
}
