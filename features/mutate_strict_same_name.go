package features

import (
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

const (
	strictInitsGo = `package main

var ready, set bool

func Positive(n int) bool { return n > 0 }
func init() { ready = true }
func init() { set = true }
func main() {}
`
	strictInitsTest = `package main
import "testing"
func TestSetup(t *testing.T){if !Positive(1)||Positive(0)||!ready||!set{t.Fatal("setup")}}
`
	strictInitsReport = `mode: set
example.com/strict/main.go:5.29,5.43 1 1
example.com/strict/main.go:6.15,6.29 1 1
example.com/strict/main.go:7.15,7.27 1 1
`
	strictInitsChangedReport = `mode: set
example.com/strict/main.go:5.29,5.43 1 1
example.com/strict/main.go:6.15,6.29 1 1
example.com/strict/main.go:7.15,7.29 1 1
`
)

func (q *strictMutation) sameNameSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a Go module whose two init functions a strict run recorded with fresh coverage evidence, every mutant killed$`, func() error {
		if err := q.goProject(strictInitsGo, strictInitsTest, strictInitsReport); err != nil {
			return err
		}
		if _, err := q.fresh("mutation run --fail-uncovered --json "+q.file, 0); err != nil {
			return err
		}
		return q.initsOwnEvidence()
	})
	sc.Step(`^I repeat "itos-cc mutation run --fail-uncovered main\.go" with nothing changed$`, func() error {
		return q.run("mutation run --fail-uncovered " + q.file)
	})
	sc.Step(`^it measures no coverage, and exits 0$`, func() error {
		if strings.Contains(q.m.w.stderr, "itos-cc: coverage ") {
			return fmt.Errorf("the repeated run measured coverage\n%s", q.m.w.report())
		}
		return q.m.w.exitsWith(0)
	})
	sc.Step(`^"itos-cc mutation check --fail-uncovered main\.go" exits 0, each init function holding its own evidence$`, func() error {
		a, log, code, err := q.capture("mutation check --fail-uncovered --json " + q.file)
		if err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("mutation check --fail-uncovered exited %d: %v\n%s", code, a, log)
		}
		return q.initsOwnEvidence()
	})
	sc.Step(`^a Go module whose two init functions a run without coverage recorded, every mutant killed$`, func() error {
		if err := q.goProject(strictInitsGo, strictInitsTest, strictInitsReport); err != nil {
			return err
		}
		if _, err := q.fresh("mutation run --no-coverage --json "+q.file, 0); err != nil {
			return err
		}
		if err := q.m.w.git("init", "-q", "-b", "main"); err != nil {
			return err
		}
		return q.s.commit("base")
	})
	sc.Step(`^a commit after "base" changed only the second init function of main\.go$`, func() error {
		if err := q.m.w.git("tag", "base"); err != nil {
			return err
		}
		changed := strings.Replace(strictInitsGo, "set = true", "set = (true)", 1)
		if err := q.m.w.write(q.file, changed); err != nil {
			return err
		}
		if err := q.declareGo(strictInitsChangedReport); err != nil {
			return err
		}
		return q.s.commit("second init")
	})
	sc.Step(`^it judges only the second init function, reports no coverage problem, and exits 0$`, func() error {
		w := q.m.w
		for _, line := range outputLines(w.stdout) {
			if rule := strings.TrimSpace(line); strings.HasPrefix(rule, "coverage-") || strings.HasPrefix(rule, "uncovered-statement ") {
				return fmt.Errorf("the run reports %q\n%s", rule, w.report())
			}
		}
		if err := w.stdoutSays("(judged 1 of 4 functions)"); err != nil {
			return err
		}
		return w.exitsWith(0)
	})
}

func (q *strictMutation) initsOwnEvidence() error {
	snap, err := q.snapshot()
	if err != nil {
		return err
	}
	hashes := map[string]bool{}
	for _, u := range objectList(snap, "units") {
		if u["name"] != "init" {
			continue
		}
		if _, err := strictEvidence(u); err != nil {
			return err
		}
		hashes[fmt.Sprint(u["hash"])] = true
	}
	if len(hashes) != 2 {
		return fmt.Errorf("the snapshot records no two init functions of their own hashes: %v", snap)
	}
	return nil
}
