package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The scenarios of "Rule: Results go stale when their tests change" in
// features/mutate.feature. Most use the module of mutate_since_test.go, with
// killedTests, so a first run kills every mutant of src/board.go; then a
// test that imports it changes, or goes, or the snapshot loses its tests.

// recordedTests is the "tests" of rel's snapshot, read as the file holds it,
// or nil when the snapshot has no such key.
func recordedTests(t *testing.T, rel string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(inWD(t, filepath.Join(".metrics", "mutate", filepath.FromSlash(rel)+".json")))
	if err != nil {
		t.Fatalf("no snapshot of %s: %v", rel, err)
	}
	var snap map[string]json.RawMessage
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("the snapshot of %s: %v\n%s", rel, err, data)
	}
	raw, ok := snap["tests"]
	if !ok {
		return nil
	}
	var tests map[string]string
	if err := json.Unmarshal(raw, &tests); err != nil {
		t.Fatalf("\"tests\" of %s's snapshot: %v\n%s", rel, err, raw)
	}
	return tests
}

// sha256Of is the hex SHA-256 of the test's directory's file rel.
func sha256Of(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(inWD(t, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// wantTests fails t unless rel's snapshot records exactly the hashes of
// tests, each of them as it is now.
func wantTests(t *testing.T, rel string, tests ...string) {
	t.Helper()
	got := recordedTests(t, rel)
	if got == nil {
		t.Fatalf("%s's snapshot records no \"tests\"", rel)
	}
	want := map[string]string{}
	for _, name := range tests {
		want[name] = sha256Of(t, name)
	}
	if !maps.Equal(got, want) {
		t.Errorf("%s's snapshot records the tests %v, want %v", rel, got, want)
	}
}

// killedRun runs mutation run over src/board.go with killedTests in place,
// which kills every mutant.
func killedRun(t *testing.T) {
	t.Helper()
	if o := mutateRun(t, boardSource); o.code != 0 {
		t.Fatalf("the first run: exit %d, want every mutant killed\n%s%s", o.code, o.stdout, o.stderr)
	}
}

// changeBoardTest changes src/board_test.go without changing what it tests.
func changeBoardTest(t *testing.T) {
	t.Helper()
	appendTo(t, filepath.FromSlash("src/board_test.go"), "\n// TestClear checks clear.\n")
}

// reusedOf is the mutants of function that a --json run took from the
// snapshot, of all those it lists.
func reusedOf(t *testing.T, o outcome, function string) (reused, listed int) {
	t.Helper()
	for _, m := range o.json(t).file(t, boardSource).mutants(t) {
		if m.Function != function {
			continue
		}
		listed++
		if m.Reused {
			reused++
		}
	}
	return reused, listed
}

// @ID-MUT-65
func TestTheSnapshotRecordsTheTestsThatImportTheFile(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	dir := t.TempDir()
	for name, text := range map[string]string{
		"src/board.ts":      "export class Board {\n  place(i: number): boolean {\n    return i > 5;\n  }\n}\n",
		"src/board.test.ts": "import { Board } from \"./board\";\n\ntest(\"place\", () => {\n  expect(new Board().place(6)).toBe(true);\n});\n",
		"src/other.ts":      "export function other(a: number): boolean {\n  return a > 0;\n}\n",
		"src/other.test.ts": "import { other } from \"./other\";\n\ntest(\"other\", () => {\n  expect(other(1)).toBe(true);\n});\n",
	} {
		writeFile(t, filepath.Join(dir, name), text)
	}
	useDir(t, dir)

	// No test runner is needed to see what the snapshot records: a command
	// that always passes lets every mutant survive.
	board := filepath.FromSlash("src/board.ts")
	o := mutateRun(t, "--test-command", "go version", board)
	if _, err := os.Stat(inWD(t, filepath.Join(".metrics", "mutate", "src", "board.ts.json"))); err != nil {
		t.Fatalf("no snapshot of %s: exit %d\n%s%s", board, o.code, o.stdout, o.stderr)
	}
	tests := recordedTests(t, "src/board.ts")
	if _, ok := tests["src/other.test.ts"]; ok {
		t.Errorf("the snapshot records src/other.test.ts, which does not import src/board.ts: %v", tests)
	}
	wantTests(t, "src/board.ts", "src/board.test.ts")
}

// @ID-MUT-66
func TestChangingATestThatImportsTheFileRerunsItsKills(t *testing.T) {
	boardRepo(t, killedTests)
	killedRun(t)
	changeBoardTest(t)

	o := mutateRun(t, "--json", boardSource)
	reused, listed := reusedOf(t, o, placeID)
	if listed == 0 {
		t.Fatalf("no mutant of %s listed\n%s", placeID, o.stdout)
	}
	if reused != 0 {
		t.Errorf("%d of %d mutants of %s reused, want every one run again: src/board_test.go changed",
			reused, listed, placeID)
	}
}

// @ID-MUT-67
func TestDeletingATestThatImportsTheFileRerunsItsKills(t *testing.T) {
	boardRepo(t, killedTests)
	killedRun(t)
	if err := os.Remove(inWD(t, filepath.FromSlash("src/board_test.go"))); err != nil {
		t.Fatal(err)
	}

	o := mutateRun(t, "--json", boardSource)
	reused, listed := reusedOf(t, o, placeID)
	if listed == 0 {
		t.Fatalf("no mutant of %s listed\n%s", placeID, o.stdout)
	}
	if reused != 0 {
		t.Errorf("%d of %d mutants of %s reused, want none: the test that killed them is gone",
			reused, listed, placeID)
	}
}

// @ID-MUT-68
func TestMutationCheckCallsResultsStaleWhenTheirTestsChanged(t *testing.T) {
	boardRepo(t, killedTests)
	killedRun(t)
	changeBoardTest(t)

	o := mutationCheck(t, "--json", boardSource)
	m := o.json(t)
	var stale []string
	for _, p := range m.Problems {
		if p["rule"] != "mutation.stale" {
			t.Errorf("problem %v, want only mutation.stale", p)
			continue
		}
		stale = append(stale, p["function"].(string))
		if msg, _ := p["message"].(string); !strings.Contains(msg, "src/board_test.go") {
			t.Errorf("message %q, want it to name src/board_test.go", msg)
		}
	}
	if !slices.Equal(stale, []string{placeID, clearID}) {
		t.Errorf("stale %q, want each function of %s\n%s", stale, boardSource, o.stdout)
	}
	if o.code != 1 {
		t.Errorf("exit %d, want 1", o.code)
	}
}

// @ID-MUT-69
func TestASnapshotThatRecordsNoTestsIsStale(t *testing.T) {
	boardRepo(t, killedTests)
	killedRun(t)
	// As written before snapshots recorded tests.
	name := inWD(t, filepath.Join(".metrics", "mutate", "src", "board.go.json"))
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var snap map[string]json.RawMessage
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatal(err)
	}
	delete(snap, "tests")
	old, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, name, string(old)+"\n")

	o := mutationCheck(t, "--json", boardSource)
	if rules := o.json(t).rules(); !slices.Equal(rules, []string{"mutation.stale", "mutation.stale"}) || o.code != 1 {
		t.Errorf("exit %d, problems %v, want place and clear stale and exit 1\n%s", o.code, rules, o.stdout)
	}

	o = mutateRun(t, "--json", boardSource)
	for _, m := range o.json(t).file(t, boardSource).mutants(t) {
		if m.Reused {
			t.Errorf("mutant %+v reused, want none: the snapshot records no tests", m)
		}
	}
}

// @ID-MUT-70
func TestAGoFilesTestsAreItsPackagesAndThoseOfPackagesThatImportIt(t *testing.T) {
	boardRepo(t, map[string]string{
		"app/app.go":      "package app\n\nimport board \"example.com/m/src\"\n\n// New makes a board.\nfunc New() *board.Board {\n\treturn &board.Board{}\n}\n",
		"app/app_test.go": "package app\n\nimport \"testing\"\n\nfunc TestNew(t *testing.T) {\n\tif New() == nil {\n\t\tt.Fatal(\"no board\")\n\t}\n}\n",
	})

	o := mutateRun(t, boardSource)
	if _, err := os.Stat(inWD(t, filepath.Join(".metrics", "mutate", "src", "board.go.json"))); err != nil {
		t.Fatalf("no snapshot of %s: exit %d\n%s%s", boardSource, o.code, o.stdout, o.stderr)
	}
	wantTests(t, "src/board.go", "src/board_test.go", "app/app_test.go")
}
