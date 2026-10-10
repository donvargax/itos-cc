package mutate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/lang"
)

// FreshBaseline is one unmutated run, in a plan's frozen root, of the own
// tests that will judge some of its selected sites.
type FreshBaseline struct {
	Command Command
	Paths   []string // root-relative files whose selected sites this command judges
	Elapsed time.Duration
}

// FreshBaselineCommands is the distinct own-test command of every file with a
// selected site, resolved against the frozen root. It is TestCommand with the
// one probe TestCommand runs itself, Python's pytest import, routed through
// execute and recorded; a cancelled probe is an error.
func FreshBaselineCommands(ctx context.Context, plan *FreshPlan, all bool, execute coverage.CommandExecutor) ([]FreshBaseline, []coverage.CommandExecution, error) {
	if ctx == nil || execute == nil {
		return nil, nil, errors.New("fresh baselines need a context and command executor")
	}
	if plan == nil || plan.FrozenRoot == "" {
		return nil, nil, errors.New("fresh baselines need a live frozen plan")
	}
	var paths []string
	seen := map[string]bool{}
	for _, candidate := range plan.Selected {
		if !seen[candidate.Path] {
			seen[candidate.Path] = true
			paths = append(paths, candidate.Path)
		}
	}
	sort.Strings(paths)
	var baselines []FreshBaseline
	var executions []coverage.CommandExecution
	byKey := map[string]int{}
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
		if spec.Name == "python" {
			var calls []coverage.CommandExecution
			var err error
			c, calls, err = freshPythonCommand(ctx, path, execute)
			executions = append(executions, calls...)
			if err != nil {
				return nil, executions, err
			}
		} else {
			c = TestCommand(path, "", all)
		}
		if len(c.Args) == 0 {
			return nil, executions, fmt.Errorf("selected file %s has no own-test command", rel)
		}
		if err := ensureInside(plan.FrozenRoot, c.Dir); err != nil {
			return nil, executions, fmt.Errorf("own-test command of %s leaves the frozen root: %w", rel, err)
		}
		if i, ok := byKey[c.Key()]; ok {
			baselines[i].Paths = append(baselines[i].Paths, rel)
			continue
		}
		byKey[c.Key()] = len(baselines)
		baselines = append(baselines, FreshBaseline{Command: c, Paths: []string{rel}})
	}
	return baselines, executions, nil
}

// freshPythonCommand is pythonCommand with its pytest probe supervised. A
// probe that fails falls back to unittest exactly as pythonCommand does.
func freshPythonCommand(ctx context.Context, path string, execute coverage.CommandExecutor) (Command, []coverage.CommandExecution, error) {
	root := orDir(lang.FindUp(path, "pyproject.toml", "setup.py", "setup.cfg"), path)
	py := "python3"
	for _, venv := range []string{".venv", "venv"} {
		if candidate := filepath.Join(root, venv, "bin", "python"); fileExists(candidate) {
			py = candidate
		}
	}
	c := Command{Root: root, Dir: root, PathEnv: "PYTHONPATH", PathDirs: []string{".", "src"}}
	probe := exec.CommandContext(ctx, py, "-c", "import pytest")
	probe.Dir, probe.Stdout, probe.Stderr = root, io.Discard, io.Discard
	err := execute(ctx, probe)
	calls := []coverage.CommandExecution{{Args: append([]string{}, probe.Args...), Dir: root, Err: err}}
	switch {
	case err == nil:
		c.Args = []string{py, "-m", "pytest", "-q", "-x", "-p", "no:cacheprovider"}
	case ctx.Err() != nil:
		return Command{}, calls, ctx.Err()
	default:
		c.Args = []string{py, "-m", "unittest", "discover", "-f"}
	}
	return c, calls, nil
}

// RunFreshBaselines runs each baseline once, without any mutant, in the
// frozen root, and records its time. Any failure, a nonzero exit included,
// fails the whole preparation: no selected site of a failing command could
// be judged.
func RunFreshBaselines(ctx context.Context, baselines []FreshBaseline, log io.Writer, execute coverage.CommandExecutor) ([]FreshBaseline, []coverage.CommandExecution, error) {
	if ctx == nil || execute == nil {
		return nil, nil, errors.New("fresh baselines need a context and command executor")
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
		cmd := exec.CommandContext(ctx, c.Args[0], c.Args[1:]...)
		cmd.Dir = c.Dir
		cmd.Env = os.Environ()
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
		fmt.Fprintf(log, "itos-cc: baseline %s$ %s\n", c.Dir, c)
		started := time.Now()
		err := execute(ctx, cmd)
		b.Elapsed = time.Since(started)
		executions = append(executions, coverage.CommandExecution{Args: append([]string{}, cmd.Args...), Dir: c.Dir, Err: err})
		if err != nil {
			return out, executions, fmt.Errorf("baseline of %s fails without any mutant: %w", strings.Join(b.Paths, ", "), err)
		}
		out = append(out, b)
	}
	return out, executions, nil
}
