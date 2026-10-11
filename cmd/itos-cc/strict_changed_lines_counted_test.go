//go:build !windows

package main

import (
	"os/exec"
	"slices"
	"testing"
)

// The counted run of @ID-MUT-226, in "Rule: With --since,
// --fail-uncovered=lines judges coverage only on the changed lines" in
// features/mutate.feature. Counted mode refuses Windows, so the step builds
// only where it runs; it drives the CLI and reads its --json object as raw
// maps, so it compiles against a product without the feature. It uses the
// Go example of @ID-MUT-225 (strict_changed_lines_test.go), whose function's
// one mutation site is on the old line no test runs: --count 1 always
// selects it, and it is uncovered.

// @ID-MUT-226
func TestChangedLineStrictnessHoldsInACountedRun(t *testing.T) {
	t.Parallel()
	requireCountedPlatform(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	e := changedLinesExamples[0]
	changedLinesRepo(t, e.files)
	edit(t, e.file, e.changed[0], e.changed[1])
	edit(t, e.testFile(), e.changedTest[0], e.changedTest[1])
	commitAll(t, "change the default")

	// And a counted Go run with --count 1 --since <base>
	// --fail-uncovered=lines judges coverage only on the changed lines, as
	// a complete run does
	counted := countedRun(t, "--count", "1", "--workers", "1", "--since", "base", "--fail-uncovered=lines", e.file)
	logRun(t, &counted)
	c := counted.counted(t)
	if len(c.Selected) != 1 || c.Selected[0]["state"] != "uncovered" {
		t.Errorf("the counted changed-line run selected %v, want the old line's one site, uncovered", c.Selected)
	}
	if got := changedLinesFindings(c.Problems); len(got) != 0 || counted.code != 0 {
		t.Errorf("the counted changed-line run: exit %d, findings %q; want 0 and none: neither the old block nor its uncovered mutant is on a changed line", counted.code, got)
	}
	complete := mutateCovered(t, "--since", "base", "--fail-uncovered=lines", "--json", e.file)
	logRun(t, &complete)
	if got, want := changedLinesFindings(c.Problems), changedLinesFindings(rawProblems(t, complete)); !slices.Equal(got, want) || complete.code != counted.code {
		t.Errorf("the counted run: exit %d, findings %q; the complete run: exit %d, findings %q; want the same", counted.code, got, complete.code, want)
	}

	// A counted run with --fail-uncovered alone still judges the whole
	// function, as before.
	plain := countedRun(t, "--count", "1", "--workers", "1", "--since", "base", "--fail-uncovered", e.file)
	logRun(t, &plain)
	want := []string{"mutation.uncovered value.go:7", "mutation.uncovered-statement value.go:7"}
	if got := changedLinesFindings(plain.counted(t).Problems); !slices.Equal(got, want) || plain.code != 1 {
		t.Errorf("the counted whole-function run: exit %d, findings %q; want 1 with %q", plain.code, got, want)
	}

	// The added line no test runs is a changed line: the counted run reports
	// it and fails, as the complete run does.
	edit(t, e.file, e.added[0], e.added[1])
	commitAll(t, "add a branch no test takes")
	added := countedRun(t, "--count", "1", "--workers", "1", "--since", "base", "--fail-uncovered=lines", e.file)
	logRun(t, &added)
	addedComplete := mutateCovered(t, "--since", "base", "--fail-uncovered=lines", "--json", e.file)
	logRun(t, &addedComplete)
	wantAdded := []string{"mutation.uncovered-statement value.go:7"}
	if got := changedLinesFindings(added.counted(t).Problems); !slices.Equal(got, wantAdded) || added.code != 1 {
		t.Errorf("the counted changed-line run after the added line: exit %d, findings %q; want 1 with %q alone", added.code, got, wantAdded)
	}
	if got := changedLinesFindings(rawProblems(t, addedComplete)); !slices.Equal(got, wantAdded) || addedComplete.code != 1 {
		t.Errorf("the complete changed-line run after the added line: exit %d, findings %q; want 1 with %q alone", addedComplete.code, got, wantAdded)
	}
}
