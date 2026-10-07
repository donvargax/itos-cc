package main

import (
	"encoding/json"
	"maps"
	"path/filepath"
	"slices"
	"testing"
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
	// Excepted is the reason itos-cc.yaml gives, on an excepted survivor
	// only.
	Excepted string `json:"excepted"`
}

// mutantKeys are the keys of each mutant: those of a mutation list site less
// "file", plus "outcome" and "reused".
var mutantKeys = []string{"column", "function", "line", "original", "outcome", "replacement", "reused"}

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
func scanned(t *testing.T, file string) []mutateSite {
	t.Helper()
	var scan struct {
		Sites []mutateSite `json:"sites"`
	}
	o := cli(t, "mutation", "list", "--json", file)
	if err := json.Unmarshal([]byte(o.stdout), &scan); err != nil {
		t.Fatalf("sites of %s: %v\n%s", file, err, o.stdout)
	}
	return scan.Sites
}

// @ID-MUT-52
func TestEveryMutantAsJSON(t *testing.T) {
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
			Original: s.Original, Replacement: s.Replacement, Outcome: outcomes[s.Function]})
	}

	o := mutateCovered(t, "--json", boardSource)
	got := o.json(t).file(t, boardSource).mutants(t)
	if !slices.Equal(got, want) {
		t.Errorf("mutants:\n%+v\nwant, in site order:\n%+v\nstderr:\n%s", got, want, o.stderr)
	}
	for _, f := range o.rawFiles(t) {
		list, _ := f["mutants"].([]any)
		for _, m := range list {
			if keys := slices.Sorted(maps.Keys(m.(map[string]any))); !slices.Equal(keys, mutantKeys) {
				t.Errorf("mutant keys %q, want %q", keys, mutantKeys)
			}
		}
	}
}

// @ID-MUT-53
func TestATimedOutMutantsOutcomeIsItsOwn(t *testing.T) {
	// Deleting the ! makes Settle sleep an hour where its test expects it
	// to return at once, so the mutant can only time out, on every
	// platform. A sleep rather than a busy loop: Windows kills the test
	// command but not the test binary it started, which then idles rather
	// than spins until its own timeout.
	wait := filepath.FromSlash("src/wait/wait.go")
	boardRepo(t, map[string]string{
		"src/wait/wait.go": `package wait

import "time"

// Settle waits for a board that is not still.
func Settle(still bool) {
	if !still {
		time.Sleep(time.Hour)
	}
}
`,
		"src/wait/wait_test.go": `package wait

import "testing"

func TestSettle(t *testing.T) {
	Settle(true)
}
`,
	})
	if sites := scanned(t, wait); len(sites) != 1 || sites[0].Original != "!" {
		t.Fatalf("sites %+v, want the one ! of Settle", sites)
	}

	// The timeout is then the baseline's duration, and at least 2s.
	o := mutateRun(t, "--json", "--timeout-factor", "1", wait)
	f := o.json(t).file(t, wait)
	got := f.mutants(t)
	if len(got) != 1 || got[0].Outcome != "timeout" {
		t.Fatalf("mutants %+v, want the one mutant timed out\nstderr:\n%s", got, o.stderr)
	}
	if f.Killed != 1 || o.code != 0 {
		t.Errorf("killed %d, exit %d, want the timed-out mutant counted as killed and exit 0", f.Killed, o.code)
	}
}

// @ID-MUT-54
func TestMutantsTakenFromTheSnapshotSaySo(t *testing.T) {
	boardRepo(t, nil)
	if o := mutateRun(t); o.code != 1 {
		t.Fatalf("the first run: exit %d, want 1 for clear's survivor\n%s%s", o.code, o.stdout, o.stderr)
	}
	edit(t, boardSource, "i < 3", "i < 4")

	o := mutateRun(t, "--json", boardSource)
	byFunction := map[string][]mutantJSON{}
	for _, m := range o.json(t).file(t, boardSource).mutants(t) {
		byFunction[m.Function] = append(byFunction[m.Function], m)
	}
	place, clear := byFunction[placeID], byFunction[clearID]
	if len(place) == 0 || len(clear) == 0 {
		t.Fatalf("mutants %+v, want those of %s and %s", byFunction, placeID, clearID)
	}
	for _, m := range place {
		if m.Outcome != "killed" || !m.Reused {
			t.Errorf("place's mutant %+v, want killed and reused: place has not changed", m)
		}
	}
	for _, m := range clear {
		if m.Reused {
			t.Errorf("clear's mutant %+v, want it not reused: clear changed", m)
		}
	}
}

// @ID-MUT-55
func TestWithSinceOnlyTheJudgedFunctionsMutantsAreListed(t *testing.T) {
	boardRepo(t, nil)
	// A first run records clear's survivor, which a run not judging clear
	// keeps in the snapshot but must not list.
	if o := mutateRun(t); o.code != 1 {
		t.Fatalf("the first run: exit %d, want 1 for clear's survivor\n%s%s", o.code, o.stdout, o.stderr)
	}
	changePlace(t)

	o := mutateRun(t, "--since", "base", "--json")
	got := o.json(t).file(t, boardSource).mutants(t)
	if len(got) == 0 {
		t.Fatalf("no mutants listed, want those of %s\n%s", placeID, o.stdout)
	}
	for _, m := range got {
		if m.Function != placeID {
			t.Errorf("mutant %+v listed, want only those of %s, the function judged", m, placeID)
		}
	}
}

// @ID-MUT-56
func TestAFailingBaselineListsNoMutant(t *testing.T) {
	boardRepo(t, map[string]string{
		"src/board_test.go": "package board\n\nimport \"testing\"\n\nfunc TestBroken(t *testing.T) {\n\tt.Fatal(\"broken\")\n}\n",
	})

	o := mutateRun(t, "--json", boardSource)
	files := o.rawFiles(t)
	if len(files) != 1 || files[0]["baseline"] != "failed" {
		t.Fatalf("files %v, want %s with a failed baseline\n%s", files, boardSource, o.stdout)
	}
	if list, ok := files[0]["mutants"].([]any); !ok || len(list) != 0 {
		t.Errorf("mutants %#v, want []\n%s", files[0]["mutants"], o.stdout)
	}
}
