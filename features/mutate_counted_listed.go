package features

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

const countedListedProfile = "mode: set\nexample.com/ownership/main.go:4.2,5.13 2 0\nexample.com/ownership/main.go:5.14,5.24 1 0\nexample.com/ownership/main.go:6.2,6.25 1 0\nexample.com/ownership/other.go:3.26,3.40 1 1\nexample.com/ownership/subject.go:3.28,3.42 1 1\n"

func (q *countedMutation) listedSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a selected committed mutant survives its file's own tests$`, q.listedProject)
	sc.Step(`^listed integration tests reach its line and kill it$`, q.listedReach)
	sc.Step(`^count-one execution judges it$`, q.listedJudge)
	sc.Step(`^own tests run before the applicable listed tests$`, q.ownBeforeListed)
	sc.Step(`^the listed selection baseline runs without the mutant before its mutation stage$`, q.listedCleanBaseline)
	sc.Step(`^the final outcome is killed with the correct listed attribution$`, q.listedKill)
	sc.Step(`^the own and listed mutation stages consume one trial, not two$`, q.listedOneTrial)
	sc.Step(`^no unselected site executes$`, q.listedSelectedOnly)
	sc.Step(`^one selected fresh mutant needs own or applicable listed baselines$`, q.listedProject)
	sc.Step(`^an unmutated required baseline fails$`, q.listedBaselineFails)
	sc.Step(`^counted execution fails with its actual baseline problem$`, q.baselineProblem)
	sc.Step(`^that site has no invented killed, timeout or survived outcome$`, q.blockedNoOutcome)
	sc.Step(`^later stages do not report a baseline as passed when it never ran$`, q.baselineNotPassed)
	sc.Step(`^already completed valid judgments remain visible in the current report$`, q.completedBesideBlocked)
}
func (q *countedMutation) listedProject() error {
	w := q.e.m.w
	if err := w.d.Counted(); err != nil {
		return err
	}
	if err := q.e.project(map[string]string{"main.go": countedListedMain, "subject.go": countedListedSubject, "other.go": countedListedOther}); err != nil {
		return err
	}
	replace := strings.NewReplacer("CONTROL_PATH", strconv.Quote(filepath.Join(q.e.outside, "counted-control.json")), "TRACE_PATH", strconv.Quote(filepath.Join(q.e.outside, "trace.jsonl")), "ORIGINAL_ROOT", strconv.Quote(w.dir))
	files := map[string]string{"main_test.go": replace.Replace(countedListedOwn), "e2e/e2e_test.go": replace.Replace(countedListedHarness), "testdata/list.go": replace.Replace(countedListedList), "itos-cc.yaml": countedListedConfig}
	for file, text := range files {
		if err := w.write(file, text); err != nil {
			return err
		}
	}
	if err := w.d.Git(w.dir, "add", "main_test.go", "e2e/e2e_test.go", "testdata/list.go", "itos-cc.yaml"); err != nil {
		return err
	}
	if err := w.d.Git(w.dir, "commit", "-qm", "counted listed fixture"); err != nil {
		return err
	}
	if err := q.listedMode("listed-kill"); err != nil {
		return err
	}
	if err := q.originalHashes("subject.go"); err != nil {
		return err
	}
	return q.auditProfile(countedListedProfile)
}
func (q *countedMutation) listedMode(mode string) error {
	q.mode = mode
	w := q.e.m.w
	b, err := json.Marshal(map[string]string{"Mode": mode})
	if err != nil {
		return err
	}
	if err = w.d.Write(filepath.Join(q.e.outside, "counted-control.json"), b); err != nil {
		return err
	}
	site := Site{File: "subject.go", Line: 3, Original: ">", Replacement: ">="}
	trace := &TestTrace{File: filepath.Join(q.e.outside, "trace.jsonl"), Source: "subject.go", Tests: map[string]string{"main_test.go": "own", "e2e/e2e_test.go": "listed"}, Coverage: true}
	own := Tests{Language: "go", Command: q.e.command, Elapsed: 100 * time.Millisecond, Surviving: []Site{site}, Trace: trace}
	if mode == "own-kill" {
		own.Surviving = nil
	}
	if mode == "timeout" {
		own.Surviving = nil
		own.TimingOut = []Site{site}
	}
	if mode == "own-baseline-fail" {
		own.Baseline = "T81 own baseline fails"
	}
	if err = w.d.Tests(w.dir, own); err != nil {
		return err
	}
	listed := &ListedTests{Files: map[string]string{"ID-A-01": "e2e/e2e_test.go", "ID-A-02": "e2e/e2e_test.go"}, Lines: map[string]map[int][]string{"subject.go": {3: {"ID-A-01", "ID-A-02"}}}, Split: true, Surviving: map[string][]Site{"ID-A-02": {site}}}
	if mode == "survive" {
		listed.Surviving["ID-A-01"] = []Site{site}
	}
	if mode == "listed-baseline-fail" {
		listed.Failing = []string{"ID-A-01", "ID-A-02"}
	}
	if mode == "list-malformed" {
		listed.ListOutput = "ID-A-01\nID-A-01\n"
	}
	if mode == "list-fail" {
		listed.ListFails = true
	}
	if mode == "listed-coverage-fail" {
		listed.CoverageExit = 1
		listed.CoverageOutput = "T81 listed coverage fails after data"
	}
	if mode == "bad-metadata" {
		listed.CoverageMetadata = []byte("T81 invalid Go coverage metadata")
		listed.ConversionExit = 1
		listed.ConversionOutput = "decoding metadata: unexpected EOF"
	}
	if err = w.d.Tests(w.dir, Tests{Language: "go", Command: "go test -count=1 ./e2e -args -tests={pattern}", Elapsed: 100 * time.Millisecond, Listed: listed, Trace: trace}); err != nil {
		return err
	}
	c := Coverage{Language: "go", Report: countedListedProfile}
	if mode == "coverage-fail" {
		c.CommandExit = 1
		c.CommandOutput = "T81 coverage fails after report"
	}
	return w.d.Coverage(w.dir, c)
}
func (q *countedMutation) listedReach() error {
	// Public listing and the declared real integration harness establish the
	// selected location before its count-one execution; actual collected reach
	// and attribution are checked in the resulting public judgment.
	o, err := q.observe("mutation", "list", "--json", "subject.go")
	if err != nil {
		return err
	}
	sites := objectList(o.object, "sites")
	if o.result.Exit != 0 || len(sites) != 1 || sites[0]["line"] != float64(3) || sites[0]["original"] != ">" {
		return fmt.Errorf("listed fixture public site: %v", o.object)
	}
	return nil
}
func (q *countedMutation) listedJudge() error {
	var err error
	q.primary, err = q.count("--count", "1", "--seed", q.seed, "--workers", "1", "subject.go")
	return err
}
func (q *countedMutation) listedTracePositions() (int, int, int, error) {
	clean := countedSHA(countedListedSubject)
	mutant := countedSHA(strings.Replace(countedListedSubject, ">", ">=", 1))
	own, baseline, listed := -1, -1, -1
	for i, r := range q.primary.traces {
		if r["coverage"] == true {
			continue
		}
		switch {
		case r["test"] == "own" && r["source_sha256"] == mutant:
			if own != -1 {
				return 0, 0, 0, fmt.Errorf("duplicate own selected execution")
			}
			own = i
		case r["test"] == "listed" && r["source_sha256"] == clean:
			if baseline != -1 {
				return 0, 0, 0, fmt.Errorf("duplicate clean listed baseline")
			}
			baseline = i
		case r["test"] == "listed" && r["source_sha256"] == mutant:
			if listed != -1 {
				return 0, 0, 0, fmt.Errorf("duplicate listed selected execution")
			}
			listed = i
		}
	}
	return own, baseline, listed, nil
}
func (q *countedMutation) ownBeforeListed() error {
	own, _, listed, err := q.listedTracePositions()
	if err != nil {
		return err
	}
	if own < 0 || listed <= own {
		return fmt.Errorf("actual own/listed mutation order %d/%d: %v", own, listed, q.primary.traces)
	}
	return nil
}
func (q *countedMutation) listedCleanBaseline() error {
	own, baseline, listed, err := q.listedTracePositions()
	if err != nil {
		return err
	}
	if baseline <= own || listed <= baseline {
		return fmt.Errorf("actual clean listed baseline order own%d baseline%d listed%d", own, baseline, listed)
	}
	return nil
}
func (q *countedMutation) listedKill() error {
	s, err := countedSelected(q.primary)
	if err != nil {
		return err
	}
	if q.primary.result.Exit != 0 || s["state"] != "judged" || s["outcome"] != "killed" || s["scope"] != "listed" || fmt.Sprint(s["tests"]) != "[ID-A-01 ID-A-02]" {
		return fmt.Errorf("fresh listed final attribution: %v", q.primary.object)
	}
	return nil
}
func (q *countedMutation) listedOneTrial() error {
	p := countedSampling(q.primary)
	if p["budget"] != float64(1) || p["executed"] != float64(1) || strings.Count(q.primary.result.Stderr, "itos-cc: [") != 1 {
		return fmt.Errorf("own/listed trial budget: %v", q.primary.object)
	}
	return nil
}
func (q *countedMutation) listedSelectedOnly() error {
	clean := countedSHA(countedListedSubject)
	mutant := countedSHA(strings.Replace(countedListedSubject, ">", ">=", 1))
	if len(q.primary.traces) < 5 {
		return fmt.Errorf("listed actual trace incomplete: %v", q.primary.traces)
	}
	for _, r := range q.primary.traces {
		if r["source_sha256"] != clean && r["source_sha256"] != mutant {
			return fmt.Errorf("unselected source executed: %v", r)
		}
		if r["cwd"] == "" || r["original_root"] == "" {
			return fmt.Errorf("listed trace lacks real root: %v", r)
		}
	}
	return nil
}
func (q *countedMutation) listedBaselineFails() error {
	if err := q.listedMode("listed-baseline-fail"); err != nil {
		return err
	}
	var err error
	q.primary, err = q.count("--count", "2", "--seed", q.seed, "--workers", "1", "subject.go", "other.go")
	return err
}
func (q *countedMutation) baselineProblem() error {
	if err := countedProblem(q.primary, "tests.selection-failed", 1); err != nil {
		return err
	}
	p := checkProblems(q.primary.object)[0]
	if fmt.Sprint(p["ids"]) != "[ID-A-01 ID-A-02]" {
		return fmt.Errorf("actual failed listed baseline subject: %v", p)
	}
	return nil
}
func (q *countedMutation) blockedNoOutcome() error {
	found := false
	for _, s := range objectList(q.primary.object, "selected") {
		if strictNamed(fmt.Sprint(s["function"]), "Compare") {
			found = true
			if s["outcome"] != nil || s["state"] != "blocked" {
				return fmt.Errorf("blocked baseline invented mutation outcome: %v", s)
			}
		}
	}
	if !found {
		return fmt.Errorf("blocked Compare absent")
	}
	own, baseline, listed, err := q.listedTracePositions()
	if err != nil {
		return err
	}
	if own < 0 || baseline <= own || listed != -1 {
		return fmt.Errorf("failed clean baseline ran mutant listed selection: %d/%d/%d", own, baseline, listed)
	}
	return nil
}
func (q *countedMutation) baselineNotPassed() error {
	for _, s := range objectList(q.primary.object, "selected") {
		if !strictNamed(fmt.Sprint(s["function"]), "Compare") {
			continue
		}
		for _, stage := range objectList(s, "stages") {
			if strings.HasPrefix(fmt.Sprint(stage["name"]), "listed") && stage["state"] == "complete" {
				return fmt.Errorf("unrun listed stage reported passed: %v", stage)
			}
		}
	}
	return nil
}
func (q *countedMutation) completedBesideBlocked() error {
	found := false
	for _, s := range objectList(q.primary.object, "selected") {
		if strictNamed(fmt.Sprint(s["function"]), "Other") && s["outcome"] == "killed" && s["state"] == "judged" {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("valid completed judgment lost beside blocked selection: %v", q.primary.object)
	}
	other, err := q.otherProject(ownedOneSource)
	if err != nil {
		return err
	}
	if err = other.setMode("baseline-fail"); err != nil {
		return err
	}
	o, err := other.count("--count", "1")
	if err != nil {
		return err
	}
	if err = countedProblem(o, "count.preparation-failed", 1); err != nil {
		return err
	}
	s, err := countedSelected(o)
	if err != nil {
		return err
	}
	stage, err := countedStage(o, "baseline")
	if err != nil {
		return err
	}
	if checkProblems(o.object)[0]["stage"] != "baseline" || stage["state"] != "failed" || s["outcome"] != nil || s["state"] != "unattempted" || countedSampling(o)["executed"] != float64(0) {
		return fmt.Errorf("actual own baseline failure: %v", o.object)
	}
	for _, stage := range objectList(o.object, "stages") {
		if slices.Contains([]any{"trials"}, stage["name"]) {
			return fmt.Errorf("trials after own baseline failure: %v", stage)
		}
	}
	return nil
}
