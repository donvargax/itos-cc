package features

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

// The steps of the Rule "What coverage measures is not changed by an
// exception" (slice-71, decision 64). ID-MUT-393 writes to ID-MUT-122's
// itos-cc.yaml after a run cached its listed tests' coverage; ID-MUT-394
// excepts slice-41's survivor of Count after a strict run recorded its
// coverage evidence. They read only what itos-cc prints and writes.

func exceptionCoverageSteps(sc *godog.ScenarioContext, r *coverageReuse) {
	w := r.l.m.w
	sc.Step(`^an exception, with "itos-cc mutation except" is written to itos-cc\.yaml$`, r.exceptSurvivor)
	sc.Step(`^an "x-note" key at the top is written to itos-cc\.yaml$`, func() error {
		return r.l.replace("itos-cc.yaml", "mutation:", "x-note: reviewed by the team\nmutation:")
	})
	sc.Step(`^an exclude of a file the run does not measure is written to itos-cc\.yaml$`, func() error {
		return r.l.replace("itos-cc.yaml", "mutation:", "exclude:\n  - unused.go\nmutation:")
	})
	sc.Step(`^the listed tests do not run for coverage, and stderr says the listed tests' coverage was reused$`, r.reused)
	sc.Step(`^the listed tests run once for coverage, and stderr says the cached coverage does not serve, naming itos-cc\.yaml$`, func() error {
		if err := r.l.splitCoverage(); err != nil {
			return fmt.Errorf("%w\n%s", err, w.report())
		}
		if r.unserved("itos-cc.yaml") < 0 {
			return fmt.Errorf("no line says the cached coverage does not serve the run, naming itos-cc.yaml\n%s", w.report())
		}
		return nil
	})

	sc.Step(`^"itos-cc (mutation run --fail-uncovered board/board\.go)" has recorded its coverage evidence$`, r.strictRecorded)
	sc.Step(`^I run "itos-cc mutation except" on Count's survivor with --reason '([^']*)'$`, func(reason string) error {
		if err := w.runWith(w.env(), "mutation except "+identityFile+":5:11 --reason '"+reason+"'"); err != nil {
			return err
		}
		return w.exitsWith(0)
	})
	sc.Step(`^"itos-cc (mutation check --fail-uncovered board/board\.go --json)" reports no stale coverage evidence$`, func(args string) error {
		if err := w.runWith(w.env(), args); err != nil {
			return err
		}
		object, err := w.json()
		if err != nil {
			return err
		}
		for _, p := range checkProblems(object) {
			if p["rule"] == "mutation.coverage-stale" {
				return fmt.Errorf("the coverage evidence reads stale: %v\n%s", p, w.report())
			}
		}
		return nil
	})
}

// exceptSurvivor excepts the mutant of main.go that the Given's run left
// surviving, `*` → `/` in n*2, which both listed tests reach and neither
// checks.
func (r *coverageReuse) exceptSurvivor() error {
	w := r.l.m.w
	line := strings.Split(r.l.m.read("main.go"), "\n")[listedStderrLine-1]
	column := strings.Index(line, "n*2") + 2
	if err := w.runWith(w.env(), fmt.Sprintf("mutation except main.go:%d:%d --reason 'n*2 is printed, never checked'", listedStderrLine, column)); err != nil {
		return err
	}
	return w.exitsWith(0)
}

// reused is whether the last run measured no listed test, saying it reused
// their coverage.
func (r *coverageReuse) reused() error {
	if err := r.notMeasured(); err != nil {
		return err
	}
	return r.says(reusedLine)
}

// strictRecorded runs args, a strict run of board/board.go whose survivor
// fails it, then reads that its snapshot records Count's coverage evidence.
func (r *coverageReuse) strictRecorded(args string) error {
	w := r.l.m.w
	if err := w.runWith(w.env(), args); err != nil {
		return err
	}
	if err := w.exitsWith(1); err != nil {
		return err
	}
	data, err := w.readIn(w.dir, ".metrics/mutate/"+identityFile+".json")
	if err != nil {
		return fmt.Errorf("%w\n%s", err, w.report())
	}
	var snap struct {
		Units []struct {
			Name     string `json:"name"`
			Coverage any    `json:"go_coverage"`
		} `json:"units"`
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	if len(snap.Units) == 0 || snap.Units[0].Coverage == nil {
		return fmt.Errorf("%s's snapshot records no coverage evidence for Count\n%s", identityFile, w.report())
	}
	return nil
}
