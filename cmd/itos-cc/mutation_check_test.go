package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Tests of "Rule: Checking cached results without running" in
// features/mutate.feature, over the module of mutate_since_test.go, whose
// snapshots a mutation run writes first; mutation check then reads them.
// With killedTests, TestClear kills Board#clear's one mutant too, so every
// mutant of src/board.go is killed. The scenarios hold ID-MUT-58 and 61 to
// 64 whole (T-101); each test left holds an assertion no scenario does.

const (
	resetID = "example.com/m/src.Board#reset"
	sizeID  = "example.com/m/src.Board#size"
)

var killedTests = map[string]string{
	"src/board_test.go": `package board

import "testing"

func TestPlace(t *testing.T) {
	var b Board
	if b.place(5) || !b.place(6) {
		t.Fatal("place")
	}
}

func TestClear(t *testing.T) {
	var b Board
	if !b.clear(2) || b.clear(3) {
		t.Fatal("clear")
	}
}
`,
}

// reset has one mutation site, `==` → `!=`; once appended to src/board.go
// it starts on line 14.
const resetSource = "\nfunc (b *Board) reset(i int) bool {\n\treturn i == 0\n}\n"

// moveDown adds a line above every function of src/board.go, so each moves
// down a line without changing.
func moveDown(t *testing.T) {
	t.Helper()
	edit(t, boardSource, "type Board struct{}\n", "// Board is a board.\ntype Board struct{}\n")
}

func appendTo(t *testing.T, name, text string) {
	t.Helper()
	data, err := os.ReadFile(inWD(t, name))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, name, string(data)+text)
}

// mutationCheck runs itos-cc mutation check with args.
func mutationCheck(t *testing.T, args ...string) outcome {
	t.Helper()
	return itosCc(t, append([]string{"mutation", "check"}, args...)...)
}

type checkFunctionJSON struct {
	Function  string `json:"function"`
	State     string `json:"state"`
	Killed    int    `json:"killed"`
	Survived  int    `json:"survived"`
	Uncovered int    `json:"uncovered"`
}

