package features

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Head reads Git's actual current commit, independently of itos-cc output.
func (binary) Head(ctx context.Context, dir string) (string, error) {
	git, err := realGit()
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, git, "rev-parse", "HEAD") //nolint:gosec // the real git the scenarios resolved, asked for HEAD
	cmd.Dir = dir
	cmd.Env = environment(nil)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Git HEAD: %w: %s", err, out)
	}
	return strings.TrimSpace(string(out)), nil
}
