package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Strict Go coverage tests of features/mutate.feature's strict-go-coverage
// rule. ID-MUT-156 to 159, 162 and 163 hold their old tests whole, the raw
// coverage flag refusals among them (T-101); each test left holds what no
// scenario asserts, beside this guard of an unloaded package's function.
// unused, importing nothing, is the package go list lists last, which
// coverage dropped until bug-11 and so reported coverage-missing; measured
// as any other, at 0%, it is an uncovered-statement failure (ID-MUT-157).
func TestStrictGoReportsExecutableFunctionInUnloadedPackageAsUncovered(t *testing.T) {
	t.Parallel()
	moduleRepo(t, map[string]string{
		"go.mod":           "module example.com/strictunloaded\n\ngo 1.22\n",
		"main.go":          "package main\n\nfunc main() {}\n",
		"unused/unused.go": "package unused\n\nfunc Dormant(flag bool) int { if flag { return 1 }; return 2 }\n",
	})
	o := mutateCovered(t, "--fail-uncovered", "--json", "main.go", "unused/unused.go")
	if o.code != 1 {
		t.Fatalf("unloaded package strict run exit %d, want 1\n%s%s", o.code, o.stdout, o.stderr)
	}
	rules := map[any]bool{}
	for _, problem := range o.json(t).Problems {
		file, _ := problem["file"].(string)
		rules[problem["rule"]] = rules[problem["rule"]] || filepath.ToSlash(file) == "unused/unused.go" && problem["function"] == "example.com/strictunloaded/unused#Dormant"
	}
	if !rules["mutation.uncovered-statement"] || !rules["mutation.uncovered"] || rules["mutation.coverage-missing"] {
		t.Errorf("unloaded executable function was not reported as uncovered statements and mutants alone:\n%s", o.stdout)
	}
}

// @ID-MUT-160. Kept: the marker its test writes is first shown to be
// written, so its absence proves check ran no test, which the scenario cannot
// show (T-101).
func TestStrictGoCheckRejectsLegacySnapshotWithoutCoverageEvidenceAndRunsNothing(t *testing.T) {
	t.Parallel()
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
	o := itosCc(t, "mutation", "check", "--fail-uncovered", "--json", "main.go")
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

// @ID-MUT-161. Kept: a change to the package's own test named by the
// coverage-stale message, then remeasured (T-101).
func TestStrictGoCoverageFingerprintStalesOnModuleTestChange(t *testing.T) {
	t.Parallel()
	dir := moduleRepo(t, map[string]string{
		"go.mod":       "module example.com/strictinputs\n\ngo 1.22\n",
		"main.go":      "package main\n\nfunc Value(flag bool) int { if flag { return 1 }; return 2 }\nfunc main() {}\n",
		"main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestValue(t *testing.T) { if Value(true) != 1 || Value(false) != 2 { t.Fatal(\"Value\") } }\n",
	})
	if o := mutateCovered(t, "--fail-uncovered", "--json", "main.go"); o.code != 0 {
		t.Fatalf("initial strict run: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	writeFile(t, filepath.Join(dir, "main_test.go"), "package main\n\nimport \"testing\"\n\nfunc TestValue(t *testing.T) { if Value(false) != 2 || Value(true) != 1 { t.Fatal(\"Value\") } }\n")
	check := itosCc(t, "mutation", "check", "--fail-uncovered", "--json", "main.go")
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

// @ID-MUT-161. Kept: inputs the scenario does not vary: an added or removed
// source, go.sum and itos-cc.yaml (T-101).
func TestStrictGoCoverageFingerprintTracksModuleAndConfiguredInputs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		edit func(t *testing.T, dir string)
		want string
	}{
		// In a repository a Go file is an input once git tracks it.
		{name: "added source", want: "extra.go", edit: func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "extra.go"), "package main\n\nvar Extra = 1\n")
			gitIn(t, dir, "add", "extra.go")
		}},
		{name: "changed module source", want: "other.go", edit: func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "other.go"), "package main\n\nvar Other = 2\n")
		}},
		{name: "removed module source", want: "other.go", edit: func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, "other.go")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "added module test", want: "other_test.go", edit: func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "other_test.go"), "package main\n\nimport \"testing\"\n\nfunc TestOther(t *testing.T) {}\n")
			gitIn(t, dir, "add", "other_test.go")
		}},
		{name: "module configuration", want: "go.mod", edit: func(t *testing.T, dir string) {
			path := filepath.Join(dir, "go.mod")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, path, string(data)+"\n// coverage input changed\n")
		}},
		{name: "module checksum", want: "go.sum", edit: func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "go.sum"), "example.com/unused v1.0.0 h1:checksum\n")
		}},
		{name: "project configuration", want: "itos-cc.yaml", edit: func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "itos-cc.yaml"), strictFingerprintConfig+"# changed\n")
		}},
		{name: "support file", want: "support:support/dep.txt", edit: func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "support", "dep.txt"), "updated support input\n")
		}},
		{name: "Go build environment", want: "@go-env", edit: func(t *testing.T, _ string) {
			useEnv(t, "GOFLAGS", "-buildvcs=false")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := moduleRepo(t, map[string]string{
				"go.mod":          "module example.com/strictfingerprints\n\ngo 1.22\n",
				"main.go":         "package main\n\nfunc Value(n int) int { return n + 1 }\nfunc main() {}\n",
				"other.go":        "package main\n\nvar Other = 1\n",
				"main_test.go":    "package main\n\nimport \"testing\"\n\nfunc TestValue(t *testing.T) { if Value(1) != 2 { t.Fatal(\"Value\") } }\n",
				"itos-cc.yaml":    strictFingerprintConfig,
				"list.sh":         "#!/bin/sh\nprintf 'TestValue\\tmain_test.go\\n'\n",
				"support/dep.txt": "initial support input\n",
			})
			if o := mutateCovered(t, "--fail-uncovered", "--json", "main.go"); o.code != 0 {
				t.Fatalf("setup strict run: exit %d\n%s%s", o.code, o.stdout, o.stderr)
			}
			tc.edit(t, dir)
			check := itosCc(t, "mutation", "check", "--fail-uncovered", "--json", "main.go")
			var stale map[string]any
			for _, problem := range check.json(t).Problems {
				if problem["rule"] == "mutation.coverage-stale" && problem["function"] == "example.com/strictfingerprints#Value" {
					stale = problem
					break
				}
			}
			if stale == nil || !strings.Contains(stale["message"].(string), tc.want) {
				t.Errorf("%s did not stale evidence with input %q: %v", tc.name, tc.want, check.json(t).Problems)
			}
			if check.code != 1 {
				t.Errorf("%s strict check exit %d, want 1", tc.name, check.code)
			}
		})
	}
}

