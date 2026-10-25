package project

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// A repository's remote origin is the URL git config gives it, asked from
// any folder of the repository; a repository with no origin, or a folder in
// none, has none.
func TestOriginIsTheRepositorysRemoteURL(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	if got := OriginAt(t.Context(), repo); got != "" {
		t.Errorf("no origin: %q", got)
	}
	git(t, repo, "config", "remote.origin.url", "https://example.com/acme/shop.git")
	write(t, filepath.Join(repo, "sub", "a.go"), "package sub\n")
	if got := OriginAt(t.Context(), filepath.Join(repo, "sub")); got != "https://example.com/acme/shop.git" {
		t.Errorf("origin from sub: %q", got)
	}
	if got := OriginAt(t.Context(), filepath.Join(repo, "gone")); got != "" {
		t.Errorf("a folder that is not there: %q", got)
	}
}
