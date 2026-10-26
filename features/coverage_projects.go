package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Writing the projects the coverage scenarios measure.

// writeIn writes files, by slash-separated path, in the folder dir of the
// scratch folder.
func (c *coverageState) writeIn(dir string, files map[string]string) error {
	for name, text := range files {
		if err := c.w.write(path.Join(dir, name), text); err != nil {
			return err
		}
	}
	return nil
}

// abs is the absolute path of rel, slash-separated from the scratch folder,
// in the system's form.
func (c *coverageState) abs(rel string) string {
	return filepath.Join(c.w.dir, filepath.FromSlash(rel))
}

// declare declares what the project's own tools measure in the folder dir
// of the scratch folder, a build root of language.
func (c *coverageState) declare(dir string, cov Coverage) error {
	c.roots[dir] = cov.Language
	return c.w.d.Coverage(c.abs(dir), cov)
}

// markerTest is a Go test of package pkg that writes test-ran in its
// package's folder when it runs, then calls Place with a.
func markerTest(pkg string, a int) string {
	return "package " + pkg + "\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestPlace(t *testing.T) {\n" +
		"\t_ = os.WriteFile(\"test-ran\", nil, 0o644)\n" + fmt.Sprintf("\tif Place(%d) < 0 {\n\t\tt.Fatal(\"Place\")\n\t}\n}\n", a)
}

// sampleTest is the sample's test of package a: it writes test-ran in a/
// when it runs, then expects A(1) to be want.
func sampleTest(want int) string {
	return "package a\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestA(t *testing.T) {\n" +
		"\t_ = os.WriteFile(\"test-ran\", nil, 0o644)\n" + fmt.Sprintf("\tif A(1) != %d {\n\t\tt.Fatal(\"A\")\n\t}\n}\n", want)
}

// sampleA is the sample's a/a.go: A runs from line 3 to 8, an if on line 4.
const sampleA = "package a\n\nfunc A(x int) int {\n\tif x > 0 {\n\t\treturn 1\n\t}\n\treturn 0\n}\n"

// sampleRecipe is what the sample's Makefile's cover target runs.
const sampleRecipe = "go test -coverprofile=cover.out ./..."

// sample is the project the scenarios that only say When I run, or that
// coverage is run, run in: the Go module example.com/m, whose a/a.go its
// test covers and b/b.go no test runs, beside a/lcov.info and
// b/coverage.out, reports a person wrote of each, and a Makefile whose
// cover target writes cover.out; and a second module, example.com/other, in
// other/. Each test writes test-ran in its folder when it runs. args is the
// run's words: a --coverage-command run with --coverage-report declares the
// program it runs.
func (c *coverageState) sample(args string) error {
	if err := c.writeIn(".", map[string]string{
		"go.mod":          "module example.com/m\n\ngo 1.22\n",
		"a/a.go":          sampleA,
		"a/a_test.go":     sampleTest(1),
		"b/b.go":          "package b\n\nfunc B(x int) int {\n\treturn x + 1\n}\n",
		"a/lcov.info":     lcovRecord("a/a.go", lcovLines([2]int{4, 1}, [2]int{5, 1}, [2]int{7, 0}), nil),
		"b/coverage.out":  "mode: set\nexample.com/m/b/b.go:3.20,5.2 1 1\n",
		"Makefile":        "cover:\n\t" + sampleRecipe + "\n",
		"other/go.mod":    "module example.com/other\n\ngo 1.22\n",
		"other/o.go":      goSource("other"),
		"other/o_test.go": markerTest("other", 5),
	}); err != nil {
		return err
	}
	c.markers = []string{"a/test-ran", "other/test-ran"}
	c.want[covFunction{"a/a.go", "A"}] = 100 * 2.0 / 3
	c.want[covFunction{"b/b.go", "B"}] = 100
	words, err := commandWords(args)
	if err != nil {
		return err
	}
	program := ""
	if i := slices.Index(words, "--coverage-command"); i >= 0 && i+1 < len(words) && slices.Contains(words, "--coverage-report") {
		program, _, _ = strings.Cut(words[i+1], " ")
	}
	if err := c.declare(".", Coverage{Language: "go", Program: program}); err != nil {
		return err
	}
	return c.declare("other", Coverage{Language: "go"})
}

