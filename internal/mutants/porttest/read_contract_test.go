package porttest

import (
	"errors"
	"github.com/donvargax/itos-cc/internal/mutants"
	"io/fs"
	"path/filepath"
	"testing"
)

func TestMemoryMutationCatalogContract(t *testing.T) {
	MutationCatalogContract(t, func(t *testing.T, files map[string]string) (mutants.MutationCatalog, string) {
		root := t.TempDir()
		return &Snapshots{RootDir: root, CatalogFiles: files}, root
	})
}
func TestMemorySourcePresenceContract(t *testing.T) {
	SourcePresenceContract(t, func(t *testing.T) (mutants.SourcePresence, string) {
		root := t.TempDir()
		return &SourceEdits{Files: map[string]string{filepath.Join(root, "file.go"): "actual bytes"}, Directories: map[string]bool{filepath.Join(root, "directory"): true}}, root
	})
}
func TestMemoryLstatFactsAreIndependentFromFollowStat(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "dangling")
	source := &SourceEdits{Links: map[string]string{link: "missing-target"}, StatErrors: map[string]error{link: fs.ErrNotExist}}
	if missing, failure := source.Missing(t.Context(), link); failure != nil || !missing {
		t.Fatalf("follow missing: %v %v", missing, failure)
	}
	for _, observe := range []func() (bool, mutants.InputFailure){func() (bool, mutants.InputFailure) { return source.Present(t.Context(), link) }, func() (bool, mutants.InputFailure) { return source.RenameTarget(t.Context(), link) }} {
		if got, failure := observe(); failure != nil || !got {
			t.Fatalf("dangling no-follow fact: %v %v", got, failure)
		}
	}
	denied := filepath.Join(root, "denied")
	source.LstatErrors = map[string]error{denied: errors.New("independent permission failure")}
	if got, failure := source.Present(t.Context(), denied); failure != nil || !got {
		t.Fatalf("error cannot prove gone: %v %v", got, failure)
	}
	if got, failure := source.RenameTarget(t.Context(), denied); failure != nil || got {
		t.Fatalf("error cannot prove target: %v %v", got, failure)
	}
	source.LstatErrors[denied] = fs.ErrNotExist
	if got, failure := source.Present(t.Context(), denied); failure != nil || got {
		t.Fatalf("not-exist: %v %v", got, failure)
	}
}
