package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/graph"
)

// The scenarios of "Rule: Language parity with Go for strict coverage and
// whole-suite freshness" in features/mutate.feature that the
// whole-suite-freshness-all slice holds: ADR-0016 for TypeScript, Python
// and Kotlin. No TypeScript, Python or JVM tool is needed. Each project
// has its build-root marker, and its test runner is a stub that always
// passes: node_modules/.bin/vitest, the Gradle wrapper, .venv/bin/python
// and run.sh, all one program built from Go that fails only on a mutant,
// with a .exe and a .cmd beside it for Windows. So every mutant is killed,
// and a fresh kill is reused. The steps drive the CLI, the graph's
// builder, and read the --json objects and the snapshots as raw JSON.

// wholeSuiteEvidence is the whole-suite freshness evidence a raw snapshot
// mutant records, under the language-neutral key or under Go's original
// one, nil for none.
func wholeSuiteEvidence(mutant map[string]any) map[string]any {
	for _, key := range []string{"suite_evidence", "go_evidence"} {
		if evidence, ok := mutant[key].(map[string]any); ok {
			return evidence
		}
	}
	return nil
}

// suiteRunner builds the stub test runner: it passes unless a file
// beneath src, where it runs, holds ">=", which is what each fixture's one
// mutant writes, so every mutant is killed and its outcome reusable while
// fresh.
func suiteRunner(t *testing.T) []byte {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/runner\n\ngo 1.22\n")
	writeFile(t, filepath.Join(dir, "main.go"), `package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	mutated := false
	filepath.WalkDir("src", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if data, err := os.ReadFile(path); err == nil && strings.Contains(string(data), ">=") {
				mutated = true
			}
		}
		return nil
	})
	if mutated {
		os.Exit(1)
	}
}
`)
	bin := filepath.Join(dir, "runner.exe")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build runner: %v\n%s", err, out)
	}
	data, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// suiteStub places the runner at path. On Windows, where path itself cannot
