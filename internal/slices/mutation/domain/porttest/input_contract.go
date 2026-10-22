package porttest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"maps"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// InputBindings is one contract fixture's actual bound observations. Factories
// install the same source/import/environment facts on real and virtual projects.
type InputBindings struct {
	Toolchain    mutants.Toolchain
	Fingerprints mutants.Fingerprints
	Reach        mutants.Reach
	Root         string
}
type InputFactory func(t *testing.T, files map[string]string, imports map[string][]string, env map[string]string) InputBindings

func contractHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// FingerprintsContract covers independently observed bytes, scope and runtime
// inputs; it never accepts a canned hash map or stored snapshot as current facts.
func FingerprintsContract(t *testing.T, makeInput InputFactory) {
	t.Helper()
	t.Run("tests support and external relative glob", func(t *testing.T) {
		b := makeInput(t, map[string]string{"go.mod": "module board\n", "a_test.go": "package board\n", "features/a.feature": "actual feature\n", "../outside.txt": "outside support\n", "../unrelated.txt": "unrelated\n"}, nil, nil)
		path := filepath.Join(b.Root, "a_test.go")
		tests, failure := b.Fingerprints.Tests(t.Context(), b.Root, []string{path})
		if failure != nil || !reflect.DeepEqual(tests, map[string]string{"a_test.go": contractHash("package board\n")}) {
			t.Fatalf("actual test hash: %v %v", tests, failure)
		}
		support, failure := b.Fingerprints.Support(t.Context(), b.Root, []string{"features/*.feature", "../outside.txt"})
		want := map[string]string{"features/a.feature": contractHash("actual feature\n"), "../outside.txt": contractHash("outside support\n")}
		if failure != nil || !reflect.DeepEqual(support, want) {
			t.Fatalf("actual support glob/key: %v %v want %v", support, failure, want)
		}
		if _, failure := b.Fingerprints.Tests(t.Context(), b.Root, []string{filepath.Join(b.Root, "missing_test.go")}); failure == nil {
			t.Fatal("missing test read became empty successful hashes")
		}
		if _, failure := b.Fingerprints.Support(t.Context(), b.Root, []string{"["}); failure == nil {
			t.Fatal("malformed glob became empty success")
		}
	})
	t.Run("Go provenance and nested module boundary", func(t *testing.T) {
		source := "package board\nfunc Value() int { return 1 }\n"
		// Outside a repository the files are those go's directory rules
		// take from the disk: never testdata, nor a directory or file whose
		// name starts with _ or ., nor a nested module.
		files := map[string]string{"go.mod": "module board\n", "a.go": source, "a_test.go": "package board\n", "nested/go.mod": "module other\n", "nested/ignored.go": "package other\n", "ignored.txt": "not an input\n",
			"testdata/fixture.go": "package fixture\n", "_scratch/x.go": "package x\n", ".hidden/x.go": "package x\n", "_x.go": "package board\n", ".x.go": "package board\n"}
		b := makeInput(t, files, nil, nil)
		inputs, failure := b.Fingerprints.Coverage(t.Context(), filepath.Join(b.Root, "a.go"), b.Root, "actual producer", map[string]string{"features/a.feature": "provided-support-hash"})
		if failure != nil {
			t.Fatal(failure)
		}
		wantKeys := []string{"@go-env", "@producer", "a.go", "a_test.go", "go.mod", "itos-cc.yaml", "support:features/a.feature"}
		if !reflect.DeepEqual(slices.Sorted(maps.Keys(inputs)), wantKeys) {
			t.Fatalf("Go provenance population: %v", inputs)
		}
		if inputs["a.go"] != contractHash(source) || inputs["@producer"] != contractHash("actual producer") || inputs["itos-cc.yaml"] != "absent" || inputs["@go-env"] == "" {
			t.Fatalf("Go facts: %v", inputs)
		}

		other := makeInput(t, files, nil, map[string]string{"GOFLAGS": "-tags=actual-control"})
		changed, failure := other.Fingerprints.Coverage(t.Context(), filepath.Join(other.Root, "a.go"), other.Root, "actual producer", map[string]string{"features/a.feature": "provided-support-hash"})
		if failure != nil || changed["@go-env"] == inputs["@go-env"] || changed["a.go"] != inputs["a.go"] {
			t.Fatalf("actual GOFLAGS fingerprint: %v -> %v (%v)", inputs, changed, failure)
		}
		// The environment is what go env resolves, so how the same Go is
		// chosen, or a default given explicitly, is the same environment.
		for _, env := range []map[string]string{{"GOTOOLCHAIN": "local"}, {"GOOS": runtime.GOOS}} {
			same := makeInput(t, files, nil, env)
			resolved, failure := same.Fingerprints.Coverage(t.Context(), filepath.Join(same.Root, "a.go"), same.Root, "actual producer", map[string]string{"features/a.feature": "provided-support-hash"})
			if failure != nil || resolved["@go-env"] != inputs["@go-env"] {
				t.Fatalf("resolved Go environment with %v: %v -> %v (%v)", env, inputs, resolved, failure)
			}
		}
		suite, failure := b.Fingerprints.Suite(t.Context(), filepath.Join(b.Root, "a.go"), b.Root)
		if failure != nil || !reflect.DeepEqual(suite, map[string]string{"a_test.go": contractHash(files["a_test.go"])}) {
			t.Fatalf("Go suite: %v %v", suite, failure)
		}
	})
	t.Run("non-Go suite uses actual discovery policy", func(t *testing.T) {
		files := map[string]string{"pyproject.toml": "[project]\n", "a.py": "def a(): return 1\n", "fixtures/test_a.py": "actual fixture test\n", "pkg/build/test_b.py": "ordinary nested build directory\n", "build/test_output.py": "real output directory\n", "nested/pyproject.toml": "[project]\n", "nested/test_other.py": "other suite\n"}
		b := makeInput(t, files, nil, nil)
		got, failure := b.Fingerprints.Suite(t.Context(), filepath.Join(b.Root, "a.py"), b.Root)
		want := map[string]string{"fixtures/test_a.py": contractHash(files["fixtures/test_a.py"]), "pkg/build/test_b.py": contractHash(files["pkg/build/test_b.py"])}
		if failure != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("discovered suite: %v %v want %v", got, failure, want)
		}
	})
	t.Run("language inventories preserve source and environment facts", func(t *testing.T) {
		for _, c := range []struct{ language, source, marker, key string }{
			{"python", "a.py", "pyproject.toml", "@python-env"}, {"typescript", "a.ts", "package.json", "@node-env"}, {"kotlin", "src/main/kotlin/A.kt", "build.gradle.kts", "@jvm-env"},
			{"dart", "lib/a.dart", "pubspec.yaml", "@flutter-env"},
		} {
			t.Run(c.language, func(t *testing.T) {
				text := "actual source bytes\n"
				files := map[string]string{c.source: text, c.marker: "", "ignored.txt": "not measured\n"}
				if c.language == "typescript" {
					files[c.marker] = "{}"
				}
				b := makeInput(t, files, nil, map[string]string{"PYTEST_DISABLE_PLUGIN_AUTOLOAD": "1", "NODE_ENV": "fixture", "JAVA_HOME": "fixture-jvm", "FLUTTER_ROOT": "fixture-flutter"})
				got, failure := b.Fingerprints.Coverage(t.Context(), filepath.Join(b.Root, filepath.FromSlash(c.source)), b.Root, "producer", nil)
				if failure != nil || got[c.source] != contractHash(text) || got["@producer"] != contractHash("producer") || got[c.key] == "" || got["ignored.txt"] != "" {
					t.Fatalf("actual %s inputs: %v %v", c.language, got, failure)
				}
				changedEnv := map[string]string{"PYTEST_DISABLE_PLUGIN_AUTOLOAD": "0", "NODE_ENV": "different", "JAVA_HOME": "different-jvm", "FLUTTER_ROOT": "different-flutter"}
				other := makeInput(t, files, nil, changedEnv)
				changed, failure := other.Fingerprints.Coverage(t.Context(), filepath.Join(other.Root, filepath.FromSlash(c.source)), other.Root, "producer", nil)
				if failure != nil || changed[c.key] == got[c.key] || changed[c.source] != got[c.source] || changed["@producer"] != got["@producer"] {
					t.Fatalf("%s environment must change its actual fingerprint alone: %v -> %v (%v)", c.language, got, changed, failure)
				}

			})
		}
	})
	t.Run("GOWORK override and lexical replacement refusal", func(t *testing.T) {
		files := map[string]string{"go.mod": "module board\n", "a.go": "package board\n", "go.work": "go 1.27\nuse .\n"}
		b := makeInput(t, files, nil, nil)
		reason, failure := b.Fingerprints.Unsupported(t.Context(), "go", filepath.Join(b.Root, "a.go"), b.Root)
		if failure != nil || reason != filepath.Join(b.Root, "go.work") {
			t.Fatalf("workspace: %q %v", reason, failure)
		}
		off := makeInput(t, files, nil, map[string]string{"GOWORK": "off"})
		reason, failure = off.Fingerprints.Unsupported(t.Context(), "go", filepath.Join(off.Root, "a.go"), off.Root)
		if failure != nil || reason != "" {
			t.Fatalf("actual GOWORK=off: %q %v", reason, failure)
		}
		files["go.mod"] = "module board\nreplace example.com/other => ../outside\n"
		replaced := makeInput(t, files, nil, map[string]string{"GOWORK": "off"})
		reason, failure = replaced.Fingerprints.Unsupported(t.Context(), "go", filepath.Join(replaced.Root, "a.go"), replaced.Root)
		if failure != nil || reason != "../outside" {
			t.Fatalf("lexical replacement: %q %v", reason, failure)
		}
	})

	t.Run("line language lexical scope admission", func(t *testing.T) {
		for _, c := range []struct{ language, source, manifest, content, token string }{
			{"typescript", "a.ts", "package.json", `{"workspaces":["packages/*"]}`, "workspace"},
			{"typescript", "a.ts", "package.json", `{"dependencies":{"other":"file:../outside"}}`, "local dependency"},
			{"python", "a.py", "pyproject.toml", "[tool.uv.sources]\nother = {path = \"../outside\"}\n", "path dependency"},
			{"kotlin", "src/main/kotlin/A.kt", "settings.gradle.kts", "includeBuild(\"../outside\")\n", "includeBuild"},
			{"dart", "lib/a.dart", "pubspec.yaml", "name: a\ndependencies:\n  other:\n    path: ../outside\n", "path dependency"},
		} {
			t.Run(c.language+" "+c.token, func(t *testing.T) {
				files := map[string]string{c.source: "actual source\n", c.manifest: c.content, "../outside/never-read.txt": "owned external marker"}
				if c.language == "kotlin" {
					files["build.gradle.kts"] = ""
				}
				b := makeInput(t, files, nil, nil)
				reason, failure := b.Fingerprints.Unsupported(t.Context(), c.language, filepath.Join(b.Root, filepath.FromSlash(c.source)), b.Root)
				if failure != nil || !strings.Contains(reason, c.token) {
					t.Fatalf("actual lexical refusal: %q %v", reason, failure)
				}
				if c.language == "typescript" {
					files[c.manifest] = "{}"
				} else {
					files[c.manifest] = ""
				}
				plain := makeInput(t, files, nil, nil)
				reason, failure = plain.Fingerprints.Unsupported(t.Context(), c.language, filepath.Join(plain.Root, filepath.FromSlash(c.source)), plain.Root)
				if failure != nil || reason != "" {
					t.Fatalf("supported plain control: %q %v", reason, failure)
				}
			})
		}
	})
	// slice-18: a pnpm-workspace.yaml is a workspace only when its packages
	// list members; a member is refused only for a local dependency.
	t.Run("pnpm settings and workspace members", func(t *testing.T) {
		workspace := func(settings, deps string) map[string]string {
			return map[string]string{"package.json": `{"name":"repro"}`, "pnpm-workspace.yaml": settings,
				"packages/label/package.json": `{"name":"label","dependencies":{` + deps + `}}`, "packages/label/a.ts": "actual source\n",
				"packages/other/package.json": `{"name":"other"}`, "tools/deep/x/package.json": `{"name":"deep"}`, "../outside/never-read.txt": "owned external marker"}
		}
		single := func(settings string, more map[string]string) map[string]string {
			files := map[string]string{"package.json": `{"name":"single"}`, "pnpm-workspace.yaml": settings, "a.ts": "actual source\n", "../outside/never-read.txt": "owned external marker"}
			maps.Copy(files, more)
			return files
		}
		const members = "packages:\n  - packages/*\n"
		for _, c := range []struct {
			name, source string
			files        map[string]string
			token        string // a word of the refusal, "" for none
		}{
			{"settings alone", "a.ts", single("minimumReleaseAge: 1440\n", nil), ""},
			{"null packages", "a.ts", single("packages:\nminimumReleaseAge: 1440\n", nil), ""},
			{"empty packages", "a.ts", single("packages: []\n", nil), ""},
			{"settings above the package", "app/a.ts", map[string]string{"pnpm-workspace.yaml": "minimumReleaseAge: 1440\n", "app/package.json": "{}", "app/a.ts": "actual source\n"}, ""},
			{"packages that are no list", "a.ts", single("packages: packages/*\n", nil), "does not parse"},
			{"the workspace's own root package", "a.ts", single(members, nil), "pnpm workspace root"},
			{"a member with registry and alias dependencies", "packages/label/a.ts", workspace(members, `"left-pad":"^1.3.0","o":"npm:other@1"`), ""},
			{"a member naming a member by a plain version", "packages/label/a.ts", workspace(members, `"other":"^1.0.0"`), `other "^1.0.0"`},
			{"a member naming a member through a catalog", "packages/label/a.ts", workspace(members, `"other":"catalog:"`), `other "catalog:"`},
			{"a member naming a member a ** pattern lists", "packages/label/a.ts", workspace(members+"  - tools/**\n", `"deep":"1"`), `deep "1"`},
			{"a member with a workspace: dependency", "packages/label/a.ts", workspace(members, `"other":"workspace:*"`), "workspace:*"},
			{"an override outside the package root", "a.ts", single("overrides:\n  lib: link:../outside\n", nil), "override lib"},
			{"a workspace override into a sibling", "packages/label/a.ts", workspace(members+"overrides:\n  lib: link:./packages/other\n", ""), "override lib"},
			{"an override inside the package root", "a.ts", single("overrides:\n  lib: link:./vendor/lib\n", map[string]string{"vendor/lib/index.js": ""}), ""},
		} {
			t.Run(c.name, func(t *testing.T) {
				b := makeInput(t, c.files, nil, nil)
				reason, failure := b.Fingerprints.Unsupported(t.Context(), "typescript", filepath.Join(b.Root, filepath.FromSlash(c.source)), b.Root)
				switch {
				case failure != nil:
					t.Fatal(failure)
				case c.token == "" && reason != "":
					t.Fatalf("refused %q, want it admitted", reason)
				case c.token != "" && !strings.Contains(reason, c.token):
					t.Fatalf("refusal %q, want one naming %q", reason, c.token)
				}
			})
		}
	})
	t.Run("a pnpm member's inputs hold what pnpm reads of its workspace root", func(t *testing.T) {
		files := map[string]string{"package.json": `{"name":"repro","pnpm":{"patchedDependencies":{"b@1.0.0":"patches/b.patch"}}}`,
			"pnpm-workspace.yaml": "packages:\n  - packages/*\npatchedDependencies:\n  a@1.0.0: patches/a.patch\n",
			"pnpm-lock.yaml":      "lockfileVersion: '9.0'\n", "tsconfig.base.json": "{}", ".npmrc": "auto-install-peers=true\n", ".pnpmfile.cjs": "module.exports = {};\n",
			"patches/a.patch": "a patch\n", "patches/b.patch": "b patch\n", "patches/unnamed.patch": "not named\n", "README.md": "not read\n", "vitest.config.ts": "export default {};\n",
			"packages/label/package.json": `{"name":"label"}`, "packages/label/src/a.ts": "actual source\n",
			"packages/other/package.json": `{"name":"other"}`, "packages/other/src/o.ts": "a sibling\n"}
		b := makeInput(t, files, nil, nil)
		got, failure := b.Fingerprints.Coverage(t.Context(), filepath.Join(b.Root, "packages", "label", "src", "a.ts"), b.Root, "producer", nil)
		want := []string{".npmrc", ".pnpmfile.cjs", "@node-env", "@producer", "itos-cc.yaml", "package.json", "packages/label/package.json", "packages/label/src/a.ts",
			"patches/a.patch", "patches/b.patch", "pnpm-lock.yaml", "pnpm-workspace.yaml", "tsconfig.base.json"}
		if failure != nil || !slices.Equal(slices.Sorted(maps.Keys(got)), want) {
			t.Fatalf("pnpm member inputs: %v %v, want %v", slices.Sorted(maps.Keys(got)), failure, want)
		}
		if got[".npmrc"] != contractHash(files[".npmrc"]) || got["patches/a.patch"] != contractHash(files["patches/a.patch"]) {
			t.Fatalf("pnpm member input hashes: %v", got)
		}
	})
	t.Run("caller cancellation precedes reads", func(t *testing.T) {
		b := makeInput(t, map[string]string{}, nil, nil)
		ctx, cancel := context.WithCancelCause(t.Context())
		cause := errors.New("actual caller stop")
		cancel(cause)
		_, failure := b.Fingerprints.Tests(ctx, b.Root, []string{filepath.Join(b.Root, "absent")})
		var stopped mutants.InputCancelled
		if !errors.As(failure, &stopped) || !errors.Is(failure, cause) {
			t.Fatalf("cancel cause: %v", failure)
		}
	})
}

