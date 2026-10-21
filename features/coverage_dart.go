package features

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
)

// The Dart projects coverage.feature measures, Flutter apps and packages
// that flutter test runs (slice-32), and what is checked of their runs.

// coverageDartSource is a Dart library of two functions: place, whose if
// runs only for an a over 1, and other.
const coverageDartSource = "int place(int a) {\n  var n = 0;\n  if (a > 1) {\n    n = 1;\n  }\n  return n;\n}\n\n" +
	"int other(int a) {\n  return a + 1;\n}\n"

// coverageDartTest is a flutter_test test of the package pkg that imports
// its lib/board.dart and calls call.
func coverageDartTest(pkg, call string) string {
	return "import 'package:flutter_test/flutter_test.dart';\nimport 'package:" + pkg + "/board.dart';\n\n" +
		"void main() {\n  test('board', () {\n    expect(" + call + ", isNotNull);\n  });\n}\n"
}

// coverageDartFlutterPubspec is the pubspec.yaml of the Flutter package
// name, which depends on the flutter SDK and tests with flutter_test, a pub
// workspace's member when member says so.
func coverageDartFlutterPubspec(name string, member bool) string {
	text := "name: " + name + "\npublish_to: none\nenvironment:\n  sdk: ^3.6.0\n"
	if member {
		text += "resolution: workspace\n"
	}
	return text + "dependencies:\n  flutter:\n    sdk: flutter\ndev_dependencies:\n  flutter_test:\n    sdk: flutter\n"
}

// coverageDartPubspec is the pubspec.yaml of the Dart package name, which
// depends on nothing, the flutter SDK neither.
func coverageDartPubspec(name string) string {
	return "name: " + name + "\npublish_to: none\nenvironment:\n  sdk: ^3.6.0\n"
}

// flutterApp writes the Flutter app board in the folder dir of the scratch
// folder: lib/board.dart and a test calling place(5).
func (c *coverageState) flutterApp(dir string) error {
	c.roots[dir] = "dart"
	return c.writeIn(dir, map[string]string{
		"pubspec.yaml":         coverageDartFlutterPubspec("board", false),
		"lib/board.dart":       coverageDartSource,
		"test/board_test.dart": coverageDartTest("board", "place(5)"),
	})
}

// dartProject is a Dart project at the scratch folder whose build root
// holds pubspec.yaml, for COV-05: board's source and test, its pubspec
// depending on nothing until the setup gives it a dependency.
func (c *coverageState) dartProject() map[string]string {
	c.roots["."] = "dart"
	return map[string]string{
		"pubspec.yaml":         coverageDartPubspec("board"),
		"lib/board.dart":       coverageDartSource,
		"test/board_test.dart": coverageDartTest("board", "place(5)"),
	}
}

// flutterDependency makes the Dart project's pubspec.yaml depend on the
// flutter SDK, and declares the Flutter that measures it.
func (c *coverageState) flutterDependency() error {
	data, err := c.w.readIn(c.w.dir, "pubspec.yaml")
	if err != nil {
		return err
	}
	if !strings.HasPrefix(string(data), "name: board\n") {
		return fmt.Errorf("the project's pubspec.yaml is not board's: %s", data)
	}
	if err := c.w.write("pubspec.yaml", coverageDartFlutterPubspec("board", false)); err != nil {
		return err
	}
	return c.declare(".", Coverage{Language: "dart"})
}

// flutterMissing writes a Flutter app whose runs find no flutter.
func (c *coverageState) flutterMissing() error {
	c.missing, c.w.without = "flutter", []string{"flutter"}
	return c.flutterApp(".")
}

// unresolvedDart writes a Flutter app with the Flutter that measures it,
// then takes away what pub get resolved: .dart_tool, which holds
// package_config.json, and pubspec.lock.
func (c *coverageState) unresolvedDart() error {
	c.missing = "flutter pub get"
	if err := c.flutterApp("."); err != nil {
		return err
	}
	if err := c.declare(".", Coverage{Language: "dart"}); err != nil {
		return err
	}
	for _, resolved := range []string{".dart_tool", "pubspec.lock"} {
		if err := c.w.d.Remove(c.abs(resolved)); err != nil {
			return err
		}
	}
	if c.w.d.Exists(c.abs(".dart_tool/package_config.json")) {
		return errors.New("the app's .dart_tool/package_config.json is still there")
	}
	return nil
}

