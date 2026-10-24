package testtools_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/testtools"
)

// CoverageRunner names the runner a source's coverage goes through, as the
// coverage runner chooses it from the manifest of the package or module
// the source is in, without running it: for TypeScript the package's
// coverage script unless the tests that reach the source run alone through
// Vitest or Jest, for Kotlin Maven, Kover or JaCoCo by the build, and none
// for another language.
func TestCoverageRunnerNamesTheRunnerOfTheSourcesPackage(t *testing.T) {
	root := t.TempDir()
	for name, text := range map[string]string{
		"web/package.json":            `{"scripts": {"coverage": "vitest run --coverage"}, "devDependencies": {"vitest": "3.2.4"}}`,
		"web/src/a.ts":                "export const a = 1;\n",
		"plain/package.json":          `{"name": "plain"}`,
		"plain/src/b.ts":              "export const b = 1;\n",
		"kover/build.gradle.kts":      "plugins { id(\"org.jetbrains.kotlinx.kover\") }\n",
		"kover/src/main/kotlin/A.kt":  "fun a() = 1\n",
		"gradle/build.gradle.kts":     "plugins { kotlin(\"jvm\") }\n",
		"gradle/src/main/kotlin/B.kt": "fun b() = 1\n",
		"maven/pom.xml":               "<project/>\n",
		"maven/src/main/kotlin/C.kt":  "fun c() = 1\n",
		"python/pyproject.toml":       "[project]\nname = \"p\"\n",
		"python/src/mod.py":           "def one():\n    return 1\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		source  string
		related bool
		want    string
	}{
		{"web/src/a.ts", false, "script"},
		{"web/src/a.ts", true, "vitest"},
		{"plain/src/b.ts", false, "c8"},
		{"kover/src/main/kotlin/A.kt", false, "kover"},
		{"gradle/src/main/kotlin/B.kt", false, "jacoco"},
		{"maven/src/main/kotlin/C.kt", false, "maven"},
		{"python/src/mod.py", false, ""},
	} {
		source := filepath.Join(root, filepath.FromSlash(c.source))
		got, failure := testtools.Toolchain{Dir: root}.CoverageRunner(t.Context(), source, c.related)
		if failure != nil || got != c.want {
			t.Errorf("CoverageRunner(%s, related %t) = %q, %v; want %q", c.source, c.related, got, failure, c.want)
		}
	}
}
