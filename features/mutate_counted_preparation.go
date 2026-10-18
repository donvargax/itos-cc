package features

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

var countedPrepStages = map[string]string{"list-fail": "listed-tests", "list-malformed": "listed-tests", "coverage-fail": "coverage", "listed-coverage-fail": "listed-coverage", "bad-metadata": "listed-coverage"}

func (q *countedMutation) preparationSteps(sc *godog.ScenarioContext) {
	sc.Step(`^fresh counted preparation requires list, coverage or provider commands$`, q.listedProject)
	sc.Step(`^a required command fails, or its report is missing or malformed$`, q.preparationFailures)
	sc.Step(`^the command fails and reports the actual failed stage$`, q.failedStages)
	sc.Step(`^a report produced by a failed command does not erase its failure$`, q.failedCommandReport)
	sc.Step(`^per-test conversion and coverage failures are not silently ignored$`, q.failedPerTest)
	sc.Step(`^no mutation trial starts after failed preparation$`, q.noFailedPreparationTrial)
	sc.Step(`^no preexisting local measurement or per-scenario coverage map is required for successful preparation$`, q.freshPreparation)
	sc.Step(`^raw or existing coverage overrides cannot silently bypass required fresh evidence or listed tests$`, q.noCoverageBypass)
}
func (q *countedMutation) preparationFailures() error {
	for _, mode := range []string{"list-fail", "list-malformed", "coverage-fail", "listed-coverage-fail", "bad-metadata"} {
		if err := q.listedMode(mode); err != nil {
			return err
		}
		o, err := q.count("--count", "1", "--workers", "1", "--seed", q.seed, "subject.go")
		if err != nil {
			return err
		}
		q.saved["prep-"+mode] = o
	}
	return nil
}
func (q *countedMutation) failedStages() error {
	for mode, want := range countedPrepStages {
		o, ok := q.saved["prep-"+mode]
		if !ok {
			return fmt.Errorf("missing actual preparation control %s", mode)
		}
		if err := countedProblem(o, "count.preparation-failed", 1); err != nil {
			return err
		}
		stage, err := countedStage(o, want)
		if err != nil {
			return err
		}
		if checkProblems(o.object)[0]["stage"] != want || stage["state"] != "failed" {
			return fmt.Errorf("actual failed %s stage: %v", mode, o.object)
		}
	}
	return nil
}
func (q *countedMutation) failedCommandReport() error {
	o := q.saved["prep-coverage-fail"]
	if !strings.Contains(o.result.Stderr, "T81 coverage fails after report") {
		return fmt.Errorf("actual failed coverage diagnostic absent: %s", o.result.Stderr)
	}
	// Observe the same failing real Go test writing a profile through public
	// crap into a separate owned artifact, outside counted scratch cleanup.
	if err := q.listedMode("coverage-fail"); err != nil {
		return err
	}
	w := q.e.m.w
	file := filepath.Join(q.e.outside, "failed.coverage")
	code, _, log, err := w.capture(w.dir, w.env(), "crap", "--json", "--coverage-command", "go test -count=1 -coverprofile="+file+" .", "--coverage-report", file, "subject.go")
	if err != nil {
		return err
	}
	b, err := w.d.Read(file)
	if err != nil {
		return err
	}
	if string(b) != countedListedProfile || !strings.Contains(log, "T81 coverage fails after report") || !strings.Contains(log, "exit status 1") {
		return fmt.Errorf("failed real command report exit%d: %s\n%s", code, b, log)
	}
	return nil
}
func (q *countedMutation) failedPerTest() error {
	collection := q.saved["prep-listed-coverage-fail"]
	conversion := q.saved["prep-bad-metadata"]
	if !strings.Contains(collection.result.Stderr, "T81 listed coverage fails after data") {
		return fmt.Errorf("listed collection failure diagnostic missing: %s", collection.result.Stderr)
	}
	if strings.Contains(conversion.result.Stderr, "T81 listed coverage fails after data") {
		return fmt.Errorf("conversion was mistaken for collection command failure")
	}
	b, err := q.e.m.w.d.Read(filepath.Join(q.e.outside, "observed-covmeta"))
	if err != nil {
		return err
	}
	if string(b) != "T81 invalid Go coverage metadata" {
		return fmt.Errorf("actual malformed per-test metadata: %s", b)
	}
	for _, o := range []countedObservation{collection, conversion} {
		own, err := countedStage(o, "coverage")
		if err != nil {
			return err
		}
		if own["state"] != "complete" {
			return fmt.Errorf("listed failure masked own coverage: %v", o.object)
		}
	}
	return nil
}
func (q *countedMutation) noFailedPreparationTrial() error {
	for mode := range countedPrepStages {
		o := q.saved["prep-"+mode]
		if countedSampling(o)["executed"] != float64(0) || countedSampling(o)["completion"] != "stopped" || strings.Contains(o.result.Stderr, "itos-cc: [") {
			return fmt.Errorf("mutation trial after failed %s preparation: %v", mode, o.object)
		}
		for _, s := range objectList(o.object, "selected") {
			if s["outcome"] != nil {
				return fmt.Errorf("failed preparation invented outcome: %v", s)
			}
		}
		for _, r := range o.traces {
			if r["source_sha256"] != countedSHA(countedListedSubject) {
				return fmt.Errorf("mutated source executed after failed %s preparation: %v", mode, r)
			}
		}
		for _, stage := range objectList(o.object, "stages") {
			if stage["name"] == "baseline" || stage["name"] == "trials" || (stage["name"] == "preparation" && stage["state"] == "complete") {
				return fmt.Errorf("stage after failed preparation: %v", stage)
			}
		}
	}
	return nil
}
func (q *countedMutation) freshPreparation() error {
	if err := q.listedMode("listed-kill"); err != nil {
		return err
	}
	w := q.e.m.w
	// Public setup artifacts belong to the fixture and are deliberately removed;
	// the final count must create its own real measurements from committed input.
	if err := w.d.Remove(filepath.Join(w.dir, ".metrics")); err != nil {
		return err
	}
	o, err := q.count("--count", "1", "--workers", "1", "subject.go")
	if err != nil {
		return err
	}
	if o.result.Exit != 0 {
		return fmt.Errorf("fresh independent preparation failed: %v", o.object)
	}
	for _, name := range []string{"listed-tests", "coverage", "listed-coverage", "preparation"} {
		s, err := countedStage(o, name)
		if err != nil {
			return err
		}
		if s["state"] != "complete" {
			return fmt.Errorf("fresh stage %s: %v", name, s)
		}
	}
	if w.d.Exists(filepath.Join(w.dir, ".metrics")) {
		return fmt.Errorf("counted preparation needed/published local measurements")
	}
	q.saved["fresh-preparation"] = o
	return nil
}
func (q *countedMutation) noCoverageBypass() error {
	for _, args := range [][]string{{"--coverage-report", "previous.out"}, {"--use-existing-coverage"}, {"--coverage-command", "go version"}, {"--no-coverage"}, {"--test-command", "go version"}} {
		o, err := q.count(append([]string{"--count", "1"}, args...)...)
		if err != nil {
			return err
		}
		if err = countedProblem(o, "flags.conflict", 80); err != nil {
			return err
		}
		if len(o.traces) != 0 {
			return fmt.Errorf("override admission launched fixture: %v", o.traces)
		}
		if err = strictNoCommands(o.result.Stderr); err != nil {
			return err
		}
	}
	return nil
}
