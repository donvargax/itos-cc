package features

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

func (q *countedMutation) admissionSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the complete mutation run and cached mutation sample commands are available$`, q.commandsAvailable)
	sc.Step(`^mutation run is given a nonpositive or noninteger count, or seed without count$`, q.invalidCounts)
	sc.Step(`^it fails with a usage error before any mutation trial$`, q.usageRefused)
	sc.Step(`^counted mode requires one supported Git repository and a resolvable HEAD$`, q.gitAdmission)
	sc.Step(`^existing complete and cached-sample behavior remains unchanged$`, q.completeAndSample)
	sc.Step(`^complete-run and cached-sample fixtures have their existing results$`, q.commandsAvailable)
	sc.Step(`^commands run without the count opt-in$`, q.recordCompleteAndSample)
	sc.Step(`^complete scheduling, scope, cache publication and source annotation behavior are unchanged$`, q.completePreserved)
	sc.Step(`^cached mutation sample still rechecks only its eligible recorded outcomes$`, q.samplePreserved)
	sc.Step(`^its existing nothing-to-sample behavior is not the counted-mode no-cache behavior$`, q.nothingToSample)
}

func (q *countedMutation) commandsAvailable() error {
	if err := q.project(ownedOneSource); err != nil {
		return err
	}
	w := q.e.m.w
	for _, cmd := range []string{"run", "sample"} {
		code, out, log, err := w.capture(w.dir, w.env(), "mutation", cmd, "--help")
		if err != nil {
			return err
		}
		if code != 0 || !strings.Contains(out, "--count N") {
			return fmt.Errorf("counted command availability %s exit%d: %s%s", cmd, code, out, log)
		}
		q.texts["help-"+cmd] = Result{Exit: code, Stdout: out, Stderr: log}
	}
	return nil
}
func (q *countedMutation) invalidCounts() error {
	for _, args := range [][]string{{"--count", "0"}, {"--count=-2"}, {"--count", "two"}, {"--seed", "abc"}} {
		o, err := q.count(args...)
		if err != nil {
			return err
		}
		q.controls = append(q.controls, o)
	}
	return nil
}
func (q *countedMutation) usageRefused() error {
	if len(q.controls) != 4 {
		return fmt.Errorf("usage control population: %d", len(q.controls))
	}
	for i, o := range q.controls {
		rule, flag := "flags.value-invalid", "--count"
		if i == 3 {
			rule, flag = "flags.conflict", "--seed"
		}
		if err := countedProblem(o, rule, 2); err != nil {
			return err
		}
		if checkProblems(o.object)[0]["flag"] != flag {
			return fmt.Errorf("usage flag subject: %v", o.object)
		}
		if err := strictNoCommands(o.result.Stderr); err != nil {
			return err
		}
	}
	w := q.e.m.w
	trace, err := w.d.Read(filepath.Join(q.e.outside, "trace.jsonl"))
	if err != nil {
		return err
	}
	if len(trace) != 0 || w.d.Exists(filepath.Join(w.dir, ".metrics")) {
		return fmt.Errorf("usage refusal ran fixture or published metrics: %s", trace)
	}
	return nil
}
func (q *countedMutation) gitAdmission(ctx context.Context) error {
	w := q.e.m.w
	dir, err := w.d.Folder()
	if err != nil {
		return err
	}
	q.e.extraDirs = append(q.e.extraDirs, dir)
	for file, text := range map[string]string{"go.mod": "module example.com/plain\n\ngo 1.22\n", "main.go": ownedOneSource} {
		if err = w.d.Write(filepath.Join(dir, file), []byte(text)); err != nil {
			return err
		}
	}
	for phase := 0; phase < 2; phase++ {
		if phase == 1 {
			if err = w.d.Git(dir, "init", "-q"); err != nil {
				return err
			}
		}
		o, err := q.observeAt(dir, "mutation", "run", "--json", "--count", "1")
		if err != nil {
			return err
		}
		if err = countedProblem(o, "count.no-git", 3); err != nil {
			return err
		}
		if err = strictNoCommands(o.result.Stderr); err != nil {
			return err
		}
		if w.d.Exists(filepath.Join(dir, ".metrics")) {
			return fmt.Errorf("Git admission refusal published metrics at phase%d", phase)
		}
	}
	q.head, err = w.d.Head(ctx, w.dir)
	if err != nil {
		return err
	}
	if err = q.setMode("uncovered"); err != nil {
		return err
	}
	o, err := q.count("--count", "1", "--workers", "1")
	if err != nil {
		return err
	}
	if o.result.Exit != 0 || countedSampling(o)["commit"] != q.head || countedSampling(o)["seed"] != q.head {
		return fmt.Errorf("resolved Git admission: %v", o.object)
	}
	return countedNoTrials(o)
}
func (q *countedMutation) recordCompleteAndSample() error {
	if err := q.setMode("covered"); err != nil {
		return err
	}
	o, err := q.observe("mutation", "run", "--json", "--workers", "1")
	if err != nil {
		return err
	}
	q.saved["complete"] = o
	o, err = q.observe("mutation", "sample", "--json", "--count", "5", "--workers", "1")
	if err != nil {
		return err
	}
	q.saved["sample"] = o
	return nil
}
func (q *countedMutation) completePreserved() error {
	o, ok := q.saved["complete"]
	if !ok {
		return fmt.Errorf("complete control absent")
	}
	file, err := executionFile(o.object, "main.go")
	if err != nil {
		return err
	}
	mutants := objectList(file, "mutants")
	if o.result.Exit != 0 || o.object["sampling"] != nil || len(mutants) != 1 || mutants[0]["outcome"] != "killed" || mutants[0]["scope"] != "own" || file["ran"] != float64(1) || file["reused"] != float64(0) {
		return fmt.Errorf("complete scheduling or scope changed: %v", o.object)
	}
	w := q.e.m.w
	data, err := w.d.Read(filepath.Join(w.dir, ".metrics", "mutate", "main.go.json"))
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("complete snapshot is empty")
	}
	source, err := w.d.Read(filepath.Join(w.dir, "main.go"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(source), "itos-cc") || !strings.Contains(string(source), "killed") {
		return fmt.Errorf("complete source annotation missing: %s", source)
	}
	return nil
}
func (q *countedMutation) samplePreserved() error {
	o, ok := q.saved["sample"]
	if !ok {
		return fmt.Errorf("sample control absent")
	}
	files := objectList(o.object, "files")
	if o.result.Exit != 0 || o.object["sampling"] != nil || len(files) != 1 || files[0]["file"] != "main.go" {
		return fmt.Errorf("cached sample population: %v", o.object)
	}
	mutants := objectList(files[0], "mutants")
	if len(mutants) != 1 || mutants[0]["recorded"] != "killed" || mutants[0]["outcome"] != "killed" || mutants[0]["scope"] != "own" {
		return fmt.Errorf("cached sample outcome/scope: %v", o.object)
	}
	return nil
}
func (q *countedMutation) completeAndSample() error {
	if err := q.recordCompleteAndSample(); err != nil {
		return err
	}
	if err := q.completePreserved(); err != nil {
		return err
	}
	return q.samplePreserved()
}
func (q *countedMutation) nothingToSample() error {
	w := q.e.m.w
	if err := w.d.Remove(filepath.Join(w.dir, ".metrics", "mutate", "main.go.json")); err != nil {
		return err
	}
	o, err := q.observe("mutation", "sample", "--json")
	if err != nil {
		return err
	}
	if o.result.Exit != 0 || len(objectList(o.object, "files")) != 0 || !strings.Contains(o.result.Stderr, "no cached mutant to sample") {
		return fmt.Errorf("nothing-to-sample control: %v\n%s", o.object, o.result.Stderr)
	}
	if err = strictNoCommands(o.result.Stderr); err != nil {
		return err
	}
	if err = q.setMode("uncovered"); err != nil {
		return err
	}
	fresh, err := q.count("--count", "1", "--seed", q.seed)
	if err != nil {
		return err
	}
	if fresh.result.Exit != 0 || countedSampling(fresh)["eligible"] != float64(1) || countedSampling(fresh)["selected"] != float64(1) || countedSampling(fresh)["completion"] == "not-applicable" {
		return fmt.Errorf("counted no-cache is not cached-sample emptiness: %v", fresh.object)
	}
	return countedNoTrials(fresh)
}
