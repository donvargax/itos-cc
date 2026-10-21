// Dart's reach fixtures: a Flutter package "demo" whose test reaches a
// library through each way one Dart library names another.
package features

// dartReachPubspec is a Flutter app's pubspec.yaml named name, with extra lines
// after its name.
func dartReachPubspec(name, extra string) string {
	return "name: " + name + "\n" + extra + "environment:\n  sdk: ^3.6.0\ndependencies:\n  flutter:\n    sdk: flutter\n" +
		"dev_dependencies:\n  flutter_test:\n    sdk: flutter\n"
}

// dartReachTest is a flutter test file that imports what imports says and
// expects call to be true.
func dartReachTest(imports, call string) string {
	return imports + "import 'package:flutter_test/flutter_test.dart';\n\nvoid main() {\n  test('reach', () {\n    expect(" + call + ", isTrue);\n  });\n}\n"
}

const (
	dartReachAlpha = "bool alpha(int i) {\n  return i > 5;\n}\n"
	dartReachOther = "bool other(int i) {\n  return i < 3;\n}\n"
)

// dartReachPackage is package demo at the scratch folder's root, holding lib/a.dart
// and lib/other.dart with their tests' other test, beside files.
func dartReachPackage(files map[string]string, edges map[string][]string) languageReachProject {
	p := languageReachProject{
		files: map[string]string{
			"pubspec.yaml":         dartReachPubspec("demo", ""),
			"lib/a.dart":           dartReachAlpha,
			"lib/other.dart":       dartReachOther,
			"test/other_test.dart": dartReachTest("import 'package:demo/other.dart';\n", "other(2)"),
		},
		edges:    map[string][]string{"test/other_test.dart": {"lib/other.dart"}},
		b:        "lib/a.dart",
		reaching: "test/a_test.dart",
		others:   "test/other_test.dart",
	}
	for name, text := range files {
		p.files[name] = text
	}
	for from, to := range edges {
		p.edges[from] = to
	}
	return p
}

// dartLinkProjects are MUT-263's packages, by its link column.
var dartLinkProjects = map[string]languageReachProject{
	"an import of package:demo/a.dart": dartReachPackage(
		map[string]string{"test/a_test.dart": dartReachTest("import 'package:demo/a.dart';\n", "alpha(6)")},
		map[string][]string{"test/a_test.dart": {"lib/a.dart"}}),
	"a relative import of ../lib/a.dart": dartReachPackage(
		map[string]string{"test/a_test.dart": dartReachTest("import '../lib/a.dart';\n", "alpha(6)")},
		map[string][]string{"test/a_test.dart": {"lib/a.dart"}}),
	"lib/demo.dart, which it imports and which exports a.dart": dartReachPackage(
		map[string]string{
			"lib/demo.dart":    "export 'a.dart';\n",
			"test/a_test.dart": dartReachTest("import 'package:demo/demo.dart';\n", "alpha(6)"),
		},
		map[string][]string{"lib/demo.dart": {"lib/a.dart"}, "test/a_test.dart": {"lib/demo.dart"}}),
	"lib/b.dart, which it imports and which holds part 'a.dart'": dartReachPackage(
		map[string]string{
			"lib/b.dart":       "part 'a.dart';\n\nbool beta(int i) => alpha(i);\n",
			"lib/a.dart":       "part of 'b.dart';\n\n" + dartReachAlpha,
			"test/a_test.dart": dartReachTest("import 'package:demo/b.dart';\n", "beta(6)"),
		},
		map[string][]string{"lib/b.dart": {"lib/a.dart"}, "test/a_test.dart": {"lib/b.dart"}}),
	"a conditional import of lib/b.dart, if (dart.library.io) lib/a.dart": dartReachPackage(
		map[string]string{
			"lib/b.dart":       "bool alpha(int i) => true;\n",
			"test/a_test.dart": dartReachTest("import 'package:demo/b.dart' if (dart.library.io) 'package:demo/a.dart';\n", "alpha(6)"),
		},
		map[string][]string{"test/a_test.dart": {"lib/b.dart", "lib/a.dart"}}),
	// A pub workspace: its root names its members, demo and core, each
	// resolving through it.
	`package:core/c.dart of a pub workspace sibling "core", which imports package:demo/a.dart`: {
		files: map[string]string{
			"pubspec.yaml":                       "name: workspace\nenvironment:\n  sdk: ^3.6.0\nworkspace:\n  - packages/demo\n  - packages/core\n",
			"packages/demo/pubspec.yaml":         dartReachPubspec("demo", "resolution: workspace\n"),
			"packages/demo/lib/a.dart":           dartReachAlpha,
			"packages/demo/lib/other.dart":       dartReachOther,
			"packages/demo/test/a_test.dart":     dartReachTest("import 'package:core/c.dart';\n", "gamma(6)"),
			"packages/demo/test/other_test.dart": dartReachTest("import 'package:demo/other.dart';\n", "other(2)"),
			"packages/core/pubspec.yaml":         "name: core\nresolution: workspace\nenvironment:\n  sdk: ^3.6.0\ndependencies:\n  demo: any\n",
			"packages/core/lib/c.dart":           "import 'package:demo/a.dart';\n\nbool gamma(int i) => alpha(i);\n",
		},
		edges: map[string][]string{
			"packages/core/lib/c.dart":           {"packages/demo/lib/a.dart"},
			"packages/demo/test/a_test.dart":     {"packages/core/lib/c.dart"},
			"packages/demo/test/other_test.dart": {"packages/demo/lib/other.dart"},
		},
		b: "packages/demo/lib/a.dart", reaching: "packages/demo/test/a_test.dart", others: "packages/demo/test/other_test.dart",
	},
}

