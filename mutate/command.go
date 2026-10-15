package mutate

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/project"
)

// TestCommand is the narrowest test run that covers path: its own Go
// package, the Vitest or Jest tests that import it, in Python tests, the
// test files that reach it (graph.TestsImporting), in Kotlin the test
// classes those files declare (coverage.KotlinTests), or the whole suite
// where no narrower run exists. A Python file none of whose tests is a test
// file pytest collects (coverage.PythonTests), or a Kotlin file none of
// whose tests declares a test class, runs nothing (RunsNothing). With
// all, it is the whole suite of path's build root, so integration and
// end-to-end tests anywhere in it can kill a mutant. Every command stops at
// the first failure, since one failing test is enough to kill a mutant. A
// non-empty shell overrides the command but not where it runs.
func TestCommand(ctx context.Context, path, shell string, all bool, tests []string) Command {
	spec := lang.Detect(path)
	var c Command
	switch spec.Name {
	case "go":
		c = goCommand(ctx, path, all)
	case "typescript":
		c = typescriptCommand(ctx, path, all)
	case "python":
		if shell != "" {
			// Only where it runs: the probe of pytest is for nothing.
			c = pythonBase(ctx, path)
			break
		}
		c = pythonCommand(ctx, path, all, tests)
	case "kotlin":
		c = kotlinCommand(ctx, path, all, tests)
	}
	if shell != "" {
		c.Args, c.Shell = nil, shell
	}
	return c
}

// goCommand runs the tests of path's own package, or with all the whole
// module, integration and end-to-end tests included.
func goCommand(ctx context.Context, path string, all bool) Command {
	root := orDir(project.FindUp(ctx, path, "go.mod"), path)
	pkg, _ := filepath.Rel(root, filepath.Dir(path))
	target := "./" + filepath.ToSlash(pkg)
	if all {
		target = "./..."
	}
	return Command{Root: root, Dir: root, Args: []string{"go", "test", "-count=1", "-failfast", target}}
}

func typescriptCommand(ctx context.Context, path string, all bool) Command {
	root := orDir(project.FindUp(ctx, path, "package.json"), path)
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
		c.Suite = true
	}
	return c
}

// OwnScope is the scope of path's own tests: ScopeAllTests when they cannot
// be narrowed and run the whole suite, as a TypeScript file's do when its
// project's test script runs without Vitest or Jest installed, else
// ScopeOwn. An outcome they decide rests on the whole suite, as a
// --test-command outcome does, and is recorded with its evidence.
func OwnScope(ctx context.Context, path string) string {
	if spec := lang.Detect(path); spec != nil && spec.Name == "typescript" && typescriptCommand(ctx, path, false).Suite {
		return ScopeAllTests
	}
	return ScopeOwn
}

// FileScope is RunScope for path: with neither shell nor all, its own
// tests' scope (OwnScope).
func FileScope(ctx context.Context, path, shell string, all bool) string {
	if scope := RunScope(shell, all); scope != ScopeOwn {
		return scope
	}
	return OwnScope(ctx, path)
}

// pythonBase is where path's Python tests run, with no command yet.
func pythonBase(ctx context.Context, path string) Command {
	root := orDir(project.FindUp(ctx, path, "pyproject.toml", "setup.py", "setup.cfg"), path)
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
func pythonCommand(ctx context.Context, path string, all bool, tests []string) Command {
	c := pythonBase(ctx, path)
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

// kotlinCommand runs the test classes the files of tests declare
// (coverage.KotlinTests), with Gradle or Maven, in the build of path's
// module; with all, its build's whole suite. It runs nothing when they
// declare none in that build.
func kotlinCommand(ctx context.Context, path string, all bool, tests []string) Command {
	module := orDir(project.FindUp(ctx, path, "build.gradle.kts", "build.gradle", "pom.xml"), path)
	if fileExists(filepath.Join(module, "pom.xml")) {
		return mavenCommand(ctx, module, all, tests)
	}
	// Gradle modules need the build root that holds settings and the wrapper.
	root := orDir(project.FindUp(ctx, path, "settings.gradle.kts", "settings.gradle", "gradlew"), module)
	gradle := "gradle"
	if fileExists(filepath.Join(root, "gradlew")) {
		gradle = "./gradlew"
	}
	c := Command{Root: root, Dir: root}
	if all {
		c.Args = []string{gradle, "test", "--fail-fast"}
		return c
	}
	classes := within(root, coverage.KotlinTests(ctx, tests))
	switch len(classes) {
	case 0:
		return c
	case 1:
		// One module's: its test task, as the project at its directory.
		for dir, names := range classes {
			rel, _ := filepath.Rel(root, dir)
			c.Args = append([]string{gradle, "-p", filepath.ToSlash(rel), "test", "--fail-fast"}, coverage.GradleTests(names)...)
		}
		return c
	}
	// Several modules': each one's test task by its project path, which
	// Gradle names after its directory, so the first failure stops the
	// build.
	c.Args = []string{gradle}
	for _, dir := range slices.Sorted(maps.Keys(classes)) {
		rel, _ := filepath.Rel(root, dir)
		task := ":test"
		if rel != "." {
			task = ":" + strings.ReplaceAll(filepath.ToSlash(rel), "/", ":") + task
		}
		c.Args = append(append(c.Args, task, "--fail-fast"), coverage.GradleTests(classes[dir])...)
	}
	return c
}

// mavenCommand runs, in the Maven module at module, the test classes the
// files of tests declare; with all, its whole suite. Classes in other
// modules of its reactor, the directories above it that hold a pom.xml,
// run from the reactor's top with those modules and the ones they depend
// on built, so they test module's sources and not an installed copy.
func mavenCommand(ctx context.Context, module string, all bool, tests []string) Command {
	c := Command{Root: module, Dir: module}
	if all {
		c.Args = []string{"mvn", "-q", "test"}
		return c
	}
	top := module
	for dir := filepath.Dir(module); dir != filepath.Dir(dir) && fileExists(filepath.Join(dir, "pom.xml")); dir = filepath.Dir(dir) {
		top = dir
	}
	classes := within(top, coverage.KotlinTests(ctx, tests))
	var names, others []string
	for _, dir := range slices.Sorted(maps.Keys(classes)) {
		names = append(names, classes[dir]...)
		if dir != module {
			rel, _ := filepath.Rel(top, dir)
			others = append(others, filepath.ToSlash(rel))
		}
	}
	if len(names) == 0 {
		return c
	}
	slices.Sort(names)
	c.Args = []string{"mvn", "-q", "test"}
	if len(others) > 0 {
		c.Root, c.Dir = top, top
		var pl []string
		if classes[module] != nil {
			rel, _ := filepath.Rel(top, module)
			pl = append(pl, filepath.ToSlash(rel))
		}
		c.Args = append(c.Args, "-pl", strings.Join(append(pl, others...), ","), "-am")
	}
	c.Args = append(c.Args, coverage.MavenTests(slices.Compact(names))...)
	return c
}

// within is the modules of classes inside root, root's own included.
func within(root string, classes map[string][]string) map[string][]string {
	out := map[string][]string{}
	for dir, names := range classes {
		if rel, err := filepath.Rel(root, dir); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			out[dir] = names
		}
	}
	return out
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
