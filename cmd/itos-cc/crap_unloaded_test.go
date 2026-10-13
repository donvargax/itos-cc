package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A file no test loads, scored alone without --threshold: beside
// crap-unloaded-measured's scenario in features/crap.feature (ID-CRAP-14),
// whose steps score it with one. It uses a TypeScript project whose one
// test imports src/board.ts and never src/unused.ts, as mutate.feature's
// ID-MUT-105 does, and a real Vitest, the one the viewer installs under
// viewer/node_modules, which finds no test related to src/unused.ts, exits
// 0 and writes an empty lcov.info. It skips where that is not installed.

var unusedSource = filepath.FromSlash("src/unused.ts")

// vitestRepo makes a TypeScript project whose one test imports
// src/board.ts and never src/unused.ts, with the node_modules languageTools
// finds, and makes it the test's directory (useDir).
func vitestRepo(t *testing.T) {
	t.Helper()
	modules := languageTools(t, "typescript")
	dir := t.TempDir()
	if err := os.Symlink(modules, filepath.Join(dir, "node_modules")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	for name, text := range map[string]string{
		"package.json": `{"name": "m", "type": "module", "devDependencies": {"vitest": "5.0.2", "@vitest/coverage-v8": "5.0.2"}}` + "\n",
		"src/board.ts": "export function place(i: number): boolean {\n  return i > 5;\n}\n",
		"src/unused.ts": "export function half(i: number): boolean {\n  return i > 2;\n}\n\n" +
			"export function zero(i: number): boolean {\n  return i === 0;\n}\n",
		"src/board.test.ts": "import { expect, test } from \"vitest\";\nimport { place } from \"./board\";\n\n" +
			"test(\"place\", () => {\n  expect(place(5)).toBe(false);\n  expect(place(6)).toBe(true);\n});\n",
	} {
		writeFile(t, filepath.Join(dir, name), text)
	}
	useDir(t, dir)
}

type crapJSON struct {
	Entries []struct {
		Name     string   `json:"name"`
		File     string   `json:"file"`
		Coverage *float64 `json:"coverage"`
		CRAP     *float64 `json:"crap"`
	} `json:"entries"`
	Problems []map[string]any `json:"problems"`
}

// crapOf is crap's --json output in o.
func crapOf(t *testing.T, o outcome) crapJSON {
	t.Helper()
	var out crapJSON
	if err := json.Unmarshal([]byte(o.stdout), &out); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s\nstderr:\n%s", err, o.stdout, o.stderr)
	}
	return out
}

// wantUntested fails t unless every function of src/unused.ts in c has 0%
// coverage.
func wantUntested(t *testing.T, c crapJSON, o outcome) {
	t.Helper()
	n := 0
	for _, e := range c.Entries {
		if filepath.Clean(e.File) != unusedSource {
			continue
		}
		n++
		if e.Coverage == nil || *e.Coverage != 0 {
			t.Errorf("%s: coverage %v, want 0%%\nstdout:\n%s\nstderr:\n%s", e.Name, e.Coverage, o.stdout, o.stderr)
		}
	}
	if n != 2 {
		t.Errorf("%d functions of %s scored, want 2\n%s", n, unusedSource, o.stdout)
	}
}

// Without --threshold, the file scores 0% as with one, and stderr warns of
// no missing coverage.
func TestWithoutAThresholdAFileNoTestLoadsIsUntestedWithNoWarning(t *testing.T) {
	t.Parallel()
	vitestRepo(t)

	o := itosCc(t, "crap", "--json", unusedSource)
	wantUntested(t, crapOf(t, o), o)
	if strings.Contains(o.stderr, "itos-cc: no coverage") {
		t.Errorf("stderr warns of missing coverage:\n%s", o.stderr)
	}
	if o.code != 0 {
		t.Errorf("exit %d, want 0", o.code)
	}
}
