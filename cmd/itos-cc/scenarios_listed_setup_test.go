package main

import (
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/donvargax/itos-cc/features"
	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
)

func listedSetupFixture(t *testing.T) (*memory, string) {
	t.Helper()
	m := newMemory(t)
	root, err := m.Folder()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Write(filepath.Join(root, "main.go"), []byte("package main\nfunc Value() bool { return true }\n")); err != nil {
		t.Fatal(err)
	}
	if err := m.Coverage(root, features.Coverage{Language: "go", Report: "mode: set\nexample/main.go:2.19,2.32 1 1\n"}); err != nil {
		t.Fatal(err)
	}
	return m, root
}
func TestListedMeasurementSetupOwnsIndependentArtifactFacts(t *testing.T) {
	m, root := listedSetupFixture(t)
	listed := &features.ListedTests{Files: map[string]string{"A": "a.feature"}, Lines: map[string]map[int][]string{"main.go": {2: {"A"}}}, Split: true, CoverageMetadata: []byte("original owned metadata"), CoverageOutput: "collection output", ConversionOutput: "conversion output"}
	if err := m.Tests(root, features.Tests{Language: "go", Command: "go test ./first", Listed: listed}); err != nil {
		t.Fatal(err)
	}
	listed.Split = false
	listed.CoverageExit = 1
	listed.ConversionExit = 1
	listed.CoverageMetadata[0] = 'X'
	listed.Lines["main.go"][2][0] = "tampered"
	d := m.measurer(root).Declarations[coverage.Unloaded{Language: "go", Dir: root}]
	if !d.Split || d.ListedFailed || d.ConversionExit != 0 || string(d.Metadata) != "original owned metadata" || d.Listed["go"]["A"] == nil {
		t.Fatalf("caller mutation changed retained measurement facts: %+v", d)
	}
}
func TestListedMeasurementKeepsLatestExplicitRootSetupAcrossCommands(t *testing.T) {
	m, root := listedSetupFixture(t)
	a := &features.ListedTests{Files: map[string]string{"A": "a.feature"}, Lines: map[string]map[int][]string{"main.go": {2: {"A"}}}, Split: false}
	b := &features.ListedTests{Files: map[string]string{"B": "b.feature"}, Lines: map[string]map[int][]string{"main.go": {2: {"B"}}}, Split: true}
	for _, declaration := range []features.Tests{{Language: "go", Command: "go test ./first", Baseline: "first", Listed: a}, {Language: "go", Command: "go test ./second", Baseline: "second", Listed: b}, {Language: "go", Command: "go test ./third", Baseline: "third"}} {
		if err := m.Tests(root, declaration); err != nil {
			t.Fatal(err)
		}
	}
	d := m.measurer(root).Declarations[coverage.Unloaded{Language: "go", Dir: root}]
	if !d.Split || d.Listed["go"]["B"] == nil || d.Listed["go"]["A"] != nil {
		t.Fatalf("root setup depends on command-map order or omission: %+v", d)
	}
	for command, want := range map[string]string{"go test ./first": "first", "go test ./second": "second", "go test ./third": "third"} {
		if got := m.tests[root][command].Baseline; got != want {
			t.Fatalf("independent command %s: %q", command, got)
		}
	}
}