// failingSampleTest writes the sample, its test of a/a.go expecting what A
// does not give.
func (c *coverageState) failingSampleTest() error {
	if err := c.sample(""); err != nil {
		return err
	}
	return c.w.write("a/a_test.go", sampleTest(2))
}

// e2eModule writes the module example.com/m whose a/a.go its own test
// covers one arm of and e2e/e2e_test.go, importing package a, the other.
func (c *coverageState) e2eModule() error {
	if err := c.writeIn(".", map[string]string{
		"go.mod":      "module example.com/m\n\ngo 1.22\n",
		"a/a.go":      sampleA,
		"a/a_test.go": sampleTest(1),
		"e2e/e2e_test.go": "package e2e\n\nimport (\n\t\"testing\"\n\n\t\"example.com/m/a\"\n)\n\n" +
			"func TestE2E(t *testing.T) {\n\tif a.A(-1) != 0 {\n\t\tt.Fatal(\"A\")\n\t}\n}\n",
	}); err != nil {
		return err
	}
	return c.declare(".", Coverage{Language: "go"})
}

// ownModule writes the module example.com/m whose a/a.go its own test
// covers, beside the files besides, by slash-separated path.
func (c *coverageState) ownModule(besides map[string]string) error {
	files := map[string]string{
		"go.mod":      "module example.com/m\n\ngo 1.22\n",
		"a/a.go":      sampleA,
		"a/a_test.go": sampleTest(1),
	}
	for name, text := range besides {
		files[name] = text
	}
	if err := c.writeIn(".", files); err != nil {
		return err
	}
	return c.declare(".", Coverage{Language: "go"})
}

// ignoredGenerator writes the module of ownModule with tools/gen.go, a
// program built only under the ignore constraint, as go generate's
// generators are, so go list ./... lists no package in tools/.
func (c *coverageState) ignoredGenerator() error {
	return c.ownModule(map[string]string{
		"tools/gen.go": "//go:build ignore\n\npackage main\n\nfunc main() {\n\tprintln(\"gen\")\n}\n",
	})
}

// unresolvableImport writes the module of ownModule with b/b.go, which
// imports example.com/m/nope, a package no folder of the module holds and
// no module provides, so go list ./... fails.
func (c *coverageState) unresolvableImport() error {
	return c.ownModule(map[string]string{
		"b/b.go": "package b\n\nimport \"example.com/m/nope\"\n\nfunc B() int {\n\treturn nope.N\n}\n",
	})
}

// slowTest writes slow/slow_test.go, a test importing nothing of the
// module.
func (c *coverageState) slowTest() error {
	text := "package slow\n\nimport \"testing\"\n\nfunc TestSlow(t *testing.T) {\n\tt.Log(\"slow\")\n}\n"
	if strings.Contains(text, "example.com/m/a") {
		return errors.New("slow/slow_test.go loads package a")
	}
	return c.w.write("slow/slow_test.go", text)
}

// vitestProject writes a TypeScript project in the folder dir of the
// scratch folder using Vitest and its v8 provider, its test of
// src/board.ts calling place, and declares Vitest measures it, linking the
// pinned node_modules there unless modules says they are elsewhere.
func (c *coverageState) vitestProject(dir string, modules bool) error {
	if err := c.writeIn(dir, map[string]string{
		"package.json": `{"name": "board", "type": "module", "devDependencies": {"vitest": "5.0.2", "@vitest/coverage-v8": "5.0.2"}}` + "\n",
		"src/board.ts": tsSource,
		"src/board.test.ts": "import { expect, test } from \"vitest\";\nimport { place } from \"./board\";\n\n" +
			"test(\"place\", () => {\n  expect(place(5)).toBe(3);\n});\n",
	}); err != nil {
		return err
	}
	if !modules {
		c.roots[dir] = "typescript"
		return nil
	}
	return c.declare(dir, Coverage{Language: "typescript"})
}

// runInVitestProject writes a Vitest project and runs itos-cc there with
// args.
func (c *coverageState) runInVitestProject(args string) error {
	if err := c.vitestProject(".", true); err != nil {
		return err
	}
	return c.w.runWith(c.w.env(), args)
}

