package graph

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func build(t *testing.T) *Graph {
	t.Helper()
	b, err := NewBuilder([]string{"testdata/shop"})
	if err != nil {
		t.Fatal(err)
	}
	g, changed, err := b.Build()
	if err != nil || !changed {
		t.Fatalf("build: changed=%v err=%v", changed, err)
	}
	return g
}

func node(g *Graph, id string) *Node {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}

func TestEdgesResolveEachLanguagesImports(t *testing.T) {
	g := build(t)
	var got []string
	for _, e := range g.Edges {
		got = append(got, fmt.Sprintf("%s -> %s", e.From, e.To))
	}
	want := []string{
		"shop/api -> shop/api/internal/store",
		"shop/app/src/main/kotlin/com/acme/billing/Invoice -> shop/app/src/main/kotlin/com/acme/util/Format",
		"shop/app/src/main/kotlin/com/acme/billing/Invoice -> shop/app/src/main/kotlin/com/acme/util/Money",
		"shop/py/src/shop/billing/invoice -> shop/py/src/shop/billing/tax",
		"shop/py/src/shop/cli -> shop/py/src/shop/billing/invoice",
		"shop/py/src/shop/cli -> shop/py/src/shop/billing/tax",
		"shop/web/src/app -> shop/web/src/cart/total",
		"shop/web/src/cart/total -> shop/web/src/format/index",
		// Through tsconfig paths: "@/format", "~format", "@/cart/total".
		"shop/web/src/cart/view -> shop/web/src/cart/total",
		"shop/web/src/cart/view -> shop/web/src/format/index",
	}
	if !slices.Equal(got, want) {
		t.Errorf("edges:\n got  %q\n want %q", got, want)
	}
}

func TestExternalPackagesAreListedNotDrawn(t *testing.T) {
	g := build(t)
	cases := map[string][]string{
		"shop/web/src/app":                 {"@scope/zod", "react"},
		"shop/py/src/shop/billing/invoice": {"os"},
		"shop/api":                         {"fmt"},
		"shop/app/src/main/kotlin/com/acme/billing/Invoice": {"kotlin.math"},
	}
	for id, want := range cases {
		n := node(g, id)
		if n == nil {
			t.Fatalf("no node %s", id)
		}
		if !slices.Equal(n.External, want) {
			t.Errorf("%s external %v, want %v", id, n.External, want)
		}
	}
}

func TestTreeAndModules(t *testing.T) {
	g := build(t)
	api := node(g, "shop/api")
	if api.Kind != "module" || api.Parent != "shop" || api.Language != "go" || len(api.children) != 1 {
		t.Errorf("a Go package with a subpackage is one module node: %+v", api)
	}
	if pkg := node(g, "shop/py/src/shop/billing"); pkg.Kind != "module" || pkg.Language != "python" {
		t.Errorf("a Python package is its __init__ module: %+v", pkg)
	}
	if n := node(g, "shop/web/src"); n == nil || n.Kind != "dir" || n.Parent != "shop/web" {
		t.Errorf("directory node %+v", n)
	}
	if n := node(g, "shop"); n == nil || n.Kind != "repo" {
		t.Errorf("repo node %+v", n)
	}
	inv := node(g, "shop/py/src/shop/billing/invoice")
	if inv.Metrics == nil || inv.Metrics.Functions != 1 || inv.Units[0].Name != "invoice" {
		t.Errorf("module units %+v metrics %+v", inv.Units, inv.Metrics)
	}
}

func TestUnchangedSourcesDoNotRebuild(t *testing.T) {
	b, _ := NewBuilder([]string{"testdata/shop"})
	first, _, _ := b.Build()
	again, changed, err := b.Build()
	if err != nil || changed || again.Version != first.Version {
		t.Errorf("second build changed=%v version %d→%d err=%v", changed, first.Version, again.Version, err)
	}
}

func TestGrades(t *testing.T) {
	cases := []struct {
		got, want float64
	}{
		{crapGrade(3), 10}, {crapGrade(30), 1}, {crapGrade(17.5), 5.5},
		{mutationGrade(9, 1), 9.1}, {mutationGrade(0, 4), 1},
		// One high-risk function in ten: noticeable, not alarming.
		{must(bandGrade(Bands{Low: 9, High: 1})), 8.2},
		{must(bandGrade(Bands{Low: 1, High: 1})), 1},
		{must(bandGrade(Bands{Low: 7, Medium: 3, Unknown: 40})), 8.4},
	}
	for i, c := range cases {
		if c.got != c.want {
			t.Errorf("case %d: %v, want %v", i, c.got, c.want)
		}
	}
}

