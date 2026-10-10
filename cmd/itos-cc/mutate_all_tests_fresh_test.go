package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/graph"
	"github.com/donvargax/itos-cc/mutate"
	"github.com/donvargax/itos-cc/project"
)

// @ID-MUT-150
func TestBinaryOnlyEndToEndTestChangeMakesBroadScopeKillStale(t *testing.T) {
	for _, scope := range []struct {
		name          string
		args          []string
		expectedScope string
	}{
		{name: "all-tests", args: []string{"--all-tests"}},
		{name: "test-command", args: []string{"--test-command", "go test -count=1 ./..."}},
	} {
		t.Run(scope.name, func(t *testing.T) {
			greetRepo(t, false, false)
			first := append(append([]string{}, scope.args...), "--no-coverage", "--json", greetSource)
			o := mutateCovered(t, first...)
			if o.code > 1 {
				t.Fatalf("initial broad run: exit %d\n%s%s", o.code, o.stdout, o.stderr)
			}
			initial, found := runMutantsAt(t, o, greetIfLine)
			if !found || initial["outcome"] != "killed" {
				t.Fatalf("initial end-to-end outcome: found=%v outcome=%v scope=%v, want killed", found, initial["outcome"], initial["scope"])
			}
			snapshot, err := os.ReadFile(".metrics/mutate/main.go.json")
			if err != nil || !strings.Contains(string(snapshot), `"go_evidence"`) && !strings.Contains(string(snapshot), `"suite_evidence"`) {
				t.Fatalf("initial broad snapshot lacks evidence: %v\n%s", err, snapshot)
			}
			path := "e2e/e2e_test.go"
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			test := string(contents)
			test = strings.Replace(test, `got != "one"`, `got != "one" && got != "0 more"`, 1)
			writeFile(t, path, test)

			check := cli(t, "mutation", "check", "--json", greetSource)
			if check.code != 1 || !strings.Contains(check.stdout, `"mutation.stale"`) || !strings.Contains(check.stdout, path) {
				t.Fatalf("after changing binary-only test: want stale naming %s, exit 1; got exit %d\n%s%s", path, check.code, check.stdout, check.stderr)
			}

			again := append(append([]string{}, scope.args...), "--no-coverage", "--json", greetSource)
			run := mutateCovered(t, again...)
			if run.code > 1 {
				t.Fatalf("rerun after test change: exit %d\n%s%s", run.code, run.stdout, run.stderr)
			}
			mutants, found := runMutantsAt(t, run, greetIfLine)
			if !found || mutants["outcome"] != "survived" || mutants["reused"] == true {
				t.Fatalf("changed test must rerun and let the mutant survive: %+v\n%s%s", mutants, run.stdout, run.stderr)
			}
		})
	}
}

func runMutantsAt(t *testing.T, result outcome, line int) (map[string]any, bool) {
	t.Helper()
	for _, file := range result.rawFiles(t) {
		if file["file"] != greetSource {
			continue
		}
		list, _ := file["mutants"].([]any)
		for _, raw := range list {
			mutant := raw.(map[string]any)
			if mutantLine(mutant) == line && mutant["replacement"] == ">=" {
				return mutant, true
			}
		}
	}
	return nil, false
}

