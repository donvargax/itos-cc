package mutants_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/donvargax/itos-cc/internal/mutants"
)

func TestCommandPolicyPreservesObservedRunnerSelections(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "board")
	path := func(p string) string { return filepath.Join(root, filepath.FromSlash(p)) }
	for _, c := range []struct {
		name    string
		request mutants.CommandRequest
		facts   mutants.ToolFacts
		args    []string
		dir     string
		suite   bool
	}{
		{name: "Go package", request: mutants.CommandRequest{Path: path("pkg/a.go")}, facts: mutants.ToolFacts{Language: "go", Root: root}, args: []string{"go", "test", "-count=1", "-failfast", "./pkg"}},
		{name: "Go suite", request: mutants.CommandRequest{Path: path("a.go"), All: true}, facts: mutants.ToolFacts{Language: "go", Root: root}, args: []string{"go", "test", "-count=1", "-failfast", "./..."}},
		{name: "Vitest related", request: mutants.CommandRequest{Path: path("src/a.ts")}, facts: mutants.ToolFacts{Language: "typescript", Root: root, Vitest: "vitest"}, args: []string{"vitest", "related", "--run", "--bail=1", "src/a.ts"}},
		{name: "Vitest all", request: mutants.CommandRequest{Path: path("a.ts"), All: true}, facts: mutants.ToolFacts{Language: "typescript", Root: root, Vitest: "vitest"}, args: []string{"vitest", "run", "--bail=1"}},
		{name: "Jest related", request: mutants.CommandRequest{Path: path("a.ts")}, facts: mutants.ToolFacts{Language: "typescript", Root: root, Jest: "jest"}, args: []string{"jest", "--bail", "--findRelatedTests", "a.ts"}},
		{name: "Jest all", request: mutants.CommandRequest{Path: path("a.ts"), All: true}, facts: mutants.ToolFacts{Language: "typescript", Root: root, Jest: "jest"}, args: []string{"jest", "--bail"}},
		{name: "npm fallback", request: mutants.CommandRequest{Path: path("a.ts")}, facts: mutants.ToolFacts{Language: "typescript", Root: root, PackageManager: "npm"}, args: []string{"npm", "run", "test", "--silent"}, suite: true},
		{name: "pnpm fallback", request: mutants.CommandRequest{Path: path("a.ts")}, facts: mutants.ToolFacts{Language: "typescript", Root: root, PackageManager: "pnpm"}, args: []string{"pnpm", "run", "test"}, suite: true},
		{name: "Python pytest own", facts: mutants.ToolFacts{Language: "python", Root: root, Interpreter: "python3", Pytest: true, PythonFiles: []string{"test_a.py"}}, args: []string{"python3", "-m", "pytest", "-q", "-x", "-p", "no:cacheprovider", "test_a.py"}},
		{name: "Python unittest own", facts: mutants.ToolFacts{Language: "python", Root: root, Interpreter: "python3", PythonFiles: []string{"test_a.py"}, PythonModules: []string{"test_a"}}, args: []string{"python3", "-m", "unittest", "-f", "test_a"}},
		{name: "Python unittest suite", request: mutants.CommandRequest{All: true}, facts: mutants.ToolFacts{Language: "python", Root: root, Interpreter: "python3"}, args: []string{"python3", "-m", "unittest", "discover", "-f"}},
		{name: "Python unreferenced", facts: mutants.ToolFacts{Language: "python", Root: root}},
		{name: "Gradle whole", request: mutants.CommandRequest{All: true}, facts: mutants.ToolFacts{Language: "kotlin", Root: root, Runner: "./gradlew"}, args: []string{"./gradlew", "test", "--fail-fast"}},
		{name: "Gradle no classes", facts: mutants.ToolFacts{Language: "kotlin", Root: root, Runner: "gradle", Classes: map[string][]string{path("../outside"): {"Unrelated"}}}},
		{name: "Gradle own", facts: mutants.ToolFacts{Language: "kotlin", Root: root, Runner: "gradle", Classes: map[string][]string{path("core"): {"a.ATest"}}}, args: []string{"gradle", "-p", "core", "test", "--fail-fast", "--tests", "a.ATest"}},
		{name: "Gradle modules", facts: mutants.ToolFacts{Language: "kotlin", Root: root, Runner: "./gradlew", Classes: map[string][]string{path("core"): {"a.ATest"}, path("web"): {"b.BTest"}}}, args: []string{"./gradlew", ":core:test", "--fail-fast", "--tests", "a.ATest", ":web:test", "--fail-fast", "--tests", "b.BTest"}},
		{name: "Maven suite", request: mutants.CommandRequest{All: true}, facts: mutants.ToolFacts{Language: "kotlin", Root: root, Module: path("core"), Runner: "mvn"}, args: []string{"mvn", "-q", "test"}, dir: path("core")},
		{name: "Maven no classes", facts: mutants.ToolFacts{Language: "kotlin", Root: root, Module: path("core"), Reactor: root, Runner: "mvn"}, dir: path("core")},
		{name: "Maven own", facts: mutants.ToolFacts{Language: "kotlin", Module: path("core"), Reactor: root, Runner: "mvn", Classes: map[string][]string{path("core"): {"a.ATest"}}}, args: []string{"mvn", "-q", "test", "-Dtest=a.ATest", "-Dsurefire.failIfNoSpecifiedTests=false"}, dir: path("core")},
		{name: "Maven reactor", facts: mutants.ToolFacts{Language: "kotlin", Module: path("core"), Reactor: root, Runner: "mvn", Classes: map[string][]string{path("core"): {"a.ATest"}, path("web"): {"b.BTest"}}}, args: []string{"mvn", "-q", "test", "-pl", "core,web", "-am", "-Dtest=a.ATest,b.BTest", "-Dsurefire.failIfNoSpecifiedTests=false"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := mutants.ChooseCommand(c.request, c.facts)
			dir := c.dir
			if dir == "" {
				dir = root
			}
			if !reflect.DeepEqual(got.Args, c.args) || got.Dir != dir || got.Suite != c.suite {
				t.Fatalf("command %+v want args%v dir%s suite%v", got, c.args, dir, c.suite)
			}
		})
	}
	shell := mutants.ChooseCommand(mutants.CommandRequest{Shell: "./run.sh"}, mutants.ToolFacts{Language: "python", Root: root})
	if shell.Shell != "./run.sh" || shell.Args != nil || shell.PathEnv != "PYTHONPATH" || !reflect.DeepEqual(shell.PathDirs, []string{".", "src"}) {
		t.Fatalf("shell command %+v", shell)
	}
}
