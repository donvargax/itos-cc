package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/metrics"
	"github.com/donvargax/itos-cc/project"
)

// The scenario of "Rule: Coverage from tests that start other processes, in
// every language" in features/mutate.feature that the
// integration-coverage-typescript slice holds. Each project's src/cli.ts is
// run by its Vitest tests only in a child Node process, node src/cli.ts N,
// which prints describe(N): no test imports it. The one test of the first
// project runs it with 1, so it takes the positive branch, lines 4 and 5,
// and never line 7; it notices `0` → `1` and `+` → `-`, and not `>` → `>=`.
// When CLI_RECORD names a file, cli.ts writes there the NODE_V8_COVERAGE it
// was started with. They need node 22.18 or later, which runs .ts files by
// stripping their types, with Vitest, its v8 provider and c8, those of
// testdata/tools/node, and skip, naming the missing one, without them; CI
// runs them on ubuntu-latest, where the language tools are installed.

const (
	tsCliIfLine       = 4
	tsCliPositiveLine = 5
	tsCliOtherLine    = 7
)

const tsCliSource = `import { appendFileSync } from "node:fs";

export function describe(n: number): number {
  if (n > 0) {
    return n + 100;
  }
  return n - 100;
}

if (import.meta.main) {
  const record = process.env.CLI_RECORD;
  if (record && process.env.NODE_V8_COVERAGE) {
    appendFileSync(record, process.env.NODE_V8_COVERAGE + "\n");
  }
  console.log(describe(Number(process.argv[2])));
}
`

// tsCliRunner is the head of a Vitest test file that runs src/cli.ts in a
// child Node process: run(test, arg) is what it prints. With a harness that
// splits coverage by test, the processes test starts write their V8
// coverage to NODE_V8_COVERAGE=<ITOS_CC_TEST_COVERDIR>/<test>.
func tsCliRunner(split bool) string {
	harness := "  const split = \"\";\n"
	if split {
		harness = "  const split = process.env.ITOS_CC_TEST_COVERDIR;\n"
	}
	return "import { execFileSync } from \"node:child_process\";\nimport { join } from \"node:path\";\n" +
		"import { fileURLToPath } from \"node:url\";\nimport { expect, test } from \"vitest\";\n\n" +
		"const cli = fileURLToPath(new URL(\"./cli.ts\", import.meta.url));\n\n" +
		"function run(name: string, arg: string): string {\n" +
		"  const env = { ...process.env };\n" + harness +
		"  if (split) {\n    env.NODE_V8_COVERAGE = join(split, name);\n  }\n" +
		"  return execFileSync(process.execPath, [cli, arg], { encoding: \"utf8\", env }).trim();\n}\n\n"
}

const tsPackageJSON = `{"name": "cli", "private": true, "type": "module", ` +
	`"devDependencies": {"vitest": "5.0.2", "@vitest/coverage-v8": "5.0.2", "c8": "12.0.0"}}` + "\n"

var typescriptIntegrationFiles = map[string]string{
	"package.json":    tsPackageJSON,
	"src/cli.ts":      tsCliSource,
	"src/cli.test.ts": tsCliRunner(false) + "test(\"positive\", () => {\n  expect(run(\"positive\", \"1\")).toBe(\"101\");\n});\n",
}

// typescriptIntegrationRepo makes a project of files in a new directory,
// with modules as its node_modules, makes it the test's directory
// (useDir), and returns the file cli.ts records its NODE_V8_COVERAGE in.
func typescriptIntegrationRepo(t *testing.T, modules string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Symlink(modules, filepath.Join(dir, "node_modules")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	for name, text := range files {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(name)), text)
	}
	useDir(t, dir)
	record := filepath.Join(t.TempDir(), "record")
	useEnv(t, "CLI_RECORD", record)
	return record
}

// withoutC8 is a node_modules holding every package of modules but c8,
// each a symlink to modules' own.
func withoutC8(t *testing.T, modules string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "node_modules")
	for _, sub := range []string{"", ".bin"} {
		entries, err := os.ReadDir(filepath.Join(modules, sub))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(out, sub), 0o755); err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			name := e.Name()
			if name == "c8" || name == "c8.cmd" || name == "c8.ps1" || sub == "" && name == ".bin" {
				continue
			}
			if err := os.Symlink(filepath.Join(modules, sub, name), filepath.Join(out, sub, name)); err != nil {
				t.Skip("symlinks unavailable:", err)
			}
		}
	}
	return out
}