// projectWithMarker writes a project of language at the scratch folder
// holding marker, with a source and, but for Kotlin, a test of it. Go and
// Python declare their tools; a Kotlin project's runs find neither Gradle
// nor Maven, so its command is chosen and never runs; a Dart project's
// tools are declared once its setup makes it a Flutter app.
func (c *coverageState) projectWithMarker(language, marker string) error {
	files := map[string]string{}
	switch language {
	case "Go":
		files["go.mod"], files["board.go"], files["board_test.go"] = "module example.com/m\n\ngo 1.22\n", goSource("board"), markerTest("board", 5)
	case "TypeScript":
		files["package.json"], files["src/board.ts"] = `{"name": "board"}`+"\n", tsSource
	case "Python":
		files[marker], files["board.py"], files["test_board.py"] = pythonMarker(marker), pySource, pythonTest
	case "Kotlin":
		files[marker], files["src/main/kotlin/board/Board.kt"] = kotlinBuild(marker), ktSource
		c.w.without = []string{"gradle", "mvn"}
	case "Dart":
		files = c.dartProject()
	}
	if _, ok := files[marker]; !ok {
		return fmt.Errorf("no %s project is written whose build root holds %s", language, marker)
	}
	if err := c.writeIn(".", files); err != nil {
		return err
	}
	switch language {
	case "Go":
		return c.declare(".", Coverage{Language: "go"})
	case "TypeScript":
		c.roots["."], c.pm = "typescript", "npm"
	case "Kotlin":
		c.roots["."] = "kotlin"
	}
	return nil
}

// pythonTest is a test of pySource's place, as unittest and pytest both run
// it.
const pythonTest = "import unittest\n\nfrom board import place\n\n\nclass TestPlace(unittest.TestCase):\n" +
	"    def test_place(self):\n        self.assertEqual(place(5), 1)\n"

// pythonMarker is a Python build root's marker file.
func pythonMarker(marker string) string {
	if marker == "setup.py" {
		return "from setuptools import setup\n\nsetup(name=\"board\")\n"
	}
	return "[project]\nname = \"board\"\nversion = \"0.1.0\"\n"
}

// kotlinBuild is a Kotlin build file, mentioning neither Kover nor JaCoCo.
func kotlinBuild(marker string) string {
	switch marker {
	case "build.gradle.kts":
		return "plugins {\n    kotlin(\"jvm\") version \"2.2.0\"\n}\n"
	case "build.gradle":
		return "plugins {\n    id 'org.jetbrains.kotlin.jvm' version '2.2.0'\n}\n"
	}
	return "<project>\n  <modelVersion>4.0.0</modelVersion>\n  <groupId>board</groupId>\n  <artifactId>board</artifactId>\n  <version>1</version>\n</project>\n"
}

// buildMarkers are the files that make a folder a build root.
var buildMarkers = []string{"go.mod", "package.json", "pyproject.toml", "setup.py", "setup.cfg", "build.gradle.kts", "build.gradle", "pom.xml", "pubspec.yaml"}

// nothingElse is whether the project's folder holds no build root's marker
// but its own and no Makefile or package.json.
func (c *coverageState) nothingElse() error {
	n := 0
	for _, marker := range append(buildMarkers, "Makefile") {
		if c.w.d.Exists(c.abs(path.Join(c.project, marker))) {
			n++
		}
	}
	if n != 1 {
		return fmt.Errorf("the project's folder holds %d build files, not its one marker", n)
	}
	return nil
}

// packageJSON writes the TypeScript project's package.json with scripts
// and devDependencies.
func (c *coverageState) packageJSON(dir string, scripts, deps map[string]string) error {
	object := map[string]any{"name": "board"}
	if scripts != nil {
		object["scripts"] = scripts
	}
	if deps != nil {
		object["devDependencies"] = deps
	}
	data, err := json.Marshal(object)
	if err != nil {
		return err
	}
	return c.w.write(path.Join(dir, "package.json"), string(data)+"\n")
}

