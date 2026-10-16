package mutate

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

// Python evidence rests on every Python file and configuration file of the
// build root, without itos-cc's own summary comment, and leaves out
// virtualenvs, hidden directories and nested build roots.
func TestPythonCoverageInputsFingerprintTheBuildRoot(t *testing.T) {
	dir := t.TempDir()
	for name, text := range map[string]string{
		"pyproject.toml":              "[project]\nname = \"m\"\n",
		"calc.py":                     "def a():\n    return 1\n",
		"tests/test_calc.py":          "def test_a():\n    pass\n",
		"tests/conftest.py":           "",
		".coveragerc":                 "[run]\n",
		"README.md":                   "readme\n",
		".venv/pyvenv.cfg":            "home = /usr\n",
		".venv/lib/site.py":           "",
		"env/pyvenv.cfg":              "home = /usr\n",
		"env/lib/site.py":             "",
		"nested/pyproject.toml":       "[project]\nname = \"n\"\n",
		"nested/other.py":             "",
		"__pycache__/calc.cpython.py": "",
	} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	inputs, err := PythonCoverageInputs(t.Context(), filepath.Join(dir, "calc.py"), dir, "producer", map[string]string{"features/a.feature": "h"})
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for key := range inputs {
		keys = append(keys, key)
	}
	want := []string{".coveragerc", "@producer", "@python-env", "calc.py", "itos-cc.yaml", "pyproject.toml", "support:features/a.feature", "tests/conftest.py", "tests/test_calc.py"}
	if !reflect.DeepEqual(sortedKeys(inputs), want) {
		t.Errorf("inputs %v, want %v", keys, want)
	}
	annotated := "def a():\n    return 1\n\n# itos-cc mutation: 0 killed, 0 survived, 0 uncovered\n# end itos-cc mutation\n"
	if err := os.WriteFile(filepath.Join(dir, "calc.py"), []byte(annotated), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := PythonCoverageInputs(t.Context(), filepath.Join(dir, "calc.py"), dir, "producer", map[string]string{"features/a.feature": "h"})
	if err != nil {
		t.Fatal(err)
	}
	if changed := GoCoverageChanges(inputs, again); len(changed) != 0 {
		t.Errorf("the summary comment changed inputs %v", changed)
	}
}

func sortedKeys(m map[string]string) []string {
	var out []string
	for key := range m {
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}

// TypeScript evidence rests on every TypeScript and JavaScript file and the
// configuration of the package root, and leaves out node_modules, hidden
// directories, nested packages and the root's build outputs.
func TestTypeScriptCoverageInputsFingerprintThePackageRoot(t *testing.T) {
	dir := t.TempDir()
	for name, text := range map[string]string{
		"package.json":                "{}\n",
		"package-lock.json":           "{}\n",
		"tsconfig.build.json":         "{}\n",
		"vitest.config.ts":            "export default {};\n",
		"src/calc.ts":                 "export const a = 1;\n",
		"src/calc.test.ts":            "",
		"src/view.tsx":                "",
		"scripts/run.mjs":             "",
		"README.md":                   "",
		"node_modules/x/index.js":     "",
		".cache/y.js":                 "",
		"dist/calc.js":                "",
		"coverage/lcov-report/x.js":   "",
		"packages/inner/package.json": "{}\n",
		"packages/inner/z.ts":         "",
		"src/dist/kept.ts":            "",
	} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	inputs, err := TypeScriptCoverageInputs(t.Context(), filepath.Join(dir, "src", "calc.ts"), dir, "producer", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"@node-env", "@producer", "itos-cc.yaml", "package-lock.json", "package.json", "scripts/run.mjs",
		"src/calc.test.ts", "src/calc.ts", "src/dist/kept.ts", "src/view.tsx", "tsconfig.build.json", "vitest.config.ts"}
	if got := sortedKeys(inputs); !reflect.DeepEqual(got, want) {
		t.Errorf("inputs %v, want %v", got, want)
	}
}

// writeTree writes text files under dir.
func writeTree(t *testing.T, dir string, files map[string]string) {
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
}

// Kotlin evidence of a module of a multi-module Gradle build rests on the
// whole build: every module's Kotlin and Java sources and tests, Gradle
// scripts, properties, version catalog and wrapper, the included build and
// buildSrc, but no build output, hidden directory or nested build.
func TestKotlinCoverageInputsFingerprintTheWholeGradleBuild(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"settings.gradle.kts":                      "include(\"app\", \"lib\")\nincludeBuild(\"build-logic\")\n",
		"build.gradle.kts":                         "",
		"gradle.properties":                        "",
		"gradle/libs.versions.toml":                "",
		"gradle/wrapper/gradle-wrapper.properties": "",
		"gradle/jacoco.gradle":                     "",
		"app/build.gradle.kts":                     "",
		"app/src/main/kotlin/calc/Calc.kt":         "package calc\n",
		"app/src/test/kotlin/calc/CalcTest.kt":     "",
		"app/src/main/resources/x.txt":             "",
		"app/build/generated/G.kt":                 "",
		"app/src/main/kotlin/calc/build/Kept.kt":   "",
		"lib/build.gradle":                         "",
		"lib/src/main/java/lib/Lib.java":           "",
		"lib/target/T.kt":                          "",
		"buildSrc/settings.gradle.kts":             "",
		"buildSrc/src/main/kotlin/Conv.kt":         "",
		"build-logic/settings.gradle.kts":          "",
		"build-logic/src/main/kotlin/Plugin.kt":    "",
		"samples/demo/settings.gradle.kts":         "",
		"samples/demo/src/main/kotlin/Demo.kt":     "",
		".gradle/8.0/x.kt":                         "",
		".idea/y.kt":                               "",
		"README.md":                                "",
	})
	inputs, err := KotlinCoverageInputs(t.Context(), filepath.Join(dir, "app", "src", "main", "kotlin", "calc", "Calc.kt"), dir, "producer", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"@jvm-env", "@producer", "app/build.gradle.kts", "app/src/main/kotlin/calc/Calc.kt",
		"app/src/main/kotlin/calc/build/Kept.kt", "app/src/test/kotlin/calc/CalcTest.kt",
		"build-logic/settings.gradle.kts", "build-logic/src/main/kotlin/Plugin.kt", "build.gradle.kts",
		"buildSrc/settings.gradle.kts", "buildSrc/src/main/kotlin/Conv.kt", "gradle.properties",
		"gradle/jacoco.gradle", "gradle/libs.versions.toml", "gradle/wrapper/gradle-wrapper.properties",
		"itos-cc.yaml", "lib/build.gradle", "lib/src/main/java/lib/Lib.java", "settings.gradle.kts"}
	if got := sortedKeys(inputs); !reflect.DeepEqual(got, want) {
		t.Errorf("inputs %v, want %v", got, want)
	}
	// A Gradle wrapper, or JAVA_HOME, is another JVM environment.
	writeTree(t, dir, map[string]string{"gradlew": ""})
	wrapped, err := KotlinCoverageInputs(t.Context(), filepath.Join(dir, "app", "src", "main", "kotlin", "calc", "Calc.kt"), dir, "producer", nil)
	if err != nil {
		t.Fatal(err)
	}
	if wrapped["@jvm-env"] == inputs["@jvm-env"] {
		t.Error("running through gradlew leaves the JVM environment as it was")
	}
	t.Setenv("JAVA_HOME", filepath.Join(dir, "other-jdk"))
	other, err := KotlinCoverageInputs(t.Context(), filepath.Join(dir, "app", "src", "main", "kotlin", "calc", "Calc.kt"), dir, "producer", nil)
	if err != nil {
		t.Fatal(err)
	}
	if other["@jvm-env"] == wrapped["@jvm-env"] {
		t.Error("another JAVA_HOME leaves the JVM environment as it was")
	}
}