// ToolchainContract asserts file-derived choices and fallback facts without
// needing a host language runtime. Physical interpreter execution has its own
// real adapter controls; the fake receives independent probe declarations.
func ToolchainContract(t *testing.T, makeInput InputFactory) {
	t.Helper()
	t.Run("installed node entry versus declared dependency", func(t *testing.T) {
		files := map[string]string{"package.json": `{"devDependencies":{"vitest":"*"},"packageManager":"pnpm@9"}`, "a.ts": "export const a=1;"}
		b := makeInput(t, files, nil, nil)
		c, failure := mutants.ObserveCommand(t.Context(), b.Toolchain, mutants.CommandRequest{Path: filepath.Join(b.Root, "a.ts")})
		if failure != nil || c.String() != "pnpm run test" || !c.Suite {
			t.Fatalf("uninstalled declared runner: %+v %v", c, failure)
		}
		// Both native names are facts, so this contract is portable; Stat alone
		// admits the installed entry without an executable-mode requirement.
		files["node_modules/.bin/vitest"], files["node_modules/.bin/vitest.cmd"] = "", ""
		installed := makeInput(t, files, nil, nil)
		c, failure = mutants.ObserveCommand(t.Context(), installed.Toolchain, mutants.CommandRequest{Path: filepath.Join(installed.Root, "a.ts")})
		if failure != nil || c.Suite || len(c.Args) != 5 || c.Args[1] != "related" || c.Args[4] != "a.ts" || !strings.HasPrefix(c.Args[0], installed.Root) {
			t.Fatalf("installed actual entry: %+v %v", c, failure)
		}
	})
	t.Run("Dart runs the test files under test/ that reach it, offline", func(t *testing.T) {
		b := makeInput(t, map[string]string{"pubspec.yaml": "name: own\ndependencies:\n  flutter:\n    sdk: flutter\n", "lib/a.dart": "bool a() => true;\n",
			"test/a_test.dart": "", "test/support.dart": "", "integration_test/app_test.dart": ""}, nil, nil)
		path := filepath.Join(b.Root, "lib", "a.dart")
		var tests []string
		for _, test := range []string{"test/a_test.dart", "test/support.dart", "integration_test/app_test.dart"} {
			tests = append(tests, filepath.Join(b.Root, filepath.FromSlash(test)))
		}
		quiet := []string{"--suppress-analytics", "--no-version-check"}
		flutter := func(words []string) bool {
			return len(words) > 0 && filepath.Base(words[0]) == "flutter" && reflect.DeepEqual(words[1:3], quiet)
		}
		c, failure := mutants.ObserveCommand(t.Context(), b.Toolchain, mutants.CommandRequest{Path: path, Tests: tests})
		if failure != nil || c.Root != b.Root || c.Dir != b.Root || !flutter(c.Args) || !reflect.DeepEqual(c.Args[3:], []string{"test", "--no-pub", "test/a_test.dart"}) ||
			!flutter(c.Ready) || !reflect.DeepEqual(c.Ready[3:], []string{"pub", "get", "--offline"}) {
			t.Fatalf("reached Dart source: %+v %v", c, failure)
		}
		c, failure = mutants.ObserveCommand(t.Context(), b.Toolchain, mutants.CommandRequest{Path: path, All: true})
		if failure != nil || !flutter(c.Args) || !reflect.DeepEqual(c.Args[3:], []string{"test", "--no-pub"}) {
			t.Fatalf("every Dart test: %+v %v", c, failure)
		}
		c, failure = mutants.ObserveCommand(t.Context(), b.Toolchain, mutants.CommandRequest{Path: path, Tests: tests[1:]})
		if failure != nil || !c.RunsNothing() || c.Dir != b.Root {
			t.Fatalf("unreached Dart source: %+v %v", c, failure)
		}
		c, failure = mutants.ObserveCommand(t.Context(), b.Toolchain, mutants.CommandRequest{Path: path, Tests: tests, Shell: "./run.sh"})
		if failure != nil || c.Shell != "./run.sh" || c.Args != nil || c.Ready != nil {
			t.Fatalf("a Dart --test-command is run as given, its copy readied by nothing: %+v %v", c, failure)
		}
	})
	t.Run("the Go platform is go env's, the run's GOOS and GOARCH first", func(t *testing.T) {
		files := map[string]string{"go.mod": "module board\n", "a.go": "package board\n"}
		b := makeInput(t, files, nil, nil)
		p, failure := b.Toolchain.GoPlatform(t.Context(), b.Root)
		if failure != nil || p.OS == "" || p.Arch == "" || p.String() != p.OS+"/"+p.Arch {
			t.Fatalf("Go platform: %+v %v", p, failure)
		}
		other := "windows"
		if p.OS == other {
			other = "linux"
		}
		set := makeInput(t, files, nil, map[string]string{"GOOS": other, "GOARCH": "arm64"})
		p, failure = set.Toolchain.GoPlatform(t.Context(), set.Root)
		if failure != nil || p != (mutants.Platform{OS: other, Arch: "arm64"}) {
			t.Fatalf("Go platform with GOOS=%s GOARCH=arm64: %+v %v", other, p, failure)
		}
	})
	t.Run("Python shell and unreached source avoid unused probe", func(t *testing.T) {
		b := makeInput(t, map[string]string{"pyproject.toml": "", "src/a.py": "def a(): return 1\n", ".venv/bin/python": "not executable"}, nil, nil)
		path := filepath.Join(b.Root, "src", "a.py")
		c, failure := mutants.ObserveCommand(t.Context(), b.Toolchain, mutants.CommandRequest{Path: path, Shell: "./run.sh"})
		if failure != nil || c.Shell != "./run.sh" || c.Root != b.Root || c.PathEnv != "PYTHONPATH" || !reflect.DeepEqual(c.PathDirs, []string{".", "src"}) {
			t.Fatalf("shell override: %+v %v", c, failure)
		}
		c, failure = mutants.ObserveCommand(t.Context(), b.Toolchain, mutants.CommandRequest{Path: path})
		if failure != nil || !c.RunsNothing() || c.Dir != b.Root {
			t.Fatalf("unreached Python source: %+v %v", c, failure)
		}
	})
}

