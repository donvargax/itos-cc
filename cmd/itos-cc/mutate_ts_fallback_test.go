package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// The TypeScript scenario of "Rule: A file's own tests are the tests that
// reach it, in every language" in features/mutate.feature, own-tests-ts-
// fallback's slice. Its project's test script runs Node's own test runner,
// node --test, over .ts files Node strips the types of, with neither Vitest
// nor Jest installed: nothing can narrow it to the tests that reach a file,
// so a file's own tests are the whole script. test/board.test.ts reaches
// src/board.ts, and test/other.test.ts reaches no module. It needs a real
// node and npm, and skips, naming the missing one, without them; CI runs it
// on ubuntu-latest (T-13).

// tsFallbackFiles is the project.
var tsFallbackFiles = map[string]string{
	"package.json": `{"name": "board", "private": true, "type": "module", "scripts": {"test": "node --test"}}` + "\n",
	"src/board.ts": "export function low(i: number): boolean {\n  return i === 3;\n}\n",
	"test/board.test.ts": "import { test } from \"node:test\";\nimport assert from \"node:assert/strict\";\n" +
		"import { low } from \"../src/board.ts\";\n\ntest(\"low\", () => {\n  assert.equal(low(3), true);\n  assert.equal(low(4), false);\n});\n",
	"test/other.test.ts": "import { test } from \"node:test\";\n\ntest(\"other\", () => {});\n",
}

// @ID-MUT-218
func TestATypeScriptTestScriptWithoutVitestOrJestRecordsWholeSuiteOutcomes(t *testing.T) {
	t.Parallel()
	languageTools(t, "node")
	// Given a TypeScript project whose test script runs Node's own test
	// runner, with neither Vitest nor Jest installed
	dir := moduleRepo(t, tsFallbackFiles)
	source, other := "src/board.ts", "test/other.test.ts"
	key := source + "#low"

	// And a mutant of one of its files has a result from "itos-cc mutation
	// run"
	first := suiteRun(t, filepath.FromSlash(source))
	defer func() {
		if t.Failed() {
			t.Logf("first run: exit %d\nstdout:\n%s\nstderr:\n%s", first.code, first.stdout, first.stderr)
		}
	}()
	f := first.json(t).file(t, filepath.FromSlash(source))
	if f.Baseline != "passed" || f.Ran == 0 || f.Killed != f.Ran {
		t.Fatalf("%s: %+v, want its baseline passed and every mutant run killed by the script", source, f)
	}
	if !strings.Contains(first.stderr, "run test") {
		t.Errorf("the run's commands are not the project's test script:\n%s", first.stderr)
	}
	// It is the whole suite's outcome, so it records the whole suite's
	// evidence, as a --test-command outcome does.
	for _, evidence := range recordedEvidence(t, dir, source) {
		tests, _ := evidence["tests"].(map[string]any)
		if tests[other] == nil {
			t.Errorf("the outcome records evidence %v, want the whole suite's, %s included", evidence, other)
		}
	}
	if states, _, check := suiteCheck(t, source); states[key] != "fresh" {
		t.Errorf("before any change: %s is %q, want fresh\n%s", key, states[key], check.stdout)
	}

	// When a test that reaches no module of that file changes
	writeFile(t, filepath.Join(dir, filepath.FromSlash(other)), tsFallbackFiles[other]+"// changed\n")

	// Then "itos-cc mutation check" reports that result stale, as it reports
	// a --test-command outcome
	states, stale, check := suiteCheck(t, source)
	if states[key] != "stale" || !strings.Contains(stale[key], other) {
		t.Errorf("after %s changed: %s is %q with message %q, want stale naming it\n%s", other, key, states[key], stale[key], check.stdout)
	}
	requireNoCommand(t, check)
}
