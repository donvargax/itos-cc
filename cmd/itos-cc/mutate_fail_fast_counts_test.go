//go:build !windows

package main

import (
	"maps"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The scenario of "Rule: A fail-fast file's counts agree with its work" in
// features/mutate.feature, the fail-fast-counts slice. It drives the CLI
// and reads its --json object as raw maps, as the fail-fast-run and
// fail-fast-partial steps do, on ID-MUT-200's blocked-selection fixture.

// @ID-MUT-211
func TestAFileBlockedByAFailingListedSelectionCountsWhatItRanAndReused(t *testing.T) {
	t.Parallel()
	// An earlier run judged shown alone: its mutant survived its own tests
	// and the listed tests killed it. Then quick and label are judged too.
	files := maps.Clone(ffListedFiles)
	files["itos-cc.yaml"] = strings.Replace(files["itos-cc.yaml"], `list: printf "ID-A-01\nID-A-02\n"`,
		`list: printf "ID-A-01\te2e/e2e_test.go\nID-A-02\te2e/e2e_test.go\n"`, 1)
	moduleRepo(t, files)
	edit(t, "main.go", "func shown(n int) bool {\n", "func shown(n int) bool {\n\t// judged\n")
	commitAll(t, "judge shown")
	ff := t.TempDir()
	useEnv(t, "FF_DIR", ff)
	prior := cli(t, "mutation", "run", "--json", "--no-annotate", "--since", "HEAD~1", "--workers", "1")
	logOutcome(t, &prior)
	if shown := ffRecorded(ffSnapshot(t, "main.go"), "shown"); prior.code != 0 || len(shown) != 1 || shown[0]["outcome"] != "killed" {
		t.Fatalf("earlier run: exit %d, shown recorded %v: want 0 and its mutant killed by the listed tests", prior.code, shown)
	}
	snapshot := filepath.Join(".metrics", "mutate", "main.go.json")
	recorded := readText(t, snapshot)
	for _, function := range []string{"quick", "label"} {
		head := "func " + function + "(n int) bool {\n"
		edit(t, "main.go", head, head+"\t// judged\n")
	}
	commitAll(t, "judge quick and label")

	// Every run below starts from the earlier run's snapshot, and the
	// listed tests fail whenever they run without coverage, as the clean
	// baseline of a selection runs them.
	useEnv(t, "FF_FAIL_ALONE", "1")
	run := func(args ...string) outcome {
		t.Helper()
		writeFile(t, snapshot, recorded)
		o := cli(t, append([]string{"mutation", "run", "--no-annotate", "--since", "HEAD~2", "--workers", "1"}, args...)...)
		logOutcome(t, &o)
		return o
	}

	// But a run without --fail-fast reports the same file as before: a
	// guardrail, run first.
	agg := run("--json")
	a := agg.ff(t).file("main.go")
	if _, has := a["state"]; agg.code != 1 || has || a["baseline"] != "failed" || a["ran"] != float64(0) || a["reused"] != float64(0) || len(mutantsOf(a)) != 0 {
		t.Errorf("without --fail-fast: exit %d, main.go %v: want 1, and main.go as before: baseline failed, ran 0, reused 0, no mutant, no state", agg.code, a)
	}
	plain := run()
	if want := "main.go: listed tests its mutants run fail without any mutant; snapshot not updated\n"; plain.code != 1 || !strings.Contains(plain.stdout, want) {
		t.Errorf("without --fail-fast, plain output, exit %d:\n%s\nwant it to hold, as before:\n%s", plain.code, plain.stdout, want)
	}

	// Given a fail-fast run that ran quick's mutant and reused shown's
	// before the failing selection of label's blocked it.
	o := run("--json", "--fail-fast")
	f := o.ff(t)
	m := f.file("main.go")
	if o.code != 1 || f.stopRule() != "tests.selection-failed" || m["state"] != "blocked" || ffMutant(m, "shown")["reused"] != true ||
		ffMutant(m, "quick")["outcome"] != "killed" || !undecided(ffMutant(m, "label"), "blocked") {
		t.Fatalf("exit %d, stop %v, main.go %v: want 1, stopped by label's selection, shown reused, quick killed and label blocked", o.code, f.Stop, m)
	}
	// Then that file reports "ran" 1 and "reused" 1.
	if m["ran"] != float64(1) || m["reused"] != float64(1) {
		t.Errorf("main.go reports ran %v and reused %v, want 1 and 1", m["ran"], m["reused"])
	}
	// And those agree with its completed work count.
	work, _ := m["work"].(map[string]any)
	ran, _ := m["ran"].(float64)
	reused, _ := m["reused"].(float64)
	if work["completed"] != float64(2) || work["blocked"] != float64(1) || ran+reused != work["completed"] {
		t.Errorf("main.go: ran %v, reused %v, work %v: want ran and reused to add up to its 2 completed, with 1 blocked", m["ran"], m["reused"], work)
	}
	// The text report says the same.
	text := run("--fail-fast")
	line := regexp.MustCompile(`(?m)^main\.go: .*$`).FindString(text.stdout)
	if text.code != 1 || !strings.Contains(line, "blocked") || !strings.Contains(line, "ran 1, reused 1") || !strings.Contains(line, "2 completed") {
		t.Errorf("fail-fast plain output, exit %d:\n%s\nwant main.go's line to say it is blocked, with ran 1, reused 1 and 2 completed", text.code, text.stdout)
	}
}
