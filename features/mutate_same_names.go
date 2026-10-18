package features

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Functions sharing a name.

// The Go module of "Rule: Functions sharing a name": src/setup.go's two
// init functions, both example.com/m/src#init, each setting a variable its
// test checks, so every mutant of both is killed.
const (
	mutateModule = "module example.com/m\n\ngo 1.22\n"
	initID       = "example.com/m/src#init"
	firstInit    = "func init() {\n\ta = 1 + 1\n}\n"
	secondInit   = "func init() {\n\tb = 3 - 1\n}\n"
	setupTest    = "package board\n\nimport \"testing\"\n\nfunc TestSetup(t *testing.T) {\n\tif a != 2 || b != 2 {\n\t\tt.Fatal(\"setup\")\n\t}\n}\n"
	setupSource  = "src/setup.go"
)

// goTests are the tests of the Go modules here, as mutation run runs them
// over src: every mutant they run is killed.
var goTests = Tests{Language: "go", Command: "go test -count=1 -failfast ./src"}

// setupProject writes the module, declares what its tests do and cover,
// and checks each init function has a site.
func (m *mutateState) setupProject() error {
	if err := m.writeAll(map[string]string{
		"go.mod":            mutateModule,
		setupSource:         "package board\n\nvar a, b int\n\n" + firstInit + "\n" + secondInit,
		"src/setup_test.go": setupTest,
	}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go", Report: "SF:src/setup.go\nDA:6,1\nDA:10,1\nend_of_record\n"}); err != nil {
		return err
	}
	if err := m.w.d.Tests(m.w.dir, goTests); err != nil {
		return err
	}
	inits, sites, err := m.initSites()
	if err != nil {
		return err
	}
	if bodies := strings.Split(m.read(setupSource), "func init() {"); len(inits) != 2 || len(bodies) != 3 || bodies[1] == bodies[2] {
		return fmt.Errorf("src/setup.go declares no two init functions with different bodies:\n%s", m.read(setupSource))
	}
	for i, in := range sites {
		if len(in) == 0 {
			return fmt.Errorf("init function %d of src/setup.go has no site: %v", i+1, sites)
		}
	}
	return nil
}

// read is the text of name, slash-separated from the scratch folder, ""
// when there is none.
func (m *mutateState) read(name string) string {
	data, _ := m.w.readIn(m.w.dir, name)
	return string(data)
}

// lineRange is the lines a function spans, its first and its last.
type lineRange struct{ first, last int }

func (l lineRange) holds(line int) bool { return line >= l.first && line <= l.last }

// initSites are the lines each init function of src/setup.go spans now, in
// file order, and the sites mutation list gives each, every one in
// example.com/m/src#init.
func (m *mutateState) initSites() ([]lineRange, [][]listedSite, error) {
	var inits []lineRange
	for i, line := range strings.Split(m.read(setupSource), "\n") {
		switch {
		case line == "func init() {":
			inits = append(inits, lineRange{first: i + 1})
		case line == "}" && len(inits) > 0 && inits[len(inits)-1].last == 0:
			inits[len(inits)-1].last = i + 1
		}
	}
	sites, err := m.sitesOf(setupSource)
	if err != nil {
		return nil, nil, err
	}
	each := make([][]listedSite, len(inits))
	for _, s := range sites {
		i := slices.IndexFunc(inits, func(l lineRange) bool { return l.holds(s.line) })
		if i < 0 || s.function != initID {
			return nil, nil, fmt.Errorf("the site %s is in no init function", s)
		}
		each[i] = append(each[i], s)
	}
	return inits, each, nil
}

// summary is what mutation run says of a file on stdout.
type summary struct{ killed, survived, uncovered, ran, reused int }

var summaryLine = regexp.MustCompile(`(?m)^(\S+): (\d+) killed, (\d+) survived, (\d+) uncovered \(ran (\d+), reused (\d+)\)\r?$`)

// summaryOf is what stdout, a mutation run's, says of file, and whether it
// says anything.
func summaryOf(stdout, file string) (summary, bool) {
	for _, match := range summaryLine.FindAllStringSubmatch(stdout, -1) {
		if filepath.ToSlash(match[1]) != file {
			continue
		}
		n := make([]int, 5)
		for i := range n {
			n[i], _ = strconv.Atoi(match[i+2])
		}
		return summary{n[0], n[1], n[2], n[3], n[4]}, true
	}
	return summary{}, false
}

// trialLine is mutation run's line on stderr for a mutant it ran.
var trialLine = regexp.MustCompile(`(?m)^itos-cc: \[\d+/\d+\] (\S+):(\d+) `)

// trials are the lines of the mutants of file a mutation run's stderr says
// it ran, one for each.
func trials(stderr, file string) []int {
	var out []int
	for _, match := range trialLine.FindAllStringSubmatch(stderr, -1) {
		if filepath.ToSlash(match[1]) == file {
			line, _ := strconv.Atoi(match[2])
			out = append(out, line)
		}
	}
	return out
}

