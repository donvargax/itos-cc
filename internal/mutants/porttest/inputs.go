package porttest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/mutants"
)

// Inputs is the actual virtual project shared by the observation fakes. Probes
// are independent setup facts, never inferred from Workers' declared commands.
// Reads/Lists record actual port operations; fixture assertion reads do not.
type Inputs struct {
	Dir   string
	Files map[string]string
	Dirs  map[string]bool
	Links map[string]string
	// Tracked are the files git tracks, by absolute path; nil outside a
	// repository, where every file on the disk is a candidate input.
	Tracked      map[string]bool
	Runtime      mutants.RuntimeInputs
	Without      []string
	Probes       map[PytestProbe]bool
	Imports      map[string][]string
	Reads, Lists []string
}

// PytestProbe includes interpreter identity and the caller cwd, not test root.
// Environment holds the actual Python probe environment inputs in fixed order.
type PytestProbe struct{ Interpreter, Dir, Environment string }

func ProbeKey(interpreter, dir string, env map[string]string) PytestProbe {
	var values []string
	for _, name := range []string{"PYTHONPATH", "PYTHONHOME", "PYTHONSAFEPATH", "VIRTUAL_ENV", "PYTEST_DISABLE_PLUGIN_AUTOLOAD"} {
		values = append(values, name+"="+env[name])
	}
	return PytestProbe{Interpreter: interpreter, Dir: filepath.Clean(dir), Environment: strings.Join(values, "\x00")}
}

func inputStopped(ctx context.Context) mutants.InputFailure {
	if cause := context.Cause(ctx); cause != nil {
		return mutants.InputCancelled{Cause: cause}
	}
	return nil
}
func inputError(err error) mutants.InputFailure { return mutants.InputFailed{Cause: err} }
func inputHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func (s *Inputs) absolute(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(s.Dir, filepath.FromSlash(path))
}
func (s *Inputs) Abs(ctx context.Context, path string) (string, bool) {
	return s.absolute(path), ctx.Err() == nil
}
func (s *Inputs) WorkingDir(ctx context.Context) (string, bool) {
	return s.Dir, s.Dir != "" && ctx.Err() == nil
}
func (s *Inputs) Exists(ctx context.Context, path string) bool {
	if ctx.Err() != nil {
		return false
	}
	path = s.absolute(path)
	if _, ok := s.Files[path]; ok || s.Dirs[path] {
		return true
	}
	for name := range s.Files {
		if inside(path, name) {
			return true
		}
	}
	return false
}
func (s *Inputs) Read(ctx context.Context, path string) ([]byte, bool) {
	if ctx.Err() != nil {
		return nil, false
	}
	path = s.absolute(path)
	text, ok := s.Files[path]
	if ok {
		s.Reads = append(s.Reads, path)
	}
	return []byte(text), ok
}
func (s *Inputs) read(ctx context.Context, path string) string {
	data, _ := s.Read(ctx, path)
	return string(data)
}
func inside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
func relative(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}
func (s *Inputs) paths(root string) []string {
	root = s.absolute(root)
	s.Lists = append(s.Lists, root)
	var names []string
	for name := range s.Files {
		if inside(root, name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}
func (s *Inputs) required(ctx context.Context, path string) (string, mutants.InputFailure) {
	if failure := inputStopped(ctx); failure != nil {
		return "", failure
	}
	if data, ok := s.Read(ctx, path); ok {
		return string(data), nil
	}
	return "", inputError(fmt.Errorf("%s: file not present in memory", path))
}