// fvmApp writes a Flutter app whose .fvmrc pins its SDK, and declares the
// Flutter that measures it, which links .fvm/flutter_sdk to that SDK as
// fvm use does.
func (c *coverageState) fvmApp() error {
	if err := c.flutterApp("."); err != nil {
		return err
	}
	if err := c.w.write(".fvmrc", "{\n  \"flutter\": \"stable\"\n}\n"); err != nil {
		return err
	}
	if err := c.declare(".", Coverage{Language: "dart"}); err != nil {
		return err
	}
	if !c.w.d.Exists(c.abs(".fvm/flutter_sdk/bin/flutter")) {
		return errors.New("no .fvm/flutter_sdk links the app's SDK")
	}
	return nil
}

// fvmRunsTests is whether the app's coverage command is its
// .fvm/flutter_sdk/bin/flutter, no command runs another flutter, and the
// tests it ran measured place.
func (c *coverageState) fvmRunsTests() error {
	commands := c.commands()
	if len(commands) == 0 {
		return fmt.Errorf("no coverage command runs for the app\n%s", c.w.report())
	}
	if sdk, _ := c.rel(commands[0].words[0]); sdk != ".fvm/flutter_sdk/bin/flutter" {
		return fmt.Errorf("the app's coverage runs %s, not .fvm/flutter_sdk/bin/flutter\n%s", commands[0].words[0], c.w.report())
	}
	for _, a := range c.announced(c.w.stderr) {
		if sdk, _ := c.rel(a.words[0]); program(a.words[0]) == "flutter" && sdk != ".fvm/flutter_sdk/bin/flutter" {
			return fmt.Errorf("coverage runs %q\n%s", a.command, c.w.report())
		}
	}
	entries, err := c.listed()
	if err != nil {
		return err
	}
	if got, err := coverageOf(entries, covFunction{"lib/board.dart", "place"}); err != nil || got == nil || *got == 0 {
		return fmt.Errorf("the tests .fvm/flutter_sdk/bin/flutter ran do not measure place\n%s", c.w.report())
	}
	return nil
}

// pureDart writes a Dart package that does not depend on the flutter SDK.
func (c *coverageState) pureDart() error {
	c.roots["."] = "dart"
	return c.writeIn(".", map[string]string{
		"pubspec.yaml":         coverageDartPubspec("board"),
		"lib/board.dart":       coverageDartSource,
		"test/board_test.dart": "import 'package:test/test.dart';\nimport 'package:board/board.dart';\n\nvoid main() {\n  test('board', () {\n    expect(place(5), 1);\n  });\n}\n",
	})
}

// onlyFlutterMeasured is whether stderr says, of the project's folder, that
// only Flutter apps and packages are measured.
func (c *coverageState) onlyFlutterMeasured() error {
	for _, line := range strings.Split(c.w.stderr, "\n") {
		rest, ok := strings.CutPrefix(line, "itos-cc: coverage: ")
		if !ok {
			continue
		}
		dir, reason, ok := strings.Cut(rest, ": ")
		if rel, in := c.rel(dir); ok && in && rel == c.project && strings.Contains(reason, "only Flutter apps and packages are measured") {
			return nil
		}
	}
	return fmt.Errorf("stderr does not say only Flutter apps and packages are measured\n%s", c.w.report())
}

// pubWorkspace writes a pub workspace whose root lists the Flutter
// packages a and b, each with a lib/board.dart of the same name: a's test
// calls place, b's other, so each member's report measures what the other's
// does not. It declares the Flutter that measures them, resolving the
// workspace at its root.
func (c *coverageState) pubWorkspace(a, b string) error {
	root := coverageDartPubspec("workspace") + "workspace:\n  - " + a + "\n  - " + b + "\n"
	files := map[string]string{"pubspec.yaml": root}
	for member, call := range map[string]string{a: "place(5)", b: "other(1)"} {
		files[member+"/pubspec.yaml"] = coverageDartFlutterPubspec(member, true)
		files[member+"/lib/board.dart"] = coverageDartSource
		files[member+"/test/board_test.dart"] = coverageDartTest(member, call)
		c.roots[member] = "dart"
	}
	if err := c.writeIn(".", files); err != nil {
		return err
	}
	c.want[covFunction{a + "/lib/board.dart", "place"}] = 1
	c.want[covFunction{a + "/lib/board.dart", "other"}] = 0
	c.want[covFunction{b + "/lib/board.dart", "place"}] = 0
	c.want[covFunction{b + "/lib/board.dart", "other"}] = 1
	return c.w.d.Coverage(c.abs("."), Coverage{Language: "dart"})
}

