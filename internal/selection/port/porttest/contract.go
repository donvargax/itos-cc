package porttest

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/internal/selection/port"
)

// TreeFixture is a working directory a TreeContract case lays out.
type TreeFixture struct {
	// Files are its files, by slash-separated paths relative to it, with
	// their text.
	Files map[string]string
	// Git says whether it is a git repository. Without one, Changed is
	// NoGit.
	Git bool
	// Changed are the files of Files, by the same paths, that git reports
	// as changed since the last commit, uncommitted or untracked; the rest
	// are committed. Only a repository has any.
	Changed []string
}

// TreeContract holds a Tree to what every one, the fake and the disk's,
// must do: what it finds, the failures a fixture both can hold gives, and
// a done context. open lays out a fixture and returns the Tree over it and
// its working directory, absolute; it may make that the test's working
// directory, so no case runs in parallel. What only a disk can do
// (symlinks, permissions, OS error texts, what git ignores) is the
// adapter's own tests'; a failure the fake is configured with, the fake's.
func TreeContract(t *testing.T, open func(t *testing.T, f TreeFixture) (port.Tree, string)) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, open func(t *testing.T, f TreeFixture) (port.Tree, string))
	}{
		{"discover gives every supported file, absolute, split, sorted and once", discoversEverySupportedFile},
		{"a directory root gives what is below it and a file root that file", discoversUnderItsRoots},
		{"hidden, skipped and build output directories are skipped, a file under one taken when named", skipsWhatAWalkSkips},
		{"a Flutter app's platform folders and generated Dart are skipped, each file taken when named", skipsFlutterOutput},
		{"a root that is not there is unwalkable", refusesAnAbsentRoot},
		{"exists says whether a file or a directory is there", existsWhereSomethingIs},
		{"read gives what a file holds, and an absent one is unreadable", readsWhatAFileHolds},
		{"changed without a repository is no git", changedWithoutGitIsNoGit},
		{"changed is the uncommitted and untracked supported files", changedListsWhatGitReports},
		{"a done context is cancelled", aDoneContextIsCancelled},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { c.run(t, open) })
	}
}

// project is a working directory holding a source and a test in each of
// three languages, and files of no supported language, one of them alone
// in its directory.
var project = map[string]string{
	"src/b.go":          "package src\n",
	"src/a.go":          "package src\n\nfunc A() {}\n",
	"src/a_test.go":     "package src\n",
	"web/cart.ts":       "export const cart = 1;\n",
	"web/cart.test.ts":  "test(\"cart\", () => {});\n",
	"web/types.d.ts":    "declare const x: number;\n",
	"lib/price.py":      "def price():\n    return 1\n",
	"lib/test_price.py": "def test_price():\n    pass\n",
	"README.md":         "# project\n",
	"docs/guide.md":     "# guide\n",
}

func discoversEverySupportedFile(t *testing.T, open func(t *testing.T, f TreeFixture) (port.Tree, string)) {
	tree, dir := open(t, TreeFixture{Files: project})
	wantSources := absolute(dir, "lib/price.py", "src/a.go", "src/b.go", "web/cart.ts")
	wantTests := absolute(dir, "lib/test_price.py", "src/a_test.go", "web/cart.test.ts")
	for _, roots := range [][]string{{"."}, {".", "src", "src/a.go", dir}} {
		discovers(t, tree, roots, wantSources, wantTests)
	}
}

func discoversUnderItsRoots(t *testing.T, open func(t *testing.T, f TreeFixture) (port.Tree, string)) {
	tree, dir := open(t, TreeFixture{Files: project})
	discovers(t, tree, []string{"src"}, absolute(dir, "src/a.go", "src/b.go"), absolute(dir, "src/a_test.go"))
	discovers(t, tree, []string{"web/"}, absolute(dir, "web/cart.ts"), absolute(dir, "web/cart.test.ts"))
	discovers(t, tree, []string{filepath.Join(dir, "lib")}, absolute(dir, "lib/price.py"), absolute(dir, "lib/test_price.py"))
	discovers(t, tree, []string{"web/cart.test.ts"}, nil, absolute(dir, "web/cart.test.ts"))
	discovers(t, tree, []string{"lib/price.py", "src/b.go"}, absolute(dir, "lib/price.py", "src/b.go"), nil)
	discovers(t, tree, []string{"README.md", "docs", "web/types.d.ts"}, nil, nil)
}

