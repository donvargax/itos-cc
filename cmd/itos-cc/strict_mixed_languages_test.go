package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The scenario of "Rule: Language parity with Go for strict coverage and
// whole-suite freshness" in features/mutate.feature that the
// strict-mixed-languages slice holds. Its steps drive the CLI and read its
// --json object and the snapshots under .metrics as raw JSON. The
// TypeScript files are plain files next to a tiny Go module: their one
// function has no mutation site, so no TypeScript tool is ever needed, and
// strict Go coverage is the only thing that could judge them.

// The module lives in gomod/, beneath the repository root, which has no
// go.mod: web/label.ts has none above it, gomod/web/label.ts has the
// module's. Entry's blocks are all covered, Dormant's one block is not, so
// strict Go coverage has one finding to make about the Go file, and
// neither function has a mutation site.
var mixedFiles = map[string]string{
	"gomod/go.mod":        "module example.com/mixed\n\ngo 1.22\n",
	"gomod/entry.go":      "package mixed\n\nfunc Entry(active bool) string { if active { return \"active\" }; return \"inactive\" }\n\nfunc Dormant() string { return \"dormant\" }\n",
	"gomod/entry_test.go": "package mixed\n\nimport \"testing\"\n\nfunc TestEntry(t *testing.T) { if Entry(true) != \"active\" || Entry(false) != \"inactive\" { t.Fatal(\"Entry\") } }\n",
	"web/label.ts":        mixedTypeScript,
}

const (
	mixedGo          = "gomod/entry.go"
	mixedOutside     = "web/label.ts"       // no go.mod above it
	mixedBeneath     = "gomod/web/label.ts" // beneath the Go module
	mixedTypeScript  = "export function label(name: string): string {\n  return name;\n}\n"
	mixedGoMeasuring = "go test -count=1 -covermode=set"
)

// mixedFindings is each problem of o naming file, as "rule function:line",
// sorted, with file compared in slash form.
func mixedFindings(t *testing.T, o outcome, file string) []string {
	t.Helper()
	var out []string
	for _, p := range o.json(t).Problems {
		named, _ := p["file"].(string)
		if filepath.ToSlash(named) != file {
			continue
		}
		line, _ := p["line"].(float64)
		out = append(out, fmt.Sprintf("%v %v:%d", p["rule"], p["function"], int(line)))
	}
	slices.Sort(out)
	return out
}

// requireTypeScriptUnjudged fails when o reports a problem about ts that
// strict Go coverage makes, or any problem that is an error rather than a
// verdict: an internal failure, as the lack of a go.mod above ts was.
func requireTypeScriptUnjudged(t *testing.T, o outcome, ts string) {
	t.Helper()
	for _, p := range o.json(t).Problems {
		rule, _ := p["rule"].(string)
		named, _ := p["file"].(string)
		if rule == "internal" {
			t.Errorf("%s in the selection is an error: %v\n%s", ts, p["message"], o.stdout)
			continue
		}
		if filepath.ToSlash(named) == ts && (strings.HasPrefix(rule, "mutation.coverage-") || rule == "mutation.uncovered-statement") {
			t.Errorf("%s is judged for Go coverage evidence: %s %v\n%s", ts, rule, p["message"], o.stdout)
		}
	}
	if o.code == 70 || o.code == 3 {
		t.Errorf("%s in the selection: exit %d\n%s%s", ts, o.code, o.stdout, o.stderr)
	}
}

// requireGoJudgedAsBefore fails unless o judges the Go file as the Go-only
// check alone did: the same findings and the same exit code.
func requireGoJudgedAsBefore(t *testing.T, o, goOnly outcome, what string) {
	t.Helper()
	got, want := mixedFindings(t, o, mixedGo), mixedFindings(t, goOnly, mixedGo)
	if !slices.Equal(got, want) {
		t.Errorf("%s judges %s %q, want %q as the Go-only check does\n%s%s", what, mixedGo, got, want, o.stdout, o.stderr)
	}
	if o.code != goOnly.code {
		t.Errorf("%s exit %d, want %d as the Go-only check\n%s%s", what, o.code, goOnly.code, o.stdout, o.stderr)
	}
}

