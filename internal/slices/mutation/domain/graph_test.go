package domain_test

import (
	"path/filepath"
	"testing"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

// The graph's judgment needs every observation it was given: a missing one
// is refused, never replaced by the disk.
func TestTheGraphsJudgmentRefusesAMissingObservation(t *testing.T) {
	inputs := &porttest.Inputs{}
	readers := func(string) mutants.MutationSnapshotReader { return &porttest.Snapshots{} }
	for name, g := range map[string]mutants.Graph{
		"snapshots":    {Toolchain: inputs, Fingerprints: inputs},
		"toolchain":    {Snapshots: readers, Fingerprints: inputs},
		"fingerprints": {Snapshots: readers, Toolchain: inputs},
	} {
		if g.Bound() == nil {
			t.Errorf("a judgment without its %s was bound", name)
		}
	}
	if err := (mutants.Graph{Snapshots: readers, Toolchain: inputs, Fingerprints: inputs}).Bound(); err != nil {
		t.Errorf("a judgment with every observation was refused: %v", err)
	}
}

// A file's snapshot is read through the reader of its project's root, as
// check reads it: none is nil, and a root with no reader is refused.
func TestTheGraphReadsASnapshotWhereCheckDoes(t *testing.T) {
	root := t.TempDir()
	reader := &porttest.Snapshots{RootDir: root, Values: map[string]mutants.Snapshot{"a.go": {Version: 1, File: "a.go", Language: "go"}}}
	inputs := &porttest.Inputs{Dir: root}
	g := mutants.Graph{Snapshots: func(top string) mutants.MutationSnapshotReader {
		if top != root {
			return nil
		}
		return reader
	}, Toolchain: inputs, Fingerprints: inputs}
	if snap, err := g.Snapshot(t.Context(), root, "a.go"); err != nil || snap == nil || snap.File != "a.go" {
		t.Errorf("a.go's snapshot: %+v %v", snap, err)
	}
	if snap, err := g.Snapshot(t.Context(), root, "b.go"); err != nil || snap != nil {
		t.Errorf("no snapshot: %+v %v, want none", snap, err)
	}
	if _, err := g.Snapshot(t.Context(), filepath.Join(root, "other"), "a.go"); err == nil {
		t.Error("a root with no reader read a snapshot")
	}
}

// The support files a project lists are hashed as a snapshot records them;
// globs that cannot be read are none.
func TestTheGraphHashesTheListedSupportFiles(t *testing.T) {
	root := t.TempDir()
	inputs := &porttest.Inputs{Dir: root, Files: map[string]string{filepath.Join(root, "testdata", "a.txt"): "a"}}
	g := mutants.Graph{Fingerprints: inputs}
	if got := g.Support(t.Context(), root, []string{"testdata/*.txt"}); len(got) != 1 || got["testdata/a.txt"] == "" {
		t.Errorf("support: %v, want testdata/a.txt's hash", got)
	}
	if got := g.Support(t.Context(), root, []string{"["}); got != nil {
		t.Errorf("support of a bad glob: %v, want none", got)
	}
}

// Each function is judged as check judges it: one its snapshot records with
// its hash and the tests that import its file as they were is fresh, with
// the snapshot's counts; one whose tests changed is stale; one the
// snapshot does not record, or a file with no snapshot, carries nothing.
func TestTheGraphJudgesEachFunctionAsCheckDoes(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.go")
	f := parseExecutionMode(t, path, "package a\nfunc A(x int) bool { return x > 1 }\nfunc B(x int) bool { return x < 1 }\n")
	facts := mutants.GraphFactsOf(f)
	if len(facts.IDs) != 2 || len(facts.Hashes) != 2 || len(facts.Sites[0]) == 0 {
		t.Fatalf("facts: %+v", facts)
	}
	entry := mutants.UnitResult{Namespace: f.Units[0].Namespace, Name: "A", Hash: facts.Hashes[0], Mutants: []mutants.Mutant{}}
	for _, s := range facts.Sites[0] {
		entry.Mutants = append(entry.Mutants, mutants.Mutant{Line: s.Line, Column: s.Column, Offset: s.Offset, Original: s.Original, Replacement: s.Replacement, Outcome: mutants.Killed})
	}
	entry.Killed, entry.Sites = len(entry.Mutants), len(entry.Mutants)
	tests := map[string]string{"a_test.go": "recorded"}
	snap := &mutants.Snapshot{Version: 1, File: "a.go", Language: "go", Tests: tests, Units: []mutants.UnitResult{entry}}
	inputs := &porttest.Inputs{Dir: root}
	g := mutants.Graph{Toolchain: inputs, Fingerprints: inputs}

	got := g.Judge(t.Context(), root, path, snap, facts, tests, nil)
	if want := (mutants.GraphVerdict{Mutated: true, Killed: entry.Killed}); len(got) != 2 || got[0] != want || got[1] != (mutants.GraphVerdict{}) {
		t.Errorf("as recorded: %+v, want A %+v and B nothing", got, want)
	}
	if got := g.Judge(t.Context(), root, path, snap, facts, map[string]string{"a_test.go": "changed"}, nil); !got[0].Mutated || !got[0].Stale {
		t.Errorf("tests changed: %+v, want A stale", got)
	}
	if got := g.Judge(t.Context(), root, path, nil, facts, tests, nil); len(got) != 2 || got[0] != (mutants.GraphVerdict{}) {
		t.Errorf("no snapshot: %+v, want nothing", got)
	}

	// A whole-suite outcome rests on the module's tests too, which the
	// judgment hashes; with no whole-suite evidence recorded it is stale.
	snap.Units[0].Mutants[0].Scope = mutants.ScopeAllTests
	if got := g.Judge(t.Context(), root, path, snap, facts, tests, nil); !got[0].Mutated || !got[0].Stale {
		t.Errorf("a whole-suite outcome without its evidence: %+v, want A stale", got)
	}
}
