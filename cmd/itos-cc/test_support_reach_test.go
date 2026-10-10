package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// The scenarios of "Rule: Tests reach code through the test-support files
// they use" in features/mutate.feature. @ID-MUT-219 seeds each project as
// @ID-MUT-215's step does (reachRepo): A's mutant recorded by A's own tests,
// with no test runner. Its test reaches A only through a support file, a
// helper or a conftest.py, which imports A; a test of another module uses no
// such support. @ID-MUT-220 runs a real pytest, and skips, naming what is
// missing, without python3, pytest and coverage; CI runs it on ubuntu-latest
// (T-13).

var supportProjects = map[string]reachProject{
	"python helper": {
		files: map[string]string{
			"pyproject.toml":      "[project]\nname = \"m\"\nversion = \"0.0.0\"\n",
			"a.py":                "def alpha(i):\n    return i > 5\n",
			"tests/helpers.py":    "from a import alpha\n\n\ndef make():\n    return alpha\n",
			"tests/test_alpha.py": "from helpers import make\n\n\ndef test_alpha():\n    assert make()(6)\n",
			"other.py":            "def other(i):\n    return i < 3\n",
			"tests/test_other.py": "from other import other\n\n\ndef test_other():\n    assert other(2)\n",
		},
		b: "a.py", reaching: "tests/test_alpha.py", others: "tests/test_other.py",
	},
	"python conftest": {
		files: map[string]string{
			"pyproject.toml":           "[project]\nname = \"m\"\nversion = \"0.0.0\"\n",
			"a.py":                     "def alpha(i):\n    return i > 5\n",
			"tests/conftest.py":        "import pytest\n\nfrom a import alpha\n\n\n@pytest.fixture\ndef high():\n    return alpha\n",
			"tests/unit/test_alpha.py": "def test_alpha(high):\n    assert high(6)\n",
			"other.py":                 "def other(i):\n    return i < 3\n",
			"test_other.py":            "from other import other\n\n\ndef test_other():\n    assert other(2)\n",
		},
		b: "a.py", reaching: "tests/unit/test_alpha.py", others: "test_other.py",
	},
	"typescript helper": {
		files: map[string]string{
			"package.json":             `{"name": "m"}` + "\n",
			"src/a.ts":                 "export function alpha(i: number): boolean {\n  return i > 5;\n}\n",
			"src/__tests__/helpers.ts": "import { alpha } from \"../a\";\n\nexport const make = () => alpha;\n",
			"src/__tests__/a.test.ts":  "import { make } from \"./helpers\";\n\ntest(\"alpha\", () => {\n  expect(make()(6)).toBe(true);\n});\n",
			"src/other.ts":             "export function other(i: number): boolean {\n  return i < 3;\n}\n",
			"src/other.test.ts":        "import { other } from \"./other\";\n\ntest(\"other\", () => {\n  expect(other(2)).toBe(true);\n});\n",
		},
		b: "src/a.ts", reaching: "src/__tests__/a.test.ts", others: "src/other.test.ts",
	},
	"kotlin helper": {
		files: map[string]string{
			"settings.gradle.kts":                     "rootProject.name = \"m\"\n",
			"build.gradle.kts":                        "plugins {\n    kotlin(\"jvm\") version \"2.4.21\"\n}\n",
			"src/main/kotlin/lib/a/Alpha.kt":          "package lib.a\n\nclass Alpha {\n    fun high(i: Int): Boolean {\n        return i > 5\n    }\n}\n",
			"src/test/kotlin/app/support/Fixtures.kt": "package app.support\n\nimport lib.a.Alpha\n\nobject Fixtures {\n    fun alpha() = Alpha()\n}\n",
			"src/test/kotlin/app/AlphaTest.kt":        "package app\n\nimport app.support.Fixtures\n\nclass AlphaTest {\n    fun high() = Fixtures.alpha().high(6)\n}\n",
			"src/main/kotlin/lib/c/Other.kt":          "package lib.c\n\nclass Other {\n    fun low(i: Int): Boolean {\n        return i < 3\n    }\n}\n",
			"src/test/kotlin/app/OtherTest.kt":        "package app\n\nimport lib.c.Other\n\nclass OtherTest {\n    fun low() = Other().low(2)\n}\n",
		},
		b: "src/main/kotlin/lib/a/Alpha.kt", reaching: "src/test/kotlin/app/AlphaTest.kt", others: "src/test/kotlin/app/OtherTest.kt",
	},
}

