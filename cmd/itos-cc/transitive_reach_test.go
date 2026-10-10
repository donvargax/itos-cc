package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The scenario of "Rule: TypeScript, Python and Kotlin tests reach what
// they import transitively" in features/mutate.feature. Each project holds
// a test that imports module A, where A imports module B and no test imports
// B, and an unrelated test of its own module. A run with a test command that
// always passes records B's mutant survived, with the tests that reach B; no
// test runner is needed. The run records its outcome as the test command's,
// which any test change makes stale, so the snapshot is then rewritten to
// hold it as decided by B's own tests, as a run of the project's runner
// records it. The Go module is the guardrail: package cmd imports app, which
// imports board, two imports away from cmd's test.

// reachProject is a project of the scenario: its files, B's source, the test
// that reaches B through A, and the test that reaches neither.
type reachProject struct {
	files               map[string]string
	b, reaching, others string
}

var reachProjects = map[string]reachProject{
	"typescript": {
		files: map[string]string{
			"package.json":      `{"name": "m"}` + "\n",
			"src/b.ts":          "export function beta(i: number): boolean {\n  return i > 5;\n}\n",
			"src/a.ts":          "import { beta } from \"./b\";\n\nexport function alpha(i: number): boolean {\n  return beta(i);\n}\n",
			"src/a.test.ts":     "import { alpha } from \"./a\";\n\ntest(\"alpha\", () => {\n  expect(alpha(6)).toBe(true);\n});\n",
			"src/other.ts":      "export function other(i: number): boolean {\n  return i < 3;\n}\n",
			"src/other.test.ts": "import { other } from \"./other\";\n\ntest(\"other\", () => {\n  expect(other(2)).toBe(true);\n});\n",
		},
		b: "src/b.ts", reaching: "src/a.test.ts", others: "src/other.test.ts",
	},
	"python": {
		files: map[string]string{
			"pyproject.toml": "[project]\nname = \"m\"\nversion = \"0.0.0\"\n",
			"b.py":           "def beta(i):\n    return i > 5\n",
			"a.py":           "from b import beta\n\n\ndef alpha(i):\n    return beta(i)\n",
			"test_a.py":      "from a import alpha\n\n\ndef test_alpha():\n    assert alpha(6)\n",
			"other.py":       "def other(i):\n    return i < 3\n",
			"test_other.py":  "from other import other\n\n\ndef test_other():\n    assert other(2)\n",
		},
		b: "b.py", reaching: "test_a.py", others: "test_other.py",
	},
	"kotlin": {
		files: map[string]string{
			"settings.gradle.kts":              "rootProject.name = \"m\"\n",
			"build.gradle.kts":                 "plugins {\n    kotlin(\"jvm\") version \"2.4.21\"\n}\n",
			"src/main/kotlin/lib/b/Beta.kt":    "package lib.b\n\nclass Beta {\n    fun high(i: Int): Boolean {\n        return i > 5\n    }\n}\n",
			"src/main/kotlin/lib/a/Alpha.kt":   "package lib.a\n\nimport lib.b.Beta\n\nclass Alpha {\n    fun high(i: Int): Boolean = Beta().high(i)\n}\n",
			"src/test/kotlin/app/AlphaTest.kt": "package app\n\nimport lib.a.Alpha\n\nclass AlphaTest {\n    fun high() = Alpha().high(6)\n}\n",
			"src/main/kotlin/lib/c/Other.kt":   "package lib.c\n\nclass Other {\n    fun low(i: Int): Boolean {\n        return i < 3\n    }\n}\n",
			"src/test/kotlin/app/OtherTest.kt": "package app\n\nimport lib.c.Other\n\nclass OtherTest {\n    fun low() = Other().low(2)\n}\n",
		},
		b: "src/main/kotlin/lib/b/Beta.kt", reaching: "src/test/kotlin/app/AlphaTest.kt", others: "src/test/kotlin/app/OtherTest.kt",
	},
	// The guardrail: cmd's test reaches cmd and app, not board.
	"go": {
		files: map[string]string{
			"go.mod":              "module example.com/m\n\ngo 1.22\n",
			"board/board.go":      "package board\n\nfunc High(i int) bool {\n\treturn i > 5\n}\n",
			"app/app.go":          "package app\n\nimport \"example.com/m/board\"\n\nfunc High(i int) bool {\n\treturn board.High(i)\n}\n",
			"cmd/cmd.go":          "package cmd\n\nimport \"example.com/m/app\"\n\nfunc High(i int) bool {\n\treturn app.High(i)\n}\n",
			"cmd/cmd_test.go":     "package cmd\n\nimport \"testing\"\n\nfunc TestHigh(t *testing.T) {\n\tif !High(6) {\n\t\tt.Fatal(\"High\")\n\t}\n}\n",
			"other/other.go":      "package other\n\nfunc Low(i int) bool {\n\treturn i < 3\n}\n",
			"other/other_test.go": "package other\n\nimport \"testing\"\n\nfunc TestLow(t *testing.T) {\n\tif !Low(2) {\n\t\tt.Fatal(\"Low\")\n\t}\n}\n",
		},
		b: "board/board.go", reaching: "cmd/cmd_test.go", others: "other/other_test.go",
	},
}

