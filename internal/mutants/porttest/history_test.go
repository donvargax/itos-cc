package porttest

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/mutants"
)

func TestMemoryHistoryContract(t *testing.T) {
	HistoryContract(t, func(t *testing.T) HistoryFixture {
		root := filepath.Join(t.TempDir(), "repo")
		repository := NewHistoryRepository()
		files := map[string]string{}
		return HistoryFixture{Root: root, At: func(dir string) mutants.History {
			return History{Root: root, Dir: dir, Repository: repository, Working: files}
		}, Outside: History{Dir: filepath.Join(filepath.Dir(root), "outside")},
			Put: func(ctx context.Context, path, text string) error {
				if ctx.Err() != nil {
					return context.Cause(ctx)
				}
				files[filepath.Join(root, filepath.FromSlash(path))] = text
				return nil
			},
			Remove: func(ctx context.Context, path string) error {
				if ctx.Err() != nil {
					return context.Cause(ctx)
				}
				delete(files, filepath.Join(root, filepath.FromSlash(path)))
				return nil
			},
			Git: func(ctx context.Context, args ...string) error {
				if ctx.Err() != nil {
					return context.Cause(ctx)
				}
				return repository.Git(root, root, files, args...)
			},
		}
	})
}
