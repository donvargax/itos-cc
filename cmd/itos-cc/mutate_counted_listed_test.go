package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/mutate"
)

// The scenarios of "Rule: Fresh counted mutation judges a bounded committed
// selection without claiming full proof" in features/mutate.feature that the
// mutation-counted-listed slice holds. They use the module of
// mutate_listed_test.go, whose binary its e2e harness tests by listed test
// ID, through selectionRepo, which makes the harness record the SHA-256 of
// the main.go each of its runs builds; ownRecorded makes the file's own test
// record it too. Like the other counted steps they drive the CLI and read
// the --json object as raw maps, and each focused case selects one or two
// sites, so a run holds at most two mutant trials.

// ownRecorded makes main_test.go record, on each run, "own main=<SHA-256 of
// the main.go it tests>" in the file GREET_RECORD names.
var ownRecorded = [3]string{"main_test.go", "import \"testing\"\n\nfunc TestHalf(t *testing.T) {\n",
	"import (\n\t\"crypto/sha256\"\n\t\"fmt\"\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestHalf(t *testing.T) {\n" +
		"\tif record := os.Getenv(\"GREET_RECORD\"); record != \"\" {\n" +
		"\t\tsrc, _ := os.ReadFile(\"main.go\")\n" +
		"\t\tif f, err := os.OpenFile(record, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {\n" +
		"\t\t\tfmt.Fprintf(f, \"own main=%x\\n\", sha256.Sum256(src))\n" +
		"\t\t\tf.Close()\n\t\t}\n\t}\n"}

// countedListedRepo is selectionRepo with a.feature, the file the list
// command names for ID-A-01, committed: counted preparation admits only a
// listed test file the commit holds.
func countedListedRepo(t *testing.T, changes ...[3]string) (string, string) {
	t.Helper()
	record, main := selectionRepo(t, changes...)
	dir, _ := os.Getwd()
	writeFile(t, filepath.Join(dir, "a.feature"), "Feature: one\n")
	gitIn(t, dir, "add", "a.feature")
	gitIn(t, dir, "commit", "-qm", "a.feature")
	return record, main
}

// sha is the hex SHA-256 of text, as the recording tests write it.
func sha(text string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(text)))
}

// at is the site of greet's main.go on line whose original text is original.
func at(line int, original string) func(mutate.FreshCandidate) bool {
	return func(c mutate.FreshCandidate) bool { return c.Site.Line == line && c.Site.Original == original }
}

