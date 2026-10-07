package mutate

import (
	"os"
	"path/filepath"
	"runtime"
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
	if got := TestCommand(src, "").String(); got != "pnpm run test" {
		t.Errorf("without vitest installed: %q, want the project's own test script", got)
	}
	os.MkdirAll(filepath.Dir(vitest), 0o755)
	os.WriteFile(vitest, nil, 0o755)
	if got := TestCommand(src, ""); got.Args[0] != vitest || got.Args[1] != "related" {
		t.Errorf("with vitest installed: %q, want %s related", got.Args, vitest)
	}
}
