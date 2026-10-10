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
