package mutants_test

import (
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
	covtest "github.com/donvargax/itos-cc/internal/coverage/porttest"
	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/mutants/porttest"
)

func TestFreshProjectionOwnsOnlyItsFactsAndNeverLiveInputBytes(t *testing.T) {
	b := memoryFreshFixture(t)
	committed := "package value\nfunc Value() int { return 1 }\n"
	for path, text := range map[string]string{"go.mod": "module example/value\n", "value.go": committed, "value_test.go": "package value\n"} {
		if err := b.Put(t.Context(), path, text); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-qm", "independent observations"}} {
		if err := b.Git(t.Context(), args...); err != nil {
			t.Fatal(err)
		}
	}
	f := b.Fresh.(*porttest.Fresh)
	source, test := filepath.Join(b.Root, "value.go"), filepath.Join(b.Root, "value_test.go")
	f.Inputs.Files[source] = "package value\nfunc Value() int { return 999 }\n"
	f.Inputs.Imports = map[string][]string{test: {source}}
	f.Inputs.Probes = map[porttest.PytestProbe]bool{porttest.ProbeKey("/external/python", b.Root, nil): true}
	f.Coverage = &covtest.Measurer{Declarations: map[coverage.Unloaded]covtest.Measurement{{Language: "go", Dir: b.Root}: {Language: "go", Report: "mode: set\nvalue.go:2.1,2.30 1 1\n", Metadata: []byte("independent metadata"), Listed: map[string]map[string]*coverage.Report{"go": {}}}}}
	f.Commands = &porttest.Workers{Files: f.Inputs.Files, Tests: map[string]map[string]porttest.Tests{b.Root: {"literal shell text": {Trace: &porttest.Trace{Source: "value.go", Tests: map[string]string{"TestValue": "value_test.go"}}}}}}
	f.Lister = &porttest.Lister{Listings: map[string]map[string]porttest.Listing{b.Root: {"literal shell text": {Stdout: "A\tvalue_test.go\n"}}}}
	first, failed := f.Plan(t.Context(), mutants.FreshRequest{Root: b.Root, Count: 1})
	if failed != nil {
		t.Fatal(failed)
	}
	defer f.Release(t.Context(), first)
	second, failed := f.Plan(t.Context(), mutants.FreshRequest{Root: b.Root, Count: 1})
	if failed != nil {
		t.Fatal(failed)
	}
	defer f.Release(t.Context(), second)
	for _, p := range []*mutants.FreshPlan{first, second} {
		if f.Inputs.Files[filepath.Join(p.FrozenRoot, "value.go")] != committed {
			t.Fatal("projection copied live source over committed bytes")
		}
		edges := f.Inputs.Imports[filepath.Join(p.FrozenRoot, "value_test.go")]
		if len(edges) != 1 || edges[0] != filepath.Join(p.FrozenRoot, "value.go") {
			t.Fatalf("root-relative import identities: %v", edges)
		}
		if !f.Inputs.Probes[porttest.ProbeKey("/external/python", p.FrozenRoot, nil)] {
			t.Fatal("external interpreter identity changed")
		}
	}
	group := coverage.Unloaded{Language: "go", Dir: first.FrozenRoot}
	d := f.Coverage.Declarations[group]
	d.Metadata[0] = 'X'
	d.Listed["go"]["first-only"] = &coverage.Report{}
	f.Inputs.Imports[filepath.Join(first.FrozenRoot, "value_test.go")][0] = "changed projected edge"
	f.Commands.Tests[first.FrozenRoot]["literal shell text"].Trace.Tests["first-only"] = "unrelated.go"
	for _, root := range []string{b.Root, second.FrozenRoot} {
		facts := f.Coverage.Declarations[coverage.Unloaded{Language: "go", Dir: root}]
		if string(facts.Metadata) != "independent metadata" || len(facts.Listed["go"]) != 0 || len(f.Commands.Tests[root]["literal shell text"].Trace.Tests) != 1 {
			t.Fatal("one projection mutated live or another plan's independent facts")
		}
	}
	firstRoot := first.FrozenRoot
	f.Release(t.Context(), first)
	if _, exists := f.Commands.Tests[firstRoot]; exists {
		t.Fatal("released command observations remained")
	}
	if _, exists := f.Coverage.Declarations[coverage.Unloaded{Language: "go", Dir: firstRoot}]; exists {
		t.Fatal("released collector observations remained")
	}
	if len(f.Commands.Tests[b.Root]) != 1 || len(f.Commands.Tests[second.FrozenRoot]) != 1 || f.Inputs.Files[source] == committed {
		t.Fatal("release removed another plan/live state")
	}
}
