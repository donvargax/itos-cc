//go:build !windows

package main

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The scenario of "Rule: A fail-fast stop keeps the valid judgments of the
// files it cut short" in features/mutate.feature, the fail-fast-partial
// slice (ADR-0022), whose holder still asserts what its scenario does not.
// Like the fail-fast-run steps it drives the CLI, reads its --json object
// as raw maps and reads the snapshots under .metrics as raw JSON. Every
// ordering it relies on waits on a marker file in $FF_DIR, never on a
// sleep.

// ffSnapshot is the snapshot of the source name under .metrics as raw JSON,
// nil when there is none.
func ffSnapshot(t *testing.T, name string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(inWD(t, filepath.Join(".metrics", "mutate", name+".json")))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("%s's snapshot: %v\n%s", name, err, data)
	}
	return out
}

// ffRecorded is the mutants the snapshot snap records for function, by its
// name, as raw maps; nil when it records none, or no entry for it.
func ffRecorded(snap map[string]any, function string) []map[string]any {
	units, _ := snap["units"].([]any)
	var out []map[string]any
	for _, u := range units {
		unit, _ := u.(map[string]any)
		if unit["name"] != function {
			continue
		}
		mutants, _ := unit["mutants"].([]any)
		for _, m := range mutants {
			out = append(out, m.(map[string]any))
		}
	}
	return out
}

// ffCheckRules is the rules mutation check reports for each function, by
// the name after its "#", from its --json object.
func ffCheckRules(t *testing.T, o outcome) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, p := range o.ff(t).Problems {
		function, _ := p["function"].(string)
		rule, _ := p["rule"].(string)
		name := function[strings.LastIndex(function, "#")+1:]
		out[name] = append(out[name], rule)
	}
	return out
}

// unjudgedRule says whether rules hold mutation check's verdict on a
// function with no valid result for some site: missing or stale.
func unjudgedRule(rules []string) bool {
	for _, r := range rules {
		if r == "mutation.missing" || r == "mutation.stale" {
			return true
		}
	}
	return false
}