// A Go package has an init per file: results for one file's init must not
// land on the others.
func TestMetricsMatchTheirOwnFile(t *testing.T) {
	root := t.TempDir()
	write := func(rel, text string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/p\n\ngo 1.22\n")
	write("a.go", "package p\n\nfunc init() { _ = 1 > 0 }\n")
	write("b.go", "package p\n\nfunc init() { _ = 2 > 0 }\n")
	write(".metrics/mutate/a.go.json", `{"version":1,"file":"a.go","language":"go","units":[
		{"namespace":"example.com/p","name":"init","hash":"x","killed":2,"survived":0,"uncovered":0,"sites":2,"mutants":[]}]}`)
	write(".metrics/crap.json", `{"version":1,"entries":[
		{"namespace":"example.com/p","name":"init","file":"a.go","complexity":1,"coverage":100,"crap":1},
		{"namespace":"example.com/p","name":"init","file":"b.go","complexity":1,"coverage":0,"crap":2}]}`)
	b, _ := NewBuilder([]string{root})
	g, _, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, n := range g.Nodes {
		for _, u := range n.Units {
			got = append(got, fmt.Sprintf("%s mutated=%v killed=%d coverage=%v", u.File, u.Mutated, u.Killed, *u.Coverage))
		}
	}
	want := []string{"a.go mutated=true killed=2 coverage=100", "b.go mutated=false killed=0 coverage=0"}
	if !slices.Equal(got, want) {
		t.Errorf("units:\n got  %q\n want %q", got, want)
	}
}

func TestSnapshotsFromAnotherDirectoryMatchBySuffix(t *testing.T) {
	k := newKeyed[int]()
	k.add("repo/lang/kotlin.go", "p", "init", 7)
	if v, ok := k.get("lang/kotlin.go", "p", "init", 0); !ok || v != 7 {
		t.Errorf("suffix match: %v %v", v, ok)
	}
	if _, ok := k.get("lang/golang.go", "p", "init", 0); ok {
		t.Error("another file's init matched")
	}
}

func must(v float64, ok bool) float64 {
	if !ok {
		panic("no grade")
	}
	return v
}

func TestContainersAreGradedByTheirShareOfRisk(t *testing.T) {
	low, high := 2.0, 120.0
	g := &repoGraph{name: "r", nodes: map[string]*Node{}}
	dir := &Node{ID: "r", Kind: "repo"}
	bad := &Node{ID: "r/bad", Kind: "module", Units: []Unit{{CRAP: &high, Mutated: true, Killed: 1, Survived: 1}}}
	var good []*Node
	for i := range 9 {
		good = append(good, &Node{ID: fmt.Sprintf("r/ok%d", i), Kind: "module", Units: []Unit{{CRAP: &low, Mutated: true, Killed: 4}}})
	}
	dir.children = append([]*Node{bad}, good...)
	g.nodes["r"] = dir
	g.summarize()
	if *bad.Metrics.CRAPGrade != 1 {
		t.Errorf("a module is as bad as its worst function: %v", *bad.Metrics.CRAPGrade)
	}
	m := dir.Metrics
	if *m.CRAPGrade != 8.2 || m.Mutated != 10 || m.Functions != 10 {
		t.Errorf("repo grade %v mutated %d/%d, want 8.2 and 10/10", *m.CRAPGrade, m.Mutated, m.Functions)
	}
	if *m.MutationGrade != mutationGrade(37, 1) {
		t.Errorf("repo mutation grade %v is not the overall kill rate", *m.MutationGrade)
	}
}

func TestTSConfigAliases(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte(`{
		// comment with "quotes" and a // inside
		"compilerOptions": { "baseUrl": "src", "paths": { "@app/*": ["app/*", "legacy/*"], "@app/core": ["core/index"], }, },
	}`), 0o644)
	a := loadTSAliases(dir)
	rels := func(spec string) []string {
		var out []string
		for _, p := range a.resolve(spec) {
			r, _ := filepath.Rel(dir, p)
			out = append(out, filepath.ToSlash(r))
		}
		return out
	}
	cases := map[string][]string{
		"@app/button":  {"src/app/button", "src/legacy/button", "src/@app/button"},
		"@app/core":    {"src/core/index", "src/@app/core"},
		"utils/format": {"src/utils/format"},
	}
	for spec, want := range cases {
		if got := rels(spec); !slices.Equal(got, want) {
			t.Errorf("%s → %v, want %v", spec, got, want)
		}
	}
}

func TestStripJSONC(t *testing.T) {
	in := `{"a": "x // not a comment", /* gone */ "b": [1, 2,], // gone
}`
	var v map[string]any
	if err := json.Unmarshal(stripJSONC([]byte(in)), &v); err != nil || v["a"] != "x // not a comment" {
		t.Errorf("stripped %q: %v %v", stripJSONC([]byte(in)), v, err)
	}
}