// ownListed owns fixture setup bytes without translating its semantic model.
func ownListed(in *features.ListedTests) *features.ListedTests {
	if in == nil {
		return nil
	}
	out := *in
	out.Files = maps.Clone(in.Files)
	out.Elapsed = maps.Clone(in.Elapsed)
	out.Failing = slices.Clone(in.Failing)
	out.CoverageMetadata = slices.Clone(in.CoverageMetadata)
	if in.Lines != nil {
		out.Lines = map[string]map[int][]string{}
		for file, lines := range in.Lines {
			out.Lines[file] = maps.Clone(lines)
			for line, ids := range lines {
				out.Lines[file][line] = slices.Clone(ids)
			}
		}
	}
	cloneSites := func(in map[string][]features.Site) map[string][]features.Site {
		if in == nil {
			return nil
		}
		out := map[string][]features.Site{}
		for id, sites := range in {
			out[id] = slices.Clone(sites)
		}
		return out
	}
	out.Surviving = cloneSites(in.Surviving)
	out.TimingOut = cloneSites(in.TimingOut)
	return &out
}
func TestListedRootSetupDeepCopiesAllNestedFacts(t *testing.T) {
	m, root := listedSetupFixture(t)
	input := &features.ListedTests{Files: map[string]string{"A": "a.feature"}, Lines: map[string]map[int][]string{"main.go": {2: {"A"}}}, Elapsed: map[string]time.Duration{"A": time.Second}, Failing: []string{"A"}, Surviving: map[string][]features.Site{"A": {{File: "main.go", Original: ">"}}}, TimingOut: map[string][]features.Site{"A": {{File: "main.go", Replacement: "<="}}}, CoverageMetadata: []byte("owned"), Split: true}
	if err := m.Tests(root, features.Tests{Language: "go", Command: "go test ./first", Listed: input}); err != nil {
		t.Fatal(err)
	}
	input.Files["A"] = "changed"
	input.Lines["main.go"][2][0] = "changed"
	input.Elapsed["A"] = 2 * time.Second
	input.Failing[0] = "changed"
	input.Surviving["A"][0].Original = "changed"
	input.TimingOut["A"][0].Replacement = "changed"
	input.CoverageMetadata[0] = 'X'
	input.Split = false
	expected := &features.ListedTests{Files: map[string]string{"A": "a.feature"}, Lines: map[string]map[int][]string{"main.go": {2: {"A"}}}, Elapsed: map[string]time.Duration{"A": time.Second}, Failing: []string{"A"}, Surviving: map[string][]features.Site{"A": {{File: "main.go", Original: ">"}}}, TimingOut: map[string][]features.Site{"A": {{File: "main.go", Replacement: "<="}}}, CoverageMetadata: []byte("owned"), Split: true}
	if !reflect.DeepEqual(m.listed[root], expected) {
		t.Fatalf("root setup not owned: %+v", m.listed[root])
	}
	if !reflect.DeepEqual(m.tests[root]["go test ./first"].Listed, expected) {
		t.Fatal("command setup not independently owned")
	}
	m.listed[root].Files["A"] = "root-only change"
	if m.tests[root]["go test ./first"].Listed.Files["A"] != "a.feature" {
		t.Fatal("root setup aliases command-keyed outcomes")
	}
}

func TestListedMeasurementRequiresExplicitRootSetup(t *testing.T) {
	m, root := listedSetupFixture(t)
	request := covport.Request{Sources: []string{filepath.Join(root, "main.go")}}
	tests := &coverage.PerTest{All: "go test ./...", Tests: []coverage.Test{{ID: "A", File: "a.feature"}}, Select: func([]string) string { return "go test ./selected" }}
	for _, explicit := range []bool{false, true} {
		if explicit {
			if err := m.Tests(root, features.Tests{Language: "go", Command: "go test ./...", Listed: &features.ListedTests{Split: true}}); err != nil {
				t.Fatal(err)
			}
		}
		measured := m.measurer(root)
		if _, failure := measured.MeasureListed(t.Context(), request, tests); failure != nil {
			t.Fatal(failure)
		}
		if explicit {
			if !reflect.DeepEqual(measured.Commands, []string{"go test ./..."}) {
				t.Fatalf("explicit empty setup did not enter its collector: %v", measured.Commands)
			}
		} else if len(measured.Commands) != 0 || measured.Declarations[coverage.Unloaded{Language: "go", Dir: root}].Listed != nil {
			t.Fatalf("omitted setup invented listed collection: %v", measured.Commands)
		}
	}
}

func TestListedSetupCopyPreservesNilAndEmptyCollections(t *testing.T) {
	for _, original := range []*features.ListedTests{
		{},
		{Files: map[string]string{}, Lines: map[string]map[int][]string{"nil": nil, "empty": {}, "ids": {1: nil, 2: {}}}, Failing: []string{}, CoverageMetadata: []byte{}},
	} {
		if copy := ownListed(original); !reflect.DeepEqual(copy, original) {
			t.Fatalf("copy changed absent/empty facts: got %#v want %#v", copy, original)
		}
	}
}
