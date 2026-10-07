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

// The scenarios of "Rule: Checking cached results without running" in
// features/mutate.feature. They use the module of mutate_since_test.go,
// whose snapshots a mutation run writes first; mutation check then reads
// them. With killedTests, TestClear kills Board#clear's one mutant too, so
// every mutant of src/board.go is killed.

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
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, name, string(data)+text)
}

// mutationCheck runs itos-cc mutation check with args.
func mutationCheck(t *testing.T, args ...string) outcome {
	t.Helper()
	return cli(t, append([]string{"mutation", "check"}, args...)...)
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

// @ID-MUT-57
func TestFreshResultsWithEveryMutantKilledPass(t *testing.T) {
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
	before, err := os.ReadFile(filepath.Join(".metrics", "mutate", "src", "board.go.json"))
	if err != nil {
		t.Fatal(err)
	}

	o := mutationCheck(t, boardSource)
	if _, err := os.Stat(marker); err == nil {
		t.Errorf("a test command ran:\n%s", o.stderr)
	}
	if _, err := os.Stat(filepath.Join(".metrics", "coverage")); err == nil {
		t.Errorf(".metrics/coverage was written: want no coverage command run")
	}
	if after, _ := os.ReadFile(filepath.Join(".metrics", "mutate", "src", "board.go.json")); string(after) != string(before) {
		t.Errorf("the snapshot changed: want mutation check to write nothing")
	}
	if want := boardSource + ": 2 fresh, 0 stale, 0 missing\n"; !strings.Contains(o.stdout, want) {
		t.Errorf("stdout:\n%s\nwant it to say %q", o.stdout, want)
	}
	if o.code != 0 {
		t.Errorf("exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}
}

// @ID-MUT-58
func TestAFunctionChangedSinceItsResultsIsStale(t *testing.T) {
	boardRepo(t, killedTests)
	if o := mutateRun(t, boardSource); o.code != 0 {
		t.Fatalf("the run: exit %d, want every mutant killed\n%s%s", o.code, o.stdout, o.stderr)
	}
	// A move is not a change.
	moveDown(t)
	if o := mutationCheck(t, "--json", boardSource); o.code != 0 {
		t.Fatalf("after a move: exit %d, want 0: a moved function is fresh\n%s", o.code, o.stdout)
	}

	edit(t, boardSource, "// past five\n", "// past five, and not at five\n")
	o := mutationCheck(t, "--json", boardSource)
	m := o.json(t)
	wantProblem(t, m.problem("mutation.stale"), map[string]any{"file": boardSource, "line": 6.0, "function": placeID}, o.stdout)
	if !slices.Equal(m.rules(), []string{"mutation.stale"}) {
		t.Errorf("problems %v, want only place stale", m.Problems)
	}
	if o.code != 1 {
		t.Errorf("exit %d, want 1", o.code)
	}
}

// @ID-MUT-59
func TestAFunctionWithNoResultsIsMissing(t *testing.T) {
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

// @ID-MUT-60
func TestARecordedSurvivorFailsTheCheck(t *testing.T) {
	boardRepo(t, nil)
	if o := mutateRun(t, boardSource); o.code != 1 {
		t.Fatalf("the run: exit %d, want 1 for clear's survivor\n%s%s", o.code, o.stdout, o.stderr)
	}

	o := mutationCheck(t, boardSource)
	if want := "  survived " + boardSource + ":11:11 `<` → `<=` in " + clearID + "\n"; !strings.Contains(o.stdout, want) {
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

// @ID-MUT-61
func TestARecordedUncoveredMutantFailsOnlyWithFailUncovered(t *testing.T) {
	boardRepo(t, nil)
	if o := mutateCovered(t, boardSource); o.code != 0 {
		t.Fatalf("the run: exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}
	if u := boardUnits(t)[clearID]; u.Uncovered != 1 {
		t.Fatalf("clear in the snapshot: %+v, want its one mutant uncovered", u)
	}

	if o := mutationCheck(t, boardSource); o.code != 0 {
		t.Errorf("without --fail-uncovered: exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}
	o := mutationCheck(t, "--fail-uncovered", "--json", boardSource)
	m := o.json(t)
	wantProblem(t, m.problem("mutation.uncovered"), map[string]any{"file": boardSource, "line": float64(clearLine),
		"column": float64(clearColumn), "function": clearID, "original": "<", "replacement": "<="}, o.stdout)
	if o.code != 1 {
		t.Errorf("with --fail-uncovered: exit %d, want 1", o.code)
	}
}

// @ID-MUT-62
func TestWithSinceOnlyTheFunctionsTheRangeChangedAreChecked(t *testing.T) {
	boardRepo(t, nil)
	if o := mutateRun(t); o.code != 1 {
		t.Fatalf("the first run: exit %d, want 1 for clear's survivor\n%s%s", o.code, o.stdout, o.stderr)
	}
	changePlace(t)
	if o := mutateRun(t, "--since", "base"); o.code != 0 {
		t.Fatalf("the run since base: exit %d, want place killed\n%s%s", o.code, o.stdout, o.stderr)
	}
	if u := boardUnits(t)[clearID]; u.Survived != 1 {
		t.Fatalf("clear in the snapshot: %+v, want its recorded survivor", u)
	}
	// Without --since, clear's survivor fails the check.
	if o := mutationCheck(t); o.code != 1 {
		t.Fatalf("without --since: exit %d, want 1 for clear's survivor\n%s%s", o.code, o.stdout, o.stderr)
	}

	o := mutationCheck(t, "--since", "base")
	if o.code != 0 {
		t.Errorf("exit %d, want 0: clear is not in the range\n%s%s", o.code, o.stdout, o.stderr)
	}
}

// @ID-MUT-63
func TestAFunctionWithNoMutationSiteNeedsNoResults(t *testing.T) {
	boardRepo(t, killedTests)
	if o := mutateRun(t, boardSource); o.code != 0 {
		t.Fatalf("the run: exit %d, want every mutant killed\n%s%s", o.code, o.stdout, o.stderr)
	}
	appendTo(t, boardSource, "\nfunc (b *Board) size() int {\n\treturn 7\n}\n")
	for _, s := range scanned(t, boardSource) {
		if s.Function == sizeID {
			t.Fatalf("size has the site %+v, want none", s)
		}
	}
	if u, ok := boardUnits(t)[sizeID]; ok {
		t.Fatalf("size in the snapshot: %+v, want no entry", u)
	}

	o := mutationCheck(t, boardSource)
	if o.code != 0 {
		t.Errorf("exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}
}

// @ID-MUT-64
func TestEachFunctionsStateAsJSON(t *testing.T) {
	boardRepo(t, killedTests)
	if o := mutateRun(t, boardSource); o.code != 0 {
		t.Fatalf("the run: exit %d, want every mutant killed\n%s%s", o.code, o.stdout, o.stderr)
	}
	edit(t, boardSource, "// past five\n", "// past five, and not at five\n")
	appendTo(t, boardSource, resetSource)

	o := mutationCheck(t, "--json", boardSource)
	functions := o.checkFunctions(t)[boardSource]
	want := []checkFunctionJSON{
		{Function: placeID, State: "stale", Killed: 1},
		{Function: clearID, State: "fresh", Killed: 1},
		{Function: resetID, State: "missing"},
	}
	if !slices.Equal(functions, want) {
		t.Errorf("functions of %s:\n%+v\nwant:\n%+v\n%s", boardSource, functions, want, o.stdout)
	}
	m := o.json(t)
	if !slices.Equal(m.rules(), []string{"mutation.stale", "mutation.missing"}) {
		t.Fatalf("problems %v, want place stale and reset missing", m.Problems)
	}

	plain := mutationCheck(t, boardSource)
	var listed []string
	for _, l := range strings.SplitAfter(plain.stdout, "\n") {
		if strings.HasPrefix(l, "  ") {
			listed = append(listed, l)
		}
	}
	var fromJSON []string
	for _, p := range m.Problems {
		fromJSON = append(fromJSON, fmt.Sprintf("  %s %s:%v in %s\n",
			strings.TrimPrefix(p["rule"].(string), "mutation."), p["file"], p["line"], p["function"]))
	}
	if !slices.Equal(listed, fromJSON) {
		t.Errorf("plain output lists:\n%q\nwant the problems of --json:\n%q", listed, fromJSON)
	}
	if plain.code != o.code || o.code != 1 {
		t.Errorf("exit %d plain, %d with --json, want 1 both", plain.code, o.code)
	}
}
