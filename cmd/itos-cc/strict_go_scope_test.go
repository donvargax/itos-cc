package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func strictScopeRepo(t *testing.T) string {
	t.Helper()
	return moduleRepo(t, map[string]string{
		"go.mod":        "module example.com/strictscope\n\ngo 1.22\n",
		"entry.go":      "package strictscope\n\nfunc Entry(active bool) string { if active { return \"active\" }; return \"inactive\" }\n",
		"entry_test.go": "package strictscope\n\nimport \"testing\"\n\nfunc TestEntry(t *testing.T) { if Entry(true) != \"active\" || Entry(false) != \"inactive\" { t.Fatal(\"Entry\") } }\n",
	})
}

func freshStrictScopeEvidence(t *testing.T) (string, []byte) {
	t.Helper()
	dir := strictScopeRepo(t)
	if o := mutateCovered(t, "--fail-uncovered", "--json", "entry.go"); o.code != 0 {
		t.Fatalf("setup strict run: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	path := filepath.Join(dir, ".metrics", "mutate", "entry.go.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	units, _ := snapshot["units"].([]any)
	if len(units) != 1 || units[0].(map[string]any)["go_coverage"] == nil {
		t.Fatalf("setup strict run did not persist independent function evidence:\n%s", data)
	}
	return dir, data
}

func requireUnsupportedCoverage(t *testing.T, o outcome, filename string) {
	t.Helper()
	var problem map[string]any
	for _, candidate := range o.json(t).Problems {
		if candidate["rule"] == "mutation.coverage-unsupported" {
			problem = candidate
			break
		}
	}
	if problem == nil {
		t.Errorf("no mutation.coverage-unsupported problem, want it to name %s\n%s", filename, o.stdout)
	} else {
		for key, expected := range map[string]any{
			"file": "entry.go", "function": "example.com/strictscope#Entry", "line": float64(3),
		} {
			if problem[key] != expected {
				t.Errorf("unsupported problem %s = %v, want %v\n%s", key, problem[key], expected, o.stdout)
			}
		}
		message, _ := problem["message"].(string)
		if !strings.Contains(message, filename) {
			t.Errorf("unsupported-scope message %q does not name %s", message, filename)
		}
	}
	if o.code != 1 {
		t.Errorf("unsupported strict coverage exit %d, want 1\n%s%s", o.code, o.stdout, o.stderr)
	}
}

// @ID-MUT-164
func TestStrictGoRunRejectsCachedEvidenceWithActiveWorkspace(t *testing.T) {
	dir, _ := freshStrictScopeEvidence(t)
	writeFile(t, filepath.Join(dir, "go.work"), "go 1.22\n\nuse .\n")
	o := mutateCovered(t, "--fail-uncovered", "--json", "entry.go")
	requireUnsupportedCoverage(t, o, "go.work")
}

func TestStrictGoCheckRejectsCachedEvidenceWithActiveWorkspace(t *testing.T) {
	dir, snapshot := freshStrictScopeEvidence(t)
	writeFile(t, filepath.Join(dir, "go.work"), "go 1.22\n\nuse .\n")
	o := cli(t, "mutation", "check", "--fail-uncovered", "--json", "entry.go")
	requireUnsupportedCoverage(t, o, "go.work")
	if strings.Contains(o.stderr, "coverage ") || strings.Contains(o.stderr, "tests ") {
		t.Errorf("strict cached check ran a command:\n%s", o.stderr)
	}
	path := filepath.Join(dir, ".metrics", "mutate", "entry.go.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(snapshot) {
		t.Error("unsupported cached check rewrote its evidence")
	}
}

func TestStrictGoRunAndCheckRejectOutOfInventoryLocalReplacements(t *testing.T) {
	for _, scope := range []struct {
		name        string
		replacement string
		outside     func(dir string) string
	}{
		{name: "external", replacement: "../replacement", outside: func(dir string) string { return filepath.Join(filepath.Dir(dir), "replacement") }},
		{name: "excluded nested module", replacement: "./replacement", outside: func(dir string) string { return filepath.Join(dir, "replacement") }},
	} {
		t.Run(scope.name, func(t *testing.T) {
			dir, _ := freshStrictScopeEvidence(t)
			target := scope.outside(dir)
			writeFile(t, filepath.Join(target, "go.mod"), "module example.com/replacement\n\ngo 1.22\n")
			writeFile(t, filepath.Join(target, "replacement.go"), "package replacement\n\nfunc Value() int { return 1 }\n")
			goMod := "module example.com/strictscope\n\ngo 1.22\n\nreplace example.com/replacement => " + scope.replacement + "\n"
			writeFile(t, filepath.Join(dir, "go.mod"), goMod)
			check := cli(t, "mutation", "check", "--fail-uncovered", "--json", "entry.go")
			requireUnsupportedCoverage(t, check, scope.replacement)
			run := mutateCovered(t, "--fail-uncovered", "--json", "entry.go")
			requireUnsupportedCoverage(t, run, scope.replacement)
		})
	}
}

func TestStrictGoScopeGuardrailsRemainSupported(t *testing.T) {
	dir, _ := freshStrictScopeEvidence(t)
	writeFile(t, filepath.Join(dir, "go.work"), "go 1.22\n\nuse .\n")
	t.Setenv("GOWORK", "off")
	if o := cli(t, "mutation", "check", "--fail-uncovered", "--json", "entry.go"); o.code != 0 {
		t.Errorf("GOWORK=off should keep inventoried module supported: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	t.Setenv("GOWORK", "")
	if o := cli(t, "mutation", "check", "--json", "entry.go"); o.code != 0 {
		t.Errorf("non-strict check should retain workspace behavior: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	repo := strictScopeRepo(t)
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/strictscope\n\ngo 1.22\n\nreplace example.com/replacement => .\n")
	if o := mutateCovered(t, "--fail-uncovered", "--json", "entry.go"); o.code != 0 {
		t.Fatalf("setup same-module replacement run: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	t.Setenv("GOWORK", "off")
	if o := cli(t, "mutation", "check", "--fail-uncovered", "--json", "entry.go"); o.code != 0 {
		t.Errorf("replacement target inside the actual module inventory should remain supported: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
}
