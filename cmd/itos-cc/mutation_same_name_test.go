package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The scenarios of "Rule: Functions sharing a name" in
// features/mutate.feature. They use the module of mutate_since_test.go with
// src/setup.go, whose two init functions, both example.com/m/src#init, set
// a variable each that TestSetup checks, so every mutant of both is killed.

const initID = "example.com/m/src#init"

var setupSource = filepath.FromSlash("src/setup.go")

const (
	firstInit  = "func init() {\n\ta = 1 + 1\n}\n"
	secondInit = "func init() {\n\tb = 3 - 1\n}\n"
)

// setupRepo makes the module with src/setup.go and its test, and records
// every mutant of src/setup.go killed.
func setupRepo(t *testing.T) {
	t.Helper()
	boardRepo(t, map[string]string{
		"src/setup.go":      "package board\n\nvar a, b int\n\n" + firstInit + "\n" + secondInit,
		"src/setup_test.go": "package board\n\nimport \"testing\"\n\nfunc TestSetup(t *testing.T) {\n\tif a != 2 || b != 2 {\n\t\tt.Fatal(\"setup\")\n\t}\n}\n",
	})
	if o := mutateRun(t, setupSource); o.code != 0 {
		t.Fatalf("the first run: exit %d, want every mutant killed\n%s%s", o.code, o.stdout, o.stderr)
	}
}

// initLines is the line each init function of src/setup.go starts on, in
// file order.
func initLines(t *testing.T) []int {
	t.Helper()
	data, err := os.ReadFile(inWD(t, setupSource))
	if err != nil {
		t.Fatal(err)
	}
	var out []int
	for i, l := range strings.Split(string(data), "\n") {
		if l == "func init() {" {
			out = append(out, i+1)
		}
	}
	return out
}

// ranLines is the line of each mutant of src/setup.go a --json run ran
// rather than reused.
func ranLines(t *testing.T, o outcome) []int {
	t.Helper()
	var out []int
	for _, m := range o.json(t).file(t, setupSource).mutants(t) {
		if !m.Reused {
			out = append(out, m.Line)
		}
	}
	return out
}

// @ID-MUT-114
func TestFunctionsSharingANameEachReuseTheirOwnResults(t *testing.T) {
	t.Parallel()
	setupRepo(t)

	o := mutateRun(t, "--json", setupSource)
	if f := o.json(t).file(t, setupSource); f.Ran != 0 || f.Killed == 0 {
		t.Errorf("%+v, want every mutant of both init functions reused and none run\n%s", f, o.stderr)
	}
	if ran := ranLines(t, o); len(ran) != 0 {
		t.Errorf("mutants on lines %v ran, want none", ran)
	}
}

// @ID-MUT-115
func TestEditingOneOfTwoFunctionsSharingANameRerunsOnlyIt(t *testing.T) {
	t.Parallel()
	setupRepo(t)
	edit(t, setupSource, "b = 3 - 1", "b = 4 - 2")
	lines := initLines(t)

	o := mutationCheck(t, "--json", setupSource)
	m := o.json(t)
	wantProblem(t, m.problem("mutation.stale"), map[string]any{"file": setupSource, "function": initID, "line": float64(lines[1])}, o.stdout)
	if !slices.Equal(m.rules(), []string{"mutation.stale"}) {
		t.Errorf("problems %v, want only the second init stale", m.Problems)
	}

	run := mutateRun(t, "--json", setupSource)
	ran := ranLines(t, run)
	if len(ran) == 0 {
		t.Fatalf("no mutant ran, want the second init's\n%s", run.stderr)
	}
	for _, l := range ran {
		if l <= lines[0]+2 {
			t.Errorf("a mutant on line %d ran, in the first init, which did not change", l)
		}
	}
}

// @ID-MUT-116
func TestReorderingFunctionsSharingANameChangesNothing(t *testing.T) {
	t.Parallel()
	setupRepo(t)
	edit(t, setupSource, firstInit+"\n"+secondInit, secondInit+"\n"+firstInit)

	if o := mutationCheck(t, "--json", setupSource); o.code != 0 {
		t.Errorf("check: exit %d, want 0: neither init changed\n%s", o.code, o.stdout)
	}
	o := mutateRun(t, "--json", setupSource)
	if ran := ranLines(t, o); len(ran) != 0 {
		t.Errorf("mutants on lines %v ran, want every one reused\n%s", ran, o.stderr)
	}
}
