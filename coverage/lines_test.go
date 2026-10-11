package coverage

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// A successful plan proves the lines of the files its report names, and
// leaves those it never names unloaded; a failed one proves nothing.
func TestLinesNeedAPlanThatMeasuredItsLanguage(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.py"), filepath.Join(dir, "b.py")
	entries, err := ParseLCOV(strings.NewReader("SF:a.py\nDA:2,1\nDA:3,0\nend_of_record\n"))
	if err != nil {
		t.Fatal(err)
	}
	r := Build([]string{a, b}, dir, entries)
	Plan{Language: "python", Dir: dir, Sources: []string{a, b}}.prove(r)
	if lines, ok := r.Lines(a); !ok || !reflect.DeepEqual(lines, []Line{{Line: 2, Covered: true}, {Line: 3}}) {
		t.Errorf("a's lines %v %v, want 2 covered and 3 not, proven", lines, ok)
	}
	if lines, ok := r.Lines(b); ok || lines != nil {
		t.Errorf("b's lines %v %v, want none proven", lines, ok)
	}
	if got := r.Unloaded("python"); !reflect.DeepEqual(got, []string{b}) {
		t.Errorf("unloaded %v, want b alone", got)
	}
	failed := Build([]string{a}, dir, entries)
	if _, ok := failed.Lines(a); ok {
		t.Error("a report no successful plan proved proves a's lines")
	}
	merged := Merge(&Report{}, r)
	if _, ok := merged.Lines(a); !ok || !reflect.DeepEqual(merged.Unloaded("python"), []string{b}) {
		t.Error("merging lost what the plan proved")
	}
}

