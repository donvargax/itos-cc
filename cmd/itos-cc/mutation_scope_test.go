package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The scenarios of "Rule: Re-running a sample of cached results" about the
// scope of the tests that decided each outcome, in features/mutate.feature.
// They use the module of mutate_since_test.go, whose own suite and whole
// suite are a test or two each, so --all-tests stays small.

// e2eFiles make Board#place's mutant one that only a test of another
// package kills: the package's own test tests nothing, and e2e/e2e_test.go
// calls place through Place, which has no mutation site.
var e2eFiles = map[string]string{
	"src/board_test.go": "package board\n\nimport \"testing\"\n\nfunc TestBoard(t *testing.T) {}\n",
	"src/export.go":     "package board\n\n// Place places on a new board.\nfunc Place(i int) bool {\n\tvar b Board\n\treturn b.place(i)\n}\n",
	"e2e/e2e_test.go": `package e2e

import (
	"testing"

	board "example.com/m/src"
)

func TestPlace(t *testing.T) {
	if board.Place(5) || !board.Place(6) {
		t.Fatal("place")
	}
}
`,
}

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

// baselines is the command of each baseline a run says on stderr it ran.
func baselines(stderr string) []string {
	var out []string
	for _, l := range strings.Split(stderr, "\n") {
		if rest, ok := strings.CutPrefix(l, "itos-cc: baseline "); ok {
			if _, command, ok := strings.Cut(rest, "$ "); ok {
				out = append(out, command)
			}
		}
	}
	return out
}

// stripScopes rewrites src/board.go's snapshot without the "scope" of any
// mutant, as one written before outcomes recorded it.
func stripScopes(t *testing.T) {
	t.Helper()
	name := inWD(t, filepath.Join(".metrics", "mutate", "src", "board.go.json"))
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var snap map[string]any
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatal(err)
	}
	for _, u := range snap["units"].([]any) {
		for _, m := range u.(map[string]any)["mutants"].([]any) {
			delete(m.(map[string]any), "scope")
		}
	}
	out, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, name, string(out)+"\n")
}

// @ID-MUT-109
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

// @ID-MUT-110
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

// @ID-MUT-111
func TestMutationSampleRerunsEachMutantWithItsRecordedScope(t *testing.T) {
	t.Parallel()
	boardRepo(t, e2eFiles)
	if o := mutateRun(t, "--all-tests", boardSource); o.code != 1 {
		t.Fatalf("the --all-tests run: exit %d, want 1 for clear's survivor\n%s%s", o.code, o.stdout, o.stderr)
	}
	if got := recorded(t, placeID, ">"); got != "killed" {
		t.Fatalf("place's mutant recorded %s, want killed by e2e/e2e_test.go", got)
	}

	o := mutationSample(t, "--json", "--count", "100", boardSource)
	if got := baselines(o.stderr); !slices.Equal(got, []string{"go test -count=1 -failfast ./..."}) {
		t.Errorf("baselines %q, want the whole suite's alone\n%s", got, o.stderr)
	}
	var place bool
	for _, m := range o.sampled(t) {
		if m.Function == placeID {
			place = true
			if m.Outcome != "killed" {
				t.Errorf("place's mutant %+v, want it killed by the whole suite", m)
			}
		}
	}
	if !place {
		t.Errorf("place's mutant was not sampled\n%s", o.stdout)
	}
	if p := o.json(t).problem("mutation.mismatch"); p != nil {
		t.Errorf("mismatch %v, want none", p)
	}
	if o.code != 0 {
		t.Errorf("exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}
}

// @ID-MUT-112
func TestAScopeGivenToMutationSampleOverridesTheRecordedOne(t *testing.T) {
	t.Parallel()
	boardRepo(t, killedTests)
	if o := mutateRun(t, "--all-tests", boardSource); o.code != 0 {
		t.Fatalf("the --all-tests run: exit %d, want every mutant killed\n%s%s", o.code, o.stdout, o.stderr)
	}

	o := mutationSample(t, "--json", "--test-command", passing, boardSource)
	if got := baselines(o.stderr); !slices.Equal(got, []string{passing}) {
		t.Errorf("baselines %q, want %q alone\n%s", got, passing, o.stderr)
	}
	if len(o.sampled(t)) == 0 {
		t.Errorf("nothing sampled\n%s", o.stdout)
	}
}

// @ID-MUT-113
func TestAnOutcomeWithNoRecordedScopeWasDecidedByTheFilesOwnTests(t *testing.T) {
	t.Parallel()
	boardRepo(t, killedTests)
	if o := mutateRun(t, "--all-tests", boardSource); o.code != 0 {
		t.Fatalf("the --all-tests run: exit %d, want every mutant killed\n%s%s", o.code, o.stdout, o.stderr)
	}
	stripScopes(t)

	o := mutationSample(t, "--json", boardSource)
	if got := baselines(o.stderr); !slices.Equal(got, []string{"go test -count=1 -failfast ./src"}) {
		t.Errorf("baselines %q, want the file's own tests' alone\n%s", got, o.stderr)
	}
	if len(o.sampled(t)) == 0 {
		t.Errorf("nothing sampled\n%s", o.stdout)
	}
}
