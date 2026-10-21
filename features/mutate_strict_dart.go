package features

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

// Strict Dart line coverage (slice-36): a library no test loads fails
// closed, and evidence rests on the package, or the pub workspace of a
// member, and goes stale when any of its inputs changes.

// strictDartState is what MUT-273's steps keep: the package, and the input
// a step changed with its text before.
type strictDartState struct {
	q                  *strictMutation
	pkg, changed, text string
}

func (q *strictMutation) strictDartSteps(sc *godog.ScenarioContext) {
	d := &strictDartState{q: q}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*d = strictDartState{q: q}
		return ctx, nil
	})
	sc.Step(`^a Flutter package whose lib/lonely\.dart no test imports, beside a library a test reaches$`, func() error { return q.lineProject(strictDartLonely) })
	sc.Step(`^I run "itos-cc ([^"]*)" for both libraries$`, func(args string) error { return q.run(args + " " + q.paths()) })
	sc.Step(`^each function of lib/lonely\.dart is reported as "mutation\.coverage-missing" and none as "mutation\.uncovered-statement", and the run fails$`, q.strictDartLonelyMissing)
	sc.Step(`^a (Flutter app|pub workspace member app beside its dependency core) whose judged library a strict mutation run measured$`, d.measured)
	sc.Step(`^(pubspec\.lock|lib/user\.g\.dart|core/lib/core\.dart) changes, and nothing else$`, d.change)
	sc.Step(`^"itos-cc mutation check --fail-uncovered --json" reports "mutation\.coverage-stale" for each function of the judged library, naming (pubspec\.lock|lib/user\.g\.dart|core/lib/core\.dart), and runs no test$`, d.stale)
	sc.Step(`^a change to integration_test/app_test\.dart alone leaves its evidence fresh$`, d.integrationFresh)
}

// strictDartLonelyMissing is whether the last run reported each function
// of lib/lonely.dart as mutation.coverage-missing, and nothing of it as an
// uncovered statement, beside lib/calc.dart's findings, with the mutant
// counts of a plain run, and failed.
func (q *strictMutation) strictDartLonelyMissing() error {
	a, err := q.object()
	if err != nil {
		return err
	}
	if !slices.Equal(strictLineFacts(a), q.expected) || !reflect.DeepEqual(strictCounts(a), strictCounts(q.plain)) {
		return fmt.Errorf("strict findings/counts %v %v, want %v %v", strictLineFacts(a), strictCounts(a), q.expected, strictCounts(q.plain))
	}
	for _, p := range checkProblems(a) {
		if p["rule"] == "mutation.uncovered-statement" && filepath.ToSlash(fmt.Sprint(p["file"])) == "lib/lonely.dart" {
			return fmt.Errorf("a library no test loads has an uncovered statement: %v", p)
		}
	}
	return q.m.w.exitsWith(1)
}

// strictDartBoard is the judged library of MUT-273's package, which its
// test runs whole, and strictDartBoardReport flutter test's LCOV of it,
// for the board at the package root and the member that calls core.
const (
	strictDartBoard       = "bool low(int i) {\n  return i == 3;\n}\n\nbool high(int i) {\n  return i > 7;\n}\n"
	strictDartBoardReport = "SF:lib/board.dart\nDA:1,2\nDA:2,2\nDA:5,2\nDA:6,2\nend_of_record\n"
	strictDartMemberBoard = "import 'package:core/core.dart';\n\nbool low(int i) {\n  return i == three();\n}\n\nbool high(int i) {\n  return i > 7;\n}\n"
	strictDartMemberLCOV  = "SF:lib/board.dart\nDA:3,2\nDA:4,2\nDA:7,2\nDA:8,2\nend_of_record\n"
)

