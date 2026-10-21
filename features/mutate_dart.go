package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

// The Flutter packages mutation run judges with flutter test (slice-35),
// and what is checked of their runs: the command a file's mutants run, the
// test files that judge them, and the copies workers run them in.

// mutateDartReaching are the test files of MUT-07's Dart project that reach
// its source, lib/board.dart; test/other_test.dart reaches nothing of it.
var mutateDartReaching = []string{"test/board_test.dart"}

// mutateDartBoard is lib/board.dart of MUT-07's Dart project.
const mutateDartBoard = "bool low(int i) {\n  return i == 3;\n}\n"

// mutateDartCommandProject writes MUT-07's Flutter package, board, whose
// test/board_test.dart reaches lib/board.dart and test/other_test.dart does
// not, with a .fvmrc where fvm says so, and readies it as its user would:
// fvm use's .fvm/flutter_sdk linked, its dependencies resolved.
func (m *mutateState) mutateDartCommandProject(fvm bool) error {
	m.file = "lib/board.dart"
	files := map[string]string{
		"pubspec.yaml":         coverageDartFlutterPubspec("board", false),
		m.file:                 mutateDartBoard,
		"test/board_test.dart": "import 'package:flutter_test/flutter_test.dart';\nimport 'package:board/board.dart';\n\nvoid main() {\n  test('low', () {\n    expect(low(3), isTrue);\n    expect(low(4), isFalse);\n  });\n}\n",
		"test/other_test.dart": "import 'package:flutter_test/flutter_test.dart';\n\nvoid main() {\n  test('other', () {\n    expect(1 + 1, 2);\n  });\n}\n",
	}
	if fvm {
		files[".fvmrc"] = "{\n  \"flutter\": \"stable\"\n}\n"
	}
	if err := m.writeAll(files); err != nil {
		return err
	}
	m.w.imports(map[string][]string{"test/board_test.dart": {m.file}})
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "dart"}); err != nil {
		return err
	}
	if fvm && !m.w.d.Exists(filepath.Join(m.w.dir, ".fvm", "flutter_sdk", "bin", "flutter")) {
		return errors.New("no .fvm/flutter_sdk links the package's SDK")
	}
	return nil
}

// mutateDartOnlyReaching is whether the baseline the last run announced
// names no Dart file but the test files that reach the source.
func (m *mutateState) mutateDartOnlyReaching() error {
	for _, run := range baselines(m.w.stderr) {
		for _, word := range run.words[1:] {
			if strings.HasSuffix(word, ".dart") && !slices.Contains(mutateDartReaching, word) {
				return fmt.Errorf("the test command %q runs %s, which does not reach %s\n%s", run.command, word, m.file, m.w.report())
			}
		}
	}
	return nil
}

func (l *languageMutation) dartSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a Flutter package where test/a_test\.dart reaches lib/a\.dart, test/b_test\.dart reaches only lib/b\.dart and fails, and integration_test/app_test\.dart imports lib/a\.dart$`, l.mutateDartOwn)
	sc.Step(`^I run "itos-cc ([^"]*)" for lib/a\.dart$`, func(args string) error { return l.run(args + " " + l.file) })
	sc.Step(`^lib/a\.dart's baseline passes and its mutant is judged by test/a_test\.dart alone, with coverage measured from test/a_test\.dart alone$`, l.mutateDartOwnJudgment)
	sc.Step(`^test/b_test\.dart and integration_test/app_test\.dart never run$`, func() error {
		_, err := l.mutateDartOnlyTest("a")
		return err
	})
	sc.Step(`^a Dart file no test reaches has its mutants reported uncovered, and no test command runs for it$`, l.mutateDartUnreached)
	sc.Step(`^a resolved (Flutter app|member of a pub workspace with a sibling member it depends on) with android and ios folders and a generated lib/user\.g\.dart its tests import$`, l.mutateDartWorkersProject)
	sc.Step(`^I run "itos-cc mutation run --workers 2" for one of its libraries$`, l.mutateDartTracedRun)
	sc.Step(`^each worker's copy holds no android or ios folder, and holds lib/user\.g\.dart$`, l.mutateDartCopies)
	sc.Step(`^each worker runs flutter pub get --offline once, before its first mutant, and nothing reaches the network$`, l.mutateDartResolvedOffline)
	sc.Step(`^every mutant's flutter test runs with --no-pub in its worker's copy$`, l.mutateDartNoPub)
	sc.Step(`^the mutants are judged as a single worker judges them$`, l.mutateDartAsOneWorker)
}

