package features

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

func (l *languageMutation) toolSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a Vitest project with a coverage script that runs the whole suite$`, l.tsCoverageProject)
	sc.Step(`^src/a\.ts has a line that only an unrelated test, which does not import it, executes$`, func() error {
		sites, err := l.m.sitesOf(l.file)
		if err != nil {
			return err
		}
		if len(sites) != 2 {
			return fmt.Errorf("TS scope fixture sites: %v", sites)
		}
		return nil
	})
	sc.Step(`^coverage runs vitest related for src/a\.ts into the run's own directory, not the coverage script$`, l.relatedCoverage)
	sc.Step(`^the mutant on that line is uncovered and none of its trials runs$`, l.relatedUncovered)
	sc.Step(`^with --all-tests the coverage script measures the whole suite as before$`, l.allTestsCoverage)
	sc.Step(`^crap still measures with the coverage script$`, l.crapScript)
	sc.Step(`^a committed (TypeScript|Python|Kotlin|Dart) project with eligible mutation sites and its dependencies installed in (node_modules at the project root|\.venv at the project root|the Gradle cache, run with --offline|the pub cache, run with --no-pub|the pub cache and the SDK its \.fvmrc pins, run with --no-pub)$`, l.installedProject)
	sc.Step(`^a count-one run judges it$`, func() error { return l.run("mutation run --count 1 --seed 4813e48 --json " + l.file) })
	sc.Step(`^preparation measures coverage and runs the clean baseline with the project's installed tools$`, l.installedPreparation)
	sc.Step(`^exactly one selected mutant is judged with a real outcome$`, l.oneRealCounted)
	sc.Step(`^no command downloads or installs anything$`, l.noDownload)
	sc.Step(`^the live working tree is left unchanged$`, l.pythonTreeKept)
}

const languageTSKept = "export function kept(i: number): boolean {\n  return i === 3;\n}\n\nexport function far(i: number): boolean {\n  return i === 9;\n}\n"

