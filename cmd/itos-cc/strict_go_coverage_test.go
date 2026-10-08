package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStrictGoRunFindsUncoveredExecutableBlockWithoutMutationSite(t *testing.T) {
	source := "main.go"
	moduleRepo(t, map[string]string{
		"go.mod":       "module example.com/strict\n\ngo 1.22\n",
		"main.go":      "package main\n\nfunc Choose(b bool) string { if b { return \"miss\" }; return \"hit\" }\nfunc main() {}\n",
		"main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestChoose(t *testing.T) { if Choose(false) != \"hit\" { t.Fatal(\"Choose\") } }\n",
	})

	o := mutateCovered(t, "--fail-uncovered", "--json", source)
	m := o.json(t)
	p := m.problem("mutation.uncovered-statement")
	wantProblem(t, p, map[string]any{"file": source, "function": "example.com/strict#Choose", "line": float64(3)}, o.stdout)
	if _, has := p["original"]; has {
		t.Errorf("statement problem invents original %v", p["original"])
	}
	if _, has := p["replacement"]; has {
		t.Errorf("statement problem invents replacement %v", p["replacement"])
	}
	if len(m.Files) != 1 || m.Files[0].Killed != 0 || m.Files[0].Survived != 0 || m.Files[0].Uncovered != 0 {
		t.Errorf("strict statement finding changed mutant counts: %+v", m.Files)
	}
	if o.code != 1 {
		t.Errorf("strict uncovered executable block exit %d, want 1\n%s%s", o.code, o.stdout, o.stderr)
	}
}

func TestStrictGoCheckRejectsLegacySnapshotWithoutCoverageEvidenceAndRunsNothing(t *testing.T) {
	dir := moduleRepo(t, map[string]string{
		"go.mod":       "module example.com/strictcheck\n\ngo 1.22\n",
		"main.go":      "package main\n\nfunc Compare(n int) bool { return n > 5 }\nfunc Choose(b bool) string { if b { return \"miss\" }; return \"hit\" }\nfunc main() {}\n",
		"main_test.go": "package main\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestChoose(t *testing.T) {\n\tif Compare(5) { t.Fatal(\"Compare(5)\") }\n\t_ = Choose(false)\n\tif err := os.WriteFile(\"tests-ran\", nil, 0o644); err != nil { t.Fatal(err) }\n}\n",
	})
	if o := mutateCovered(t, "--json", "main.go"); o.code > 1 {
		t.Fatalf("initial run: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	// Exercise the public snapshot format as a legacy snapshot: retain mutation
	// outcomes but remove any independent evidence added by newer versions.
	path := filepath.Join(dir, ".metrics", "mutate", "main.go.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	delete(snapshot, "coverage")
	delete(snapshot, "coverage_evidence")
	units, _ := snapshot["units"].([]any)
	for _, raw := range units {
		unit := raw.(map[string]any)
		delete(unit, "coverage")
		delete(unit, "coverage_evidence")
	}
	legacy, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(legacy, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "tests-ran")
	if err := os.Remove(marker); err != nil {
		// The initial go test may have emitted no marker only if coverage was
		// not measured, which would invalidate this cache-check fixture.
		t.Fatalf("initial coverage did not run the test: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	o := cli(t, "mutation", "check", "--fail-uncovered", "--json", "main.go")
	p := o.json(t).problem("mutation.coverage-missing")
	wantProblem(t, p, map[string]any{"file": "main.go", "function": "example.com/strictcheck#Choose"}, o.stdout)
	if o.code != 1 {
		t.Errorf("legacy strict check exit %d, want 1\n%s%s", o.code, o.stdout, o.stderr)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("cached check ran a test, marker stat error: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("cached check wrote the snapshot")
	}
	if strings.Contains(o.stderr, "coverage ") || strings.Contains(o.stderr, "tests ") {
		t.Errorf("cached check ran a coverage or list command:\n%s", o.stderr)
	}
}

func TestStrictGoNoCoverageIsAUsageConflictEvenWithNoSelection(t *testing.T) {
	inEmptyDir(t)
	o := cli(t, "mutation", "run", "--no-coverage", "--fail-uncovered", "--json")
	wantProblem(t, o.json(t).problem("flags.conflict"), map[string]any{"flag": "--no-coverage"}, o.stdout)
	if o.code != 2 {
		t.Errorf("strict no-coverage exit %d, want 2\n%s%s", o.code, o.stdout, o.stderr)
	}
}