// coverageScript gives the project a coverage script, which writes
// coverage/lcov.info: place's statements all run but the if's arm.
func (c *coverageState) coverageScript() error {
	lcov := lcovRecord("src/board.ts", lcovLines([2]int{2, 1}, [2]int{3, 1}, [2]int{4, 0}, [2]int{6, 1}, [2]int{7, 1}), nil)
	script := "const fs = require(\"fs\");\nfs.mkdirSync(\"coverage\", { recursive: true });\nfs.writeFileSync(\"coverage/lcov.info\", " +
		strconv.Quote(lcov) + ");\n"
	if err := c.packageJSON(".", map[string]string{"coverage": "node write-lcov.cjs"}, nil); err != nil {
		return err
	}
	if err := c.w.write("write-lcov.cjs", script); err != nil {
		return err
	}
	c.want[covFunction{"src/board.ts", "place"}] = 80
	return c.declare(".", Coverage{Language: "typescript", Report: lcov})
}

// vitestDependency makes Vitest of the pinned version, major or later, the
// project's, with its v8 provider and a test of place.
func (c *coverageState) vitestDependency(major int) error {
	const pinned = "5.0.2"
	if n, _ := strconv.Atoi(strings.Split(pinned, ".")[0]); n < major {
		return fmt.Errorf("the pinned Vitest %s is older than %d", pinned, major)
	}
	return c.vitestProject(".", true)
}

// vitestThroughAnotherPackage makes Vitest of the pinned version, major
// or later, the project's through another package (slice-17): its
// package.json, its test script script, names @vitest/coverage-v8 and a
// package named toolchain, which nothing installs, and never vitest, and
// the pinned tools, Vitest and c8 among them, are installed.
func (c *coverageState) vitestThroughAnotherPackage(major int, script string) error {
	const pinned = "5.0.2"
	if n, _ := strconv.Atoi(strings.Split(pinned, ".")[0]); n < major {
		return fmt.Errorf("the pinned Vitest %s is older than %d", pinned, major)
	}
	manifest, err := vitestThroughManifest(script)
	if err != nil {
		return err
	}
	if err := c.writeIn(".", map[string]string{
		"package.json": manifest,
		"src/board.test.ts": "import { expect, test } from \"vitest\";\nimport { place } from \"./board\";\n\n" +
			"test(\"place\", () => {\n  expect(place(5)).toBe(3);\n});\n",
	}); err != nil {
		return err
	}
	return c.declare(".", Coverage{Language: "typescript"})
}

// placeholder writes an installed package name of version at dir's
// node_modules, its executable a file that cannot run: what itos-cc finds
// installed, where no pinned tool set has it.
func (c *coverageState) placeholder(dir, name, version string) error {
	return c.writeIn(path.Join(dir, "node_modules"), map[string]string{
		name + "/package.json":  fmt.Sprintf(`{"name": %q, "version": %q}`, name, version) + "\n",
		".bin/" + name:          "placeholder: no " + name + " runs here\n",
		".bin/" + name + ".cmd": "placeholder: no " + name + " runs here\n",
	})
}

// jestDependency makes Jest the project's, installed as a placeholder.
func (c *coverageState) jestDependency() error {
	if err := c.packageJSON(".", nil, map[string]string{"jest": "30.0.0"}); err != nil {
		return err
	}
	return c.placeholder(".", "jest", "30.0.0")
}

// neitherVitestNorJest gives the project a test script and no test runner,
// and declares the pinned tools, c8 among them.
func (c *coverageState) neitherVitestNorJest() error {
	if err := c.packageJSON(".", map[string]string{"test": "node -e \"\""}, nil); err != nil {
		return err
	}
	data, err := c.w.readIn(c.w.dir, "package.json")
	if err != nil {
		return err
	}
	if strings.Contains(string(data), "vitest") || strings.Contains(string(data), "jest") {
		return fmt.Errorf("package.json names vitest or jest: %s", data)
	}
	return c.declare(".", Coverage{Language: "typescript"})
}

// pytest declares python3's pytest and coverage, and where pytest is not
// importable, writes the project a pytest.py that cannot be imported.
func (c *coverageState) pytest(which string) error {
	if which == "not importable" {
		if err := c.w.write("pytest.py", "raise ImportError(\"pytest is not importable in this project\")\n"); err != nil {
			return err
		}
	}
	return c.declare(".", Coverage{Language: "python"})
}

