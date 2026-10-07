package mutate

import (
	"reflect"
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/lang"
)

func TestBuildCountsEveryUnit(t *testing.T) {
	f := parse(t, "x.py", "def a(x):\n    return x > 0\n\ndef b(x):\n    return x < 1\n\ndef c(x):\n    return not x\n")
	sites := Sites(f)
	outcomes := make([]string, len(sites))
	for i := range sites {
		outcomes[i] = []string{Killed, Survived, Uncovered, Timeout}[i%4]
	}
	snap := build(f, "x.py", sites, outcomes)
	total := 0
	for _, u := range snap.Units {
		total += u.Killed + u.Survived + u.Uncovered
		if u.Sites != len(u.Mutants) {
			t.Errorf("%s: %d sites but %d mutants", u.Name, u.Sites, len(u.Mutants))
		}
	}
	if total != len(sites) {
		t.Errorf("counted %d outcomes for %d sites", total, len(sites))
	}
}

func TestRememberedKeepsOnlyKilledMutantsOfUnchangedUnits(t *testing.T) {
	before := parse(t, "x.py", "def a(x):\n    return x > 0\n\ndef b(x):\n    return x < 1\n")
	sites := Sites(before)
	snap := build(before, "x.py", sites, []string{Killed, Killed, Survived, Killed})

	// b changes; a only moves down.
	after := parse(t, "x.py", "import os\n\ndef a(x):\n    return x > 0\n\ndef b(x):\n    return x <= 1\n")
	prev := remembered(&snap, after)
	var kept []bool
	for _, s := range Sites(after) {
		_, ok := prev.kept(after, s)
		kept = append(kept, ok)
	}
	want := []bool{true, true, false, false}
	for i := range want {
		if kept[i] != want[i] {
			t.Fatalf("kept %v, want %v", kept, want)
		}
	}
}

func TestSurvivorsAreAlwaysRetried(t *testing.T) {
	f := parse(t, "x.py", "def a(x):\n    return x > 0\n")
	snap := build(f, "x.py", Sites(f), []string{Survived, Killed})
	prev := remembered(&snap, f)
	if _, ok := prev.kept(f, Sites(f)[0]); ok {
		t.Error("a survivor was kept instead of retried")
	}
}

func TestFunctionsNotJudgedKeepTheirRecord(t *testing.T) {
	before := parse(t, "x.py", "def a(x):\n    return x > 0\n\ndef b(x):\n    return x < 1\n\ndef c(x):\n    return x == 1\n")
	snap := build(before, "x.py", Sites(before), byUnit(before, map[string]string{"a": Killed, "b": Survived, "c": Killed}))
	recorded := map[string]UnitResult{}
	for _, u := range snap.Units {
		recorded[u.Name] = u
	}
	snap.Units = slices.DeleteFunc(snap.Units, func(u UnitResult) bool { return u.Name == "c" })

	// a is judged; b moves down unchanged; c changes, but the snapshot has
	// no record of it; d is new and has none either.
	after := parse(t, "x.py", "import os\n\ndef a(x):\n    return x > 0\n\ndef b(x):\n    return x < 1\n\ndef c(x):\n    return x != 1\n\ndef d(x):\n    return x\n")
	built := build(after, "x.py", Sites(after), byUnit(after, map[string]string{"a": Killed}))
	for _, u := range built.Units {
		if u.Sites != len(u.Mutants) {
			t.Errorf("%s: %d sites but %d mutants: a skipped site was counted", u.Name, u.Sites, len(u.Mutants))
		}
	}
	units := keepUnjudged(built.Units, map[string]bool{after.Units[0].Namespace + "#a": true}, &snap)
	var names []string
	for _, u := range units {
		names = append(names, u.Name)
	}
	if !slices.Equal(names, []string{"a", "b"}) {
		t.Fatalf("units %q, want a, judged, and b, recorded; c and d have no record", names)
	}
	if b := units[1]; b.StartLine != 6 || b.Hash != recorded["b"].Hash {
		t.Errorf("b: %+v, want its record at its current lines", b)
	}

	// b changes too: its record is kept as it was, old hash and all.
	changed := parse(t, "x.py", "def a(x):\n    return x > 0\n\ndef b(x):\n    return x <= 1\n")
	built = build(changed, "x.py", Sites(changed), byUnit(changed, map[string]string{"a": Killed}))
	units = keepUnjudged(built.Units, map[string]bool{changed.Units[0].Namespace + "#a": true}, &snap)
	if len(units) != 2 || !reflect.DeepEqual(units[1], recorded["b"]) {
		t.Errorf("b: %+v, want it as recorded: %+v", units[1], recorded["b"])
	}
}

// byUnit gives each site of f the outcome of its function's name, and
// skipped to those outcome does not name.
func byUnit(f *lang.File, outcome map[string]string) []string {
	var out []string
	for _, s := range Sites(f) {
		o := outcome[f.Units[s.Unit].Name]
		if o == "" {
			o = skipped
		}
		out = append(out, o)
	}
	return out
}
