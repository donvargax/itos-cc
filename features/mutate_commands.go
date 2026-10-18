package features

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Running mutants: the test command a source's project gives its mutants, and
// the whole suite an end-to-end module needs.

// The projects of "Each file runs its narrowest test command", each a
// source file and a test that reaches it.
const (
	pyBoard     = "def place(row, col):\n    return row > 0\n"
	pytestBoard = "from board import place\n\n\ndef test_place():\n    assert place(1, 2)\n    assert not place(0, 2)\n"
	unitBoard   = "import unittest\n\nfrom board import place\n\n\nclass TestPlace(unittest.TestCase):\n" +
		"    def test_place(self):\n        self.assertTrue(place(1, 2))\n        self.assertFalse(place(0, 2))\n"
	ktTest = "package board\n\nimport kotlin.test.Test\nimport kotlin.test.assertEquals\n\nclass BoardTest {\n" +
		"    @Test\n    fun placesPastOne() {\n        assertEquals(1, place(5))\n    }\n}\n"
	pyproject = "[project]\nname = \"board\"\nversion = \"0\"\n"
	// noPytest shadows pytest in the project's folder, where itos-cc asks
	// python whether it can import it.
	noPytest = "raise ImportError(\"pytest is not importable in this project\")\n"
)

// sourceInProject writes a project of language with what has says, its
// source file and a test that reaches it, and declares what its tests do.
func (m *mutateState) sourceInProject(ctx context.Context, language, has string) error {
	m.language, m.has = language, has
	w := m.w
	files := map[string]string{}
	var tests *Tests
	switch language + "/" + has {
	case "Go/go.mod":
		m.file = "src/board.go"
		files["go.mod"], files[m.file], files["src/board_test.go"] = mutateModule, boardGo, placeTest+clearTest
		tests = &Tests{Language: "go", Command: "go test -count=1 -failfast ./src"}
	case "TypeScript/vitest":
		m.file = "src/board.ts"
		files["package.json"] = `{"name": "board", "type": "module", "devDependencies": {"vitest": "5.0.2", "@vitest/coverage-v8": "5.0.2"}}` + "\n"
		files[m.file] = tsSource
		files["src/board.test.ts"] = "import { expect, test } from \"vitest\";\nimport { place } from \"./board\";\n\n" +
			"test(\"place\", () => {\n  expect(place(5)).toBe(3);\n});\n"
		tests = &Tests{Language: "typescript", Command: "node_modules/.bin/vitest related --run --bail=1 src/board.ts"}
	case "TypeScript/jest":
		// Jest is installed as a placeholder, which cannot run: the
		// scenario reads which command itos-cc chose.
		m.file = "src/board.ts"
		files["package.json"] = `{"name": "board", "devDependencies": {"jest": "30.0.0"}}` + "\n"
		files[m.file] = tsSource
		if err := (&coverageState{w: w}).placeholder(".", "jest", "30.0.0"); err != nil {
			return err
		}
	case "TypeScript/neither installed":
		// The project declares pnpm, which no run finds, so its script
		// never runs: the scenario reads which command itos-cc chose.
		m.file, m.pm = "src/board.ts", "pnpm"
		files["package.json"] = `{"name": "board", "packageManager": "pnpm@9.12.0", "scripts": {"test": "node -e \"\""}}` + "\n"
		files[m.file] = tsSource
		w.without = append(w.without, "pnpm")
	case "Python/pytest importable":
		m.file = "src/board.py"
		files["pyproject.toml"], files[m.file], files["tests/test_board.py"] = pyproject, pyBoard, pytestBoard
		tests = &Tests{Language: "python", Command: "python3 -m pytest -q -x -p no:cacheprovider tests/test_board.py"}
	case "Python/no pytest":
		m.file = "src/board.py"
		files["pyproject.toml"], files[m.file], files["tests/test_board.py"], files["pytest.py"] = pyproject, pyBoard, unitBoard, noPytest
		tests = &Tests{Language: "python", Command: "python3 -m unittest -f tests.test_board"}
	case "Kotlin/Gradle", "Kotlin/Maven":
		// Gradle and Maven are off the PATH, so neither runs: the scenario
		// reads which command itos-cc chose.
		m.file = "src/main/kotlin/board/Board.kt"
		files[m.file], files["src/test/kotlin/board/BoardTest.kt"] = ktSource, ktTest
		if has == "Gradle" {
			files["build.gradle.kts"], files["settings.gradle.kts"] = kotlinBuild("build.gradle.kts"), "rootProject.name = \"board\"\n"
		} else {
			files["pom.xml"] = kotlinBuild("pom.xml")
		}
		w.without = append(w.without, "gradle", "mvn")
	default:
		return fmt.Errorf("no %s project is written with %s", language, has)
	}
	if err := m.writeAll(files); err != nil {
		return err
	}
	if language == "TypeScript" && has == "vitest" {
		if err := w.d.Coverage(w.dir, Coverage{Language: "typescript"}); err != nil {
			return err
		}
	}
	switch language {
	case "TypeScript":
		if _, exists := files["src/board.test.ts"]; exists {
			w.imports(map[string][]string{"src/board.test.ts": {m.file}})
		}
	case "Python":
		w.imports(map[string][]string{"tests/test_board.py": {m.file}})
	case "Kotlin":
		w.imports(map[string][]string{"src/test/kotlin/board/BoardTest.kt": {m.file}})
	}
	if tests != nil {
		if err := w.d.Tests(w.dir, *tests); err != nil {
			return err
		}
	}
	setup := ToolSetup{Env: w.env()}
	if language == "TypeScript" && (has == "vitest" || has == "jest") {
		setup.NodeBins = []string{has}
	}
	if language == "Python" {
		setup.Python = &PythonProbe{Interpreter: "python3", Importable: has == "pytest importable"}
	}
	return w.d.Tools(ctx, w.dir, setup)
}

