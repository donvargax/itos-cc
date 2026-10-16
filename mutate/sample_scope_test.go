package mutate

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/mutants/porttest"
	snaptest "github.com/donvargax/itos-cc/internal/snapshot/port/porttest"
)

// Decoding accepts a Tests field independently of scope. Only a recorded listed
// scope admits those IDs to the shared scheduler; sampling remains read-only.
func TestSampleIgnoresListedIDsOnNonListedRecordedOutcome(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	path := filepath.Join(root, "board.go")
	files := map[string]string{path: "package board\nfunc Value(x int) bool { return x > 1 }\n", filepath.Join(root, "go.mod"): "module example.test/board\n"}
	for name, text := range files {
		if err := os.WriteFile(name, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	inputs := &porttest.Inputs{Dir: root, Files: files}
	command, failure := mutants.ObserveCommand(t.Context(), inputs, mutants.CommandRequest{Path: path})
	if failure != nil {
		t.Fatal(failure)
	}
	workers := &porttest.Workers{Files: files, Tests: map[string]map[string]porttest.Tests{root: {command.String(): {}}}}
	store := &snaptest.Store{RootDir: root}
	opt := Options{Store: store, Root: root, Dir: root, WorkerFactory: workers, Toolchain: inputs, Fingerprints: inputs, Workers: 1, TimeoutFactor: 10, Log: io.Discard}
	if _, err := Run(t.Context(), []string{path}, opt); err != nil {
		t.Fatal(err)
	}
	snap, err := LoadSnapshotOf(t.Context(), store, "board.go", inputs)
	if err != nil || snap == nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	for i := range snap.Units {
		for j := range snap.Units[i].Mutants {
			snap.Units[i].Mutants[j].Tests = []string{"not-a-listed-scope"}
		}
	}
	if failure := store.Write(t.Context(), SnapshotName("board.go"), SnapshotJSON(*snap)); failure != nil {
		t.Fatal(failure)
	}
	before := store.Snapshots["mutate/board.go.json"]
	parsed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	facts := porttest.Tests{}
	for _, site := range Sites(parse(t, path, string(parsed))) {
		facts.Verdicts = append(facts.Verdicts, porttest.Verdict{Path: "board.go", Site: site, Outcome: Survived})
	}
	workers.Tests[root][command.String()] = facts
	opt.Listed = &Listed{Root: root, Select: func([]string) string {
		return "undeclared selection"
	}}
	result, err := Sample(t.Context(), []string{path}, 100, "fixed", opt)
	if err != nil || len(result.Files) != 1 || len(result.Files[0].Mutants) == 0 {
		t.Fatalf("actual sample: %+v %v", result, err)
	}
	for _, m := range result.Files[0].Mutants {
		if m.Scope != ScopeOwn || m.Outcome != Survived {
			t.Fatalf("recorded scope changed: %+v", m)
		}
	}
	if store.Snapshots["mutate/board.go.json"] != before {
		t.Fatal("sample wrote snapshot")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(parsed) {
		t.Fatalf("sample wrote source: %v", err)
	}
}
