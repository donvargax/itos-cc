package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The scenarios of "Rule: The mutation group" in features/cli.feature: the
// group mutation, with mutation run, what mutate was, and mutation list,
// what mutate --scan was.

// cli runs itos-cc with args and returns what it printed.
func cli(t *testing.T, args ...string) outcome {
	t.Helper()
	var o outcome
	o.stdout, o.stderr = captured(t, func() { o.code = run(args) })
	return o
}

// inEmptyDir makes an empty temporary directory the working directory, so a
// command that mutates finds nothing to change.
func inEmptyDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	return dir
}

// @ID-CLI-12
func TestTheGroupAloneNamesItsSubcommands(t *testing.T) {
	inEmptyDir(t)
	o := cli(t, "mutation")
	for _, sub := range []string{"usage: itos-cc mutation", "  run ", "  list "} {
		if !strings.Contains(o.stdout, sub) {
			t.Errorf("stdout:\n%s\nwant it to contain %q", o.stdout, sub)
		}
	}
	if o.code != 0 {
		t.Errorf("exit %d, want 0; stderr:\n%s", o.code, o.stderr)
	}
}

// @ID-CLI-13
func TestMutationRunMutates(t *testing.T) {
	// Board#place's one mutant is killed by TestPlace; Board#clear's one
	// mutant survives, as no test calls it.
	boardRepo(t, nil)
	o := cli(t, "mutation", "run", "--no-coverage", "--no-annotate", "--workers", "1", boardSource)
	want := boardSource + ": 1 killed, 1 survived, 0 uncovered (ran 2, reused 0)\n"
	if !strings.Contains(o.stdout, want) {
		t.Fatalf("exit %d, stdout:\n%s\nwant it to say %q; stderr:\n%s", o.code, o.stdout, want, o.stderr)
	}
	o = cli(t, "mutation", "run", "--no-coverage", "--no-annotate", "--workers", "1", "--json", boardSource)
	m := o.json(t)
	var rules []string
	for _, p := range m.Problems {
		rules = append(rules, p["rule"].(string))
	}
	if p := m.problem("mutation.survived"); p == nil || p["function"] != clearID || !slices.Equal(rules, []string{"mutation.survived"}) {
		t.Errorf("problems %v, want one mutation.survived, for %s", m.Problems, clearID)
	}
	if o.code != 1 {
		t.Errorf("exit %d, want 1 for the survivor", o.code)
	}
}

// @ID-CLI-14
func TestMutationListListsTheSitesWithoutRunningTests(t *testing.T) {
	dir := inEmptyDir(t)
	// package.json makes the directory a project, so the namespace is x;
	// its test script would fail any baseline.
	writeFile(t, filepath.Join(dir, "package.json"), `{"scripts": {"test": "exit 1"}}`+"\n")
	writeFile(t, filepath.Join(dir, "x.ts"), "export function place(a: number): boolean {\n  return a > 0;\n}\n")

	o := cli(t, "mutation", "list", "x.ts")
	want := "x.ts:2:12 `>` → `>=` in x#place\nx.ts:2:14 `0` → `1` in x#place\n"
	if o.stdout != want || o.code != 0 {
		t.Errorf("exit %d, stdout:\n%s\nwant exit 0 and:\n%s\nstderr:\n%s", o.code, o.stdout, want, o.stderr)
	}

	o = cli(t, "mutation", "list", "--json", "x.ts")
	var got struct {
		OK    bool             `json:"ok"`
		Sites []map[string]any `json:"sites"`
	}
	if err := json.Unmarshal([]byte(o.stdout), &got); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s\nstderr:\n%s", err, o.stdout, o.stderr)
	}
	wantSites := []map[string]any{
		{"file": "x.ts", "line": 2.0, "column": 12.0, "function": "x#place", "original": ">", "replacement": ">="},
		{"file": "x.ts", "line": 2.0, "column": 14.0, "function": "x#place", "original": "0", "replacement": "1"},
	}
	if !got.OK || len(got.Sites) != len(wantSites) {
		t.Fatalf("stdout:\n%s\nwant ok and the two sites of x.ts", o.stdout)
	}
	for i, s := range got.Sites {
		for k, v := range wantSites[i] {
			if s[k] != v {
				t.Errorf("site %d: %s %v, want %v", i, k, s[k], v)
			}
		}
	}

	// A test command run would print its baseline, and coverage or a
	// snapshot would be written under .metrics.
	if strings.Contains(o.stderr, "baseline") {
		t.Errorf("stderr:\n%s\nwant no test command run", o.stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, ".metrics")); err == nil {
		t.Error(".metrics was written: want no test or coverage command run")
	}
}

// @ID-CLI-15
func TestMutateIsNoLongerACommand(t *testing.T) {
	inEmptyDir(t)
	o := cli(t, "mutate", filepath.FromSlash("src/board.ts"))
	want := "itos-cc: there is no command \"mutate\". Did you mean 'mutation'? Run 'itos-cc --help' for the commands.\n"
	if o.stderr != want {
		t.Errorf("stderr:\n%s\nwant:\n%s", o.stderr, want)
	}
	if o.code != 2 {
		t.Errorf("exit %d, want 2", o.code)
	}
}

// @ID-CLI-16
func TestAnUnknownSubcommandOfTheGroupIsAUsageError(t *testing.T) {
	inEmptyDir(t)
	o := cli(t, "--json", "mutation", "nosuch")
	if p := o.json(t).problem("command.unknown"); p == nil || !strings.Contains(p["command"].(string), "nosuch") {
		t.Errorf("stdout:\n%s\nwant a command.unknown problem naming nosuch", o.stdout)
	}
	if o.code != 2 {
		t.Errorf("--json: exit %d, want 2", o.code)
	}
	o = cli(t, "mutation", "nosuch")
	if !strings.Contains(o.stderr, "run and list") {
		t.Errorf("stderr:\n%s\nwant it to name the subcommands run and list", o.stderr)
	}
	if o.code != 2 {
		t.Errorf("exit %d, want 2", o.code)
	}
}

// @ID-CLI-17
func TestScanIsNotAFlagOfMutationRun(t *testing.T) {
	dir := inEmptyDir(t)
	writeFile(t, filepath.Join(dir, "x.ts"), "export function place(a: number): boolean {\n  return a > 0;\n}\n")
	o := cli(t, "--json", "mutation", "run", "--scan", "x.ts")
	if p := o.json(t).problem("flags.unknown"); p == nil || p["flag"] != "--scan" {
		t.Errorf("stdout:\n%s\nwant a flags.unknown problem with flag --scan", o.stdout)
	}
	if o.code != 2 {
		t.Errorf("exit %d, want 2", o.code)
	}
}
