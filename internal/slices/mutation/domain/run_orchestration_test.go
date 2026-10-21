package mutants_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
	seltest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

const runSource = "package board\nfunc Value(x int) bool { return x > 1 }\n"

type runFixture struct {
	root, path string
	files      map[string]string
	snapshots  *porttest.Snapshots
	inputs     *porttest.Inputs
	edits      *porttest.SourceEdits
	workers    *porttest.Workers
	options    mutants.Options
}

func newRunFixture(t *testing.T) *runFixture {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "board.go")
	files := map[string]string{path: runSource, filepath.Join(root, "go.mod"): "module example.test/board\n", filepath.Join(root, "board_test.go"): "package board\n"}
	relative := map[string]string{}
	for name, text := range files {
		rel, _ := filepath.Rel(root, name)
		relative[rel] = text
	}
	inputs := &porttest.Inputs{Dir: root, Files: files}
	snapshots := &porttest.Snapshots{RootDir: root}
	edits := &porttest.SourceEdits{Files: files}
	command, failure := mutants.ObserveCommand(t.Context(), inputs, mutants.CommandRequest{Path: path})
	if failure != nil {
		t.Fatal(failure)
	}
	workers := &porttest.Workers{Files: files, Tests: map[string]map[string]porttest.Tests{root: {command.String(): {}}}}
	opt := mutants.Options{Root: root, Dir: root, Tree: seltest.TreeOf(root, relative, nil), Surroundings: inputs, Store: snapshots, Source: edits, Toolchain: inputs, Fingerprints: inputs, WorkerFactory: workers, Workers: 1, TimeoutFactor: 10, Tests: func(string) []string { return []string{filepath.Join(root, "board_test.go")} }}
	return &runFixture{root, path, files, snapshots, inputs, edits, workers, opt}
}
func (f *runFixture) run(t *testing.T) []mutants.FileResult {
	t.Helper()
	r, err := mutants.Run(t.Context(), []string{f.path}, f.options)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRunUsesActualPortsAndRetainsFreshOutcomes(t *testing.T) {
	f := newRunFixture(t)
	f.options.Annotate = true
	first := f.run(t)
	if len(first) != 1 || first[0].Ran == 0 || len(f.snapshots.Written) != 1 || len(f.edits.Writes) != 1 || !strings.Contains(f.files[f.path], "itos-cc mutation:") {
		t.Fatalf("first actual run: %+v writes%v edits%v", first, f.snapshots.Written, f.edits.Writes)
	}
	// Actual source bytes without the annotation are unchanged; fingerprints remain independent.
	second := f.run(t)
	if second[0].Ran != 0 || second[0].Reused != first[0].Ran || second[0].Baseline != mutants.BaselineNotRun || len(f.edits.Writes) != 1 {
		t.Fatalf("reuse: %+v edits%v", second, f.edits.Writes)
	}
	f.options.MutateAll = true
	if third := f.run(t); third[0].Ran != first[0].Ran || third[0].Reused != 0 {
		t.Fatalf("rerun: %+v", third)
	}
}
func TestRunRequiresActualBindingsAndForwardsCancellation(t *testing.T) {
	for _, name := range []string{"snapshot", "tree", "surroundings", "tools", "inputs", "workers", "source"} {
		t.Run(name, func(t *testing.T) {
			f := newRunFixture(t)
			f.options.Annotate = true
			switch name {
			case "snapshot":
				f.options.Store = nil
			case "tree":
				f.options.Tree = nil
			case "surroundings":
				f.options.Surroundings = nil
			case "tools":
				f.options.Toolchain = nil
			case "inputs":
				f.options.Fingerprints = nil
			case "workers":
				f.options.WorkerFactory = nil
			case "source":
				f.options.Source = nil
			}
			if _, err := mutants.Run(t.Context(), []string{f.path}, f.options); err == nil {
				t.Fatal("missing binding silently accepted")
			}
			if len(f.snapshots.Written) != 0 || len(f.workers.Opened) != 0 {
				t.Fatal("missing binding did work")
			}
		})
	}
	f := newRunFixture(t)
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("actual parent stopped")
	cancel(cause)
	if _, err := mutants.Run(ctx, []string{f.path}, f.options); !errors.Is(err, cause) {
		t.Fatalf("cancellation cause: %v", err)
	}
	if len(f.inputs.Reads) != 0 || len(f.snapshots.Written) != 0 {
		t.Fatal("cancelled caller did work")
	}
}
func TestRunPreservesBaselineAndPublicationFailureBoundaries(t *testing.T) {
	t.Run("baseline", func(t *testing.T) {
		f := newRunFixture(t)
		for command, facts := range f.workers.Tests[f.root] {
			facts.Baseline = "actual fixture failed"
			f.workers.Tests[f.root][command] = facts
		}
		r := f.run(t)
		if !r[0].BaselineFailed || r[0].Ran != 0 || len(f.snapshots.Written) != 0 {
			t.Fatalf("baseline: %+v writes%v", r, f.snapshots.Written)
		}
	})
	t.Run("snapshot read", func(t *testing.T) {
		f := newRunFixture(t)
		f.options.Annotate = true
		f.snapshots.Failures = map[string]snapport.Failure{"board.go": &snapport.Unreadable{Name: "actual", Reason: "denied"}}
		// Read failures retain original precedence, before workers/publication.
		_, err := mutants.Run(t.Context(), []string{f.path}, f.options)
		var read mutants.SnapshotReadFailure
		if !errors.As(err, &read) || len(f.workers.Opened) != 0 || len(f.edits.Writes) != 0 {
			t.Fatalf("read precedence: %v", err)
		}
	})
}
func TestRunPublicationFailureDoesNotRemoveMovedSnapshotOrAnnotate(t *testing.T) {
	f := newRunFixture(t)
	first := f.run(t)
	old := first[0].Snapshot
	old.File = "old.go"
	f.snapshots.Values = map[string]mutants.Snapshot{"old.go": old}
	f.snapshots.Written = nil
	f.snapshots.WriteFailures = map[string]snapport.Failure{"board.go": &snapport.Unwritable{Name: "mutate/board.go.json", Reason: "read-only fixture state"}}
	f.options.Renamed = func(string) string { return filepath.Join(f.root, "old.go") }
	f.options.Annotate = true
	_, err := mutants.Run(t.Context(), []string{f.path}, f.options)
	var failure *snapport.Unwritable
	if !errors.As(err, &failure) || failure.Reason != "read-only fixture state" {
		t.Fatalf("actual publication failure: %v", err)
	}
	if len(f.snapshots.ReadKeys) < 2 || len(f.snapshots.Removed) != 0 || len(f.snapshots.Written) != 0 || len(f.edits.Writes) != 0 {
		t.Fatalf("publication order: reads%v writes%v remove%v annotation%v", f.snapshots.ReadKeys, f.snapshots.Written, f.snapshots.Removed, f.edits.Writes)
	}
	if _, ok := f.snapshots.Values["old.go"]; !ok {
		t.Fatal("failed publication removed old snapshot")
	}
}

func TestSnapshotAdmissionPrecedesCurrentTypeScriptObservation(t *testing.T) {
	root := t.TempDir()
	key := "board.ts"
	store := &porttest.Snapshots{RootDir: root, Values: map[string]mutants.Snapshot{key: {File: "other.ts"}}}
	// A missing tool binding would fail an admitted record; neither absence nor
	// mismatch needs it. Root/decode failures retain their independent precedence.
	if value, err := mutants.LoadSnapshot(t.Context(), store, key, nil); err != nil || value != nil {
		t.Fatalf("mismatch: %+v %v", value, err)
	}
	delete(store.Values, key)
	if value, err := mutants.LoadSnapshot(t.Context(), store, key, nil); err != nil || value != nil {
		t.Fatalf("absence: %+v %v", value, err)
	}
	store.Values[key] = mutants.Snapshot{File: key, Units: []mutants.UnitResult{{Mutants: []mutants.Mutant{{Outcome: mutants.Killed}}}}}
	if _, err := mutants.LoadSnapshot(t.Context(), store, key, nil); err == nil {
		t.Fatal("admitted record ignored missing tools")
	}
	inputs := &porttest.Inputs{Dir: root, Files: map[string]string{filepath.Join(root, "package.json"): `{"scripts":{"test":"fixture"}}`, filepath.Join(root, key): "export function f(x) { return x > 0; }"}}
	got, err := mutants.LoadSnapshot(t.Context(), store, key, inputs)
	if err != nil || got.Units[0].Mutants[0].Scope != mutants.ScopeAllTests {
		t.Fatalf("legacy own normalization: %+v %v", got, err)
	}
	if store.Values[key].Units[0].Mutants[0].Scope != "" {
		t.Fatal("read mutated independent stored value")
	}
}
func TestRunFollowsOnlyActuallyAbsentSourcesAndMovesAfterPublication(t *testing.T) {
	f := newRunFixture(t)
	first := f.run(t)
	old := filepath.Join(f.root, "old.go")
	value := first[0].Snapshot
	value.File = "old.go"
	f.snapshots.Values = map[string]mutants.Snapshot{"old.go": value}
	f.options.Renamed = func(string) string { return old }
	r := f.run(t)
	if r[0].Reused != first[0].Ran || !reflect.DeepEqual(f.snapshots.Removed, []string{"old.go"}) || f.snapshots.Values["board.go"].File != "board.go" {
		t.Fatalf("follow: %+v removed%v stored%+v", r, f.snapshots.Removed, f.snapshots.Values)
	}
	for _, present := range []string{"directory", "error", "file"} {
		t.Run(present, func(t *testing.T) {
			f := newRunFixture(t)
			old := filepath.Join(f.root, "old.go")
			f.options.Renamed = func(string) string { return old }
			switch present {
			case "directory":
				f.edits.Directories = map[string]bool{old: true}
			case "error":
				f.edits.StatErrors = map[string]error{old: errors.New("denied")}
			case "file":
				f.files[old] = "still present"
			}
			f.run(t)
			if len(f.snapshots.Removed) != 0 {
				t.Fatalf("followed present/error source: %v", f.snapshots.Removed)
			}
		})
	}
}

func TestParseListingKeepsOnlyFirstStdoutIdentity(t *testing.T) {
	got := mutants.ParseListing("  first \t tests/a.py \r\nfirst\twrong.py\n\n second\r\n")
	want := []coverage.Test{{ID: "first", File: "tests/a.py"}, {ID: "second"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("listed: %+v", got)
	}
}
