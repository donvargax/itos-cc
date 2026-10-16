package porttest

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/donvargax/itos-cc/internal/mutants"
)

// SourceEdits holds actual current bytes, distinct from a parsed planning copy.
// Directories and conservative stat errors are present for rename following.
type SourceEdits struct {
	Files       map[string]string
	Directories map[string]bool
	StatErrors  map[string]error
	// Links and LstatErrors are independent no-follow observations.
	Links                     map[string]string
	LstatErrors               map[string]error
	PresentPaths, TargetPaths []string
	Writes                    []string
	Write                     func(path string, data []byte) error
}

func (s *SourceEdits) Missing(ctx context.Context, path string) (bool, mutants.InputFailure) {
	if ctx.Err() != nil {
		return false, mutants.InputCancelled{Cause: context.Cause(ctx)}
	}
	path = filepath.Clean(path)
	if err := s.StatErrors[path]; err != nil {
		return errors.Is(err, fs.ErrNotExist), nil
	}
	if s.Directories[path] {
		return false, nil
	}
	_, ok := s.Files[path]
	return !ok, nil
}
func (s *SourceEdits) Annotate(ctx context.Context, path, comment, block string) mutants.InputFailure {
	if ctx.Err() != nil {
		return mutants.InputCancelled{Cause: context.Cause(ctx)}
	}
	path = filepath.Clean(path)
	text, ok := s.Files[path]
	if !ok {
		return mutants.InputFailed{Cause: fmt.Errorf("annotation source is absent: %s", path)}
	}
	updated := mutants.Annotated(text, comment, block)
	if updated == text {
		return nil
	}
	if s.Write != nil {
		if err := s.Write(path, []byte(updated)); err != nil {
			return mutants.InputFailed{Cause: err}
		}
	}
	s.Files[path] = updated
	s.Writes = append(s.Writes, path)
	return nil
}

func (s *SourceEdits) Present(ctx context.Context, path string) (bool, mutants.InputFailure) {
	if ctx.Err() != nil {
		return false, mutants.InputCancelled{Cause: context.Cause(ctx)}
	}
	path = filepath.Clean(path)
	s.PresentPaths = append(s.PresentPaths, path)
	if err := s.LstatErrors[path]; err != nil {
		return !errors.Is(err, fs.ErrNotExist), nil
	}
	_, file := s.Files[path]
	_, link := s.Links[path]
	return file || link || s.Directories[path], nil
}

func (s *SourceEdits) RenameTarget(ctx context.Context, path string) (bool, mutants.InputFailure) {
	if ctx.Err() != nil {
		return false, mutants.InputCancelled{Cause: context.Cause(ctx)}
	}
	path = filepath.Clean(path)
	s.TargetPaths = append(s.TargetPaths, path)
	if s.LstatErrors[path] != nil {
		return false, nil
	}
	_, file := s.Files[path]
	_, link := s.Links[path]
	return file || link || s.Directories[path], nil
}
