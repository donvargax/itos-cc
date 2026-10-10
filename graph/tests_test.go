package graph

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/project"
)

// A Go file's tests are those of its own package and of the packages that
// import it; a TypeScript, Python or Kotlin file's are those whose imports
// reach it, transitively.
func TestTheTestsOfAFileAreThoseThatReachIt(t *testing.T) {
	root := t.TempDir()
	for name, text := range map[string]string{
		"go.mod":              "module example.com/m\n\ngo 1.22\n",
		"board/board.go":      "package board\n\ntype Board struct{}\n",
		"board/board_test.go": "package board\n\nimport \"testing\"\n\nfunc TestB(t *testing.T) {}\n",
		"app/app.go":          "package app\n\nimport \"example.com/m/board\"\n\nvar B board.Board\n",
		"app/app_test.go":     "package app\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n",
		// cmd reaches board only through app: not its test.
		"cmd/cmd.go":      "package cmd\n\nimport \"example.com/m/app\"\n\nvar A = app.B\n",
		"cmd/cmd_test.go": "package cmd\n\nimport \"testing\"\n\nfunc TestC(t *testing.T) {}\n",

		"web/total.ts":      "export function total(a: number): number {\n  return a + 1;\n}\n",
		"web/view.ts":       "import { total } from \"./total\";\nexport const v = total(1);\n",
		"web/total.test.ts": "import { total } from \"./total\";\ntest(\"t\", () => total(1));\n",
		"web/view.test.ts":  "import { v } from \"./view\";\ntest(\"v\", () => v);\n",
		// ping and pong import each other: a cycle reaches both, once.
		"web/ping.ts":      "import { pong } from \"./pong\";\nexport const ping = () => pong;\n",
		"web/pong.ts":      "import { ping } from \"./ping\";\nexport const pong = () => ping;\n",
		"web/ping.test.ts": "import { ping } from \"./ping\";\ntest(\"p\", () => ping);\n",

		"py/pyproject.toml": "[project]\nname = \"tax\"\n",
		"py/tax.py":         "from rate import rate\n\ndef tax(x):\n    return rate(x)\n",
		"py/rate.py":        "def rate(x):\n    return x\n",
		"py/test_tax.py":    "from tax import tax\n\ndef test_tax():\n    assert tax(1)\n",

		"kt/src/main/kotlin/com/acme/Money.kt":        "package com.acme\n\nimport com.acme.cur.Currency\n\nclass Money(val cents: Int, val c: Currency)\n",
		"kt/src/main/kotlin/com/acme/cur/Currency.kt": "package com.acme.cur\n\nclass Currency(val code: String)\n",
		"kt/src/test/kotlin/com/acme/MoneyTest.kt":    "package com.acme\n\nclass MoneyTest {\n    fun t() = Money(1)\n}\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := project.Discover([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	tests, err := TestsImporting(root, files)
	if err != nil {
		t.Fatal(err)
	}
	of := func(name string) []string {
		var out []string
		for _, p := range tests[filepath.Join(root, filepath.FromSlash(name))] {
			out = append(out, rel(root, p))
		}
		return out
	}
	for name, want := range map[string][]string{
		"board/board.go": {"app/app_test.go", "board/board_test.go"},
		"app/app.go":     {"app/app_test.go", "cmd/cmd_test.go"},
		"cmd/cmd.go":     {"cmd/cmd_test.go"},
		"web/total.ts":   {"web/total.test.ts", "web/view.test.ts"},
		"web/ping.ts":    {"web/ping.test.ts"},
		"web/pong.ts":    {"web/ping.test.ts"},
		"py/rate.py":     {"py/test_tax.py"},
		"kt/src/main/kotlin/com/acme/cur/Currency.kt": {"kt/src/test/kotlin/com/acme/MoneyTest.kt"},
		"web/view.ts":                          {"web/view.test.ts"},
		"py/tax.py":                            {"py/test_tax.py"},
		"kt/src/main/kotlin/com/acme/Money.kt": {"kt/src/test/kotlin/com/acme/MoneyTest.kt"},
	} {
		if got := of(name); !slices.Equal(got, want) {
			t.Errorf("tests of %s: %q, want %q", name, got, want)
		}
	}
}

// A test reaches what the test-support files it uses reach: helpers it
// imports, in a cycle or importing nothing of the sources, another test it
// imports, and in Python the conftest.py files of its directory and those
// above it, up to its build root.
func TestATestReachesThroughTheTestSupportFilesItUses(t *testing.T) {
	root := t.TempDir()
	for name, text := range map[string]string{
		"py/pyproject.toml":     "[project]\nname = \"m\"\n",
		"py/a.py":               "def a():\n    return 1\n",
		"py/b.py":               "def b():\n    return 1\n",
		"py/c.py":               "def c():\n    return 1\n",
		"py/conftest.py":        "from c import c\n",
		"py/tests/conftest.py":  "from b import b\n",
		"py/tests/one.py":       "from two import TWO\n\nfrom a import a\n",
		"py/tests/two.py":       "from one import a\n\nTWO = 2\n",
		"py/tests/empty.py":     "import os\n",
		"py/tests/test_one.py":  "from one import a\nfrom empty import os\n",
		"py/tests/test_more.py": "from tests.test_one import a\n",
		"py/other/test_c.py":    "def test_c():\n    pass\n",
		// Above the build root: it applies to none of its tests.
		"conftest.py": "",

		"web/package.json":            `{"name": "w"}` + "\n",
		"web/src/total.ts":            "export const total = 1;\n",
		"web/src/__tests__/make.ts":   "import { total } from \"../total\";\nexport const make = () => total;\n",
		"web/src/__tests__/t.test.ts": "import { make } from \"./make\";\ntest(\"t\", () => make());\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := project.Discover([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	tests, err := TestsImporting(root, files)
	if err != nil {
		t.Fatal(err)
	}
	of := func(name string) []string {
		var out []string
		for _, p := range tests[filepath.Join(root, filepath.FromSlash(name))] {
			out = append(out, rel(root, p))
		}
		return out
	}
	for name, want := range map[string][]string{
		"py/a.py": {"py/tests/one.py", "py/tests/test_more.py", "py/tests/test_one.py", "py/tests/two.py"},
		// A conftest.py applies to the tests pytest runs, not to helpers.
		"py/b.py":          {"py/tests/conftest.py", "py/tests/test_more.py", "py/tests/test_one.py"},
		"py/c.py":          {"py/conftest.py", "py/other/test_c.py", "py/tests/conftest.py", "py/tests/test_more.py", "py/tests/test_one.py"},
		"web/src/total.ts": {"web/src/__tests__/make.ts", "web/src/__tests__/t.test.ts"},
	} {
		if got := of(name); !slices.Equal(got, want) {
			t.Errorf("tests of %s: %q, want %q", name, got, want)
		}
	}
}
