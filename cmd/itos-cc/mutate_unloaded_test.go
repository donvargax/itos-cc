package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The scenario of "Rule: Coverage decides which mutants run" about a file no
// test loads, mutated alone, in features/mutate.feature. It runs a real
// Vitest, the one the viewer installs under viewer/node_modules, and skips
// where that is not installed: CI runs no npm install.

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

// @ID-MUT-105
func TestAFileNoTestLoadsMutatedAloneIsEntirelyUncovered(t *testing.T) {
	t.Parallel()
	vitestRepo(t)
	sites := scanned(t, unusedSource)
	if len(sites) != 3 {
		t.Fatalf("sites of %s: %+v, want 3", unusedSource, sites)
	}

	o := mutateCovered(t, "--json", unusedSource)
	f := o.json(t).file(t, unusedSource)
	if f.Uncovered != len(sites) || f.Ran != 0 {
		t.Errorf("%s: %+v, want its %d mutants uncovered and none run\nstderr:\n%s", unusedSource, f, len(sites), o.stderr)
	}
	if strings.Contains(o.stderr, "no coverage") {
		t.Errorf("stderr says \"no coverage\":\n%s", o.stderr)
	}

	o = mutateCovered(t, "--fail-uncovered", unusedSource)
	for _, s := range sites {
		if want := uncoveredLine(s.File, s.Line, s.Column, s.Original, s.Replacement, s.Function); !strings.Contains(o.stdout, want) {
			t.Errorf("stdout lacks %q", want)
		}
	}
	if t.Failed() {
		t.Logf("stdout:\n%s\nstderr:\n%s", o.stdout, o.stderr)
	}
	if o.code != 1 {
		t.Errorf("--fail-uncovered: exit %d, want 1 for the uncovered mutants", o.code)
	}
}
