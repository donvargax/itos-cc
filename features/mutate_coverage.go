package features

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Coverage decides which mutants run.

// The TypeScript project of "Mutants on lines no test executes are not
// run": score, whose mutant its test kills, and place, which no test
// calls, its one site on line 12, the > at column 9.
const (
	placeBoardTS = `// The board's rules.

// score is whether a row is past the second.
export function score(row: number): boolean {
  return row > 2;
}

// place moves a piece past the fifth column,
// which no test reaches.
export function place(i: number): number {
  let n = i;
  if (i > 5) {
    n = i;
  }
  return n;
}
`
	placeBoardTestTS = "import { expect, test } from \"vitest\";\nimport { score } from \"./board\";\n\n" +
		"test(\"score\", () => {\n  expect(score(2)).toBe(false);\n  expect(score(3)).toBe(true);\n});\n"
	vitestPackage = `{"name": "board", "type": "module", "devDependencies": {"vitest": "5.0.2", "@vitest/coverage-v8": "5.0.2"}}` + "\n"
	vitestCommand = "node_modules/.bin/vitest related --run --bail=1 "
)

// lineNeverExecuted writes the project and declares what its tests do and
// cover, and checks the line holds a site in a function no test calls.
func (m *mutateState) lineNeverExecuted(line int) error {
	m.file = "src/board.ts"
	if err := m.writeProject(map[string][]string{"src/board.test.ts": {m.file}}, map[string]string{"package.json": vitestPackage, m.file: placeBoardTS, "src/board.test.ts": placeBoardTestTS}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "typescript",
		Report: "SF:src/board.ts\nDA:5,1\nDA:11,0\nDA:12,0\nDA:13,0\nDA:15,0\nend_of_record\n"}); err != nil {
		return err
	}
	if err := m.w.d.Tests(m.w.dir, Tests{Language: "typescript", Command: vitestCommand + m.file}); err != nil {
		return err
	}
	on, err := m.sitesWhere(m.file, func(s listedSite) bool { return s.line == line })
	if err != nil {
		return err
	}
	if len(on) == 0 || slices.ContainsFunc(on, func(s listedSite) bool {
		return strings.Contains(placeBoardTestTS, strings.TrimPrefix(s.function, "board#"))
	}) {
		return fmt.Errorf("line %d of %s holds no site in a function no test calls: %v", line, m.file, on)
	}
	return nil
}

// untestedBoard writes the project with no tools declared: no run may run
// its tests.
func (m *mutateState) untestedBoard() error {
	m.file = "src/board.ts"
	return m.writeProject(map[string][]string{"src/board.test.ts": {m.file}}, map[string]string{"package.json": vitestPackage, m.file: placeBoardTS, "src/board.test.ts": placeBoardTestTS})
}

// executedKilled checks every site of the file but those of functions no
// test calls is in one the test calls, every mutant of which it kills.
func (m *mutateState) executedKilled() error {
	sites, err := m.sitesOf(m.file)
	if err != nil {
		return err
	}
	for _, s := range sites {
		if s.function != "board#place" && !strings.Contains(placeBoardTestTS, strings.TrimPrefix(s.function, "board#")+"(") {
			return fmt.Errorf("the site %s is in a function the test does not call", s)
		}
	}
	return nil
}

// lineUncovered is whether the last run left every mutant on the line
// uncovered without running it, and ran the rest.
func (m *mutateState) lineUncovered(line int) error {
	on, err := m.sitesWhere(m.file, func(s listedSite) bool { return s.line == line })
	if err != nil {
		return err
	}
	all, err := m.sitesOf(m.file)
	if err != nil {
		return err
	}
	ran := trials(m.w.stderr, m.file)
	s, ok := summaryOf(m.w.stdout, m.file)
	if !ok || len(on) == 0 || s.uncovered != len(on) || s.ran != len(all)-len(on) || slices.Contains(ran, line) {
		return fmt.Errorf("the run did not leave the %d mutants on line %d uncovered, running the rest\n%s", len(on), line, m.w.report())
	}
	return nil
}