// selecting is a seedSelecting predicate: the selection is, in order, the
// sites wants accept, and the candidates are kept in *got.
func selecting(got *[]mutate.FreshCandidate, wants ...func(mutate.FreshCandidate) bool) func([]mutate.FreshCandidate) bool {
	return func(s []mutate.FreshCandidate) bool {
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

// firstIndex is the index of the first of lines that is line, -1 for none.
func firstIndex(lines []string, line string) int {
	return slices.Index(lines, line)
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
func exceptionEntry(t *testing.T, dir string, c mutate.FreshCandidate, hash, reason string) string {
	t.Helper()
	plan, err := mutate.PlanFresh(dir, nil, "", 1, "any")
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
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

// @ID-MUT-178
func TestApplicableListedIntegrationTestsFinishASingleFreshJudgment(t *testing.T) {
	requireCountedPlatform(t)
	record, main := countedListedRepo(t, ownRecorded)
	dir, _ := os.Getwd()
	// Line 17's n-1 → n+1 survives the own test, and ID-A-02 alone reaches
	// it and kills it.
	var chosen []mutate.FreshCandidate
	seed := seedSelecting(t, dir, 1, selecting(&chosen, at(listedManyLine, "-")))
	clean, mutated := sha(main), sha(string(chosen[0].Site.Apply([]byte(main))))

	o := countedRun(t, "--count", "1", "--seed", seed, "--workers", "1")
	logOnFailure(t, &o)
	c := o.counted(t)
	lines := harnessRuns(t, record)

	// Own tests run on the mutant before the listed tests do.
	own := firstIndex(lines, "own main="+mutated)
	baseline := firstIndex(lines, "run tests=ID-A-02 coverdir= main="+clean)
	listed := firstIndex(lines, "run tests=ID-A-02 coverdir= main="+mutated)
	if own < 0 || listed < 0 || own > listed {
		t.Errorf("the own test ran on the mutant at %d and ID-A-02 at %d of the record, want the own test first:\n%s", own, listed, strings.Join(lines, "\n"))
	}
	// The selection's baseline runs without the mutant before its mutation stage.
	if baseline < 0 || baseline > listed || baseline < own {
		t.Errorf("ID-A-02 ran without the mutant at %d, want after the own stage (%d) and before its mutation stage (%d):\n%s",
			baseline, own, listed, strings.Join(lines, "\n"))
	}
	// The final outcome is killed with the listed attribution.
	if len(c.Selected) != 1 {
		t.Fatalf("selected = %+v, want the one site", c.Selected)
	}
	site := c.Selected[0]
	if o.code != 0 || site["state"] != "judged" || site["outcome"] != "killed" || site["scope"] != "listed" || !slices.Equal(siteTests(site), []string{"ID-A-02"}) {
		t.Errorf("exit %d, selected %+v: want it judged killed with scope listed by [ID-A-02]", o.code, site)
	}
	// The own and listed mutation stages consume one trial, not two.
	if c.number("executed") != 1 || trialLines(o.stderr) != 1 || c.number("budget") != 1 {
		t.Errorf("executed %d with %d trial lines, want one trial for the own and listed stages", c.number("executed"), trialLines(o.stderr))
	}
	// No unselected site executes: every main.go the tests saw is the
	// committed one or the selected mutant.
	for _, l := range lines {
		if _, hash, ok := strings.Cut(l, "main="); ok && hash != clean && hash != mutated {
			t.Errorf("a test ran on a main.go that is neither committed nor the selected mutant: %s", l)
		}
	}
}

// @ID-MUT-179
func TestFreshOutcomesPreserveSurvivorExceptionAndOwnTimeoutMeanings(t *testing.T) {
	requireCountedPlatform(t)
	countedListedRepo(t)
	dir, _ := os.Getwd()

	// The complete sequence passes on line 11's n*2 → n/2: own test, then
	// both listed tests. Line 27's mutant, drawn after it, is killed by the
	// own test, and is still judged.
	var pair []mutate.FreshCandidate
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
	var one []mutate.FreshCandidate
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
	dir, _ = os.Getwd()
	var stale []mutate.FreshCandidate
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

// preparationFailure is one way fresh preparation of greet's module fails:
// the changes that make it fail and the stage that must report it.
type preparationFailure struct {
	name    string
	changes [][3]string
	stage   string
}

// @ID-MUT-181
func TestCountedPreparationNeverConvertsFailedMeasurementIntoSuccessfulAssurance(t *testing.T) {
	requireCountedPlatform(t)
	harness := "e2e/e2e_test.go"
	failures := []preparationFailure{
		{"a failing list command", [][3]string{{"itos-cc.yaml", "list: go run ./testdata/list.go", "list: go run ./testdata/list.go fail"}}, "listed-tests"},
		{"a malformed list", [][3]string{{"itos-cc.yaml", "list: go run ./testdata/list.go", `list: "printf 'ID-A-01\\nID-A-01\\n'"`}}, "listed-tests"},
		{"an own coverage command that fails after writing its report", [][3]string{{"main_test.go", "func TestHalf(t *testing.T) {\n",
			"func TestHalf(t *testing.T) {\n\tif testing.CoverMode() != \"\" {\n\t\tt.Fatal(\"fails under coverage\")\n\t}\n"}}, "coverage"},
		{"a listed coverage command that fails after writing its data", [][3]string{{harness, "\tfor _, f := range features {\n",
			"\tif split != \"\" {\n\t\tdefer t.Error(\"fails after writing coverage\")\n\t}\n\tfor _, f := range features {\n"}}, "listed-coverage"},
		{"listed coverage data that does not convert", [][3]string{{harness, "\t\t\tcmd.Env = append(os.Environ(), \"GOCOVERDIR=\"+dir)\n",
			"\t\t\tos.WriteFile(filepath.Join(dir, \"covmeta.bogus\"), []byte(\"not coverage\"), 0o644)\n"}}, "listed-coverage"},
	}
	for _, f := range failures {
		t.Run(f.name, func(t *testing.T) {
			countedListedRepo(t, f.changes...)
			o := countedRun(t, "--count", "1", "--workers", "1")
			logOnFailure(t, &o)
			c := o.counted(t)
			p := c.problem("count.preparation-failed")
			if o.code != 1 || p == nil || p["stage"] != f.stage {
				t.Errorf("exit %d, problems %+v: want 1 with count.preparation-failed at stage %s", o.code, c.Problems, f.stage)
			}
			if s := c.stage(f.stage); s == nil || s["state"] != "failed" {
				t.Errorf("stage %s = %+v in %+v, want failed: a report never erases its command's failure", f.stage, s, c.Stages)
			}
			for _, s := range c.Stages {
				if s["name"] == "baseline" || s["name"] == "trials" || (s["name"] == "preparation" && s["state"] == "complete") {
					t.Errorf("stage %+v follows the failed %s", s, f.stage)
				}
			}
			if c.number("executed") != 0 || trialLines(o.stderr) != 0 || c.Sampling["completion"] != "stopped" {
				t.Errorf("executed %d with %d trial lines, completion %v: no trial starts after failed preparation",
					c.number("executed"), trialLines(o.stderr), c.Sampling["completion"])
			}
			for _, s := range c.Selected {
				if s["outcome"] != nil {
					t.Errorf("site %+v has an outcome after failed preparation", s)
				}
			}
		})
	}

	// Raw or existing coverage cannot stand in for fresh evidence or listed
	// tests: each is refused before anything runs.
	countedListedRepo(t)
	dir, _ := os.Getwd()
	for _, args := range [][]string{{"--coverage-report", "coverage.out"}, {"--use-existing-coverage"},
		{"--coverage-command", "true"}, {"--no-coverage"}, {"--test-command", "true"}} {
		o := countedRun(t, append([]string{"--count", "1"}, args...)...)
		if p := o.counted(t).problem("flags.conflict"); o.code != 2 || p == nil || trialLines(o.stderr) != 0 || strings.Contains(o.stderr, "itos-cc: coverage") {
			t.Errorf("--count with %v: exit %d, want a flags.conflict before any command\n%s%s", args, o.code, o.stdout, o.stderr)
		}
	}

	// No local measurement or per-scenario coverage map is needed: a fresh
	// clone with none prepares every stage, here for an uncovered site,
	// which runs no trial.
	var uncovered []mutate.FreshCandidate
	seed := seedSelecting(t, dir, 1, selecting(&uncovered, at(listedTwiceLine, "*")))
	o := countedRun(t, "--count", "1", "--seed", seed)
	logOnFailure(t, &o)
	c := o.counted(t)
	if o.code != 0 || len(c.Selected) != 1 || c.Selected[0]["outcome"] != "uncovered" {
		t.Errorf("exit %d, selected %+v: want the uncovered site measured with no local measurement", o.code, c.Selected)
	}
	for _, name := range []string{"listed-tests", "coverage", "listed-coverage", "preparation"} {
		if s := c.stage(name); s == nil || s["state"] != "complete" {
			t.Errorf("stage %s = %+v, want complete", name, s)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".metrics")); !os.IsNotExist(err) {
		t.Errorf("a counted run needed or wrote .metrics: %v", err)
	}
}

// @ID-MUT-182
func TestCleanBaselineFailureIsNotAMutationKill(t *testing.T) {
	requireCountedPlatform(t)
	// The harness fails whenever it runs a selection outside coverage, with
	// or without a mutant, so only its clean baseline tells a failing
	// selection from a kill.
	record, main := countedListedRepo(t, [3]string{"e2e/e2e_test.go", "\tsplit := os.Getenv(\"ITOS_CC_TEST_COVERDIR\")\n",
		"\tsplit := os.Getenv(\"ITOS_CC_TEST_COVERDIR\")\n\tif split == \"\" && *selected != \"\" {\n\t\tt.Fatal(\"the selection fails outside coverage\")\n\t}\n"})
	dir, _ := os.Getwd()
	// Line 17's mutant needs ID-A-02 once it survives the own test; line
	// 27's, drawn after it, is killed by the own test.
	var pair []mutate.FreshCandidate
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
