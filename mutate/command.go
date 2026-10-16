package mutate

import (
	"context"
	"os"
	"path/filepath"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/testtools"
)

// TestCommand is the legacy direct-call boundary. Actual Run receives its
// Toolchain through Options; direct library/frozen callers retain real facts
// until T-92/T-95 moves their orchestration.
func TestCommand(ctx context.Context, path, shell string, all bool, tests []string) Command {
	c, _ := mutants.ObserveCommand(ctx, testtools.Toolchain{}, mutants.CommandRequest{Path: path, Shell: shell, All: all, Tests: tests})
	return c
}

func OwnScope(ctx context.Context, path string) string {
	if spec := lang.Detect(path); spec != nil && spec.Name == "typescript" && TestCommand(ctx, path, "", false, nil).Suite {
		return ScopeAllTests
	}
	return ScopeOwn
}
func FileScope(ctx context.Context, path, shell string, all bool) string {
	if scope := RunScope(shell, all); scope != ScopeOwn {
		return scope
	}
	return OwnScope(ctx, path)
}

// These fresh-preparation callers keep their independently supervised probe.
func pythonBase(ctx context.Context, path string) Command {
	root := testtools.Base(ctx, path, "pyproject.toml", "setup.py", "setup.cfg")
	return Command{Root: root, Dir: root, PathEnv: "PYTHONPATH", PathDirs: []string{".", "src"}}
}
func pythonInterpreter(root string) string { return testtools.PythonInterpreter(root) }
func pythonArgs(py string, pytest bool, run []string) []string {
	return mutants.PythonArgs(py, pytest, run, coverage.PythonModules(run))
}

func orDir(dir, path string) string {
	if dir != "" {
		return dir
	}
	return filepath.Dir(path)
}
func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }

func scopeForCommand(shell string, all bool, command Command) string {
	if scope := RunScope(shell, all); scope != ScopeOwn {
		return scope
	}
	if command.Suite {
		return ScopeAllTests
	}
	return ScopeOwn
}
