package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/graph"
)

// The scenarios of "Rule: A kill made by listed tests goes stale when its
// tests change" in features/mutate.feature. They use the module of
// mutate_listed_test.go, with what main writes to stderr left out, so no
// mutant survives, and a list that names a file for each test: ID-A-01 is
// defined in c.feature, ID-A-02 in a.feature, and ID-A-03, which the harness
// runs nothing for and so covers nothing, in b.feature. itos-cc.yaml names
// features/*_test.go as support files. ID-A-02 alone kills the mutants of
// greet's line 17; the own test kills half's; twice's is uncovered.

const (
	greetID = "example.com/greet#greet"
	mainID  = "example.com/greet#main"
	halfID  = "example.com/greet#half"
)

// freshRepo is the module with a first run of mutation run recorded.
func freshRepo(t *testing.T) {
	t.Helper()
	listedRepo(t, map[string][2]string{
		"main.go":          {"fmt.Fprintln(os.Stderr, n*2, half(n))", "fmt.Fprintln(os.Stderr, half(n))"},
		"testdata/list.go": {`"ID-A-01\ta.feature\nID-A-02\n"`, `"ID-A-01\tc.feature\nID-A-02\ta.feature\nID-A-03\tb.feature\n"`},
		"itos-cc.yaml":     {"      sep: \",\"\n", "      sep: \",\"\n    support: [\"features/*_test.go\"]\n"},
	})
	for name, text := range map[string]string{
		"a.feature":              "Feature: a\n",
		"b.feature":              "Feature: b\n",
		"c.feature":              "Feature: c\n",
		"features/steps_test.go": "package features\n\nimport \"testing\"\n\nfunc TestSteps(t *testing.T) {}\n",
	} {
		writeFile(t, name, text)
	}
	mutants, o := listedJSONOf(t, listedMutants-1)
	for _, m := range mutants {
		if mutantLine(m) == listedManyLine && (m["outcome"] != "killed" || m["scope"] != "listed") {
			t.Fatalf("the first run: %s, scope %v, want killed by ID-A-02\nstderr:\n%s", describe(m), m["scope"], o.stderr)
		}
	}
}

// listedJSONOf is listedJSON for a main.go of n mutants.
func listedJSONOf(t *testing.T, n int) ([]map[string]any, outcome) {
	t.Helper()
	o := mutateCovered(t, "--json", greetSource)
	for _, f := range o.rawFiles(t) {
		if f["file"] != greetSource {
			continue
		}
		var out []map[string]any
		list, _ := f["mutants"].([]any)
		for _, m := range list {
			out = append(out, m.(map[string]any))
		}
		if len(out) != n {
			t.Fatalf("%d mutants of %s, want %d\n%s\nstderr:\n%s", len(out), greetSource, n, o.stdout, o.stderr)
		}
		return out, o
	}
	t.Fatalf("no %s in files:\n%s\nstderr:\n%s", greetSource, o.stdout, o.stderr)
	return nil, o
}

