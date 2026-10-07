package graph

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/mutate"
	"github.com/donvargax/itos-cc/project"
)

// module is a Go module, example.com/p, in a directory of its own.
type module struct {
	t    *testing.T
	root string // absolute, its symbolic links resolved, as os.Getwd names it
}

func newModule(t *testing.T) module {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := module{t, root}
	m.write("go.mod", "module example.com/p\n\ngo 1.22\n")
	return m
}

func (m module) write(rel, text string) {
	m.t.Helper()
	p := filepath.Join(m.root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		m.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		m.t.Fatal(err)
	}
}

// recorded is each function of the source file rel as a run that killed
// every mutant records it: its hash, and a mutant for each of its sites.
func (m module) recorded(rel string) []mutate.UnitResult {
	m.t.Helper()
	f, err := lang.ParseFile(filepath.Join(m.root, rel))
	if err != nil {
		m.t.Fatal(err)
	}
	defer f.Close()
	sites := mutate.Sites(f)
	var out []mutate.UnitResult
	for i, u := range f.Units {
		e := mutate.UnitResult{Namespace: u.Namespace, Name: u.Name, Hash: mutate.UnitHash(f, u),
			StartLine: u.StartLine, EndLine: u.EndLine, Mutants: []mutate.Mutant{}}
		for _, s := range sites {
			if s.Unit == i {
				e.Mutants = append(e.Mutants, mutate.Mutant{Line: s.Line, Column: s.Column, Offset: s.Offset,
					Original: s.Original, Replacement: s.Replacement, Outcome: mutate.Killed})
			}
		}
		e.Killed, e.Sites = len(e.Mutants), len(e.Mutants)
		out = append(out, e)
	}
	return out
}

// tests is what a snapshot records of the test files rel, as they are now.
func (m module) tests(rel ...string) map[string]string {
	m.t.Helper()
	var paths []string
	for _, r := range rel {
		paths = append(paths, filepath.Join(m.root, r))
	}
	h, err := mutate.TestHashesUnder(m.root, paths)
	if err != nil {
		m.t.Fatal(err)
	}
	return h
}

// snapshot writes the snapshot of the source file rel, recording tests and
// units.
func (m module) snapshot(rel string, tests map[string]string, units []mutate.UnitResult) {
	m.t.Helper()
	data, err := json.Marshal(mutate.Snapshot{Version: 1, File: rel, Language: "go", Tests: tests, Units: units})
	if err != nil {
		m.t.Fatal(err)
	}
	m.write(filepath.Join(".metrics", "mutate", rel+".json"), string(data))
}

// build builds the module's graph afresh.
func (m module) build() *Graph {
	m.t.Helper()
	b, err := NewBuilder([]string{m.root})
	if err != nil {
		m.t.Fatal(err)
	}
	g, _, err := b.Build()
	if err != nil {
		m.t.Fatal(err)
	}
	return g
}

// unit is the node holding the function named name, with its unit.
func unit(t *testing.T, g *Graph, name string) (*Node, Unit) {
	t.Helper()
	for _, n := range g.Nodes {
		for _, u := range n.Units {
			if u.Name == name {
				return n, u
			}
		}
	}
	t.Fatalf("no node holds %s", name)
	return nil, Unit{}
}

// posModule is the node holding Pos, with Pos's unit.
func posModule(t *testing.T, g *Graph) (*Node, Unit) {
	t.Helper()
	return unit(t, g, "Pos")
}

const posTest = "package p\n\nimport \"testing\"\n\nfunc TestPos(t *testing.T) {\n\tif !Pos(1) {\n\t\tt.Fatal(\"Pos\")\n\t}\n}\n"

// posModuleRecorded is a module whose a.go holds Pos and a_test.go tests
// it, with the snapshot a run that killed every mutant of Pos writes, less
// what edit takes from Pos's entry.
func posModuleRecorded(t *testing.T, edit func(*mutate.UnitResult)) module {
	m := newModule(t)
	m.write("a.go", "package p\n\nfunc Pos(i int) bool { return i > 0 }\n")
	m.write("a_test.go", posTest)
	units := m.recorded("a.go")
	edit(&units[0])
	m.snapshot("a.go", m.tests("a_test.go"), units)
	return m
}

// @ID-GRAPH-34
func TestMutationResultsWhoseTestsChangedAreStale(t *testing.T) {
	// The snapshot records Pos as it is and a_test.go as it was written.
	m := posModuleRecorded(t, func(*mutate.UnitResult) {})

	b, err := NewBuilder([]string{m.root})
	if err != nil {
		t.Fatal(err)
	}
	g, _, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	if _, u := posModule(t, g); !u.Mutated || u.Stale {
		t.Fatalf("Pos before its test changed: %+v, want mutated and not stale", u)
	}

	m.write("a_test.go", posTest+"\nfunc TestNegative(t *testing.T) {\n\tif Pos(-1) {\n\t\tt.Fatal(\"Pos\")\n\t}\n}\n")

	// A graph built afresh, and the one serve rebuilds as files change.
	if _, u := posModule(t, m.build()); !u.Stale {
		t.Errorf("Pos in a new graph after a_test.go changed: %+v, want stale", u)
	}
	g, changed, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Errorf("the graph did not change when a_test.go did")
	}
	n, u := posModule(t, g)
	if !u.Stale {
		t.Errorf("Pos after a_test.go changed: %+v, want stale", u)
	}
	if n.Metrics == nil || n.Metrics.Stale != 1 {
		t.Errorf("module %s metrics %+v, want one stale function", n.ID, n.Metrics)
	}
}

