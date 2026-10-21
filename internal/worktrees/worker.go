package worktrees

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

	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/processes"
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
	runner processes.Runner
	// prepare, when set, adjusts each command before it runs, such as its
	// environment; complete runs leave it unset.
	prepare func(*exec.Cmd)
}

// copyOf returns the worker's copy of root, creating it on first use.
func (w *worker) copyOf(ctx context.Context, root string) (string, error) {
	if c, ok := w.copies[root]; ok {
		return c, nil
	}
	h := sha256.Sum256([]byte(root))
	dst := filepath.Join(w.dir, hex.EncodeToString(h[:4]), filepath.Base(root))
	if err := copyTree(ctx, root, dst); err != nil {
		return "", err
	}
	w.copies[root] = dst
	return dst, nil
}

// copyTree copies the tree at src to dst under ctx, linking dependencies
// and leaving out history, caches and build output.
func copyTree(ctx context.Context, src, dst string) error {
	return project.WalkDir(ctx, src, func(ctx context.Context, path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir() && path != src && (skipDirs[d.Name()] || project.IsBuildOutput(ctx, path)):
			return filepath.SkipDir
		case d.IsDir() && path != src && linkDirs[d.Name()]:
			return skipAfter(os.Symlink(path, target))
		case d.IsDir():
			return os.MkdirAll(target, 0o755) //nolint:gosec // preserve project directory behavior inside the owner's private 0700 scratch
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
	in, err := os.Open(src) //nolint:gosec // copying the person's selected project file is this adapter's purpose
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm()) //nolint:gosec // copy into the owned worker tree while preserving the source mode
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

type result = mutants.CommandResult

// run executes c inside the worker's copy of c.Root, under parent. A zero
// timeout waits as long as the tests take.
func (w *worker) run(parent context.Context, c Command, timeout time.Duration) (result, error) {
	root, err := w.copyOf(parent, c.Root)
	if err != nil {
		return result{}, err
	}
	dir := filepath.Join(root, strings.TrimPrefix(c.Dir, c.Root))
	ctx := parent
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeoutCause(parent, timeout, processes.ErrCommandTimeout)
		defer cancel()
	}
	var cmd *exec.Cmd
	if c.Shell != "" {
		cmd = shellCommand(ctx, c.Shell, processes.PlatformAtBoundary(w.runner.Platform))
	} else {
		cmd = exec.CommandContext(ctx, c.Args[0], c.Args[1:]...) //nolint:gosec // execute the selected test command with its unchanged argv
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
	r, err := w.runner.Run(ctx, cmd, &out)
	return result(r), err
}

// withMutant writes src to the worker's copy of path, runs fn under ctx,
// and restores the original text.
func (w *worker) withMutant(ctx context.Context, root, path string, original, mutated []byte, fn func(context.Context) (result, error)) (result, error) {
	copyRoot, err := w.copyOf(ctx, root)
	if err != nil {
		return result{}, err
	}
	rel, _ := filepath.Rel(root, path)
	target := filepath.Join(copyRoot, rel)
	if err := os.WriteFile(target, mutated, 0o644); err != nil { //nolint:gosec // preserve the legacy trial mode inside private worker scratch
		return result{}, err
	}
	defer func() {
		_ = os.WriteFile(target, original, 0o644) //nolint:gosec // restore the owned copy with the exact legacy file mode
	}()
	return fn(ctx)
}

func shellCommand(ctx context.Context, line, platform string) *exec.Cmd {
	if platform == "windows" {
		return exec.CommandContext(ctx, "cmd", "/C", line) //nolint:gosec // the person explicitly selected a shell test command
	}
	return exec.CommandContext(ctx, "sh", "-c", line) //nolint:gosec // the person explicitly selected a shell test command
}
