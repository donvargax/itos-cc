package freshenv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/project"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

type FreshBaseline = mutants.FreshBaseline
type Command = mutants.Command

// FreshBaselineCommands is the distinct own-test command of every file with a
// selected site, resolved against the frozen root. It is TestCommand, given
// tests, the test files that reach a file in the frozen root, with the one
// probe TestCommand runs itself, Python's pytest import, routed through
// execute and recorded; a cancelled probe is an error. A Python or Kotlin
// file whose own tests run nothing has no baseline: no test reaches it, so
// its sites are uncovered.
func (e Environment) baselineCommands(ctx context.Context, plan *FreshPlan, all bool, tests func(path string) []string, execute coverage.CommandExecutor) ([]FreshBaseline, []coverage.CommandExecution, error) {
	if execute == nil {
		return nil, nil, errors.New("fresh baselines need a command executor")
	}
	if plan == nil || plan.FrozenRoot == "" {
		return nil, nil, errors.New("fresh baselines need a live frozen plan")
	}
	paths := mutants.FreshSelectedPaths(plan)
	var baselines []FreshBaseline
	var executions []coverage.CommandExecution
	for _, rel := range paths {
		if err := ctx.Err(); err != nil {
			return nil, executions, err
		}
		path := filepath.Join(plan.FrozenRoot, filepath.FromSlash(rel))
		spec := lang.Detect(path)
		if spec == nil {
			return nil, executions, fmt.Errorf("selected file %s has no supported language", rel)
		}
		var c Command
		var reaching []string
		if tests != nil {
			reaching = tests(path)
		}
		if spec.Name == "python" {
			var calls []coverage.CommandExecution
			var err error
			c, calls, err = freshPythonCommand(ctx, path, all, reaching, execute)
			executions = append(executions, calls...)
			if err != nil {
				return nil, executions, err
			}
		} else {
			var failure mutants.InputFailure
			c, failure = mutants.ObserveCommand(ctx, e.Toolchain, mutants.CommandRequest{Path: path, All: all, Tests: reaching})
			if failure != nil {
				return nil, executions, failure
			}
		}
		var err error
		baselines, err = mutants.AddFreshBaseline(baselines, plan.FrozenRoot, rel, c)
		if err != nil {
			return nil, executions, err
		}

	}
	return baselines, executions, nil
}

// freshPythonCommand is pythonCommand with its pytest probe supervised. A
// probe that fails falls back to unittest exactly as pythonCommand does.
func freshPythonCommand(ctx context.Context, path string, all bool, tests []string, execute coverage.CommandExecutor) (Command, []coverage.CommandExecution, error) {
	c := pythonBase(ctx, path)
	root := c.Root
	var run []string
	if !all {
		if run = coverage.PythonTests(c.Dir, tests); len(run) == 0 {
			return c, nil, nil
		}
	}
	py := pythonInterpreter(root)
	probe := exec.CommandContext(ctx, py, "-c", "import pytest") //nolint:gosec // observe the selected installed interpreter through the owned executor
	probe.Dir, probe.Stdout, probe.Stderr = root, io.Discard, io.Discard
	err := execute(ctx, probe)
	calls := []coverage.CommandExecution{{Args: append([]string{}, probe.Args...), Dir: root, Err: err}}
	if err != nil && ctx.Err() != nil {
		return Command{}, calls, ctx.Err()
	}
	c.Args = pythonArgs(py, err == nil, run)
	return c, calls, nil
}

// RunFreshBaselines runs each baseline once, without any mutant, in the
// frozen root, and records its time. Any failure, a nonzero exit included,
// fails the whole preparation: no selected site of a failing command could
// be judged.
func RunFreshBaselines(ctx context.Context, baselines []FreshBaseline, log io.Writer, execute coverage.CommandExecutor) ([]FreshBaseline, []coverage.CommandExecution, error) {
	return runFreshBaselines(ctx, baselines, log, execute, nil)
}

