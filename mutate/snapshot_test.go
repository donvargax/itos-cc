package mutate

import (
	"os"
	"path/filepath"
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
	snap := build(f, "x.py", nil, sites, outcomes)
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
	snap := build(before, "x.py", nil, sites, []string{Killed, Killed, Survived, Killed})

	// b changes; a only moves down.
	after := parse(t, "x.py", "import os\n\ndef a(x):\n    return x > 0\n\ndef b(x):\n    return x <= 1\n")
	prev := remembered(&snap, after)
	var kept []bool
	for _, s := range Sites(after) {
		_, ok := prev.kept(after, s, false)
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
	snap := build(f, "x.py", nil, Sites(f), []string{Survived, Killed})
	prev := remembered(&snap, f)
	if _, ok := prev.kept(f, Sites(f)[0], false); ok {
		t.Error("a survivor was kept instead of retried")
	}
	// An excepted survivor is the one exception: it is reused as a kill is.
	if outcome, ok := prev.kept(f, Sites(f)[0], true); !ok || outcome != Survived {
		t.Errorf("an excepted survivor: kept %v as %q, want it reused as survived", ok, outcome)
	}
}

func TestFunctionsNotJudgedKeepTheirRecord(t *testing.T) {
	before := parse(t, "x.py", "def a(x):\n    return x > 0\n\ndef b(x):\n    return x < 1\n\ndef c(x):\n    return x == 1\n")
	snap := build(before, "x.py", nil, Sites(before), byUnit(before, map[string]string{"a": Killed, "b": Survived, "c": Killed}))
	recorded := map[string]UnitResult{}
	for _, u := range snap.Units {
		recorded[u.Name] = u
	}
	snap.Units = slices.DeleteFunc(snap.Units, func(u UnitResult) bool { return u.Name == "c" })

	// a is judged; b moves down unchanged; c changes, but the snapshot has
	// no record of it; d is new and has none either.
	after := parse(t, "x.py", "import os\n\ndef a(x):\n    return x > 0\n\ndef b(x):\n    return x < 1\n\ndef c(x):\n    return x != 1\n\ndef d(x):\n    return x\n")
	built := build(after, "x.py", nil, Sites(after), byUnit(after, map[string]string{"a": Killed}))
	for _, u := range built.Units {
		if u.Sites != len(u.Mutants) {
			t.Errorf("%s: %d sites but %d mutants: a skipped site was counted", u.Name, u.Sites, len(u.Mutants))
		}
	}
	units := keepUnjudged(built.Units, map[int]bool{0: true}, &snap, true)
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
	built = build(changed, "x.py", nil, Sites(changed), byUnit(changed, map[string]string{"a": Killed}))
	units = keepUnjudged(built.Units, map[int]bool{0: true}, &snap, true)
	if len(units) != 2 || !reflect.DeepEqual(units[1], recorded["b"]) {
		t.Errorf("b: %+v, want it as recorded: %+v", units[1], recorded["b"])
	}
}

// @ID-MUT-162
func TestUnjudgedFunctionsKeepTheirIndependentCoverageEvidence(t *testing.T) {
	f := parse(t, "main.go", "package main\n\nfunc a(x int) int { return x > 0 }\nfunc b(x int) int { return x < 1 }\n")
	defer f.Close()
	sites := Sites(f)
	previous := build(f, "main.go", nil, sites, byUnit(f, map[string]string{"a": Killed, "b": Killed}))
	old := &GoCoverageEvidence{Version: goCoverageEvidenceVersion, File: "main.go", Function: "main#b", Hash: "old-hash", Producer: "old producer", Inputs: map[string]string{"main.go": "old"}, Complete: true,
		Blocks: []GoCoverageBlock{{Span: "4.20,4.30", Line: 4, Column: 20, Weight: 1, Covered: true}}}
	previous.Units[1].Coverage = old
	built := build(f, "main.go", nil, sites, byUnit(f, map[string]string{"a": Killed, "b": Killed}))
	built.Units[1].Coverage = &GoCoverageEvidence{Version: goCoverageEvidenceVersion, File: "main.go", Function: "main#b", Hash: "new-hash", Producer: "new producer", Inputs: map[string]string{"main.go": "new"}, Complete: true}
	units := keepUnjudged(built.Units, map[int]bool{0: true}, &previous, true)
	if len(units) != 2 || !reflect.DeepEqual(units[1].Coverage, old) {
		t.Errorf("unjudged coverage evidence %+v, want preserved original %+v", units[1].Coverage, old)
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

func TestResultsHoldOnlyWhileTheirTestsDo(t *testing.T) {
	f := parse(t, "x.py", "def a(x):\n    return x > 0\n")
	tests := map[string]string{"test_x.py": "1", "test_y.py": "2"}
	snap := build(f, "x.py", tests, Sites(f), []string{Killed, Killed})
	if usable(&snap, map[string]string{"test_x.py": "1", "test_y.py": "2"}) == nil {
		t.Error("the same tests: want the snapshot usable")
	}
	now := map[string]string{"test_x.py": "1b", "test_z.py": "3"}
	if usable(&snap, now) != nil {
		t.Error("other tests: want the snapshot unusable")
	}
	want := &TestChange{Added: []string{"test_z.py"}, Removed: []string{"test_y.py"}, Changed: []string{"test_x.py"}}
	if got := compareTests(snap.Tests, now); !reflect.DeepEqual(got, want) {
		t.Errorf("changes %+v, want %+v", got, want)
	}

	snap.Tests = nil
	if usable(&snap, map[string]string{}) != nil {
		t.Error("a snapshot that records no tests: want it unusable, even for a file no test imports")
	}
	if c := compareTests(nil, map[string]string{}); !c.Unrecorded {
		t.Errorf("changes %+v, want unrecorded", c)
	}
	if empty := build(f, "x.py", nil, Sites(f), []string{Killed, Killed}); empty.Tests == nil {
		t.Error("a file no test imports records nil tests, want an empty map: nil reads as written before tests were recorded")
	}
}

func TestAnEntryKeptUnderOtherTestsIsMarkedStaleAndNeverReused(t *testing.T) {
	f := parse(t, "x.py", "def a(x):\n    return x > 0\n\ndef b(x):\n    return x < 1\n")
	snap := build(f, "x.py", nil, Sites(f), byUnit(f, map[string]string{"a": Killed, "b": Killed}))
	judged := map[int]bool{0: true}
	built := build(f, "x.py", nil, Sites(f), byUnit(f, map[string]string{"a": Killed}))

	// The tests changed: b, not judged, keeps its entry, marked.
	units := keepUnjudged(built.Units, judged, &snap, false)
	if len(units) != 2 || units[0].Stale || !units[1].Stale || units[1].Killed == 0 {
		t.Fatalf("units %+v, want a as judged and b as recorded, marked stale", units)
	}
	// A marked entry is not reused, and stays marked while not judged.
	snap.Units = units
	if prev := remembered(&snap, f); len(prev[1]) != 0 || len(prev[0]) == 0 {
		t.Errorf("remembered %v, want a's kill and nothing of b", prev)
	}
	if units := keepUnjudged(built.Units, judged, &snap, true); len(units) != 2 || !units[1].Stale {
		t.Errorf("units %+v, want b still marked stale", units)
	}
	// The tests held: b is kept as it is, unmarked.
	snap = build(f, "x.py", nil, Sites(f), byUnit(f, map[string]string{"a": Killed, "b": Killed}))
	if units := keepUnjudged(built.Units, judged, &snap, true); len(units) != 2 || units[1].Stale {
		t.Errorf("units %+v, want b unmarked: its tests did not change", units)
	}
}

func TestEachOutcomeKeepsTheScopeThatDecidedIt(t *testing.T) {
	f := parse(t, "x.py", "def a(x):\n    return x > 0\n")
	sites := Sites(f)
	scopes := []string{ScopeAllTests, ScopeOwn}
	snap := buildScoped(f, "x.py", nil, sites, []string{Killed, Killed}, scopes, nil)
	got := map[string]string{}
	for _, m := range snap.Units[0].Mutants {
		got[m.key()] = m.Scope
	}
	if got[sites[0].Key()] != ScopeAllTests || got[sites[1].Key()] != "" {
		t.Errorf("scopes %v, want %q, and none for %q", got, ScopeAllTests, ScopeOwn)
	}
	_, prev := rememberedWithScopes(&snap, f)
	if prev[0][sites[0].Key()].scope != ScopeAllTests || prev[0][sites[1].Key()].scope != ScopeOwn {
		t.Errorf("remembered scopes %v, want %q and %q: none recorded reads as %q", prev, ScopeAllTests, ScopeOwn, ScopeOwn)
	}
	for _, c := range []struct {
		shell string
		all   bool
		want  string
	}{{"", false, ScopeOwn}, {"", true, ScopeAllTests}, {"make test", false, "make test"}, {"make test", true, "make test"}} {
		if got := RunScope(c.shell, c.all); got != c.want {
			t.Errorf("RunScope(%q, %v) = %q, want %q", c.shell, c.all, got, c.want)
		}
	}
}

func TestFunctionsSharingANameMatchTheirEntriesByHash(t *testing.T) {
	cases := []struct {
		now, was []string
		want     []int
	}{
		{[]string{"a", "b"}, []string{"a", "b"}, []int{0, 1}},
		{[]string{"b", "a"}, []string{"a", "b"}, []int{1, 0}},      // reordered
		{[]string{"a", "c"}, []string{"a", "b"}, []int{0, 1}},      // the second changed
		{[]string{"c", "a"}, []string{"a", "b"}, []int{1, 0}},      // the first changed, then moved
		{[]string{"a", "a"}, []string{"a", "a"}, []int{0, 1}},      // one hash: file order
		{[]string{"a", "b", "c"}, []string{"b"}, []int{-1, 0, -1}}, // more functions than entries
	}
	for _, c := range cases {
		if got := PairByHash(c.now, c.was); !slices.Equal(got, c.want) {
			t.Errorf("PairByHash(%q, %q) = %v, want %v", c.now, c.was, got, c.want)
		}
	}

	// Two Python functions named a: run reuses each one's outcomes, and a
	// run that judges neither keeps one entry for each.
	before := parse(t, "x.py", "def a(x):\n    return x > 0\n\ndef a(x):\n    return x < 1\n")
	snap := build(before, "x.py", nil, Sites(before), []string{Killed, Killed, Survived, Killed})
	swapped := parse(t, "x.py", "def a(x):\n    return x < 1\n\ndef a(x):\n    return x > 0\n")
	prev := remembered(&snap, swapped)
	for _, s := range Sites(swapped) {
		if _, ok := prev.kept(swapped, s, false); !ok && s.Original != "<" {
			t.Errorf("site %+v not reused, want each function's own kills reused", s)
		}
	}
	var outcomes []string
	for _, s := range Sites(swapped) {
		o := prev[s.Unit][s.Key()]
		if o == "" {
			o = skipped
		}
		outcomes = append(outcomes, o)
	}
	built := build(swapped, "x.py", nil, Sites(swapped), outcomes)
	units := keepUnjudged(built.Units, map[int]bool{}, &snap, true)
	if len(units) != 2 || units[0].Hash == units[1].Hash || units[0].Survived != 1 || units[1].Survived != 0 {
		t.Errorf("units %+v, want each function's own entry, the survivor with the first", units)
	}
}

func TestAnOwnOutcomeOfAWholeSuiteScriptReadsAsTheWholeSuites(t *testing.T) {
	root := t.TempDir()
	for name, text := range map[string]string{
		"package.json": `{"scripts": {"test": "node --test"}}`,
		"src/a.ts":     "export function a(i: number): boolean {\n  return i === 3;\n}\n",
		".metrics/mutate/src/a.ts.json": `{"version": 1, "file": "src/a.ts", "language": "typescript", "units": [{"name": "a", "mutants": [` +
			`{"outcome": "killed"}, {"outcome": "killed", "scope": "make check"}, {"outcome": "survived", "scope": "listed", "tests": ["t"]}]}]}`,
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	snap, err := LoadSnapshotOf(root, "src/a.ts")
	if err != nil || snap == nil {
		t.Fatalf("snapshot: %v, %v", snap, err)
	}
	var scopes []string
	for _, m := range snap.Units[0].Mutants {
		scopes = append(scopes, m.TestScope())
	}
	if want := []string{ScopeAllTests, "make check", ScopeListed}; !slices.Equal(scopes, want) {
		t.Errorf("scopes %q, want %q: the own outcome the whole script decided reads as the whole suite's", scopes, want)
	}
	if !HasBroadOutcome(snap.Units) || snap.Units[0].Mutants[0].SuiteEvidence != nil {
		t.Errorf("the own outcome recorded without evidence: %+v, want a broad outcome with none, stale once", snap.Units[0].Mutants[0])
	}
}

func TestACoverageBlockSpansTheLinesOfItsSpan(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		block      CoverageBlock
		start, end int
	}{
		{CoverageBlock{Span: "5.2,6.10", Line: 5}, 5, 6},
		// It ends before line 8's first character: line 8 is not its.
		{CoverageBlock{Span: "7.3,8.1", Line: 7}, 7, 7},
		{CoverageBlock{Span: "9.2,9.10", Line: 9}, 9, 9},
		// An executable line of a line-precision language.
		{CoverageBlock{Span: "4", Line: 4}, 4, 4},
	} {
		if start, end := c.block.Lines(); start != c.start || end != c.end {
			t.Errorf("%q: lines %d to %d, want %d to %d", c.block.Span, start, end, c.start, c.end)
		}
	}
	var whole LineScope
	only := LineScope(func(path string, start, end int) bool { return start <= 6 && end >= 6 })
	if !whole.JudgesBlock("x.go", CoverageBlock{Span: "7.3,8.1", Line: 7}) || only.JudgesBlock("x.go", CoverageBlock{Span: "7.3,8.1", Line: 7}) ||
		!only.JudgesBlock("x.go", CoverageBlock{Span: "5.2,6.10", Line: 5}) {
		t.Error("a nil scope judges every block, and a scope only the blocks overlapping its lines")
	}
}