// unusedTS is a source no test imports.
const unusedTS = "export function half(i: number): boolean {\n  return i > 2;\n}\n\nexport function zero(i: number): boolean {\n  return i === 0;\n}\n"

// unusedProject writes a TypeScript project whose test imports
// src/board.ts and never file, and declares what its tests do and cover.
func (m *mutateState) unusedProject(file string) error {
	if file != "src/unused.ts" {
		return fmt.Errorf("no project is written that never loads %s", file)
	}
	m.file = file
	test := "import { expect, test } from \"vitest\";\nimport { place } from \"./board\";\n\n" +
		"test(\"place\", () => {\n  expect(place(5)).toBe(false);\n  expect(place(6)).toBe(true);\n});\n"
	if err := m.writeProject(map[string][]string{"src/board.test.ts": {"src/board.ts"}}, map[string]string{
		"package.json": vitestPackage, "src/board.ts": "export function place(i: number): boolean {\n  return i > 5;\n}\n",
		file: unusedTS, "src/board.test.ts": test,
	}); err != nil {
		return err
	}
	if strings.Contains(test, "unused") {
		return errors.New("the test loads src/unused.ts")
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "typescript", Report: "SF:src/board.ts\nDA:2,1\nend_of_record\n"}); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, Tests{Language: "typescript", Command: vitestCommand + file})
}

// listsThemUncovered runs itos-cc with args, and is whether it lists
// every mutant of the file as uncovered and exits with code.
func (m *mutateState) listsThemUncovered(args string, code int) error {
	if err := m.w.runWith(m.w.env(), args); err != nil {
		return err
	}
	if err := m.everyListedUncovered(m.file); err != nil {
		return err
	}
	return m.w.exitsWith(code)
}

// everyListedUncovered is whether stdout lists every mutant of file as
// uncovered, and no other of it.
func (m *mutateState) everyListedUncovered(file string) error {
	sites, err := m.sitesOf(file)
	if err != nil {
		return err
	}
	if listed := uncoveredListed(m.w.stdout, file); len(sites) == 0 || !sameSites(listed, sites) {
		return fmt.Errorf("stdout lists %v as uncovered, not every mutant of %s, %v\n%s", listed, file, sites, m.w.report())
	}
	return nil
}

// kotlinUnmeasured writes a Kotlin module built with Maven and no JaCoCo,
// so coverage measures none of its files, a test class reaching its
// source, with Gradle and Maven off the PATH.
func (m *mutateState) kotlinUnmeasured() error {
	m.file = "src/Board.kt"
	pom := kotlinBuild("pom.xml")
	if strings.Contains(pom, "jacoco") {
		return errors.New("the pom declares JaCoCo")
	}
	m.w.without = append(m.w.without, "gradle", "mvn")
	return m.writeProject(map[string][]string{"src/test/kotlin/board/BoardTest.kt": {m.file}}, map[string]string{"pom.xml": pom, m.file: ktSource, "src/test/kotlin/board/BoardTest.kt": ktTest})
}