// checked runs mutation check --json on main.go and returns what it said
// of each function, by namespace#name: its state, and the message of its
// mutation.stale problem, if any.
func checked(t *testing.T) (map[string]string, map[string]string, outcome) {
	t.Helper()
	o := cli(t, "mutation", "check", "--json", greetSource)
	m := o.json(t)
	var out struct {
		Files []struct {
			Functions []struct {
				Function string `json:"function"`
				State    string `json:"state"`
			} `json:"functions"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(o.stdout), &out); err != nil {
		t.Fatal(err)
	}
	states, stale := map[string]string{}, map[string]string{}
	for _, f := range out.Files {
		for _, fn := range f.Functions {
			states[fn.Function] = fn.State
		}
	}
	for _, p := range m.Problems {
		if p["rule"] == "mutation.stale" {
			fn, _ := p["function"].(string)
			stale[fn], _ = p["message"].(string)
		}
	}
	return states, stale, o
}

// @ID-MUT-130
func TestAListedKillStaysFreshWhileItsTestsFilesAndTheSupportFilesAreUnchanged(t *testing.T) {
	t.Parallel()
	freshRepo(t)

	if _, stale, o := checked(t); o.code != 0 {
		t.Fatalf("mutation check: exit %d, stale %v, want 0\n%s%s", o.code, stale, o.stdout, o.stderr)
	}
	mutants, o := listedJSONOf(t, listedMutants-1)
	for _, m := range mutants {
		if mutantLine(m) == listedManyLine && (m["reused"] != true || m["scope"] != "listed") {
			t.Errorf("%s: reused %v, scope %v, want the listed kill reused\nstderr:\n%s", describe(m), m["reused"], m["scope"], o.stderr)
		}
	}
}

// @ID-MUT-131
func TestEditingTheFileOfACoveringTestMakesItsKillStale(t *testing.T) {
	t.Parallel()
	freshRepo(t)
	writeFile(t, "a.feature", "Feature: a, edited\n")

	states, stale, o := checked(t)
	if states[greetID] != "stale" || !strings.Contains(stale[greetID], "a.feature") {
		t.Fatalf("mutation check: %s is %q, stale problem %q, want stale naming a.feature\n%s", greetID, states[greetID], stale[greetID], o.stdout)
	}
	mutants, o := listedJSONOf(t, listedMutants-1)
	for _, m := range mutants {
		if mutantLine(m) != listedManyLine {
			continue
		}
		if m["reused"] == true || m["outcome"] != "killed" {
			t.Errorf("%s: reused %v, want it run again and killed", describe(m), m["reused"])
		}
		want := fmt.Sprintf(" %s:%d `%v` → `%v` survived its own tests, then killed with ID-A-02 (", greetSource, listedManyLine, m["original"], m["replacement"])
		if !strings.Contains(o.stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, o.stderr)
		}
	}
}

// @ID-MUT-132
func TestEditingAFileNoCoveringTestNamesChangesNothing(t *testing.T) {
	t.Parallel()
	freshRepo(t)
	writeFile(t, "b.feature", "Feature: b, edited\n")

	if states, stale, o := checked(t); o.code != 0 {
		t.Errorf("mutation check: exit %d, states %v, stale %v, want 0", o.code, states, stale)
	}
}

// @ID-MUT-133
func TestEditingASupportFileMakesEveryListedKillStale(t *testing.T) {
	t.Parallel()
	freshRepo(t)
	writeFile(t, filepath.Join("features", "steps_test.go"), "package features\n\nimport \"testing\"\n\nfunc TestSteps(t *testing.T) { t.Log(1) }\n")

	states, stale, o := checked(t)
	for _, fn := range []string{mainID, greetID} {
		if states[fn] != "stale" || !strings.Contains(stale[fn], "features/steps_test.go") {
			t.Errorf("%s is %q, stale problem %q, want stale naming features/steps_test.go", fn, states[fn], stale[fn])
		}
	}
	if t.Failed() {
		t.Fatalf("stdout:\n%s", o.stdout)
	}
	if states[halfID] != "fresh" {
		t.Errorf("%s is %q, want fresh: its own test killed its mutant", halfID, states[halfID])
	}
}

// @ID-MUT-134
func TestTheGraphAgrees(t *testing.T) {
	t.Parallel()
	freshRepo(t)
	writeFile(t, "a.feature", "Feature: a, edited\n")
	root := wd(t)

	b, err := graph.NewBuilder([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	g, _, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	stale := map[string]bool{}
	for _, n := range g.Nodes {
		for _, u := range n.Units {
			if u.File == greetSource {
				stale[u.Namespace+"#"+u.Name] = u.Stale
			}
		}
	}
	if !stale[greetID] {
		t.Errorf("the graph's %s is not stale (%v), as mutation check calls it", greetID, stale)
	}
	if stale[halfID] {
		t.Errorf("the graph's %s is stale, while mutation check calls it fresh", halfID)
	}
}
