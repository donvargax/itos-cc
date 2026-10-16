package project

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
)

// historyCommand always executes in the declared caller directory. An empty
// directory preserves the legacy direct callers' current-working-directory API.
func historyCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	return cmd
}

func historyAbs(dir, path string) (string, error) {
	if dir == "" || filepath.IsAbs(path) {
		return filepath.Abs(path)
	}
	return filepath.Abs(filepath.Join(dir, path))
}

// historySurroundings makes namespace/path interpretation use the same caller
// directory as Git, while preserving the existing disk surroundings observations.
type historySurroundings struct{ Dir string }

func (s historySurroundings) Exists(ctx context.Context, path string) bool {
	abs, err := historyAbs(s.Dir, path)
	return err == nil && (Surroundings{}).Exists(ctx, abs)
}
func (s historySurroundings) Read(ctx context.Context, path string) ([]byte, bool) {
	abs, err := historyAbs(s.Dir, path)
	if err != nil {
		return nil, false
	}
	return (Surroundings{}).Read(ctx, abs)
}
func (s historySurroundings) WorkingDir(ctx context.Context) (string, bool) {
	if s.Dir == "" {
		return (Surroundings{}).WorkingDir(ctx)
	}
	return s.Dir, true
}
func (s historySurroundings) Abs(_ context.Context, path string) (string, bool) {
	abs, err := historyAbs(s.Dir, path)
	return abs, err == nil
}

// HeadAt observes HEAD in dir without changing process cwd.
func HeadAt(ctx context.Context, dir string) (string, error) {
	if ctx.Err() != nil {
		return "", context.Cause(ctx)
	}
	out, err := historyCommand(ctx, dir, "rev-parse", "--verify", "HEAD^{commit}").Output()
	if ctx.Err() != nil {
		return "", context.Cause(ctx)
	}
	if err != nil {
		return "", &NoGitError{gitError(nil, err)}
	}
	return strings.TrimSpace(string(out)), nil
}

// ResolveAt validates a commit spelling, preserving invalid-ref classification.
func ResolveAt(ctx context.Context, dir, ref string) (string, error) {
	if ctx.Err() != nil {
		return "", context.Cause(ctx)
	}
	out, err := historyCommand(ctx, dir, "rev-parse", "--git-dir").CombinedOutput()
	if ctx.Err() != nil {
		return "", context.Cause(ctx)
	}
	if err != nil {
		return "", &NoGitError{gitError(out, err)}
	}
	if strings.HasPrefix(ref, "-") {
		return "", ErrBadRef
	}
	out, err = historyCommand(ctx, dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}").Output()
	if ctx.Err() != nil {
		return "", context.Cause(ctx)
	}
	if err != nil {
		return "", ErrBadRef
	}
	return strings.TrimSpace(string(out)), nil
}

// ReadCommittedAt reads an immutable root-relative blob. It exports nothing and
// follows no filesystem target; frozen materialization/mode admission is separate.
func ReadCommittedAt(ctx context.Context, dir, ref, path string) ([]byte, error) {
	commit, err := ResolveAt(ctx, dir, ref)
	if err != nil {
		return nil, err
	}
	out, err := historyCommand(ctx, dir, "cat-file", "blob", commit+":"+filepath.ToSlash(path)).Output()
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	return out, err
}
