package graph

import (
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/mutants/porttest"
)

func TestBuilderUsesActualMutationObservationBindings(t *testing.T) {
	m := newModule(t)
	source := "package p\nfunc Value(x int) bool { return x > 1 }\n"
	test := "package p\nimport \"testing\"\nfunc TestValue(t *testing.T) { _ = Value(2) }\n"
	m.write("value.go", source)
	m.write("value_test.go", test)
	// The physical snapshot is a decoy and a real cache stamp. The injected
	// reader owns a different result for the same source identity.
	m.snapshot("value.go", m.tests("value_test.go"), m.recorded("value.go"))
	snapshot := mutants.Snapshot{Version: 1, File: "value.go", Language: "go", Tests: m.tests("value_test.go"), Units: m.recorded("value.go")}
	for i := range snapshot.Units {
		for j := range snapshot.Units[i].Mutants {
			snapshot.Units[i].Mutants[j].Outcome = mutants.Survived
		}
		snapshot.Units[i].Survived = snapshot.Units[i].Sites
		snapshot.Units[i].Killed = 0
	}
	reader := &porttest.Snapshots{RootDir: m.root, Values: map[string]mutants.Snapshot{"value.go": snapshot}}
	inputs := &porttest.Inputs{Dir: m.root, Files: map[string]string{filepath.Join(m.root, "go.mod"): "module example.com/p\n", filepath.Join(m.root, "value.go"): source, filepath.Join(m.root, "value_test.go"): test}}
	readers := map[string]mutants.MutationSnapshotReader{m.root: reader}
	bindings := MutationInputs{Snapshots: func(root string) mutants.MutationSnapshotReader { return readers[root] }, Toolchain: inputs, Fingerprints: inputs}
	build := func(t *testing.T, bindings MutationInputs) *Graph {
		t.Helper()
		b, err := NewBuilder([]string{m.root}, bindings)
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
			if _, err := NewBuilder([]string{m.root}, missing); err == nil {
				t.Fatal("missing binding silently selected disk")
			}
		})
	}
	bindings.Snapshots = func(string) mutants.MutationSnapshotReader { return nil }
	b, err := NewBuilder([]string{m.root}, bindings)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.Build(t.Context()); err == nil {
		t.Fatal("missing root-specific reader silently selected disk")
	}
}

func TestBuilderUsesActualToolchainAfterSnapshotAdmission(t *testing.T) {
	m := newModule(t)
	m.write("a.ts", "export function value(x: number) { return x + 1; }\n")
	m.write(".metrics/mutate/a.ts.json", "physical decode decoy")
	reader := &porttest.Snapshots{RootDir: m.root, Values: map[string]mutants.Snapshot{"a.ts": {File: "a.ts", Language: "typescript", Units: []mutants.UnitResult{{Mutants: []mutants.Mutant{{Scope: mutants.ScopeOwn, Outcome: mutants.Killed}}}}}}}
	broad := &porttest.Inputs{Dir: m.root, Files: map[string]string{filepath.Join(m.root, "package.json"): `{"scripts":{"test":"independent whole-suite command"}}`}}
	narrow := &porttest.Inputs{Dir: m.root, Files: map[string]string{filepath.Join(m.root, "package.json"): `{"devDependencies":{"vitest":"*"}}`, filepath.Join(m.root, "node_modules", ".bin", "vitest"): "independent installed entry", filepath.Join(m.root, "node_modules", ".bin", "vitest.cmd"): "independent installed entry"}}
	for _, tc := range []struct {
		name  string
		tools mutants.Toolchain
		want  string
	}{{"whole suite", broad, mutants.ScopeAllTests}, {"different actual tool facts", narrow, mutants.ScopeOwn}} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := NewBuilder([]string{m.root}, MutationInputs{Snapshots: func(string) mutants.MutationSnapshotReader { return reader }, Toolchain: tc.tools, Fingerprints: broad})
			if err != nil {
				t.Fatal(err)
			}
			s, err := b.snapshot(t.Context(), m.root, filepath.Join(m.root, "a.ts"), map[string]bool{})
			if err != nil || s == nil || s.Units[0].Mutants[0].Scope != tc.want {
				t.Fatalf("current bound command normalization: %+v %v", s, err)
			}
		})
	}
	if reader.Values["a.ts"].Units[0].Mutants[0].Scope != mutants.ScopeOwn {
		t.Fatal("current command normalization rewrote stored evidence")
	}
}
