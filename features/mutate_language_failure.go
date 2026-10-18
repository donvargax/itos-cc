package features

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/cucumber/godog"
)

func (l *languageMutation) failureSteps(sc *godog.ScenarioContext) {
	var goOnly map[string]any
	sc.Step(`^a project with a Go module and a TypeScript file that has no go\.mod above it, and no TypeScript tool$`, l.mixedStrictProject)
	sc.Step(`^the Go functions have fresh complete coverage evidence$`, func() error {
		a, _, code, err := l.capture("mutation run --fail-uncovered --json gomod/entry.go")
		if err != nil {
			return err
		}
		if code != 1 {
			return fmt.Errorf("the Go strict setup exit%d: %v", code, a)
		}
		snap, err := l.s.snapshot("gomod/entry.go")
		if err != nil {
			return err
		}
		units := objectList(snap, "units")
		if len(units) != 2 {
			return fmt.Errorf("missing Go units: %v", snap)
		}
		for _, u := range units {
			c, ok := u["go_coverage"].(map[string]any)
			if !ok || c["complete"] != true {
				return fmt.Errorf("incomplete Go coverage: %v", u)
			}
		}
		goOnly, _, _, err = l.capture("mutation check --fail-uncovered --json gomod/entry.go")
		return err
	})
	sc.Step(`^I run "itos-cc mutation check --fail-uncovered --json" over both files$`, func() error { return l.run("mutation check --fail-uncovered --json gomod/entry.go web/label.ts") })
	sc.Step(`^the TypeScript function is reported as mutation\.coverage-missing of TypeScript line evidence, never of Go evidence, and nothing is an error$`, func() error { return l.typeScriptOnlyEvidence("web/label.ts") })
	sc.Step(`^the Go functions are judged as before$`, func() error {
		a, err := l.object()
		if err != nil {
			return err
		}
		return sameGoFindings(a, goOnly)
	})
	sc.Step(`^a TypeScript file sits beneath the Go module's directory$`, func() error {
		if err := l.m.w.write("gomod/web/label.ts", languageIdentityTS); err != nil {
			return err
		}
		return l.run("mutation check --fail-uncovered --json gomod/entry.go gomod/web/label.ts")
	})
	sc.Step(`^it is still not judged for Go coverage evidence$`, func() error {
		if err := l.typeScriptOnlyEvidence("gomod/web/label.ts"); err != nil {
			return err
		}
		a, err := l.object()
		if err != nil {
			return err
		}
		return sameGoFindings(a, goOnly)
	})
	sc.Step(`^a strict mutation run over a mixed selection can reuse fresh Go evidence instead of measuring Go every time$`, func() error {
		a, log, code, err := l.capture("mutation run --fail-uncovered --json gomod/entry.go web/label.ts gomod/web/label.ts")
		if err != nil {
			return err
		}
		if code != 1 || strings.Contains(log, "go test -count=1 -covermode=set") {
			return fmt.Errorf("fresh mixed Go evidence remeasured exit%d: %v\n%s", code, a, log)
		}
		return sameGoFindings(a, goOnly)
	})
	sc.Step(`^a (TypeScript|Python|Kotlin) project whose coverage (tool is missing|command exits 0 but writes no report|command fails)$`, l.failedCoverage)
	sc.Step(`^the run fails with "([^"]*)" naming the language and directory, and the exit code is (\d+)$`, l.coverageFailed)
	sc.Step(`^no mutant of that file is judged or reported as covered$`, l.noJudgment)
	sc.Step(`^without --fail-uncovered the run falls back to running every mutant as before$`, l.failureFallback)
	sc.Step(`^a committed project with a Go module and a TypeScript file with no mutation site$`, l.unjudgedCountProject)
	sc.Step(`^no TypeScript coverage tool is installed$`, func() error {
		if l.m.w.d.Exists(filepath.Join(l.m.w.dir, "node_modules")) {
			return fmt.Errorf("unexpected TS tool")
		}
		return nil
	})
	sc.Step(`^a strict count-one run judges both files$`, func() error {
		return l.run("mutation run --count 1 --seed 4813e48 --fail-uncovered --json board.go label.ts")
	})
	sc.Step(`^its Go functions are judged for coverage evidence and one Go mutant is judged with a real outcome$`, l.countedGo)
	sc.Step(`^no stage fails and no TypeScript coverage is measured$`, l.noTypeScriptPreparation)
	sc.Step(`^when the TypeScript file has an eligible site the run fails with "count\.preparation-failed" at its coverage stage, as before$`, l.eligibleTypeScriptFails)
	sc.Step(`^a TypeScript project whose test script runs Node's own test runner, with neither Vitest nor Jest installed$`, l.nodeFallbackProject)
	sc.Step(`^a mutant of one of its files has a result from "itos-cc mutation run"$`, l.recordNodeFallback)
	sc.Step(`^a test that reaches no module of that file changes$`, func() error { return l.appendComment("test/other.test.ts") })
	sc.Step(`^"itos-cc mutation check" reports that result stale, as it reports a --test-command outcome$`, func() error { b := broadLanguage{l: l}; return b.stale("low", "test/other.test.ts", nil) })
}

