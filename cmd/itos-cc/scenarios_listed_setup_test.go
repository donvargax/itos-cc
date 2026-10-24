package main

import (
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/donvargax/itos-cc/features"
	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/settings"
	mutation "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	mutantstest "github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
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

// listedSelections declares, beside the commands the scenario declared, the
// run command of mutation.tests selecting each set of the listed tests a
// root's harness lists, in their listed order, as the harness behaves:
// each selection takes the time its tests take, fails without a mutant
// when it is the set said to fail alone, and runs a mutant only on the
// lines its tests reach, where the first of them, in order, to notice it
// kills it, to hang on it times it out, and none leaves it surviving.
func (m *memory) listedSelections(w *mutantstest.Workers, text []byte) {
	cfg, err := settings.Parse(text)
	if err != nil || cfg.Tests == nil {
		return
	}
	for dir, listed := range m.listed {
		if listed == nil || listed.ListFails {
			continue
		}
		ids := listedOrder(listed)
		if len(ids) > 12 {
			continue
		}
		if w.Tests[dir] == nil {
			w.Tests[dir] = map[string]mutantstest.Tests{}
		}
		for mask := 1; mask < 1<<len(ids); mask++ {
			var selected []string
			for i, id := range ids {
				if mask&(1<<i) != 0 {
					selected = append(selected, id)
				}
			}
			command := cfg.Tests.Select(selected)
			if _, declared := w.Tests[dir][command]; declared {
				continue
			}
			t := listedSelection(listed, selected)
			// The run command itself, {pattern} unexpanded, declares the
			// trace every selection of it records.
			if template, ok := w.Tests[dir][cfg.Tests.Run]; ok {
				t.Trace = template.Trace
			}
			w.Tests[dir][command] = t
		}
	}
}

// listedOrder is the order the harness lists its tests in: its list
// output's, else by ID.
func listedOrder(listed *features.ListedTests) []string {
	if listed.ListOutput == "" {
		return slices.Sorted(maps.Keys(listed.Files))
	}
	var ids []string
	for _, line := range strings.Split(strings.ReplaceAll(listed.ListOutput, "\r\n", "\n"), "\n") {
		if id, _, _ := strings.Cut(line, "\t"); strings.TrimSpace(id) != "" {
			ids = append(ids, strings.TrimSpace(id))
		}
	}
	return ids
}

// listedSelection is what the run command selecting the tests selected
// does, as listed declares each of them.
func listedSelection(listed *features.ListedTests, selected []string) mutantstest.Tests {
	t := mutantstest.Tests{Reached: map[string][]int{}, Selected: slices.Clone(selected)}
	for _, id := range selected {
		t.Elapsed += listed.Elapsed[id]
	}
	if failing := slices.Sorted(slices.Values(listed.Failing)); len(failing) > 0 && slices.Equal(failing, slices.Sorted(slices.Values(selected))) {
		t.Baseline = "the selection " + strings.Join(selected, ",") + " fails without any mutant"
	}
	for file, lines := range listed.Lines {
		for line, ids := range lines {
			if slices.ContainsFunc(ids, func(id string) bool { return slices.Contains(selected, id) }) {
				t.Reached[file] = append(t.Reached[file], line)
			}
		}
		slices.Sort(t.Reached[file])
	}
	var sites []features.Site
	for _, id := range selected {
		for _, site := range append(slices.Clone(listed.Surviving[id]), listed.TimingOut[id]...) {
			if !slices.Contains(sites, site) {
				sites = append(sites, site)
			}
		}
	}
	for _, site := range sites {
		outcome := mutation.Survived
		for _, id := range selected {
			if !slices.Contains(listed.Lines[site.File][site.Line], id) {
				continue
			}
			if slices.Contains(listed.TimingOut[id], site) {
				outcome = mutation.Timeout
				break
			}
			if !slices.Contains(listed.Surviving[id], site) {
				outcome = mutation.Killed
				break
			}
		}
		if outcome != mutation.Killed {
			t.Verdicts = append(t.Verdicts, mutantstest.Verdict{Path: site.File, Site: mutation.Site{Line: site.Line, Original: site.Original, Replacement: site.Replacement}, Outcome: outcome})
		}
	}
	return t
}
