package porttest

import (
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

func TestMemoryExceptionsContract(t *testing.T) {
	ExceptionsContract(t, func(t *testing.T, entries []mutants.Exception, missing bool) ExceptionFixture {
		s := &Exceptions{Entries: slices.Clone(entries), Missing: missing}
		return ExceptionFixture{Store: s, Entries: func() []mutants.Exception { return slices.Clone(s.Entries) }, Replace: func(e []mutants.Exception) { s.Entries = slices.Clone(e) }}
	})
}
func TestMemorySourceStatusContract(t *testing.T) {
	SourceStatusContract(t, func(t *testing.T) (mutants.SourceStatus, string, string, string) {
		root := t.TempDir()
		file, dir, missing := filepath.Join(root, "file"), filepath.Join(root, "dir"), filepath.Join(root, "missing")
		return &SourceEdits{Files: map[string]string{file: "actual"}, Directories: map[string]bool{dir: true}}, file, dir, missing
	})
}
func TestMemoryExceptionFailuresDoNotChangeState(t *testing.T) {
	entry := mutants.Exception{File: "a.go"}
	s := &Exceptions{Entries: []mutants.Exception{entry}, Failure: mutants.ExceptionInvalid{Reason: "wrong shape", Key: "mutation"}}
	if _, failure := s.Except(t.Context(), mutants.Exception{File: "b.go"}); failure == nil {
		t.Fatal("invalid accepted")
	}
	if failure := s.Renew(t.Context(), []mutants.Renewal{{Old: entry}}); failure == nil {
		t.Fatal("invalid renewal accepted")
	}
	if s.Writes != 0 || !reflect.DeepEqual(s.Entries, []mutants.Exception{entry}) {
		t.Fatal("failed update wrote")
	}
}
func TestMemoryStatusKeepsFollowFactsSeparate(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "link.go")
	target := filepath.Join(root, "target.go")
	denied := errors.New("following stat denied")
	s := &SourceEdits{Links: map[string]string{path: target}, Files: map[string]string{target: "actual"}, StatErrors: map[string]error{path: denied}}
	if present, failure := s.RenameTarget(t.Context(), path); failure != nil || !present {
		t.Fatalf("link Lstat: %v %v", present, failure)
	}
	if _, failure := s.Kind(t.Context(), path); !errors.Is(failure, denied) {
		t.Fatalf("independent Stat error: %v", failure)
	}
	delete(s.StatErrors, path)
	if kind, failure := s.Kind(t.Context(), path); failure != nil || kind != mutants.SourceNonDirectory {
		t.Fatalf("follow target: %v %v", kind, failure)
	}
	delete(s.Files, target)
	if kind, failure := s.Kind(t.Context(), path); failure != nil || kind != mutants.SourceMissing {
		t.Fatalf("dangling target: %v %v", kind, failure)
	}
}
