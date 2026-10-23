package features

import (
	"encoding/json"
	"fmt"

	"github.com/cucumber/godog"
)

// slice-40: a kill decided by listed tests rests on those tests and the
// support files, not on the tests that import its file (MUT-308 to 317).
// main_test.go, the own package's test, is the importing test the
// scenarios change; every observation is the --json output of a command
// or the raw snapshot.

func listedImporterSteps(sc *godog.ScenarioContext, l *listedMutation) {
	sc.Step(`^main_test\.go, a test of main\.go's own package, has changed since$`, l.importerChanged)
	sc.Step(`^"greet" and "main", whose kills were all by listed tests, are fresh, and "half", killed by its own test, is "mutation\.stale", its message naming main_test\.go$`, func() error {
		if err := l.functionsFresh("greet", "main"); err != nil {
			return err
		}
		return l.stale([]string{"half"}, "main_test.go")
	})
	sc.Step(`^that run judges the mutant of "half" again$`, func() error { return l.judgedAgain(listedHalfLine) })
	sc.Step(`^"half", killed by its own test, is "mutation\.stale", its message naming main_test\.go$`, func() error { return l.stale([]string{"half"}, "main_test.go") })
	sc.Step(`^"greet", whose kills were all by listed tests, is fresh$`, func() error { return l.functionsFresh("greet") })
	sc.Step(`^"main", holding a survivor of the listed tests, "half", killed by its own test, and "twice", uncovered, are each "mutation\.stale", naming main_test\.go$`, func() error {
		return l.stale([]string{"main", "half", "twice"}, "main_test.go")
	})
	sc.Step(`^"itos-cc mutation run" judges the survivor of "main" and the mutant of "half" again$`, func() error {
		if err := l.m.w.runWith(l.m.w.env(), "mutation run --json main.go"); err != nil {
			return err
		}
		return l.judgedAgain(listedStderrLine, listedHalfLine)
	})
	sc.Step(`^a commit after "base" changed main_test\.go and "twice", and nothing else of main\.go$`, l.commitAfterBase)
	sc.Step(`^"itos-cc mutation check" calls "greet" and "main" fresh, and "half" "mutation\.stale", kept by a mutation run that did not judge it$`, func() error {
		if err := l.m.w.runWith(l.m.w.env(), "mutation check --json main.go"); err != nil {
			return err
		}
		if err := l.functionsFresh("greet", "main"); err != nil {
			return err
		}
		return l.stale([]string{"half"}, "kept by a mutation run that did not judge it")
	})
	sc.Step(`^main_test\.go has changed since a run recorded a kill by ID-A-02$`, func() error {
		if err := l.freshRun(); err != nil {
			return err
		}
		return l.importerChanged()
	})
	sc.Step(`^the function holding that kill is not marked stale and "half" is, as mutation check calls them$`, l.graphAgreesImporter)
	sc.Step(`^"greet" is "mutation\.stale", its message naming main_test\.go$`, func() error { return l.stale([]string{"greet"}, "main_test.go") })
}

// importerChanged edits main_test.go, once the snapshot the run wrote
// records it among the tests that import main.go: without that, the
// scenario's change would test nothing.
func (l *listedMutation) importerChanged() error {
	snap, err := l.snapshot()
	if err != nil {
		return fmt.Errorf("main.go's snapshot: %w\n%s", err, l.m.w.report())
	}
	tests, _ := snap["tests"].(map[string]any)
	if _, ok := tests["main_test.go"]; !ok {
		return fmt.Errorf("main.go's snapshot records no main_test.go among the tests that import it: %v", tests)
	}
	return l.edit("main_test.go")
}

// functionsFresh is whether the last mutation check's --json object calls each of
// functions, of main.go, fresh.
func (l *listedMutation) functionsFresh(functions ...string) error {
	object, err := l.checkObject()
	if err != nil {
		return err
	}
	states := map[string]any{}
	files, _ := object["files"].([]any)
	for _, f := range files {
		list, _ := f.(map[string]any)["functions"].([]any)
		for _, fn := range list {
			v := fn.(map[string]any)
			states[fmt.Sprint(v["function"])] = v["state"]
		}
	}
	for _, fn := range functions {
		if states["example.com/greet#"+fn] != "fresh" {
			return fmt.Errorf("%s is %v, not fresh\n%s", fn, states["example.com/greet#"+fn], l.m.w.report())
		}
	}
	return nil
}

// judgedAgain is whether the last mutation run's --json object holds
// mutants on each of lines and reuses none of them.
func (l *listedMutation) judgedAgain(lines ...int) error {
	entries, err := l.m.greetMutants()
	if err != nil {
		return err
	}
	for _, line := range lines {
		if err := l.assertLines(entries, line, func(e map[string]any) bool { return e["reused"] != true }); err != nil {
			return err
		}
	}
	return nil
}

// commitAfterBase commits the fixture as the run left it, tags it base,
// then commits main_test.go changed and twice multiplying by 3, the same
// site, and nothing else of main.go.
func (l *listedMutation) commitAfterBase() error {
	if err := l.freshRun(); err != nil {
		return err
	}
	w := l.m.w
	for _, args := range [][]string{{"add", "."}, {"commit", "-qm", "recorded"}, {"tag", "base"}} {
		if err := w.d.Git(w.dir, args...); err != nil {
			return err
		}
	}
	if err := l.importerChanged(); err != nil {
		return err
	}
	if err := l.replace("main.go", "return i * 2", "return i * 3"); err != nil {
		return err
	}
	for _, args := range [][]string{{"add", "main.go", "main_test.go"}, {"commit", "-qm", "changed"}} {
		if err := w.d.Git(w.dir, args...); err != nil {
			return err
		}
	}
	if err := l.declare(); err != nil {
		return err
	}
	return l.clearRecords()
}

// graphAgreesImporter is whether the graph built leaves greet, which holds
// the kill by ID-A-02, unmarked and marks half stale, and mutation check
// calls them so.
func (l *listedMutation) graphAgreesImporter() error {
	var graph struct {
		Nodes []struct {
			Units []struct {
				File, Namespace, Name string
				Stale                 bool
			}
		}
	}
	if err := json.Unmarshal(l.graph, &graph); err != nil {
		return err
	}
	states := map[string]bool{}
	for _, n := range graph.Nodes {
		for _, u := range n.Units {
			if u.File == "main.go" {
				states[u.Namespace+"#"+u.Name] = u.Stale
			}
		}
	}
	if _, ok := states["example.com/greet#greet"]; !ok {
		return fmt.Errorf("the graph has no greet: %v", states)
	}
	if states["example.com/greet#greet"] || !states["example.com/greet#half"] {
		return fmt.Errorf("graph freshness %v", states)
	}
	if err := l.m.w.runWith(l.m.w.env(), "mutation check --json main.go"); err != nil {
		return err
	}
	if err := l.functionsFresh("greet"); err != nil {
		return err
	}
	return l.stale([]string{"half"}, "main_test.go")
}