// mutateDartRecorder is Dart that defines record(name), which appends to
// the trace file at trace what the test name saw where it ran: its folder,
// the package's real folder root, whether the platform folders and
// lib/user.g.dart are beside it, and whether a pub workspace's root
// pubspec.yaml and pubspec.lock and its sibling member core are above it.
func mutateDartRecorder(trace, root string) string {
	q := func(s string) string { b, _ := json.Marshal(s); return string(b) }
	return "import 'dart:convert';\nimport 'dart:io';\n\nvoid record(String name) {\n" +
		"  File(" + q(trace) + ").writeAsStringSync(jsonEncode({\n" +
		"    'test': name,\n" +
		"    'cwd': Directory.current.resolveSymbolicLinksSync(),\n" +
		"    'original_root': Directory(" + q(root) + ").resolveSymbolicLinksSync(),\n" +
		"    'android': Directory('android').existsSync(),\n" +
		"    'ios': Directory('ios').existsSync(),\n" +
		"    'generated': File('lib/user.g.dart').existsSync(),\n" +
		"    'workspace': File('../pubspec.yaml').existsSync() && File('../pubspec.lock').existsSync() && File('../core/pubspec.yaml').existsSync(),\n" +
		"  }) + '\\n', mode: FileMode.append, flush: true);\n}\n"
}

// mutateDartTest is a flutter_test test file named name that records its
// run and then runs body, importing imports and the recorder.
func mutateDartTest(recorder, name string, imports []string, body string) string {
	text := "import 'package:flutter_test/flutter_test.dart';\n"
	for _, i := range imports {
		text += "import '" + i + "';\n"
	}
	return text + "\n" + recorder + "\nvoid main() {\n  test('" + name + "', () {\n    record('" + name + "');\n" + body + "  });\n}\n"
}

// mutateDartOwn writes MUT-264's Flutter package, own: lib/a.dart's low,
// which test/a_test.dart calls, and spare, which only
// integration_test/app_test.dart calls; test/b_test.dart, which reaches
// only lib/b.dart and fails; and lib/u.dart, which no test reaches.
func (l *languageMutation) mutateDartOwn() error {
	w := l.m.w
	l.file, l.language = "lib/a.dart", "dart"
	if err := l.ownTrace(); err != nil {
		return err
	}
	rec := mutateDartRecorder(l.trace, w.dir)
	if err := l.m.writeAll(map[string]string{
		"pubspec.yaml":                   coverageDartFlutterPubspec("own", false),
		l.file:                           "bool low(int i) {\n  return i == 3;\n}\n\nbool spare(int i) {\n  return i == 7;\n}\n",
		"lib/b.dart":                     "bool high(int i) {\n  return i == 9;\n}\n",
		"lib/u.dart":                     "bool mid(int i) {\n  return i == 5;\n}\n",
		"test/a_test.dart":               mutateDartTest(rec, "a", []string{"package:own/a.dart"}, "    expect(low(3), isTrue);\n    expect(low(4), isFalse);\n"),
		"test/b_test.dart":               mutateDartTest(rec, "b", []string{"package:own/b.dart"}, "    expect(high(9), isTrue);\n    fail('test/b_test.dart fails');\n"),
		"integration_test/app_test.dart": mutateDartTest(rec, "app", []string{"package:own/a.dart"}, "    expect(spare(7), isTrue);\n    expect(spare(8), isFalse);\n"),
	}); err != nil {
		return err
	}
	w.imports(map[string][]string{"test/a_test.dart": {l.file}, "test/b_test.dart": {"lib/b.dart"}, "integration_test/app_test.dart": {l.file}})
	if err := w.d.Coverage(w.dir, Coverage{Language: "dart"}); err != nil {
		return err
	}
	return w.d.Write(l.trace, nil)
}

