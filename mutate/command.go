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

// RunsNothing says whether c runs no command at all: the own tests of a
// Python file no test reaches.
func (c Command) RunsNothing() bool {
	return len(c.Args) == 0 && c.Shell == ""
}

// TestCommand is the narrowest test run that covers path: its own Go
// package, the Vitest or Jest tests that import it, in Python tests, the
// test files that reach it (graph.TestsImporting), or the whole suite where
// no narrower run exists. A Python file none of whose tests is a test file
// pytest collects (coverage.PythonTests) runs nothing (RunsNothing). With
// all, it is the whole suite of path's build root, so integration and
// end-to-end tests anywhere in it can kill a mutant. Every command stops at
// the first failure, since one failing test is enough to kill a mutant. A
// non-empty shell overrides the command but not where it runs.
func TestCommand(path, shell string, all bool, tests []string) Command {
	spec := lang.Detect(path)
	var c Command
	switch spec.Name {
	case "go":
		c = goCommand(path, all)
	case "typescript":
		c = typescriptCommand(path, all)
	case "python":
		if shell != "" {
			// Only where it runs: the probe of pytest is for nothing.
			c = pythonBase(path)
			break
		}
		c = pythonCommand(path, all, tests)
	case "kotlin":
		c = kotlinCommand(path, all)
	}
	if shell != "" {
		c.Args, c.Shell = nil, shell
	}
	return c
}

// goCommand runs the tests of path's own package, or with all the whole
// module, integration and end-to-end tests included.
func goCommand(path string, all bool) Command {
	root := orDir(lang.FindUp(path, "go.mod"), path)
	pkg, _ := filepath.Rel(root, filepath.Dir(path))
	target := "./" + filepath.ToSlash(pkg)
	if all {
		target = "./..."
	}
	return Command{Root: root, Dir: root, Args: []string{"go", "test", "-count=1", "-failfast", target}}
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

// pythonBase is where path's Python tests run, with no command yet.
func pythonBase(path string) Command {
	root := orDir(lang.FindUp(path, "pyproject.toml", "setup.py", "setup.cfg"), path)
	return Command{Root: root, Dir: root, PathEnv: "PYTHONPATH", PathDirs: []string{".", "src"}}
}

// pythonInterpreter is the python that runs root's tests: its own
// virtualenv's, else python3.
func pythonInterpreter(root string) string {
	py := "python3"
	for _, venv := range []string{".venv", "venv"} {
		if candidate := filepath.Join(root, venv, "bin", "python"); fileExists(candidate) {
			py = candidate
		}
	}
	return py
}

// pythonCommand runs the test files of tests that pytest collects, with
// pytest, or as their modules with unittest when pytest is not installed;
// with all, the whole suite. It runs nothing when none of tests is such a
// file.
func pythonCommand(path string, all bool, tests []string) Command {
	c := pythonBase(path)
	var run []string
	if !all {
		if run = coverage.PythonTests(c.Dir, tests); len(run) == 0 {
			return c
		}
	}
	py := pythonInterpreter(c.Root)
	c.Args = pythonArgs(py, exec.Command(py, "-c", "import pytest").Run() == nil, run)
	return c
}

// pythonArgs is the own-test command of python py: pytest, when installed,
// else unittest, over run, test files relative to the directory it runs
// in, or over the whole suite when run is nil.
func pythonArgs(py string, pytest bool, run []string) []string {
	switch {
	case pytest:
		return append([]string{py, "-m", "pytest", "-q", "-x", "-p", "no:cacheprovider"}, run...)
	case run != nil:
		return append([]string{py, "-m", "unittest", "-f"}, coverage.PythonModules(run)...)
	}
	return []string{py, "-m", "unittest", "discover", "-f"}
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