// coverage.py's own analysis lists the executable lines of a file no test
// loaded, running no test, all of them unexecuted.
func TestInventoryListsTheExecutableLinesOfUnloadedPythonFiles(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil || exec.Command("python3", "-c", "import coverage").Run() != nil {
		t.Skip("python3 with coverage.py is not installed")
	}
	dir := t.TempDir()
	lonely := filepath.Join(dir, "lonely.py")
	text := "\"\"\"Doc.\"\"\"\n\n\ndef lonely(name):\n    \"\"\"Doc.\"\"\"\n    label = name.strip()\n    return label  # pragma: no cover\n"
	if err := os.WriteFile(lonely, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	r := (Plan{Language: "python", Dir: dir, Sources: []string{lonely}, Unreached: true}).unreached(io.Discard)
	calls, err := r.Inventory(context.Background(), "python", io.Discard, nil)
	if err != nil || len(calls) != 1 {
		t.Fatalf("inventory: %v, calls %v", err, calls)
	}
	lines, ok := r.Lines(lonely)
	if want := []Line{{Line: 4}, {Line: 6}}; !ok || !reflect.DeepEqual(lines, want) {
		t.Errorf("lonely's lines %v %v, want %v proven", lines, ok, want)
	}
	if got := r.Unloaded("python"); len(got) != 0 {
		t.Errorf("unloaded after the inventory %v, want none", got)
	}
}

// Vitest's v8 provider, made to run no test, lists the executable lines of
// a file no test loaded, all of them unexecuted.
func TestInventoryListsTheExecutableLinesOfUnloadedTypeScriptFiles(t *testing.T) {
	modules, err := filepath.Abs(filepath.Join("..", "testdata", "tools", "node", "node_modules"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(modules, "@vitest", "coverage-v8")); err != nil {
		t.Skip("Vitest with @vitest/coverage-v8 is not installed in testdata/tools/node")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	dir := t.TempDir()
	if err := os.Symlink(modules, filepath.Join(dir, "node_modules")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	files := map[string]string{
		"package.json":  `{"name": "m", "type": "module", "devDependencies": {"vitest": "5.0.2", "@vitest/coverage-v8": "5.0.2"}}` + "\n",
		"src/lonely.ts": "export function lonely(name: string): string {\n  const label = name.trim();\n  return label;\n}\n",
	}
	for name, text := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lonely := filepath.Join(dir, "src", "lonely.ts")
	r := (Plan{Language: "typescript", Dir: dir, Sources: []string{lonely}}).unreached(io.Discard)
	calls, err := r.Inventory(context.Background(), "typescript", io.Discard, nil)
	if err != nil || len(calls) != 1 {
		t.Fatalf("inventory: %v, calls %v", err, calls)
	}
	lines, ok := r.Lines(lonely)
	if want := []Line{{Line: 2}, {Line: 3}}; !ok || !reflect.DeepEqual(lines, want) {
		t.Errorf("lonely's lines %v %v, want %v proven", lines, ok, want)
	}
}

// koverReport is Kover's XML report, JaCoCo's format, of a module of
// package calc where a test ran reached: line 4 ran one of its branches,
// line 5 never ran, and line 3, reached's declaration, holds the bridge of
// a default argument no call omitted. Lonely.kt, of package other, no test
// loaded, yet the report lists it, as every class of the module's output.
const koverReport = `<?xml version="1.0" ?>
<report name="Kover Gradle Plugin XML report for :">
<package name="calc">
<class name="calc/CalcKt" sourcefilename="Calc.kt">
<method name="reached" desc="(IZLjava/lang/String;)Z" line="4">
<counter type="INSTRUCTION" missed="5" covered="7"/>
</method>
</class>
<sourcefile name="Calc.kt">
<line nr="3" mi="9" ci="0" mb="0" cb="0"/>
<line nr="4" mi="0" ci="2" mb="1" cb="1"/>
<line nr="5" mi="5" ci="0" mb="0" cb="0"/>
<line nr="7" mi="1" ci="9" mb="1" cb="3"/>
<counter type="INSTRUCTION" missed="15" covered="11"/>
</sourcefile>
</package>
<package name="other">
<sourcefile name="Lonely.kt">
<line nr="4" mi="7" ci="0" mb="0" cb="0"/>
<line nr="5" mi="0" ci="1" mb="0" cb="0"/>
</sourcefile>
</package>
</report>
`

// Kover's XML parses as JaCoCo's does, a partly executed line covered. A
// file of the module no test reaches, whose Unreached plan runs no test,
// has the lines the report of a successful plan of its module lists, all
// unexecuted; one of another module, with no such report, stays unproven.
func TestAKotlinFileNoTestReachesHasTheLinesItsModulesReportLists(t *testing.T) {
	dir := t.TempDir()
	module, elsewhere := filepath.Join(dir, "app"), filepath.Join(dir, "tool")
	calc := filepath.Join(module, "src", "main", "kotlin", "calc", "Calc.kt")
	lonely := filepath.Join(module, "src", "main", "kotlin", "other", "Lonely.kt")
	far := filepath.Join(elsewhere, "src", "main", "kotlin", "other", "Far.kt")
	for _, path := range []string{calc, lonely, far} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	report := filepath.Join(module, "build", "reports", "kover", "report.xml")
	t.Setenv("HELPER_COVERAGE_REPORT", report)
	t.Setenv("HELPER_COVERAGE_TEXT", koverReport)
	t.Setenv("HELPER_COVERAGE_FAIL", "")
	plans := []Plan{
		{Language: "kotlin", Dir: module, Sources: []string{lonely}, Unreached: true},
		{Language: "kotlin", Dir: elsewhere, Sources: []string{far}, Unreached: true},
		{Language: "kotlin", Dir: module, Sources: []string{calc}, OwnSources: true, Reports: []string{report},
			Commands: [][]string{{os.Args[0], "-test.run=^TestHelperCoverageCommand$"}}},
	}
	r := Run(plans, []string{calc, lonely, far}, io.Discard)
	if lines, ok := r.Lines(calc); !ok || !reflect.DeepEqual(lines, []Line{{Line: 3}, {Line: 4, Covered: true}, {Line: 5}, {Line: 7, Covered: true}}) {
		t.Errorf("Calc.kt's lines %v %v, want 4 and 7 covered, 3 and 5 not, proven", lines, ok)
	}
	if lines, ok := r.Lines(lonely); !ok || !reflect.DeepEqual(lines, []Line{{Line: 4}, {Line: 5}}) {
		t.Errorf("Lonely.kt's lines %v %v, want 4 and 5 unexecuted, proven", lines, ok)
	}
	if lines, ok := r.Lines(far); ok || lines != nil {
		t.Errorf("Far.kt's lines %v %v, want none proven", lines, ok)
	}
	if got := r.Unloaded("kotlin"); !reflect.DeepEqual(got, []string{far}) {
		t.Errorf("unloaded %v, want Far.kt alone", got)
	}
	// Without a successful plan of its module, a file no test reaches has
	// nothing listed.
	t.Setenv("HELPER_COVERAGE_FAIL", "1")
	if _, ok := Run(plans, []string{calc, lonely, far}, io.Discard).Lines(lonely); ok {
		t.Error("a failed plan's report lists Lonely.kt's lines")
	}
}
