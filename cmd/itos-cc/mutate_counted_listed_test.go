package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	mutation "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// The scenarios of "Rule: Fresh counted mutation judges a bounded committed
// selection without claiming full proof" in features/mutation.feature that the
// mutation-counted-listed slice holds, whose holders still assert what their
// scenarios do not. They use the module of mutate_listed_test.go, whose
// binary its e2e harness tests by listed test ID, through selectionRepo,
// which makes the harness record the SHA-256 of the main.go each of its runs
// builds. Like the other counted steps they drive the CLI and read the
// --json object as raw maps, and each focused case selects one or two sites,
// so a run holds at most two mutant trials.

// countedListedRepo is selectionRepo with a.feature, the file the list
// command names for ID-A-01, committed: counted preparation admits only a
// listed test file the commit holds.
func countedListedRepo(t *testing.T, changes ...[3]string) (string, string) {
	t.Helper()
	record, main := selectionRepo(t, changes...)
	dir := wd(t)
	writeFile(t, filepath.Join(dir, "a.feature"), "Feature: one\n")
	gitIn(t, dir, "add", "a.feature")
	gitIn(t, dir, "commit", "-qm", "a.feature")
	return record, main
}

// at is the site of greet's main.go on line whose original text is original.
func at(line int, original string) func(mutation.FreshCandidate) bool {
	return func(c mutation.FreshCandidate) bool { return c.Site.Line == line && c.Site.Original == original }
}

// selecting is a seedSelecting predicate: the selection is, in order, the
// sites wants accept, and the candidates are kept in *got.
func selecting(got *[]mutation.FreshCandidate, wants ...func(mutation.FreshCandidate) bool) func([]mutation.FreshCandidate) bool {
	return func(s []mutation.FreshCandidate) bool {
		if len(s) != len(wants) {
			return false
		}
		for i, want := range wants {
			if !want(s[i]) {
				return false
			}
		}
		*got = slices.Clone(s)
		return true
	}
}

// siteTests is the "tests" of a selected site, or nil.
func siteTests(site map[string]any) []string {
	var out []string
	list, _ := site["tests"].([]any)
	for _, id := range list {
		s, _ := id.(string)
		out = append(out, s)
	}
	return out
}

// exceptionEntry is an itos-cc.yaml exception, under mutation.exceptions,
// for the candidate c of the plan of the repository at dir, with hash
// instead of its function's when hash is not empty.
func exceptionEntry(t *testing.T, dir string, c mutation.FreshCandidate, hash, reason string) string {
	t.Helper()
	plan, err := planFreshForTest(t.Context(), dir, nil, "", 1, "any")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFreshPlan(t.Context(), plan)
	for _, u := range plan.Units {
		if u.Identity != c.UnitIdentity {
			continue
		}
		if hash == "" {
			hash = u.Hash
		}
		return fmt.Sprintf("    - file: %s\n      function: %q\n      hash: %q\n      line_in_function: %d\n      column: %d\n"+
			"      original: %q\n      replacement: %q\n      reason: %q\n",
			c.Path, c.Function, hash, c.Site.Line-u.StartLine+1, c.Site.Column, c.Site.Original, c.Site.Replacement, reason)
	}
	t.Fatalf("no unit %s in the plan", c.UnitIdentity)
	return ""
}

// commitExceptions adds entries under mutation.exceptions of the committed
// itos-cc.yaml of the repository at dir, in a new commit.
func commitExceptions(t *testing.T, dir string, entries ...string) {
	t.Helper()
	path := filepath.Join(dir, "itos-cc.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(data)+"  exceptions:\n"+strings.Join(entries, ""))
	gitIn(t, dir, "add", "itos-cc.yaml")
	gitIn(t, dir, "commit", "-qm", "exceptions")
}

// logOnFailure shows what o printed once t has failed.
func logOnFailure(t *testing.T, o *outcome) {
	t.Helper()
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("stdout:\n%s\nstderr:\n%s", o.stdout, o.stderr)
		}
	})
}

