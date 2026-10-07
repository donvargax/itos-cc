package main

import (
	"slices"
	"strings"
	"testing"
)

// The progress scenario of "Rule: Coverage decides which mutants run" in
// features/mutate.feature. It uses the module of mutate_since_test.go, with
// coverage measured: TestPlace kills Board#place's one mutant, on line 7, and
// no test executes Board#clear, so its one mutant is uncovered and never
// runs.

// @ID-MUT-100
func TestProgressLinesBeginItosCc(t *testing.T) {
	boardRepo(t, nil)

	o := mutateCovered(t, boardSource)
	if u := boardUnits(t); u[placeID].Killed != 1 || u[clearID].Uncovered != 1 {
		t.Fatalf("the snapshot: %+v, want place's mutant killed and clear's uncovered\n%s%s", u, o.stdout, o.stderr)
	}
	lines := strings.Split(o.stderr, "\n")
	for _, want := range []string{
		"itos-cc: baseline ",
		"itos-cc: [1/1] " + boardSource + ":7 `>` → `>=` killed (",
	} {
		if !slices.ContainsFunc(lines, func(l string) bool { return strings.HasPrefix(l, want) }) {
			t.Errorf("stderr:\n%s\nwant a line beginning %q", o.stderr, want)
		}
	}
	for _, l := range lines {
		if strings.HasPrefix(l, "mutate: ") {
			t.Errorf("stderr line %q begins \"mutate: \"", l)
		}
	}
	if o.code != 0 {
		t.Errorf("exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}
}
