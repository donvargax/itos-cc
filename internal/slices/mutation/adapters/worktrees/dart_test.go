package worktrees

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFlutterCopyLayout holds what a worker's copy of a Flutter package
// and of a pub workspace's member holds on every platform's go test: MUT-265,
// which runs the real flutter in them, runs in cd.yml's Linux dart job alone.
func TestFlutterCopyLayout(t *testing.T) {
	tree := t.TempDir()
	flutter := "dependencies:\n  flutter:\n    sdk: flutter\n"
	for name, text := range map[string]string{
		"pubspec.yaml":                 "name: ws\nworkspace:\n  - app\n  - pkgs/*\n",
		"pubspec.lock":                 "packages: {}\n",
		"README.md":                    "not a member\n",
		"app/pubspec.yaml":             "name: app\nresolution: workspace\n" + flutter,
		"app/lib/board.dart":           "bool low(int i) => i == 3;\n",
		"app/lib/user.g.dart":          "// GENERATED CODE - DO NOT MODIFY BY HAND\n",
		"app/android/build.gradle.kts": "\n",
		"app/ios/Podfile":              "\n",
		"app/web/index.html":           "\n",
		"app/build/test_cache/x":       "\n",
		"app/coverage/lcov.info":       "\n",
		"app/.dart_tool/pub/x":         "\n",
		"pkgs/core/pubspec.yaml":       "name: core\nresolution: workspace\n",
		"pkgs/core/lib/core.dart":      "int three() => 3;\n",
		"pkgs/core/web/kept.txt":       "a pure Dart package's web folder is its own\n",
		"other/pubspec.yaml":           "name: other\n",
	} {
		path := filepath.Join(tree, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	root, members := pubWorkspace(filepath.Join(tree, "app"))
	if root != tree || len(members) != 2 {
		t.Fatalf("app's workspace is %s with members %v", root, members)
	}
	if other, none := pubWorkspace(filepath.Join(tree, "other")); other != filepath.Join(tree, "other") || none != nil {
		t.Fatalf("a package of no workspace copies %s with members %v", other, none)
	}
	dst := filepath.Join(t.TempDir(), "copy")
	if err := copyPackages(t.Context(), root, members, dst); err != nil {
		t.Fatal(err)
	}
	for name, held := range map[string]bool{
		"pubspec.yaml": true, "pubspec.lock": true, "README.md": false, "other": false,
		"app/lib/board.dart": true, "app/lib/user.g.dart": true, "pkgs/core/lib/core.dart": true, "pkgs/core/web/kept.txt": true,
		"app/android": false, "app/ios": false, "app/web": false, "app/build": false, "app/coverage": false, "app/.dart_tool": false,
	} {
		if _, err := os.Stat(filepath.Join(dst, filepath.FromSlash(name))); (err == nil) != held {
			t.Errorf("the copy holds %s: %v, want %v", name, err == nil, held)
		}
	}
}
