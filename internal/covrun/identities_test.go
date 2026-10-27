package covrun

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
)

// The plans Identities names are Plans' groups: a Go module's sources one
// plan, a Python source by the tests that reach it, of its own, and one no
// test reaches unreached, sorted by language, build root and first source.
func TestIdentitiesNamePlansGroups(t *testing.T) {
	root := t.TempDir()
	for name, text := range map[string]string{
		"go.mod": "module example.com/m\n", "a.go": "package m\n", "sub/b.go": "package sub\n",
		"py/pyproject.toml": "[project]\nname='p'\n", "py/calc.py": "x = 1\n", "py/other.py": "y = 1\n", "py/test_calc.py": "",
		"notes.txt": "",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	at := func(name string) string { return filepath.Join(root, filepath.FromSlash(name)) }
	reach := func(source string) []string {
		if source == at("py/calc.py") {
			return []string{at("py/test_calc.py")}
		}
		return nil
	}
	sources := []string{at("sub/b.go"), at("py/other.py"), at("a.go"), at("py/calc.py"), at("notes.txt")}
	got := Identities(t.Context(), sources, OwnTests, reach)
	want := []coverage.Plan{
		{Language: "go", Dir: root, Sources: []string{at("sub/b.go"), at("a.go")}},
		{Language: "python", Dir: at("py"), Sources: []string{at("py/calc.py")}, Tests: []string{"test_calc.py"}},
		{Language: "python", Dir: at("py"), Sources: []string{at("py/other.py")}, Unreached: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("identities %+v, want %+v", got, want)
	}
	if all := Identities(t.Context(), sources[1:2], AllTests, reach); len(all) != 1 || all[0].Tests != nil || all[0].Unreached {
		t.Errorf("the whole suite's plan %+v names no tests of its own", all)
	}
}

// A Kotlin source is measured by the test classes of its own module that
// reach it, a Dart source by the test files of its package that reach it,
// and a source no build root holds by its own folder.
func TestIdentitiesOfKotlinAndDartAndALooseFile(t *testing.T) {
	root, loose, pkg := t.TempDir(), t.TempDir(), t.TempDir()
	for path, text := range map[string]string{
		filepath.Join(root, "build.gradle.kts"):                   "",
		filepath.Join(root, "src/main/kotlin/board/Board.kt"):     "package board\n\nfun place(i: Int) = i > 5\n",
		filepath.Join(root, "src/test/kotlin/board/BoardTest.kt"): "package board\n\nimport org.junit.jupiter.api.Test\n\nclass BoardTest {\n    @Test\n    fun places() {}\n}\n",
		filepath.Join(pkg, "pubspec.yaml"):                        "name: p\n",
		filepath.Join(pkg, "lib/a.dart"):                          "int a() => 1;\n",
		filepath.Join(pkg, "test/a_test.dart"):                    "void main() {}\n",
		filepath.Join(loose, "a.go"):                              "package a\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	source, test := filepath.Join(root, "src/main/kotlin/board/Board.kt"), filepath.Join(root, "src/test/kotlin/board/BoardTest.kt")
	got := Identities(t.Context(), []string{source}, OwnTests, func(string) []string { return []string{test} })
	if len(got) != 1 || got[0].Dir != root || !reflect.DeepEqual(got[0].Tests, []string{"board.BoardTest"}) || got[0].Unreached {
		t.Errorf("the Kotlin plan %+v, want its module's test classes that reach it", got)
	}
	dart := Identities(t.Context(), []string{filepath.Join(pkg, "lib/a.dart")}, OwnTests, func(string) []string { return []string{filepath.Join(pkg, "test/a_test.dart")} })
	if len(dart) != 1 || dart[0].Dir != pkg || !reflect.DeepEqual(dart[0].Tests, DartTests(pkg, []string{filepath.Join(pkg, "test/a_test.dart")})) || dart[0].Unreached {
		t.Errorf("the Dart plan %+v, want its package's test files that reach it", dart)
	}
	if got := Identities(t.Context(), []string{filepath.Join(loose, "a.go")}, OwnTests, nil); len(got) != 1 || got[0].Dir != loose {
		t.Errorf("a loose Go file's plan %+v, want its own folder", got)
	}
}
