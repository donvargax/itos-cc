package githistory

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/lang"
	langtest "github.com/donvargax/itos-cc/internal/lang/port/porttest"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
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

// Functions sharing a name are told apart by their hash: of two Python
// functions a, only the one the range changed is judged, and b, unchanged,
// is not. Moved from project's tests when ChangedSince left mutation's
// judgment to its caller (T-156); the contract checks the same of Go's init.
func TestChangedSinceTellsFunctionsSharingANameApartByHash(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = repo
		command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=T156 Fixture", "GIT_AUTHOR_EMAIL=t156@example.invalid", "GIT_COMMITTER_NAME=T156 Fixture", "GIT_COMMITTER_EMAIL=t156@example.invalid", "GIT_CONFIG_NOSYSTEM=1")
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("fixture git %v: %s: %v", args, out, err)
		}
	}
	name := filepath.Join(repo, "x.py")
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(name, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-qb", "main")
	write("def a(x):\n    return x\n\n\ndef a(x):\n    return x\n\n\ndef b(x):\n    return x\n")
	git("add", "-A")
	git("commit", "-qm", "base")
	git("tag", "base")
	after := "def a(x):\n    return x\n\n\ndef a(x):\n    return x + 1\n\n\ndef b(x):\n    return x\n"
	write(after)
	git("commit", "-qam", "change the second a")

	changed, _, failure := History{Dir: repo}.ChangedSince(t.Context(), "base")
	if failure != nil {
		t.Fatal(failure)
	}
	c := changed[name]
	var id string
	for f := range c.Functions {
		id = f
	}
	if len(c.Functions) != 1 || !strings.HasSuffix(id, "#a") {
		t.Fatalf("functions %v, want only a", c.Functions)
	}
	parsed, err := lang.Parse(t.Context(), langtest.Surroundings{Dir: repo, Files: map[string]string{name: after}}, lang.Detect(name), name, []byte(after))
	if err != nil {
		t.Fatal(err)
	}
	defer parsed.Close()
	var hashes []string
	for _, unit := range parsed.Units {
		hashes = append(hashes, mutants.UnitHash(parsed, unit))
	}
	if len(hashes) != 3 {
		t.Fatalf("units %d, want a, a and b", len(hashes))
	}
	if c.Judges(id, hashes[0]) {
		t.Errorf("the first a, unchanged, is judged")
	}
	if !c.Judges(id, hashes[1]) {
		t.Errorf("the second a, changed, is not judged")
	}
	if c.Judges(strings.TrimSuffix(id, "a")+"b", hashes[2]) {
		t.Errorf("b, unchanged, is judged")
	}
}