// reachRepo writes p in a new directory, makes it the test's directory and
// records B's mutant survived by B's own tests.
func reachRepo(t *testing.T, p reachProject) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	dir := t.TempDir()
	for name, text := range p.files {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(name)), text)
	}
	useDir(t, dir)
	b := filepath.FromSlash(p.b)
	if o := mutateRun(t, "--test-command", "go version", b); o.code != 1 || !strings.Contains(o.stdout, "survived") {
		t.Fatalf("recording %s: exit %d, want 1 for its survivor\n%s%s", p.b, o.code, o.stdout, o.stderr)
	}
	// Its own tests' outcome has no scope and no whole-suite evidence; the
	// tests it rests on are the snapshot's "tests", those that reach B.
	snapshot := filepath.Join(".metrics", "mutate", b+".json")
	raw, err := os.ReadFile(inWD(t, snapshot))
	if err != nil {
		t.Fatal(err)
	}
	var snap map[string]any
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatal(err)
	}
	units, _ := snap["units"].([]any)
	for _, u := range units {
		mutants, _ := u.(map[string]any)["mutants"].([]any)
		for _, m := range mutants {
			delete(m.(map[string]any), "scope")
			delete(m.(map[string]any), "suite_evidence")
		}
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, snapshot, string(data)+"\n")
}

// staleOf is the mutation.stale problems mutation check reports of B.
func staleOf(t *testing.T, o outcome, b string) []map[string]any {
	t.Helper()
	var stale []map[string]any
	for _, p := range o.json(t).Problems {
		if p["rule"] == "mutation.stale" && p["file"] == filepath.FromSlash(b) {
			stale = append(stale, p)
		}
	}
	return stale
}

// changeTest changes the test file name without changing what it tests.
func changeTest(t *testing.T, name string) {
	t.Helper()
	comment := "// changed\n"
	if strings.HasSuffix(name, ".py") {
		comment = "# changed\n"
	}
	appendTo(t, filepath.FromSlash(name), "\n"+comment)
}

// @ID-MUT-215
func TestATestThatReachesAFileThroughAnotherModuleIsAmongItsTests(t *testing.T) {
	t.Parallel()
	for _, language := range []string{"typescript", "python", "kotlin"} {
		t.Run(language, func(t *testing.T) {
			t.Parallel()
			p := reachProjects[language]
			reachRepo(t, p)
			b := filepath.FromSlash(p.b)

			// A test that reaches neither A nor B changing leaves B's result
			// fresh.
			changeTest(t, p.others)
			o := mutationCheck(t, "--json", b)
			if stale := staleOf(t, o, p.b); len(stale) != 0 {
				t.Errorf("after %s changed, %s's result is stale: %v\n%s", p.others, p.b, stale, o.stdout)
			}

			// When the test that imports A changes, mutation check reports B's
			// result stale, naming the test.
			changeTest(t, p.reaching)
			o = mutationCheck(t, "--json", b)
			stale := staleOf(t, o, p.b)
			if len(stale) == 0 {
				t.Fatalf("after %s changed, mutation check reports no stale result of %s:\n%s%s", p.reaching, p.b, o.stdout, o.stderr)
			}
			for _, s := range stale {
				if msg, _ := s["message"].(string); !strings.Contains(msg, p.reaching) {
					t.Errorf("message %q, want it to name %s", msg, p.reaching)
				}
			}
			if o.code != 1 {
				t.Errorf("exit %d, want 1 for the stale result", o.code)
			}
		})
	}
	// But in a Go module a test still reaches only its own package and the
	// packages it imports, not a package two imports away.
	t.Run("go", func(t *testing.T) {
		t.Parallel()
		p := reachProjects["go"]
		reachRepo(t, p)
		changeTest(t, p.reaching)
		o := mutationCheck(t, "--json", filepath.FromSlash(p.b))
		if stale := staleOf(t, o, p.b); len(stale) != 0 {
			t.Errorf("after %s changed, %s's result is stale: %v\n%s", p.reaching, p.b, stale, o.stdout)
		}
		for _, f := range o.checkFunctions(t)[filepath.FromSlash(p.b)] {
			if f.State != "fresh" {
				t.Errorf("%s is %s, want fresh: %s is two imports away from %s", f.Function, f.State, p.b, p.reaching)
			}
		}
	})
}
