package project

import (
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Renames are read across the whole repository, from a subdirectory too,
// by absolute path under the root: a range's committed ones, and the
// working tree's once the new file is tracked, staged by git mv or added
// after a plain mv. A plain mv alone, or a deletion, is no rename.
func TestRenamesAreWhatGitShowsRenamed(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	for _, name := range []string{"src/committed.py", "src/staged.py", "src/added.py", "src/plain.py", "src/deleted.py"} {
		write(t, filepath.Join(repo, filepath.FromSlash(name)), "def f(x):\n    return x  # "+name+"\n")
	}
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-qm", "base")
	git(t, repo, "tag", "base")
	write(t, filepath.Join(repo, "lib", "keep"), "")
	git(t, repo, "mv", "src/committed.py", "lib/committed.py")
	git(t, repo, "commit", "-qm", "rename")
	git(t, repo, "mv", "src/staged.py", "lib/staged.py")
	for _, name := range []string{"added", "plain"} {
		if err := os.Rename(filepath.Join(repo, "src", name+".py"), filepath.Join(repo, "lib", name+".py")); err != nil {
			t.Fatal(err)
		}
	}
	git(t, repo, "add", "lib/added.py")
	git(t, repo, "rm", "-q", "src/deleted.py")

	t.Chdir(filepath.Join(repo, "lib"))
	root := Root()
	at := func(name string) string { return filepath.Join(root, filepath.FromSlash(name)) }
	since, err := RenamesSince("base")
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{at("src/committed.py"): at("lib/committed.py")}; !maps.Equal(since, want) {
		t.Errorf("renames since base %v, want %v", since, want)
	}
	uncommitted, err := RenamesUncommitted()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{at("src/staged.py"): at("lib/staged.py"), at("src/added.py"): at("lib/added.py")}
	if !maps.Equal(uncommitted, want) {
		t.Errorf("uncommitted renames %v, want %v: a plain mv with nothing staged is a deletion, and a deletion is none", uncommitted, want)
	}
}

// Outside a git repository, or in one with no commit, renames cannot be
// read, and the error says so.
func TestRenamesNeedAGitRepositoryWithACommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	t.Chdir(dir)
	var noGit *NoGitError
	if _, err := RenamesUncommitted(); !errors.As(err, &noGit) {
		t.Errorf("renames outside a repository: %v, want a NoGitError", err)
	}
	git(t, dir, "init", "-q")
	if _, err := RenamesUncommitted(); !errors.As(err, &noGit) {
		t.Errorf("renames in a repository with no commit: %v, want a NoGitError", err)
	}
}