// run, it also places path.exe, which a path without extension runs, and
// path.cmd calling it, which node_modules/.bin holds there.
func suiteStub(t *testing.T, runner []byte, path string) {
	t.Helper()
	targets := []string{path}
	if runtime.GOOS == "windows" {
		targets = append(targets, path+".exe")
		writeFile(t, path+".cmd", "@\"%~dp0"+filepath.Base(path)+".exe\" %*\r\n@exit /b %errorlevel%\r\n")
	}
	for _, target := range targets {
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, runner, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// suiteLanguage is one language's project in its own directory: its build
// root marker, its stub test runner, its sources and a test file that
// imports none of them.
type suiteLanguage struct {
	name, root string
	markers    map[string]string
	stubs      []string // relative to root
	sourceDir  string   // relative to root
	extension  string
	test       string // relative to root
	testText   string
}

var (
	suiteTypeScript = suiteLanguage{
		name: "typescript", root: "ts",
		markers:   map[string]string{"package.json": `{"name": "ts", "devDependencies": {"vitest": "1.0.0"}}` + "\n"},
		stubs:     []string{"node_modules/.bin/vitest"},
		sourceDir: "src", extension: ".ts",
		test: "e2e/cli.test.ts", testText: "test(\"cli\", () => {});\n",
	}
	suitePython = suiteLanguage{
		name: "python", root: "py",
		markers:   map[string]string{"pyproject.toml": "[project]\nname = \"py\"\n"},
		stubs:     []string{".venv/bin/python"},
		sourceDir: "src", extension: ".py",
		test: "tests/test_cli.py", testText: "def test_cli():\n    pass\n",
	}
	suiteKotlin = suiteLanguage{
		name: "kotlin", root: "kt",
		markers:   map[string]string{"settings.gradle.kts": "rootProject.name = \"kt\"\n", "build.gradle.kts": "plugins { kotlin(\"jvm\") }\n"},
		stubs:     []string{"gradlew"},
		sourceDir: "src/main/kotlin", extension: ".kt",
		test: "src/test/kotlin/CliTest.kt", testText: "class CliTest\n",
	}
	suiteLanguages = []suiteLanguage{suiteTypeScript, suitePython, suiteKotlin}
)

// function is the source of a function name with one mutation site.
func (l suiteLanguage) function(name string) string {
	switch l.name {
	case "typescript":
		return "export function " + name + "(a: number, b: number): boolean {\n  return a > b;\n}\n"
	case "python":
		return "def " + name + "(a, b):\n    return a > b\n"
	}
	return "fun " + name + "(a: Int, b: Int): Boolean {\n    return a > b\n}\n"
}

// source is the path, from the repository root in slash form, of the
// source file base.
func (l suiteLanguage) source(base string) string {
	return l.root + "/" + l.sourceDir + "/" + base + l.extension
}

// files are the project's files with sources, each base holding the
// functions it names, all beneath l.root.
func (l suiteLanguage) files(sources map[string][]string) map[string]string {
	out := map[string]string{l.root + "/" + l.test: l.testText}
	for name, text := range l.markers {
		out[l.root+"/"+name] = text
	}
	for base, functions := range sources {
		text := ""
		for _, f := range functions {
			text += l.function(f) + "\n"
		}
		out[l.source(base)] = text
	}
	return out
}

// placeStubs writes the project's stub test runners beneath dir. Python's
// run.sh runs its interpreter, the runner, so a --test-command "./run.sh"
// judges as its own tests do.
func (l suiteLanguage) placeStubs(t *testing.T, runner []byte, dir string) {
	t.Helper()
	for _, stub := range l.stubs {
		suiteStub(t, runner, filepath.Join(dir, l.root, filepath.FromSlash(stub)))
	}
	if l.name == "python" {
		run := filepath.Join(dir, l.root, "run.sh")
		writeFile(t, run, "#!/bin/sh\nexec .venv/bin/python \"$@\"\n")
		if err := os.Chmod(run, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// suiteCommand is a --test-command line running the Python project's
// runner by its absolute path. It is quoted only when it holds a space:
// cmd, Windows's platform shell, gets a quoted line escaped in a form it
// cannot run.
func suiteCommand(dir string) string {
	path := filepath.Join(dir, "py", ".venv", "bin", "python")
	if strings.ContainsRune(path, ' ') {
		return `"` + path + `"`
	}
	return path
}

// changeTest edits the project's test file, which imports no source.
func (l suiteLanguage) changeTest(t *testing.T, dir, mark string) {
	t.Helper()
	path := filepath.Join(dir, l.root, filepath.FromSlash(l.test))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	comment := "// "
	if l.name == "python" {
		comment = "# "
	}
	writeFile(t, path, string(data)+comment+mark+"\n")
}

// suiteRun runs mutation run --json, without coverage, with args.
func suiteRun(t *testing.T, args ...string) outcome {
	t.Helper()
	o := cli(t, append([]string{"mutation", "run", "--json", "--no-annotate", "--no-coverage", "--workers", "1"}, args...)...)
	if o.code > 1 {
		t.Fatalf("mutation run %v: exit %d\n%s%s", args, o.code, o.stdout, o.stderr)
	}
	return o
}

// suiteKey is a function's key in these steps: its file in slash form, "#",
// and its name.
func suiteKey(file, function string) string {
	return filepath.ToSlash(file) + "#" + function[strings.LastIndex(function, "#")+1:]
}

// suiteCheck runs mutation check --json over files and returns each
// function's state and each stale function's message, by suiteKey.
func suiteCheck(t *testing.T, files ...string) (map[string]string, map[string]string, outcome) {
	t.Helper()
	o := cli(t, append([]string{"mutation", "check", "--json"}, osPaths(files)...)...)
	var out struct {
		Files []struct {
			File      string `json:"file"`
			Functions []struct {
				Function string `json:"function"`
				State    string `json:"state"`
			} `json:"functions"`
		} `json:"files"`
		Problems []map[string]any `json:"problems"`
	}
	if err := json.Unmarshal([]byte(o.stdout), &out); err != nil {
		t.Fatalf("mutation check --json: %v\n%s%s", err, o.stdout, o.stderr)
	}
	states, stale := map[string]string{}, map[string]string{}
	for _, f := range out.Files {
		for _, fn := range f.Functions {
			states[suiteKey(f.File, fn.Function)] = fn.State
		}
	}
	for _, p := range out.Problems {
		if p["rule"] == "mutation.stale" {
			file, _ := p["file"].(string)
			function, _ := p["function"].(string)
			stale[suiteKey(file, function)], _ = p["message"].(string)
		}
	}
	return states, stale, o
}

func osPaths(files []string) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = filepath.FromSlash(f)
	}
	return out
}

// requireNoCommand fails if o ran a test, list, coverage or baseline
// command.
func requireNoCommand(t *testing.T, o outcome) {
	t.Helper()
	for _, mark := range []string{"itos-cc: tests", "itos-cc: coverage", "itos-cc: baseline"} {
		if strings.Contains(o.stderr, mark) {
			t.Errorf("mutation check ran a command (%q):\n%s", mark, o.stderr)
		}
	}
}

// suiteReuse is, for each function of the run's --json output by
// suiteKey, whether every one of its mutants was reused.
func suiteReuse(t *testing.T, o outcome) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, f := range o.rawFiles(t) {
		file, _ := f["file"].(string)
		mutants, _ := f["mutants"].([]any)
		for _, raw := range mutants {
			m := raw.(map[string]any)
			function, _ := m["function"].(string)
			key := suiteKey(file, function)
			reused, _ := m["reused"].(bool)
			if previous, seen := out[key]; seen {
				out[key] = previous && reused
			} else {
				out[key] = reused
			}
		}
	}
	return out
}

// graphStale is, for each function the graph's builder finds under dir,
// whether it marks it stale, by suiteKey.
func graphStale(t *testing.T, dir string) map[string]bool {
	t.Helper()
	builder, err := graph.NewBuilder([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	g, _, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, node := range g.Nodes {
		for _, unit := range node.Units {
			if unit.Mutated {
				out[suiteKey(unit.File, unit.Name)] = unit.Stale
			}
		}
	}
	return out
}

// snapshotEdit rewrites the raw snapshot of source with edit applied to
// each recorded mutant.
func snapshotEdit(t *testing.T, dir, source string, edit func(map[string]any)) {
	t.Helper()
	path := filepath.Join(dir, ".metrics", "mutate", filepath.FromSlash(source)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	units, _ := snapshot["units"].([]any)
	for _, u := range units {
		mutants, _ := u.(map[string]any)["mutants"].([]any)
		for _, m := range mutants {
			edit(m.(map[string]any))
		}
	}
	if data, err = json.Marshal(snapshot); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// recordedEvidence is the whole-suite evidence of every mutant the
// snapshot of source records, one entry each, nil where it has none.
func recordedEvidence(t *testing.T, dir, source string) []map[string]any {
	t.Helper()
	var out []map[string]any
	snapshotEdit(t, dir, source, func(m map[string]any) { out = append(out, wholeSuiteEvidence(m)) })
	return out
}

// @ID-MUT-205
func TestANonImportingTestChangeMakesAnotherLanguagesWholeSuiteOutcomeStale(t *testing.T) {
	t.Parallel()
	for _, example := range []struct {
		language suiteLanguage
		scope    []string
	}{
		{suiteTypeScript, []string{"--all-tests"}},
		{suitePython, []string{"--test-command", "./run.sh"}},
		{suiteKotlin, []string{"--all-tests"}},
	} {
		l := example.language
		t.Run(l.name, func(t *testing.T) {
			if example.scope[0] == "--test-command" && runtime.GOOS == "windows" {
				t.Skip(`"./run.sh" cannot run through cmd, Windows's platform shell`)
			}
			// Given a <language> project whose test <test> does not import
			// the production file
			dir := moduleRepo(t, l.files(map[string][]string{"cli": {"above"}}))
			l.placeStubs(t, suiteRunner(t), dir)
			source, test := l.source("cli"), l.root+"/"+l.test

			// And a mutation run with <scope> recorded an outcome for that
			// file with whole-suite evidence
			first := suiteRun(t, append(slices.Clone(example.scope), filepath.FromSlash(source))...)
			if f := first.json(t).file(t, filepath.FromSlash(source)); f.Ran == 0 {
				t.Fatalf("the whole-suite run judged nothing:\n%s%s", first.stdout, first.stderr)
			}
			for _, evidence := range recordedEvidence(t, dir, source) {
				tests, _ := evidence["tests"].(map[string]any)
				if tests[test] == nil {
					t.Errorf("the whole-suite outcome records evidence %v, want it to hold %s", evidence, test)
				}
			}

			// And only that test file changed since
			l.changeTest(t, dir, "changed")

			// When I run "itos-cc mutation check --json" for the production
			// file
			states, stale, check := suiteCheck(t, source)
			// Then its function is stale and the "mutation.stale" message
			// names the changed test file
			key := source + "#above"
			if states[key] != "stale" || !strings.Contains(stale[key], test) {
				t.Errorf("after %s changed: %s is %q with message %q, want stale naming it\n%s", test, key, states[key], stale[key], check.stdout)
			}
			// And no test, list or coverage command runs
			requireNoCommand(t, check)

			// When I run mutation run with <scope> for the production file
			again := suiteRun(t, append(slices.Clone(example.scope), filepath.FromSlash(source))...)
			// Then the outcome runs again rather than being reused
			if reused := suiteReuse(t, again); reused[key] {
				t.Errorf("the whole-suite outcome was reused after %s changed\n%s", test, again.stdout)
			}
		})
	}
}

// @ID-MUT-206
func TestBuildRootsSupportFilesAndLegacyEvidenceAreExplicitForEveryLanguage(t *testing.T) {
	t.Parallel()
	// Given whole-suite outcomes in a TypeScript, a Python and a Kotlin
	// project with support globs
	files := map[string]string{
		"go/go.mod":       "module example.com/suite\n\ngo 1.22\n",
		"go/calc.go":      "package suite\n\nfunc Above(a, b int) bool { return a > b }\n",
		"go/calc_test.go": "package suite\n\nimport \"testing\"\n\nfunc TestAbove(t *testing.T) { if !Above(2, 1) || Above(1, 1) { t.Fatal(\"Above\") } }\n",
		"itos-cc.yaml": `mutation:
  tests:
    list: "touch list-ran"
    run: "touch test-ran-{pattern}"
    ids_pattern: "{ids}"
    join:
      each: "{id}"
      sep: ","
    support: ["support/*.txt"]
`,
	}
	for _, l := range suiteLanguages {
		for name, text := range l.files(map[string][]string{"cli": {"above"}}) {
			files[name] = text
		}
	}
	dir := moduleRepo(t, files)
	runner := suiteRunner(t)
	for _, l := range suiteLanguages {
		l.placeStubs(t, runner, dir)
	}
	ts, py, kt := suiteTypeScript.source("cli"), suitePython.source("cli"), suiteKotlin.source("cli")
	record := func() {
		t.Helper()
		suiteRun(t, "--all-tests", filepath.FromSlash(ts), filepath.FromSlash(kt))
		suiteRun(t, "--test-command", suiteCommand(dir), filepath.FromSlash(py))
	}
	record()

	// When a support file is added, changed or removed
	support := "support/data.txt"
	for _, change := range []string{"added", "changed", "removed"} {
		switch change {
		case "removed":
			if err := os.Remove(filepath.Join(dir, filepath.FromSlash(support))); err != nil {
				t.Fatal(err)
			}
		default:
			writeFile(t, filepath.Join(dir, filepath.FromSlash(support)), change+"\n")
		}
		// Then each outcome is stale and its message names the support file
		states, stale, check := suiteCheck(t, ts, py, kt)
		for _, source := range []string{ts, py, kt} {
			key := source + "#above"
			if states[key] != "stale" || !strings.Contains(stale[key], support) {
				t.Errorf("support file %s: %s is %q with message %q, want stale naming it\n%s", change, key, states[key], stale[key], check.stdout)
			}
		}
		requireNoCommand(t, check)
		for _, ran := range []string{"list-ran", "test-ran-*"} {
			if matches, _ := filepath.Glob(filepath.Join(dir, ran)); len(matches) > 0 {
				t.Errorf("mutation check ran a command of mutation.tests: %v", matches)
			}
		}
		record()
	}

	// When a test file is added only beneath a nested package.json, a .venv
	// or node_modules
	writeFile(t, filepath.Join(dir, "ts", "nested", "package.json"), `{"name": "nested"}`+"\n")
	writeFile(t, filepath.Join(dir, "ts", "nested", "nested.test.ts"), "test(\"nested\", () => {});\n")
	writeFile(t, filepath.Join(dir, "ts", "node_modules", "dep", "dep.test.ts"), "test(\"dep\", () => {});\n")
	writeFile(t, filepath.Join(dir, "py", ".venv", "lib", "test_dep.py"), "def test_dep():\n    pass\n")
	// Then the outer outcome stays fresh
	states, _, check := suiteCheck(t, ts, py)
	for _, source := range []string{ts, py} {
		if key := source + "#above"; states[key] != "fresh" {
			t.Errorf("a test beneath a nested root made %s %q, want fresh\n%s", key, states[key], check.stdout)
		}
	}
	reuseTS := suiteReuse(t, suiteRun(t, "--all-tests", filepath.FromSlash(ts)))
	reusePy := suiteReuse(t, suiteRun(t, "--test-command", suiteCommand(dir), filepath.FromSlash(py)))
	if !reuseTS[ts+"#above"] || !reusePy[py+"#above"] {
		t.Errorf("fresh whole-suite outcomes were not reused: TypeScript %v, Python %v", reuseTS, reusePy)
	}

	// Given a non-Go whole-suite outcome with matching source and importer
	// hashes but no whole-suite evidence
	for _, source := range []string{ts, py, kt} {
		snapshotEdit(t, dir, source, func(m map[string]any) {
			delete(m, "suite_evidence")
			delete(m, "go_evidence")
		})
	}
	// Then it is stale and must rerun before it can be reused
	states, _, check = suiteCheck(t, ts, py, kt)
	for _, source := range []string{ts, py, kt} {
		if key := source + "#above"; states[key] != "stale" {
			t.Errorf("legacy whole-suite outcome %s is %q, want stale\n%s", key, states[key], check.stdout)
		}
	}
	rerun := suiteReuse(t, suiteRun(t, "--all-tests", filepath.FromSlash(ts), filepath.FromSlash(kt)))
	for _, source := range []string{ts, kt} {
		if rerun[source+"#above"] {
			t.Errorf("legacy whole-suite outcome of %s was reused", source)
		}
	}

	// But a Go outcome recorded with the existing Go evidence key stays
	// fresh
	goSource := "go/calc.go"
	suiteRun(t, "--all-tests", filepath.FromSlash(goSource))
	snapshotEdit(t, dir, goSource, func(m map[string]any) {
		if e, ok := m["suite_evidence"]; ok {
			m["go_evidence"] = e
			delete(m, "suite_evidence")
		}
		if m["go_evidence"] == nil {
			t.Errorf("Go whole-suite outcome records no evidence: %v", m)
		}
	})
	states, _, check = suiteCheck(t, goSource)
	if key := goSource + "#Above"; states[key] != "fresh" {
		t.Errorf("Go outcome with go_evidence is %q, want fresh\n%s", states[key], check.stdout)
	}
	if reuse := suiteReuse(t, suiteRun(t, "--all-tests", filepath.FromSlash(goSource))); !reuse[goSource+"#Above"] {
		t.Errorf("Go outcome with go_evidence was not reused: %v", reuse)
	}
}

// @ID-MUT-207
func TestMixedScopesAndPartialRunsKeepTheirOwnDependenciesInEveryLanguage(t *testing.T) {
	t.Parallel()
	// Given TypeScript, Python and Kotlin outcomes recorded with own,
	// all-tests and test-command scopes
	files := map[string]string{}
	for _, l := range suiteLanguages {
		for name, text := range l.files(map[string][]string{"own": {"own"}, "whole": {"first", "second"}, "command": {"command"}}) {
			files[name] = text
		}
	}
	// A Python or Kotlin file's own tests are the tests that reach it:
	// own.py's is test_own.py, and own.kt's the class OwnTest, which no
	// step changes.
	files["py/tests/test_own.py"] = "from own import own\n\n\ndef test_own():\n    assert own(2, 1)\n"
	files["kt/src/test/kotlin/OwnTest.kt"] = "import kotlin.test.Test\n\nclass OwnTest {\n    @Test\n    fun own() {\n        own(2, 1)\n    }\n}\n"
	dir := moduleRepo(t, files)
	runner := suiteRunner(t)
	for _, l := range suiteLanguages {
		l.placeStubs(t, runner, dir)
	}
	base := gitOut(t, dir, "rev-parse", "HEAD")
	var own, whole, command, all []string
	for _, l := range suiteLanguages {
		own, whole, command = append(own, l.source("own")), append(whole, l.source("whole")), append(command, l.source("command"))
	}
	all = append(append(append(all, own...), whole...), command...)
	suiteRun(t, osPaths(own)...)
	suiteRun(t, append([]string{"--all-tests"}, osPaths(whole)...)...)
	suiteRun(t, append([]string{"--test-command", suiteCommand(dir)}, osPaths(command)...)...)

	// And only a non-importing test file changed beneath each build root
	for _, l := range suiteLanguages {
		l.changeTest(t, dir, "changed")
	}
	gitIn(t, dir, "add", "-A", "--", "ts/e2e", "py/tests", "kt/src/test")
	gitIn(t, dir, "commit", "-qm", "change the tests")

	// When their freshness is checked by mutation check, mutation run,
	// mutation sample and the graph
	want := map[string]string{}
	for _, source := range own {
		want[source+"#own"] = "fresh"
	}
	for _, source := range whole {
		want[source+"#first"], want[source+"#second"] = "stale", "stale"
	}
	for _, source := range command {
		want[source+"#command"] = "stale"
	}
	agree := func(stage string, want map[string]string) {
		t.Helper()
		// Then the own outcomes stay usable and every whole-suite outcome
		// is stale
		states, _, check := suiteCheck(t, all...)
		requireNoCommand(t, check)
		stale := graphStale(t, dir)
		sample := cli(t, "mutation", "sample", "--workers", "1", "--count", "1000", "--seed", "whole-suite", "--json")
		sampled := map[string]bool{}
		for _, m := range sample.sampled(t) {
			sampled[suiteKey(m.File, m.Function)] = true
		}
		// And all four agree on each function's freshness
		for key, state := range want {
			if states[key] != state {
				t.Errorf("%s: mutation check calls %s %q, want %s\n%s", stage, key, states[key], state, check.stdout)
			}
			if stale[key] != (state == "stale") {
				t.Errorf("%s: the graph marks %s stale=%v, want %s as mutation check", stage, key, stale[key], state)
			}
			if sampled[key] != (state == "fresh") {
				t.Errorf("%s: mutation sample drew %s=%v, want only fresh outcomes drawn, and every one with 1000 to draw", stage, key, sampled[key])
			}
		}
	}
	agree("after the test change", want)

	// And a --since run that judges one function never refreshes the
	// evidence of an unjudged one
	for _, l := range suiteLanguages {
		path := filepath.Join(dir, filepath.FromSlash(l.source("whole")))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, path, strings.Replace(string(data), l.function("first"), l.function("first")+"\n"+l.function("helper"), 1))
	}
	gitIn(t, dir, "add", "-A", "--", "ts/src", "py/src", "kt/src/main")
	gitIn(t, dir, "commit", "-qm", "add helpers")
	partial := suiteRun(t, append([]string{"--all-tests", "--since", base}, osPaths(whole)...)...)
	partialReuse := suiteReuse(t, partial)
	for _, source := range whole {
		if partialReuse[source+"#second"] {
			t.Errorf("the --since run reused unjudged %s#second", source)
		}
		want[source+"#helper"] = "fresh"
	}
	agree("after the --since run", want)

	// The run itself agrees: a plain run reuses exactly what is fresh.
	reuse := suiteReuse(t, suiteRun(t, osPaths(all)...))
	for key, state := range want {
		if strings.HasSuffix(key, "#helper") {
			continue
		}
		if reuse[key] != (state == "fresh") {
			t.Errorf("mutation run reused %s=%v, want %s as mutation check", key, reuse[key], state)
		}
	}
}