// @ID-MUT-179. Kept: the survivor names both listed tests it survived, as
// its "tests", which no scenario asserts: ID-MUT-179's reads its scope
// alone (T-130).
func TestFreshOutcomesPreserveSurvivorExceptionAndOwnTimeoutMeanings(t *testing.T) {
	t.Parallel()
	requireCountedPlatform(t)
	countedListedRepo(t)
	dir := wd(t)

	// The complete sequence passes on line 11's n*2 → n/2: own test, then
	// both listed tests. Line 27's mutant, drawn after it, is killed by the
	// own test, and is still judged.
	var pair []mutation.FreshCandidate
	seed := seedSelecting(t, dir, 2, selecting(&pair, at(listedStderrLine, "*"), at(listedHalfLine, ">")))
	o := countedRun(t, "--count", "2", "--seed", seed, "--workers", "1")
	logOnFailure(t, &o)
	c := o.counted(t)
	p := c.problem("mutation.survived")
	if o.code != 1 || p == nil || p["line"] != float64(listedStderrLine) {
		t.Errorf("exit %d, want 1 with mutation.survived for line %d: %+v", o.code, listedStderrLine, c.Problems)
	}
	if len(c.Selected) != 2 {
		t.Fatalf("selected = %+v, want two sites", c.Selected)
	}
	survivor, killed := c.Selected[0], c.Selected[1]
	if survivor["state"] != "judged" || survivor["outcome"] != "survived" || survivor["scope"] != "listed" ||
		!slices.Equal(siteTests(survivor), []string{"ID-A-01", "ID-A-02"}) {
		t.Errorf("survivor = %+v, want it judged survived after its own and both listed tests", survivor)
	}
	// Counted mode does not silently become fail-fast.
	if killed["state"] != "judged" || killed["outcome"] != "killed" || c.number("executed") != 2 || c.Sampling["completion"] != "completed" {
		t.Errorf("after the survivor: %+v, executed %d, completion %v; want the next site judged killed and the run completed",
			killed, c.number("executed"), c.Sampling["completion"])
	}

	// A valid matching exception: the run passes, yet the site is drawn and
	// its trial runs.
	var one []mutation.FreshCandidate
	seed = seedSelecting(t, dir, 1, selecting(&one, at(listedStderrLine, "*")))
	commitExceptions(t, dir, exceptionEntry(t, dir, one[0], "", "stderr is diagnostics only"))
	o = countedRun(t, "--count", "1", "--seed", seed, "--workers", "1")
	logOnFailure(t, &o)
	c = o.counted(t)
	if len(c.Selected) != 1 || c.Selected[0]["identity"] != one[0].Identity {
		t.Fatalf("selected = %+v, want the excepted site %s still drawn", c.Selected, one[0].Identity)
	}
	excepted := c.Selected[0]
	if o.code != 0 || excepted["outcome"] != "survived" || excepted["excepted"] != "stderr is diagnostics only" {
		t.Errorf("exit %d, site %+v: want the excepted survivor reported with its reason and the run passing", o.code, excepted)
	}
	if c.number("executed") != 1 || trialLines(o.stderr) != 1 {
		t.Errorf("executed %d, %d trial lines: an exception must not skip its trial", c.number("executed"), trialLines(o.stderr))
	}

	// Stale exceptions fail: one whose mutant the own test kills, and one
	// written for another version of its function, on an uncovered site
	// that runs no trial.
	countedListedRepo(t)
	dir = wd(t)
	var stale []mutation.FreshCandidate
	seed = seedSelecting(t, dir, 2, selecting(&stale, at(listedHalfLine, ">"), at(listedTwiceLine, "*")))
	commitExceptions(t, dir,
		exceptionEntry(t, dir, stale[0], "", "thought equivalent"),
		exceptionEntry(t, dir, stale[1], strings.Repeat("0", 64), "written for another version"))
	o = countedRun(t, "--count", "2", "--seed", seed, "--workers", "1")
	logOnFailure(t, &o)
	c = o.counted(t)
	why := map[string]bool{}
	for _, p := range c.problems("mutation.exception-stale") {
		w, _ := p["why"].(string)
		why[w] = true
	}
	if o.code != 1 || !why["killed"] || !why["changed"] {
		t.Errorf("exit %d, stale exceptions %v: want 1, with one killed and one changed", o.code, why)
	}
	if len(c.Selected) == 2 && (c.Selected[0]["outcome"] != "killed" || c.Selected[0]["excepted"] != nil) {
		t.Errorf("site %+v: want the killed mutant reported killed, not excepted", c.Selected[0])
	}

	// A mutant's own test deadline is a valid completed judgment.
	moduleRepo(t, map[string]string{
		"go.mod":       "module example.com/wait\n\ngo 1.22\n",
		"main.go":      "package main\n\nimport \"time\"\n\nfunc Wait(n int) int {\n\tfor n > 1 {\n\t\ttime.Sleep(time.Second)\n\t}\n\treturn n\n}\n\nfunc main() {}\n",
		"main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestWait(t *testing.T) {\n\tif Wait(1) != 1 {\n\t\tt.Fatal(\"Wait\")\n\t}\n}\n",
	})
	o = countedRun(t, "--count", "1", "--workers", "1", "--timeout-factor", "1")
	logOnFailure(t, &o)
	c = o.counted(t)
	if o.code != 0 || len(c.Selected) != 1 || c.Selected[0]["state"] != "judged" || c.Selected[0]["outcome"] != "timeout" || c.number("executed") != 1 {
		t.Errorf("exit %d, selected %+v: want its endless mutant judged timeout, a passing run", o.code, c.Selected)
	}
}