// kover adds the Kover plugin to the Gradle build, or checks it is not
// there.
func (c *coverageState) kover(which string) error {
	marker := "build.gradle.kts"
	if !c.w.d.Exists(c.abs(marker)) {
		marker = "build.gradle"
	}
	data, err := c.w.readIn(c.w.dir, marker)
	if err != nil {
		return err
	}
	mentions := strings.Contains(string(data), "kover")
	switch {
	case which == "does not mention" && mentions:
		return fmt.Errorf("%s mentions kover", marker)
	case which == "does not mention":
		return nil
	}
	return c.w.write(marker, strings.Replace(string(data), "\n}", "\n    id(\"org.jetbrains.kotlinx.kover\") version \"0.9.1\"\n}", 1))
}

// jacocoPlugin declares jacoco-maven-plugin in the project's pom.xml.
func (c *coverageState) jacocoPlugin() error {
	return c.w.write("pom.xml", strings.Replace(kotlinBuild("pom.xml"), "</project>",
		"  <build>\n    <plugins>\n      <plugin>\n        <groupId>org.jacoco</groupId>\n"+
			"        <artifactId>jacoco-maven-plugin</artifactId>\n        <version>0.8.13</version>\n"+
			"      </plugin>\n    </plugins>\n  </build>\n</project>", 1))
}

// packageManagerWorkspace writes a workspace whose root package.json
// declares manager, and in packages/app a project with a coverage script.
// Its runs find no manager, so its script never runs.
func (c *coverageState) packageManagerWorkspace(manager string) error {
	name, _, _ := strings.Cut(manager, "@")
	root, err := json.Marshal(map[string]any{"name": "workspace", "private": true, "packageManager": manager, "workspaces": []string{"packages/*"}})
	if err != nil {
		return err
	}
	c.project, c.w.without = "packages/app", []string{name}
	if err := c.writeIn(".", map[string]string{"package.json": string(root) + "\n", "packages/app/src/board.ts": tsSource}); err != nil {
		return err
	}
	c.roots[c.project] = "typescript"
	return c.packageJSON(c.project, map[string]string{"coverage": "node -e \"\""}, nil)
}

// toolsAtWorkspaceRoot writes a workspace whose root holds the pinned
// node_modules, and in packages/app a Vitest project without its own.
func (c *coverageState) toolsAtWorkspaceRoot() error {
	if err := c.w.write("package.json", `{"name": "workspace", "private": true, "workspaces": ["packages/*"]}`+"\n"); err != nil {
		return err
	}
	c.project = "packages/app"
	if err := c.vitestProject(c.project, false); err != nil {
		return err
	}
	if err := c.w.d.Coverage(c.w.dir, Coverage{Language: "typescript"}); err != nil {
		return err
	}
	if c.w.d.Exists(c.abs(c.project + "/node_modules")) {
		return errors.New("the project holds a node_modules of its own")
	}
	return nil
}

// nodeToolMissing writes a TypeScript project using runner, "" for no
// runner, without missing, the tool itos-cc names as not installed. A
// version installs runner as a placeholder, its own provider missing.
func (c *coverageState) nodeToolMissing(runner, missing, version string) error {
	c.missing, c.roots["."] = missing, "typescript"
	if missing == "" {
		c.missing = runner
	}
	deps := map[string]string{}
	if runner != "" {
		deps[runner] = orElse(version, "5.0.2")
	}
	if err := c.packageJSON(".", nil, deps); err != nil {
		return err
	}
	if err := c.w.write("src/board.ts", tsSource); err != nil {
		return err
	}
	if version == "" {
		return nil
	}
	return c.placeholder(".", runner, version)
}

// orElse is s, or otherwise when s is "".
func orElse(s, otherwise string) string {
	if s == "" {
		return otherwise
	}
	return s
}

// pythonWithoutCoverage writes a Python project whose own .venv's python
// cannot run, so it imports nothing.
func (c *coverageState) pythonWithoutCoverage() error {
	c.missing, c.roots["."] = "coverage", "python"
	return c.writeIn(".", map[string]string{
		"pyproject.toml": pythonMarker("pyproject.toml"), "board.py": pySource, "test_board.py": pythonTest,
		".venv/bin/python": "placeholder: this python cannot run\n",
	})
}