// @ID-MUT-152
func TestPlainRunReusesBroadScopeKillsAndKeepsTheirEvidence(t *testing.T) {
	for _, scope := range []struct {
		name          string
		args          []string
		expectedScope string
	}{
		{name: "all-tests", args: []string{"--all-tests"}},
		{name: "test-command", args: []string{"--test-command", "go test -count=1 ./..."}, expectedScope: "go test -count=1 ./..."},
	} {
		t.Run(scope.name, func(t *testing.T) {
			greetRepo(t, false, false)
			first := append(append([]string{}, scope.args...), "--no-coverage", "--json", greetSource)
			result := mutateCovered(t, first...)
			if result.code > 1 {
				t.Fatalf("initial run: exit %d\n%s%s", result.code, result.stdout, result.stderr)
			}
			initial, initialFound := runMutantsAt(t, result, greetIfLine)
			if !initialFound || initial["outcome"] != "killed" {
				t.Fatalf("initial run mutant: found=%v outcome=%v scope=%v", initialFound, initial["outcome"], initial["scope"])
			}
			beforeEvidence := broadEvidenceAt(t, ".metrics/mutate/main.go.json", greetIfLine)
			plain := mutateCovered(t, "--no-coverage", "--json", greetSource)
			mutant, found := runMutantsAt(t, plain, greetIfLine)
			expected := scope.expectedScope
			if expected == "" {
				expected = scope.name
			}
			if !found || mutant["outcome"] != "killed" || mutant["reused"] != true || mutant["scope"] != expected {
				t.Fatalf("plain run mutant: found=%v outcome=%v reused=%v scope=%v, want reused kill with scope %s", found, mutant["outcome"], mutant["reused"], mutant["scope"], expected)
			}
			afterEvidence := broadEvidenceAt(t, ".metrics/mutate/main.go.json", greetIfLine)
			if !reflect.DeepEqual(beforeEvidence, afterEvidence) {
				t.Fatalf("plain run changed reused broad evidence: before=%+v after=%+v", beforeEvidence, afterEvidence)
			}
			if states, _, check := checked(t); states[greetID] != "fresh" {
				t.Fatalf("mutation check after plain reuse: %q, want fresh\n%s", states[greetID], check.stdout)
			}
		})
	}
}

func broadEvidenceAt(t *testing.T, path string, line int) *mutate.SuiteEvidence {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot mutate.Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	for _, unit := range snapshot.Units {
		for _, mutant := range unit.Mutants {
			if mutant.Line == line && mutant.Replacement == ">=" {
				if mutant.SuiteEvidence == nil {
					t.Fatalf("broad mutant at line %d has no Go freshness evidence", line)
				}
				return mutant.SuiteEvidence
			}
		}
	}
	t.Fatalf("snapshot has no broad mutant at line %d", line)
	return nil
}