// @ID-MUT-182. Kept: the run a failing selection blocks reports no trials
// stage complete, and the failing own baseline's problem names its file,
// which no scenario asserts. ID-MUT-178 and 181's holders went, every
// assertion their scenarios' (T-130).
func TestCleanBaselineFailureIsNotAMutationKill(t *testing.T) {
	t.Parallel()
	requireCountedPlatform(t)
	// The harness fails whenever it runs a selection outside coverage, with
	// or without a mutant, so only its clean baseline tells a failing
	// selection from a kill.
	record, main := countedListedRepo(t, [3]string{"e2e/e2e_test.go", "\tsplit := os.Getenv(\"ITOS_CC_TEST_COVERDIR\")\n",
		"\tsplit := os.Getenv(\"ITOS_CC_TEST_COVERDIR\")\n\tif split == \"\" && *selected != \"\" {\n\t\tt.Fatal(\"the selection fails outside coverage\")\n\t}\n"})
	dir := wd(t)
	// Line 17's mutant needs ID-A-02 once it survives the own test; line
	// 27's, drawn after it, is killed by the own test.
	var pair []mutation.FreshCandidate
	seed := seedSelecting(t, dir, 2, selecting(&pair, at(listedManyLine, "-"), at(listedHalfLine, ">")))
	o := countedRun(t, "--count", "2", "--seed", seed, "--workers", "1")
	logOnFailure(t, &o)
	c := o.counted(t)

	// The run fails with the actual baseline problem.
	p := c.problem("tests.selection-failed")
	if o.code != 1 || p == nil || fmt.Sprint(p["ids"]) != "[ID-A-02]" {
		t.Errorf("exit %d, problems %+v: want 1 with tests.selection-failed for [ID-A-02]", o.code, c.Problems)
	}
	if len(c.Selected) != 2 {
		t.Fatalf("selected = %+v, want two sites", c.Selected)
	}
	// That site has no invented outcome, and its mutant never ran the
	// selection whose baseline failed.
	blocked := c.Selected[0]
	if blocked["outcome"] != nil || blocked["state"] == "judged" {
		t.Errorf("site %+v: want no killed, timeout or survived outcome", blocked)
	}
	n, _, mutated := unmutatedRuns(harnessRuns(t, record), "ID-A-02", main)
	if n != 1 || mutated >= 0 {
		t.Errorf("ID-A-02 ran %d times without the mutant and first with it at %d: want its baseline once and no mutant run", n, mutated)
	}
	// No stage reports a baseline or a mutation stage that never passed as
	// complete.
	if s := c.stage("trials"); s != nil && s["state"] == "complete" {
		t.Errorf("stage trials = %+v while a selected judgment could not finish", s)
	}
	stages, _ := blocked["stages"].([]any)
	for _, raw := range stages {
		s, _ := raw.(map[string]any)
		if name, _ := s["name"].(string); strings.HasPrefix(name, "listed") && s["state"] == "complete" {
			t.Errorf("site stage %+v reports a listed stage complete though its baseline failed", s)
		}
	}
	// The judgment completed before is still in the report.
	if done := c.Selected[1]; done["state"] != "judged" || done["outcome"] != "killed" {
		t.Errorf("site %+v: want the completed kill still reported", done)
	}

	// A failing own baseline: the own test passes under coverage only.
	moduleRepo(t, withFiles(compareFiles, map[string]string{
		"main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestCompare(t *testing.T) {\n\tif testing.CoverMode() == \"\" {\n\t\tt.Fatal(\"passes under coverage only\")\n\t}\n\tif !Compare(6) || Compare(5) {\n\t\tt.Fatal(\"Compare\")\n\t}\n}\n",
	}))
	o = countedRun(t, "--count", "1", "--workers", "1")
	logOnFailure(t, &o)
	c = o.counted(t)
	if p := c.problem("count.preparation-failed"); o.code != 1 || p == nil || p["stage"] != "baseline" || !strings.Contains(fmt.Sprint(p["message"]), "main.go") {
		t.Errorf("exit %d, problems %+v: want 1 with its baseline problem naming main.go", o.code, c.Problems)
	}
	if s := c.stage("baseline"); s == nil || s["state"] != "failed" {
		t.Errorf("stage baseline = %+v, want failed", s)
	}
	if s := c.stage("trials"); s != nil {
		t.Errorf("stage trials = %+v after a failed baseline", s)
	}
	if len(c.Selected) != 1 || c.Selected[0]["outcome"] != nil || c.number("executed") != 0 || trialLines(o.stderr) != 0 {
		t.Errorf("selected %+v, executed %d: want no outcome and no trial", c.Selected, c.number("executed"))
	}
}
