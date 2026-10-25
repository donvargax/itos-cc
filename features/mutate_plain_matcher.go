package features

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cucumber/godog"
)

// Plain lines no CI problem matcher reads as errors (slice-51, issue #21,
// q-97): mutation run, check and sample give counts or a verdict first and
// the file after them, so no plain line starts with a path or holds
// path:line:column.

// setupGoMatcher is the pattern of setup-go's Go problem matcher, the
// "regexp" of its matchers.json at the actions/setup-go version
// .github/workflows/cd.yml pins (b7ad1dad31e06c5925ef5d2fc7ad053ef454303e,
// v7.0.0), owner "go": a line it matches is a ##[error] annotation.
const setupGoMatcher = `^\s*(.+\.go):(?:(\d+):(\d+):)? (.*)`

var setupGoPattern = regexp.MustCompile(setupGoMatcher)

// leadingCount is a line that begins with a count, such as "2 killed".
var leadingCount = regexp.MustCompile(`^\d+ \S`)

func plainMatcherSteps(sc *godog.ScenarioContext, m *mutateState) {
	sc.Step(`^its test checks no result, so every mutant of (\S+) survives$`, m.plainUncheckedTest)
	sc.Step(`^"itos-cc ([^"]*)" has recorded its outcomes$`, m.plainRecorded)
	sc.Step(`^no line of its output matches setup-go's Go problem matcher$`, m.plainNoMatcherLine)
	sc.Step(`^its line for (\S+) gives its counts before naming (\S+)$`, m.plainCountsFirst)
}

// plainUncheckedTest writes the test of file, in its package and beside
// it, calling every function of it and checking no result, and declares
// that Go runs it, every mutant of file surviving, and measures it, every
// line of file run, then commits the module, as mutation sample seeds
// its draw with the HEAD commit.
func (m *mutateState) plainUncheckedTest(file string) error {
	pkg, ok := emptyCasePackage(m.read(file))
	if !ok {
		return fmt.Errorf("%s declares no package", file)
	}
	module, ok := strings.CutPrefix(strings.SplitN(m.read("go.mod"), "\n", 2)[0], "module ")
	if !ok {
		return fmt.Errorf("go.mod names no module first\n%s", m.read("go.mod"))
	}
	sites, err := m.sitesOf(file)
	if err != nil {
		return err
	}
	var calls, profile strings.Builder
	surviving := []Site{}
	called := map[string]bool{}
	for _, s := range sites {
		surviving = append(surviving, Site{File: file, Line: s.line, Original: s.original, Replacement: s.replacement})
		name := s.function[strings.LastIndex(s.function, "#")+1:]
		if !called[name] {
			called[name] = true
			_, _ = fmt.Fprintf(&calls, "\t_ = %s(1)\n", name)
			_, _ = fmt.Fprintf(&profile, "%s/%s:%d.1,%d.%d 1 1\n", strings.TrimSpace(module), file, s.line, s.line,
				len(strings.Split(m.read(file), "\n")[s.line-1])+1)
		}
	}
	if len(surviving) == 0 {
		return fmt.Errorf("%s has no mutation site", file)
	}
	test := strings.TrimSuffix(file, filepath.Ext(file)) + "_test.go"
	text := fmt.Sprintf("package %s\n\nimport \"testing\"\n\nfunc TestUnchecked(t *testing.T) {\n%s}\n", pkg, calls.String())
	if err := m.w.write(test, text); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go", Report: "mode: set\n" + profile.String()}); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-qm", "module"}} {
		if err := m.w.git(args...); err != nil {
			return err
		}
	}
	return m.w.d.Tests(m.w.dir, Tests{Language: "go", Command: "go test -count=1 -failfast ./.", Surviving: surviving})
}

// plainRecorded runs itos-cc with args, which must record the outcomes of
// the mutants it judges: it exits 0, or 1 for a survivor.
func (m *mutateState) plainRecorded(args string) error {
	words, err := commandWords(args)
	if err != nil {
		return err
	}
	code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), words...)
	if err != nil {
		return err
	}
	if code != 0 && code != 1 {
		return fmt.Errorf("itos-cc %s exited %d, recording nothing\n--- stdout\n%s--- stderr\n%s", args, code, stdout, stderr)
	}
	return nil
}

// plainNoMatcherLine is whether no line of the last run's standard output
// or standard error matches setup-go's Go problem matcher.
func (m *mutateState) plainNoMatcherLine() error {
	var matched []string
	for _, output := range []string{m.w.stdout, m.w.stderr} {
		for _, line := range outputLines(output) {
			if setupGoPattern.MatchString(strings.TrimSuffix(line, "\r")) {
				matched = append(matched, line)
			}
		}
	}
	if len(matched) > 0 {
		return fmt.Errorf("setup-go's problem matcher reads %q as errors\n%s", matched, m.w.report())
	}
	return nil
}

// plainCountsFirst is whether the last run's standard output has one line
// for file that is not indented, as the survivors listed below it are, and
// that line begins with a count and names file after it.
func (m *mutateState) plainCountsFirst(file, named string) error {
	if file != named {
		return fmt.Errorf("the step names %s, then %s", file, named)
	}
	var lines []string
	for _, line := range outputLines(filepath.ToSlash(m.w.stdout)) {
		if strings.Contains(line, file) && strings.TrimLeft(line, " \t") == line {
			lines = append(lines, line)
		}
	}
	if len(lines) != 1 {
		return fmt.Errorf("stdout has %d lines for %s, not one: %q\n%s", len(lines), file, lines, m.w.report())
	}
	if !leadingCount.MatchString(lines[0]) {
		return fmt.Errorf("the line for %s does not begin with a count: %q\n%s", file, lines[0], m.w.report())
	}
	return nil
}
