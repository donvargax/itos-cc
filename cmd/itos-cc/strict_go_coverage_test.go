package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/mutate"
)

// @ID-MUT-156
func TestStrictGoRunFindsUncoveredExecutableBlockWithoutMutationSite(t *testing.T) {
	source := "main.go"
	moduleRepo(t, map[string]string{
		"go.mod":       "module example.com/strict\n\ngo 1.22\n",
		"main.go":      "package main\n\nfunc Compare(n int) bool { return n > 5 }\nfunc Choose(b bool) string { if b { return \"miss\" }; return \"hit\" }\nfunc main() {}\n",
		"main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestChoose(t *testing.T) { if Compare(5) { t.Fatal(\"Compare\") }; if Choose(false) != \"hit\" { t.Fatal(\"Choose\") } }\n",
	})

	o := mutateCovered(t, "--fail-uncovered", "--json", source)
	m := o.json(t)
	p := m.problem("mutation.uncovered-statement")
	wantProblem(t, p, map[string]any{"file": source, "function": "example.com/strict#Choose", "line": float64(4)}, o.stdout)
	if _, has := p["original"]; has {
		t.Errorf("statement problem invents original %v", p["original"])
	}
	if _, has := p["replacement"]; has {
		t.Errorf("statement problem invents replacement %v", p["replacement"])
	}
	if len(m.Files) != 1 || m.Files[0].Killed != 1 || m.Files[0].Survived != 0 || m.Files[0].Uncovered != 0 {
		t.Errorf("strict statement finding changed the one killed mutant's counts: %+v", m.Files)
	}
	if o.code != 1 {
		t.Errorf("strict uncovered executable block exit %d, want 1\n%s%s", o.code, o.stdout, o.stderr)
	}
	snapshotPath := filepath.Join(".metrics", "mutate", "main.go.json")
	before, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	check := cli(t, "mutation", "check", "--fail-uncovered", "--json", source)
	var checkProblem map[string]any
	for _, candidate := range check.json(t).Problems {
		if candidate["rule"] == "mutation.uncovered-statement" && candidate["function"] == "example.com/strict#Choose" {
			checkProblem = candidate
			break
		}
	}
	wantProblem(t, checkProblem, map[string]any{"file": source, "function": "example.com/strict#Choose", "line": float64(4)}, check.stdout)
	if check.code != 1 || strings.Contains(check.stderr, "coverage ") || strings.Contains(check.stderr, "tests ") {
		t.Errorf("cached strict check must return the same verdict without commands, exit=%d\n%s%s", check.code, check.stdout, check.stderr)
	}
	after, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("cached strict check wrote the snapshot")
	}
}

// @ID-MUT-157
func TestStrictGoChecksZeroSiteFunctionsAndExemptsEmptyBodies(t *testing.T) {
	dir := moduleRepo(t, map[string]string{
		"go.mod":       "module example.com/strictzero\n\ngo 1.22\n",
		"main.go":      "package main\n\nfunc Dormant(flag bool) string { if flag { return \"yes\" }; return \"no\" }\nfunc Empty() {}\nfunc CommentsOnly() { // no executable work\n}\nfunc main() {}\n",
		"main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestEmpty(t *testing.T) { Empty(); CommentsOnly() }\n",
	})
	o := mutateCovered(t, "--fail-uncovered", "--json", "main.go")
	if o.code != 1 {
		t.Fatalf("uncovered zero-site function exit %d, want 1\n%s%s", o.code, o.stdout, o.stderr)
	}
	var dormant, empty, comments int
	for _, problem := range o.json(t).Problems {
		if problem["rule"] != "mutation.uncovered-statement" {
			continue
		}
		switch problem["function"] {
		case "example.com/strictzero#Dormant":
			dormant++
		case "example.com/strictzero#Empty":
			empty++
		case "example.com/strictzero#CommentsOnly":
			comments++
		}
	}
	if dormant == 0 || empty != 0 || comments != 0 {
		t.Errorf("strict findings: Dormant=%d Empty=%d CommentsOnly=%d; want uncovered zero-site code only\n%s", dormant, empty, comments, o.stdout)
	}
	path := filepath.Join(dir, ".metrics", "mutate", "main.go.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Units []struct {
			Name     string                     `json:"name"`
			Coverage *mutate.GoCoverageEvidence `json:"go_coverage"`
		} `json:"units"`
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	for _, unit := range snapshot.Units {
		if unit.Name == "Empty" || unit.Name == "CommentsOnly" {
			if unit.Coverage == nil || !unit.Coverage.Complete || len(unit.Coverage.Blocks) != 0 {
				t.Errorf("empty function %s evidence = %+v, want complete empty inventory", unit.Name, unit.Coverage)
			}
		}
	}
}