// mutateDartRecords are the runs the trace recorded.
func (l *languageMutation) mutateDartRecords() ([]map[string]any, error) {
	b, err := l.m.w.d.Read(l.trace)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		var a map[string]any
		if err := json.Unmarshal([]byte(line), &a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// mutateDartOnlyTest is whether every run the trace recorded was the test
// name's, at least once in the real tree, as coverage measures it, and
// twice in workers' copies, a baseline and a mutant: those runs.
func (l *languageMutation) mutateDartOnlyTest(name string) ([]map[string]any, error) {
	records, err := l.mutateDartRecords()
	if err != nil {
		return nil, err
	}
	tree, copies := 0, 0
	for _, r := range records {
		if r["test"] != name {
			return nil, fmt.Errorf("the test %v ran, not %s alone: %v\n%s", r["test"], name, records, l.m.w.report())
		}
		if r["cwd"] == r["original_root"] {
			tree++
		} else {
			copies++
		}
	}
	if tree < 1 || copies < 2 {
		return nil, fmt.Errorf("the test %s ran %d times in the tree and %d in copies, not for coverage, a baseline and a mutant: %v\n%s", name, tree, copies, records, l.m.w.report())
	}
	return records, nil
}

// mutateDartOwnJudgment is whether lib/a.dart's baseline passed, low's
// mutants were killed and spare's uncovered, coverage ran
// test/a_test.dart alone, and only test/a_test.dart ran.
func (l *languageMutation) mutateDartOwnJudgment() error {
	a, err := l.object()
	if err != nil {
		return err
	}
	f, err := languageRunFile(a, l.file)
	if err != nil {
		return err
	}
	if f["baseline"] != "passed" {
		return fmt.Errorf("lib/a.dart's baseline: %v\n%s", f, l.m.w.report())
	}
	seen := map[string]int{}
	for _, m := range objectList(f, "mutants") {
		name := fmt.Sprint(m["function"])
		switch {
		case strings.HasSuffix(name, "#low") && m["outcome"] == "killed":
			seen["low"]++
		case strings.HasSuffix(name, "#spare") && m["outcome"] == "uncovered":
			seen["spare"]++
		default:
			return fmt.Errorf("lib/a.dart's mutant %v\n%s", m, l.m.w.report())
		}
	}
	if seen["low"] == 0 || seen["spare"] == 0 {
		return fmt.Errorf("lib/a.dart's mutants are not low's killed and spare's uncovered: %v\n%s", f, l.m.w.report())
	}
	lines := languageCoverageLines(l.m.w.stderr)
	if len(lines) != 1 || !strings.Contains(lines[0], " --coverage") || !strings.HasSuffix(lines[0], " test/a_test.dart") {
		return fmt.Errorf("coverage of lib/a.dart is not measured from test/a_test.dart alone: %v\n%s", lines, l.m.w.report())
	}
	_, err = l.mutateDartOnlyTest("a")
	return err
}

// mutateDartUnreached runs mutation run over lib/u.dart, which no test
// reaches: its mutants are uncovered, and no command and no test runs.
func (l *languageMutation) mutateDartUnreached() error {
	if err := l.m.w.d.Write(l.trace, nil); err != nil {
		return err
	}
	a, log, _, err := l.capture("mutation run --json lib/u.dart")
	if err != nil {
		return err
	}
	f, err := languageRunFile(a, "lib/u.dart")
	if err != nil {
		return err
	}
	records, err := l.mutateDartRecords()
	if err != nil {
		return err
	}
	if f["baseline"] != "not-run" || f["ran"] != float64(0) || f["uncovered"] == float64(0) || len(records) != 0 || strings.Contains(log, "$ ") {
		return fmt.Errorf("lib/u.dart, which no test reaches, ran tests: %v %v\n%s", f, records, log)
	}
	return nil
}

// mutateDartPackage is the folder, from the scratch folder, of MUT-265's
// package: the app, or the workspace's member app.
func (l *languageMutation) mutateDartPackage() string { return path.Dir(path.Dir(l.file)) }

// mutateDartWorkersProject writes MUT-265's package, kind: a Flutter app
// board at the scratch folder, or the member app of a pub workspace there
// whose lib/board.dart calls its sibling member core. Either has android
// and ios folders, and a generated lib/user.g.dart its test imports; the
// test records what it saw where it ran. It is resolved as its user would.
func (l *languageMutation) mutateDartWorkersProject(kind string) error {
	w := l.m.w
	l.language = "dart"
	if err := l.ownTrace(); err != nil {
		return err
	}
	pkg, name, pubspec := ".", "board", coverageDartFlutterPubspec("board", false)
	files := map[string]string{}
	high := "bool high(int i) {\n  return i > 7 && i < 100;\n}\n"
	board := "bool low(int i) {\n  return i == 3;\n}\n\n" + high
	if kind != "Flutter app" {
		pkg, name = "app", "app"
		pubspec = "name: app\npublish_to: none\nenvironment:\n  sdk: ^3.6.0\nresolution: workspace\n" +
			"dependencies:\n  flutter:\n    sdk: flutter\n  core:\n    path: ../core\ndev_dependencies:\n  flutter_test:\n    sdk: flutter\n"
		files["pubspec.yaml"] = coverageDartPubspec("workspace") + "workspace:\n  - app\n  - core\n"
		files["core/pubspec.yaml"] = coverageDartPubspec("core") + "resolution: workspace\n"
		files["core/lib/core.dart"] = "int three() {\n  return 3;\n}\n"
		board = "import 'package:core/core.dart';\n\nbool low(int i) {\n  return i == three();\n}\n\n" + high
	}
	in := func(rel string) string { return path.Join(pkg, rel) }
	l.file = in("lib/board.dart")
	rec := mutateDartRecorder(l.trace, filepath.Join(w.dir, filepath.FromSlash(pkg)))
	files[in("pubspec.yaml")] = pubspec
	files[l.file] = board
	files[in("lib/user.g.dart")] = "// GENERATED CODE - DO NOT MODIFY BY HAND\n\nint userId(int i) => i + 1;\n"
	files[in("android/build.gradle.kts")] = "plugins {}\n"
	files[in("android/app/src/main/kotlin/MainActivity.kt")] = "package board\n\nclass MainActivity\n"
	files[in("ios/Runner/AppDelegate.swift")] = "import UIKit\n"
	files[in("test/board_test.dart")] = mutateDartTest(rec, "board", []string{"package:" + name + "/board.dart", "package:" + name + "/user.g.dart"},
		"    expect(userId(1), 2);\n    expect(low(3), isTrue);\n    expect(low(4), isFalse);\n    expect(high(8), isTrue);\n    expect(high(7), isFalse);\n    expect(high(100), isFalse);\n")
	if err := l.m.writeAll(files); err != nil {
		return err
	}
	w.imports(map[string][]string{in("test/board_test.dart"): {l.file, in("lib/user.g.dart")}})
	if err := w.d.Coverage(w.dir, Coverage{Language: "dart"}); err != nil {
		return err
	}
	return w.d.Write(l.trace, nil)
}

// mutateDartTools are the folder of MUT-265's run's flutter, which logs
// each command it starts and its folder before running the real flutter,
// and the files that log and strace's trace of the run's connections go
// to.
type mutateDartTools struct{ bin, log, connections string }

func (l *languageMutation) mutateDartTools() mutateDartTools {
	dir := filepath.Dir(l.trace)
	return mutateDartTools{bin: filepath.Join(dir, "bin"), log: filepath.Join(dir, "flutter.log"), connections: filepath.Join(dir, "connections")}
}

// mutateDartTracedRun runs mutation run --workers 2 over the package's
// lib/board.dart under strace, which records every connection the run and
// the commands it starts make, with a flutter first on the PATH that logs
// each of its commands. strace runs on Linux alone, where the dart job
// runs: elsewhere, or without strace, the scenario skips, and fails where
// that job installed Flutter.
func (l *languageMutation) mutateDartTracedRun() error {
	w := l.m.w
	b, ok := w.d.(binary)
	if !ok {
		return errors.New("only the binary driver runs the real flutter")
	}
	strace, err := exec.LookPath("strace")
	if runtime.GOOS != "linux" || err != nil {
		if os.Getenv(flutterToolsEnv) != "" {
			return fmt.Errorf("the run's connections are traced with strace on linux: %v", err)
		}
		return fmt.Errorf("%w: the run's connections are traced with strace on linux alone", godog.ErrSkip)
	}
	flutter, err := exec.LookPath("flutter")
	if err != nil {
		return missing("flutter")
	}
	t := l.mutateDartTools()
	q := func(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
	wrapper := "#!/bin/sh\nprintf '%s\\t%s\\n' \"$(pwd -P)\" \"$*\" >> " + q(t.log) + "\nexec " + q(flutter) + " \"$@\"\n"
	if err := w.d.WriteExecutable(filepath.Join(t.bin, "flutter"), []byte(wrapper)); err != nil {
		return err
	}
	if err := w.d.Write(t.log, nil); err != nil {
		return err
	}
	env := setEnv(w.env(), "PATH", t.bin+string(os.PathListSeparator)+callerPath())
	before := w.binary
	w.binary = strace
	defer func() { w.binary = before }()
	return w.runEnv(w.dir, env, "-f", "-qq", "-s", "256", "-e", "trace=connect,execve,clone,clone3,fork,vfork", "-o", t.connections, b.bin,
		"mutation", "run", "--workers", "2", "--no-annotate", "--json", filepath.FromSlash(l.file))
}

// mutateDartCopies is whether the test ran in at least two workers'
// copies, and each held no android or ios folder and held lib/user.g.dart,
// and, for a workspace's member, the workspace's root pubspec.yaml and
// pubspec.lock and its sibling member core.
func (l *languageMutation) mutateDartCopies() error {
	records, err := l.mutateDartRecords()
	if err != nil {
		return err
	}
	copies := map[string]bool{}
	for _, r := range records {
		if r["cwd"] == r["original_root"] {
			continue
		}
		copies[fmt.Sprint(r["cwd"])] = true
		if r["android"] != false || r["ios"] != false || r["generated"] != true || l.mutateDartPackage() != "." && r["workspace"] != true {
			return fmt.Errorf("the worker's copy %v holds android %v, ios %v, lib/user.g.dart %v, the workspace %v\n%s", r["cwd"], r["android"], r["ios"], r["generated"], r["workspace"], l.m.w.report())
		}
	}
	if len(copies) < 2 {
		return fmt.Errorf("the tests ran in %d workers' copies, not 2: %v\n%s", len(copies), records, l.m.w.report())
	}
	return nil
}

// mutateDartCommand is a flutter command the run started: its folder and
// its arguments.
type mutateDartCommand struct {
	dir  string
	args []string
}

// mutateDartCommands are the flutter commands the run started in workers'
// copies, by copy, in the order each started: every one outside the
// package's real folder.
func (l *languageMutation) mutateDartCommands() (map[string][]mutateDartCommand, error) {
	w := l.m.w
	data, err := w.d.Read(l.mutateDartTools().log)
	if err != nil {
		return nil, err
	}
	real, err := filepath.EvalSymlinks(filepath.Join(w.dir, filepath.FromSlash(l.mutateDartPackage())))
	if err != nil {
		return nil, err
	}
	out := map[string][]mutateDartCommand{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		dir, args, ok := strings.Cut(line, "\t")
		if !ok || dir == real {
			continue
		}
		out[dir] = append(out[dir], mutateDartCommand{dir, strings.Fields(args)})
	}
	if len(out) < 2 {
		return nil, fmt.Errorf("flutter ran in %d workers' copies, not 2:\n%s\n%s", len(out), data, w.report())
	}
	return out, nil
}

// mutateDartConnection is a connection strace saw: the address it went to.
var mutateDartConnection = regexp.MustCompile(`(?m)^(\d+)\s+connect\(\d+, \{sa_family=AF_INET6?, .*?(?:inet_addr\("([^"]+)"\)|inet_pton\(AF_INET6, "([^"]+)")`)

// mutateDartStarted are the threads and processes strace saw started, and
// the programs they ran: a clone's or fork's new thread or process by its
// parent, and an execve's program.
var mutateDartStarted = regexp.MustCompile(`(?m)^(\d+)\s+(?:execve\("[^"]+", \[(.*?)\].*= 0$|.*(?:clone3?|fork|vfork)(?:\(| resumed>).*= (\d+)$)`)

// mutateDartProgramOf names the command the thread tid of strace's trace
// ran, and the commands that started it: the last each one executed.
func mutateDartProgramOf(trace, tid string) string {
	parent, program := map[string]string{}, map[string]string{}
	for _, m := range mutateDartStarted.FindAllStringSubmatch(trace, -1) {
		if m[2] != "" {
			program[m[1]] = m[2]
		} else {
			parent[m[3]] = m[1]
		}
	}
	var chain []string
	for id := tid; id != ""; id = parent[id] {
		if p := program[id]; p != "" && !slices.Contains(chain, p) {
			chain = append(chain, p)
		}
	}
	return "thread " + tid + " of " + strings.Join(chain, ", started by ")
}

// mutateDartResolvedOffline is whether, in each worker's copy, the first
// flutter command was pub get --offline and no other was a pub get, and
// whether no command of the run connected anywhere but this machine.
func (l *languageMutation) mutateDartResolvedOffline() error {
	commands, err := l.mutateDartCommands()
	if err != nil {
		return err
	}
	for dir, list := range commands {
		pubs := 0
		for _, c := range list {
			if slices.Contains(c.args, "pub") {
				pubs++
			}
		}
		first := list[0].args
		if pubs != 1 || !subsequence(first, []string{"pub", "get", "--offline"}) {
			return fmt.Errorf("the worker's copy %s ran %d pub commands, its first %v, not pub get --offline once before its first mutant\n%s", dir, pubs, first, l.m.w.report())
		}
	}
	data, err := l.m.w.d.Read(l.mutateDartTools().connections)
	if err != nil {
		return err
	}
	seen := 0
	for _, match := range mutateDartConnection.FindAllStringSubmatch(string(data), -1) {
		seen++
		address := match[2] + match[3]
		if !strings.HasPrefix(address, "127.") && address != "::1" && !strings.HasPrefix(address, "::ffff:127.") {
			return fmt.Errorf("%s connected to %s", mutateDartProgramOf(string(data), match[1]), address)
		}
	}
	if seen == 0 {
		return fmt.Errorf("strace saw no connection, not even flutter test's own on this machine, so it traced nothing:\n%s", data)
	}
	return nil
}

// mutateDartNoPub is whether every flutter test of the workers' copies ran
// with --no-pub, the baseline and at least one mutant in each copy.
func (l *languageMutation) mutateDartNoPub() error {
	commands, err := l.mutateDartCommands()
	if err != nil {
		return err
	}
	tests := 0
	for dir, list := range commands {
		for _, c := range list {
			if !slices.Contains(c.args, "test") {
				continue
			}
			tests++
			if !slices.Contains(c.args, "--no-pub") {
				return fmt.Errorf("flutter %s ran in the worker's copy %s without --no-pub\n%s", strings.Join(c.args, " "), dir, l.m.w.report())
			}
		}
	}
	if a, err := l.object(); err != nil {
		return err
	} else if f, err := languageRunFile(a, l.file); err != nil {
		return err
	} else if ran, _ := f["ran"].(float64); tests < int(ran)+1 || ran < 2 {
		return fmt.Errorf("flutter test ran %d times in workers' copies, for %v mutants and the baseline\n%s", tests, f["ran"], l.m.w.report())
	}
	return nil
}

// mutateDartAsOneWorker runs the same run again with one worker, nothing
// reused, and is whether it judged every mutant as the two workers did.
func (l *languageMutation) mutateDartAsOneWorker() error {
	w := l.m.w
	outcomes := func(a map[string]any) (map[string]any, error) {
		f, err := languageRunFile(a, l.file)
		if err != nil {
			return nil, err
		}
		out := map[string]any{}
		for _, m := range objectList(f, "mutants") {
			out[fmt.Sprint(m["line"], ":", m["column"], " ", m["original"], " → ", m["replacement"])] = m["outcome"]
		}
		return out, nil
	}
	a, err := l.object()
	if err != nil {
		return err
	}
	two, err := outcomes(a)
	if err != nil {
		return err
	}
	if err := w.d.Remove(filepath.Join(w.dir, ".metrics", "mutate")); err != nil {
		return err
	}
	b, log, _, err := l.capture("mutation run --workers 1 --no-annotate --json " + filepath.FromSlash(l.file))
	if err != nil {
		return err
	}
	one, err := outcomes(b)
	if err != nil {
		return err
	}
	killed := 0
	for _, outcome := range one {
		if outcome == "killed" {
			killed++
		}
	}
	if len(two) == 0 || killed == 0 || fmt.Sprint(two) != fmt.Sprint(one) {
		return fmt.Errorf("two workers judged %v, one %v\n%s", two, one, log)
	}
	return nil
}