// skipped is a working directory with a file under each directory a walk
// skips, and two it walks: out, a build output's name with no build
// manifest beside it, and web, which holds one.
var skipped = map[string]string{
	"main.go":                 "package main\n",
	"out/o.go":                "package out\n",
	".hidden/h.go":            "package hidden\n",
	"node_modules/n/index.ts": "export const n = 1;\n",
	"vendor/v/v.go":           "package v\n",
	"pkg/testdata/t.go":       "package testdata\n",
	".metrics/m.go":           "package metrics\n",
	"venv/lib/site.py":        "x = 1\n",
	"web/package.json":        "{}\n",
	"web/app.ts":              "export const app = 1;\n",
	"web/dist/bundle.js":      "var b = 1;\n",
}

func skipsWhatAWalkSkips(t *testing.T, open func(t *testing.T, f TreeFixture) (port.Tree, string)) {
	tree, dir := open(t, TreeFixture{Files: skipped})
	discovers(t, tree, []string{"."}, absolute(dir, "main.go", "out/o.go", "web/app.ts"), nil)
	discovers(t, tree, []string{"pkg"}, nil, nil)
	discovers(t, tree, []string{"web"}, absolute(dir, "web/app.ts"), nil)
	// A file is taken when named, wherever it is, and a skipped directory
	// is walked when it is the root.
	discovers(t, tree, []string{".hidden/h.go", "vendor/v/v.go", "web/dist/bundle.js"},
		absolute(dir, ".hidden/h.go", "vendor/v/v.go", "web/dist/bundle.js"), nil)
	discovers(t, tree, []string{".metrics", "pkg/testdata"}, absolute(dir, ".metrics/m.go", "pkg/testdata/t.go"), nil)
}

// flutterApp is a working directory holding a Flutter app, its pubspec.yaml
// depending on the flutter SDK, with Dart of its own, generated Dart of
// every kind, a source in its build output and in three of its platform
// folders; a pure Dart package, pkg, with its own l10n.yaml; and a web
// folder, plain/web, beside no pubspec.yaml.
var flutterApp = map[string]string{
	"pubspec.yaml":                                "name: app\ndependencies:\n  flutter:\n    sdk: flutter\n",
	"l10n.yaml":                                   "template-arb-file: app_en.arb\n",
	"lib/main.dart":                               "void main() {}\n",
	"lib/late.dart":                               "class Late {}\n// GENERATED CODE - DO NOT MODIFY BY HAND\n",
	"lib/model.dart":                              "/*\n * GENERATED CODE - DO NOT MODIFY BY HAND\n */\nclass Model {}\n",
	"lib/user.g.dart":                             "part of 'user.dart';\n",
	"lib/user.freezed.dart":                       "part of 'user.dart';\n",
	"lib/api/user.pbserver.dart":                  "class UserService {}\n",
	"lib/generated_plugin_registrant.dart":        "void registerPlugins() {}\n",
	"lib/l10n/app_localizations.dart":             "abstract class AppLocalizations {}\n",
	"lib/l10n/app_localizations_en.dart":          "class AppLocalizationsEn {}\n",
	"lib/l10n/strings.dart":                       "const strings = 1;\n",
	"test/widget_test.dart":                       "void main() {}\n",
	"test/widget_test.mocks.dart":                 "class MockClient {}\n",
	"build/app.dart":                              "void main() {}\n",
	"android/app/src/main/kotlin/MainActivity.kt": "class MainActivity\n",
	"linux/held.dart":                             "void held() {}\n",
	"web/held.ts":                                 "export const held = 1;\n",
	"pkg/pubspec.yaml":                            "name: pkg\ndependencies:\n  path: ^1.9.0\n",
	"pkg/l10n.yaml":                               "output-dir: lib/i18n\noutput-localization-file: strings.dart\n",
	"pkg/lib/i18n/strings_es.dart":                "class StringsEs {}\n",
	"pkg/lib/l10n/app_localizations.dart":         "abstract class AppLocalizations {}\n",
	"pkg/linux/held.dart":                         "void held() {}\n",
	"plain/web/held.dart":                         "void held() {}\n",
}

