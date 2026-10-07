package coverage

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeFiles writes text files under dir.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, text := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// bin is an installed executable's path under node_modules/.bin.
func bin(name string) string {
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	return "node_modules/.bin/" + name
}

// vitestProject is a package that depends on Vitest, with version and its
// coverage provider installed.
func vitestProject(t *testing.T, version string) (dir, source string) {
	t.Helper()
	dir = t.TempDir()
	writeFiles(t, dir, map[string]string{
		"package.json":                                  `{"devDependencies": {"vitest": "*"}}`,
		"node_modules/vitest/package.json":              `{"version": "` + version + `"}`,
		"node_modules/@vitest/coverage-v8/package.json": `{"version": "` + version + `"}`,
		bin("vitest"):                                   "",
		"src/board.ts":                                  "export const f = () => 1;\n",
	})
	return dir, filepath.Join(dir, "src", "board.ts")
}

func onePlan(t *testing.T, dir, src string) Plan {
	t.Helper()
	plans := Plans([]string{src}, filepath.Join(dir, ".metrics"))
	if len(plans) != 1 {
		t.Fatalf("plans %+v, want one", plans)
	}
	return plans[0]
}

func TestVitestBeforeTheCurrentMajorIsNotRun(t *testing.T) {
	dir, src := vitestProject(t, "4.1.11")
	plans := Plans([]string{src}, filepath.Join(dir, ".metrics"))
	if len(plans) != 1 || len(plans[0].Commands) != 0 || plans[0].Unsupported == "" {
		t.Fatalf("plans %+v, want one unsupported plan with no commands", plans)
	}
	var log bytes.Buffer
	Run(plans, []string{src}, &log)
	if !strings.Contains(log.String(), "Vitest 4.1.11 is not supported; upgrade to Vitest 5 or later") {
		t.Errorf("log %q does not say why", log.String())
	}
}

func TestCurrentVitestRunsFromNodeModules(t *testing.T) {
	dir, src := vitestProject(t, "5.0.2")
	plan := onePlan(t, dir, src)
	if plan.Unsupported != "" || len(plan.Commands) != 1 ||
		plan.Commands[0][0] != filepath.Join(dir, filepath.FromSlash(bin("vitest"))) {
		t.Errorf("plan %+v, want the installed vitest and nothing else", plan)
	}
}

func TestMissingNodeToolsAreNeverFetched(t *testing.T) {
	for name, remove := range map[string]string{
		"vitest":   bin("vitest"),
		"provider": "node_modules/@vitest/coverage-v8",
	} {
		t.Run(name, func(t *testing.T) {
			dir, src := vitestProject(t, "5.0.2")
			os.RemoveAll(filepath.Join(dir, filepath.FromSlash(remove)))
			plan := onePlan(t, dir, src)
			if len(plan.Commands) != 0 || !strings.Contains(plan.Unsupported, "not installed") {
				t.Errorf("plan %+v, want no commands and why", plan)
			}
		})
	}
	t.Run("c8", func(t *testing.T) {
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"package.json": `{}`, "src/a.ts": ""})
		plan := onePlan(t, dir, filepath.Join(dir, "src", "a.ts"))
		if len(plan.Commands) != 0 || !strings.Contains(plan.Unsupported, "c8 is not installed") {
			t.Errorf("plan %+v, want no commands and why", plan)
		}
	})
}

func TestAWorkspaceMemberUsesItsRootsPackageManagerAndTools(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"package.json":              `{"packageManager": "pnpm@9.12.0"}`,
		"pnpm-lock.yaml":            "",
		bin("c8"):                   "",
		"packages/web/package.json": `{"scripts": {"test": "node --test"}}`,
		"packages/web/src/a.ts":     "",
	})
	dir := filepath.Join(root, "packages", "web")
	plan := onePlan(t, dir, filepath.Join(dir, "src", "a.ts"))
	if len(plan.Commands) != 1 {
		t.Fatalf("plan %+v, want one command", plan)
	}
	got := plan.Commands[0]
	if got[0] != filepath.Join(root, filepath.FromSlash(bin("c8"))) || strings.Join(got[len(got)-3:], " ") != "pnpm run test" {
		t.Errorf("command %q, want the root's c8 running pnpm run test", got)
	}
}

func TestMavenRunsTheJaCoCoTheProjectDeclares(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"pom.xml":                  "<project><artifactId>parent</artifactId></project>",
		"app/pom.xml":              "<project><artifactId>app</artifactId></project>",
		"app/src/main/kotlin/A.kt": "",
	})
	dir := filepath.Join(root, "app")
	src := filepath.Join(dir, "src", "main", "kotlin", "A.kt")
	if plan := onePlan(t, dir, src); len(plan.Commands) != 0 || !strings.Contains(plan.Unsupported, "jacoco-maven-plugin") {
		t.Errorf("plan %+v, want no commands and why", plan)
	}
	writeFiles(t, root, map[string]string{
		"pom.xml": "<project><build><plugins><plugin><artifactId>jacoco-maven-plugin</artifactId></plugin></plugins></build></project>",
	})
	plan := onePlan(t, dir, src)
	if len(plan.Commands) != 1 || strings.Join(plan.Commands[0], " ") != "mvn -q jacoco:prepare-agent test jacoco:report" {
		t.Errorf("plan %+v, want the parent's JaCoCo by its prefix", plan)
	}
}

func TestPythonWithoutCoveragePyIsNotRun(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"pyproject.toml":   "",
		".venv/bin/python": "#!/bin/sh\nexit 1\n",
		"src/a.py":         "",
	})
	plan := onePlan(t, dir, filepath.Join(dir, "src", "a.py"))
	if len(plan.Commands) != 0 || !strings.Contains(plan.Unsupported, "coverage.py is not installed") {
		t.Errorf("plan %+v, want no commands and why", plan)
	}
}
