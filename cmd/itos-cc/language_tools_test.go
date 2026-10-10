package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The real TypeScript, Python and Kotlin tools the tests that need them run,
// found by languageTools. CI installs pinned ones on ubuntu-latest only
// (T-13): Node's from testdata/tools/node with npm ci, Python's from
// testdata/tools/python/requirements.txt, and a JDK with Gradle and Maven.
// Elsewhere a test that needs a tool skips, naming it.

const (
	// languageToolsEnv, set by the job that installed the pinned tools, turns
	// a missing tool into a failure, so a broken install is never a skip.
	languageToolsEnv = "ITOS_CC_LANGUAGE_TOOLS"
	// nodeModulesEnv names a node_modules that holds vitest and
	// @vitest/coverage-v8.
	nodeModulesEnv = "ITOS_CC_NODE_MODULES"
)

// languageTools finds the tools language's tests run, or skips the test with
// the missing one named. For "typescript" it returns the absolute
// node_modules that holds vitest and @vitest/coverage-v8: the one
// ITOS_CC_NODE_MODULES names, else testdata/tools/node's, else the viewer's.
// For "python" it needs python3 with pytest and coverage importable, for
// "kotlin" java and gradle, for "maven", Kotlin's other build, java and
// mvn, and for "node" node and npm alone, a node that runs .ts files by
// stripping their types (22.18 or later); it returns "" for each.
//
// A CI job that did not install the pinned tools skips: its runner's
// preinstalled ones are whatever version its image has.
func languageTools(t *testing.T, language string) string {
	t.Helper()
	required := os.Getenv(languageToolsEnv) != ""
	missing := func(what string) {
		t.Helper()
		if required {
			t.Fatalf("%s is not installed, and %s says the language tools are", what, languageToolsEnv)
		}
		t.Skip(what, "is not installed")
	}
	if !required && os.Getenv("CI") != "" {
		t.Skipf("CI installs the %s tools on ubuntu-latest only, where it sets %s", language, languageToolsEnv)
	}
	switch language {
	case "typescript":
		if _, err := exec.LookPath("node"); err != nil {
			missing("node")
		}
		candidates := []string{
			filepath.Join("..", "..", "testdata", "tools", "node", "node_modules"),
			filepath.Join("..", "..", "viewer", "node_modules"),
		}
		if dir := os.Getenv(nodeModulesEnv); dir != "" {
			candidates = []string{dir}
		}
		for _, dir := range candidates {
			dir, err := filepath.Abs(dir)
			if err != nil {
				t.Fatal(err)
			}
			if installed(dir, "vitest") && installed(dir, "@vitest/coverage-v8") {
				return dir
			}
		}
		missing("vitest with @vitest/coverage-v8 (npm ci in testdata/tools/node, or " + nodeModulesEnv + ")")
	case "python":
		if _, err := exec.LookPath("python3"); err != nil {
			missing("python3")
		}
		for _, module := range []string{"pytest", "coverage"} {
			if exec.Command("python3", "-c", "import "+module).Run() != nil {
				missing(module + " for python3")
			}
		}
	case "kotlin":
		for _, tool := range []string{"java", "gradle"} {
			if _, err := exec.LookPath(tool); err != nil {
				missing(tool)
			}
		}
	case "node":
		for _, tool := range []string{"node", "npm"} {
			if _, err := exec.LookPath(tool); err != nil {
				missing(tool)
			}
		}
		out, err := exec.Command("node", "--version").Output()
		if err != nil {
			missing("node")
		}
		parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(string(out)), "v"), ".")
		major, _ := strconv.Atoi(parts[0])
		minor := 0
		if len(parts) > 1 {
			minor, _ = strconv.Atoi(parts[1])
		}
		if major < 22 || major == 22 && minor < 18 {
			missing("a node that strips TypeScript types by default (22.18 or later; this is " + strings.TrimSpace(string(out)) + ")")
		}
	case "maven":
		for _, tool := range []string{"java", "mvn"} {
			if _, err := exec.LookPath(tool); err != nil {
				missing(tool)
			}
		}
	default:
		t.Fatalf("no tools known for %s", language)
	}
	return ""
}

// installed says whether modules holds the package name.
func installed(modules, name string) bool {
	_, err := os.Stat(filepath.Join(modules, filepath.FromSlash(name), "package.json"))
	return err == nil
}
