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
	plans := Plans([]string{src}, filepath.Join(dir, ".metrics"), RelatedTests)
	if len(plans) != 1 {
		t.Fatalf("plans %+v, want one", plans)
	}
	return plans[0]
}

func TestVitestBeforeTheCurrentMajorIsNotRun(t *testing.T) {
	dir, src := vitestProject(t, "4.1.11")
	plans := Plans([]string{src}, filepath.Join(dir, ".metrics"), RelatedTests)
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

func TestGoMeasuresWithTheTestsThatLoadTheChosenPackagesOrTheirOwn(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":               "module example.com/m\n\ngo 1.22\n",
		"a/a.go":               "package a\n\nfunc A() int { return 1 }\n",
		"a/a_test.go":          "package a\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) { A() }\n",
		"b/b.go":               "package b\n\nimport \"example.com/m/a\"\n\nfunc B() int { return a.A() }\n",
		"b/b_test.go":          "package b\n\nimport \"testing\"\n\nfunc TestB(t *testing.T) { B() }\n",
		"e2e/e2e_test.go":      "package e2e\n\nimport (\n\t\"testing\"\n\n\t\"example.com/m/b\"\n)\n\nfunc TestFlow(t *testing.T) { b.B() }\n",
		"slow/slow.go":         "package slow\n",
		"slow/slow_test.go":    "package slow\n\nimport \"testing\"\n\nfunc TestSlow(t *testing.T) {}\n",
		"untested/untested.go": "package untested\n\nfunc U() {}\n",
	})
	sources := []string{filepath.Join(dir, "a", "a.go"), filepath.Join(dir, "untested", "untested.go")}
	profile := "-count=1 -covermode=set -coverprofile=" + filepath.Join("/out", "coverage.out")
	got := strings.Join(goPlan(dir, "/out", sources, false).Commands[0][2:], " ")
	want := profile + " -coverpkg=example.com/m/a,example.com/m/untested example.com/m/a example.com/m/b example.com/m/e2e example.com/m/untested"
	if got != want {
		t.Errorf("related tests: go test %s\nwant                  %s", got, want)
	}
	own := strings.Join(goPlan(dir, "/out", sources, true).Commands[0][2:], " ")
	if want := profile + " example.com/m/a example.com/m/untested"; own != want {
		t.Errorf("own tests: go test %s\nwant              %s", own, want)
	}
	whole := strings.Join(goPlan(dir, "/out", nil, false).Commands[0], " ")
	if !strings.HasSuffix(whole, " -coverpkg=./... ./...") {
		t.Errorf("with no sources: %s, want the whole module", whole)
	}
}

func TestCoverageThatCouldNotBeMeasuredIsMissing(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.go")
	failing := Plan{Language: "go", Dir: dir, Sources: []string{src}, Commands: [][]string{{"go", "no-such-command"}},
		Reports: []string{filepath.Join(dir, "out", "coverage.out")}}
	unsupported := Plan{Language: "typescript", Dir: dir, Unsupported: "Vitest is not installed"}
	var log bytes.Buffer
	missing := Run([]Plan{failing, unsupported}, []string{src}, &log).Missing()
	if len(missing) != 2 || !strings.Contains(missing[0], "(go): its coverage run measured none of its files; go: exit status") ||
		!strings.Contains(missing[1], "Vitest is not installed") {
		t.Errorf("missing %q, want the failed run and the missing tool", missing)
	}
	if missing := Existing([]Plan{failing}, []string{src}, &log).Missing(); len(missing) != 1 {
		t.Errorf("existing: missing %q, want the plan with no report on disk", missing)
	}
	os.MkdirAll(filepath.Join(dir, "out"), 0o755)
	os.WriteFile(failing.Reports[0], []byte("mode: set\n"), 0o644)
	if missing := Existing([]Plan{failing}, []string{src}, &log).Missing(); len(missing) != 1 {
		t.Errorf("existing: missing %q, want the plan whose report measures none of its files", missing)
	}
	if missing := Files([]string{filepath.Join(dir, "nope.info")}, []string{src}, &log).Missing(); len(missing) != 1 {
		t.Errorf("files: missing %q, want the unreadable report", missing)
	}
}