// flutterRunsIn is whether flutter test runs in a and in b, and no
// coverage command runs at the scratch folder, the workspace's root.
func (c *coverageState) flutterRunsIn(a, b string) error {
	announced := c.announced(c.w.stderr)
	for _, member := range []string{a, b} {
		if !slices.ContainsFunc(announced, func(x covAnnouncement) bool {
			return x.dir == member && program(x.words[0]) == "flutter" && slices.Contains(x.words, "test")
		}) {
			return fmt.Errorf("flutter test does not run in %s\n%s", member, c.w.report())
		}
	}
	for _, x := range announced {
		if x.dir == "." {
			return fmt.Errorf("coverage runs %q at the workspace root\n%s", x.command, c.w.report())
		}
	}
	return nil
}

// membersRead is whether each member's functions have the coverage its own
// tests gave them: some for a function its test called, none for one it
// did not, which the other member's report, read against the wrong
// directory, would swap.
func (c *coverageState) membersRead() error {
	entries, err := c.listed()
	if err != nil {
		return err
	}
	for f, ran := range c.want {
		got, err := coverageOf(entries, f)
		if err != nil {
			return fmt.Errorf("%v\n%s", err, c.w.report())
		}
		if got == nil || (*got > 0) != (ran > 0) {
			return fmt.Errorf("%s of %s has %s%% coverage, though its member's test %s it\n%s",
				f.name, f.file, shown(got), map[bool]string{true: "called", false: "never called"}[ran > 0], c.w.report())
		}
	}
	return nil
}

// unloadedApp writes a Flutter app whose test loads loaded, its
// lib/board.dart, and two libraries no test loads, and declares the
// Flutter that measures it.
func (c *coverageState) unloadedApp(loaded, unused, ignored string) error {
	if loaded != "lib/board.dart" {
		return fmt.Errorf("the app's test loads lib/board.dart, not %s", loaded)
	}
	if err := c.flutterApp("."); err != nil {
		return err
	}
	for _, name := range []string{unused, ignored} {
		function := strings.TrimSuffix(path.Base(name), ".dart")
		if err := c.w.write(name, "int "+function+"(int a) {\n  return a + 1;\n}\n"); err != nil {
			return err
		}
	}
	return c.declare(".", Coverage{Language: "dart"})
}

// firstLine makes line the first line of the file name.
func (c *coverageState) firstLine(name, line string) error {
	data, err := c.w.readIn(c.w.dir, name)
	if err != nil {
		return err
	}
	return c.w.write(name, line+"\n"+string(data))
}

// fileFunctions are the functions the last run listed of the file name,
// an error when it listed none.
func (c *coverageState) fileFunctions(name string) ([]crapEntry, error) {
	entries, err := c.listed()
	if err != nil {
		return nil, err
	}
	var out []crapEntry
	for _, e := range entries {
		if strings.ReplaceAll(e.File, "\\", "/") == name {
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no function of %s is listed\n%s", name, c.w.report())
	}
	return out, nil
}

// functionsAt is whether every function of the file name has percent
// coverage.
func (c *coverageState) functionsAt(name string, percent int) error {
	functions, err := c.fileFunctions(name)
	if err != nil {
		return err
	}
	want := float64(percent)
	for _, e := range functions {
		if shown(e.Coverage) != shown(&want) {
			return fmt.Errorf("%s has %s%% coverage, not %d%%\n%s", e.id(), shown(e.Coverage), percent, c.w.report())
		}
	}
	return nil
}

// functionsUnscored is whether every function of the file name has
// neither coverage nor a CRAP score.
func (c *coverageState) functionsUnscored(name string) error {
	functions, err := c.fileFunctions(name)
	if err != nil {
		return err
	}
	for _, e := range functions {
		if e.Coverage != nil || e.CRAP != nil {
			return fmt.Errorf("%s has coverage %s and CRAP %s\n%s", e.id(), shown(e.Coverage), shown(e.CRAP), c.w.report())
		}
	}
	return nil
}