// tsCliMutants is the mutants of src/cli.ts in o's --json output, as plain
// maps.
func tsCliMutants(t *testing.T, o outcome) []map[string]any {
	t.Helper()
	for _, f := range o.rawFiles(t) {
		if filepath.ToSlash(f["file"].(string)) != "src/cli.ts" {
			continue
		}
		var out []map[string]any
		list, _ := f["mutants"].([]any)
		for _, m := range list {
			out = append(out, m.(map[string]any))
		}
		if len(out) != 4 {
			t.Fatalf("%d mutants of src/cli.ts, want 4\n%s\nstderr:\n%s", len(out), o.stdout, o.stderr)
		}
		return out
	}
	t.Fatalf("no src/cli.ts in files:\n%s\nstderr:\n%s", o.stdout, o.stderr)
	return nil
}

// tsCliMutant describes a mutant of src/cli.ts.
func tsCliMutant(m map[string]any) string {
	return strings.Replace(describe(m), greetSource, "src/cli.ts", 1)
}

// @ID-MUT-229
func TestLinesATypeScriptTestReachesThroughAChildNodeProcessAreCovered(t *testing.T) {
	t.Parallel()
	modules := languageTools(t, "c8")
	languageTools(t, "node")
	cli := filepath.FromSlash("src/cli.ts")
	t.Run("whole suite", func(t *testing.T) {
		t.Parallel()
		// Given a TypeScript project with Vitest and c8 installed whose only
		// test runs its CLI in a child Node process and checks one branch of
		// its output
		record := typescriptIntegrationRepo(t, modules, typescriptIntegrationFiles)
		out := filepath.Join(metrics.DirOf(project.RootOf(wd(t))), "coverage")

		// When I run "itos-cc mutation run --all-tests --fail-uncovered --json"
		o := mutateCovered(t, "--all-tests", "--fail-uncovered", "--json", cli)
		logRun(t, &o)

		// Then the lines the child process ran are covered, with "coverage"
		// listing "integration", and their mutants run
		// And a mutant the test notices is killed, one it does not notice
		// survives, and only mutants on lines never run are uncovered
		want := map[string]string{">=": "survived", "1": "killed", "-": "killed", "+": "uncovered"}
		for _, m := range tsCliMutants(t, o) {
			replacement, _ := m["replacement"].(string)
			if m["outcome"] != want[replacement] {
				t.Errorf("%s, want %s", tsCliMutant(m), want[replacement])
			}
			got := cliCoverage(m)
			if mutantLine(m) == tsCliOtherLine {
				if got != nil {
					t.Errorf("%s has \"coverage\" %q, want none", tsCliMutant(m), got)
				}
				continue
			}
			if !slices.Equal(got, []string{"integration"}) {
				t.Errorf("%s: \"coverage\" %q, want [\"integration\"]: only the child process ran its line, not Vitest's own", tsCliMutant(m), got)
			}
		}
		// The strict line evidence counts the child's lines too: only the
		// line no process ran is an uncovered statement.
		if got, want := strictLineFindings(t, o), []string{"mutation.uncovered-statement src/cli.ts cli#describe:7"}; !slices.Equal(got, want) {
			t.Errorf("strict findings %q, want %q", got, want)
		}
		if o.code != 1 {
			t.Errorf("exit %d, want 1: the line never run holds an uncovered mutant", o.code)
		}

		// The child's coverage data stays with the run: under the run's own
		// run-* directory of .metrics/coverage/, and gone once it ends.
		data, err := os.ReadFile(record)
		if err != nil {
			t.Fatalf("the child process was started without NODE_V8_COVERAGE, and recorded none: %v", err)
		}
		dir, _, _ := strings.Cut(string(data), "\n")
		rel, err := filepath.Rel(out, dir)
		first, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
		if err != nil || !strings.HasPrefix(first, "run-") || first == rel {
			t.Fatalf("NODE_V8_COVERAGE %s, want a directory under a run-* directory of %s", dir, out)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("NODE_V8_COVERAGE %s is still there after the run (%v)", dir, err)
		}
		if _, err := os.Stat(filepath.Join(out, first)); !os.IsNotExist(err) {
			t.Errorf("the run's directory %s is still there after the run (%v)", first, err)
		}
		filepath.WalkDir(wd(t), func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() && d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			if err == nil && strings.HasPrefix(d.Name(), "coverage-") && strings.HasSuffix(d.Name(), ".json") {
				t.Errorf("%s is left in the project", path)
			}
			return nil
		})
	})
	// And with mutation.tests listing two tests that run different
	// branches, each line is covered by the IDs of the tests that reached it
	for _, split := range []bool{false, true} {
		name := "listed tests, each run alone"
		if split {
			name = "listed tests, split by the harness"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			files := map[string]string{
				"package.json": tsPackageJSON,
				"src/cli.ts":   tsCliSource,
				"src/listed.test.ts": tsCliRunner(split) +
					"test(\"positive\", () => {\n  expect(run(\"positive\", \"1\")).toBe(\"101\");\n});\n\n" +
					"test(\"other\", () => {\n  expect(run(\"other\", \"-1\")).toBe(\"-101\");\n});\n",
				"tests.txt": "positive\tsrc/listed.test.ts\nother\tsrc/listed.test.ts\n",
				"itos-cc.yaml": "mutation:\n  tests:\n" +
					"    list: node -e \"process.stdout.write(require('fs').readFileSync('tests.txt', 'utf8'))\"\n" +
					"    run: node_modules/.bin/vitest run src/listed.test.ts -t \"{pattern}\"\n" +
					"    ids_pattern: \"^({ids})$\"\n" +
					"    join:\n      each: \"{id}\"\n      sep: \"|\"\n",
			}
			typescriptIntegrationRepo(t, modules, files)

			o := mutateCovered(t, "--json", cli)
			logRun(t, &o)
			wantTests := map[int][]string{
				tsCliIfLine:       {"positive", "other"},
				tsCliPositiveLine: {"positive"},
				tsCliOtherLine:    {"other"},
			}
			for _, m := range tsCliMutants(t, o) {
				want := wantTests[mutantLine(m)]
				if got := listedTests(m); m["scope"] != "listed" || !slices.Equal(got, want) {
					t.Errorf("%s: scope %v, tests %q, want \"listed\" and %q, the tests that reach its line", tsCliMutant(m), m["scope"], got, want)
				}
			}
			var runs []string
			for _, l := range strings.Split(o.stderr, "\n") {
				if strings.HasPrefix(l, "itos-cc: coverage ") && strings.Contains(l, "src/listed.test.ts -t") {
					runs = append(runs, l)
				}
			}
			if wantRuns := map[bool]int{false: 3, true: 1}[split]; len(runs) != wantRuns {
				t.Errorf("coverage ran the listed tests as %q, want %d runs", runs, wantRuns)
			}
		})
	}
	// But without c8 the run logs that integration coverage needs it and
	// goes on, and a strict run fails with "coverage.tool-missing"
	t.Run("without c8", func(t *testing.T) {
		t.Parallel()
		typescriptIntegrationRepo(t, withoutC8(t, modules), typescriptIntegrationFiles)

		// The strict run comes first: a plain run's uncovered outcomes,
		// reused, would leave it no mutant to judge.
		strict := mutateCovered(t, "--all-tests", "--fail-uncovered", "--json", cli)
		logRun(t, &strict)
		var found bool
		for _, p := range strict.json(t).Problems {
			if p["rule"] == "coverage.tool-missing" && p["language"] == "typescript" {
				found = true
				if message, _ := p["message"].(string); !strings.Contains(message, "c8") {
					t.Errorf("coverage.tool-missing says %q, want it to name c8", message)
				}
			}
		}
		if !found {
			t.Errorf("a strict run's problems are %q, want coverage.tool-missing for typescript", problemRules(t, strict))
		}
		if strict.code == 0 {
			t.Errorf("a strict run exits 0, want it to fail")
		}

		plain := mutateCovered(t, "--all-tests", "--json", cli)
		logRun(t, &plain)
		if !strings.Contains(plain.stderr, "integration coverage") || !strings.Contains(plain.stderr, "c8") {
			t.Errorf("a run without --fail-uncovered logs nothing of integration coverage needing c8")
		}
		for _, p := range plain.json(t).Problems {
			if p["rule"] == "coverage.tool-missing" {
				t.Errorf("a run without --fail-uncovered reports %v", p)
			}
		}
		for _, m := range tsCliMutants(t, plain) {
			if m["outcome"] != "uncovered" {
				t.Errorf("%s, want uncovered, as before: no process the test started was measured", tsCliMutant(m))
			}
		}
	})
}
