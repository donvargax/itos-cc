package main

import (
	"errors"
	"io"
	"log"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/slices/graph"
	mutation "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

func TestBuilderUsesActualMutationObservationBindings(t *testing.T) {
	m := newGraphModule(t)
	source := "package p\nfunc Value(x int) bool { return x > 1 }\n"
	test := "package p\nimport \"testing\"\nfunc TestValue(t *testing.T) { _ = Value(2) }\n"
	m.write("value.go", source)
	m.write("value_test.go", test)
	// The physical snapshot is a decoy and a real cache stamp. The injected
	// reader owns a different result for the same source identity.
	m.snapshot("value.go", m.tests("value_test.go"), m.recorded("value.go"))
	snapshot := mutation.Snapshot{Version: 1, File: "value.go", Language: "go", Tests: m.tests("value_test.go"), Units: m.recorded("value.go")}
	for i := range snapshot.Units {
		for j := range snapshot.Units[i].Mutants {
			snapshot.Units[i].Mutants[j].Outcome = mutation.Survived
		}
		snapshot.Units[i].Survived = snapshot.Units[i].Sites
		snapshot.Units[i].Killed = 0
	}
	reader := &porttest.Snapshots{RootDir: m.root, Values: map[string]mutation.Snapshot{"value.go": snapshot}}
	inputs := &porttest.Inputs{Dir: m.root, Files: map[string]string{filepath.Join(m.root, "go.mod"): "module example.com/p\n", filepath.Join(m.root, "value.go"): source, filepath.Join(m.root, "value_test.go"): test}}
	readers := map[string]mutation.MutationSnapshotReader{m.root: reader}
	bindings := graphInputs{Snapshots: func(root string) mutation.MutationSnapshotReader { return readers[root] }, Toolchain: inputs, Fingerprints: inputs}
	build := func(t *testing.T, bindings graphInputs) *graph.Graph {
		t.Helper()
		b, err := newGraphBuilder([]string{m.root}, bindings)
		if err != nil {
			t.Fatal(err)
		}
		g, _, err := b.Build(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	_, value := unit(t, build(t, bindings), "Value")
	if !value.Mutated || value.Stale || value.Survived == 0 || value.Killed != 0 || len(reader.ReadKeys) == 0 {
		t.Fatalf("physical snapshot hid actual reader: %+v reads%v", value, reader.ReadKeys)
	}
	inputs.Files[filepath.Join(m.root, "value_test.go")] = test + "// independently changed memory test\n"
	_, value = unit(t, build(t, bindings), "Value")
	if !value.Stale {
		t.Fatal("physical current tests hid actual fingerprint binding")
	}
	for _, field := range []string{"snapshots", "toolchain", "fingerprints"} {
		t.Run("missing "+field, func(t *testing.T) {
			missing := bindings
			switch field {
			case "snapshots":
				missing.Snapshots = nil
			case "toolchain":
				missing.Toolchain = nil
			case "fingerprints":
				missing.Fingerprints = nil
			}
			if _, err := newGraphBuilder([]string{m.root}, missing); err == nil {
				t.Fatal("missing binding silently selected disk")
			}
		})
	}
	bindings.Snapshots = func(string) mutation.MutationSnapshotReader { return nil }
	b, err := newGraphBuilder([]string{m.root}, bindings)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.Build(t.Context()); err == nil {
		t.Fatal("missing root-specific reader silently selected disk")
	}
}

func TestBuilderUsesActualToolchainAfterSnapshotAdmission(t *testing.T) {
	m := newGraphModule(t)
	m.write("a.ts", "export function value(x: number) { return x + 1; }\n")
	m.write(".metrics/mutate/a.ts.json", "physical decode decoy")
	reader := &porttest.Snapshots{RootDir: m.root, Values: map[string]mutation.Snapshot{"a.ts": {File: "a.ts", Language: "typescript", Units: []mutation.UnitResult{{Mutants: []mutation.Mutant{{Scope: mutation.ScopeOwn, Outcome: mutation.Killed}}}}}}}
	broad := &porttest.Inputs{Dir: m.root, Files: map[string]string{filepath.Join(m.root, "package.json"): `{"scripts":{"test":"independent whole-suite command"}}`}}
	narrow := &porttest.Inputs{Dir: m.root, Files: map[string]string{filepath.Join(m.root, "package.json"): `{"devDependencies":{"vitest":"*"}}`, filepath.Join(m.root, "node_modules", ".bin", "vitest"): "independent installed entry", filepath.Join(m.root, "node_modules", ".bin", "vitest.cmd"): "independent installed entry"}}
	for _, tc := range []struct {
		name  string
		tools mutation.Toolchain
		want  string
	}{{"whole suite", broad, mutation.ScopeAllTests}, {"different actual tool facts", narrow, mutation.ScopeOwn}} {
		t.Run(tc.name, func(t *testing.T) {
			g, err := newGraphMutation(func(string) mutation.MutationSnapshotReader { return reader }, tc.tools, broad)
			if err != nil {
				t.Fatal(err)
			}
			read, err := g.Snapshot(t.Context(), m.root, "a.ts")
			s, _ := read.(*mutation.Snapshot)
			if err != nil || s == nil || s.Units[0].Mutants[0].Scope != tc.want {
				t.Fatalf("current bound command normalization: %+v %v", s, err)
			}
		})
	}
	if reader.Values["a.ts"].Units[0].Mutants[0].Scope != mutation.ScopeOwn {
		t.Fatal("current command normalization rewrote stored evidence")
	}
}

// The binding hands graph what the mutation slice's judgment says: the
// support files the settings at a project's root list, hashed, none when
// the settings list no tests or cannot be read, and the hashes of the
// tests, refused with the fingerprints' failure.
func TestTheGraphBindingReadsEachProjectsSettings(t *testing.T) {
	root := t.TempDir()
	inputs := &porttest.Inputs{Dir: root, Files: map[string]string{filepath.Join(root, "testdata", "a.txt"): "a", filepath.Join(root, "a_test.go"): "package a\n"}}
	g, err := newGraphMutation(func(string) mutation.MutationSnapshotReader { return &porttest.Snapshots{RootDir: root} }, inputs, inputs)
	if err != nil {
		t.Fatal(err)
	}
	configs := map[string]*mutation.Config{root: {Tests: &mutation.Tests{Support: []string{"testdata/*.txt"}}}, "untested": {}}
	g.settings = func(root string) (*mutation.Config, error) {
		if c, ok := configs[root]; ok {
			return c, nil
		}
		return nil, errors.New("unreadable")
	}
	if got := g.Support(t.Context(), root); len(got) != 1 || got["testdata/a.txt"] == "" {
		t.Errorf("support: %v, want testdata/a.txt's hash", got)
	}
	for _, top := range []string{"untested", "unreadable"} {
		if got := g.Support(t.Context(), top); got != nil {
			t.Errorf("support of %s: %v, want none", top, got)
		}
	}
	if got, err := g.Tests(t.Context(), root, []string{filepath.Join(root, "a_test.go")}); err != nil || got["a_test.go"] == "" {
		t.Errorf("tests: %v %v, want a_test.go's hash", got, err)
	}
	if _, err := g.Tests(t.Context(), root, []string{filepath.Join(root, "gone_test.go")}); err == nil {
		t.Error("the hash of a test that is not there was given")
	}
	if got, want := g.SnapshotPath(root, "src/a.go"), filepath.Join(root, ".metrics", "mutate", "src", "a.go.json"); got != want {
		t.Errorf("snapshot path %s, want %s", got, want)
	}
}

// serve builds its graph over the snapshot readers, toolchain and
// fingerprints cmd chose, and publishes what each build gives; a missing
// binding, or a repository that cannot be read, is refused.
func TestServeBuildsItsGraphOverTheMutationBindings(t *testing.T) {
	m := newGraphModule(t)
	m.write("a.go", "package p\n\nfunc Pos(i int) bool { return i > 0 }\n")
	inputs := realMutationInputs()
	a := adapters{SnapshotReaders: inputs.Snapshots, Toolchain: inputs.Toolchain, Fingerprints: inputs.Fingerprints}
	logger := log.New(io.Discard, "", 0)
	if _, err := newServer(t.Context(), []string{m.root}, "", logger, a); err != nil {
		t.Fatal(err)
	}
	b, err := graph.NewBuilder([]string{m.root}, graphMutation{judge: mutation.Graph{Snapshots: inputs.Snapshots, Toolchain: inputs.Toolchain, Fingerprints: inputs.Fingerprints}, settings: func(string) (*mutation.Config, error) { return nil, errors.New("none") }})
	if err != nil {
		t.Fatal(err)
	}
	g, changed, err := servedGraph{b}.Build(t.Context())
	if err != nil || !changed || g.Version != 1 || len(g.Files[filepath.Base(m.root)]) != 1 {
		t.Errorf("served graph: %+v changed=%v err=%v, want version 1 listing a.go", g, changed, err)
	}
	a.Toolchain = nil
	if _, err := newServer(t.Context(), []string{m.root}, "", logger, a); err == nil {
		t.Error("serve built a graph without its toolchain")
	}
	gone := filepath.Join(m.root, "gone")
	b, err = graph.NewBuilder([]string{gone}, graphMutation{judge: mutation.Graph{Snapshots: inputs.Snapshots, Toolchain: inputs.Toolchain, Fingerprints: inputs.Fingerprints}, settings: func(string) (*mutation.Config, error) { return nil, errors.New("none") }})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := (servedGraph{b}).Build(t.Context()); err == nil {
		t.Errorf("serve built the graph of %s, which is not there", gone)
	}
}
