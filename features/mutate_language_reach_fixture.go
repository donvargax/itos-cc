// Recorded-state fixtures preserve the original transitive import examples.
package features

type languageReachProject struct {
	files map[string]string
	// edges are the imports among files, as their text says (imports).
	edges               map[string][]string
	b, reaching, others string
}

var languageReachProjects = map[string]languageReachProject{
	"typescript": {
		files: map[string]string{
			"package.json": `{"name": "m", "devDependencies": {"vitest": "5.0.2"}}` + "\n",
			// Vitest installed: its own tests are its related tests.
			"node_modules/.bin/vitest":     "",
			"node_modules/.bin/vitest.cmd": "",
			"src/b.ts":                     "export function beta(i: number): boolean {\n  return i > 5;\n}\n",
			"src/a.ts":                     "import { beta } from \"./b\";\n\nexport function alpha(i: number): boolean {\n  return beta(i);\n}\n",
			"src/a.test.ts":                "import { alpha } from \"./a\";\n\ntest(\"alpha\", () => {\n  expect(alpha(6)).toBe(true);\n});\n",
			"src/other.ts":                 "export function other(i: number): boolean {\n  return i < 3;\n}\n",
			"src/other.test.ts":            "import { other } from \"./other\";\n\ntest(\"other\", () => {\n  expect(other(2)).toBe(true);\n});\n",
		},
		edges: map[string][]string{"src/a.ts": {"src/b.ts"}, "src/a.test.ts": {"src/a.ts"}, "src/other.test.ts": {"src/other.ts"}},
		b:     "src/b.ts", reaching: "src/a.test.ts", others: "src/other.test.ts",
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
		edges: map[string][]string{"a.py": {"b.py"}, "test_a.py": {"a.py"}, "test_other.py": {"other.py"}},
		b:     "b.py", reaching: "test_a.py", others: "test_other.py",
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
		edges: map[string][]string{"src/main/kotlin/lib/a/Alpha.kt": {"src/main/kotlin/lib/b/Beta.kt"}, "src/test/kotlin/app/AlphaTest.kt": {"src/main/kotlin/lib/a/Alpha.kt"}, "src/test/kotlin/app/OtherTest.kt": {"src/main/kotlin/lib/c/Other.kt"}},
		b:     "src/main/kotlin/lib/b/Beta.kt", reaching: "src/test/kotlin/app/AlphaTest.kt", others: "src/test/kotlin/app/OtherTest.kt",
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
		edges: map[string][]string{"app/app.go": {"board/board.go"}, "cmd/cmd.go": {"app/app.go"}},
		b:     "board/board.go", reaching: "cmd/cmd_test.go", others: "other/other_test.go",
	},
}

var languageSupportProjects = map[string]languageReachProject{
	"python helper": {
		files: map[string]string{
			"pyproject.toml":      "[project]\nname = \"m\"\nversion = \"0.0.0\"\n",
			"a.py":                "def alpha(i):\n    return i > 5\n",
			"tests/helpers.py":    "from a import alpha\n\n\ndef make():\n    return alpha\n",
			"tests/test_alpha.py": "from helpers import make\n\n\ndef test_alpha():\n    assert make()(6)\n",
			"other.py":            "def other(i):\n    return i < 3\n",
			"tests/test_other.py": "from other import other\n\n\ndef test_other():\n    assert other(2)\n",
		},
		edges: map[string][]string{"tests/helpers.py": {"a.py"}, "tests/test_alpha.py": {"tests/helpers.py"}, "tests/test_other.py": {"other.py"}},
		b:     "a.py", reaching: "tests/test_alpha.py", others: "tests/test_other.py",
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
		edges: map[string][]string{"tests/conftest.py": {"a.py"}, "test_other.py": {"other.py"}},
		b:     "a.py", reaching: "tests/unit/test_alpha.py", others: "test_other.py",
	},
	"typescript helper": {
		files: map[string]string{
			"package.json": `{"name": "m", "devDependencies": {"vitest": "5.0.2"}}` + "\n",
			// Vitest installed: its own tests are its related tests.
			"node_modules/.bin/vitest":     "",
			"node_modules/.bin/vitest.cmd": "",
			"src/a.ts":                     "export function alpha(i: number): boolean {\n  return i > 5;\n}\n",
			"src/__tests__/helpers.ts":     "import { alpha } from \"../a\";\n\nexport const make = () => alpha;\n",
			"src/__tests__/a.test.ts":      "import { make } from \"./helpers\";\n\ntest(\"alpha\", () => {\n  expect(make()(6)).toBe(true);\n});\n",
			"src/other.ts":                 "export function other(i: number): boolean {\n  return i < 3;\n}\n",
			"src/other.test.ts":            "import { other } from \"./other\";\n\ntest(\"other\", () => {\n  expect(other(2)).toBe(true);\n});\n",
		},
		edges: map[string][]string{"src/__tests__/helpers.ts": {"src/a.ts"}, "src/__tests__/a.test.ts": {"src/__tests__/helpers.ts"}, "src/other.test.ts": {"src/other.ts"}},
		b:     "src/a.ts", reaching: "src/__tests__/a.test.ts", others: "src/other.test.ts",
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
		edges: map[string][]string{"src/test/kotlin/app/support/Fixtures.kt": {"src/main/kotlin/lib/a/Alpha.kt"}, "src/test/kotlin/app/AlphaTest.kt": {"src/test/kotlin/app/support/Fixtures.kt"}, "src/test/kotlin/app/OtherTest.kt": {"src/main/kotlin/lib/c/Other.kt"}},
		b:     "src/main/kotlin/lib/a/Alpha.kt", reaching: "src/test/kotlin/app/AlphaTest.kt", others: "src/test/kotlin/app/OtherTest.kt",
	},
}