const languageIdentityTS = "export function label(name: string): string {\n  return name;\n}\n"

func (l *languageMutation) mixedStrictProject() error {
	w := l.m.w
	for file, text := range map[string]string{
		"gomod/go.mod":        "module example.com/mixed\n\ngo 1.22\n",
		"gomod/entry.go":      "package mixed\n\nfunc Entry(active bool) string { if active { return \"active\" }; return \"inactive\" }\n\nfunc Dormant() string { return \"dormant\" }\n",
		"gomod/entry_test.go": "package mixed\nimport \"testing\"\nfunc TestEntry(t *testing.T) { if Entry(true)!=\"active\" || Entry(false)!=\"inactive\" { t.Fatal(\"Entry\") } }\n",
		"web/label.ts":        languageIdentityTS,
	} {
		if err := w.write(file, text); err != nil {
			return err
		}
	}
	if err := w.d.Coverage(filepath.Join(w.dir, "gomod"), Coverage{Language: "go", Report: "mode: set\nexample.com/mixed/entry.go:3.40,3.52 1 1\nexample.com/mixed/entry.go:3.52,3.71 1 1\nexample.com/mixed/entry.go:3.74,3.91 1 1\nexample.com/mixed/entry.go:5.23,5.43 1 0\n"}); err != nil {
		return err
	}
	return w.d.Tests(filepath.Join(w.dir, "gomod"), Tests{Language: "go", Command: "go test -count=1 -failfast ./..."})
}
func (l *languageMutation) typeScriptOnlyEvidence(file string) error {
	a, err := l.object()
	if err != nil {
		return err
	}
	count := 0
	for _, p := range checkProblems(a) {
		if p["rule"] == "internal" {
			return fmt.Errorf("mixed selection internal error: %v", p)
		}
		if filepath.ToSlash(fmt.Sprint(p["file"])) != file {
			continue
		}
		message := fmt.Sprint(p["message"])
		if strings.Contains(message, "Go ") || strings.Contains(fmt.Sprint(p["fix"]), "Go ") {
			return fmt.Errorf("TS judged as Go: %v", p)
		}
		if p["rule"] == "mutation.coverage-missing" && strings.Contains(message, "TypeScript line coverage") {
			count++
		}
	}
	if count != 1 || l.m.w.exit != 1 {
		return fmt.Errorf("TS missing-evidence count%d exit%d: %v", count, l.m.w.exit, a)
	}
	return nil
}
func sameGoFindings(a, b map[string]any) error {
	filter := func(o map[string]any) []map[string]any {
		out := []map[string]any{}
		for _, p := range checkProblems(o) {
			if filepath.ToSlash(fmt.Sprint(p["file"])) == "gomod/entry.go" {
				out = append(out, p)
			}
		}
		return out
	}
	got, want := filter(a), filter(b)
	if len(want) != 1 || !reflect.DeepEqual(got, want) {
		return fmt.Errorf("mixed Go judgments %v want%v", got, want)
	}
	return nil
}
func (l *languageMutation) failedCoverage(language, failure string) error {
	w := l.m.w
	l.language = strings.ToLower(language)
	files := map[string]string{}
	switch language {
	case "TypeScript":
		l.file = "app/src/calc.ts"
		files["app/package.json"] = "{\"name\":\"app\"}\n"
		files[l.file] = "export function above(a: number,b: number): boolean {\n return a > b;\n}\n"
	case "Python":
		l.file = "app/src/calc.py"
		files["app/pyproject.toml"] = "[project]\nname = \"app\"\n"
		files[l.file] = "def above(a,b):\n    return a > b\n"
	case "Kotlin":
		l.file = "app/src/main/kotlin/Calc.kt"
		files["app/settings.gradle.kts"] = "rootProject.name = \"app\"\n"
		files["app/build.gradle.kts"] = "plugins { kotlin(\"jvm\") version \"2.4.21\" }\n"
		files[l.file] = "fun above(a: Int,b: Int): Boolean {\n return a > b\n}\n"
	}
	for file, text := range files {
		if err := w.write(file, text); err != nil {
			return err
		}
	}
	if failure != "tool is missing" {
		path := "app/.venv/bin/python"
		status := "0"
		if language == "Kotlin" {
			path = "app/gradlew"
			status = "1"
		}
		if err := w.d.WriteExecutable(filepath.Join(w.dir, filepath.FromSlash(path)), []byte("#!/bin/sh\nexit "+status+"\n")); err != nil {
			return err
		}
		if err := w.d.Coverage(filepath.Join(w.dir, "app"), Coverage{Language: l.language}); err != nil {
			return err
		}
	}
	sites, err := l.m.sitesOf(l.file)
	if err != nil {
		return err
	}
	t := Tests{Language: "go", Command: "go version"}
	for _, s := range sites {
		t.Surviving = append(t.Surviving, Site{File: strings.TrimPrefix(l.file, "app/"), Line: s.line, Original: s.original, Replacement: s.replacement})
	}
	if err := w.d.Tests(filepath.Join(w.dir, "app"), t); err != nil {
		return err
	}
	base := w.prepare
	w.prepare = func(args []string) []string {
		args = base(args)
		if len(args) > 1 && args[0] == "mutation" && args[1] == "run" {
			args = append(args, "--test-command", "go version")
		}
		return args
	}
	return nil
}
func (l *languageMutation) coverageFailed(rule string, exit int) error {
	a, err := l.object()
	if err != nil {
		return err
	}
	if l.m.w.exit != exit {
		return fmt.Errorf("coverage exit%d want%d: %v", l.m.w.exit, exit, a)
	}
	for _, p := range checkProblems(a) {
		if p["rule"] == rule && p["language"] == l.language && filepath.ToSlash(fmt.Sprint(p["dir"])) == "app" {
			return nil
		}
	}
	return fmt.Errorf("missing coverage failure metadata: %v", a)
}
func (l *languageMutation) noJudgment() error {
	a, err := l.object()
	if err != nil {
		return err
	}
	if len(trialsOf(l.m.w.stderr, l.file)) != 0 || l.m.w.d.Exists(filepath.Join(l.m.w.dir, ".metrics/mutate/"+l.file+".json")) {
		return fmt.Errorf("strict coverage failure executed or recorded a mutant")
	}
	for _, f := range objectList(a, "files") {
		for _, key := range []string{"ran", "killed", "survived", "uncovered"} {
			if f[key] != float64(0) {
				return fmt.Errorf("strict failure judged%s: %v", key, f)
			}
		}
		for _, m := range objectList(f, "mutants") {
			if m["outcome"] != nil && m["outcome"] != "" {
				return fmt.Errorf("strict failure recorded outcome: %v", m)
			}
		}
	}
	for _, p := range checkProblems(a) {
		if strings.HasPrefix(fmt.Sprint(p["rule"]), "mutation.") {
			return fmt.Errorf("strict measurement failure made judgment: %v", p)
		}
	}
	if strings.Contains(l.m.w.stderr, "running every mutant") {
		return fmt.Errorf("strict run fell back")
	}
	return nil
}
func (l *languageMutation) failureFallback() error {
	a, log, code, err := l.capture("mutation run --json " + l.file)
	if err != nil {
		return err
	}
	f, err := languageRunFile(a, l.file)
	if err != nil {
		return err
	}
	if code != 1 || f["ran"] != float64(1) || f["survived"] != float64(1) || !strings.Contains(log, "running every mutant") {
		return fmt.Errorf("measurement fallback exit%d: %v\n%s", code, a, log)
	}
	return nil
}
func (l *languageMutation) unjudgedCountProject() error {
	w := l.m.w
	if err := w.d.Counted(); err != nil {
		return err
	}
	l.file = "board.go"
	for file, text := range map[string]string{"go.mod": "module example.com/board\n\ngo 1.22\n", "board.go": "package board\nfunc Low(i int) bool { return i == 3 }\n", "board_test.go": "package board\nimport \"testing\"\nfunc TestLow(t *testing.T){if !Low(3)||Low(4){t.Fatal(\"low\")}}\n", "label.ts": languageIdentityTS} {
		if err := w.write(file, text); err != nil {
			return err
		}
	}
	if err := w.d.Coverage(w.dir, Coverage{Language: "go", Report: "mode: set\nexample.com/board/board.go:2.22,2.39 1 1\n"}); err != nil {
		return err
	}
	// The file's own tests, its package's, beside the whole module's.
	for _, command := range []string{"go test -count=1 -failfast ./.", "go test -count=1 -failfast ./..."} {
		if err := w.d.Tests(w.dir, Tests{Language: "go", Command: command}); err != nil {
			return err
		}
	}
	if err := w.git("init", "-q"); err != nil {
		return err
	}
	return l.s.commit("counted mixed fixture")
}
func (l *languageMutation) countedGo() error {
	a, err := l.object()
	if err != nil {
		return err
	}
	for _, stage := range objectList(a, "stages") {
		if stage["name"] == "coverage" && stage["state"] != "complete" {
			return fmt.Errorf("counted coverage incomplete: %v", stage)
		}
	}
	text, _ := jsonBytes(a)
	if !strings.Contains(text, "killed") || !strings.Contains(text, "board.go") {
		return fmt.Errorf("no actual counted Go judgment: %v", a)
	}
	draws := objectList(a, "selected")
	if len(draws) != 1 {
		return fmt.Errorf("counted draws not one: %v", a)
	}
	return nil
}
func jsonBytes(a any) (string, error) { b, err := json.Marshal(a); return string(b), err }
func (l *languageMutation) noTypeScriptPreparation() error {
	a, err := l.object()
	if err != nil {
		return err
	}
	for _, stage := range objectList(a, "stages") {
		if stage["state"] == "failed" {
			return fmt.Errorf("counted stage failed: %v", stage)
		}
	}
	for _, line := range strings.Split(l.m.w.stderr, "\n") {
		if strings.Contains(line, "coverage ") && strings.Contains(line, "node") {
			return fmt.Errorf("unjudged TS measured: %s", line)
		}
	}
	return l.m.w.exitsWith(0)
}
func (l *languageMutation) eligibleTypeScriptFails() error {
	if err := l.m.w.write("label.ts", "export function low(i: number): boolean { return i === 3; }\n"); err != nil {
		return err
	}
	if err := l.s.commit("eligible TS site"); err != nil {
		return err
	}
	a, _, code, err := l.capture("mutation run --count 1 --seed 4813e48 --fail-uncovered --json board.go label.ts")
	if err != nil {
		return err
	}
	if code != 1 {
		return fmt.Errorf("eligible TS preparation exit%d: %v", code, a)
	}
	for _, p := range checkProblems(a) {
		if p["rule"] == "count.preparation-failed" && p["stage"] == "coverage" {
			return nil
		}
	}
	return fmt.Errorf("missing coverage-stage count failure: %v", a)
}
func (l *languageMutation) nodeFallbackProject() error {
	w := l.m.w
	l.language = "typescript"
	l.file = "src/board.ts"
	for file, text := range map[string]string{
		"package.json":       `{"name":"board","private":true,"type":"module","scripts":{"test":"node --test"}}` + "\n",
		l.file:               "export function low(i: number): boolean {\n return i === 3;\n}\n",
		"test/board.test.ts": "import {test} from 'node:test';\nimport assert from 'node:assert/strict';\nimport {low} from '../src/board.ts';\ntest('low',()=>{assert.equal(low(3),true);assert.equal(low(4),false);});\n",
		"test/other.test.ts": "import {test} from 'node:test';\ntest('other',()=>{});\n",
	} {
		if err := w.write(file, text); err != nil {
			return err
		}
	}
	return w.d.Tests(w.dir, Tests{Language: "typescript", Command: "npm run test --silent"})
}
func (l *languageMutation) recordNodeFallback() error {
	a, log, code, err := l.capture("mutation run --json --no-coverage " + l.file)
	if err != nil {
		return err
	}
	f, err := languageRunFile(a, l.file)
	if err != nil {
		return err
	}
	if code != 0 || f["baseline"] != "passed" || f["ran"] != float64(1) || f["killed"] != float64(1) || !strings.Contains(log, "run test") {
		return fmt.Errorf("node test script outcome exit%d %v\n%s", code, a, log)
	}
	snap, err := l.s.snapshot(l.file)
	if err != nil {
		return err
	}
	for _, u := range objectList(snap, "units") {
		for _, m := range objectList(u, "mutants") {
			ev, ok := m["suite_evidence"].(map[string]any)
			if !ok {
				return fmt.Errorf("node script lacks broad evidence: %v", m)
			}
			tests, _ := ev["tests"].(map[string]any)
			if tests["test/other.test.ts"] == nil {
				return fmt.Errorf("non-importing test missing from script evidence: %v", ev)
			}
		}
	}
	return nil
}
