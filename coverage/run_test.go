package coverage

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// vitestProject is a package that depends on Vitest, with version installed.
func vitestProject(t *testing.T, version string) (dir, source string) {
	t.Helper()
	dir = t.TempDir()
	files := map[string]string{
		"package.json":                     `{"devDependencies": {"vitest": "*"}}`,
		"node_modules/vitest/package.json": `{"version": "` + version + `"}`,
		"src/board.ts":                     "export const f = () => 1;\n",
	}
	for name, text := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir, filepath.Join(dir, "src", "board.ts")
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

func TestCurrentVitestRunsWithItsOwnProvider(t *testing.T) {
	dir, src := vitestProject(t, "5.0.2")
	plans := Plans([]string{src}, filepath.Join(dir, ".metrics"))
	if len(plans) != 1 || plans[0].Unsupported != "" {
		t.Fatalf("plans %+v, want one supported plan", plans)
	}
	got := plans[0].Commands
	if len(got) != 2 || strings.Join(got[0], " ") != "npm install --no-save @vitest/coverage-v8@5.0.2" ||
		got[1][1] != "vitest" {
		t.Errorf("commands %q, want the provider installed, then vitest", got)
	}
}
