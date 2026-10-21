package workdir_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
	"github.com/donvargax/itos-cc/internal/workdir"
)

func TestRealSourceStatusContract(t *testing.T) {
	porttest.SourceStatusContract(t, func(t *testing.T) (mutants.SourceStatus, string, string, string) {
		root := t.TempDir()
		file, dir, missing := filepath.Join(root, "file"), filepath.Join(root, "dir"), filepath.Join(root, "missing")
		if err := os.WriteFile(file, []byte("actual"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		return workdir.SourceEdits{}, file, dir, missing
	})
}
func TestSourceStatusPreservesNonMissingStatError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid\x00source.go")
	_, err := os.Stat(path)
	if err == nil || os.IsNotExist(err) {
		t.Fatalf("fixture needs ordinary Stat error: %v", err)
	}
	_, failure := (workdir.SourceEdits{}).Kind(t.Context(), path)
	var failed mutants.InputFailed
	if !errors.As(failure, &failed) || failed.Cause == nil {
		t.Fatalf("ordinary failure: %v", failure)
	}
}
