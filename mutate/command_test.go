package mutate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTypeScriptTestsRunFromNodeModules(t *testing.T) {
	dir := t.TempDir()
	vitest := filepath.Join(dir, "node_modules", ".bin", "vitest")
	if runtime.GOOS == "windows" {
		vitest += ".cmd"
	}
	src := filepath.Join(dir, "src", "a.ts")
	for path, text := range map[string]string{
		filepath.Join(dir, "package.json"):   `{"devDependencies": {"vitest": "*"}}`,
		filepath.Join(dir, "pnpm-lock.yaml"): "",
		src:                                  "",
	} {
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte(text), 0o644)
	}
	if got := TestCommand(src, "", false, nil).String(); got != "pnpm run test" {
		t.Errorf("without vitest installed: %q, want the project's own test script", got)
	}
	os.MkdirAll(filepath.Dir(vitest), 0o755)
	os.WriteFile(vitest, nil, 0o755)
	if got := TestCommand(src, "", false, nil); got.Args[0] != vitest || got.Args[1] != "related" {
		t.Errorf("with vitest installed: %q, want %s related", got.Args, vitest)
	}
}

func TestGoMutantsRunTheirOwnPackagesTestsOrTheWholeSuite(t *testing.T) {
	dir := t.TempDir()
	for name, text := range map[string]string{
		"go.mod":            "module example.com/m\n\ngo 1.22\n",
		"a/a.go":            "package a\n\nfunc A() int { return 1 }\n",
		"e2e/e2e_test.go":   "package e2e\n\nimport (\n\t\"testing\"\n\n\t\"example.com/m/a\"\n)\n\nfunc TestFlow(t *testing.T) { a.A() }\n",
		"slow/slow_test.go": "package slow\n\nimport \"testing\"\n\nfunc TestSlow(t *testing.T) {}\n",
	} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte(text), 0o644)
	}
	src := filepath.Join(dir, "a", "a.go")
	if got := TestCommand(src, "", false, nil).String(); got != "go test -count=1 -failfast ./a" {
		t.Errorf("default: %q, want a's own tests", got)
	}
	if got := TestCommand(src, "", true, nil).String(); got != "go test -count=1 -failfast ./..." {
		t.Errorf("all tests: %q, want the whole module, e2e tests included", got)
	}
}

func TestPythonMutantsRunTheTestFilesThatReachTheirFile(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"pyproject.toml", "a.py", "tests/test_a.py", "conftest.py"} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, nil, 0o644)
	}
	src := filepath.Join(dir, "a.py")
	reach := []string{filepath.Join(dir, "conftest.py"), filepath.Join(dir, "tests", "test_a.py")}
	got := TestCommand(src, "", false, reach)
	args := strings.Join(got.Args, " ")
	if !strings.HasSuffix(args, " -m pytest -q -x -p no:cacheprovider tests/test_a.py") && !strings.HasSuffix(args, " -m unittest -f tests.test_a") {
		t.Errorf("own tests: %q, want pytest over tests/test_a.py, or unittest over tests.test_a", args)
	}
	if got.Dir != dir || got.RunsNothing() {
		t.Errorf("own tests: %+v, want them run in %s", got, dir)
	}
	if none := TestCommand(src, "", false, reach[:1]); !none.RunsNothing() || none.Dir != dir {
		t.Errorf("reached by conftest.py alone: %+v, want nothing run", none)
	}
	if all := strings.Join(TestCommand(src, "", true, reach).Args, " "); strings.Contains(all, "test_a") {
		t.Errorf("all tests: %q, want the whole suite", all)
	}
	if given := TestCommand(src, "make check", false, nil); given.String() != "make check" || given.Dir != dir {
		t.Errorf("--test-command: %+v, want make check run in %s", given, dir)
	}
}
