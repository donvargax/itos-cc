package porttest

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// FreshFixture supplies independent committed and live bytes through setup.
// Read observes the actual materialized tree, not a cached plan description.
type FreshFixture struct {
	HistoryFixture
	Fresh mutants.Fresh
	Read  func(context.Context, string) ([]byte, error)
}

// FreshContract holds immutable planning and settings admission shared by the
// physical adapter and memory implementation. Physical export/process guards
// and successful collector preparation have their own adapter contracts.
func FreshContract(t *testing.T, open func(*testing.T) FreshFixture) {
	t.Helper()
	t.Run("committed inputs and global selection stay immutable", func(t *testing.T) {
		b := open(t)
		original := "package sub\nfunc Value(x int) int { return x + 1 }\n"
		historyCommit(t, b.HistoryFixture, map[string]string{"go.mod": "module example/fresh\n", "sub/value.go": original})
		head, failure := b.At(b.Root).Head(t.Context())
		if failure != nil {
			t.Fatal(failure)
		}
		historyPut(t, b.HistoryFixture, "sub/value.go", "package sub\nfunc Value(x int) int { return x + 99 }\n")
		historyGit(t, b.HistoryFixture, "add", "sub/value.go")
		historyPut(t, b.HistoryFixture, "sub/value.go", "package sub\nfunc Value(x int) int { return x + 777 }\n")
		historyPut(t, b.HistoryFixture, "untracked.go", "package extra\nfunc Extra() int { return 9 }\n")
		request := mutants.FreshRequest{Root: filepath.Join(b.Root, "sub"), Count: 1, Seed: "independent-seed"}
		p, failed := b.Fresh.Plan(t.Context(), request)
		if failed != nil {
			t.Fatal(failed)
		}
		defer b.Fresh.Release(t.Context(), p)
		bytes, err := b.Read(t.Context(), filepath.Join(p.FrozenRoot, "sub", "value.go"))
		if err != nil || string(bytes) != original {
			t.Fatalf("frozen committed bytes = %q, %v", bytes, err)
		}
		if p.Commit != head || p.Seed != request.Seed || len(p.Selected) != 1 || p.Selected[0].Path != "sub/value.go" {
			t.Fatalf("plan identity/population: %+v", p)
		}
		if _, err := b.Read(t.Context(), filepath.Join(p.FrozenRoot, "untracked.go")); err == nil {
			t.Fatal("untracked live input entered frozen plan")
		}
		other, failed := b.Fresh.Plan(t.Context(), request)
		if failed != nil {
			t.Fatal(failed)
		}
		defer b.Fresh.Release(t.Context(), other)
		if !reflect.DeepEqual(p.Selected, other.Selected) {
			t.Fatal("same committed inputs and seed changed selection")
		}
		root := p.FrozenRoot
		b.Fresh.Release(t.Context(), p)
		if p.FrozenRoot != "" {
			t.Fatal("released plan remains live")
		}
		if _, err := b.Read(t.Context(), filepath.Join(root, "sub", "value.go")); err == nil {
			t.Fatal("released frozen bytes remain")
		}
	})
	t.Run("committed settings rather than live settings admit sources", func(t *testing.T) {
		b := open(t)
		historyCommit(t, b.HistoryFixture, map[string]string{"go.mod": "module example/settings\n", "value.go": "package value\nfunc Value() int { return 1 }\n", "itos-cc.yaml": "exclude:\n  - value.go\n"})
		historyPut(t, b.HistoryFixture, "itos-cc.yaml", "[invalid live settings")
		p, failed := b.Fresh.Plan(t.Context(), mutants.FreshRequest{Root: b.Root, Count: 1})
		if failed != nil {
			t.Fatal(failed)
		}
		defer b.Fresh.Release(t.Context(), p)
		if len(p.Eligible) != 0 || len(p.Units) != 0 {
			t.Fatalf("committed exclusion ignored: %+v", p)
		}
		historyCommit(t, b.HistoryFixture, map[string]string{"itos-cc.yaml": "[invalid committed settings"})
		historyPut(t, b.HistoryFixture, "itos-cc.yaml", "")
		if _, failed := b.Fresh.Plan(t.Context(), mutants.FreshRequest{Root: b.Root, Count: 1}); failed == nil {
			t.Fatal("valid live settings hid invalid committed settings")
		} else if _, ok := failed.(mutants.FreshInvalid); !ok {
			t.Fatalf("invalid settings kind: %T %v", failed, failed)
		}
	})
	t.Run("request root and cancellation are actual observations", func(t *testing.T) {
		b := open(t)
		historyCommit(t, b.HistoryFixture, map[string]string{"go.mod": "module example/root\n", "value.go": "package value\nfunc Value() int { return 1 }\n"})
		if _, failed := b.Fresh.Plan(t.Context(), mutants.FreshRequest{Root: filepath.Join(b.Root, "missing"), Count: 1}); failed == nil {
			t.Fatal("unrelated root silently used bound repository")
		} else if refused, ok := failed.(mutants.FreshRefused); !ok || refused.Kind != mutants.FreshRepository {
			t.Fatalf("bad root classification: %T %v", failed, failed)
		}
		cause := errors.New("caller stopped planning")
		ctx, cancel := context.WithCancelCause(t.Context())
		cancel(cause)
		_, failed := b.Fresh.Plan(ctx, mutants.FreshRequest{Root: b.Root, Count: 1})
		if _, ok := failed.(mutants.FreshCancelled); !ok || !errors.Is(failed, cause) {
			t.Fatalf("caller cause: %T %v", failed, failed)
		}
	})
	t.Run("the plan names each live change to its commit, and nothing ignored", func(t *testing.T) {
		b := open(t)
		historyCommit(t, b.HistoryFixture, map[string]string{"go.mod": "module example/live\n", "value.go": "package value\nfunc Value() int { return 1 }\n",
			"value_test.go": "package value\n", "README.md": "# value\n", ".gitignore": "ignored.go\nbuild/\n", "itos-cc.yaml": "exclude: []\n"})
		p, failed := b.Fresh.Plan(t.Context(), mutants.FreshRequest{Root: b.Root, Count: 1})
		if failed != nil {
			t.Fatal(failed)
		}
		if len(p.Uncommitted) != 0 || p.UncommittedInputs() == nil || len(p.UncommittedInputs()) != 0 {
			t.Errorf("a clean checkout's uncommitted = %q, inputs %q", p.Uncommitted, p.UncommittedInputs())
		}
		b.Fresh.Release(t.Context(), p)
		historyPut(t, b.HistoryFixture, "value.go", "package value\nfunc Value() int { return 2 }\n")
		historyGit(t, b.HistoryFixture, "add", "value.go")
		historyPut(t, b.HistoryFixture, "value_test.go", "package value\n\n// unstaged\n")
		historyPut(t, b.HistoryFixture, "itos-cc.yaml", "exclude: [value.go]\n")
		historyPut(t, b.HistoryFixture, "README.md", "# value, changed\n")
		historyPut(t, b.HistoryFixture, "sub/new_test.go", "package sub\n")
		historyPut(t, b.HistoryFixture, "ignored.go", "package value\n")
		historyPut(t, b.HistoryFixture, "build/out.go", "package build\n")
		p, failed = b.Fresh.Plan(t.Context(), mutants.FreshRequest{Root: b.Root, Count: 1})
		if failed != nil {
			t.Fatal(failed)
		}
		defer b.Fresh.Release(t.Context(), p)
		if want := []string{"README.md", "itos-cc.yaml", "sub/new_test.go", "value.go", "value_test.go"}; !reflect.DeepEqual(p.Uncommitted, want) {
			t.Errorf("uncommitted = %q, want %q", p.Uncommitted, want)
		}
		if want := []string{"itos-cc.yaml", "sub/new_test.go", "value.go", "value_test.go"}; !reflect.DeepEqual(p.UncommittedInputs(), want) {
			t.Errorf("uncommitted inputs = %q, want %q", p.UncommittedInputs(), want)
		}
	})
	FreshNamedContract(t, open)
}
