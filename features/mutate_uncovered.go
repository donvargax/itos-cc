package features

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// Uncovered mutants as failures.

// eachListedAs is whether stdout lists every mutant of the file on line
// as text says, and lists one.
func (m *mutateState) eachListedAs(line int, text string) error {
	on, err := m.sitesWhere(m.file, func(s listedSite) bool { return s.line == line })
	if err != nil {
		return err
	}
	var listed []listedSite
	for _, s := range uncoveredListed(m.w.stdout, m.file) {
		if s.line == line {
			listed = append(listed, s)
		}
	}
	if len(on) == 0 || len(listed) != len(on) {
		return fmt.Errorf("stdout lists %d of the %d mutants on line %d as uncovered\n%s", len(listed), len(on), line, m.w.report())
	}
	for _, s := range listed {
		if got := "uncovered " + s.Plain(); got != text {
			return fmt.Errorf("stdout lists %q, not %q\n%s", got, text, m.w.report())
		}
	}
	return nil
}

// commitChanged writes the board module, no test of which executes
// either function, commits it and tags the commit ref, records a run's
// results, place's mutant and clear's uncovered, then commits a change to
// the function named.
func (m *mutateState) commitChanged(ref, function string) error {
	if function != "Board#place" {
		return fmt.Errorf("no commit is made changing %s", function)
	}
	m.file = "src/board.go"
	if err := m.writeAll(map[string]string{"go.mod": mutateModule, m.file: boardGo,
		"src/board_test.go": "package board\n\nimport \"testing\"\n\nfunc TestNothing(t *testing.T) {}\n"}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go", Report: "SF:src/board.go\nDA:7,0\nDA:11,0\nend_of_record\n"}); err != nil {
		return err
	}
	if err := m.w.d.Tests(m.w.dir, goTests); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-qm", ref}, {"tag", ref}} {
		if err := m.w.git(args...); err != nil {
			return err
		}
	}
	if code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), "mutation", "run"); err != nil || code != 0 {
		return fmt.Errorf("the run after %s: exit %d, %v\n%s%s", ref, code, err, stdout, stderr)
	}
	if err := m.w.write(m.file, strings.Replace(boardGo, "return i > 5", "return (i > 5)", 1)); err != nil {
		return err
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-qm", "change place"}} {
		if err := m.w.git(args...); err != nil {
			return err
		}
	}
	return nil
}

// unchangedUnexecuted checks the function is as committed and no test
// calls it.
func (m *mutateState) unchangedUnexecuted(function string) error {
	name := function[strings.LastIndex(function, "#")+1:]
	if !strings.Contains(m.read(m.file), "func (b *Board) "+name+"(i int) bool {\n\treturn i < 3\n}") ||
		strings.Contains(m.read("src/board_test.go"), name+"(") {
		return fmt.Errorf("%s changed, or a test calls it", function)
	}
	return nil
}

// ofFunction is whether s is a site of the function a scenario names, the
// type and method of its name.
func ofFunction(s listedSite, function string) bool {
	return strings.HasSuffix(s.function, "."+function) || strings.HasSuffix(s.function, "/"+function)
}

// functionListedUncovered is whether stdout lists every mutant of the
// function as uncovered.
func (m *mutateState) functionListedUncovered(function string) error {
	sites, err := m.sitesWhere(m.file, func(s listedSite) bool { return ofFunction(s, function) })
	if err != nil {
		return err
	}
	var listed []listedSite
	for _, s := range uncoveredListed(m.w.stdout, m.file) {
		if ofFunction(s, function) {
			listed = append(listed, s)
		}
	}
	if len(sites) == 0 || !sameSites(listed, sites) {
		return fmt.Errorf("stdout lists %v of %s as uncovered, not %v\n%s", listed, function, sites, m.w.report())
	}
	return nil
}

// noneOfFunctionListed is whether stdout lists no mutant of the function
// as uncovered.
func (m *mutateState) noneOfFunctionListed(function string) error {
	for _, s := range uncoveredListed(m.w.stdout, m.file) {
		if ofFunction(s, function) {
			return fmt.Errorf("stdout lists %s as uncovered\n%s", s, m.w.report())
		}
	}
	return nil
}

// uncoveredProblems is whether every mutant the last run's --json object
// lists as uncovered is a problem of rule naming its file, line, column,
// function and texts.
func (m *mutateState) uncoveredProblems(rule string) error {
	object, err := m.w.json()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	files, _ := object["files"].([]any)
	n := 0
	for _, one := range files {
		f, _ := one.(map[string]any)
		list, _ := f["mutants"].([]any)
		for _, mu := range list {
			mutant, _ := mu.(map[string]any)
			if mutant["outcome"] != "uncovered" {
				continue
			}
			n++
			if !slices.ContainsFunc(problems, func(p any) bool {
				problem, _ := p.(map[string]any)
				file, _ := problem["file"].(string)
				named, _ := f["file"].(string)
				if problem["rule"] != rule || filepath.ToSlash(file) != filepath.ToSlash(named) {
					return false
				}
				for _, key := range []string{"line", "column", "function", "original", "replacement"} {
					if problem[key] == nil || problem[key] != mutant[key] {
						return false
					}
				}
				return true
			}) {
				return fmt.Errorf("the uncovered mutant %v is no %s problem naming it\n%s", mutant, rule, m.w.report())
			}
		}
	}
	if n == 0 {
		return fmt.Errorf("the object lists no uncovered mutant\n%s", m.w.report())
	}
	return nil
}