// halfCovered writes the board module whose test calls place alone, so
// coverage would leave clear's mutant uncovered, and it survives a run.
func (m *mutateState) halfCovered() error {
	m.file = "src/board.go"
	if err := m.writeAll(map[string]string{"go.mod": mutateModule, m.file: boardGo, "src/board_test.go": placeTest}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go", Report: "SF:src/board.go\nDA:7,1\nDA:11,0\nend_of_record\n"}); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, Tests{Language: "go", Command: goTests.Command, Surviving: []Site{{m.file, 11, "<", "<="}}})
}

// everyMutantRuns is whether the last run ran every mutant of the file,
// none uncovered, and measured no coverage.
func (m *mutateState) everyMutantRuns() error {
	sites, err := m.sitesOf(m.file)
	if err != nil {
		return err
	}
	s, ok := summaryOf(m.w.stdout, m.file)
	if !ok || len(sites) == 0 || s.ran != len(sites) || s.uncovered != 0 || len(trials(m.w.stderr, m.file)) != len(sites) {
		return fmt.Errorf("the run did not run the %d mutants of %s\n%s", len(sites), m.file, m.w.report())
	}
	if announced := (&coverageState{w: m.w}).announced(m.w.stderr); len(announced) != 0 {
		return fmt.Errorf("the run measured coverage with %q\n%s", announced[0].command, m.w.report())
	}
	return nil
}

// markedTest is the board module's test of both functions, which leaves
// test-ran in its folder when it runs.
const markedTest = "package board\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestPlace(t *testing.T) {\n" +
	"\t_ = os.WriteFile(\"test-ran\", nil, 0o644)\n\tvar b Board\n\tif b.place(5) || !b.place(6) || !b.clear(2) || b.clear(3) {\n" +
	"\t\tt.Fatal(\"board\")\n\t}\n}\n"

// everyReusable writes the board module, its tests killing every mutant,
// and records a run of it, then removes the mark its tests left.
func (m *mutateState) everyReusable() error {
	m.file = "src/board.go"
	if err := m.writeAll(map[string]string{"go.mod": mutateModule, m.file: boardGo, "src/board_test.go": markedTest}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go", Report: "SF:src/board.go\nDA:7,1\nDA:11,1\nend_of_record\n"}); err != nil {
		return err
	}
	if err := m.w.d.Tests(m.w.dir, goTests); err != nil {
		return err
	}
	code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), "mutation", "run")
	if s, ok := summaryOf(stdout, m.file); err != nil || code != 0 || !ok || s.killed != 2 {
		return fmt.Errorf("the previous run did not kill every mutant: exit %d, %v\n%s%s", code, err, stdout, stderr)
	}
	m.snapshot = ".metrics/mutate/" + m.file + ".json"
	m.recorded = m.read(m.snapshot)
	return m.w.d.Remove(filepath.Join(m.w.dir, "src", "test-ran"))
}

// nothingRuns is whether the last run announced no coverage command,
// baseline or mutant, the project's tests left no mark, and a project no
// run recorded holds no .metrics.
func (m *mutateState) nothingRuns() error {
	for _, line := range strings.Split(strings.ReplaceAll(m.w.stderr, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "itos-cc: coverage") || baselineLine.MatchString(line) || trialLine.MatchString(line) {
			return fmt.Errorf("stderr says %q\n%s", line, m.w.report())
		}
	}
	if m.w.d.Exists(filepath.Join(m.w.dir, "src", "test-ran")) {
		return fmt.Errorf("the tests ran: they left src/test-ran\n%s", m.w.report())
	}
	if m.recorded == "" && m.w.d.Exists(filepath.Join(m.w.dir, ".metrics")) {
		return fmt.Errorf("the run left .metrics: a coverage or test command ran\n%s", m.w.report())
	}
	if m.quietCheck != nil {
		return m.quietCheck()
	}
	return nil
}

// progressLinesBegin is whether the last run's stderr announces its
// baseline and each mutant it ran on lines beginning prefix, and every
// line naming a baseline or a mutant's outcome begins so.
func (m *mutateState) progressLinesBegin(prefix string) error {
	about := regexp.MustCompile("baseline .*\\$ |\\[\\d+/\\d+\\] \\S+:\\d+ `")
	base, mutants := 0, 0
	for _, line := range strings.Split(strings.ReplaceAll(m.w.stderr, "\r\n", "\n"), "\n") {
		if !about.MatchString(line) {
			continue
		}
		if !strings.HasPrefix(line, prefix) {
			return fmt.Errorf("the line %q does not begin %q\n%s", line, prefix, m.w.report())
		}
		if strings.Contains(line, "baseline ") {
			base++
		} else {
			mutants++
		}
	}
	if base == 0 || mutants == 0 {
		return fmt.Errorf("stderr announces %d baselines and %d mutants\n%s", base, mutants, m.w.report())
	}
	return nil
}