func TestStrictGoReportsExecutableFunctionInUnloadedPackageAsMissing(t *testing.T) {
	moduleRepo(t, map[string]string{
		"go.mod":           "module example.com/strictunloaded\n\ngo 1.22\n",
		"main.go":          "package main\n\nfunc main() {}\n",
		"unused/unused.go": "package unused\n\nfunc Dormant(flag bool) int { if flag { return 1 }; return 2 }\n",
	})
	o := mutateCovered(t, "--fail-uncovered", "--json", "main.go", "unused/unused.go")
	if o.code != 1 {
		t.Fatalf("unloaded package strict run exit %d, want 1\n%s%s", o.code, o.stdout, o.stderr)
	}
	var found bool
	for _, problem := range o.json(t).Problems {
		if problem["rule"] == "mutation.coverage-missing" && problem["file"] == "unused/unused.go" && problem["function"] == "example.com/strictunloaded/unused#Dormant" {
			found = true
		}
	}
	if !found {
		t.Errorf("unloaded executable function was not reported as missing evidence:\n%s", o.stdout)
	}
}

// @ID-MUT-158
func TestStrictGoKeepsSameLineSpansSeparateAndAttributesClosureBlocks(t *testing.T) {
	dir := moduleRepo(t, map[string]string{
		"go.mod":       "module example.com/strictspans\n\ngo 1.22\n",
		"main.go":      "package main\n\nfunc Branch(call bool) int { if call { return 1 }; return 2 }\nfunc Outer(call bool) int { f := func() int { return 7 }; if call { return f() }; return 0 }\nfunc main() {}\n",
		"main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestBranch(t *testing.T) { if Branch(true) != 1 { t.Fatal(\"Branch\") }; if Outer(false) != 0 { t.Fatal(\"Outer\") } }\n",
	})
	o := mutateCovered(t, "--fail-uncovered", "--json", "main.go")
	if o.code != 1 {
		t.Fatalf("uncovered spans exit %d, want 1\n%s%s", o.code, o.stdout, o.stderr)
	}
	var outerFinding bool
	for _, problem := range o.json(t).Problems {
		if problem["rule"] == "mutation.uncovered-statement" && problem["function"] == "example.com/strictspans#Outer" {
			outerFinding = true
		}
	}
	if !outerFinding {
		t.Errorf("unexecuted function literal block was not attributed to Outer:\n%s", o.stdout)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".metrics", "mutate", "main.go.json"))
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Units []struct {
			Namespace string                     `json:"namespace"`
			Name      string                     `json:"name"`
			Coverage  *mutate.GoCoverageEvidence `json:"go_coverage"`
		} `json:"units"`
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	var branch *mutate.GoCoverageEvidence
	for i := range snapshot.Units {
		if snapshot.Units[i].Name == "Branch" {
			branch = snapshot.Units[i].Coverage
		}
	}
	if branch == nil {
		t.Fatal("Branch has no persisted coverage evidence")
	}
	var covered, uncovered int
	spans := map[string]bool{}
	for _, block := range branch.Blocks {
		spans[block.Span] = true
		if block.Covered {
			covered++
		} else {
			uncovered++
		}
	}
	if len(spans) < 2 || covered == 0 || uncovered == 0 {
		t.Errorf("same-line block inventory has %d spans, %d covered, %d uncovered; want distinct covered and uncovered spans", len(spans), covered, uncovered)
	}
}

// @ID-MUT-160
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
	var p map[string]any
	for _, candidate := range o.json(t).Problems {
		if candidate["rule"] == "mutation.coverage-missing" && candidate["function"] == "example.com/strictcheck#Choose" {
			p = candidate
			break
		}
	}
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