func (l *languageMutation) tsCoverageProject() error {
	w := l.m.w
	l.file = "src/a.ts"
	l.language = "typescript"
	for file, text := range map[string]string{
		"package.json":      `{"name":"scope","private":true,"type":"module","scripts":{"coverage":"vitest run --coverage.enabled --coverage.reporter=lcov"},"devDependencies":{"vitest":"5.0.2","@vitest/coverage-v8":"5.0.2"}}` + "\n",
		l.file:              languageTSKept,
		"src/a.test.ts":     "import {expect,test} from 'vitest';\nimport {kept} from './a';\ntest('kept',()=>{expect(kept(3)).toBe(true);expect(kept(4)).toBe(false);});\n",
		"src/other.test.ts": "import {expect,test} from 'vitest';\ntest('far',async()=>{const name=['.','a'].join('/');const {far}=await import(/* @vite-ignore */ name);expect(far(9)).toBe(true);expect(far(8)).toBe(false);});\n",
	} {
		if err := w.write(file, text); err != nil {
			return err
		}
	}
	if err := w.d.Coverage(w.dir, Coverage{Language: "typescript", Report: "SF:src/a.ts\nDA:1,1\nDA:2,1\nDA:5,1\nDA:6,0\nend_of_record\n"}); err != nil {
		return err
	}
	return w.d.Tests(w.dir, Tests{Language: "typescript", Command: "node_modules/.bin/vitest related --run src/a.ts"})
}
func (l *languageMutation) relatedCoverage() error {
	lines := languageCoverageLines(l.m.w.stderr)
	if len(lines) != 1 || !strings.Contains(lines[0], "vitest related --run src/a.ts ") || !strings.Contains(lines[0], "--coverage.reportsDirectory="+filepath.Join(l.m.w.dir, ".metrics", "coverage", "run-")) || strings.Contains(l.m.w.stderr, "run coverage") {
		return fmt.Errorf("related coverage command: %v", lines)
	}
	return nil
}
func languageCoverageLines(log string) []string {
	var out []string
	for _, line := range strings.Split(log, "\n") {
		if strings.HasPrefix(line, "itos-cc: coverage ") && strings.Contains(line, "$ ") {
			out = append(out, line)
		}
	}
	return out
}
func (l *languageMutation) relatedUncovered() error {
	a, err := l.object()
	if err != nil {
		return err
	}
	f, err := languageRunFile(a, l.file)
	if err != nil {
		return err
	}
	if f["ran"] != float64(1) || f["uncovered"] != float64(1) {
		return fmt.Errorf("related outcome counts: %v", f)
	}
	seen := map[string]int{}
	for _, m := range objectList(f, "mutants") {
		name := fmt.Sprint(m["function"])
		want := "killed"
		key := "kept"
		if strings.HasSuffix(name, "#far") {
			want = "uncovered"
			key = "far"
		}
		if m["outcome"] != want {
			return fmt.Errorf("related mutant: %v", m)
		}
		seen[key]++
	}
	if seen["kept"] != 1 || seen["far"] != 1 {
		return fmt.Errorf("missing related mutant population: %v", f)
	}
	for _, trial := range trialsOf(l.m.w.stderr, l.file) {
		if trial.line == 6 {
			return fmt.Errorf("uncovered far trial executed: %v", trial)
		}
	}
	return nil
}
func (l *languageMutation) allTestsCoverage() error {
	w := l.m.w
	if err := w.d.Remove(filepath.Join(w.dir, ".metrics", "mutate")); err != nil {
		return err
	}
	if err := w.d.Remove(filepath.Join(w.dir, "node_modules")); err != nil {
		return err
	}
	if err := w.d.Coverage(w.dir, Coverage{Language: "typescript", Report: "SF:src/a.ts\nDA:1,1\nDA:2,1\nDA:5,1\nDA:6,1\nend_of_record\n"}); err != nil {
		return err
	}
	if err := w.d.Tests(w.dir, Tests{Language: "typescript", Command: "node_modules/.bin/vitest run"}); err != nil {
		return err
	}
	a, log, code, err := l.capture("mutation run --all-tests --json " + l.file)
	if err != nil {
		return err
	}
	lines := languageCoverageLines(log)
	if code != 0 || len(lines) != 1 || !strings.HasSuffix(lines[0], "$ npm run coverage") {
		return fmt.Errorf("all-tests coverage exit%d %v\n%s", code, a, log)
	}
	f, err := languageRunFile(a, l.file)
	if err != nil {
		return err
	}
	if f["killed"] != float64(2) || f["uncovered"] != float64(0) {
		return fmt.Errorf("whole-suite did not reach and kill far: %v", f)
	}
	return nil
}
func (l *languageMutation) crapScript() error {
	a, log, code, err := l.capture("crap --json " + l.file)
	if err != nil {
		return err
	}
	lines := languageCoverageLines(log)
	if code != 0 || len(lines) != 1 || !strings.HasSuffix(lines[0], "$ npm run coverage") {
		return fmt.Errorf("crap coverage script exit%d %v\n%s", code, a, log)
	}
	return nil
}
func (l *languageMutation) installedProject(ctx context.Context, language, installed string) error {
	w := l.m.w
	if err := w.d.Counted(); err != nil {
		return err
	}
	l.language = strings.ToLower(language)
	files := map[string]string{}
	coverage := Coverage{Language: l.language}
	command := ""
	setup := ""
	switch language {
	case "TypeScript":
		l.file = "src/board.ts"
		files = map[string]string{
			".gitignore": "node_modules\n.metrics\n", "package.json": `{"name":"board","private":true,"type":"module","devDependencies":{"vitest":"5.0.2","@vitest/coverage-v8":"5.0.2"}}` + "\n",
			l.file:              "export function low(i: number): boolean {\n return i === 3;\n}\n",
			"src/board.test.ts": "import {expect,test} from 'vitest';\nimport {low} from './board';\ntest('low',()=>{expect(low(3)).toBe(true);expect(low(4)).toBe(false);});\n",
		}
		coverage.Report = "SF:src/board.ts\nDA:1,1\nDA:2,1\nend_of_record\n"
		command = "node_modules/.bin/vitest related --run src/board.ts"
		setup = "crap --json " + l.file
	case "Python":
		l.file = "board.py"
		files = map[string]string{
			".gitignore":     ".venv\n.metrics\n__pycache__\n.coverage\noracle.lcov\n",
			"pyproject.toml": "[project]\nname = \"board\"\nversion = \"0.0.0\"\n",
			l.file:           languagePythonSource, "test_board.py": "from boardhelp import THREE\nfrom board import low\ndef test_low():\n    assert low(THREE)\n    assert not low(4)\n",
			"setup_dependency.py": "import pathlib,sys,venv\nvenv.EnvBuilder(system_site_packages=True,with_pip=False).create('.venv')\nsite=pathlib.Path('.venv')/'lib'/('python'+str(sys.version_info.major)+'.'+str(sys.version_info.minor))/'site-packages'\nsite.mkdir(parents=True,exist_ok=True)\n(site/'boardhelp.py').write_text('THREE = 3\\n')\n",
		}
		coverage.Report = "SF:board.py\nDA:1,1\nDA:2,1\nend_of_record\n"
		command = filepath.Join(w.dir, ".venv/bin/python") + " -m pytest -q -x -p no:cacheprovider test_board.py"
		setup = `crap --json --coverage-command 'python3 setup_dependency.py && .venv/bin/python -m coverage run -m pytest -q -p no:cacheprovider test_board.py && .venv/bin/python -m coverage lcov -o oracle.lcov' --coverage-report oracle.lcov board.py`
	case "Kotlin":
		l.file = "src/main/kotlin/board/Board.kt"
		for file, text := range languageKotlinBuilds["gradle"] {
			files[file] = strings.ReplaceAll(text, "own", "board")
		}
		files[".gitignore"] = ".gradle\nbuild\n.metrics\n"
		files["build.gradle.kts"] = strings.Replace(files["build.gradle.kts"], "useJUnitPlatform()\n", "useJUnitPlatform()\n    outputs.upToDateWhen { false }\n", 1)
		files[l.file] = "package board\nfun low(i: Int): Boolean {\n return i == 3\n}\n"
		files["src/test/kotlin/board/BoardTest.kt"] = "package board\nimport kotlin.test.*\nclass BoardTest { @Test fun judges(){ assertTrue(low(3));assertFalse(low(4)); } }\n"
		coverage.Program = "gradle"
		coverage.Report = `<report><package name="board"><sourcefile name="Board.kt"><line nr="3" mi="0" ci="1"/></sourcefile></package></report>`
		command = "gradle --offline -p . test --fail-fast --tests board.BoardTest"
		setup = "crap --json " + l.file
	case "Dart":
		// Resolved as its user would before it is committed, pubspec.lock
		// with it; .dart_tool and an .fvmrc's .fvm are ignored, so the
		// frozen export holds neither.
		l.file = "lib/board.dart"
		files = map[string]string{
			".gitignore":   ".dart_tool\n.fvm\n.metrics\nbuild\ncoverage\n",
			"pubspec.yaml": coverageDartFlutterPubspec("board", false),
			l.file:         mutateDartBoard,
			"test/board_test.dart": "import 'package:flutter_test/flutter_test.dart';\nimport 'package:board/board.dart';\n\n" +
				"void main() {\n  test('low', () {\n    expect(low(3), isTrue);\n    expect(low(4), isFalse);\n  });\n}\n",
		}
		if strings.Contains(installed, ".fvmrc") {
			files[".fvmrc"] = "{\n  \"flutter\": \"stable\"\n}\n"
		}
		setup = "crap --json " + l.file
	}
	if err := l.instrumentInstalled(files); err != nil {
		return err
	}
	l.frozenSource = files[l.file]
	for file, text := range files {
		if err := w.write(file, text); err != nil {
			return err
		}
	}
	if err := w.d.Coverage(w.dir, coverage); err != nil {
		return err
	}
	// Dart's tools are flutter's, which its Coverage checked and its
	// command chooses.
	if command != "" {
		if err := w.d.Tests(w.dir, Tests{Language: l.language, Command: command}); err != nil {
			return err
		}
	}
	if err := w.git("init", "-q"); err != nil {
		return err
	}
	if err := l.s.commit("frozen language source and tests"); err != nil {
		return err
	}
	w.variables = append(w.variables, "PYTEST_DISABLE_PLUGIN_AUTOLOAD=1")
	if setup != "" {
		a, log, code, err := l.capture(setup)
		if err != nil {
			return err
		}
		if code != 0 || a["ok"] != true {
			return fmt.Errorf("installed dependency setup %s exit%d: %v\n%s", installed, code, a, log)
		}
	}
	if err := l.rememberInstalledTarget(); err != nil {
		return err
	}
	if err := w.d.Write(l.trace, nil); err != nil {
		return err
	}
	comment := "\n// live source outside the frozen commit\n"
	if l.language == "python" {
		comment = "\n# live source outside the frozen commit\n"
	}
	if err := w.write(l.file, l.frozenSource+comment); err != nil {
		return err
	}
	var err error
	l.before, err = l.files(ctx)
	return err
}
func (l *languageMutation) installedPreparation() error {
	a, err := l.object()
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, s := range objectList(a, "stages") {
		seen[fmt.Sprint(s["name"])] = s["state"] == "complete"
	}
	if !seen["coverage"] || !seen["baseline"] {
		return fmt.Errorf("installed preparation stages: %v", a)
	}
	if err := l.installedProvenance(); err != nil {
		return err
	}
	w := l.m.w
	pinned := w.d.Exists(filepath.Join(w.dir, ".fvmrc"))
	coverage, baseline := false, false
	for _, line := range strings.Split(l.m.w.stderr, "\n") {
		if !strings.Contains(line, "$ ") {
			continue
		}
		isCoverage := strings.HasPrefix(line, "itos-cc: coverage ")
		isBaseline := strings.HasPrefix(line, "itos-cc: baseline ")
		if !isCoverage && !isBaseline {
			continue
		}
		coverage = coverage || isCoverage
		baseline = baseline || isBaseline
		switch l.language {
		case "typescript":
			if !installedToolPath(line, l.m.w.dir, filepath.Join("node_modules", ".bin", "vitest")) {
				return fmt.Errorf("frozen command did not use installed live node dependency: %s", line)
			}
		case "python":
			if !installedToolPath(line, l.m.w.dir, filepath.Join(".venv", "bin", "python")) {
				return fmt.Errorf("frozen command did not use installed live venv: %s", line)
			}
		case "kotlin":
			if !strings.Contains(line, "--offline") || !strings.Contains(line, "--tests board.BoardTest") {
				return fmt.Errorf("the Gradle frozen command not offline/narrowed: %s", line)
			}
		case "dart":
			if !strings.Contains(line, " test --no-pub") {
				return fmt.Errorf("the frozen flutter test does not run with --no-pub: %s", line)
			}
			// The SDK the .fvmrc pins is the live one, linked into the
			// export: its flutter there, not the PATH's nor the live link.
			live := filepath.Join(w.dir, ".fvm", "flutter_sdk", "bin", "flutter")
			if pinned && (!installedToolPath(line, w.dir, filepath.Join(".fvm", "flutter_sdk", "bin", "flutter")) || strings.Contains(line, "$ "+live+" ")) {
				return fmt.Errorf("the frozen command does not run the SDK the .fvmrc pins, linked into the export: %s", line)
			}
		}
	}
	if !coverage || !baseline {
		return fmt.Errorf("missing actual installed-tool preparation command: %s", l.m.w.stderr)
	}
	return nil
}
func (l *languageMutation) oneRealCounted() error {
	a, err := l.object()
	if err != nil {
		return err
	}
	selected := objectList(a, "selected")
	if len(selected) != 1 || selected[0]["state"] != "judged" || selected[0]["outcome"] != "killed" || selected[0]["file"] != l.file {
		return fmt.Errorf("one actual installed-tool outcome: %v", a)
	}
	sampling, ok := a["sampling"].(map[string]any)
	if !ok || sampling["executed"] != float64(1) || sampling["selected"] != float64(1) {
		return fmt.Errorf("counted execution count: %v", a)
	}
	return nil
}
func (l *languageMutation) noDownload() error {
	for _, line := range strings.Split(l.m.w.stderr, "\n") {
		low := strings.ToLower(line)
		if strings.Contains(low, "downloading ") || strings.Contains(low, "npm install") || strings.Contains(low, "npm ci") || strings.Contains(low, "pip install") {
			return fmt.Errorf("counted command installed dependency: %s", line)
		}
	}
	if l.language == "kotlin" && !strings.Contains(l.m.w.stderr, "--offline") {
		return fmt.Errorf("no offline Gradle command")
	}
	if l.language == "dart" {
		return countedDartOffline(l.m.w.stderr)
	}
	return nil
}