// itsMutantsRun runs mutation run over the file the Given wrote, without
// coverage, which decides nothing of the command.
func (m *mutateState) itsMutantsRun() error {
	if m.file == "" {
		return errors.New("no Given wrote a source file")
	}
	return m.w.runEnv(m.w.dir, m.w.env(), "mutation", "run", "--no-coverage", filepath.FromSlash(m.file))
}

// pythonName is a Python interpreter's name.
var pythonName = regexp.MustCompile(`^python(?:3(?:\.\d+)?)?(?:\.exe)?$`)

// testCommandIs is whether the baseline the last run announced is the
// command spec says: its words, in order, among the command's, the first
// the command's program; "the installed <tool>" the project's
// node_modules/.bin/<tool>, "python" any Python interpreter, <pm> the
// package manager the project declares and <the file's test modules> the
// modules of the tests that reach the file. "on the file's package" says
// the command names the file's package folder, "on the file" the file,
// and "for the module" Gradle's -p and the module's folder, anywhere among
// its words; "via ./gradlew if present" checks
// a run with a gradlew in the build root runs it instead.
func (m *mutateState) testCommandIs(spec string) error {
	var after []string
	gradlew := false
	for _, q := range []struct {
		suffix string
		words  []string
	}{
		{" on the file's package", []string{"./" + path.Dir(m.file)}},
		{" on the file", []string{m.file}},
		{", the package manager the project declares", nil},
		{" for the module, via ./gradlew if present", []string{"-p", "."}},
	} {
		if rest, ok := strings.CutSuffix(spec, q.suffix); ok {
			spec, after = rest, q.words
			gradlew = strings.Contains(q.suffix, "gradlew")
			break
		}
	}
	installed := ""
	if rest, ok := strings.CutPrefix(spec, "the installed "); ok {
		spec = rest
		installed, _, _ = strings.Cut(rest, " ")
	}
	spec = strings.ReplaceAll(spec, "<pm>", m.pm)
	spec = strings.ReplaceAll(spec, "<the file's test modules>", "tests.test_board")
	want := strings.Fields(spec)
	if err := m.commandHolds(want, after, installed); err != nil {
		return err
	}
	if !gradlew {
		return nil
	}
	if err := m.w.write("gradlew", "#!/bin/sh\nexit 1\n"); err != nil {
		return err
	}
	if err := m.itsMutantsRun(); err != nil {
		return err
	}
	want[0] = "./gradlew"
	return m.commandHolds(want, after, "")
}

// commandHolds is whether the last run announced one baseline, whose
// words hold want, in order, and after, one after another, its program
// want's first, or the project's installed tool of that name.
func (m *mutateState) commandHolds(want, after []string, installed string) error {
	runs := baselines(m.w.stderr)
	if len(runs) != 1 || len(want) == 0 {
		return fmt.Errorf("the run announced %d baselines, not one\n%s", len(runs), m.w.report())
	}
	words := slices.Clone(runs[0].words)
	program := filepath.ToSlash(words[0])
	switch {
	case installed != "":
		if !strings.HasSuffix(program, "/node_modules/.bin/"+installed) && !strings.HasSuffix(program, "/node_modules/.bin/"+installed+".cmd") {
			return fmt.Errorf("the test command %q runs no installed %s\n%s", runs[0].command, installed, m.w.report())
		}
		words[0] = installed
	case want[0] == "python" && pythonName.MatchString(path.Base(program)):
		words[0] = "python"
	}
	if words[0] != want[0] || !subsequence(words, want) || !contiguous(words[1:], after) {
		return fmt.Errorf("the test command is %q, not %s with %s\n%s", runs[0].command, strings.Join(want, " "), strings.Join(after, " "), m.w.report())
	}
	return nil
}

