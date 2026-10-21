package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	mutation "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	snapporttest "github.com/donvargax/itos-cc/internal/snapshot/port/porttest"
)

func TestAnOwnOutcomeOfAWholeSuiteScriptReadsAsTheWholeSuites(t *testing.T) {
	root := t.TempDir()
	for name, text := range map[string]string{
		"package.json": `{"scripts": {"test": "node --test"}}`,
		"src/a.ts":     "export function a(i: number): boolean {\n  return i === 3;\n}\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	store := &snapporttest.Store{RootDir: root, Snapshots: map[string]string{
		"mutate/src/a.ts.json": `{"version": 1, "file": "src/a.ts", "language": "typescript", "units": [{"name": "a", "mutants": [` +
			`{"outcome": "killed"}, {"outcome": "killed", "scope": "make check"}, {"outcome": "survived", "scope": "listed", "tests": ["t"]}]}]}`,
	}}
	snap, err := loadMutationSnapshot(t.Context(), store, "src/a.ts")
	if err != nil || snap == nil {
		t.Fatalf("snapshot: %v, %v", snap, err)
	}
	var scopes []string
	for _, m := range snap.Units[0].Mutants {
		scopes = append(scopes, m.TestScope())
	}
	if want := []string{mutation.ScopeAllTests, "make check", mutation.ScopeListed}; !slices.Equal(scopes, want) {
		t.Errorf("scopes %q, want %q: the own outcome the whole script decided reads as the whole suite's", scopes, want)
	}
	if !mutation.HasBroadOutcome(snap.Units) || snap.Units[0].Mutants[0].SuiteEvidence != nil {
		t.Errorf("the own outcome recorded without evidence: %+v, want a broad outcome with none, stale once", snap.Units[0].Mutants[0])
	}
}