// @ID-MUT-153
func TestMixedRecordedScopesKeepTheirOwnFreshnessAndMatchTheGraph(t *testing.T) {
	moduleRepo(t, map[string]string{
		"go.mod":                  "module example.com/mixed\n\ngo 1.22\n",
		"main.go":                 "package main\n\nfunc Own() bool { return true }\nfunc Listed() bool { return false }\nfunc AllTests() bool { return true }\nfunc TestCommand() bool { return false }\nfunc Survivor() bool { return true }\nfunc Excepted() bool { return false }\nfunc main() {}\n",
		"main_test.go":            "package main\n\nimport \"testing\"\n\nfunc TestOwn(t *testing.T) { if !Own() { t.Fatal(\"Own\") } }\n",
		"e2e/support_test.go":     "package e2e\n\nimport \"testing\"\n\nfunc TestSupport(t *testing.T) {}\n",
		"features/listed.feature": "Feature: listed test definition\n",
		"itos-cc.yaml": `mutation:
  tests:
    list: "touch list-ran"
    run: "touch test-ran-{pattern}"
    ids_pattern: "{ids}"
    join:
      each: "{id}"
      sep: ","
    support: ["features/*.feature"]
`,
	})
	if result := mutateCovered(t, "--all-tests", "--no-coverage", "--json", greetSource); result.code > 1 {
		t.Fatalf("initial run: exit %d\n%s%s", result.code, result.stdout, result.stderr)
	}
	snapshotPath := ".metrics/mutate/main.go.json"
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot mutate.Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	byName := map[string]*mutate.Mutant{}
	for i := range snapshot.Units {
		unit := &snapshot.Units[i]
		if len(unit.Mutants) == 0 {
			continue
		}
		if len(unit.Mutants) != 1 {
			t.Fatalf("%s has %d mutants, want one", unit.Name, len(unit.Mutants))
		}
		byName[unit.Name] = &unit.Mutants[0]
	}
	if len(byName) != 6 {
		t.Fatalf("snapshot units %v, want six functions", byName)
	}
	excepted := byName["Excepted"]
	if result := cli(t, "mutation", "except", fmt.Sprintf("main.go:%d:%d", excepted.Line, excepted.Column), "--reason", "equivalent"); result.code != 0 {
		t.Fatalf("except broad survivor: exit %d\n%s%s", result.code, result.stdout, result.stderr)
	}
	byName["Own"].Scope = ""
	byName["Own"].SuiteEvidence = nil
	byName["Listed"].Scope = mutate.ScopeListed
	byName["Listed"].Tests = []string{"ID-A-01"}
	byName["Listed"].SuiteEvidence = nil
	projectRoot := project.Root()
	listedDefinition := filepath.Join(projectRoot, "features", "listed.feature")
	listedFiles, err := mutate.TestHashesUnder(projectRoot, []string{listedDefinition})
	if err != nil {
		t.Fatal(err)
	}
	support, err := mutate.SupportHashes(projectRoot, []string{"features/*.feature"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Listed = map[string]string{"ID-A-01": "features/listed.feature"}
	snapshot.ListedFiles = listedFiles
	snapshot.Support = support
	byName["AllTests"].Scope = mutate.ScopeAllTests
	byName["TestCommand"].Scope = "go test -count=1 ./..."
	byName["Survivor"].Scope = mutate.ScopeAllTests
	byName["Excepted"].Scope = "go test -count=1 ./..."
	data, err = json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshotPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	writeFile(t, "e2e/support_test.go", "package e2e\n\nimport \"testing\"\n\nfunc TestSupport(t *testing.T) { t.Log(1) }\n")
	check := cli(t, "mutation", "check", "--json", greetSource)
	if check.code != 1 {
		t.Fatalf("mutation check: exit %d, want stale broad outcomes\n%s%s", check.code, check.stdout, check.stderr)
	}
	var checked struct {
		Files []struct {
			Functions []struct {
				Function string `json:"function"`
				State    string `json:"state"`
			} `json:"functions"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(check.stdout), &checked); err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, file := range checked.Files {
		for _, function := range file.Functions {
			states[strings.TrimPrefix(function.Function, "example.com/mixed#")] = function.State
		}
	}
	for _, name := range []string{"Own", "Listed"} {
		if states[name] != "fresh" {
			t.Errorf("%s scope is %q, want fresh: %s", name, states[name], check.stdout)
		}
	}
	for _, name := range []string{"AllTests", "TestCommand", "Survivor", "Excepted"} {
		if states[name] != "stale" {
			t.Errorf("%s broad scope is %q, want stale: %s", name, states[name], check.stdout)
		}
	}
	wd, _ := os.Getwd()
	builder, err := graph.NewBuilder([]string{wd})
	if err != nil {
		t.Fatal(err)
	}
	g, _, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	graphStates := map[string]bool{}
	for _, node := range g.Nodes {
		for _, unit := range node.Units {
			if unit.File == greetSource {
				graphStates[unit.Name] = unit.Stale
			}
		}
	}
	for _, name := range []string{"Own", "Listed"} {
		if graphStates[name] {
			t.Errorf("graph marks %s stale while mutation check calls it fresh", name)
		}
	}
	for _, name := range []string{"AllTests", "TestCommand", "Survivor", "Excepted"} {
		if !graphStates[name] {
			t.Errorf("graph marks %s fresh while mutation check calls it stale", name)
		}
	}
}

// @ID-MUT-151
func TestBroadScopeSupportFilesAreHashed(t *testing.T) {
	for _, tc := range []struct{ scope, change string }{
		{"--all-tests", "added"}, {"--all-tests", "changed"}, {"--all-tests", "removed"},
		{"--test-command", "added"}, {"--test-command", "changed"}, {"--test-command", "removed"},
	} {
		t.Run(tc.scope+"/"+tc.change, func(t *testing.T) {
			greetRepo(t, false, false)
			config := strings.Replace(listedFiles["itos-cc.yaml"], "go run ./testdata/list.go", "touch list-ran", 1)
			config = strings.Replace(config, "go test -count=1 ./e2e -args -tests={pattern}", "touch test-ran-{pattern}", 1)
			writeFile(t, "itos-cc.yaml", config+"    support: [\"features/*.feature\"]\n")
			if tc.change != "added" {
				writeFile(t, "features/example.feature", "Feature: first\n")
			}
			args := []string{tc.scope, "--no-coverage", "--json", greetSource}
			if tc.scope == "--test-command" {
				args = []string{"--test-command", "go test -count=1 ./...", "--no-coverage", "--json", greetSource}
			}
			if o := mutateCovered(t, args...); o.code > 1 {
				t.Fatalf("initial run: exit %d\n%s%s", o.code, o.stdout, o.stderr)
			}
			switch tc.change {
			case "added":
				writeFile(t, "features/example.feature", "Feature: added\n")
			case "changed":
				writeFile(t, "features/example.feature", "Feature: changed\n")
			case "removed":
				if err := os.Remove("features/example.feature"); err != nil {
					t.Fatal(err)
				}
			}
			check := cli(t, "mutation", "check", "--json", greetSource)
			if check.code != 1 || !strings.Contains(check.stdout, `"mutation.stale"`) || !strings.Contains(check.stdout, "features/example.feature") {
				t.Fatalf("support %s: want stale naming features/example.feature, got exit %d\n%s%s", tc.change, check.code, check.stdout, check.stderr)
			}
			if _, err := os.Stat("list-ran"); !os.IsNotExist(err) {
				t.Fatalf("mutation check ran the list command (stat error %v)", err)
			}
			if matches, err := filepath.Glob("test-ran-*"); err != nil || len(matches) != 0 {
				t.Fatalf("mutation check ran tests: %v (glob error %v)", matches, err)
			}
		})
	}
}

// @ID-MUT-155
func TestGoBroadScopeIncludesBuildTaggedTestsButExcludesNestedModules(t *testing.T) {
	greetRepo(t, false, false)
	if o := mutateCovered(t, "--all-tests", "--no-coverage", "--json", greetSource); o.code > 1 {
		t.Fatalf("initial run: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	data, err := os.ReadFile(".metrics/mutate/main.go.json")
	if err != nil {
		t.Fatal(err)
	}
	var legacy map[string]any
	if err := json.Unmarshal(data, &legacy); err != nil {
		t.Fatal(err)
	}
	for _, rawUnit := range legacy["units"].([]any) {
		unit := rawUnit.(map[string]any)
		for _, rawMutant := range unit["mutants"].([]any) {
			delete(rawMutant.(map[string]any), "go_evidence")
			delete(rawMutant.(map[string]any), "suite_evidence")
		}
	}
	data, err = json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".metrics/mutate/main.go.json", data, 0o600); err != nil {
		t.Fatal(err)
	}
	states, stale, check := checked(t)
	if states[greetID] != "stale" || !strings.Contains(stale[greetID], "whole-suite evidence") {
		t.Fatalf("legacy broad result: %q, want stale for missing evidence\n%s", states[greetID], check.stdout)
	}
	refresh := mutateCovered(t, "--all-tests", "--no-coverage", "--json", greetSource)
	if refresh.code > 1 {
		t.Fatalf("refresh legacy evidence: exit %d\n%s%s", refresh.code, refresh.stdout, refresh.stderr)
	}
	if mutant, found := runMutantsAt(t, refresh, greetIfLine); !found || mutant["reused"] == true {
		t.Fatalf("legacy mutant: found=%v reused=%v, want rerun", found, mutant["reused"])
	}
	writeFile(t, "tagged_test.go", "//go:build freshness_test\n\npackage main\n\nimport \"testing\"\n\nfunc TestTagged(t *testing.T) {}\n")
	check = cli(t, "mutation", "check", "--json", greetSource)
	if check.code != 1 || !strings.Contains(check.stdout, "tagged_test.go") {
		t.Fatalf("build-tagged test: want stale naming tagged_test.go, got exit %d\n%s%s", check.code, check.stdout, check.stderr)
	}
	if o := mutateCovered(t, "--all-tests", "--no-coverage", "--json", greetSource); o.code > 1 {
		t.Fatalf("refresh build-tagged test: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	writeFile(t, "nested/go.mod", "module example.com/greet/nested\n\ngo 1.22\n")
	writeFile(t, "nested/nested_test.go", "package nested\n\nimport \"testing\"\n\nfunc TestNested(t *testing.T) {}\n")
	states, _, check = checked(t)
	if states[greetID] != "fresh" {
		t.Fatalf("nested module test changed outer module result: %q, want fresh\n%s", states[greetID], check.stdout)
	}
}

// @ID-MUT-154
func TestPartialRunKeepsUnjudgedBroadOutcomesStale(t *testing.T) {
	dir := greetRepo(t, false, false)
	base, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	if result := mutateCovered(t, "--all-tests", "--no-coverage", "--json", greetSource); result.code > 1 {
		t.Fatalf("initial broad run: exit %d\n%s%s", result.code, result.stdout, result.stderr)
	}
	writeFile(t, "e2e/suite_test.go", "package e2e\n\nimport \"testing\"\n\nfunc TestSuiteSupport(t *testing.T) {}\n")
	gitIn(t, dir, "add", "e2e/suite_test.go")
	gitIn(t, dir, "commit", "-m", "change module tests")
	source, err := os.ReadFile(greetSource)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(source), "n*2", "n*3", 1)
	if updated == string(source) {
		t.Fatal("main.go has no n*2 expression to change")
	}
	writeFile(t, greetSource, updated)
	gitIn(t, dir, "add", greetSource)
	gitIn(t, dir, "commit", "-m", "change main")
	partial := mutateCovered(t, "--all-tests", "--no-coverage", "--since", strings.TrimSpace(string(base)), "--json", greetSource)
	if partial.code > 1 {
		t.Fatalf("partial broad run: exit %d\n%s%s", partial.code, partial.stdout, partial.stderr)
	}
	var partialResult struct {
		Files []struct {
			Judged  []string `json:"judged"`
			Mutants []struct {
				Function string `json:"function"`
				Reused   bool   `json:"reused"`
			} `json:"mutants"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(partial.stdout), &partialResult); err != nil {
		t.Fatal(err)
	}
	mainID := "example.com/greet#main"
	judgedMain, reranMain := false, false
	for _, file := range partialResult.Files {
		judgedMain = slices.Contains(file.Judged, mainID)
		for _, mutant := range file.Mutants {
			if mutant.Function == mainID && !mutant.Reused {
				reranMain = true
			}
			if mutant.Function == mainID && mutant.Reused {
				t.Errorf("judged main mutant was reused: %+v", mutant)
			}
		}
	}
	if !judgedMain || !reranMain {
		t.Fatalf("partial run judged main=%v and reran main=%v, want both\n%s", judgedMain, reranMain, partial.stdout)
	}
	states, _, check := checked(t)
	if states[greetID] != "stale" {
		t.Fatalf("unjudged greet is %q, want stale\n%s", states[greetID], check.stdout)
	}
	wd, _ := os.Getwd()
	builder, err := graph.NewBuilder([]string{wd})
	if err != nil {
		t.Fatal(err)
	}
	g, _, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range g.Nodes {
		for _, unit := range node.Units {
			if unit.File == greetSource && unit.Namespace+"#"+unit.Name == greetID && !unit.Stale {
				t.Errorf("graph's %s is not stale as mutation check calls it", greetID)
			}
		}
	}
	sample := cli(t, "mutation", "sample", "--count", "100", "--seed", "all-tests-freshness", "--json", greetSource)
	if strings.Contains(sample.stdout, greetID) {
		t.Fatalf("mutation sample included stale %s:\n%s%s", greetID, sample.stdout, sample.stderr)
	}
}