// mavenWithoutJacoco writes a Kotlin module built by Maven whose pom.xml
// declares no JaCoCo; its runs find neither Gradle nor Maven.
func (c *coverageState) mavenWithoutJacoco() error {
	c.missing, c.roots["."], c.w.without = "jacoco-maven-plugin", "kotlin", []string{"gradle", "mvn"}
	return c.writeIn(".", map[string]string{"pom.xml": kotlinBuild("pom.xml"), "src/main/kotlin/board/Board.kt": ktSource})
}

// pythonVenv writes a Python project and declares its .venv's python.
func (c *coverageState) pythonVenv() error {
	if err := c.writeIn(".", map[string]string{"pyproject.toml": pythonMarker("pyproject.toml"), "board.py": pySource, "test_board.py": pythonTest}); err != nil {
		return err
	}
	return c.declare(".", Coverage{Language: "python", Venv: true})
}

// gradleWrapper writes a build root holding gradlew, a placeholder, and the
// Kotlin Gradle module app under it; its runs find neither Gradle nor
// Maven.
func (c *coverageState) gradleWrapper() error {
	c.project, c.w.without = "app", []string{"gradle", "mvn"}
	c.roots["app"] = "kotlin"
	return c.writeIn(".", map[string]string{
		"gradlew":                            "placeholder: no Gradle runs here\n",
		"app/build.gradle.kts":               kotlinBuild("build.gradle.kts"),
		"app/src/main/kotlin/board/Board.kt": ktSource,
	})
}

// goAndTypeScript writes a git repository holding the Go module api in
// api/ and a Vitest package in web/, and declares each one's tools.
func (c *coverageState) goAndTypeScript() error {
	if err := c.w.git("init"); err != nil {
		return err
	}
	if err := c.writeIn("api", map[string]string{"go.mod": "module example.com/api\n\ngo 1.22\n", "api.go": goSource("api"), "api_test.go": markerTest("api", 5)}); err != nil {
		return err
	}
	if err := c.declare("api", Coverage{Language: "go"}); err != nil {
		return err
	}
	return c.vitestProject("web", true)
}

// failingTests writes a Go module whose test runs Place, all but its if's
// arm, and then fails.
func (c *coverageState) failingTests() error {
	if err := c.writeIn(".", map[string]string{
		"go.mod":   "module example.com/m\n\ngo 1.22\n",
		"board.go": goSource("board"),
		"board_test.go": "package board\n\nimport \"testing\"\n\nfunc TestPlace(t *testing.T) {\n" +
			"\tif Place(0) != 2 {\n\t\tt.Fatal(\"Place\")\n\t}\n\tt.Fatal(\"this test fails after it ran Place\")\n}\n",
	}); err != nil {
		return err
	}
	return c.declare(".", Coverage{Language: "go"})
}

// earlierRun writes a Go project of two modules, each with reports from an
// earlier run that differ from what its tests measure, which write
// test-ran when they run: one/ holds coverage.out and cover.out, two/
// cover.out alone.
func (c *coverageState) earlierRun() error {
	for _, m := range []struct {
		dir     string
		reports map[string][]bool
		first   string
	}{
		{"one", map[string][]bool{"coverage.out": {true, false, false}, "cover.out": {true, true, true}}, "coverage.out"},
		{"two", map[string][]bool{"cover.out": {true, true, false}}, "cover.out"},
	} {
		files := map[string]string{
			"go.mod": "module example.com/" + m.dir + "\n\ngo 1.22\n", m.dir + ".go": goSource(m.dir), m.dir + "_test.go": markerTest(m.dir, 5),
		}
		for name, ran := range m.reports {
			files[name] = "mode: set\n" + goRecords("example.com/"+m.dir+"/"+m.dir+".go", goBlocks(ran...))
		}
		if err := c.writeIn(m.dir, files); err != nil {
			return err
		}
		c.want[covFunction{m.dir + "/" + m.dir + ".go", "Place"}], _ = share(goBlocks(m.reports[m.first]...), byUnit)
		c.markers = append(c.markers, m.dir+"/test-ran")
		if err := c.declare(m.dir, Coverage{Language: "go"}); err != nil {
			return err
		}
	}
	return nil
}
