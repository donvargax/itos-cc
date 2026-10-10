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
	// A fixture repository never runs git's automatic maintenance or gc.
	if len(args) > 0 && args[0] == "init" {
		git(t, dir, "config", "maintenance.auto", "false")
		git(t, dir, "config", "gc.auto", "0")
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

func TestBuildOutputNamesAreSkippedOnlyWhenTheyAreBuildOutput(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-q")
	write(t, filepath.Join(root, ".gitignore"), "gen/out/\n")
	write(t, filepath.Join(root, "go.mod"), "module example.com/r\n")
	write(t, filepath.Join(root, "out", "out.go"), "package out\n")
	write(t, filepath.Join(root, "internal", "build", "build.go"), "package build\n")
	write(t, filepath.Join(root, "gen", "out", "gen.go"), "package out\n")
	write(t, filepath.Join(root, "web", "package.json"), "{}\n")
	write(t, filepath.Join(root, "web", "src", "app.ts"), "export const a = 1\n")
	write(t, filepath.Join(root, "web", "dist", "app.js"), "export const a = 1\n")

	files, err := Discover([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range files.Sources {
		rel, _ := filepath.Rel(root, f)
		got = append(got, filepath.ToSlash(rel))
	}
	want := []string{"internal/build/build.go", "out/out.go", "web/src/app.ts"}
	if !slices.Equal(got, want) {
		t.Errorf("sources = %q, want %q", got, want)
	}
}

func TestPackageManagerIsTheOneTheProjectDeclares(t *testing.T) {
	for file, want := range map[string]string{
		"":                  "npm",
		"package-lock.json": "npm",
		"pnpm-lock.yaml":    "pnpm",
		"yarn.lock":         "yarn",
		"bun.lock":          "bun",
	} {
		root := t.TempDir()
		write(t, filepath.Join(root, "app", "package.json"), "{}")
		if file != "" {
			write(t, filepath.Join(root, file), "")
		}
		if got := PackageManager(filepath.Join(root, "app")); got != want {
			t.Errorf("with %q at the workspace root: %s, want %s", file, got, want)
		}
	}
	root := t.TempDir()
	write(t, filepath.Join(root, "package.json"), `{"packageManager": "yarn@4.5.0"}`)
	write(t, filepath.Join(root, "package-lock.json"), "")
	if got := PackageManager(root); got != "yarn" {
		t.Errorf("packageManager yarn@4.5.0 beside package-lock.json: %s, want yarn", got)
	}
}
