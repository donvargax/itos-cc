package mutants_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	seltest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

// absentFreshSettings admits only the absence held by pure domain fixtures.
// Actual committed settings use config.Parse in adapter-owned shared suites.
func absentFreshSettings(data []byte) ([]string, *mutants.Tests, []mutants.Exception, *mutants.FreshInvalid) {
	if data != nil {
		return nil, nil, nil, &mutants.FreshInvalid{Reason: "this pure fixture has no configuration codec"}
	}
	return nil, nil, nil, nil
}

func memoryFreshFixture(t *testing.T) porttest.FreshFixture {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{}
	repo := porttest.NewHistoryRepository()
	inputs := &porttest.Inputs{Dir: root, Files: files}
	tree := seltest.TreeOf(root, map[string]string{}, nil)
	f := &porttest.Fresh{History: porttest.History{Root: root, Dir: root, Repository: repo, Working: files}, DecodeConfig: absentFreshSettings, Inputs: inputs, Tree: &tree}
	h := porttest.HistoryFixture{Root: root, At: func(dir string) mutants.History {
		return porttest.History{Root: root, Dir: dir, Repository: repo, Working: files}
	},
		Put: func(_ context.Context, name, text string) error {
			files[filepath.Join(root, filepath.FromSlash(name))] = text
			return nil
		},
		Git: func(_ context.Context, args ...string) error { return repo.Git(root, root, files, args...) },
	}
	return porttest.FreshFixture{HistoryFixture: h, Fresh: f, Read: func(ctx context.Context, path string) ([]byte, error) {
		if text, ok := inputs.Read(ctx, path); ok {
			return text, nil
		}
		return nil, fmt.Errorf("missing virtual frozen input %s", path)
	}}
}