func skipsFlutterOutput(t *testing.T, open func(t *testing.T, f TreeFixture) (port.Tree, string)) {
	tree, dir := open(t, TreeFixture{Files: flutterApp})
	discovers(t, tree, []string{"."},
		absolute(dir, "lib/l10n/strings.dart", "lib/late.dart", "lib/main.dart", "pkg/lib/l10n/app_localizations.dart", "pkg/linux/held.dart", "plain/web/held.dart"),
		absolute(dir, "test/widget_test.dart"))
	// An l10n.yaml above the root still names what gen-l10n wrote, and a
	// platform folder is walked when it is the root.
	discovers(t, tree, []string{"lib"}, absolute(dir, "lib/l10n/strings.dart", "lib/late.dart", "lib/main.dart"), nil)
	discovers(t, tree, []string{"android", "web"}, absolute(dir, "android/app/src/main/kotlin/MainActivity.kt", "web/held.ts"), nil)
	// A file is taken when named, generated or not.
	discovers(t, tree, []string{"lib/user.g.dart", "lib/model.dart", "lib/l10n/app_localizations_en.dart", "test/widget_test.mocks.dart", "linux/held.dart"},
		absolute(dir, "lib/l10n/app_localizations_en.dart", "lib/model.dart", "lib/user.g.dart", "linux/held.dart"), absolute(dir, "test/widget_test.mocks.dart"))
}

func refusesAnAbsentRoot(t *testing.T, open func(t *testing.T, f TreeFixture) (port.Tree, string)) {
	tree, dir := open(t, TreeFixture{Files: project})
	for _, roots := range [][]string{{"absent"}, {".", "absent.go"}, {filepath.Join(dir, "src", "absent")}} {
		sources, tests, failure := tree.Discover(t.Context(), roots)
		var unwalkable *port.Unwalkable
		if !errors.As(failure, &unwalkable) || sources != nil || tests != nil {
			t.Errorf("Discover(%q) = %q, %q, %T %v; want Unwalkable and nothing", roots, sources, tests, failure, failure)
		}
	}
}

func existsWhereSomethingIs(t *testing.T, open func(t *testing.T, f TreeFixture) (port.Tree, string)) {
	tree, dir := open(t, TreeFixture{Files: project})
	for path, want := range map[string]bool{
		".": true, dir: true, "src": true, "src/": true, filepath.Join(dir, "src"): true, "src/a.go": true,
		"src/a_test.go": true, "README.md": true, "docs": true, "web/types.d.ts": true,
		"absent": false, "src/absent.go": false, "src/a.go/x": false, filepath.Join(dir, "absent"): false,
	} {
		if got := tree.Exists(t.Context(), path); got != want {
			t.Errorf("Exists(%q) = %t, want %t", path, got, want)
		}
	}
}

func readsWhatAFileHolds(t *testing.T, open func(t *testing.T, f TreeFixture) (port.Tree, string)) {
	tree, dir := open(t, TreeFixture{Files: project})
	sources, tests, failure := tree.Discover(t.Context(), []string{"."})
	if failure != nil {
		t.Fatal(failure)
	}
	for _, path := range append(sources, tests...) {
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			t.Fatal(err)
		}
		if text, failure := tree.Read(t.Context(), path); failure != nil || string(text) != project[filepath.ToSlash(rel)] {
			t.Errorf("Read(%q) = %q, %v; want what it holds", path, text, failure)
		}
	}
	if text, failure := tree.Read(t.Context(), "README.md"); failure != nil || string(text) != project["README.md"] {
		t.Errorf("Read(README.md) = %q, %v; want what it holds", text, failure)
	}
	for _, path := range []string{"absent.go", filepath.Join(dir, "src", "absent.go"), "src"} {
		text, failure := tree.Read(t.Context(), path)
		var unreadable *port.Unreadable
		if !errors.As(failure, &unreadable) || unreadable.Path != path || text != nil {
			t.Errorf("Read(%q) = %q, %T %v; want Unreadable naming it", path, text, failure, failure)
		}
	}
}

