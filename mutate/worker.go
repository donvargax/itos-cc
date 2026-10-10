package mutate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/donvargax/itos-cc/project"
)

// linkDirs hold dependencies: a worker links them instead of copying.
var linkDirs = map[string]bool{"node_modules": true, ".venv": true, "venv": true, "vendor": true}

// skipDirs are history and caches a worker neither copies nor shares, as
// is build output (project.IsBuildOutput); tools rebuild what they need
// inside the worker.
var skipDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true, ".metrics": true, ".idea": true, ".vscode": true,
	".gradle": true, "__pycache__": true, ".pytest_cache": true, ".mypy_cache": true, ".tox": true,
}

// worker owns private copies of project trees, so mutants run in parallel
// without ever touching the real files.
type worker struct {
	dir    string
	copies map[string]string // real root → copy
	runner commandRunner
	// prepare, when set, adjusts each command before it runs, such as its
	// environment; complete runs leave it unset.
	prepare func(*exec.Cmd)
}

// copyOf returns the worker's copy of root, creating it on first use.
func (w *worker) copyOf(root string) (string, error) {
	if c, ok := w.copies[root]; ok {
		return c, nil
	}
	h := sha256.Sum256([]byte(root))
	dst := filepath.Join(w.dir, hex.EncodeToString(h[:4]), filepath.Base(root))
	if err := copyTree(root, dst); err != nil {
		return "", err
	}
	w.copies[root] = dst
	return dst, nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir() && path != src && (skipDirs[d.Name()] || project.IsBuildOutput(path)):
			return filepath.SkipDir
		case d.IsDir() && path != src && linkDirs[d.Name()]:
			return skipAfter(os.Symlink(path, target))
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case d.Type()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case d.Type().IsRegular():
			return copyFile(path, target)
		}
		return nil
	})
}

func skipAfter(err error) error {
	if err != nil {
		return err
	}
	return filepath.SkipDir
}

func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// result of one test run.
type result struct {
	passed    bool
	timedOut  bool
	cancelled bool
	elapsed   time.Duration
	output    string
	exitCode  int // -1 when the command gave none
}

// run executes c inside the worker's copy of c.Root. A zero timeout waits
// as long as the tests take.
func (w *worker) run(c Command, timeout time.Duration) (result, error) {
	return w.runContext(context.Background(), c, timeout)
}

// runContext is the internal parent-context entrypoint. Command setup stays
// here so both the legacy adapter and supervised callers use the same copy,
// arguments, environment and streams.
func (w *worker) runContext(parent context.Context, c Command, timeout time.Duration) (result, error) {
	root, err := w.copyOf(c.Root)
	if err != nil {
		return result{}, err
	}
	dir := filepath.Join(root, strings.TrimPrefix(c.Dir, c.Root))
	ctx := parent
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeoutCause(parent, timeout, errMutationTimeout)
		defer cancel()
	}
	var cmd *exec.Cmd
	if c.Shell != "" {
		cmd = shellCommand(ctx, c.Shell)
	} else {
		cmd = exec.CommandContext(ctx, c.Args[0], c.Args[1:]...)
	}
	cmd.Dir = dir
	cmd.Env = project.NoBytecodeEnv(os.Environ())
	if c.PathEnv != "" {
		var paths []string
		for _, p := range c.PathDirs {
			paths = append(paths, filepath.Join(root, p))
		}
		if old := os.Getenv(c.PathEnv); old != "" {
			paths = append(paths, old)
		}
		cmd.Env = append(cmd.Env, c.PathEnv+"="+strings.Join(paths, string(os.PathListSeparator)))
	}
	if w.prepare != nil {
		w.prepare(cmd)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	return w.runner.run(ctx, cmd, &out)
}

// withMutant writes src to the worker's copy of path, runs fn, and restores
// the original text.
func (w *worker) withMutant(root, path string, original, mutated []byte, fn func() (result, error)) (result, error) {
	copyRoot, err := w.copyOf(root)
	if err != nil {
		return result{}, err
	}
	rel, _ := filepath.Rel(root, path)
	target := filepath.Join(copyRoot, rel)
	if err := os.WriteFile(target, mutated, 0o644); err != nil {
		return result{}, err
	}
	defer os.WriteFile(target, original, 0o644)
	return fn()
}

func isExit(err error) bool {
	_, ok := err.(*exec.ExitError)
	return ok
}