const strictFingerprintConfig = `mutation:
  tests:
    list: sh list.sh
    run: go test ./... -run '{pattern}'
    ids_pattern: '^Test({ids})$'
    join:
      each: '{id}'
      sep: '|'
    support: ['support/**']
`

// @ID-MUT-160. Kept: another function's evidence of the same module
// refused, where the scenario refuses an unrelated module's (T-101).
func TestStrictGoCheckRejectsPartialOrMisattributedEvidence(t *testing.T) {
	t.Parallel()
	dir := moduleRepo(t, map[string]string{
		"go.mod":       "module example.com/strictownership\n\ngo 1.22\n",
		"main.go":      "package main\n\nfunc First(n int) int { if n > 0 { return 1 }; return 2 }\nfunc Second(n int) int { if n > 0 { return 3 }; return 4 }\nfunc main() {}\n",
		"main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestFunctions(t *testing.T) { if First(1) != 1 || First(0) != 2 || Second(1) != 3 || Second(0) != 4 { t.Fatal(\"functions\") } }\n",
	})
	if o := mutateCovered(t, "--fail-uncovered", "--json", "main.go"); o.code != 0 {
		t.Fatalf("setup strict run: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	path := filepath.Join(dir, ".metrics", "mutate", "main.go.json")
	for _, mode := range []string{"null", "partial", "wrong-owner"} {
		t.Run(mode, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var snapshot map[string]any
			if err := json.Unmarshal(data, &snapshot); err != nil {
				t.Fatal(err)
			}
			units := snapshot["units"].([]any)
			first, second := units[0].(map[string]any), units[1].(map[string]any)
			switch mode {
			case "null":
				second["go_coverage"] = nil
			case "partial":
				evidence := second["go_coverage"].(map[string]any)
				evidence["complete"] = false
				evidence["blocks"] = []any{}
			case "wrong-owner":
				second["go_coverage"] = first["go_coverage"]
			}
			updated, err := json.MarshalIndent(snapshot, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, append(updated, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
			o := itosCc(t, "mutation", "check", "--fail-uncovered", "--json", "main.go")
			var missing bool
			for _, problem := range o.json(t).Problems {
				if problem["rule"] == "mutation.coverage-missing" && problem["function"] == "example.com/strictownership#Second" {
					missing = true
				}
			}
			if !missing || o.code != 1 {
				t.Errorf("%s evidence was accepted: exit %d, problems %v", mode, o.code, o.json(t).Problems)
			}
			// Restore the fixture before the next case.
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err)
			}
		})
	}
}
