package project

import (
	"errors"
	"maps"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestDiffLinesReadsEveryNameGitPrints(t *testing.T) {
	diff := strings.Join([]string{
		"diff --git a/plain.py b/plain.py",
		"--- a/plain.py",
		"+++ b/plain.py",
		"@@ -3,2 +3,3 @@ def f():",
		"+++ an added line that reads like a header",
		"@@ -9 +10,0 @@",
		"diff --git a/año nuevo.py b/año nuevo.py",
		"--- a/año nuevo.py\t",
		"+++ b/año nuevo.py\t",
		"@@ -1 +1 @@",
		`diff --git "a/q\"x.py" "b/q\"x.py"`,
		`--- "a/q\"x.py"`,
		`+++ "b/q\"x.py"`,
		"@@ -0,0 +1,2 @@",
		"diff --git a/gone.py b/gone.py",
		"--- a/gone.py",
		"+++ /dev/null",
		"@@ -1 +0,0 @@",
		// Renamed unchanged: no hunk, and still a file of the range.
		"diff --git a/old.py b/new.py",
		"similarity index 100%",
		"rename from old.py",
		"rename to new.py",
		"diff --git a/was.py b/now.py",
		"similarity index 90%",
		"rename from was.py",
		"rename to now.py",
		"--- a/was.py",
		"+++ b/now.py",
		"@@ -2 +2 @@",
		`diff --git "a/q\"old.py" "b/q\"new.py"`,
		"similarity index 100%",
		`rename from "q\"old.py"`,
		`rename to "q\"new.py"`,
	}, "\n")
	want := map[string][]lines{
		"plain.py":     {{3, 5}, {10, 11}},
		"año nuevo.py": {{1, 1}},
		`q"x.py`:       {{1, 2}},
		"new.py":       nil,
		"now.py":       {{2, 2}},
		`q"new.py`:     nil,
	}
	wantRenames := map[string]string{"new.py": "old.py", "now.py": "was.py", `q"new.py`: `q"old.py`}
	got, renames := diffLines(diff)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("diffLines = %v, want %v", got, want)
	}
	if !maps.Equal(renames, wantRenames) {
		t.Errorf("renames = %v, want %v", renames, wantRenames)
	}
}

func TestChangedSinceFindsTheFunctionsInHEAD(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	name := filepath.Join(repo, "sub", "año nuevo.py")
	write(t, name, "def a(x):\n    return x\n\n\ndef b(x):\n    return x\n")
	write(t, filepath.Join(repo, "top.py"), "def c(x):\n    return x\n")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-qm", "base")
	git(t, repo, "tag", "base")
	write(t, name, "def a(x):\n    return x\n\n\ndef b(x):\n    return x + 1\n")
	write(t, filepath.Join(repo, "top.py"), "def c(x):\n    return x + 1\n")
	git(t, repo, "commit", "-qam", "change b and c")
	// Uncommitted: b moves down onto the lines of a's HEAD version, and a
	// changes. Neither is in the range.
	write(t, name, "import os\n\n\ndef a(x):\n    return x * 2\n\n\ndef b(x):\n    return x + 1\n")

	t.Chdir(filepath.Join(repo, "sub"))
	got, _, err := ChangedSince("base")
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs("año nuevo.py")
	if keys := slices.Collect(maps.Keys(got)); !slices.Equal(keys, []string{abs}) {
		t.Fatalf("files %q, want only %q: top.py is not under sub", keys, abs)
	}
	var functions []string
	for f := range got[abs] {
		functions = append(functions, f[strings.Index(f, "#")+1:])
	}
	if !slices.Equal(functions, []string{"b"}) {
		t.Errorf("functions %q, want [b]", functions)
	}

	if _, _, err := ChangedSince("--output=x"); err != ErrBadRef {
		t.Errorf("a ref that reads as an option: %v, want ErrBadRef", err)
	}
}

func TestHeadIsTheHEADCommitsId(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	repo := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(repo))
	t.Chdir(repo)
	var noGit *NoGitError
	if _, err := Head(); !errors.As(err, &noGit) {
		t.Errorf("outside a repository: %v, want a NoGitError", err)
	}
	git(t, repo, "init", "-q")
	if _, err := Head(); !errors.As(err, &noGit) {
		t.Errorf("in a repository with no commit: %v, want a NoGitError", err)
	}
	write(t, filepath.Join(repo, "a.py"), "def a(x):\n    return x\n")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-qm", "base")
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	if id, err := Head(); err != nil || id != strings.TrimSpace(string(out)) {
		t.Errorf("Head() = %q, %v, want %q", id, err, strings.TrimSpace(string(out)))
	}
}