// Kotlin evidence of a Maven module rests on its whole reactor, from the
// top pom.xml, with each module's pom.xml and .mvn's configuration, but no
// target output.
func TestKotlinCoverageInputsFingerprintTheWholeMavenReactor(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"pom.xml":                        "",
		".mvn/maven.config":              "",
		"core/pom.xml":                   "",
		"core/src/main/kotlin/Core.kt":   "",
		"app/pom.xml":                    "",
		"app/src/main/kotlin/App.kt":     "",
		"app/src/test/kotlin/AppTest.kt": "",
		"app/target/classes/Gen.kt":      "",
	})
	if got := KotlinBuildRoot(t.Context(), filepath.Join(dir, "app", "src", "main", "kotlin", "App.kt")); got != dir {
		t.Errorf("build root %s, want the reactor's top %s", got, dir)
	}
	inputs, err := KotlinCoverageInputs(t.Context(), filepath.Join(dir, "app", "src", "main", "kotlin", "App.kt"), dir, "producer", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".mvn/maven.config", "@jvm-env", "@producer", "app/pom.xml", "app/src/main/kotlin/App.kt",
		"app/src/test/kotlin/AppTest.kt", "core/pom.xml", "core/src/main/kotlin/Core.kt", "itos-cc.yaml", "pom.xml"}
	if got := sortedKeys(inputs); !reflect.DeepEqual(got, want) {
		t.Errorf("inputs %v, want %v", got, want)
	}
}