// measured writes MUT-273's package, kind, resolved as its user would,
// and runs a strict mutation run of its judged library, which passes.
func (d *strictDartState) measured(kind string) error {
	q, w := d.q, d.q.m.w
	q.language = "dart"
	d.pkg = "."
	name, pubspec, board, report := "board", coverageDartFlutterPubspec("board", false), strictDartBoard, strictDartBoardReport
	files := map[string]string{}
	if kind != "Flutter app" {
		d.pkg, name, board, report = "app", "app", strictDartMemberBoard, strictDartMemberLCOV
		pubspec = "name: app\npublish_to: none\nenvironment:\n  sdk: ^3.6.0\nresolution: workspace\n" +
			"dependencies:\n  flutter:\n    sdk: flutter\n  core:\n    path: ../core\ndev_dependencies:\n  flutter_test:\n    sdk: flutter\n"
		files["pubspec.yaml"] = coverageDartPubspec("workspace") + "workspace:\n  - app\n  - core\n"
		files["core/pubspec.yaml"] = coverageDartPubspec("core") + "resolution: workspace\n"
		files["core/lib/core.dart"] = "int three() {\n  return 3;\n}\n"
	}
	in := func(rel string) string { return path.Join(d.pkg, rel) }
	q.file, q.testFile = in("lib/board.dart"), in("test/board_test.dart")
	q.selected = []string{q.file}
	files[in("pubspec.yaml")] = pubspec
	files[q.file] = board
	if kind == "Flutter app" {
		// The app's generated Dart, which its tests compile against.
		files["lib/user.g.dart"] = "// GENERATED CODE - DO NOT MODIFY BY HAND\n\nint userId(int i) => i + 1;\n"
	}
	files[q.testFile] = "import 'package:flutter_test/flutter_test.dart';\nimport 'package:" + name + "/board.dart';\n\n" +
		"void main() {\n  test('board', () {\n    expect(low(3), isTrue);\n    expect(low(4), isFalse);\n    expect(high(8), isTrue);\n    expect(high(7), isFalse);\n  });\n}\n"
	files[in("integration_test/app_test.dart")] = "import 'package:flutter_test/flutter_test.dart';\nimport 'package:" + name + "/board.dart';\n\n" +
		"void main() {\n  test('app', () {\n    expect(low(3), isTrue);\n  });\n}\n"
	if err := q.m.writeAll(files); err != nil {
		return err
	}
	edges := map[string][]string{q.testFile: {q.file}, in("integration_test/app_test.dart"): {q.file}}
	if d.pkg != "." {
		edges[q.file] = []string{"core/lib/core.dart"}
	}
	w.imports(edges)
	dir := filepath.Join(w.dir, filepath.FromSlash(d.pkg))
	if err := w.d.Coverage(dir, Coverage{Language: "dart", Report: report}); err != nil {
		return err
	}
	// flutter pub get wrote the lock where the binary runs; in process
	// the fixture holds one as it would.
	if !w.d.Exists(filepath.Join(w.dir, "pubspec.lock")) {
		if err := w.write("pubspec.lock", "packages: {}\nsdks:\n  dart: \">=3.6.0 <4.0.0\"\n"); err != nil {
			return err
		}
	}
	if err := w.d.Tests(dir, Tests{Language: "dart", Command: strictDartFlutterTest + "test/board_test.dart"}); err != nil {
		return err
	}
	if _, err := q.fresh("mutation run --fail-uncovered --json "+q.file, 0); err != nil {
		return err
	}
	_, err := q.fresh("mutation check --fail-uncovered --json "+q.file, 0)
	return err
}

// change changes the input file, from the scratch folder, keeping its text
// to restore.
func (d *strictDartState) change(file string) error {
	d.changed, d.text = file, d.q.m.read(file)
	comment := "\n// changed\n"
	if strings.HasSuffix(file, ".lock") {
		comment = "\n# changed\n"
	}
	return d.q.m.w.write(file, d.text+comment)
}

// stale is whether a strict check reports each function of the judged
// library's evidence stale, naming the changed input, without running a
// test or writing a file.
func (d *strictDartState) stale(file string) error {
	q := d.q
	if err := q.observed(); err != nil {
		return err
	}
	a, log, code, err := q.capture("mutation check --fail-uncovered --json " + q.file)
	if err != nil {
		return err
	}
	ps := q.findings(a, "mutation.coverage-stale", "")
	var functions []string
	for _, p := range ps {
		if filepath.ToSlash(fmt.Sprint(p["file"])) != q.file || !strings.Contains(filepath.ToSlash(fmt.Sprint(p["message"])), file) {
			return fmt.Errorf("stale evidence of %s does not name %s: %v", q.file, file, p)
		}
		functions = append(functions, fmt.Sprint(p["function"]))
	}
	slices.Sort(functions)
	if code != 1 || len(functions) != 2 || !strictNamed(functions[0], "high") || !strictNamed(functions[1], "low") {
		return fmt.Errorf("stale strict Dart evidence exit%d %v: %v\n%s", code, functions, a, log)
	}
	return q.quiet(log)
}

// integrationFresh restores the changed input, changes the package's
// integration_test/app_test.dart alone, and is whether a strict check
// finds the evidence fresh, no strict coverage finding, and exits 0, the
// mutation results resting on the tests flutter test runs (slice-38).
func (d *strictDartState) integrationFresh() error {
	q := d.q
	if err := q.m.w.write(d.changed, d.text); err != nil {
		return err
	}
	test := path.Join(d.pkg, "integration_test/app_test.dart")
	if err := q.m.w.write(test, q.m.read(test)+"\n// changed\n"); err != nil {
		return err
	}
	a, log, code, err := q.capture("mutation check --fail-uncovered --json " + q.file)
	if err != nil {
		return err
	}
	if code != 0 || len(strictLineFacts(a)) != 0 {
		return fmt.Errorf("a change to %s staled the evidence exit%d: %v\n%s", test, code, a, log)
	}
	return nil
}
