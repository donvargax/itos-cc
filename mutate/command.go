package mutate

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"itos-cc/lang"
)

// Command runs the tests that should kill a file's mutants. Root is the tree a
// worker copies; Dir is where the command runs, inside Root.
type Command struct {
	Root  string
	Dir   string
	Args  []string
	Shell string // a user-supplied command line, instead of Args
	// PathEnv names an environment variable that gets the worker's copy of
	// these Root-relative directories prepended, so the copy wins over an
	// editable install that points at the real tree.
	PathEnv  string
	PathDirs []string
}

// Key identifies a command for sharing one baseline between files.
func (c Command) Key() string {
	return c.Root + "\x00" + c.Dir + "\x00" + strings.Join(c.Args, "\x00") + c.Shell
}

func (c Command) String() string {
	if c.Shell != "" {
		return c.Shell
	}
	return strings.Join(c.Args, " ")
}

// TestCommand is the narrowest test run that covers path: its own Go
// package, the Vitest or Jest tests that import it, or the whole suite where
// no narrower run exists. Every command stops at the first failure, since one
// failing test is enough to kill a mutant. A non-empty shell overrides the
// command but not where it runs.
func TestCommand(path, shell string) Command {
	spec := lang.Detect(path)
	var c Command
	switch spec.Name {
	case "go":
		c = goCommand(path)
	case "typescript":
		c = typescriptCommand(path)
	case "python":
		c = pythonCommand(path)
	case "kotlin":
		c = kotlinCommand(path)
	}
	if shell != "" {
		c.Args, c.Shell = nil, shell
	}
	return c
}

func goCommand(path string) Command {
	root := orDir(lang.FindUp(path, "go.mod"), path)
	pkg, _ := filepath.Rel(root, filepath.Dir(path))
	return Command{Root: root, Dir: root,
		Args: []string{"go", "test", "-count=1", "-failfast", "./" + filepath.ToSlash(pkg)}}
}

func typescriptCommand(path string) Command {
	root := orDir(lang.FindUp(path, "package.json"), path)
	rel, _ := filepath.Rel(root, path)
	c := Command{Root: root, Dir: root}
	deps := packageDeps(root)
	switch {
	case deps["vitest"]:
		c.Args = []string{"npx", "vitest", "related", "--run", "--bail=1", filepath.ToSlash(rel)}
	case deps["jest"]:
		c.Args = []string{"npx", "jest", "--bail", "--findRelatedTests", filepath.ToSlash(rel)}
	default:
		c.Args = []string{"npm", "test", "--silent"}
	}
	return c
}

func pythonCommand(path string) Command {
	root := orDir(lang.FindUp(path, "pyproject.toml", "setup.py", "setup.cfg"), path)
	py := "python3"
	for _, venv := range []string{".venv", "venv"} {
		if candidate := filepath.Join(root, venv, "bin", "python"); fileExists(candidate) {
			py = candidate
		}
	}
	c := Command{Root: root, Dir: root, PathEnv: "PYTHONPATH", PathDirs: []string{".", "src"}}
	if exec.Command(py, "-c", "import pytest").Run() == nil {
		c.Args = []string{py, "-m", "pytest", "-q", "-x", "-p", "no:cacheprovider"}
	} else {
		c.Args = []string{py, "-m", "unittest", "discover", "-f"}
	}
	return c
}

func kotlinCommand(path string) Command {
	module := orDir(lang.FindUp(path, "build.gradle.kts", "build.gradle", "pom.xml"), path)
	if fileExists(filepath.Join(module, "pom.xml")) {
		return Command{Root: module, Dir: module, Args: []string{"mvn", "-q", "test"}}
	}
	// Gradle modules need the build root that holds settings and the wrapper.
	root := orDir(lang.FindUp(path, "settings.gradle.kts", "settings.gradle", "gradlew"), module)
	gradle := "gradle"
	if fileExists(filepath.Join(root, "gradlew")) {
		gradle = "./gradlew"
	}
	rel, _ := filepath.Rel(root, module)
	return Command{Root: root, Dir: root, Args: []string{gradle, "-p", filepath.ToSlash(rel), "test", "--fail-fast"}}
}

func orDir(dir, path string) string {
	if dir != "" {
		return dir
	}
	return filepath.Dir(path)
}

func packageDeps(dir string) map[string]bool {
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	data, _ := os.ReadFile(filepath.Join(dir, "package.json"))
	json.Unmarshal(data, &pkg)
	deps := map[string]bool{}
	for k := range pkg.Dependencies {
		deps[k] = true
	}
	for k := range pkg.DevDependencies {
		deps[k] = true
	}
	return deps
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
