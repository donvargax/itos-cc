package features

import (
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

func (q *countedMutation) outcomeSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a counted selection has one covered mutation site$`, q.listedProject)
	sc.Step(`^its complete applicable test sequence passes on the mutant$`, q.survivorJudge)
	sc.Step(`^the run fails for that selected survivor unless its matching exception is valid$`, q.survivorUnlessReviewed)
	sc.Step(`^an exception must not exclude the site from fresh selection or skip its trial$`, q.exceptionStillExecuted)
	sc.Step(`^a killed mutant or its own test deadline is a valid completed judgment$`, q.killOrTimeout)
	sc.Step(`^stale exceptions fail without being trusted as successful outcomes$`, q.staleExceptions)
	sc.Step(`^counted mode does not silently become fail-fast scheduling$`, q.countedAggregate)
}
func (q *countedMutation) survivorJudge() error {
	if err := q.listedMode("survive"); err != nil {
		return err
	}
	return q.listedJudge()
}
func (q *countedMutation) survivorUnlessReviewed() error {
	if err := countedProblem(q.primary, "mutation.survived", 1); err != nil {
		return err
	}
	s, err := countedSelected(q.primary)
	if err != nil {
		return err
	}
	p := checkProblems(q.primary.object)[0]
	if s["outcome"] != "survived" || s["state"] != "judged" || s["scope"] != "listed" || s["excepted"] != nil || p["file"] != "subject.go" || p["line"] != s["line"] || p["column"] != s["column"] || p["function"] != s["function"] {
		return fmt.Errorf("fresh survivor problem/subject: %v", q.primary.object)
	}
	aggregate, err := q.count("--count", "2", "--seed", q.seed, "--workers", "1", "subject.go", "other.go")
	if err != nil {
		return err
	}
	q.saved["aggregate"] = aggregate
	recorded, err := q.observe("mutation", "run", "--json", "--workers", "1", "--mutate-all", "--no-annotate", "subject.go")
	if err != nil {
		return err
	}
	if recorded.result.Exit != 1 {
		return fmt.Errorf("actual reviewed survivor setup: %v", recorded.object)
	}
	review, err := q.observe("mutation", "except", "--json", "subject.go:3:37", "--reason", "T81 equivalent fixture case")
	if err != nil {
		return err
	}
	if review.result.Exit != 0 {
		return fmt.Errorf("actual survivor exception review: %v", review.object)
	}
	w := q.e.m.w
	if err = w.d.Git(w.dir, "add", "itos-cc.yaml"); err != nil {
		return err
	}
	if err = w.d.Git(w.dir, "commit", "-qm", "reviewed counted exception"); err != nil {
		return err
	}
	valid, err := q.count("--count", "1", "--seed", q.seed, "--workers", "1", "subject.go")
	if err != nil {
		return err
	}
	q.saved["excepted"] = valid
	site, err := countedSelected(valid)
	if err != nil {
		return err
	}
	if valid.result.Exit != 0 || valid.object["ok"] != true || site["excepted"] != "T81 equivalent fixture case" || site["outcome"] != "survived" || len(checkProblems(valid.object)) != 0 {
		return fmt.Errorf("valid reviewed fresh survivor: %v", valid.object)
	}
	return nil
}
func (q *countedMutation) exceptionStillExecuted() error {
	original, err := countedSelected(q.primary)
	if err != nil {
		return err
	}
	o := q.saved["excepted"]
	site, err := countedSelected(o)
	if err != nil {
		return err
	}
	if site["identity"] != original["identity"] || countedSampling(o)["selected"] != float64(1) || countedSampling(o)["executed"] != float64(1) || strings.Count(o.result.Stderr, "itos-cc: [") != 1 {
		return fmt.Errorf("review skipped/excluded fresh trial: %v", o.object)
	}
	// Actual command trace includes own and listed mutant executions even when
	// the public outcome is excepted. Use that observation, not the declaration.
	q.saved["original-when"] = q.primary
	q.primary = o
	defer func() { q.primary = q.saved["original-when"] }()
	if err = q.ownBeforeListed(); err != nil {
		return err
	}
	return q.listedCleanBaseline()
}
func (q *countedMutation) killOrTimeout() error {
	for _, mode := range []string{"covered", "timeout"} {
		other, err := q.otherProject(ownedOneSource)
		if err != nil {
			return err
		}
		if err = other.setMode(mode); err != nil {
			return err
		}
		o, err := other.count("--count", "1", "--workers", "1", "--timeout-factor", "1")
		if err != nil {
			return err
		}
		s, err := countedSelected(o)
		if err != nil {
			return err
		}
		want := "killed"
		if mode == "timeout" {
			want = "timeout"
		}
		if o.result.Exit != 0 || s["state"] != "judged" || s["outcome"] != want || countedSampling(o)["executed"] != float64(1) {
			return fmt.Errorf("valid own %s judgment: %v", want, o.object)
		}
	}
	return nil
}
func (q *countedMutation) staleExceptions() error {
	if err := q.listedMode("own-kill"); err != nil {
		return err
	}
	killed, err := q.count("--count", "1", "--seed", q.seed, "--workers", "1", "subject.go")
	if err != nil {
		return err
	}
	if err = countedProblem(killed, "mutation.exception-stale", 1); err != nil {
		return err
	}
	s, err := countedSelected(killed)
	if err != nil {
		return err
	}
	if checkProblems(killed.object)[0]["why"] != "killed" || s["outcome"] != "killed" || s["excepted"] != nil {
		return fmt.Errorf("killed exception trusted as assurance: %v", killed.object)
	}
	w := q.e.m.w
	changed := strings.Replace(countedListedSubject, "n > 5", "n > 5 /*changed*/", 1)
	if err = w.write("subject.go", changed); err != nil {
		return err
	}
	q.e.source["subject.go"] = changed
	if err = w.d.Git(w.dir, "add", "subject.go"); err != nil {
		return err
	}
	if err = w.d.Git(w.dir, "commit", "-qm", "changed excepted function"); err != nil {
		return err
	}
	if err = q.listedMode("survive"); err != nil {
		return err
	}
	if err = q.auditProfile(strings.Replace(countedListedProfile, "subject.go:3.28,3.42", "subject.go:3.28,3.54", 1)); err != nil {
		return err
	}
	changedRun, err := q.count("--count", "1", "--seed", q.seed, "--workers", "1", "subject.go")
	if err != nil {
		return err
	}
	stale := false
	for _, p := range checkProblems(changedRun.object) {
		if p["rule"] == "mutation.exception-stale" && p["why"] == "changed" {
			stale = true
		}
	}
	site, err := countedSelected(changedRun)
	if err != nil {
		return err
	}
	if changedRun.result.Exit != 1 || !stale || site["outcome"] != "survived" || site["excepted"] != nil {
		return fmt.Errorf("changed exception trusted as assurance: %v", changedRun.object)
	}
	return nil
}
func (q *countedMutation) countedAggregate() error {
	o := q.saved["aggregate"]
	if err := countedProblem(o, "mutation.survived", 1); err != nil {
		return err
	}
	selected := objectList(o.object, "selected")
	if len(selected) != 2 || countedSampling(o)["executed"] != float64(2) || countedSampling(o)["completion"] != "completed" {
		return fmt.Errorf("counted became fail-fast: %v", o.object)
	}
	outcomes := map[string]string{}
	for _, site := range selected {
		if site["state"] != "judged" {
			return fmt.Errorf("aggregate selected work not judged: %v", site)
		}
		outcomes[fmt.Sprint(site["function"])] = fmt.Sprint(site["outcome"])
	}
	foundSurvivor, foundKill := false, false
	for name, outcome := range outcomes {
		foundSurvivor = foundSurvivor || (strictNamed(name, "Compare") && outcome == "survived")
		foundKill = foundKill || (strictNamed(name, "Other") && outcome == "killed")
	}
	if !foundSurvivor || !foundKill {
		return fmt.Errorf("aggregate lost survivor or later valid judgment: %v", o.object)
	}
	return nil
}
