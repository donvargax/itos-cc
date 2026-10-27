package features

import (
	"path/filepath"

	"github.com/cucumber/godog"
)

// A project under a path holding a comma (bug-13, ID-MUT-392): ID-MUT-117's
// and ID-MUT-122's modules, each written to a folder of the scenario's
// scratch folder named as the Given says, from which the run starts.
func commaPathSteps(sc *godog.ScenarioContext, l *listedMutation) {
	sc.Step(`^a Go module whose only test builds its binary with go build -cover when GOCOVERDIR is set, and runs it, as ID-MUT-117's, at a folder named "([^"]*)"$`, func(name string) error {
		if err := l.m.atFolder(name); err != nil {
			return err
		}
		return l.m.greetModule(true, false)
	})
	sc.Step(`^the module of ID-MUT-122, whose harness splits the run, at a folder named "([^"]*)"$`, func(name string) error {
		if err := l.m.atFolder(name); err != nil {
			return err
		}
		return l.project()
	})
	sc.Step(`^the lines the binary ran are covered, and only the mutants on lines it never ran are uncovered$`, func() error {
		if err := l.m.greetRanCovered(); err != nil {
			return err
		}
		return l.m.greetNeverRanUncovered()
	})
}

// atFolder makes the scenario's project the folder name of its scratch
// folder, so what the Givens after it write, and the run, are there.
func (m *mutateState) atFolder(name string) error {
	dir := filepath.Join(m.w.dir, name)
	if err := m.w.d.Mkdir(dir); err != nil {
		return err
	}
	m.w.dir = dir
	return nil
}