// @ID-MUT-159
func TestStrictGoRunMeasuresMissingEvidenceWhenMutantsAreReusable(t *testing.T) {
	dir := moduleRepo(t, map[string]string{
		"go.mod":       "module example.com/strictreuse\n\ngo 1.22\n",
		"main.go":      "package main\n\nfunc Positive(n int) bool { return n > 0 }\nfunc main() {}\n",
		"main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestPositive(t *testing.T) { if !Positive(1) || Positive(0) { t.Fatal(\"Positive\") } }\n",
	})
	if o := mutateRun(t, "--json", "main.go"); o.code != 0 {
		t.Fatalf("initial non-strict run: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	o := mutateCovered(t, "--fail-uncovered", "--json", "main.go")
	if o.code != 0 {
		t.Fatalf("strict run from fresh reusable mutants: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	if !strings.Contains(o.stderr, "coverage ") || !strings.Contains(o.stderr, "no mutations to test") {
		t.Errorf("strict run must measure missing coverage while reusing all mutants:\n%s", o.stderr)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".metrics", "mutate", "main.go.json"))
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	units, _ := snapshot["units"].([]any)
	if len(units) == 0 || units[0].(map[string]any)["go_coverage"] == nil {
		t.Errorf("strict run failed to persist missing independent evidence:\n%s", data)
	}
}

// @ID-MUT-161
func TestStrictGoCoverageFingerprintStalesOnModuleTestChange(t *testing.T) {
	dir := moduleRepo(t, map[string]string{
		"go.mod":       "module example.com/strictinputs\n\ngo 1.22\n",
		"main.go":      "package main\n\nfunc Value(flag bool) int { if flag { return 1 }; return 2 }\nfunc main() {}\n",
		"main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestValue(t *testing.T) { if Value(true) != 1 || Value(false) != 2 { t.Fatal(\"Value\") } }\n",
	})
	if o := mutateCovered(t, "--fail-uncovered", "--json", "main.go"); o.code != 0 {
		t.Fatalf("initial strict run: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	writeFile(t, filepath.Join(dir, "main_test.go"), "package main\n\nimport \"testing\"\n\nfunc TestValue(t *testing.T) { if Value(false) != 2 || Value(true) != 1 { t.Fatal(\"Value\") } }\n")
	check := cli(t, "mutation", "check", "--fail-uncovered", "--json", "main.go")
	var stale map[string]any
	for _, problem := range check.json(t).Problems {
		if problem["rule"] == "mutation.coverage-stale" && problem["function"] == "example.com/strictinputs#Value" {
			stale = problem
			break
		}
	}
	if stale == nil || !strings.Contains(stale["message"].(string), "main_test.go") {
		t.Fatalf("test input change did not stale independent coverage evidence: %v\n%s", check.json(t).Problems, check.stdout)
	}
	if check.code != 1 {
		t.Errorf("stale strict check exit %d, want 1", check.code)
	}
	if o := mutateCovered(t, "--fail-uncovered", "--json", "main.go"); o.code != 0 {
		t.Fatalf("strict run did not remeasure changed module test input: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
}

// @ID-MUT-163
func TestStrictGoNoCoverageIsAUsageConflictEvenWithNoSelection(t *testing.T) {
	inEmptyDir(t)
	o := cli(t, "mutation", "run", "--no-coverage", "--fail-uncovered", "--json")
	wantProblem(t, o.json(t).problem("flags.conflict"), map[string]any{"flag": "--no-coverage"}, o.stdout)
	if o.code != 2 {
		t.Errorf("strict no-coverage exit %d, want 2\n%s%s", o.code, o.stdout, o.stderr)
	}
}

func TestStrictGoRejectsUnattestedRawCoverageFlags(t *testing.T) {
	moduleRepo(t, map[string]string{
		"go.mod":  "module example.com/strictflags\n\ngo 1.22\n",
		"main.go": "package main\n\nfunc main() {}\n",
	})
	for _, args := range [][]string{
		{"--coverage-report", "coverage.out"},
		{"--use-existing-coverage"},
		{"--coverage-command", "go test -coverprofile=coverage.out"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			command := []string{"mutation", "run", "--fail-uncovered", "--json"}
			command = append(command, args...)
			command = append(command, "main.go")
			o := cli(t, command...)
			wantProblem(t, o.json(t).problem("flags.conflict"), map[string]any{"flag": args[0]}, o.stdout)
			if o.code != 2 {
				t.Errorf("strict raw coverage flag %v exit %d, want 2\n%s%s", args, o.code, o.stdout, o.stderr)
			}
		})
	}
}
