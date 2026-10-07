package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The scenario of "Rule: Results go stale when their tests change" about
// --since when the tests changed, in features/mutate.feature. It uses the
// module of mutate_since_test.go, whose first run kills Board#place's mutant
// and records Board#clear's survivor.

// rawUnit is the entry of function in src/board.go's snapshot as the file
// holds it, key by key.
func rawUnit(t *testing.T, function string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(".metrics", "mutate", "src", "board.go.json"))
	if err != nil {
		t.Fatalf("no snapshot of %s: %v", boardSource, err)
	}
	var snap struct {
		Units []map[string]any `json:"units"`
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatal(err)
	}
	for _, u := range snap.Units {
		if u["namespace"].(string)+"#"+u["name"].(string) == function {
			return u
		}
	}
	return nil
}

// @ID-MUT-108
func TestSinceKeepsTheEntriesItDoesNotJudgeMarkedStaleWhenTheirTestsChanged(t *testing.T) {
	boardRepo(t, nil)
	if o := mutateRun(t); o.code != 1 {
		t.Fatalf("the first run: exit %d, want 1 for clear's survivor\n%s%s", o.code, o.stdout, o.stderr)
	}
	changeBoardTest(t)
	changePlace(t)

	if o := mutateRun(t, "--since", "base"); o.code != 0 {
		t.Fatalf("the --since run: exit %d, want 0: only place is judged\n%s%s", o.code, o.stdout, o.stderr)
	}
	u := boardUnits(t)[clearID]
	if u.Survived != 1 || len(u.Mutants) != 1 || u.Mutants[0].Outcome != "survived" {
		t.Errorf("clear in the snapshot: %+v, want its entry and the survivor it recorded", u)
	}
	if raw := rawUnit(t, clearID); raw == nil || raw["stale"] != true {
		t.Errorf("clear's entry %v, want it marked \"stale\": true", raw)
	}
	if raw := rawUnit(t, placeID); raw == nil || raw["stale"] != nil {
		t.Errorf("place's entry %v, want no mark: the run judged it", raw)
	}

	o := mutationCheck(t, "--json", boardSource)
	m := o.json(t)
	if p := m.problem("mutation.missing"); p != nil {
		t.Errorf("mutation.missing %v, want clear stale, not missing", p)
	}
	p := m.problem("mutation.stale")
	wantProblem(t, p, map[string]any{"file": boardSource, "function": clearID}, o.stdout)
	if msg, _ := p["message"].(string); !strings.Contains(msg, "tests") {
		t.Errorf("message %q, want it to say the tests changed", msg)
	}
	if o.code != 1 {
		t.Errorf("check: exit %d, want 1", o.code)
	}

	o = mutateRun(t, "--json", boardSource)
	reused, listed := reusedOf(t, o, clearID)
	if listed == 0 || reused != 0 {
		t.Errorf("%d of %d mutants of %s reused, want each run again\n%s", reused, listed, clearID, o.stdout)
	}
	if raw := rawUnit(t, clearID); raw == nil || raw["stale"] != nil {
		t.Errorf("clear's entry %v after a run that judged it, want no mark", raw)
	}
}
