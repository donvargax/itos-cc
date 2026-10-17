package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// Tests of "Rule: Re-running a sample of cached results" about the scope of
// the tests that decided each outcome, in features/mutate.feature, over the
// module of mutate_since_test.go, whose own suite and whole suite are a test
// or two each, so --all-tests stays small. ID-MUT-111 to 113 hold their old
// tests whole (T-101); each test left holds an assertion no scenario does.

// recordedScopes is the "scope" of each mutant of src/board.go's snapshot,
// by function, as the file holds it: "" where the key is left out.
func recordedScopes(t *testing.T) map[string][]string {
	t.Helper()
	data, err := os.ReadFile(inWD(t, filepath.Join(".metrics", "mutate", "src", "board.go.json")))
	if err != nil {
		t.Fatalf("no snapshot of %s: %v", boardSource, err)
	}
	var snap struct {
		Units []struct {
			Namespace string           `json:"namespace"`
			Name      string           `json:"name"`
			Mutants   []map[string]any `json:"mutants"`
		} `json:"units"`
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, u := range snap.Units {
		for _, m := range u.Mutants {
			scope, _ := m["scope"].(string)
			out[u.Namespace+"#"+u.Name] = append(out[u.Namespace+"#"+u.Name], scope)
		}
	}
	return out
}

// wantScopes fails t unless every mutant of src/board.go's snapshot records
// scope, "" for none.
func wantScopes(t *testing.T, scope string) {
	t.Helper()
	got := recordedScopes(t)
	if len(got) == 0 {
		t.Fatalf("the snapshot of %s records no mutant", boardSource)
	}
	for function, scopes := range got {
		for _, s := range scopes {
			if s != scope {
				t.Errorf("%s records scope %q, want %q", function, s, scope)
			}
		}
	}
}

// @ID-MUT-109. Kept: an outcome of the file's own tests leaves "scope" out
// of the snapshot, which no scenario asserts; the scenario reads a missing
// scope and "own" alike.
func TestEachRecordedOutcomeKeepsTheScopeOfTheTestsThatDecidedIt(t *testing.T) {
	t.Parallel()
	boardRepo(t, killedTests)
	if o := mutateRun(t, "--all-tests", boardSource); o.code != 0 {
		t.Fatalf("the --all-tests run: exit %d, want every mutant killed\n%s%s", o.code, o.stdout, o.stderr)
	}
	wantScopes(t, "all-tests")

	mutateRun(t, "--mutate-all", "--test-command", passing, boardSource)
	wantScopes(t, passing)

	// "own" is the default, left out of the file as an outcome recorded
	// before scopes is.
	if o := mutateRun(t, "--mutate-all", boardSource); o.code != 0 {
		t.Fatalf("the run with neither: exit %d, want every mutant killed\n%s%s", o.code, o.stdout, o.stderr)
	}
	wantScopes(t, "")
}

// @ID-MUT-110. Kept: a reused outcome's scope in mutation run --json, which
// no scenario asserts; the scenario reads the snapshot's.
func TestAReusedOutcomeKeepsItsScope(t *testing.T) {
	t.Parallel()
	boardRepo(t, killedTests)
	if o := mutateRun(t, "--all-tests", boardSource); o.code != 0 {
		t.Fatalf("the --all-tests run: exit %d, want every mutant killed\n%s%s", o.code, o.stdout, o.stderr)
	}

	o := mutateRun(t, "--json", boardSource)
	var listed int
	for _, m := range o.json(t).file(t, boardSource).mutants(t) {
		if m.Function != placeID {
			continue
		}
		listed++
		if !m.Reused || m.Scope != "all-tests" {
			t.Errorf("place's mutant %+v, want it reused with scope \"all-tests\"", m)
		}
	}
	if listed == 0 {
		t.Fatalf("no mutant of %s listed\n%s", placeID, o.stdout)
	}
	if got := recordedScopes(t)[placeID]; len(got) == 0 || slices.ContainsFunc(got, func(s string) bool { return s != "all-tests" }) {
		t.Errorf("place's mutants record scopes %q, want each \"all-tests\"", got)
	}
}