// countedDartOffline is whether every flutter command log announces that
// resolves runs --offline and every flutter test --no-pub, at least one.
func countedDartOffline(log string) error {
	tests := 0
	for _, line := range strings.Split(log, "\n") {
		_, command, ok := strings.Cut(line, "$ ")
		if !ok {
			continue
		}
		words, err := commandWords(command)
		if err != nil || len(words) == 0 || filepath.Base(words[0]) != "flutter" {
			continue
		}
		if slices.Contains(words, "pub") && !slices.Contains(words, "--offline") {
			return fmt.Errorf("flutter resolved online: %s", line)
		}
		if slices.Contains(words, "test") {
			tests++
			if !slices.Contains(words, "--no-pub") {
				return fmt.Errorf("flutter test ran without --no-pub: %s", line)
			}
		}
	}
	if tests == 0 {
		return fmt.Errorf("no flutter test ran:\n%s", log)
	}
	return nil
}

func installedToolPath(line, root, suffix string) bool {
	_, command, ok := strings.Cut(line, "$ ")
	if !ok {
		return false
	}
	words, err := commandWords(command)
	return err == nil && len(words) > 0 && strings.HasPrefix(words[0], root+string(filepath.Separator)) && strings.HasSuffix(words[0], string(filepath.Separator)+suffix)
}
