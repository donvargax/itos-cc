package features

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

type languageSuiteProject struct {
	language, root, file, test, other, command string
	names                                      []string
}
type languageSuites struct {
	l                         *languageMutation
	projects                  []languageSuiteProject
	initial                   map[string]map[string]any
	checked                   map[string]string
	graph                     map[string]bool
	sampled                   map[string]bool
	partial                   map[string]map[string]any
	supportChecks             []map[string]any
	scope                     string
	postChecked               map[string]string
	postGraph                 map[string]bool
	postSampled               map[string]bool
	partialResult, fullResult map[string]any
}

func (l *languageMutation) suiteSteps(sc *godog.ScenarioContext) {
	u := &languageSuites{l: l}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*u = languageSuites{l: l, initial: map[string]map[string]any{}, checked: map[string]string{}, graph: map[string]bool{}, sampled: map[string]bool{}, partial: map[string]map[string]any{}}
		return ctx, nil
	})
	sc.Step(`^a (TypeScript|Python|Kotlin|Dart) project whose test (e2e/cli\.test\.ts|tests/test_cli\.py|src/test/kotlin/CliTest\.kt|test/cli_test\.dart) does not import the production file$`, func(language, test string) error {
		p, err := u.project(strings.ToLower(language), "", false, true)
		if err != nil {
			return err
		}
		if p.other != test {
			return fmt.Errorf("literal non-importing test %s want%s", p.other, test)
		}
		u.projects = []languageSuiteProject{p}
		l.file = p.file
		return nil
	})
	sc.Step(`^a mutation run with (--all-tests|--test-command "\./run\.sh") recorded an outcome for that file with whole-suite evidence$`, func(scope string) error { u.scope = scope; return u.record(u.projects[0], scope, true) })
	sc.Step(`^only that test file changed since$`, func() error { return l.appendComment(u.projects[0].other) })
	sc.Step(`^its function is stale and the "mutation\.stale" message names the changed test file$`, func() error { p := u.projects[0]; return (&broadLanguage{l: l}).stale("low", p.other, nil) })
	sc.Step(`^no test, list or coverage command runs$`, func() error { return (&broadLanguage{l: l}).noCommands() })

	sc.Step(`^the outcome runs again rather than being reused$`, func() error { return (&broadLanguage{l: l}).executed("low", "killed") })
	sc.Step(`^whole-suite outcomes in a TypeScript, a Python and a Kotlin project with support globs$`, u.supportProjects)
	sc.Step(`^a support file is added, changed or removed$`, u.changeSupport)
	sc.Step(`^each outcome is stale and its message names the support file$`, u.supportStale)
	sc.Step(`^a test file is added only beneath a nested package\.json, a \.venv or node_modules$`, u.nestedTests)
	sc.Step(`^the outer outcome stays fresh$`, u.outerFresh)
	sc.Step(`^a non-Go whole-suite outcome with matching source and importer hashes but no whole-suite evidence$`, u.legacyNonGo)
	sc.Step(`^it is stale and must rerun before it can be reused$`, u.legacyNonGoRerun)
	sc.Step(`^a Go outcome recorded with the existing Go evidence key stays fresh$`, u.legacyGoKey)
	sc.Step(`^TypeScript, Python and Kotlin outcomes recorded with own, all-tests and test-command scopes$`, u.mixedProjects)
	sc.Step(`^only a non-importing test file changed beneath each build root$`, func() error {
		for _, p := range u.projects {
			if err := l.appendComment(p.other); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^their freshness is checked by mutation check, mutation run, mutation sample and the graph$`, u.observeMixed)
	sc.Step(`^the own outcomes stay usable and every whole-suite outcome is stale$`, u.mixedStates)
	sc.Step(`^all four agree on each function's freshness$`, u.mixedAgreement)
	sc.Step(`^a --since run that judges one function never refreshes the evidence of an unjudged one$`, u.partialMixed)
}
func (u *languageSuites) project(language, root string, multi, nonImporting bool) (languageSuiteProject, error) {
	w := u.l.m.w
	p := languageSuiteProject{language: language, root: root, names: []string{"low"}}
	if multi {
		p.names = []string{"own", "all", "command"}
	}
	join := func(s string) string {
		if root == "" {
			return s
		}
		return root + "/" + s
	}
	files := map[string]string{}
	coverage := Coverage{Language: language}
	source := ""
	test := ""
	switch language {
	case "typescript":
		p.file = join("src/board.ts")
		p.test = join("src/board.test.ts")
		p.other = join("e2e/cli.test.ts")
		p.command = "node_modules/.bin/vitest run"
		files[join("package.json")] = `{"name":"board","private":true,"type":"module","devDependencies":{"vitest":"5.0.2","@vitest/coverage-v8":"5.0.2"}}` + "\n"
		test = "import {expect,test} from 'vitest';\nimport * as board from './board';\ntest('board',()=>{\n"
		for i, name := range p.names {
			n := 3 + i*3
			source += "export function " + name + "(i: number): boolean {\n return i === " + strconv.Itoa(n) + ";\n}\n"
			test += " expect(board." + name + "(" + strconv.Itoa(n) + ")).toBe(true);expect(board." + name + "(" + strconv.Itoa(n+1) + ")).toBe(false);\n"
			coverage.Report += "DA:" + strconv.Itoa(i*3+2) + ",1\n"
		}
		if multi {
			test += " if ((board as any).helper) expect((board as any).helper()).toBe(true);\n"
		}
		test += "});\n"
		coverage.Report = "SF:src/board.ts\n" + coverage.Report + "end_of_record\n"
		files[p.other] = "import {test,expect} from 'vitest';\ntest('cli',()=>{expect('cli').toBe('cli');});\n"
		if nonImporting {
			delete(files, p.test)
			test = ""
			files[p.other] = "import {test,expect} from 'vitest';\ntest('cli',async()=>{const name=['..','src','board'].join('/');const b=await import(/* @vite-ignore */ name);expect(b.low(3)).toBe(true);expect(b.low(4)).toBe(false);});\n"
		}
	case "python":
		p.file = join("board.py")
		p.test = join("test_board.py")
		p.other = join("tests/test_cli.py")
		p.command = "python3 -m pytest -q -x -p no:cacheprovider"
		files[join("pyproject.toml")] = "[project]\nname = \"board\"\nversion = \"0.0.0\"\n"
		test = "import board\ndef test_board():\n"
		for i, name := range p.names {
			n := 3 + i*3
			source += "def " + name + "(i):\n    return i == " + strconv.Itoa(n) + "\n\n"
			test += "    assert board." + name + "(" + strconv.Itoa(n) + ")\n    assert not board." + name + "(" + strconv.Itoa(n+1) + ")\n"
			coverage.Report += "DA:" + strconv.Itoa(i*3+2) + ",1\n"
		}
		if multi {
			test += "    if hasattr(board, 'helper'):\n        assert board.helper()\n"
		}
		coverage.Report = "SF:board.py\n" + coverage.Report + "end_of_record\n"
		files[p.other] = "def test_cli():\n    assert 'cli'\n"
		if nonImporting {
			test = ""
			files[p.other] = "import importlib\ndef test_cli():\n    b=importlib.import_module('boa'+'rd')\n    assert b.low(3)\n    assert not b.low(4)\n"
		}
	case "kotlin":
		p.file = join("src/main/kotlin/board/Board.kt")
		p.test = join("src/test/kotlin/board/BoardTest.kt")
		p.other = join("src/test/kotlin/CliTest.kt")
		p.command = "gradle -p . test --fail-fast"
		for file, text := range languageKotlinBuilds["gradle"] {
			files[join(file)] = strings.ReplaceAll(text, "own", "board")
		}
		source = "package board\n\n"
		test = "package board\nimport kotlin.test.*\nclass BoardTest { @Test fun judges(){\n"
		for i, name := range p.names {
			n := 3 + i*3
			source += "fun " + name + "(i: Int): Boolean {\n return i == " + strconv.Itoa(n) + "\n}\n"
			test += " assertTrue(" + name + "(" + strconv.Itoa(n) + "));assertFalse(" + name + "(" + strconv.Itoa(n+1) + "));\n"
			coverage.Report += "<line nr=\"" + strconv.Itoa(i*3+4) + "\" mi=\"0\" ci=\"1\"/>"
		}
		if multi {
			test += " try { val m=Class.forName(\"board.\"+\"BoardKt\").getMethod(\"help\"+\"er\");assertEquals(true,m.invoke(null)); } catch (e: NoSuchMethodException) {}\n"
		}
		test += "} }\n"
		coverage.Program = "gradle"
		coverage.Report = `<report><package name="board"><sourcefile name="Board.kt">` + coverage.Report + `</sourcefile></package></report>`
		files[p.other] = "import kotlin.test.*\nclass CliTest { @Test fun cli(){ assertTrue(true); } }\n"
		if nonImporting {
			test = ""
			files[p.other] = "import kotlin.test.*\nclass CliTest { @Test fun cli(){ val m=Class.forName(\"board.\"+\"BoardKt\").getMethod(\"lo\"+\"w\",Int::class.javaPrimitiveType);assertEquals(true,m.invoke(null,3));assertEquals(false,m.invoke(null,4)); } }\n"
		}
	case "dart":
		// flutter test runs the whole suite as itos-cc words it. The
		// non-importing test runs bin/board.dart, which imports the file,
		// with dart run: flutter_tester has no dart:mirrors to call a
		// library a test does not import.
		p.file = join("lib/board.dart")
		p.test = join("test/board_test.dart")
		p.other = join("test/cli_test.dart")
		p.command = "flutter --suppress-analytics --no-version-check test --no-pub"
		files[join("pubspec.yaml")] = coverageDartFlutterPubspec("board", false)
		test = "import 'package:flutter_test/flutter_test.dart';\nimport 'package:board/board.dart';\n\nvoid main() {\n  test('board', () {\n"
		calls := ""
		for i, name := range p.names {
			n := 3 + i*3
			source += "bool " + name + "(int i) {\n  return i == " + strconv.Itoa(n) + ";\n}\n"
			test += "    expect(" + name + "(" + strconv.Itoa(n) + "), isTrue);\n    expect(" + name + "(" + strconv.Itoa(n+1) + "), isFalse);\n"
			calls += " ${" + name + "(" + strconv.Itoa(n) + ")} ${" + name + "(" + strconv.Itoa(n+1) + ")}"
			coverage.Report += "DA:" + strconv.Itoa(i*3+1) + ",1\nDA:" + strconv.Itoa(i*3+2) + ",1\n"
		}
		test += "  });\n}\n"
		coverage.Report = "SF:lib/board.dart\n" + coverage.Report + "end_of_record\n"
		files[p.other] = "import 'package:flutter_test/flutter_test.dart';\n\nvoid main() {\n  test('cli', () {\n    expect(1 + 1, 2);\n  });\n}\n"
		if nonImporting {
			test = ""
			files[join("bin/board.dart")] = "import 'package:board/board.dart';\n\nvoid main() {\n  print('" + strings.TrimSpace(calls) + "');\n}\n"
			files[p.other] = "import 'dart:io';\n\nimport 'package:flutter_test/flutter_test.dart';\n\nvoid main() {\n  test('cli', () {\n" +
				"    final run = Process.runSync('dart', ['run', 'bin/board.dart']);\n" +
				"    expect(run.stdout.toString().trim(), 'true false', reason: run.stderr.toString());\n  });\n}\n"
		}
	}
	files[p.file] = source
	if test != "" {
		files[p.test] = test
	}
	for file, text := range files {
		if err := w.write(file, text); err != nil {
			return p, err
		}
	}
	dir := filepath.Join(w.dir, filepath.FromSlash(root))
	if err := w.d.Coverage(dir, coverage); err != nil {
		return p, err
	}
	if err := w.d.Tests(dir, Tests{Language: language, Command: p.command}); err != nil {
		return p, err
	}
	// --all-tests runs the whole suite as itos-cc words it, where the
	// --test-command above words it another way.
	if all := map[string]string{"typescript": "node_modules/.bin/vitest run --bail=1", "kotlin": "gradle test --fail-fast"}[language]; all != "" {
		if err := w.d.Tests(dir, Tests{Language: language, Command: all}); err != nil {
			return p, err
		}
	}
	if language == "python" && nonImporting {
		if err := w.d.WriteExecutable(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\nexec python3 -m pytest -q -p no:cacheprovider\n")); err != nil {
			return p, err
		}
		if err := w.d.Tests(dir, Tests{Language: "python", Command: "./run.sh"}); err != nil {
			return p, err
		}
	}
	w.variables = append(w.variables, "PYTEST_DISABLE_PLUGIN_AUTOLOAD=1")
	return p, nil
}
func (u *languageSuites) record(p languageSuiteProject, scope string, killed bool) error {
	l := u.l
	a, log, code, err := l.capture("mutation run --json --no-coverage " + scope + " " + p.file)
	if err != nil {
		return err
	}
	if code > 1 {
		return fmt.Errorf("whole-suite record exit%d %v\n%s", code, a, log)
	}
	f, err := languageRunFile(a, p.file)
	if err != nil {
		return err
	}
	if killed && (f["killed"] != float64(len(p.names)) || f["ran"] != float64(len(p.names))) {
		return fmt.Errorf("whole-suite fixture did not kill expected population: %v", f)
	}
	snap, err := l.s.snapshot(p.file)
	if err != nil {
		return err
	}
	for _, unit := range objectList(snap, "units") {
		for _, m := range objectList(unit, "mutants") {
			ev, ok := m["suite_evidence"].(map[string]any)
			if !ok {
				return fmt.Errorf("whole-suite outcome lacks evidence: %v", m)
			}
			tests, _ := ev["tests"].(map[string]any)
			if tests[p.other] == nil {
				return fmt.Errorf("whole-suite evidence lacks non-importer%s: %v", p.other, ev)
			}
		}
	}
	u.initial[p.file] = snap
	return nil
}
func (u *languageSuites) supportProjects() error {
	w := u.l.m.w
	if err := w.write("itos-cc.yaml", "mutation:\n  tests:\n    list: go version\n    run: go version {pattern}\n    ids_pattern: \"{ids}\"\n    join:\n      each: \"{id}\"\n      sep: \",\"\n    support: [\"features/*.feature\"]\n"); err != nil {
		return err
	}
	for _, language := range []string{"typescript", "python", "kotlin"} {
		p, err := u.project(language, language, false, false)
		if err != nil {
			return err
		}
		u.projects = append(u.projects, p)
	}
	return u.recordSupport()
}
func (u *languageSuites) recordSupport() error {
	l := u.l
	for _, p := range u.projects {
		sites, err := l.m.sitesOf(p.file)
		if err != nil {
			return err
		}
		t := Tests{Language: "go", Command: "go version"}
		for _, s := range sites {
			t.Surviving = append(t.Surviving, Site{File: strings.TrimPrefix(p.file, p.root+"/"), Line: s.line, Original: s.original, Replacement: s.replacement})
		}
		if err := l.m.w.d.Tests(filepath.Join(l.m.w.dir, p.root), t); err != nil {
			return err
		}
		if err := u.record(p, `--test-command "go version"`, false); err != nil {
			return err
		}
	}
	return nil
}
func (u *languageSuites) changeSupport() error {
	l, w := u.l, u.l.m.w
	for _, change := range []string{"added", "changed", "removed"} {
		if change == "added" {
			if err := w.d.Remove(filepath.Join(w.dir, "features/example.feature")); err != nil {
				return err
			}
		} else {
			if err := w.write("features/example.feature", "Feature: original\n"); err != nil {
				return err
			}
		}
		if err := u.recordSupport(); err != nil {
			return err
		}
		if change == "removed" {
			if err := w.d.Remove(filepath.Join(w.dir, "features/example.feature")); err != nil {
				return err
			}
		} else {
			if err := w.write("features/example.feature", "Feature: "+change+"\n"); err != nil {
				return err
			}
		}
		for _, p := range u.projects {
			a, _, _, err := l.capture("mutation check --json " + p.file)
			if err != nil {
				return err
			}
			u.supportChecks = append(u.supportChecks, a)
		}
	}
	return nil
}
func (u *languageSuites) supportStale() error {
	if len(u.supportChecks) != 9 {
		return fmt.Errorf("missing support change/language population: %d", len(u.supportChecks))
	}
	for i, a := range u.supportChecks {
		p := u.projects[i%3]
		old := u.l.file
		u.l.file = p.file
		err := (&broadLanguage{l: u.l}).stale("low", "features/example.feature", a)
		u.l.file = old
		if err != nil {
			return err
		}
	}
	return nil
}
func (u *languageSuites) nestedTests() error {
	if err := u.recordSupport(); err != nil {
		return err
	}
	w := u.l.m.w
	for file, text := range map[string]string{"typescript/nested/package.json": "{\"name\":\"nested\"}\n", "typescript/nested/new.test.ts": "test('nested',()=>{});\n", "python/.venv/test_ignored.py": "def test_ignored():\n    assert False\n", "kotlin/node_modules/src/test/kotlin/OtherTest.kt": "class OtherTest\n"} {
		if err := w.write(file, text); err != nil {
			return err
		}
	}
	return nil
}
func (u *languageSuites) outerFresh() error {
	for _, p := range u.projects {
		a, _, _, err := u.l.capture("mutation check --json " + p.file)
		if err != nil {
			return err
		}
		old := u.l.file
		u.l.file = p.file
		err = (&broadLanguage{l: u.l}).freshAll(a)
		u.l.file = old
		if err != nil {
			return err
		}
	}
	return nil
}
func (u *languageSuites) legacyNonGo() error {
	p := u.projects[0]
	a, err := u.l.s.snapshot(p.file)
	if err != nil {
		return err
	}
	for _, unit := range objectList(a, "units") {
		for _, m := range objectList(unit, "mutants") {
			delete(m, "suite_evidence")
			delete(m, "go_evidence")
		}
	}
	return u.l.writeSnapshot(p.file, a)
}
func (u *languageSuites) legacyNonGoRerun() error {
	l, p := u.l, u.projects[0]
	old := l.file
	l.file = p.file
	defer func() { l.file = old }()
	if err := (&broadLanguage{l: l}).stale("low", "whole-suite evidence", nil); err != nil {
		return err
	}
	if err := l.run("mutation run --no-coverage --json --test-command 'go version' " + p.file); err != nil {
		return err
	}
	return (&broadLanguage{l: l}).executed("low", "survived")
}
func (u *languageSuites) legacyGoKey() error {
	l, w := u.l, u.l.m.w
	root := "go"
	file := "go/main.go"
	for path, text := range map[string]string{"go/go.mod": "module example.com/legacy\n\ngo 1.22\n", file: "package legacy\nfunc Low()bool{return true}\n", "go/main_test.go": "package legacy\nimport \"testing\"\nfunc TestLow(t *testing.T){if !Low(){t.Fatal(\"Low\")}}\n"} {
		if err := w.write(path, text); err != nil {
			return err
		}
	}
	if err := w.d.Tests(filepath.Join(w.dir, root), Tests{Language: "go", Command: "go test -count=1 -failfast ./..."}); err != nil {
		return err
	}
	a, _, code, err := l.capture("mutation run --all-tests --no-coverage --json " + file)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("the Go legacy seed exit%d: %v", code, a)
	}
	snap, err := l.s.snapshot(file)
	if err != nil {
		return err
	}
	for _, unit := range objectList(snap, "units") {
		for _, m := range objectList(unit, "mutants") {
			m["go_evidence"] = m["suite_evidence"]
			delete(m, "suite_evidence")
		}
	}
	if err := l.writeSnapshot(file, snap); err != nil {
		return err
	}
	a, _, _, err = l.capture("mutation check --json " + file)
	if err != nil {
		return err
	}
	old := l.file
	l.file = file
	defer func() { l.file = old }()
	return (&broadLanguage{l: l}).freshAll(a)
}
