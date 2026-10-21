package features

import (
	"fmt"
	"path"
	"strings"

	"github.com/cucumber/godog"
)

// A site in a Go case clause the tests run (slice-14, issue #53): Go's
// cover profile gives an empty case clause a block of no statements, and a
// block that ran is covered whatever its statement count.

func emptyCaseSteps(sc *godog.ScenarioContext, m *mutateState) {
	sc.Step(`^the Go module "([^"]*)" with the file "([^"]*)":$`, m.emptyCaseModule)
	sc.Step(`^its test calls (\w+)\((-?\d+)\) and expects "([^"]*)"$`, m.emptyCaseTest)
	sc.Step(`^the mutant of line (\d+)'s (\S+) into (\S+) is (\w+), not uncovered$`, m.emptyCaseOutcome)
}

// emptyCaseModule writes go.mod naming module and file, slash-separated
// from the scratch folder, holding text.
func (m *mutateState) emptyCaseModule(module, file string, text *godog.DocString) error {
	m.file = file
	return m.writeAll(map[string]string{"go.mod": "module " + module + "\n\ngo 1.22\n", file: text.Content + "\n"})
}

// emptyCaseTest writes the test of the Given's file, in its package and
// beside it, calling function with arg and failing unless it returns want,
// and declares that Go measures and runs it.
func (m *mutateState) emptyCaseTest(function, arg, want string) error {
	pkg, ok := emptyCasePackage(m.read(m.file))
	if !ok {
		return fmt.Errorf("%s declares no package", m.file)
	}
	test := strings.TrimSuffix(m.file, path.Ext(m.file)) + "_test.go"
	call := fmt.Sprintf("%s(%s)", function, arg)
	text := fmt.Sprintf("package %s\n\nimport \"testing\"\n\nfunc Test%s(t *testing.T) {\n"+
		"\tif got := %s; got != %q {\n\t\tt.Fatalf(\"%s = %%q, want %%q\", got, %q)\n\t}\n}\n",
		pkg, function, call, want, call, want)
	if err := m.w.write(test, text); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go"}); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, Tests{Language: "go", Command: "go test -count=1 -failfast ./..."})
}

// emptyCasePackage is the package a Go source's clause names.
func emptyCasePackage(source string) (string, bool) {
	for _, line := range strings.Split(source, "\n") {
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), "package "); ok {
			return strings.TrimSpace(name), true
		}
	}
	return "", false
}

// emptyCaseOutcome is whether the last run's --json object gives the one
// mutant of the Given's file on line that turns original into replacement
// the outcome want.
func (m *mutateState) emptyCaseOutcome(line int, original, replacement, want string) error {
	mutants, err := m.greetMutants()
	if err != nil {
		return err
	}
	var found []map[string]any
	for _, mu := range mutants {
		if lineOf(mu) == line && mu["original"] == original && mu["replacement"] == replacement {
			found = append(found, mu)
		}
	}
	if len(found) != 1 {
		return fmt.Errorf("the object lists %d mutants of %s on line %d turning %s into %s, not one\n%s",
			len(found), m.file, line, original, replacement, m.w.report())
	}
	if got := found[0]["outcome"]; got != want {
		return fmt.Errorf("the mutant of line %d's %s into %s is %v, not %s\n%s", line, original, replacement, got, want, m.w.report())
	}
	return nil
}
