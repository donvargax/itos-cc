package githistory

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/mutants/porttest"
)

// Native Git setup/translation and caller-directory namespace identity cannot be
// proved by CLI selection alone. The process cwd intentionally stays unrelated.
func TestRealHistoryContract(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	porttest.HistoryContract(t, func(t *testing.T) porttest.HistoryFixture {
		outer, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		root, outside := filepath.Join(outer, "repo"), filepath.Join(outer, "outside")
		for _, path := range []string{root, outside} {
			if err := os.MkdirAll(path, 0700); err != nil {
				t.Fatal(err)
			}
		}
		git := func(ctx context.Context, args ...string) error {
			if ctx.Err() != nil {
				return context.Cause(ctx)
			}
			command := exec.CommandContext(ctx, "git", args...)
			command.Dir = root
			command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=T91 Fixture", "GIT_AUTHOR_EMAIL=t91@example.invalid", "GIT_COMMITTER_NAME=T91 Fixture", "GIT_COMMITTER_EMAIL=t91@example.invalid", "GIT_CONFIG_NOSYSTEM=1")
			out, err := command.CombinedOutput()
			if err != nil {
				return fmt.Errorf("fixture git %v: %s: %w", args, out, err)
			}
			return nil
		}
		if err := git(t.Context(), "init", "-qb", "main"); err != nil {
			t.Fatal(err)
		}
		return porttest.HistoryFixture{Root: root, At: func(dir string) mutants.History { return History{Dir: dir} }, Outside: History{Dir: outside},
			Put: func(ctx context.Context, path, text string) error {
				if ctx.Err() != nil {
					return context.Cause(ctx)
				}
				abs := filepath.Join(root, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
					return err
				}
				return os.WriteFile(abs, []byte(text), 0600)
			},
			Remove: func(ctx context.Context, path string) error {
				if ctx.Err() != nil {
					return context.Cause(ctx)
				}
				return os.Remove(filepath.Join(root, filepath.FromSlash(path)))
			}, Git: git,
		}
	})
}