// previousRun runs mutation run over src/setup.go, which must kill every
// mutant of both init functions, and keeps the text the run leaves.
func (m *mutateState) previousRun() error {
	_, each, err := m.initSites()
	if err != nil {
		return err
	}
	all := len(each[0]) + len(each[1])
	code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), "mutation", "run", filepath.FromSlash(setupSource))
	if err != nil {
		return err
	}
	if s, ok := summaryOf(stdout, setupSource); code != 0 || !ok || s != (summary{killed: all, ran: all}) {
		return fmt.Errorf("the previous run did not kill the %d mutants of both init functions: exit %d\n%s%s", all, code, stdout, stderr)
	}
	m.setup = m.read(setupSource)
	return nil
}

// runUnchanged runs itos-cc with args once src/setup.go is as the previous
// run left it.
func (m *mutateState) runUnchanged(args string) error {
	if m.setup == "" || m.read(setupSource) != m.setup {
		return errors.New("src/setup.go is not as a previous run left it")
	}
	return m.w.runWith(m.w.env(), args)
}

// bothReused is whether the last run reused every mutant of both init
// functions and ran none.
func (m *mutateState) bothReused() error {
	_, each, err := m.initSites()
	if err != nil {
		return err
	}
	s, ok := summaryOf(m.w.stdout, setupSource)
	if !ok || len(each[0]) == 0 || len(each[1]) == 0 || s.ran != 0 || s.reused != len(each[0])+len(each[1]) || len(trials(m.w.stderr, setupSource)) != 0 {
		return fmt.Errorf("the run did not reuse the %d and %d mutants of the two init functions alone\n%s", len(each[0]), len(each[1]), m.w.report())
	}
	return nil
}

// secondChanged changes the second init function's body.
func (m *mutateState) secondChanged() error {
	text := m.read(setupSource)
	if !strings.Contains(text, secondInit) {
		return fmt.Errorf("src/setup.go holds no second init function to change:\n%s", text)
	}
	return m.w.write(setupSource, strings.Replace(text, "b = 3 - 1", "b = 4 - 2", 1))
}

// initsSwapped swaps the two init functions, each unchanged.
func (m *mutateState) initsSwapped() error {
	text := m.read(setupSource)
	both := firstInit + "\n" + secondInit
	if !strings.Contains(text, both) {
		return fmt.Errorf("src/setup.go does not hold the two init functions in order:\n%s", text)
	}
	return m.w.write(setupSource, strings.Replace(text, both, secondInit+"\n"+firstInit, 1))
}

// onlyProblemOfSecond is whether the last run's one problem is of rule,
// naming the second init function of src/setup.go by its name and line.
func (m *mutateState) onlyProblemOfSecond(rule string) error {
	object, report, err := m.w.lastObject()
	if err != nil {
		return err
	}
	inits, _, err := m.initSites()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	if len(problems) != 1 {
		return fmt.Errorf("the run reported %d problems, not one\n%s", len(problems), report)
	}
	p, _ := problems[0].(map[string]any)
	file, _ := p["file"].(string)
	if p["rule"] != rule || filepath.ToSlash(file) != setupSource || p["function"] != initID || p["line"] != float64(inits[1].first) {
		return fmt.Errorf("the problem is %v, not %s of %s at line %d\n%s", p, rule, initID, inits[1].first, report)
	}
	return nil
}

// runsOnlySecond is whether a run of args runs the second init function's
// mutants, every one, and no other.
func (m *mutateState) runsOnlySecond(args string) error {
	inits, each, err := m.initSites()
	if err != nil {
		return err
	}
	if err := m.w.runWith(m.w.env(), args); err != nil {
		return err
	}
	ran := trials(m.w.stderr, setupSource)
	s, _ := summaryOf(m.w.stdout, setupSource)
	if len(ran) == 0 || len(ran) != len(each[1]) || s.ran != len(ran) || slices.ContainsFunc(ran, func(l int) bool { return !inits[1].holds(l) }) {
		return fmt.Errorf("the run ran mutants on lines %v, not the %d of the second init function, lines %d to %d\n%s",
			ran, len(each[1]), inits[1].first, inits[1].last, m.w.report())
	}
	return nil
}

// reusesEvery is whether a run of args reuses every mutant of src/setup.go
// and runs none.
func (m *mutateState) reusesEvery(args string) error {
	_, each, err := m.initSites()
	if err != nil {
		return err
	}
	if err := m.w.runWith(m.w.env(), args); err != nil {
		return err
	}
	all := len(each[0]) + len(each[1])
	if s, ok := summaryOf(m.w.stdout, setupSource); !ok || s.ran != 0 || s.reused != all || len(trials(m.w.stderr, setupSource)) != 0 {
		return fmt.Errorf("the run did not reuse the %d mutants of src/setup.go alone\n%s", all, m.w.report())
	}
	return nil
}
