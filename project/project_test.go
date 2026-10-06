package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestChangedFromASubdirectoryWithUnusualNames(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	for _, name := range []string{"top.py", "sub/plain.py", "sub/año nuevo.py"} {
		write(t, filepath.Join(repo, name), "x = 1\n")
	}
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-qm", "init")
	for _, name := range []string{"top.py", "sub/plain.py", "sub/año nuevo.py", "sub/new file.py"} {
		write(t, filepath.Join(repo, name), "x = 2\n")
	}

	t.Chdir(filepath.Join(repo, "sub"))
	got, err := Changed()
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	if want := []string{"año nuevo.py", "new file.py", "plain.py"}; !slices.Equal(got, want) {
		t.Errorf("Changed() = %q, want %q", got, want)
	}
}

func TestChangedBeforeTheFirstCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	write(t, filepath.Join(repo, "staged.py"), "x = 1\n")
	git(t, repo, "add", "staged.py")

	t.Chdir(repo)
	got, err := Changed()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"staged.py"}; !slices.Equal(got, want) {
		t.Errorf("Changed() = %q, want %q", got, want)
	}
}