// @ID-MUT-219
func TestATestReachesAFileThroughTheTestSupportFilesItUses(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"python helper", "python conftest", "typescript helper", "kotlin helper"} {
		t.Run(strings.ReplaceAll(name, " ", "_"), func(t *testing.T) {
			t.Parallel()
			// Given a <language> project whose test reaches module A only
			// through <support>, which imports A, and A's mutant has a
			// recorded result.
			p := supportProjects[name]
			reachRepo(t, p)
			a := filepath.FromSlash(p.b)

			// When that test changes, "itos-cc mutation check" reports A's
			// result stale, naming the test.
			changeTest(t, p.reaching)
			o := mutationCheck(t, "--json", a)
			stale := staleOf(t, o, p.b)
			if len(stale) == 0 {
				t.Errorf("after %s changed, mutation check reports no stale result of %s:\n%s%s", p.reaching, p.b, o.stdout, o.stderr)
			}
			for _, s := range stale {
				if msg, _ := s["message"].(string); !strings.Contains(msg, p.reaching) {
					t.Errorf("message %q, want it to name %s", msg, p.reaching)
				}
			}
		})
		t.Run(strings.ReplaceAll(name, " ", "_")+"/unrelated", func(t *testing.T) {
			t.Parallel()
			// And a test that uses no support reaching A changing leaves A's
			// result fresh.
			p := supportProjects[name]
			reachRepo(t, p)
			changeTest(t, p.others)
			o := mutationCheck(t, "--json", filepath.FromSlash(p.b))
			if stale := staleOf(t, o, p.b); len(stale) != 0 {
				t.Errorf("after %s changed, %s's result is stale: %v\n%s", p.others, p.b, stale, o.stdout)
			}
		})
	}
}

// @ID-MUT-220
func TestAPythonMutantReachedOnlyThroughConftestIsJudgedByTheTestThatUsesTheFixture(t *testing.T) {
	t.Parallel()
	languageTools(t, "python")
	// pytest plugins installed beside pytest are no part of the scenario.
	useEnv(t, "PYTEST_DISABLE_PLUGIN_AUTOLOAD", "1")
	// Given a Python project whose test_a.py uses a conftest.py fixture that
	// imports a.py, and no test imports a.py. test_other.py, beside it but
	// out of the fixture's directory, fails, so a run of it fails too.
	dir := t.TempDir()
	for name, text := range map[string]string{
		"pyproject.toml":    "[project]\nname = \"m\"\nversion = \"0.0.0\"\n",
		"a.py":              "def low(i):\n    return i == 3\n",
		"tests/conftest.py": "import pytest\n\nfrom a import low\n\n\n@pytest.fixture\ndef check():\n    return low\n",
		"tests/test_a.py":   "def test_low(check):\n    assert check(3)\n    assert not check(4)\n",
		"test_other.py":     "def test_other():\n    assert False\n",
	} {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(name)), text)
	}
	useDir(t, dir)

	// When I run "itos-cc mutation run --json" for a.py.
	o := mutateCovered(t, "--json", "a.py")
	logPytestRun(t, &o)

	// Then a.py's baseline runs test_a.py, its coverage is measured from
	// test_a.py, and its mutant is killed, not reported uncovered.
	want := " tests/test_a.py"
	ran := map[string]bool{}
	for _, line := range strings.Split(o.stderr, "\n") {
		if !strings.Contains(line, " -m pytest") {
			continue
		}
		stage, _, _ := strings.Cut(strings.TrimPrefix(line, "itos-cc: "), " ")
		ran[stage] = true
		if !strings.HasSuffix(line, want) {
			t.Errorf("%q runs other tests than tests/test_a.py", line)
		}
	}
	if !ran["baseline"] || !ran["coverage"] {
		t.Errorf("pytest ran for %v, want for a.py's baseline and its coverage", ran)
	}
	if f := o.json(t).file(t, "a.py"); f.Baseline != "passed" || f.Killed != 1 || f.Uncovered != 0 {
		t.Errorf("a.py: %+v, want its baseline passed and its one mutant killed, not uncovered", f)
	}
}