// checkFunctions is the "functions" of each file in "files", by file.
func (o outcome) checkFunctions(t *testing.T) map[string][]checkFunctionJSON {
	t.Helper()
	var out struct {
		Files []struct {
			File      string               `json:"file"`
			Functions *[]checkFunctionJSON `json:"functions"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(o.stdout), &out); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s\nstderr:\n%s", err, o.stdout, o.stderr)
	}
	files := map[string][]checkFunctionJSON{}
	for _, f := range out.Files {
		if f.Functions == nil {
			t.Fatalf("%s has no \"functions\" in:\n%s", f.File, o.stdout)
		}
		files[f.File] = *f.Functions
	}
	return files
}

// rules is the rule of each problem, in order.
func (m mutateJSON) rules() []string {
	var rules []string
	for _, p := range m.Problems {
		rules = append(rules, p["rule"].(string))
	}
	return rules
}

// wantProblem fails t unless p holds each key of want.
func wantProblem(t *testing.T, p map[string]any, want map[string]any, stdout string) {
	t.Helper()
	if p == nil {
		t.Fatalf("no such problem in:\n%s", stdout)
	}
	for k, v := range want {
		if p[k] != v {
			t.Errorf("problem %s = %v, want %v\n%s", k, p[k], v, stdout)
		}
	}
}

// @ID-MUT-57. Kept: the marker its tests write when anything runs them is
// first shown to work, so its absence after check proves no test ran; the
// scenario's marker has no such positive control, and the memory's Workers
// write none.
func TestFreshResultsWithEveryMutantKilledPass(t *testing.T) {
	t.Parallel()
	dir := boardRepo(t, killedTests)
	// Any test or coverage run of package board writes the marker. It is
	// in place before the run, since a test changed after it would make
	// its results stale.
	marker := filepath.Join(dir, "tests-ran")
	appendTo(t, filepath.FromSlash("src/board_test.go"),
		fmt.Sprintf("\nfunc init() { os.WriteFile(%s, nil, 0o644) }\n", strconv.Quote(filepath.ToSlash(marker))))
	edit(t, filepath.FromSlash("src/board_test.go"), `import "testing"`, "import (\n\t\"os\"\n\t\"testing\"\n)")
	if o := mutateRun(t, boardSource); o.code != 0 {
		t.Fatalf("the run: exit %d, want every mutant killed\n%s%s", o.code, o.stdout, o.stderr)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatalf("the run's tests wrote no marker: %v", err)
	}
	before, err := os.ReadFile(inWD(t, filepath.Join(".metrics", "mutate", "src", "board.go.json")))
	if err != nil {
		t.Fatal(err)
	}

	o := mutationCheck(t, boardSource)
	if _, err := os.Stat(marker); err == nil {
		t.Errorf("a test command ran:\n%s", o.stderr)
	}
	if _, err := os.Stat(inWD(t, filepath.Join(".metrics", "coverage"))); err == nil {
		t.Errorf(".metrics/coverage was written: want no coverage command run")
	}
	if after, _ := os.ReadFile(inWD(t, filepath.Join(".metrics", "mutate", "src", "board.go.json"))); string(after) != string(before) {
		t.Errorf("the snapshot changed: want mutation check to write nothing")
	}
	if want := "2 fresh, 0 stale, 0 missing in " + boardSource + "\n"; !strings.Contains(o.stdout, want) {
		t.Errorf("stdout:\n%s\nwant it to say %q", o.stdout, want)
	}
	if o.code != 0 {
		t.Errorf("exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}
}

// @ID-MUT-59. Kept: the missing problem's line, the added function's own,
// is asserted by no scenario.
func TestAFunctionWithNoResultsIsMissing(t *testing.T) {
	t.Parallel()
	boardRepo(t, killedTests)
	// With no snapshot at all, every function with a site is missing.
	o := mutationCheck(t, "--json", boardSource)
	m := o.json(t)
	if !slices.Equal(m.rules(), []string{"mutation.missing", "mutation.missing"}) || o.code != 1 {
		t.Errorf("no snapshot: exit %d, problems %v, want place and clear missing and exit 1", o.code, m.Problems)
	}
	if o := mutateRun(t, boardSource); o.code != 0 {
		t.Fatalf("the run: exit %d, want every mutant killed\n%s%s", o.code, o.stdout, o.stderr)
	}

	appendTo(t, boardSource, resetSource)
	o = mutationCheck(t, "--json", boardSource)
	m = o.json(t)
	wantProblem(t, m.problem("mutation.missing"), map[string]any{"file": boardSource, "line": 14.0, "function": resetID}, o.stdout)
	if !slices.Equal(m.rules(), []string{"mutation.missing"}) {
		t.Errorf("problems %v, want only reset missing", m.Problems)
	}
	if o.code != 1 {
		t.Errorf("exit %d, want 1", o.code)
	}
}

// @ID-MUT-60. Kept: check's plain listing of a survivor, as mutation run
// prints it, is asserted by no scenario.
func TestARecordedSurvivorFailsTheCheck(t *testing.T) {
	t.Parallel()
	boardRepo(t, nil)
	if o := mutateRun(t, boardSource); o.code != 1 {
		t.Fatalf("the run: exit %d, want 1 for clear's survivor\n%s%s", o.code, o.stdout, o.stderr)
	}

	o := mutationCheck(t, boardSource)
	if want := "  survived `<` → `<=` in " + clearID + ", " + boardSource + " line 11 column 11\n"; !strings.Contains(o.stdout, want) {
		t.Errorf("stdout:\n%s\nwant it to list %q, as mutation run does", o.stdout, want)
	}
	if o.code != 1 {
		t.Errorf("exit %d, want 1\n%s%s", o.code, o.stdout, o.stderr)
	}

	// Moved, the survivor is where it is now, as a run would report it.
	moveDown(t)
	o = mutationCheck(t, "--json", boardSource)
	m := o.json(t)
	wantProblem(t, m.problem("mutation.survived"), map[string]any{"file": boardSource, "line": 12.0, "column": 11.0,
		"function": clearID, "original": "<", "replacement": "<="}, o.stdout)
	if !slices.Equal(m.rules(), []string{"mutation.survived"}) || o.code != 1 {
		t.Errorf("exit %d, problems %v, want clear's survivor alone and exit 1", o.code, m.Problems)
	}
}
