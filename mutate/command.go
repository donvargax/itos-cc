package mutate

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/project"
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

// TestCommand is the narrowest test run that covers path: the Go packages
// whose tests link its package, the Vitest or Jest tests that import it, or
// the whole suite where no narrower run exists. With all, it is the whole suite of path's build
// root, so integration and end-to-end tests anywhere in it can kill a
// mutant. Every command stops at the first failure, since one failing test
// is enough to kill a mutant. A non-empty shell overrides the command but not
// where it runs.
func TestCommand(path, shell string, all bool) Command {
	spec := lang.Detect(path)
	var c Command
	switch spec.Name {
	case "go":
		c = goCommand(path, all)
	case "typescript":
		c = typescriptCommand(path, all)
	case "python":
		c = pythonCommand(path)
	case "kotlin":
		c = kotlinCommand(path, all)
	}
	if shell != "" {
		c.Args, c.Shell = nil, shell
	}
	return c
}

// goScopes remembers each package directory's test scope; every file of a
// package shares it.
var goScopes = map[string][]string{}

// goCommand runs the tests of every package whose test binary links path's
// package, the same tests coverage measured it with, so an integration test
// in another package can kill its mutants. With all, or when go list
// fails, it runs the whole module.
func goCommand(path string, all bool) Command {
	root := orDir(lang.FindUp(path, "go.mod"), path)
	targets := []string{"./..."}
	if !all {
		dir := filepath.Dir(path)
		if _, ok := goScopes[dir]; !ok {
			_, goScopes[dir] = coverage.GoScope(root, []string{path})
		}
		if len(goScopes[dir]) > 0 {
			targets = goScopes[dir]
		}
	}
	return Command{Root: root, Dir: root, Args: append([]string{"go", "test", "-count=1", "-failfast"}, targets...)}
}

func typescriptCommand(path string, all bool) Command {
	root := orDir(lang.FindUp(path, "package.json"), path)
	rel, _ := filepath.Rel(root, path)
	c := Command{Root: root, Dir: root}
	// Tools run from node_modules, never through npx, which would download
	// whatever version the registry has when the project installed none.
	deps := packageDeps(root)
	vitest, jest := project.NodeBin(root, "vitest"), project.NodeBin(root, "jest")
	switch {
	case deps["vitest"] && vitest != "" && all:
		c.Args = []string{vitest, "run", "--bail=1"}
	case deps["vitest"] && vitest != "":
		c.Args = []string{vitest, "related", "--run", "--bail=1", filepath.ToSlash(rel)}
	case deps["jest"] && jest != "" && all:
		c.Args = []string{jest, "--bail"}
	case deps["jest"] && jest != "":
		c.Args = []string{jest, "--bail", "--findRelatedTests", filepath.ToSlash(rel)}
	default:
		// bun test is Bun's own runner, so run the script by name.
		c.Args = []string{project.PackageManager(root), "run", "test"}
		if c.Args[0] == "npm" {
			c.Args = append(c.Args, "--silent")
		}
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

func kotlinCommand(path string, all bool) Command {
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
	if all {
		return Command{Root: root, Dir: root, Args: []string{gradle, "test", "--fail-fast"}}
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