// requireCompleteGoEvidence fails unless the Go file's snapshot records
// complete measured coverage evidence for both its functions.
func requireCompleteGoEvidence(t *testing.T, dir string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".metrics", "mutate", filepath.FromSlash(mixedGo)+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Units []struct {
			Name     string         `json:"name"`
			Coverage map[string]any `json:"go_coverage"`
		} `json:"units"`
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	complete := map[string]bool{}
	for _, unit := range snapshot.Units {
		complete[unit.Name] = unit.Coverage != nil && unit.Coverage["complete"] == true
	}
	if !complete["Entry"] || !complete["Dormant"] {
		t.Fatalf("%s's snapshot has no complete Go evidence for Entry and Dormant:\n%s", mixedGo, data)
	}
}

// @ID-MUT-203
func TestStrictGoCoverageIgnoresTheOtherLanguagesOfAMixedSelection(t *testing.T) {
	t.Parallel()
	// Given a project with a Go module and a TypeScript file that has no
	// go.mod above it
	dir := moduleRepo(t, mixedFiles)
	for d := filepath.Dir(filepath.Join(dir, filepath.FromSlash(mixedOutside))); ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			t.Fatalf("setup: %s has a go.mod above it in %s", mixedOutside, d)
		}
		if filepath.Dir(d) == d {
			break
		}
	}

	// And the Go functions have fresh complete coverage evidence
	if o := mutateCovered(t, "--fail-uncovered", "--json", mixedGo); o.code != 1 || len(mixedFindings(t, o, mixedGo)) != 1 {
		t.Fatalf("setup strict run: exit %d, want 1 for Dormant's uncovered block alone\n%s%s", o.code, o.stdout, o.stderr)
	}
	requireCompleteGoEvidence(t, dir)
	goOnly := cli(t, "mutation", "check", "--fail-uncovered", "--json", mixedGo)
	if findings := mixedFindings(t, goOnly, mixedGo); goOnly.code != 1 || len(findings) != 1 ||
		!strings.HasPrefix(findings[0], "mutation.uncovered-statement example.com/mixed#Dormant:") {
		t.Fatalf("setup Go-only check: exit %d findings %q, want Dormant's uncovered block alone\n%s%s", goOnly.code, findings, goOnly.stdout, goOnly.stderr)
	}

	// When I run "itos-cc mutation check --fail-uncovered --json" over both
	// files
	mixed := cli(t, "mutation", "check", "--fail-uncovered", "--json", mixedGo, mixedOutside)
	// Then no TypeScript function is reported as mutation.coverage-missing
	// or as an error
	requireTypeScriptUnjudged(t, mixed, mixedOutside)
	// And the Go functions are judged as before
	requireGoJudgedAsBefore(t, mixed, goOnly, "the mixed check")

	// When a TypeScript file sits beneath the Go module's directory
	writeFile(t, filepath.Join(dir, filepath.FromSlash(mixedBeneath)), mixedTypeScript)
	beneath := cli(t, "mutation", "check", "--fail-uncovered", "--json", mixedGo, mixedBeneath)
	// Then it is still not judged for Go coverage evidence
	requireTypeScriptUnjudged(t, beneath, mixedBeneath)
	requireGoJudgedAsBefore(t, beneath, goOnly, "the check with a TypeScript file beneath the module")

	// And a strict mutation run over a mixed selection can reuse fresh Go
	// evidence instead of measuring every time
	selection := []string{"--fail-uncovered", "--json", mixedGo, mixedOutside, mixedBeneath}
	first := mutateCovered(t, selection...)
	requireTypeScriptUnjudged(t, first, mixedOutside)
	requireTypeScriptUnjudged(t, first, mixedBeneath)
	requireGoJudgedAsBefore(t, first, goOnly, "the first mixed strict run")
	again := mutateCovered(t, selection...)
	requireTypeScriptUnjudged(t, again, mixedOutside)
	requireTypeScriptUnjudged(t, again, mixedBeneath)
	requireGoJudgedAsBefore(t, again, goOnly, "the second mixed strict run")
	if strings.Contains(again.stderr, mixedGoMeasuring) || strings.Contains(again.stderr, "itos-cc: coverage") {
		t.Errorf("the second mixed strict run measured coverage again instead of reusing fresh Go evidence:\n%s", again.stderr)
	}
	requireCompleteGoEvidence(t, dir)
}
