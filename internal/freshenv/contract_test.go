package freshenv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/internal/githistory"
	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/mutants/porttest"
)

func TestRealFreshPreparationContract(t *testing.T) {
	porttest.FreshPreparationContract(t, func(t *testing.T) porttest.FreshPreparationFixture {
		root := freshPreparationFixture(t)
		environment := fixtureEnvironment(root)
		exit := 0
		var calls []string
		execute := freshPreparationExecutor(t, &calls, nil)
		environment.Execute = func(ctx context.Context, cmd *exec.Cmd) error {
			if err := execute(ctx, cmd); err != nil {
				return err
			}
			if exit != 0 && commandTool(cmd) == "go" && len(cmd.Args) > 1 && cmd.Args[1] == "test" && !slices.Contains(cmd.Args, "-failfast") {
				return errors.New("independent collector failed after report write")
			}
			return nil
		}
		return porttest.FreshPreparationFixture{Root: root, Fresh: environment, Collector: func(_ *mutants.FreshPlan, status int) { exit = status }}
	})
}

func TestRealFreshContract(t *testing.T) {
	porttest.FreshContract(t, func(t *testing.T) porttest.FreshFixture {
		root := t.TempDir()
		git := func(ctx context.Context, args ...string) error {
			cmd := exec.CommandContext(ctx, "git", args...)
			cmd.Dir = root
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("git %v: %w: %s", args, err, out)
			}
			return nil
		}
		for _, args := range [][]string{{"init", "-q"}, {"config", "user.name", "fixture"}, {"config", "user.email", "fixture@example.invalid"}, {"config", "maintenance.auto", "false"}} {
			if err := git(t.Context(), args...); err != nil {
				t.Fatal(err)
			}
		}
		return porttest.FreshFixture{Fresh: fixtureEnvironment(root), HistoryFixture: porttest.HistoryFixture{Root: root,
			At: func(dir string) mutants.History { return githistory.History{Dir: dir} }, Git: git,
			Put: func(_ context.Context, name, text string) error {
				path := filepath.Join(root, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					return err
				}
				return os.WriteFile(path, []byte(text), 0o600)
			},
		}, Read: func(_ context.Context, path string) ([]byte, error) { return os.ReadFile(path) }}
	})
}