// ReachContract exercises actual declaration closure, not desired reach lists.
func ReachContract(t *testing.T, makeInput InputFactory) {
	t.Helper()
	for _, c := range []struct {
		name           string
		files          map[string]string
		edges          map[string][]string
		sources, tests []string
		want           map[string][]string
	}{
		{name: "Go package reach is one hop", files: map[string]string{
			"go.mod": "module example.com/m\n", "board/board.go": "package board\ntype Board struct{}\n", "board/board_test.go": "package board\n",
			"app/app.go": "package app\nimport \"example.com/m/board\"\nvar B board.Board\n", "app/app_test.go": "package app\n",
			"cmd/cmd.go": "package cmd\nimport \"example.com/m/app\"\nvar A = app.B\n", "cmd/cmd_test.go": "package cmd\n"},
			edges:   map[string][]string{"board/board.go": {}, "board/board_test.go": {}, "app/app.go": {"board/board.go"}, "app/app_test.go": {}, "cmd/cmd.go": {"app/app.go"}, "cmd/cmd_test.go": {}},
			sources: []string{"board/board.go", "app/app.go", "cmd/cmd.go"}, tests: []string{"board/board_test.go", "app/app_test.go", "cmd/cmd_test.go"},
			want: map[string][]string{"board/board.go": {"app/app_test.go", "board/board_test.go"}, "app/app.go": {"app/app_test.go", "cmd/cmd_test.go"}, "cmd/cmd.go": {"cmd/cmd_test.go"}}},
		{name: "Python conftest applies only below its build root", files: map[string]string{
			"pyproject.toml": "", "a.py": "def a(): return 1\n", "b.py": "def b(): return 2\n", "tests/conftest.py": "from a import a\n", "tests/test_one.py": "def test_one(): pass\n", "other/test_other.py": "from b import b\n"},
			edges:   map[string][]string{"a.py": {}, "b.py": {}, "tests/conftest.py": {"a.py"}, "tests/test_one.py": {}, "other/test_other.py": {"b.py"}},
			sources: []string{"a.py", "b.py"}, tests: []string{"tests/conftest.py", "tests/test_one.py", "other/test_other.py"},
			want: map[string][]string{"a.py": {"tests/conftest.py", "tests/test_one.py"}, "b.py": {"other/test_other.py"}}},
		{name: "Kotlin same-package reference reaches imported types", files: map[string]string{
			"settings.gradle.kts": "", "build.gradle.kts": "", "src/main/kotlin/com/acme/Money.kt": "package com.acme\nimport com.acme.cur.Currency\nclass Money(val c: Currency)\n", "src/main/kotlin/com/acme/cur/Currency.kt": "package com.acme.cur\nclass Currency\n", "src/test/kotlin/com/acme/MoneyTest.kt": "package com.acme\nclass MoneyTest { fun test() = Money(Currency()) }\n", "src/test/kotlin/com/acme/UnrelatedTest.kt": "package com.acme\nclass UnrelatedTest { fun test() = 1 }\n"},
			edges:   map[string][]string{"src/main/kotlin/com/acme/Money.kt": {"src/main/kotlin/com/acme/cur/Currency.kt"}, "src/main/kotlin/com/acme/cur/Currency.kt": {}, "src/test/kotlin/com/acme/MoneyTest.kt": {"src/main/kotlin/com/acme/Money.kt"}, "src/test/kotlin/com/acme/UnrelatedTest.kt": {}},
			sources: []string{"src/main/kotlin/com/acme/Money.kt", "src/main/kotlin/com/acme/cur/Currency.kt"}, tests: []string{"src/test/kotlin/com/acme/MoneyTest.kt", "src/test/kotlin/com/acme/UnrelatedTest.kt"},
			want: map[string][]string{"src/main/kotlin/com/acme/Money.kt": {"src/test/kotlin/com/acme/MoneyTest.kt"}, "src/main/kotlin/com/acme/cur/Currency.kt": {"src/test/kotlin/com/acme/MoneyTest.kt"}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			b := makeInput(t, c.files, c.edges, nil)
			absolute := func(names []string) []string {
				out := []string{}
				for _, name := range names {
					out = append(out, filepath.Join(b.Root, filepath.FromSlash(name)))
				}
				slices.Sort(out)
				return out
			}
			got, failure := b.Reach.TestsImporting(t.Context(), b.Root, absolute(c.sources), absolute(c.tests))
			want := map[string][]string{}
			for path, names := range c.want {
				want[filepath.Join(b.Root, filepath.FromSlash(path))] = absolute(names)
			}
			if failure != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("actual resolved reach: %v %v want %v", got, failure, want)
			}
		})
	}

	t.Run("transitive sources and test support exclude unrelated tests", func(t *testing.T) {
		files := map[string]string{"package.json": `{"name":"board","type":"module"}`, "a.ts": "export const a=1;\n", "middle.ts": "import {a} from './a'; export const m=a;\n", "tests/support.ts": "import {m} from '../middle'; export const support=m;\n", "tests/board.test.ts": "import {support} from './support'; export const test=()=>support;\n", "tests/unrelated.test.ts": "export const test=()=>1;\n"}
		edges := map[string][]string{"a.ts": {}, "middle.ts": {"a.ts"}, "tests/support.ts": {"middle.ts"}, "tests/board.test.ts": {"tests/support.ts"}, "tests/unrelated.test.ts": {}}
		b := makeInput(t, files, edges, nil)
		at := func(p string) string { return filepath.Join(b.Root, filepath.FromSlash(p)) }
		got, failure := b.Reach.TestsImporting(t.Context(), b.Root, []string{at("a.ts"), at("middle.ts")}, []string{at("tests/support.ts"), at("tests/board.test.ts"), at("tests/unrelated.test.ts")})
		want := []string{at("tests/board.test.ts"), at("tests/support.ts")}
		if failure != nil || !reflect.DeepEqual(got[at("a.ts")], want) || !reflect.DeepEqual(got[at("middle.ts")], want) || len(got) != 2 {
			t.Fatalf("transitive importing tests: %v %v want %v", got, failure, want)
		}
	})
}
