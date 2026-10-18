package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// A guard beside "Rule: Language parity with Go for strict coverage and
// whole-suite freshness" in features/mutate.feature, whose ID-MUT-204 holds
// the uncovered-fails-closed slice's old test whole (T-101). No TypeScript, Python or JVM tool is
// needed: the TypeScript project has no node_modules, so its coverage tool
// is missing by the plan's own detection, and the Python interpreter and
// the Gradle wrapper are stub executables built from Go, so each cause
// happens the same way on Linux, macOS and Windows. Every mutant's tests are
// "go version", which passes whatever the source says, so each mutant that
// runs survives. The steps drive the CLI and read its --json object and the
// snapshots under .metrics as raw JSON.

// failClosedRun runs mutation run --json over source, with "go version" as
// every mutant's tests, and args.
func failClosedRun(t *testing.T, source string, args ...string) outcome {
	t.Helper()
	return itosCc(t, append([]string{"mutation", "run", "--json", "--no-annotate", "--workers", "1", "--test-command", "go version"},
		append(args, source)...)...)
}

// In a mixed strict run, another language that measured nothing stops the
// whole run before any mutant runs, as Go's missing coverage stops it, and
// a file of that language with no mutation site needs no coverage for its
// mutants, though its functions still need their line evidence. Raw-report
// flags are refused for it, as for Go.
func TestStrictMixedRunStopsWhenAnotherLanguageMeasuredNothing(t *testing.T) {
	t.Parallel()
	dir := moduleRepo(t, map[string]string{
		"gomod/go.mod":        "module example.com/mixed\n\ngo 1.22\n",
		"gomod/entry.go":      "package mixed\n\nfunc Above(a, b int) bool { return a > b }\n",
		"gomod/entry_test.go": "package mixed\n\nimport \"testing\"\n\nfunc TestAbove(t *testing.T) { if !Above(2, 1) || Above(1, 1) { t.Fatal(\"Above\") } }\n",
		"app/package.json":    `{"name": "app"}` + "\n",
		"app/src/calc.ts":     "export function above(a: number, b: number): boolean {\n  return a > b;\n}\n",
		"app/src/label.ts":    "export function label(name: string): string {\n  return name;\n}\n",
	})
	goSource, ts, siteFree := filepath.FromSlash("gomod/entry.go"), filepath.FromSlash("app/src/calc.ts"), filepath.FromSlash("app/src/label.ts")

	mixed := itosCc(t, "mutation", "run", "--json", "--no-annotate", "--workers", "1", "--fail-uncovered", goSource, ts)
	if p := mixed.json(t).problem("coverage.tool-missing"); p == nil || p["language"] != "typescript" || mixed.code != 82 {
		t.Errorf("mixed strict run: exit %d, want 82 with TypeScript's coverage.tool-missing\n%s%s", mixed.code, mixed.stdout, mixed.stderr)
	}
	if files := mixed.rawFiles(t); len(files) != 0 {
		t.Errorf("mixed strict run judged files %v, want the run stopped before any mutant", files)
	}
	for _, name := range []string{goSource, ts} {
		if _, err := os.Stat(filepath.Join(dir, ".metrics", "mutate", name+".json")); err == nil {
			t.Errorf("mixed strict run wrote %s's snapshot", name)
		}
	}

	// A site-free TypeScript file needs no coverage for its mutants, but its
	// function needs TypeScript line evidence (strict-lines-typescript),
	// which no tool measures here: it is missing, never a tool failure.
	siteFreeRun := itosCc(t, "mutation", "run", "--json", "--no-annotate", "--workers", "1", "--fail-uncovered", goSource, siteFree)
	missing := siteFreeRun.json(t).problem("mutation.coverage-missing")
	if siteFreeRun.code != 1 || siteFreeRun.json(t).problem("coverage.tool-missing") != nil ||
		missing == nil || filepath.ToSlash(fmt.Sprint(missing["file"])) != "app/src/label.ts" {
		t.Errorf("a site-free TypeScript file with no tool: exit %d, want 1 with its evidence missing and no tool failure\n%s%s", siteFreeRun.code, siteFreeRun.stdout, siteFreeRun.stderr)
	}

	// Strict TypeScript coverage has its own line evidence now
	// (strict-lines-typescript), so a raw report is refused as Go's and
	// Python's are, rather than falling back to running every mutant.
	raw := failClosedRun(t, ts, "--fail-uncovered", "--coverage-report", "absent.info")
	if p := raw.json(t).problem("flags.conflict"); raw.code != 80 || p == nil || p["flag"] != "--coverage-report" {
		t.Errorf("--coverage-report under --fail-uncovered for TypeScript: exit %d, want 80 with flags.conflict naming it\n%s%s", raw.code, raw.stdout, raw.stderr)
	}
}