// readText is the text of the file name, a relative one in the test's
// directory.
func readText(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(inWD(t, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// release lets every held judgment of test.sh end once the test is over.
func release(t *testing.T, ff string) {
	t.Cleanup(func() {
		if err := os.WriteFile(filepath.Join(ff, "release"), nil, 0o644); err != nil {
			t.Error(err)
		}
	})
}

// partialFiles is the module of ID-MUT-200's stopped file. prior.sh, the
// test command of an earlier run, kills Done's mutant. test.sh kills it
// too; Hold's mutant marks that it started and waits for a release that
// never comes; Surv's survives once Hold's started; it kills Later's.
var partialFiles = map[string]string{
	"go.mod":       "module example.com/ff\n\ngo 1.22\n",
	"main.go":      "package main\n\nfunc Done(n int) bool { return n > 4 }\n\nfunc Hold(n int) bool { return n > 5 }\n\nfunc Surv(n int) bool { return n > 6 }\n\nfunc Later(n int) bool { return n > 7 }\n\nfunc main() {}\n",
	"main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestNothing(t *testing.T) {}\n",
	"prior.sh":     ffScript("grep -q 'n > 4' main.go || exit 1", "exit 0"),
	"test.sh": ffScript(
		"grep -q 'n > 4' main.go || exit 1",
		"if ! grep -q 'n > 5' main.go; then",
		"\ttouch \"$FF_DIR/started\"",
		"\twhile [ ! -e \"$FF_DIR/release\" ]; do sleep 0.01; done",
		"\texit 1",
		"fi",
		"if ! grep -q 'n > 6' main.go; then",
		"\twhile [ ! -e \"$FF_DIR/started\" ]; do sleep 0.01; done",
		"\texit 0",
		"fi",
		"grep -q 'n > 7' main.go || exit 1",
		"exit 0"),
}

// @ID-MUT-200. Kept: its second case, a stop by a failing listed selection,
// keeps a reused listed kill with the snapshot's listed tests, their files
// and its support as recorded, and records the measured uncovered mutant
// beside it, which no scenario asserts: ID-MUT-200's stop is a survivor's,
// and ID-MUT-211's reads no snapshot. ID-MUT-201 and 202's holders went,
// every assertion their scenarios' (T-130).
func TestValidJudgmentsInAStoppedFileArePreservedUnfinishedSitesStayUnjudged(t *testing.T) {
	t.Parallel()
	t.Run("cut short by a survivor", func(t *testing.T) {
		_, ff := ffRepo(t, partialFiles)
		release(t, ff)
		// An earlier run judged Done alone, with prior.sh: its kill is
		// recorded with that scope and the module's tests as its evidence.
		edit(t, "main.go", "func Done(n int) bool { return n > 4 }", "func Done(n int) bool { /* judged */ return n > 4 }")
		commitAll(t, "judge Done")
		prior := itosCc(t, "mutation", "run", "--json", "--no-coverage", "--no-annotate", "--test-command", "sh prior.sh", "--since", "HEAD~1")
		logOutcome(t, &prior)
		done := ffRecorded(ffSnapshot(t, "main.go"), "Done")
		if prior.code != 0 || len(done) != 1 || done[0]["outcome"] != "killed" || done[0]["scope"] != "sh prior.sh" || wholeSuiteEvidence(done[0]) == nil {
			t.Fatalf("earlier run: exit %d, Done recorded %v: want 0 and Done's mutant killed by sh prior.sh, with its evidence", prior.code, done)
		}
		source := readText(t, "main.go")

		// Given a fail-fast run judges some mutants of the file, Done's
		// reused and Surv's run, and the stop cancels Hold's and leaves
		// Later's.
		o := itosCc(t, "mutation", "run", "--json", "--no-coverage", "--test-command", "sh test.sh", "--workers", "2",
			"--timeout-factor", "10000", "--fail-fast")
		logOutcome(t, &o)
		f := o.ff(t)
		m := f.file("main.go")
		if o.code != 1 || f.stopRule() != "mutation.survived" || m["state"] != "stopped" ||
			ffMutant(m, "Done")["reused"] != true || ffMutant(m, "Surv")["outcome"] != "survived" ||
			ffMutant(m, "Hold")["outcome"] != nil || ffMutant(m, "Later")["outcome"] != nil {
			t.Fatalf("exit %d, stop %v, main.go %v: want 1, stopped by Surv's survivor, Done's mutant reused, Hold's and Later's undecided", o.code, f.Stop, m)
		}

		// Then the snapshot keeps each completed or reused judgment with
		// its original scope and freshness evidence.
		snap := ffSnapshot(t, "main.go")
		if got := ffRecorded(snap, "Done"); len(got) != 1 || !reflect.DeepEqual(got[0], done[0]) {
			t.Errorf("Done recorded %v: want the reused kill as the earlier run recorded it, %v", got, done)
		}
		surv := ffRecorded(snap, "Surv")
		evidence, _ := func() (map[string]any, bool) {
			if len(surv) != 1 {
				return nil, false
			}
			e := wholeSuiteEvidence(surv[0])
			return e, e != nil
		}()
		tests, _ := evidence["tests"].(map[string]any)
		if len(surv) != 1 || surv[0]["outcome"] != "survived" || surv[0]["scope"] != "sh test.sh" || tests["main_test.go"] == nil {
			t.Errorf("Surv recorded %v: want its survivor, decided by sh test.sh, with the module's tests as its evidence", surv)
		}
		// And cancelled and unattempted sites get no outcome and no new
		// freshness proof.
		for _, function := range []string{"Hold", "Later"} {
			if got := ffRecorded(snap, function); len(got) != 0 {
				t.Errorf("%s recorded %v: want no entry for a site the stop left undecided", function, got)
			}
		}
		// And mutation check still fails the functions with missing valid
		// site entries.
		check := itosCc(t, "mutation", "check", "--json", "main.go")
		logOutcome(t, &check)
		rules := ffCheckRules(t, check)
		if check.code != 1 || !unjudgedRule(rules["Hold"]) || !unjudgedRule(rules["Later"]) || len(rules["Done"]) != 0 {
			t.Errorf("mutation check: exit %d, rules %v: want 1, Hold and Later missing or stale, nothing of Done", check.code, rules)
		}
		// And the file's annotated source bytes are unchanged.
		if got := readText(t, "main.go"); got != source {
			t.Errorf("main.go changed:\n%s\nwant it as it was:\n%s", got, source)
		}
	})

	t.Run("blocked by a failing selection", func(t *testing.T) {
		// An earlier run judged shown alone: its mutant survived its own
		// tests and the listed tests killed it. The list names the file
		// of each test, and a support file, so the kill rests on both.
		files := maps.Clone(ffListedFiles)
		files["itos-cc.yaml"] = strings.Replace(files["itos-cc.yaml"], `list: printf "ID-A-01\nID-A-02\n"`,
			`list: printf "ID-A-01\te2e/e2e_test.go\nID-A-02\te2e/e2e_test.go\n"`, 1) + "    support: [\"support/*.txt\"]\n"
		files["support/dep.txt"] = "one\n"
		moduleRepo(t, files)
		edit(t, "main.go", "func shown(n int) bool {\n", "func shown(n int) bool {\n\t// judged\n")
		commitAll(t, "judge shown")
		ff := t.TempDir()
		useEnv(t, "FF_DIR", ff)
		release(t, ff)
		prior := itosCc(t, "mutation", "run", "--json", "--no-annotate", "--since", "HEAD~1", "--workers", "1")
		logOutcome(t, &prior)
		before := ffSnapshot(t, "main.go")
		shown := ffRecorded(before, "shown")
		if prior.code != 0 || len(shown) != 1 || shown[0]["outcome"] != "killed" || shown[0]["scope"] != "listed" ||
			before["listed_files"] == nil || before["support"] == nil {
			t.Fatalf("earlier run: exit %d, shown recorded %v: want 0 and its mutant killed by the listed tests, their files and the support file recorded", prior.code, shown)
		}
		for _, function := range []string{"idle", "quick", "wait", "label", "spare"} {
			head := "func " + function + "(n int) bool {\n"
			edit(t, "main.go", head, head+"\t// judged\n")
		}
		commitAll(t, "judge the rest")
		source := readText(t, "main.go")

		// Given a fail-fast run, as ID-MUT-197's, reuses shown's kill,
		// measures idle's mutant uncovered and kills quick's; label's
		// selection fails while wait's mutant runs, which is cancelled,
		// and spare's never starts.
		useEnv(t, "FF_FAIL_ALONE", "1")
		useEnv(t, "FF_FAIL_AFTER", filepath.Join(ff, "started"))
		o := itosCc(t, "mutation", "run", "--json", "--since", "HEAD~2", "--workers", "2", "--fail-fast")
		logOutcome(t, &o)
		f := o.ff(t)
		m := f.file("main.go")
		if o.code != 1 || f.stopRule() != "tests.selection-failed" || m["state"] != "blocked" ||
			ffMutant(m, "shown")["reused"] != true || ffMutant(m, "quick")["outcome"] != "killed" ||
			!undecided(ffMutant(m, "wait"), "cancelled") || !undecided(ffMutant(m, "label"), "blocked") ||
			!undecided(ffMutant(m, "spare"), "unattempted") {
			t.Fatalf("exit %d, stop %v, main.go %v: want 1, stopped by label's selection, shown reused, quick killed, wait cancelled, label blocked, spare unattempted",
				o.code, f.Stop, m)
		}

		// Then the snapshot keeps each completed or reused judgment with
		// its original scope and freshness evidence: shown's listed kill,
		// its tests and the files they rest on as recorded.
		snap := ffSnapshot(t, "main.go")
		if got := ffRecorded(snap, "shown"); len(got) != 1 || !reflect.DeepEqual(got[0], shown[0]) {
			t.Errorf("shown recorded %v: want the reused listed kill as the earlier run recorded it, %v", got, shown)
		}
		for _, key := range []string{"listed", "listed_files", "support"} {
			if !reflect.DeepEqual(snap[key], before[key]) {
				t.Errorf("%s = %v: want it as the earlier run recorded it, %v", key, snap[key], before[key])
			}
		}
		if got := ffRecorded(snap, "quick"); len(got) != 1 || got[0]["outcome"] != "killed" || got[0]["scope"] != nil {
			t.Errorf("quick recorded %v: want its kill by its own tests", got)
		}
		if got := ffRecorded(snap, "idle"); len(got) != 1 || got[0]["outcome"] != "uncovered" {
			t.Errorf("idle recorded %v: want its mutant measured uncovered", got)
		}
		// And cancelled, blocked and unattempted sites get no outcome and
		// no new freshness proof.
		for _, function := range []string{"wait", "label", "spare"} {
			if got := ffRecorded(snap, function); len(got) != 0 {
				t.Errorf("%s recorded %v: want no entry for a site the stop left undecided", function, got)
			}
		}
		// And mutation check still fails the functions with missing valid
		// site entries.
		check := itosCc(t, "mutation", "check", "--json", "--since", "HEAD~2")
		logOutcome(t, &check)
		rules := ffCheckRules(t, check)
		for _, function := range []string{"wait", "label", "spare"} {
			if !unjudgedRule(rules[function]) {
				t.Errorf("mutation check: rules %v: want %s missing or stale", rules, function)
			}
		}
		if check.code != 1 || len(rules["shown"]) != 0 || len(rules["quick"]) != 0 {
			t.Errorf("mutation check: exit %d, rules %v: want 1, nothing of shown or quick", check.code, rules)
		}
		// And the file's annotated source bytes are unchanged.
		if got := readText(t, "main.go"); got != source {
			t.Errorf("main.go changed:\n%s\nwant it as it was:\n%s", got, source)
		}
	})
}
