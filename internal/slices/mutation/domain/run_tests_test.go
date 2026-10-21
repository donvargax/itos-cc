package domain

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/donvargax/itos-cc/internal/lang/port/porttest"
)

// runTestsAround is a pub workspace at /w: its root's member app, whose
// library a.dart the paths below reach, beside its sibling member core,
// and a loose Dart file under tool/ with no pubspec.yaml above it.
var runTestsAround = porttest.Surroundings{Dir: "/w", Files: map[string]string{
	"/w/pubspec.yaml":      "name: workspace\n",
	"/w/app/pubspec.yaml":  "name: app\n",
	"/w/core/pubspec.yaml": "name: core\n",
}}

func at(paths ...string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, filepath.FromSlash(p))
	}
	return out
}

func TestADartOutcomeRestsOnTheTestsFlutterTestRuns(t *testing.T) {
	source := filepath.FromSlash("/w/app/lib/a.dart")
	reached := at(
		"/w/app/test/a_test.dart",
		"/w/app/test/helpers.dart",
		"/w/app/test/integration_test/support.dart",
		"/w/app/integration_test/app_test.dart",
		"/w/app/integration_test/robot.dart",
		"/w/app/test_driver/app.dart",
		"/w/app/tool/smoke_test.dart",
		"/w/core/test/core_test.dart",
		"/w/core/test/fixtures.dart",
		"/w/core/integration_test/helpers.dart",
	)
	want := at(
		"/w/app/test/a_test.dart",
		"/w/app/test/helpers.dart",
		"/w/app/test/integration_test/support.dart",
		"/w/core/test/fixtures.dart",
	)
	if got := RunTests(t.Context(), runTestsAround, source, reached); !reflect.DeepEqual(got, want) {
		t.Fatalf("RunTests = %v, want %v", got, want)
	}
	// No runnable test of test/ reaches it: its run runs nothing, so no
	// support file is on any run's way.
	if got := RunTests(t.Context(), runTestsAround, source, at("/w/app/test/helpers.dart", "/w/app/integration_test/app_test.dart")); got != nil {
		t.Fatalf("RunTests with no runnable test = %v, want none", got)
	}
	// Without a pubspec.yaml above it, a Dart file's package is its folder.
	loose := porttest.Surroundings{Dir: "/x"}
	if got := RunTests(t.Context(), loose, filepath.FromSlash("/x/tool/b.dart"), at("/x/tool/test/b_test.dart", "/x/test/b_test.dart")); !reflect.DeepEqual(got, at("/x/tool/test/b_test.dart")) {
		t.Fatalf("RunTests of a loose Dart file = %v", got)
	}
	if got := RunTests(t.Context(), porttest.Surroundings{}, "b.dart", at("test/b_test.dart")); got != nil {
		t.Fatalf("RunTests of a Dart file with no absolute path = %v", got)
	}
}

func TestOtherLanguagesRestOnEveryTestThatReachesThem(t *testing.T) {
	tests := at("/w/integration_test/app_test.py", "/w/tests/test_a.py")
	if got := RunTests(t.Context(), runTestsAround, filepath.FromSlash("/w/a.py"), tests); !reflect.DeepEqual(got, tests) {
		t.Fatalf("RunTests of Python = %v", got)
	}
	whole := map[string]string{"integration_test/app_test.py": "1", "tests/test_a.py": "2"}
	s := SuiteOf(t.Context(), runTestsAround, filepath.FromSlash("/w"), filepath.FromSlash("/w/a.py"), whole)
	if !reflect.DeepEqual(s.Of(ScopeAllTests), whole) || !reflect.DeepEqual(s.Of("./run.sh"), whole) {
		t.Fatalf("SuiteOf Python = %v", s)
	}
}

func TestADartAllTestsOutcomeRestsOnItsPackagesTestFolder(t *testing.T) {
	whole := map[string]string{
		"app/test/a_test.dart":               "1",
		"app/test/helpers.dart":              "2",
		"app/integration_test/app_test.dart": "3",
		"app/test_driver/app.dart":           "4",
		"app/tool/smoke_test.dart":           "5",
	}
	s := SuiteOf(t.Context(), runTestsAround, filepath.FromSlash("/w"), filepath.FromSlash("/w/app/lib/a.dart"), whole)
	runs := map[string]string{"app/test/a_test.dart": "1", "app/test/helpers.dart": "2"}
	if !reflect.DeepEqual(s.Of(ScopeAllTests), runs) {
		t.Fatalf("an --all-tests outcome rests on %v, want %v", s.Of(ScopeAllTests), runs)
	}
	// A --test-command may run any test of the package.
	if !reflect.DeepEqual(s.Of("flutter test integration_test"), whole) {
		t.Fatalf("a --test-command outcome rests on %v", s.Of("flutter test integration_test"))
	}
	if s := SuiteOf(t.Context(), runTestsAround, filepath.FromSlash("/w"), filepath.FromSlash("/w/app/lib/a.dart"), nil); s.Of(ScopeAllTests) != nil || s.Of("x") != nil {
		t.Fatalf("no whole suite gives %v", s)
	}
	snap := Snapshot{Units: []UnitResult{{Mutants: []Mutant{{Scope: ScopeAllTests}, {Scope: "flutter test integration_test"}}}}}
	RecordSuiteEvidence(&snap, s, nil)
	if got := snap.Units[0].Mutants; !reflect.DeepEqual(got[0].SuiteEvidence.Tests, runs) || !reflect.DeepEqual(got[1].SuiteEvidence.Tests, whole) {
		t.Fatalf("recorded evidence %v %v", got[0].SuiteEvidence, got[1].SuiteEvidence)
	}
	whole["app/integration_test/app_test.dart"] = "changed"
	now := SuiteOf(t.Context(), runTestsAround, filepath.FromSlash("/w"), filepath.FromSlash("/w/app/lib/a.dart"), whole)
	all, command := snap.Units[0].Mutants[0], snap.Units[0].Mutants[1]
	if got := BroadChanges(all, now.Of(all.TestScope()), nil); len(got) != 0 {
		t.Fatalf("an --all-tests outcome is stale over %v", got)
	}
	if got := BroadChanges(command, now.Of(command.TestScope()), nil); !reflect.DeepEqual(got, []string{"app/integration_test/app_test.dart"}) {
		t.Fatalf("a --test-command outcome is stale over %v", got)
	}
}
