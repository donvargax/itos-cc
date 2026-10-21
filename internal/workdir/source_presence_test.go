package workdir_test

import (
	"errors"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
	"github.com/donvargax/itos-cc/internal/workdir"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestRealSourcePresenceContract(t *testing.T) {
	porttest.SourcePresenceContract(t, func(t *testing.T) (mutants.SourcePresence, string) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "file.go"), []byte("actual bytes"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(root, "directory"), 0700); err != nil {
			t.Fatal(err)
		}
		return workdir.SourceEdits{}, root
	})
}
func TestLstatErrorsArePresentButNotSuccessfulRenameTargets(t *testing.T) {

	path := filepath.Join(t.TempDir(), "invalid\x00path")
	source := workdir.SourceEdits{}
	if _, err := os.Lstat(path); err == nil || os.IsNotExist(err) {
		t.Fatalf("fixture needs a genuine non-not-exist Lstat error: %v", err)
	}
	if got, failure := source.Present(t.Context(), path); failure != nil || !got {
		t.Fatalf("ordinary error cannot prove absence: %v %v", got, failure)
	}
	if got, failure := source.RenameTarget(t.Context(), path); failure != nil || got {
		t.Fatalf("ordinary error cannot prove target: %v %v", got, failure)
	}
}

func TestRealDanglingSourceIsPresentAndARenameTarget(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "dangling")
	if err := os.Symlink(filepath.Join(root, "absent"), link); err != nil {
		if errors.Is(err, errors.ErrUnsupported) || errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.Errno(1314)) {
			t.Skipf("native symlink capability: %v", err)
		}
		t.Fatal(err)
	}
	source := workdir.SourceEdits{}
	if missing, failure := source.Missing(t.Context(), link); failure != nil || !missing {
		t.Fatalf("follow must see missing target: %v %v", missing, failure)
	}
	if got, failure := source.Present(t.Context(), link); failure != nil || !got {
		t.Fatalf("dangling source considered gone: %v %v", got, failure)
	}
	if got, failure := source.RenameTarget(t.Context(), link); failure != nil || !got {
		t.Fatalf("successful dangling Lstat ignored: %v %v", got, failure)
	}
}