// The module of "By default a file is measured and mutated by its own
// tests": a/a.go, whose package has no test of its own, and e2e/e2e_test.go,
// which imports it and kills every mutant of it.
const (
	e2eSource = "package a\n\n// Half is whether i is past two.\nfunc Half(i int) bool {\n\treturn i > 2\n}\n\n" +
		"// Zero is whether i is zero.\nfunc Zero(i int) bool {\n\treturn i == 0\n}\n"
	e2eTest = "package e2e\n\nimport (\n\t\"testing\"\n\n\t\"example.com/m/a\"\n)\n\nfunc TestE2E(t *testing.T) {\n" +
		"\tif a.Half(2) || !a.Half(3) || !a.Zero(0) || a.Zero(1) {\n\t\tt.Fatal(\"a\")\n\t}\n}\n"
)

// e2eModule writes the module and declares what its tests do and cover.
func (m *mutateState) e2eModule() error {
	m.file = "a/a.go"
	if err := m.writeProject(map[string][]string{"e2e/e2e_test.go": {m.file}}, map[string]string{"go.mod": mutateModule, m.file: e2eSource, "e2e/e2e_test.go": e2eTest}); err != nil {
		return err
	}
	if m.w.d.Exists(filepath.Join(m.w.dir, "a", "a_test.go")) || !strings.Contains(e2eTest, "\"example.com/m/a\"") {
		return errors.New("a/a.go is not tested by e2e/e2e_test.go alone")
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go", Report: "SF:a/a.go\nDA:5,0\nDA:10,0\nend_of_record\n"}); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, Tests{Language: "go", Command: "go test -count=1 -failfast ./..."})
}

// allUncovered is whether the last run left every mutant of file
// uncovered, and with none, ran none.
func (m *mutateState) allUncovered(file string, none bool) error {
	sites, err := m.sitesOf(file)
	if err != nil {
		return err
	}
	s, ok := summaryOf(m.w.stdout, file)
	if !ok || len(sites) == 0 || s.uncovered != len(sites) || s.killed+s.survived != 0 {
		return fmt.Errorf("the run did not leave the %d mutants of %s uncovered\n%s", len(sites), file, m.w.report())
	}
	if none && (s.ran != 0 || len(trials(m.w.stderr, file)) != 0) {
		return fmt.Errorf("the run ran mutants of %s\n%s", file, m.w.report())
	}
	return nil
}

// wholeSuite is whether the last run measured coverage and ran the
// mutants' baseline with commands each of which is one of the list's,
// its words in order among the command's, its program the list's first.
func (m *mutateState) wholeSuite(list string) error {
	var suites [][]string
	for _, one := range regexp.MustCompile(`, or |, `).Split(list, -1) {
		suites = append(suites, strings.Fields(one))
	}
	coverage := (&coverageState{w: m.w}).announced(m.w.stderr)
	runs := baselines(m.w.stderr)
	if len(coverage) == 0 || len(runs) == 0 || len(trials(m.w.stderr, m.file)) == 0 {
		return fmt.Errorf("the run announced no coverage command, baseline or mutant\n%s", m.w.report())
	}
	var commands [][]string
	for _, a := range coverage {
		commands = append(commands, a.words)
	}
	for _, r := range runs {
		commands = append(commands, r.words)
	}
	for _, words := range commands {
		if !slices.ContainsFunc(suites, func(suite []string) bool {
			return path.Base(filepath.ToSlash(words[0])) == suite[0] && subsequence(words, suite)
		}) {
			return fmt.Errorf("%q runs none of %s\n%s", strings.Join(words, " "), list, m.w.report())
		}
	}
	return nil
}

// e2eKills is whether the last run killed every mutant of the file, each
// run.
func (m *mutateState) e2eKills() error {
	sites, err := m.sitesOf(m.file)
	if err != nil {
		return err
	}
	if s, ok := summaryOf(m.w.stdout, m.file); !ok || len(sites) == 0 || s != (summary{killed: len(sites), ran: len(sites)}) {
		return fmt.Errorf("the run did not run and kill the %d mutants of %s\n%s", len(sites), m.file, m.w.report())
	}
	return nil
}

// laterRunReuses runs the last run again without --all-tests, which must
// reuse every kill and run nothing.
func (m *mutateState) laterRunReuses() error {
	if m.w.last == nil || !slices.Contains(m.w.last.args, "--all-tests") {
		return errors.New("no run with --all-tests came before")
	}
	sites, err := m.sitesOf(m.file)
	if err != nil {
		return err
	}
	args := slices.DeleteFunc(slices.Clone(m.w.last.args), func(a string) bool { return a == "--all-tests" })
	if err := m.w.runEnv(m.w.dir, m.w.env(), args...); err != nil {
		return err
	}
	if s, ok := summaryOf(m.w.stdout, m.file); !ok || s != (summary{killed: len(sites), reused: len(sites)}) || len(trials(m.w.stderr, m.file)) != 0 {
		return fmt.Errorf("the run without --all-tests did not reuse the %d kills of %s\n%s", len(sites), m.file, m.w.report())
	}
	return nil
}
