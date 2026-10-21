package features

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

// A Dart result's freshness rests on the tests its run runs (slice-38):
// flutter test never runs integration_test/ or test_driver/ (q-76), so a
// change there leaves a result recorded by an own-test or an --all-tests
// run fresh, and a change to the test under test/ that reaches the file
// makes it stale.

// dartFreshFile, dartFreshTest and dartFreshDevice are MUT-274's judged
// library, the test under test/ that reaches it, and the files beside the
// package's test/ folder that import it, which flutter test never runs.
const (
	dartFreshFile = "lib/a.dart"
	dartFreshTest = "test/a_test.dart"
)

var dartFreshDevice = []string{"integration_test/app_test.dart", "test_driver/app.dart"}

func (l *languageMutation) dartFreshSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a Flutter package where test/a_test\.dart reaches lib/a\.dart, and integration_test/app_test\.dart and test_driver/app\.dart import lib/a\.dart$`, l.dartFreshProject)
	sc.Step(`^a mutant of lib/a\.dart has a result recorded by "itos-cc (mutation run(?: --all-tests)?)"$`, l.dartFreshRecord)
	sc.Step(`^integration_test/app_test\.dart and test_driver/app\.dart change, and nothing else$`, func() error {
		for _, file := range dartFreshDevice {
			if err := l.appendComment(file); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^"itos-cc mutation check --json" reports no result stale and exits 0$`, l.dartFreshCheck)
	sc.Step(`^after test/a_test\.dart changes, it reports that result stale$`, func() error {
		if err := l.appendComment(dartFreshTest); err != nil {
			return err
		}
		return l.reachStale(languageReachProject{b: dartFreshFile, reaching: dartFreshTest})
	})
}

// dartFreshProject writes MUT-274's Flutter package, fresh: lib/a.dart's
// low, which test/a_test.dart judges, and which
// integration_test/app_test.dart and test_driver/app.dart import too. The
// run's flutter test is declared for both commands a row's run starts:
// the reaching test's, and the whole suite's with no path.
func (l *languageMutation) dartFreshProject() error {
	w := l.m.w
	l.file, l.language = dartFreshFile, "dart"
	if err := l.m.writeAll(map[string]string{
		"pubspec.yaml": coverageDartFlutterPubspec("fresh", false),
		dartFreshFile:  "bool low(int i) {\n  return i == 3;\n}\n",
		dartFreshTest: "import 'package:flutter_test/flutter_test.dart';\nimport 'package:fresh/a.dart';\n\n" +
			"void main() {\n  test('a', () {\n    expect(low(3), isTrue);\n    expect(low(4), isFalse);\n  });\n}\n",
		dartFreshDevice[0]: "import 'package:flutter_test/flutter_test.dart';\nimport 'package:fresh/a.dart';\n\n" +
			"void main() {\n  test('app', () {\n    expect(low(3), isTrue);\n  });\n}\n",
		dartFreshDevice[1]: "import 'package:fresh/a.dart';\n\nvoid main() {\n  low(3);\n}\n",
	}); err != nil {
		return err
	}
	w.imports(map[string][]string{dartFreshTest: {dartFreshFile}, dartFreshDevice[0]: {dartFreshFile}, dartFreshDevice[1]: {dartFreshFile}})
	if err := w.d.Coverage(w.dir, Coverage{Language: "dart", Report: "SF:lib/a.dart\nDA:1,2\nDA:2,2\nend_of_record\n"}); err != nil {
		return err
	}
	for _, command := range []string{strictDartFlutterTest + dartFreshTest, strings.TrimSpace(strictDartFlutterTest)} {
		if err := w.d.Tests(w.dir, Tests{Language: "dart", Command: command}); err != nil {
			return err
		}
	}
	return nil
}

// dartFreshRecord runs the row's mutation run over lib/a.dart, which kills
// every mutant it runs, and is whether a check then finds its result
// fresh, so that only the change that follows can make it stale.
func (l *languageMutation) dartFreshRecord(run string) error {
	args := strings.Replace(run, "mutation run", "mutation run --json", 1) + " " + l.file
	a, log, code, err := l.capture(args)
	if err != nil {
		return err
	}
	f, err := languageRunFile(a, l.file)
	if err != nil {
		return err
	}
	if code != 0 || f["ran"] == float64(0) || f["killed"] != f["ran"] {
		return fmt.Errorf("%s exit%d did not kill every mutant of %s: %v\n%s", args, code, l.file, f, log)
	}
	return l.dartFreshCheck()
}

// dartFreshCheck is whether mutation check finds every function of
// lib/a.dart fresh, reports no stale result, and exits 0.
func (l *languageMutation) dartFreshCheck() error {
	a, log, code, err := l.capture("mutation check --json " + l.file)
	if err != nil {
		return err
	}
	f, err := languageRunFile(a, l.file)
	if err != nil {
		return err
	}
	functions := objectList(f, "functions")
	for _, u := range functions {
		if u["state"] != "fresh" {
			return fmt.Errorf("%s's function %v is %v, not fresh: %v\n%s", l.file, u["function"], u["state"], a, log)
		}
	}
	for _, p := range checkProblems(a) {
		if p["rule"] == "mutation.stale" && filepath.ToSlash(fmt.Sprint(p["file"])) == l.file {
			return fmt.Errorf("a result of %s is stale: %v", l.file, p)
		}
	}
	if len(functions) == 0 || code != 0 {
		return fmt.Errorf("mutation check of %s exit%d: %v\n%s", l.file, code, a, log)
	}
	return nil
}