// The Dart rows of MUT-215 and MUT-219.
var (
	dartReachProject = languageReachProject{
		files: map[string]string{
			"pubspec.yaml":         dartReachPubspec("demo", ""),
			"lib/b.dart":           "bool beta(int i) {\n  return i > 5;\n}\n",
			"lib/a.dart":           "import 'b.dart';\n\nbool alpha(int i) {\n  return beta(i);\n}\n",
			"test/a_test.dart":     dartReachTest("import 'package:demo/a.dart';\n", "alpha(6)"),
			"lib/other.dart":       dartReachOther,
			"test/other_test.dart": dartReachTest("import 'package:demo/other.dart';\n", "other(2)"),
		},
		edges: map[string][]string{"lib/a.dart": {"lib/b.dart"}, "test/a_test.dart": {"lib/a.dart"}, "test/other_test.dart": {"lib/other.dart"}},
		b:     "lib/b.dart", reaching: "test/a_test.dart", others: "test/other_test.dart",
	}
	dartHelperProject = dartReachPackage(
		map[string]string{
			"test/helpers.dart":    "import 'package:demo/a.dart';\n\nbool Function(int) make() => alpha;\n",
			"test/alpha_test.dart": dartReachTest("import 'helpers.dart';\n", "make()(6)"),
		},
		map[string][]string{"test/helpers.dart": {"lib/a.dart"}, "test/alpha_test.dart": {"test/helpers.dart"}}).reachedBy("test/alpha_test.dart")
	// flutter test loads the nearest flutter_test_config.dart above a test
	// file, up to its package: test/widget's, not the other test's.
	dartConfigProject = dartReachPackage(
		map[string]string{
			"test/widget/flutter_test_config.dart": "import 'dart:async';\n\nimport 'package:demo/a.dart';\n\n" +
				"Future<void> testExecutable(FutureOr<void> Function() testMain) async {\n  alpha(6);\n  await testMain();\n}\n",
			"test/widget/alpha/alpha_test.dart": dartReachTest("", "true"),
		},
		map[string][]string{"test/widget/flutter_test_config.dart": {"lib/a.dart"}}).reachedBy("test/widget/alpha/alpha_test.dart")
)

// reachedBy is p with test as the test that reaches its file.
func (p languageReachProject) reachedBy(test string) languageReachProject {
	p.reaching = test
	return p
}
