package lang

import (
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// leafAt is the first leaf of f on line whose text is text.
func leafAt(t *testing.T, f *File, line int, text string) *sitter.Node {
	t.Helper()
	var found *sitter.Node
	Walk(f.Root, func(n *sitter.Node) bool {
		if found == nil && n.ChildCount() == 0 && int(n.StartPosition().Row)+1 == line && n.Utf8Text(f.Src) == text {
			found = n
		}
		return found == nil
	})
	if found == nil {
		t.Fatalf("%s holds no %q on line %d", f.Path, text, line)
	}
	return found
}

// Each language's statement start comes from its own parse (slice-79): the
// line its innermost statement holding a site starts on, which a line
// coverage report marks when the statement is split over several lines.
func TestASitesStatementStartsWhereItsInnermostStatementDoes(t *testing.T) {
	for _, c := range []struct {
		name, path, src, text string
		line, want            int
	}{
		{"a Python return's second line", "calc.py", "def add(a, b):\n    return (a\n            + b)\n", "+", 3, 2},
		// The binary expression starts on line 3, inside the call the
		// statement on line 2 makes.
		{"a Python expression starting inside its statement", "calc.py", "def add(a, b):\n    x = f(1,\n          a\n          + b)\n    return x\n", "+", 4, 2},
		{"a Python statement nested in a block", "calc.py", "def add(a, b):\n    if a:\n        return (a\n                * b)\n    return 0\n", "*", 4, 3},
		{"a Python site on its statement's line", "calc.py", "def add(a, b):\n    return a + b\n", "+", 2, 2},
		{"a TypeScript return's second line", "src/calc.ts", "export function add(a: number, b: number): number {\n  return (a\n    + b);\n}\n", "+", 3, 2},
		// The arrow's expression body is no statement: Vitest measures it
		// on its own line, which keeps its own answer whatever this says.
		{"a TypeScript arrow body's line", "src/inc.ts", "export function inc(xs: number[]): number[] {\n  return xs.map((x) =>\n    x + 1);\n}\n", "+", 3, 2},
		{"a TypeScript if's body without braces", "src/calc.ts", "export function add(a: number, b: number): number {\n  if (a > 0)\n    return a\n      + b;\n  return 0;\n}\n", "+", 4, 3},
		{"a Kotlin statement's second line", "Calc.kt", "fun add(a: Int, b: Int): Int {\n    val c = a +\n        b\n    return (a\n        + c)\n}\n", "+", 5, 4},
		{"a Kotlin if's body without braces", "Calc.kt", "fun add(a: Int, b: Int): Int {\n    if (a > 0)\n        return a +\n            b\n    return 0\n}\n", "+", 3, 3},
		{"a Kotlin expression body", "Calc.kt", "fun add(a: Int, b: Int): Int =\n    a +\n        b\n", "+", 2, 1},
		{"a Dart return's second line", "lib/calc.dart", "int add(int a, int b) {\n  return (a\n      + b);\n}\n", "+", 3, 2},
		{"a Dart expression body", "lib/calc.dart", "int add(int a, int b) =>\n    a\n    + b;\n", "+", 3, 1},
		// Go's coverage is spans: a site's own line is its statement's.
		{"a Go site's own line", "calc.go", "package calc\n\nfunc add(a, b int) int {\n\treturn (a +\n\t\tb) * 2\n}\n", "*", 5, 5},
	} {
		f := parse(t, c.path, c.src)
		if got := f.StatementLine(leafAt(t, f, c.line, c.text)); got != c.want {
			t.Errorf("%s: StatementLine = %d, want %d", c.name, got, c.want)
		}
	}
}