// runFreshBaselines is RunFreshBaselines, each baseline run through
// measured, when it is set, which also gives what it used, the bytes it
// wrote and its peak memory, that size its mutants' limits.
func runFreshBaselines(ctx context.Context, baselines []FreshBaseline, log io.Writer, execute coverage.CommandExecutor, measured func(context.Context, *exec.Cmd) (mutants.CommandResult, error)) ([]FreshBaseline, []coverage.CommandExecution, error) {
	if execute == nil {
		return nil, nil, errors.New("fresh baselines need a command executor")
	}
	if log == nil {
		log = io.Discard
	}
	out := make([]FreshBaseline, 0, len(baselines))
	var executions []coverage.CommandExecution
	for _, b := range baselines {
		if err := ctx.Err(); err != nil {
			return out, executions, err
		}
		c := b.Command
		if len(c.Args) == 0 {
			return out, executions, fmt.Errorf("baseline of %s has no command", strings.Join(b.Paths, ", "))
		}
		cmd := exec.CommandContext(ctx, c.Args[0], c.Args[1:]...) //nolint:gosec // run the admitted frozen baseline with its unchanged command argv
		cmd.Dir = c.Dir
		cmd.Env = project.NoBytecodeEnv(os.Environ())
		if c.PathEnv != "" {
			var paths []string
			for _, p := range c.PathDirs {
				paths = append(paths, filepath.Join(c.Root, p))
			}
			if old := os.Getenv(c.PathEnv); old != "" {
				paths = append(paths, old)
			}
			cmd.Env = append(cmd.Env, c.PathEnv+"="+strings.Join(paths, string(os.PathListSeparator)))
		}
		cmd.Stdout, cmd.Stderr = log, log
		_, _ = fmt.Fprintf(log, "itos-cc: baseline %s$ %s\n", c.Dir, c)
		started := time.Now()
		var err error
		if measured != nil {
			var r mutants.CommandResult
			r, err = measured(ctx, cmd)
			err = ownedError(ctx, cmd, r, err)
			b.Output, b.Memory, b.MemoryMeasured = r.OutputBytes, r.PeakMemory, r.MemoryMeasured
		} else {
			err = execute(ctx, cmd)
		}
		b.Elapsed = time.Since(started)
		executions = append(executions, coverage.CommandExecution{Args: append([]string{}, cmd.Args...), Dir: c.Dir, Err: err})
		if err != nil {
			return out, executions, fmt.Errorf("baseline of %s fails without any mutant: %w", strings.Join(b.Paths, ", "), err)
		}
		out = append(out, b)
	}
	return out, executions, nil
}

// ownedError is how cmd's run, r and err, fails, as the owned executor's
// Run words it: nil when it passed.
func ownedError(ctx context.Context, cmd *exec.Cmd, r mutants.CommandResult, err error) error {
	switch {
	case err != nil:
		return fmt.Errorf("execute %s: %w", cmd.Path, err)
	case r.Cancelled:
		return fmt.Errorf("execute %s cancelled: %w", cmd.Path, context.Cause(ctx))
	case r.TimedOut:
		return fmt.Errorf("execute %s timed out", cmd.Path)
	case !r.Passed:
		return fmt.Errorf("execute %s exited %d", cmd.Path, r.ExitCode)
	}
	return nil
}

func pythonBase(ctx context.Context, path string) Command {
	root := project.FindUp(ctx, path, "pyproject.toml", "setup.py", "setup.cfg")
	if root == "" {
		root = filepath.Dir(path)
	}
	return Command{Root: root, Dir: root, PathEnv: "PYTHONPATH", PathDirs: []string{".", "src"}}
}
func pythonInterpreter(root string) string {
	py := "python3"
	for _, venv := range []string{".venv", "venv"} {
		if candidate := filepath.Join(root, venv, "bin", "python"); fileExists(candidate) {
			py = candidate
		}
	}
	return py
}
func pythonArgs(py string, pytest bool, run []string) []string {
	return mutants.PythonArgs(py, pytest, run, coverage.PythonModules(run))
}
func fileExists(path string) bool {
	_, err := os.Stat(path) // #nosec G703 -- Existence-only installed-interpreter observation deliberately permits external environments and symlink identities; no bytes are read or written.
	return err == nil
}
func orDir(dir, path string) string {
	if dir != "" {
		return dir
	}
	return filepath.Dir(path)
}
