package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/graph"
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
			if err != nil || !strings.Contains(string(snapshot), `"go_evidence"`) {
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
			plain := mutateCovered(t, "--no-coverage", "--json", greetSource)
			mutant, found := runMutantsAt(t, plain, greetIfLine)
			expected := scope.expectedScope
			if expected == "" {
				expected = scope.name
			}
			if !found || mutant["outcome"] != "killed" || mutant["reused"] != true || mutant["scope"] != expected {
				t.Fatalf("plain run mutant: found=%v outcome=%v reused=%v scope=%v, want reused kill with scope %s", found, mutant["outcome"], mutant["reused"], mutant["scope"], expected)
			}
			if states, _, check := checked(t); states[greetID] != "fresh" {
				t.Fatalf("mutation check after plain reuse: %q, want fresh\n%s", states[greetID], check.stdout)
			}
		})
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
