package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The scenario of "Rule: Language parity with Go for strict coverage and
// whole-suite freshness" in features/mutate.feature that the
// uncovered-fails-closed slice holds. No TypeScript, Python or JVM tool is
// needed: the TypeScript project has no node_modules, so its coverage tool
// is missing by the plan's own detection, and the Python interpreter and
// the Gradle wrapper are stub executables built from Go, so each cause
// happens the same way on Linux, macOS and Windows. Every mutant's tests are
// "go version", which passes whatever the source says, so each mutant that
// runs survives. The steps drive the CLI and read its --json object and the
// snapshots under .metrics as raw JSON.

// stubExecutable builds a program that exits with code whatever its
// arguments, and returns its path.
func stubExecutable(t *testing.T, code string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/stub\n\ngo 1.22\n")
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\nimport \"os\"\n\nfunc main() { os.Exit("+code+") }\n")
	bin := filepath.Join(dir, "stub")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build stub: %v\n%s", err, out)
	}
	return bin
}

// placeStub copies the stub to path, where the coverage plan looks for its
// tool, and to path.exe on Windows, which is what runs there.
func placeStub(t *testing.T, stub, path string) {
	t.Helper()
	data, err := os.ReadFile(stub)
	if err != nil {
		t.Fatal(err)
	}
	targets := []string{path}
	if runtime.GOOS == "windows" {
		targets = append(targets, path+".exe")
	}
	for _, target := range targets {
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// failClosedRun runs mutation run --json over source, with "go version" as
// every mutant's tests, and args.
func failClosedRun(t *testing.T, source string, args ...string) outcome {
	t.Helper()
	return cli(t, append([]string{"mutation", "run", "--json", "--no-annotate", "--workers", "1", "--test-command", "go version"},
		append(args, source)...)...)
}

// @ID-MUT-204
func TestStrictRunsFailClosedWhenAnotherLanguageMeasuredNothing(t *testing.T) {
	t.Parallel()
	for _, example := range []struct {
		language, failure string
		files             map[string]string
		source            string
		stub              func(t *testing.T, dir string) // places the tool, if any
		rule              string
		exit              int
	}{
		{
			language: "typescript", failure: "tool is missing",
			files: map[string]string{
				"app/package.json": `{"name": "app"}` + "\n",
				"app/src/calc.ts":  "export function above(a: number, b: number): boolean {\n  return a > b;\n}\n",
			},
			source: "app/src/calc.ts",
			rule:   "coverage.tool-missing", exit: 3,
		},
		{
			language: "python", failure: "command exits 0 but writes no report",
			files: map[string]string{
				"app/pyproject.toml": "[project]\nname = \"app\"\n",
				"app/src/calc.py":    "def above(a, b):\n    return a > b\n",
			},
			source: "app/src/calc.py",
			stub: func(t *testing.T, dir string) {
				placeStub(t, stubExecutable(t, "0"), filepath.Join(dir, "app", ".venv", "bin", "python"))
			},
			rule: "coverage.measured-nothing", exit: 1,
		},
		{
			language: "kotlin", failure: "command fails",
			files: map[string]string{
				"app/settings.gradle.kts":     "rootProject.name = \"app\"\n",
				"app/build.gradle.kts":        "plugins { kotlin(\"jvm\") version \"2.0.0\" }\n",
				"app/src/main/kotlin/Calc.kt": "fun above(a: Int, b: Int): Boolean {\n    return a > b\n}\n",
			},
			source: "app/src/main/kotlin/Calc.kt",
			stub: func(t *testing.T, dir string) {
				placeStub(t, stubExecutable(t, "1"), filepath.Join(dir, "app", "gradlew"))
			},
			rule: "coverage.measured-nothing", exit: 1,
		},
	} {
		t.Run(example.language, func(t *testing.T) {
			// Given a <language> project whose coverage <failure>
			dir := moduleRepo(t, example.files)
			if example.stub != nil {
				example.stub(t, dir)
			}
			source := filepath.FromSlash(example.source)

			// When I run "itos-cc mutation run --fail-uncovered --json" for
			// one of its files
			strict := failClosedRun(t, source, "--fail-uncovered")

			// Then the run fails with "<rule>" naming the language and
			// directory, and the exit code is <exit>
			var named map[string]any
			for _, p := range strict.json(t).Problems {
				if p["rule"] == example.rule {
					named = p
					break
				}
			}
			if named == nil {
				t.Errorf("%s coverage that %s: no %s problem\n%s%s", example.language, example.failure, example.rule, strict.stdout, strict.stderr)
			} else {
				if named["language"] != example.language {
					t.Errorf("%s names language %v, want %s\n%s", example.rule, named["language"], example.language, strict.stdout)
				}
				if d, _ := named["dir"].(string); filepath.ToSlash(d) != "app" {
					t.Errorf("%s names dir %v, want app\n%s", example.rule, named["dir"], strict.stdout)
				}
			}
			if strict.code != example.exit {
				t.Errorf("%s coverage that %s: exit %d, want %d\n%s%s", example.language, example.failure, strict.code, example.exit, strict.stdout, strict.stderr)
			}

			// And no mutant of that file is judged or reported as covered
			for _, f := range strict.rawFiles(t) {
				for _, key := range []string{"ran", "killed", "survived", "uncovered"} {
					if n, _ := f[key].(float64); n != 0 {
						t.Errorf("the strict run reports %s %v for %v\n%s", key, f[key], f["file"], strict.stdout)
					}
				}
				mutants, _ := f["mutants"].([]any)
				for _, m := range mutants {
					if outcome, _ := m.(map[string]any)["outcome"].(string); outcome != "" {
						t.Errorf("the strict run judged a mutant %v\n%s", m, strict.stdout)
					}
				}
			}
			for _, p := range strict.json(t).Problems {
				if rule, _ := p["rule"].(string); strings.HasPrefix(rule, "mutation.") {
					t.Errorf("the strict run judged %s: %v\n%s", rule, p["message"], strict.stdout)
				}
			}
			if strings.Contains(strict.stderr, "running every mutant") {
				t.Errorf("the strict run fell back to running every mutant:\n%s", strict.stderr)
			}
			snapshot := filepath.Join(dir, ".metrics", "mutate", filepath.FromSlash(example.source)+".json")
			if data, err := os.ReadFile(snapshot); err == nil && strings.Contains(string(data), `"outcome"`) {
				t.Errorf("the strict run recorded an outcome for %s:\n%s", example.source, data)
			}

			// But without --fail-uncovered the run falls back to running
			// every mutant as before
			plain := failClosedRun(t, source)
			if want := "no coverage for " + source + "; running every mutant"; !strings.Contains(plain.stderr, want) {
				t.Errorf("without --fail-uncovered the run does not say %q:\n%s", want, plain.stderr)
			}
			f := plain.json(t).file(t, source)
			if f.Ran == 0 || f.Survived != f.Ran || f.Uncovered != 0 {
				t.Errorf("without --fail-uncovered: ran %d, survived %d, uncovered %d; want every mutant run, none uncovered\n%s%s",
					f.Ran, f.Survived, f.Uncovered, plain.stdout, plain.stderr)
			}
			if plain.code != 1 {
				t.Errorf("without --fail-uncovered: exit %d, want 1 for the survivors\n%s%s", plain.code, plain.stdout, plain.stderr)
			}
		})
	}
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

	mixed := cli(t, "mutation", "run", "--json", "--no-annotate", "--workers", "1", "--fail-uncovered", goSource, ts)
	if p := mixed.json(t).problem("coverage.tool-missing"); p == nil || p["language"] != "typescript" || mixed.code != 3 {
		t.Errorf("mixed strict run: exit %d, want 3 with TypeScript's coverage.tool-missing\n%s%s", mixed.code, mixed.stdout, mixed.stderr)
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
	siteFreeRun := cli(t, "mutation", "run", "--json", "--no-annotate", "--workers", "1", "--fail-uncovered", goSource, siteFree)
	missing := siteFreeRun.json(t).problem("mutation.coverage-missing")
	if siteFreeRun.code != 1 || siteFreeRun.json(t).problem("coverage.tool-missing") != nil ||
		missing == nil || filepath.ToSlash(fmt.Sprint(missing["file"])) != "app/src/label.ts" {
		t.Errorf("a site-free TypeScript file with no tool: exit %d, want 1 with its evidence missing and no tool failure\n%s%s", siteFreeRun.code, siteFreeRun.stdout, siteFreeRun.stderr)
	}

	// Strict TypeScript coverage has its own line evidence now
	// (strict-lines-typescript), so a raw report is refused as Go's and
	// Python's are, rather than falling back to running every mutant.
	raw := failClosedRun(t, ts, "--fail-uncovered", "--coverage-report", "absent.info")
	if p := raw.json(t).problem("flags.conflict"); raw.code != 2 || p == nil || p["flag"] != "--coverage-report" {
		t.Errorf("--coverage-report under --fail-uncovered for TypeScript: exit %d, want 2 with flags.conflict naming it\n%s%s", raw.code, raw.stdout, raw.stderr)
	}
}
