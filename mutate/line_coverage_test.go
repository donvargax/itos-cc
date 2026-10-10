package mutate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/coverage"
)

// Go evidence is written under go_coverage exactly as before, and the
// evidence of a line-precision language under line_coverage, each reading
// back as the unit's Coverage.
func TestUnitCoverageEvidenceKeepsGoKeyAndWritesLinesBesideIt(t *testing.T) {
	goUnit := UnitResult{Namespace: "main", Name: "a", Mutants: []Mutant{{Original: ">", Replacement: ">="}},
		Coverage: &CoverageEvidence{Version: goCoverageEvidenceVersion, File: "main.go", Function: "main#a", Complete: true,
			Blocks: []CoverageBlock{{Span: "3.20,3.30", Line: 3, Column: 20, Weight: 1, Covered: true}}}}
	type plain UnitResult
	before, err := json.Marshal(plain(goUnit))
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(goUnit)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("Go unit written as\n%s\nwant as before\n%s", after, before)
	}
	pyUnit := UnitResult{Namespace: "calc", Name: "dormant", Mutants: []Mutant{},
		Coverage: &CoverageEvidence{Version: lineCoverageEvidenceVersion, Language: "python", File: "calc.py", Function: "calc#dormant", Complete: true,
			Blocks: []CoverageBlock{{Span: "8", Line: 8, Weight: 1}}}}
	data, err := json.Marshal(pyUnit)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"line_coverage":{"version":1,"language":"python"`) || strings.Contains(string(data), "go_coverage") {
		t.Errorf("Python unit written as %s, want its evidence under line_coverage", data)
	}
	for _, unit := range []UnitResult{goUnit, pyUnit} {
		data, err := json.Marshal(unit)
		if err != nil {
			t.Fatal(err)
		}
		var read UnitResult
		if err := json.Unmarshal(data, &read); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(read, unit) {
			t.Errorf("%s read back as %+v, want %+v", data, read, unit)
		}
	}
}

// A function's lines are those of a proven inventory from its body to its
// end: its def line, run at import, is not its own.
func TestLineCoverageEvidenceTakesTheFunctionsBodyLines(t *testing.T) {
	f := parse(t, "calc.py", "def reached(i, strict):\n    if strict:\n        raise ValueError(\"strict\")\n    return i > 5\n\n\ndef dormant(name):\n    label = name.strip()\n    return label\n")
	defer f.Close()
	lines := []coverage.Line{{Line: 1, Covered: true}, {Line: 2, Covered: true}, {Line: 3}, {Line: 4, Covered: true}, {Line: 7, Covered: true}, {Line: 8}, {Line: 9}}
	got := map[string][]int{}
	for _, unit := range f.Units {
		e := lineCoverageEvidence(f, unit, lines, true, "p", nil)
		if !e.Complete || e.Language != "python" || e.Version != lineCoverageEvidenceVersion {
			t.Errorf("%s evidence %+v, want complete Python line evidence", unit.Name, e)
		}
		for _, b := range e.Blocks {
			if !b.Covered {
				got[unit.Name] = append(got[unit.Name], b.Line)
			}
		}
		if n := len(e.Blocks); unit.Name == "reached" && n != 3 || unit.Name == "dormant" && n != 2 {
			t.Errorf("%s has %d lines %+v, want its body's", unit.Name, n, e.Blocks)
		}
		if unproven := lineCoverageEvidence(f, unit, lines, false, "p", nil); unproven.Complete || len(unproven.Blocks) != 0 {
			t.Errorf("%s evidence without proof %+v, want incomplete and empty", unit.Name, unproven)
		}
	}
	if want := map[string][]int{"reached": {3}, "dormant": {8, 9}}; !reflect.DeepEqual(got, want) {
		t.Errorf("uncovered lines %v, want %v", got, want)
	}
}

// Python evidence rests on every Python file and configuration file of the
// build root, without itos-cc's own summary comment, and leaves out
// virtualenvs, hidden directories and nested build roots.
func TestPythonCoverageInputsFingerprintTheBuildRoot(t *testing.T) {
	dir := t.TempDir()
	for name, text := range map[string]string{
		"pyproject.toml":              "[project]\nname = \"m\"\n",
		"calc.py":                     "def a():\n    return 1\n",
		"tests/test_calc.py":          "def test_a():\n    pass\n",
		"tests/conftest.py":           "",
		".coveragerc":                 "[run]\n",
		"README.md":                   "readme\n",
		".venv/pyvenv.cfg":            "home = /usr\n",
		".venv/lib/site.py":           "",
		"env/pyvenv.cfg":              "home = /usr\n",
		"env/lib/site.py":             "",
		"nested/pyproject.toml":       "[project]\nname = \"n\"\n",
		"nested/other.py":             "",
		"__pycache__/calc.cpython.py": "",
	} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	inputs, err := PythonCoverageInputs(filepath.Join(dir, "calc.py"), dir, "producer", map[string]string{"features/a.feature": "h"})
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for key := range inputs {
		keys = append(keys, key)
	}
	want := []string{".coveragerc", "@producer", "@python-env", "calc.py", "itos-cc.yaml", "pyproject.toml", "support:features/a.feature", "tests/conftest.py", "tests/test_calc.py"}
	if !reflect.DeepEqual(sortedKeys(inputs), want) {
		t.Errorf("inputs %v, want %v", keys, want)
	}
	annotated := "def a():\n    return 1\n\n# itos-cc mutation: 0 killed, 0 survived, 0 uncovered\n# end itos-cc mutation\n"
	if err := os.WriteFile(filepath.Join(dir, "calc.py"), []byte(annotated), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := PythonCoverageInputs(filepath.Join(dir, "calc.py"), dir, "producer", map[string]string{"features/a.feature": "h"})
	if err != nil {
		t.Fatal(err)
	}
	if changed := GoCoverageChanges(inputs, again); len(changed) != 0 {
		t.Errorf("the summary comment changed inputs %v", changed)
	}
}

func sortedKeys(m map[string]string) []string {
	var out []string
	for key := range m {
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}

// TypeScript units take their lines from their body's first statement: an
// arrow function bound to a name runs its declaration line at import, as
// Vitest's v8 provider reports it, and a function declaration's own line is
// no line v8 names at all.
func TestTypeScriptLineEvidenceLeavesTheDeclarationLineOut(t *testing.T) {
	f := parse(t, "calc.ts", "export function dormant(name: string): string {\n  const label = name.trim();\n  return label;\n}\n\n"+
		"export const arrow = (n: number): number => {\n  return n * 2;\n};\n\nexport class Box {\n  open(x: number): number {\n    return x;\n  }\n}\n")
	defer f.Close()
	// The lines Vitest 5's v8 LCOV names for this file when a test imports
	// it and calls nothing: the arrow's declaration line ran.
	lines := []coverage.Line{{Line: 2}, {Line: 3}, {Line: 6, Covered: true}, {Line: 7}, {Line: 12}}
	got := map[string][]int{}
	for _, unit := range f.Units {
		for _, b := range lineCoverageEvidence(f, unit, lines, true, "p", nil).Blocks {
			if b.Covered {
				t.Errorf("%s holds the covered line %d", unit.Name, b.Line)
			}
			got[unit.Name] = append(got[unit.Name], b.Line)
		}
	}
	if want := map[string][]int{"dormant": {2, 3}, "arrow": {7}, "open": {12}}; !reflect.DeepEqual(got, want) {
		t.Errorf("lines %v, want %v", got, want)
	}
}

// TypeScript evidence rests on every TypeScript and JavaScript file and the
// configuration of the package root, and leaves out node_modules, hidden
// directories, nested packages and the root's build outputs.
func TestTypeScriptCoverageInputsFingerprintThePackageRoot(t *testing.T) {
	dir := t.TempDir()
	for name, text := range map[string]string{
		"package.json":                "{}\n",
		"package-lock.json":           "{}\n",
		"tsconfig.build.json":         "{}\n",
		"vitest.config.ts":            "export default {};\n",
		"src/calc.ts":                 "export const a = 1;\n",
		"src/calc.test.ts":            "",
		"src/view.tsx":                "",
		"scripts/run.mjs":             "",
		"README.md":                   "",
		"node_modules/x/index.js":     "",
		".cache/y.js":                 "",
		"dist/calc.js":                "",
		"coverage/lcov-report/x.js":   "",
		"packages/inner/package.json": "{}\n",
		"packages/inner/z.ts":         "",
		"src/dist/kept.ts":            "",
	} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	inputs, err := TypeScriptCoverageInputs(filepath.Join(dir, "src", "calc.ts"), dir, "producer", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"@node-env", "@producer", "itos-cc.yaml", "package-lock.json", "package.json", "scripts/run.mjs",
		"src/calc.test.ts", "src/calc.ts", "src/dist/kept.ts", "src/view.tsx", "tsconfig.build.json", "vitest.config.ts"}
	if got := sortedKeys(inputs); !reflect.DeepEqual(got, want) {
		t.Errorf("inputs %v, want %v", got, want)
	}
}
