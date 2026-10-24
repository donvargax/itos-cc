package project

import (
	"context"
	"maps"
	"path/filepath"
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/internal/lang/port"
	"github.com/donvargax/itos-cc/internal/lang/port/porttest"
)

// Surroundings keep the contract every Surroundings keeps, the fake's
// included, answering from the disk of a temporary working directory, git
// above it out of reach.
func TestSurroundingsKeepTheContract(t *testing.T) {
	porttest.SurroundingsContract(t, func(t *testing.T, files map[string]string) (port.Surroundings, string) {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range slices.Sorted(maps.Keys(files)) {
			write(t, filepath.Join(dir, filepath.FromSlash(name)), files[name])
		}
		t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
		t.Chdir(dir)
		return Surroundings{}, dir
	})
}

// A done context reads nothing.
func TestSurroundingsReadNothingOnceTheContextIsDone(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module m\n")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if text, ok := (Surroundings{}).Read(ctx, filepath.Join(dir, "go.mod")); ok || text != nil {
		t.Errorf("Read() = %q, %t; want nothing", text, ok)
	}
}
