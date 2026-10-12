package workdir

import (
	"context"
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/internal/selection/port"
	"github.com/donvargax/itos-cc/internal/selection/port/porttest"
)

// workingDir is a temporary directory holding files, by slash-separated
// paths with their text, the test's working directory for its duration,
// git above it out of reach.
func workingDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range slices.Sorted(maps.Keys(files)) {
		write(t, filepath.Join(dir, filepath.FromSlash(name)), files[name])
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	t.Chdir(dir)
	return dir
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

// tree is a working directory holding a Go source and its test.
func tree(t *testing.T) string {
	return workingDir(t, map[string]string{"pkg/sum.go": "package pkg\n", "pkg/sum_test.go": "package pkg\n"})
}

func cancelled(t *testing.T) context.Context {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return ctx
}

// git runs git in the working directory, with an identity of its own,
// no signing, line ending conversion or upkeep, skipping the test when
// there is no git.
func git(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false", "-c", "core.autocrlf=false", "-c", "maintenance.auto=false", "-c", "gc.auto=0"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		if _, missing := exec.LookPath("git"); missing != nil {
			t.Skipf("no git: %v", missing)
		}
		t.Fatalf("git %q: %v\n%s", args, err, out)
	}
}

// open lays f out as the working directory. In a repository every file is
// committed but the changed ones, which alternate between modified since
// the commit and never added, so Changed meets both kinds git reports.
func open(t *testing.T, f porttest.TreeFixture) (port.Tree, string) {
	dir := workingDir(t, f.Files)
	if !f.Git {
		return Tree{}, dir
	}
	git(t, "init", "-q", ".")
	untracked := map[string]bool{}
	for i, name := range f.Changed {
		if i%2 == 0 {
			write(t, filepath.Join(dir, filepath.FromSlash(name)), "committed before it changed\n")
		} else {
			untracked[name] = true
		}
	}
	for name := range f.Files {
		if !untracked[name] {
			git(t, "add", "--", name)
		}
	}
	git(t, "commit", "-q", "--allow-empty", "-m", "fixture")
	for i, name := range f.Changed {
		if i%2 == 0 {
			write(t, filepath.Join(dir, filepath.FromSlash(name)), f.Files[name])
		}
	}
	return Tree{}, dir
}

// The adapter does what every Tree does, the fake's included.
func TestTreeKeepsTheContract(t *testing.T) {
	porttest.TreeContract(t, open)
}

// A root that is not there is Unwalkable with the walk's own message, as
// the command line has always printed it.
func TestDiscoverOfARootThatIsNotThereSaysWhatTheWalkSaid(t *testing.T) {
	absent := filepath.Join(tree(t), "absent")
	_, _, failure := (Tree{}).Discover(t.Context(), []string{absent})
	var unwalkable *port.Unwalkable
	if !errors.As(failure, &unwalkable) {
		t.Fatalf("Discover(absent) = %T %v, want Unwalkable", failure, failure)
	}
	if _, statErr := os.Stat(absent); unwalkable.Reason != statErr.Error() {
		t.Errorf("Reason = %q, want %q", unwalkable.Reason, statErr.Error())
	}
}

// Without a repository, NoGit carries what git said.
func TestChangedOutsideGitSaysWhatGitSaid(t *testing.T) {
	tree(t)
	_, failure := (Tree{}).Changed(t.Context())
	var noGit *port.NoGit
	if !errors.As(failure, &noGit) || noGit.Reason == "" {
		t.Fatalf("Changed() = %T %v, want NoGit with git's reason", failure, failure)
	}
}

// A file that is not there is Unreadable with the read's own message, as
// the command line has always printed it.
func TestReadOfAFileThatIsNotThereSaysWhatTheReadSaid(t *testing.T) {
	absent := filepath.Join(tree(t), "absent.go")
	_, failure := (Tree{}).Read(t.Context(), absent)
	var unreadable *port.Unreadable
	if !errors.As(failure, &unreadable) || unreadable.Path != absent {
		t.Fatalf("Read(absent) = %T %v, want Unreadable", failure, failure)
	}
	if _, readErr := os.ReadFile(absent); unreadable.Reason != readErr.Error() {
		t.Errorf("Reason = %q, want %q", unreadable.Reason, readErr.Error())
	}
}
