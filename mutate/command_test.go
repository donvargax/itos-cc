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

// kotlinFiles writes files, by slash path and text, beneath dir, and
// returns dir.
func kotlinFiles(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	for name, text := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// kotlinTestFiles are test files of package pkg beneath test, a test
// directory: a test class, a subclass of an abstract test class that
// declares no test of its own, a Kotest spec, a JUnit 5 class whose tests
// are nested, and a test-support helper object and class that declare no
// test.
func kotlinTestFiles(test, pkg string) map[string]string {
	header := "package " + pkg + "\n\nimport kotlin.test.Test\n\n"
	return map[string]string{
		test + "/ATest.kt":    header + "class ATest {\n    @Test\n    fun low() {}\n}\n",
		test + "/BaseTest.kt": header + "abstract class BaseTest {\n    @Test\n    fun inherited() {}\n}\n",
		test + "/SubTest.kt":  header + "class SubTest : BaseTest()\n",
		test + "/LowSpec.kt":  "package " + pkg + "\n\nclass LowSpec : FunSpec({ test(\"low\") {} })\n",
		test + "/Outer.kt":    header + "class Outer {\n    @Nested\n    inner class Inner {\n        @ParameterizedTest\n        fun low(i: Int) {}\n    }\n}\n",
		test + "/Record.kt":   "package " + pkg + "\n\nobject Record {\n    fun ran(name: String) {}\n}\n",
		test + "/Fixtures.kt": "package " + pkg + "\n\nclass Fixtures {\n    fun low() = 3\n}\n",
		test + "/Contract.kt": header + "interface Contract {\n    @Test\n    fun holds() {}\n}\n",
	}
}

func TestKotlinMutantsRunTheTestClassesThatReachTheirFile(t *testing.T) {
	test := "src/test/kotlin/own"
	files := kotlinTestFiles(test, "own")
	files["settings.gradle.kts"] = "rootProject.name = \"own\"\n"
	files["build.gradle.kts"] = ""
	files["src/main/kotlin/own/A.kt"] = "package own\n\nfun low(i: Int) = i == 3\n"
	dir := kotlinFiles(t, t.TempDir(), files)
	src := filepath.Join(dir, "src", "main", "kotlin", "own", "A.kt")
	var reach []string
	for name := range kotlinTestFiles(test, "own") {
		reach = append(reach, filepath.Join(dir, filepath.FromSlash(name)))
	}
	got := TestCommand(src, "", false, reach)
	want := "gradle -p . test --fail-fast --tests own.ATest --tests own.LowSpec --tests own.Outer --tests own.SubTest"
	if got.String() != want || got.Dir != dir || got.Root != dir {
		t.Errorf("own tests: %q in %s, want %q in %s: the test classes the reaching files declare, and no helper",
			got.String(), got.Dir, want, dir)
	}
	helpers := []string{filepath.Join(dir, filepath.FromSlash(test+"/Record.kt")), filepath.Join(dir, filepath.FromSlash(test+"/Fixtures.kt"))}
	if none := TestCommand(src, "", false, helpers); !none.RunsNothing() || none.Dir != dir {
		t.Errorf("reached by helpers alone: %+v, want nothing run", none)
	}
	if none := TestCommand(src, "", false, nil); !none.RunsNothing() {
		t.Errorf("reached by no test: %+v, want nothing run", none)
	}
	if all := TestCommand(src, "", true, reach).String(); all != "gradle test --fail-fast" {
		t.Errorf("all tests: %q, want the whole build", all)
	}
	if given := TestCommand(src, "make check", false, reach); given.String() != "make check" || given.Dir != dir {
		t.Errorf("--test-command: %+v, want make check run in %s", given, dir)
	}
}

func TestKotlinTestClassesOfSeveralGradleModulesRunAsEachModulesTestTask(t *testing.T) {
	header := "import kotlin.test.Test\n\n"
	dir := kotlinFiles(t, t.TempDir(), map[string]string{
		"settings.gradle.kts":                       "include(\"lib\", \"apps:cli\")\n",
		"lib/build.gradle.kts":                      "",
		"apps/cli/build.gradle.kts":                 "",
		"lib/src/main/kotlin/lib/Low.kt":            "package lib\n\nfun low(i: Int) = i == 3\n",
		"lib/src/test/kotlin/lib/LowTest.kt":        "package lib\n\n" + header + "class LowTest {\n    @Test\n    fun low() {}\n}\n",
		"apps/cli/src/test/kotlin/cli/CliTest.kt":   "package cli\n\n" + header + "class CliTest {\n    @Test\n    fun low() {}\n}\n",
		"apps/cli/src/test/kotlin/cli/CliRecord.kt": "package cli\n\nobject CliRecord\n",
	})
	src := filepath.Join(dir, "lib", "src", "main", "kotlin", "lib", "Low.kt")
	at := func(name string) string { return filepath.Join(dir, filepath.FromSlash(name)) }
	own := TestCommand(src, "", false, []string{at("lib/src/test/kotlin/lib/LowTest.kt"), at("apps/cli/src/test/kotlin/cli/CliRecord.kt")})
	if want := "gradle -p lib test --fail-fast --tests lib.LowTest"; own.String() != want || own.Dir != dir {
		t.Errorf("one module's classes: %q in %s, want %q in %s", own.String(), own.Dir, want, dir)
	}
	both := TestCommand(src, "", false, []string{at("apps/cli/src/test/kotlin/cli/CliTest.kt"), at("lib/src/test/kotlin/lib/LowTest.kt")})
	if want := "gradle :apps:cli:test --fail-fast --tests cli.CliTest :lib:test --fail-fast --tests lib.LowTest"; both.String() != want || both.Dir != dir {
		t.Errorf("two modules' classes: %q in %s, want %q in %s", both.String(), both.Dir, want, dir)
	}
}

func TestKotlinTestClassesRunWithMavenThroughSurefire(t *testing.T) {
	header := "import kotlin.test.Test\n\n"
	dir := kotlinFiles(t, t.TempDir(), map[string]string{
		"pom.xml":                            "<project/>",
		"lib/pom.xml":                        "<project/>",
		"app/pom.xml":                        "<project/>",
		"lib/src/main/kotlin/lib/Low.kt":     "package lib\n\nfun low(i: Int) = i == 3\n",
		"lib/src/test/kotlin/lib/LowTest.kt": "package lib\n\n" + header + "class LowTest {\n    @Test\n    fun low() {}\n}\n",
		"lib/src/test/kotlin/lib/Helper.kt":  "package lib\n\nobject Helper\n",
		"app/src/test/kotlin/app/AppTest.kt": "package app\n\n" + header + "class AppTest {\n    @Test\n    fun low() {}\n}\n",
	})
	src := filepath.Join(dir, "lib", "src", "main", "kotlin", "lib", "Low.kt")
	lib := filepath.Join(dir, "lib")
	at := func(name string) string { return filepath.Join(dir, filepath.FromSlash(name)) }
	own := TestCommand(src, "", false, []string{at("lib/src/test/kotlin/lib/LowTest.kt"), at("lib/src/test/kotlin/lib/Helper.kt")})
	if want := "mvn -q test -Dtest=lib.LowTest -Dsurefire.failIfNoSpecifiedTests=false"; own.String() != want || own.Dir != lib || own.Root != lib {
		t.Errorf("its module's classes: %q in %s, want %q in %s", own.String(), own.Dir, want, lib)
	}
	both := TestCommand(src, "", false, []string{at("app/src/test/kotlin/app/AppTest.kt"), at("lib/src/test/kotlin/lib/LowTest.kt")})
	if want := "mvn -q test -pl lib,app -am -Dtest=app.AppTest,lib.LowTest -Dsurefire.failIfNoSpecifiedTests=false"; both.String() != want || both.Dir != dir || both.Root != dir {
		t.Errorf("two modules' classes: %q in %s, want %q from the reactor's top, %s", both.String(), both.Dir, want, dir)
	}
	if none := TestCommand(src, "", false, []string{at("lib/src/test/kotlin/lib/Helper.kt")}); !none.RunsNothing() {
		t.Errorf("reached by a helper alone: %+v, want nothing run", none)
	}
	if all := TestCommand(src, "", true, nil); all.String() != "mvn -q test" || all.Dir != lib {
		t.Errorf("all tests: %+v, want its module's whole suite", all)
	}
}

func TestATypeScriptScriptWithoutVitestOrJestRunsTheWholeSuite(t *testing.T) {
	dir := kotlinFiles(t, t.TempDir(), map[string]string{
		"package.json":                    `{"scripts": {"test": "node --test"}}`,
		"src/a.ts":                        "export const a = 1;\n",
		"vitest/package.json":             `{"devDependencies": {"vitest": "*"}}`,
		"vitest/a.ts":                     "export const a = 1;\n",
		"vitest/node_modules/.bin/vitest": "",
		"py/pyproject.toml":               "",
		"py/a.py":                         "",
	})
	src := filepath.Join(dir, "src", "a.ts")
	if c := TestCommand(src, "", false, nil); !c.Suite || !strings.HasSuffix(c.String(), " run test") && !strings.HasSuffix(c.String(), " run test --silent") {
		t.Errorf("own tests: %+v, want the test script, marked as the whole suite", c)
	}
	if got := OwnScope(src); got != ScopeAllTests {
		t.Errorf("OwnScope of a script without Vitest or Jest: %q, want %q", got, ScopeAllTests)
	}
	if got := FileScope(src, "make check", false); got != "make check" {
		t.Errorf("FileScope with --test-command: %q, want the command", got)
	}
	vitest := filepath.Join(dir, "vitest", "a.ts")
	if runtime.GOOS != "windows" {
		if got := OwnScope(vitest); got != ScopeOwn || TestCommand(vitest, "", false, nil).Suite {
			t.Errorf("OwnScope with Vitest installed: %q, want %q", got, ScopeOwn)
		}
	}
	if got := OwnScope(filepath.Join(dir, "py", "a.py")); got != ScopeOwn {
		t.Errorf("OwnScope of a Python file: %q, want %q", got, ScopeOwn)
	}
}