func changedWithoutGitIsNoGit(t *testing.T, open func(t *testing.T, f TreeFixture) (port.Tree, string)) {
	tree, _ := open(t, TreeFixture{Files: project})
	changed, failure := tree.Changed(t.Context())
	var noGit *port.NoGit
	if !errors.As(failure, &noGit) || changed != nil {
		t.Errorf("Changed() = %q, %T %v; want NoGit and nothing", changed, failure, failure)
	}
}

func changedListsWhatGitReports(t *testing.T, open func(t *testing.T, f TreeFixture) (port.Tree, string)) {
	unchanged, _ := open(t, TreeFixture{Files: project, Git: true})
	if changed, failure := unchanged.Changed(t.Context()); failure != nil || len(changed) != 0 {
		t.Errorf("Changed() of a clean repository = %q, %v; want nothing", changed, failure)
	}
	files := map[string]string{"notes.txt": "notes\n", "vendor/v.go": "package v\n", "src/new_test.go": "package src\n", "web/new.ts": "export const n = 1;\n"}
	for name, text := range project {
		files[name] = text
	}
	tree, _ := open(t, TreeFixture{Files: files, Git: true, Changed: []string{"src/a.go", "src/new_test.go", "README.md", "notes.txt", "vendor/v.go", "web/new.ts"}})
	changed, failure := tree.Changed(t.Context())
	if failure != nil {
		t.Fatal(failure)
	}
	want := []string{"src/a.go", "src/new_test.go", "vendor/v.go", "web/new.ts"}
	if got := slices.Sorted(slices.Values(changed)); !slices.Equal(got, want) {
		t.Errorf("Changed() = %q, want the supported changed files %q", changed, want)
	}
	changed[0] = "other"
	if again, _ := tree.Changed(t.Context()); slices.Contains(again, "other") {
		t.Errorf("Changed() gave its own list: %q after the caller wrote to it", again)
	}
}

func aDoneContextIsCancelled(t *testing.T, open func(t *testing.T, f TreeFixture) (port.Tree, string)) {
	tree, dir := open(t, TreeFixture{Files: project})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var cancelled *port.Cancelled
	if sources, tests, failure := tree.Discover(ctx, []string{"."}); !errors.As(failure, &cancelled) || sources != nil || tests != nil {
		t.Errorf("Discover() = %q, %q, %T; want Cancelled and nothing", sources, tests, failure)
	}
	if changed, failure := tree.Changed(ctx); !errors.As(failure, &cancelled) || changed != nil {
		t.Errorf("Changed() = %q, %T; want Cancelled and nothing", changed, failure)
	}
	if text, failure := tree.Read(ctx, filepath.Join(dir, "src", "a.go")); !errors.As(failure, &cancelled) || text != nil {
		t.Errorf("Read() = %q, %T; want Cancelled and nothing", text, failure)
	}
}

// discovers checks that Discover(roots) gives sources and tests.
func discovers(t *testing.T, tree port.Tree, roots, sources, tests []string) {
	t.Helper()
	gotSources, gotTests, failure := tree.Discover(t.Context(), roots)
	if failure != nil || !slices.Equal(gotSources, sources) || !slices.Equal(gotTests, tests) {
		t.Errorf("Discover(%q) = %q, %q, %v; want %q, %q", roots, gotSources, gotTests, failure, sources, tests)
	}
}

// absolute is each of rels, slash-separated from dir, made absolute, and
// sorted as Discover sorts them.
func absolute(dir string, rels ...string) []string {
	var paths []string
	for _, rel := range rels {
		paths = append(paths, filepath.Join(dir, filepath.FromSlash(rel)))
	}
	slices.Sort(paths)
	return paths
}
