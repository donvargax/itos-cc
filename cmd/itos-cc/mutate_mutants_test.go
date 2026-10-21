package main

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"

	mutationslice "github.com/donvargax/itos-cc/internal/slices/mutation"
)

// The scenarios of "Rule: Results" that list every mutant in --json, in
// features/mutate.feature. They use the module of mutate_since_test.go:
// Board#place's one mutant is killed by TestPlace, and Board#clear's one
// mutant survives whenever it runs and is uncovered when coverage is
// measured.

// mutantJSON is one entry of a file's "mutants".
type mutantJSON struct {
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Function    string `json:"function"`
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
	Outcome     string `json:"outcome"`
	Reused      bool   `json:"reused"`
	// Scope is the scope of the tests that decided the outcome.
	Scope string `json:"scope"`
	// Excepted is the reason itos-cc.yaml gives, on an excepted survivor
	// only.
	Excepted string `json:"excepted"`
}

// mutantKeys are the keys of each mutant: those of a mutation list site less
// "file", plus "outcome", "reused", and "scope".
var mutantKeys = []string{"column", "function", "line", "original", "outcome", "replacement", "reused", "scope"}

// mutants is the "mutants" of f, which must be there.
func (f fileJSON) mutants(t *testing.T) []mutantJSON {
	t.Helper()
	if f.Mutants == nil {
		t.Fatalf("%s has no \"mutants\": %+v", f.File, f)
	}
	return *f.Mutants
}

// rawFiles is "files" of the --json output as plain maps, to see its keys.
func (o outcome) rawFiles(t *testing.T) []map[string]any {
	t.Helper()
	var out struct {
		Files []map[string]any `json:"files"`
	}
	if err := json.Unmarshal([]byte(o.stdout), &out); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, o.stdout)
	}
	return out.Files
}

// scanned is the sites mutation list lists in file.
func scanned(t *testing.T, file string) []mutationslice.Site {
	t.Helper()
	var scan struct {
		Sites []mutationslice.Site `json:"sites"`
	}
	o := itosCc(t, "mutation", "list", "--json", file)
	if err := json.Unmarshal([]byte(o.stdout), &scan); err != nil {
		t.Fatalf("sites of %s: %v\n%s", file, err, o.stdout)
	}
	return scan.Sites
}

// @ID-MUT-52. Kept: each mutant holds exactly its keys, "coverage" beside
// them once the tests executed it, where the scenario asserts only that each
// is there. ID-MUT-53 to 56 hold their old tests whole (T-101).
func TestEveryMutantAsJSON(t *testing.T) {
	t.Parallel()
	// Board#full is executed but its test cannot tell > from >=, so its
	// mutant survives; Board#clear is never executed.
	fullID := "example.com/m/src.Board#full"
	boardRepo(t, map[string]string{
		"src/board.go": `package board

type Board struct{}

func (b *Board) place(i int) bool {
	// past five
	return i > 5
}

func (b *Board) full(i int) bool {
	return i >= 9
}

func (b *Board) clear(i int) bool {
	return i < 3
}
`,
		"src/board_test.go": `package board

import "testing"

func TestPlace(t *testing.T) {
	var b Board
	if b.place(5) || !b.place(6) {
		t.Fatal("place")
	}
}

func TestFull(t *testing.T) {
	var b Board
	if !b.full(20) {
		t.Fatal("full")
	}
}
`,
	})
	sites := scanned(t, boardSource)
	if len(sites) != 3 {
		t.Fatalf("sites %+v, want one in each function", sites)
	}
	outcomes := map[string]string{placeID: "killed", fullID: "survived", clearID: "uncovered"}
	var want []mutantJSON
	for _, s := range sites {
		want = append(want, mutantJSON{Line: s.Line, Column: s.Column, Function: s.Function,
			Original: s.Original, Replacement: s.Replacement, Outcome: outcomes[s.Function], Scope: "own"})
	}

	o := mutateCovered(t, "--json", boardSource)
	got := o.json(t).file(t, boardSource).mutants(t)
	if !slices.Equal(got, want) {
		t.Errorf("mutants:\n%+v\nwant, in site order:\n%+v\nstderr:\n%s", got, want, o.stderr)
	}
	for _, f := range o.rawFiles(t) {
		list, _ := f["mutants"].([]any)
		for _, m := range list {
			// A mutant the tests executed also says which coverage did
			// (ID-MUT-119).
			want := mutantKeys
			if m.(map[string]any)["outcome"] != "uncovered" {
				want = slices.Sorted(slices.Values(append([]string{"coverage"}, mutantKeys...)))
			}
			if keys := slices.Sorted(maps.Keys(m.(map[string]any))); !slices.Equal(keys, want) {
				t.Errorf("mutant keys %q, want %q", keys, want)
			}
		}
	}
}
