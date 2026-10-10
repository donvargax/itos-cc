package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The scenario of "Rule: Language parity with Go for TypeScript coverage
// scope and counted runs" about coverage scope, in features/mutate.feature.
// It runs a real Vitest, the one languageTools finds, and skips where none
// is installed. src/a.ts has two functions with one mutation site each:
// src/a.test.ts imports it and checks kept, so kept's mutant runs and is
// killed; src/other.test.ts never imports it, as vitest related sees
// imports, but loads it at run time through a path it builds and checks
// far, so only the whole suite executes far's line, and only the whole
// suite kills its mutant.

const (
	tsScopeSource = "src/a.ts"
	// The project's coverage script, which runs the whole suite and writes
	// coverage/lcov.info.
	tsScopeScript = "vitest run --coverage.enabled --coverage.reporter=lcov"
)

// tsScopeRepo makes the project, with the node_modules languageTools
// finds, in a new directory and makes it the test's directory (useDir),
// which it returns.
func tsScopeRepo(t *testing.T, modules string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Symlink(modules, filepath.Join(dir, "node_modules")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	for name, text := range map[string]string{
		"package.json": `{"name": "scope", "private": true, "type": "module", ` +
			`"scripts": {"coverage": "` + tsScopeScript + `"}, ` +
			`"devDependencies": {"vitest": "5.0.2", "@vitest/coverage-v8": "5.0.2"}}` + "\n",
		"src/a.ts": "export function kept(i: number): boolean {\n  return i === 3;\n}\n\n" +
			"export function far(i: number): boolean {\n  return i === 9;\n}\n",
		"src/a.test.ts": "import { expect, test } from \"vitest\";\nimport { kept } from \"./a\";\n\n" +
			"test(\"kept\", () => {\n  expect(kept(3)).toBe(true);\n  expect(kept(4)).toBe(false);\n});\n",
		"src/other.test.ts": "import { expect, test } from \"vitest\";\n\n" +
			"test(\"far\", async () => {\n" +
			"  const name = [\".\", \"a\"].join(\"/\");\n" +
			"  const { far } = await import(/* @vite-ignore */ name);\n" +
			"  expect(far(9)).toBe(true);\n  expect(far(8)).toBe(false);\n});\n",
	} {
		writeFile(t, filepath.Join(dir, name), text)
	}
	useDir(t, dir)
	return dir
}

// coverageLines are the coverage commands stderr says ran.
func coverageLines(stderr string) []string {
	var out []string
	for _, line := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(line, "itos-cc: coverage ") && strings.Contains(line, "$ ") {
			out = append(out, line)
		}
	}
	return out
}

// tsMutantOf is the mutant of function in f.
func tsMutantOf(t *testing.T, f fileJSON, function string) mutantJSON {
	t.Helper()
	for _, m := range f.mutants(t) {
		if strings.HasSuffix(m.Function, "#"+function) {
			return m
		}
	}
	t.Fatalf("no mutant of %s in %+v", function, f.mutants(t))
	return mutantJSON{}
}

// @ID-MUT-208
func TestTypeScriptCoverageComesFromTheRelatedTestsThatJudgeTheMutants(t *testing.T) {
	t.Parallel()
	modules := languageTools(t, "typescript")
	source := filepath.FromSlash(tsScopeSource)

	// Given a Vitest project with a coverage script that runs the whole
	// suite
	// And src/a.ts has a line that only an unrelated test, which does not
	// import it, executes
	dir := tsScopeRepo(t, modules)
	if sites := scanned(t, source); len(sites) != 2 {
		t.Fatalf("sites of %s: %+v, want kept's and far's", source, sites)
	}

	// When I run "itos-cc mutation run --json src/a.ts"
	o := mutateCovered(t, "--json", source)

	// Then coverage runs vitest related for src/a.ts into the run's own
	// directory, not the coverage script
	ran := coverageLines(o.stderr)
	runs := filepath.Join(dir, ".metrics", "coverage", "run-")
	if len(ran) != 1 || !strings.Contains(ran[0], "vitest related --run "+tsScopeSource+" ") ||
		!strings.Contains(ran[0], "--coverage.reportsDirectory="+runs) {
		t.Errorf("coverage commands %q, want the one vitest related --run %s into %s…", ran, tsScopeSource, runs)
	}
	if strings.Contains(o.stderr, "run coverage") {
		t.Errorf("the coverage script ran:\n%s", o.stderr)
	}

	// And the mutant on that line is uncovered and none of its trials runs
	f := o.json(t).file(t, source)
	if m := tsMutantOf(t, f, "far"); m.Outcome != "uncovered" {
		t.Errorf("far's mutant: %+v, want uncovered", m)
	}
	if m := tsMutantOf(t, f, "kept"); m.Outcome != "killed" {
		t.Errorf("kept's mutant: %+v, want killed", m)
	}
	if f.Ran != 1 || f.Uncovered != 1 {
		t.Errorf("%s: %+v, want kept's mutant the only one run and far's uncovered", source, f)
	}
	if t.Failed() {
		t.Logf("exit %d\nstdout:\n%s\nstderr:\n%s", o.code, o.stdout, o.stderr)
	}

	// But with --all-tests the coverage script measures the whole suite as
	// before
	tsScopeRepo(t, modules)
	o = mutateCovered(t, "--json", "--all-tests", source)
	failed := t.Failed()
	if ran := coverageLines(o.stderr); len(ran) != 1 || !strings.HasSuffix(ran[0], "$ npm run coverage") {
		t.Errorf("--all-tests: coverage commands %q, want the coverage script, npm run coverage", ran)
	}
	f = o.json(t).file(t, source)
	if m := tsMutantOf(t, f, "far"); m.Outcome != "killed" {
		t.Errorf("--all-tests: far's mutant: %+v, want covered by the whole suite and killed by it", m)
	}
	if !failed && t.Failed() {
		t.Logf("--all-tests: exit %d\nstdout:\n%s\nstderr:\n%s", o.code, o.stdout, o.stderr)
	}

	// And crap still measures with the coverage script
	tsScopeRepo(t, modules)
	o = cli(t, "crap", "--json", source)
	if ran := coverageLines(o.stderr); len(ran) != 1 || !strings.HasSuffix(ran[0], "$ npm run coverage") {
		t.Errorf("crap: coverage commands %q, want the coverage script, npm run coverage\nstderr:\n%s", ran, o.stderr)
	}
}