// @ID-GRAPH-35
func TestTheGraphAndMutationCheckAgreeOnFunctionsSharingAName(t *testing.T) {
	m := newModule(t)
	first, second := "func init() {\n\ta = 1 + 1\n}\n", "func init() {\n\tb = 3 - 1\n}\n"
	source := func(units ...string) string { return "package p\n\nvar a, b int\n\n" + strings.Join(units, "\n") }
	m.write("a.go", source(first, second))

	// The snapshot records both init functions as they are, in file order.
	m.snapshot("a.go", map[string]string{}, m.recorded("a.go"))

	stale := func() []bool {
		t.Helper()
		var out []bool
		for _, n := range m.build().Nodes {
			for _, u := range n.Units {
				if u.Name == "init" {
					if !u.Mutated {
						t.Errorf("init on line %d has no results, want each matched to its entry", u.Line)
					}
					out = append(out, u.Stale)
				}
			}
		}
		return out
	}
	if got := stale(); !slices.Equal(got, []bool{false, false}) {
		t.Fatalf("init functions stale %v as recorded, want neither", got)
	}

	m.write("a.go", source(first, strings.Replace(second, "3 - 1", "4 - 2", 1)))
	if got := stale(); !slices.Equal(got, []bool{false, true}) {
		t.Errorf("init functions stale %v with the second edited, want only the second", got)
	}

	m.write("a.go", source(second, first))
	if got := stale(); !slices.Equal(got, []bool{false, false}) {
		t.Errorf("init functions stale %v swapped, unchanged, want neither", got)
	}
}

// @ID-GRAPH-36
func TestAFunctionWhoseEntryLacksOneOfItsSitesIsStale(t *testing.T) {
	m := posModuleRecorded(t, func(e *mutate.UnitResult) {
		if len(e.Mutants) < 2 {
			t.Fatalf("Pos has %d sites, want at least 2", len(e.Mutants))
		}
		e.Mutants = e.Mutants[1:]
		e.Killed, e.Sites = len(e.Mutants), len(e.Mutants)
	})
	n, u := posModule(t, m.build())
	if !u.Mutated {
		t.Fatalf("Pos: %+v, want its results", u)
	}
	if !u.Stale {
		t.Errorf("Pos whose entry lacks one of its sites: %+v, want stale", u)
	}
	if n.Metrics == nil || n.Metrics.Stale != 1 {
		t.Errorf("module %s metrics %+v, want one stale function", n.ID, n.Metrics)
	}
}

// @ID-GRAPH-37
func TestAnEntryKeptMarkedStaleIsStale(t *testing.T) {
	// A run with --since that did not judge Pos kept its entry, marked, and
	// recorded the tests as they are.
	m := posModuleRecorded(t, func(e *mutate.UnitResult) { e.Stale = true })
	if _, u := posModule(t, m.build()); !u.Mutated || !u.Stale {
		t.Errorf("Pos whose entry is marked stale: %+v, want its results, stale", u)
	}
}

// @ID-GRAPH-38
func TestTheGraphAndMutationCheckAgreeOnEveryFunction(t *testing.T) {
	m := newModule(t)
	fn := func(name, body string) string { return "func " + name + "(i int) bool { return " + body + " }\n" }
	m.write("a.go", "package p\n\n"+fn("Fresh", "i > 0")+fn("Changed", "i > 1")+fn("Unrecorded", "i < 2 && i != 7")+
		fn("Marked", "i >= 3")+fn("Missing", "i <= 4"))
	m.write("b.go", "package p\n\n"+fn("TestsChanged", "i > 5"))
	m.write("c.go", "package p\n\n"+fn("TestsUnrecorded", "i > 6"))
	m.write("d.go", "package p\n\n"+fn("NoSnapshot", "i > 8"))
	m.write("a_test.go", "package p\n\nimport \"testing\"\n\nfunc TestFresh(t *testing.T) {\n\tif !Fresh(1) {\n\t\tt.Fatal(\"Fresh\")\n\t}\n}\n")
	tests := m.tests("a_test.go")

	a := m.recorded("a.go")
	a[1].Hash = "changed since"
	a[2].Mutants = a[2].Mutants[1:]
	a[3].Stale = true
	m.snapshot("a.go", tests, a[:4])
	m.snapshot("b.go", map[string]string{"a_test.go": "changed since"}, m.recorded("b.go"))
	m.snapshot("c.go", nil, m.recorded("c.go"))

	g := m.build()

	t.Chdir(m.root)
	files, err := project.Discover([]string{m.root})
	if err != nil {
		t.Fatal(err)
	}
	importing, err := TestsImporting(m.root, files)
	if err != nil {
		t.Fatal(err)
	}
	checks, err := mutate.Check(files.Sources, nil, func(path string) []string { return importing[path] }, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	state := map[string]string{}
	for _, c := range checks {
		for _, f := range c.Functions {
			state[f.Function[strings.Index(f.Function, "#")+1:]] = f.State
		}
	}
	// Each reason check knows, as check calls it.
	want := map[string]string{"Fresh": mutate.Fresh, "Changed": mutate.Stale, "Unrecorded": mutate.Stale, "Marked": mutate.Stale,
		"Missing": mutate.Missing, "TestsChanged": mutate.Stale, "TestsUnrecorded": mutate.Stale, "NoSnapshot": mutate.Missing}
	for name, w := range want {
		if state[name] != w {
			t.Fatalf("mutation check calls %s %q, want %q: %v", name, state[name], w, state)
		}
	}

	for name, s := range state {
		_, u := unit(t, g, name)
		if u.Stale != (s == mutate.Stale) {
			t.Errorf("%s: graph stale=%v, check calls it %s", name, u.Stale, s)
		}
		if u.Mutated != (s != mutate.Missing) {
			t.Errorf("%s: graph has results=%v, check calls it %s", name, u.Mutated, s)
		}
	}
}
