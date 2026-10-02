package lang

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// describe renders a unit as "kind namespace#name lines [private]" so a
// whole file's expectations read as one list.
func describe(units []Unit) []string {
	out := make([]string, len(units))
	for i, u := range units {
		s := fmt.Sprintf("%s %s#%s %d-%d", u.Kind, u.Namespace, u.Name, u.StartLine, u.EndLine)
		if u.Private {
			s += " private"
		}
		out[i] = s
	}
	return out
}

func parse(t *testing.T, path, src string) *File {
	t.Helper()
	spec := Detect(path)
	if spec == nil {
		t.Fatalf("no spec for %s", path)
	}
	f, err := Parse(spec, path, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.Close)
	return f
}

func unitsOf(t *testing.T, path, src string) []string {
	t.Helper()
	return describe(parse(t, path, src).Units)
}

func assertUnits(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("units:\n got  %q\n want %q", got, want)
	}
}

func TestTypeScript(t *testing.T) {
	src := `import { x } from "./x";

export function place(b: Board): void {
  const inner = () => 1;
  [1, 2].forEach((n) => n + 1);
}

export const score = (b: Board): number => b.cells.length;
const helper = function () { return 1; };

describe("board", () => {
  const notAUnit = () => 2;
});

export abstract class Board {
  cells: number[] = [];
  abstract draw(): void;
  constructor() {}
  public place(n: number) { this.cells.push(n); }
  private clear() { this.cells = []; }
  #reset() {}
  onClick = () => this.clear();
}
`
	assertUnits(t, unitsOf(t, "testdata/ts/src/demo/board.ts", src), []string{
		"function demo.board#place 3-6",
		"function demo.board#score 8-8",
		"function demo.board#helper 9-9",
		"method demo.board.Board#constructor 18-18",
		"method demo.board.Board#place 19-19",
		"method demo.board.Board#clear 20-20 private",
		// #reset and reset are different members, so the # stays in the name.
		"method demo.board.Board##reset 21-21 private",
		"method demo.board.Board#onClick 22-22",
	})
}

func TestTSXUsesTheTSXGrammar(t *testing.T) {
	src := `export function Cell({ v }: { v: number }) {
  return <td>{v > 0 && <b>{v}</b>}</td>;
}
`
	assertUnits(t, unitsOf(t, "testdata/ts/src/ui/cell.tsx", src), []string{
		"function ui.cell#Cell 1-3",
	})
}

func TestDeclarationFilesAreSkipped(t *testing.T) {
	if Detect("src/types.d.ts") != nil {
		t.Error("a .d.ts file has no bodies to measure")
	}
}

func TestPython(t *testing.T) {
	src := `import os

def place(board, n):
    def inner():
        return n
    return inner()

@cache
def _hidden():
    pass

class Board:
    def __init__(self):
        self.cells = []

    @property
    def size(self):
        return len(self.cells)

    def _clear(self):
        self.cells = []

    class Cell:
        def value(self):
            return 0
`
	assertUnits(t, unitsOf(t, "testdata/py/src/demo/board.py", src), []string{
		"function demo.board#place 3-6",
		"function demo.board#_hidden 9-10 private",
		"method demo.board.Board#__init__ 13-14",
		"method demo.board.Board#size 17-18",
		"method demo.board.Board#_clear 20-21 private",
		"method demo.board.Board.Cell#value 24-25",
	})
}

func TestPythonPackageInit(t *testing.T) {
	assertUnits(t, unitsOf(t, "testdata/py/src/demo/__init__.py", "def main():\n    pass\n"), []string{
		"function demo#main 1-2",
	})
}

func TestKotlin(t *testing.T) {
	src := `package demo.game

import kotlin.math.max

fun place(board: Board, n: Int) {
    fun inner() = n
    listOf(1).forEach { it + 1 }
}

private fun hidden() = 1

class Board {
    private val cells = mutableListOf<Int>()

    fun place(n: Int) { cells.add(n) }

    private fun clear() { cells.clear() }

    companion object {
        fun empty() = Board()
    }
}

object Rules {
    internal fun max() = 9
}
`
	assertUnits(t, unitsOf(t, "src/main/kotlin/demo/game/Board.kt", src), []string{
		"function demo.game#place 5-8",
		"function demo.game#hidden 10-10 private",
		"method demo.game.Board#place 15-15",
		"method demo.game.Board#clear 17-17 private",
		"method demo.game.Board.Companion#empty 20-20",
		"method demo.game.Rules#max 25-25",
	})
}

func TestGo(t *testing.T) {
	units, err := UnitsInFile("testdata/gomod/board/board.go")
	if err != nil {
		t.Fatal(err)
	}
	assertUnits(t, describe(units), []string{
		"function example.com/demo/board#New 5-7",
		"method example.com/demo/board.Board#Place 9-12",
		"method example.com/demo/board.Board#size 14-14 private",
		"function example.com/demo/board#helper 16-16 private",
	})
}

// complexities maps each unit name to its cyclomatic complexity.
func complexities(t *testing.T, path, src string) map[string]int {
	t.Helper()
	f := parse(t, path, src)
	out := map[string]int{}
	for _, u := range f.Units {
		out[u.Name] = f.Complexity(u)
	}
	return out
}

func assertComplexities(t *testing.T, got, want map[string]int) {
	t.Helper()
	if !maps.Equal(got, want) {
		t.Errorf("complexity:\n got  %v\n want %v", got, want)
	}
}

func TestTypeScriptComplexity(t *testing.T) {
	src := `function straight() { return 1; }
function branches(a: number, b?: string) {
  if (a > 0 && b) { return 1; } else if (a < 0 || !b) { return 2; }
  for (const x of [1]) {}
  while (a--) {}
  try { a++; } catch (e) {}
  const c = a ? 1 : 2;
  const d = b ?? "x";
  [1].forEach((n) => { if (n) {} });
  switch (a) { case 1: break; case 2: break; default: break; }
}
`
	assertComplexities(t, complexities(t, "testdata/ts/src/c.ts", src), map[string]int{
		// if, &&, else-if, ||, for, while, catch, ?:, ??, callback if, 2 cases
		"straight": 1, "branches": 13,
	})
}

func TestPythonComplexity(t *testing.T) {
	src := `def straight():
    return 1

def branches(a, b):
    if a and b:
        pass
    elif a or not b:
        pass
    for x in range(3):
        pass
    while a:
        a -= 1
    try:
        pass
    except ValueError:
        pass
    c = 1 if a else 2
    d = [x for x in range(3) if x]
    match a:
        case 1:
            pass
        case _:
            pass
`
	assertComplexities(t, complexities(t, "testdata/py/c.py", src), map[string]int{
		// if, and, elif, or, for, while, except, if-else, comprehension for + if, 2 cases
		"straight": 1, "branches": 13,
	})
}

func TestGoComplexity(t *testing.T) {
	src := `package c

func straight() int { return 1 }

func branches(a int, ch chan int, v any) {
	if a > 0 && a < 9 || a == 20 {
	}
	for i := 0; i < a; i++ {
	}
	switch a {
	case 1:
	case 2:
	default:
	}
	switch v.(type) {
	case int:
	}
	select {
	case <-ch:
	default:
	}
	f := func() { if a > 0 {} }
	f()
}
`
	assertComplexities(t, complexities(t, "testdata/c.go", src), map[string]int{
		// if, &&, ||, for, 2 cases, type case, comm case, closure if
		"straight": 1, "branches": 10,
	})
}

func TestKotlinComplexity(t *testing.T) {
	src := `package c

fun straight() = 1

fun branches(a: Int, b: String?) {
    if (a > 0 && b != null || a < -5) { }
    for (x in 1..3) { }
    while (false) { }
    do { } while (false)
    try { } catch (e: Exception) { }
    val c = b ?: "x"
    when (a) {
        1 -> {}
        2, 3 -> {}
        else -> {}
    }
    listOf(1).forEach { if (it > 0) { } }
}
`
	assertComplexities(t, complexities(t, "testdata/c.kt", src), map[string]int{
		// if, &&, ||, for, while, do-while, catch, ?:, 2 when entries, lambda if
		"straight": 1, "branches": 12,
	})
}

func TestTestFiles(t *testing.T) {
	cases := map[string]bool{
		"src/board.ts":                      false,
		"src/board.test.ts":                 true,
		"src/board.spec.tsx":                true,
		"src/__tests__/board.ts":            true,
		"pkg/board.py":                      false,
		"pkg/test_board.py":                 true,
		"pkg/board_test.py":                 true,
		"tests/helpers.py":                  true,
		"conftest.py":                       true,
		"board.go":                          false,
		"board_test.go":                     true,
		"src/main/kotlin/demo/Board.kt":     false,
		"src/test/kotlin/demo/BoardTest.kt": true,
		"src/test/kotlin/demo/Fixtures.kt":  true,
		// Each Kotlin rule on its own, so no rule hides behind another.
		"app/src/test/kotlin/demo/Fixtures.kt": true,
		"app/tests/Fixtures.kt":                true,
		"src/main/kotlin/demo/BoardTests.kt":   true,
		"src/main/kotlin/demo/BoardSpec.kt":    true,
		"src/main/kotlin/demo/BoardTest.kt":    true,
	}
	for path, want := range cases {
		if got := Detect(path).IsTest(path); got != want {
			t.Errorf("IsTest(%s) = %v, want %v", path, got, want)
		}
	}
}

func TestPythonCoverageStartsAtTheBody(t *testing.T) {
	f := parse(t, "testdata/py/c.py", "@cache\ndef f(\n    x,\n):\n    return x\n\ndef g(): return 1\n")
	got := []int{f.Units[0].StartLine, f.Units[0].BodyLine, f.Units[1].BodyLine}
	if !slices.Equal(got, []int{2, 5, 7}) {
		t.Errorf("start, body, one-liner body = %v, want [2 5 7]", got)
	}
}

func TestTypeScriptCoverageStartsAtTheFirstStatement(t *testing.T) {
	src := "export const f = (x: number) => {\n  // why\n  return x;\n};\nfunction g() {\n  return 1;\n}\nconst h = () => 1;\n"
	f := parse(t, "testdata/ts/src/b.ts", src)
	var got []int
	for _, u := range f.Units {
		got = append(got, u.BodyLine)
	}
	if !slices.Equal(got, []int{3, 6, 8}) {
		t.Errorf("body lines = %v, want [3 6 8]", got)
	}
}

// callees lists the identifiers in src that name the function a call invokes.
func callees(t *testing.T, path, src string) []string {
	t.Helper()
	f := parse(t, path, src)
	var out []string
	Walk(f.Root, func(n *sitter.Node) bool {
		if f.Spec.Syntax.Identifiers[n.Kind()] && f.Spec.Syntax.IsCallee(n) {
			out = append(out, n.Utf8Text(f.Src))
		}
		return true
	})
	return out
}

func TestCallees(t *testing.T) {
	cases := []struct {
		path, src string
		want      []string
	}{
		{"testdata/ts/src/a.ts", "function f(xs) { return xs.map(inc).filter(odd) + g(xs.length) + new Board(); }",
			[]string{"map", "filter", "g", "Board"}},
		{"testdata/py/a.py", "def f(xs):\n    return xs.append(g(xs.size))\n",
			[]string{"append", "g"}},
		{"testdata/a.go", "package a\nfunc f(xs []int) { fmt.Println(g(xs), xs.n) }\n",
			[]string{"Println", "g"}},
		{"testdata/a.kt", "fun f(xs: List<Int>) = xs.map(::inc).size + g(xs.first())\n",
			[]string{"map", "g", "first"}},
	}
	for _, c := range cases {
		if got := callees(t, c.path, c.src); !slices.Equal(got, c.want) {
			t.Errorf("%s: callees %v, want %v", c.path, got, c.want)
		}
	}
}

func importsOf(t *testing.T, path, src string) []string {
	t.Helper()
	var out []string
	for _, imp := range parse(t, path, src).Imports() {
		s := imp.Path
		if len(imp.Names) > 0 {
			s += " " + strings.Join(imp.Names, ",")
		}
		if imp.Wildcard {
			s += ".*"
		}
		out = append(out, fmt.Sprintf("%d:%s", imp.Line, s))
	}
	return out
}

func TestImports(t *testing.T) {
	cases := []struct {
		path, src string
		want      []string
	}{
		{"testdata/ts/src/x.ts", "import { a } from './a';\nimport * as b from \"lodash\";\nexport { c } from '../c';\nconst d = require('./d');\nasync function f() { await import('./e'); }\nimport type { F } from './f';\nexport const g = 1;\n",
			[]string{"1:./a", "2:lodash", "3:../c", "4:./d", "5:./e", "6:./f"}},
		{"testdata/py/x.py", "import os.path\nimport shop.cart as c, shop.items\nfrom . import util\nfrom ..core.models import User\nfrom shop import cart as k, items\ndef f():\n    import json\n",
			[]string{"1:os.path", "2:shop.cart", "2:shop.items", "3:. util", "4:..core.models User", "5:shop cart,items", "7:json"}},
		{"testdata/x.go", "package x\nimport (\n\t\"fmt\"\n\tm \"example.com/demo/board\"\n)\nimport \"strings\"\n",
			[]string{"3:fmt", "4:example.com/demo/board", "6:strings"}},
		{"testdata/x.kt", "package a.b\nimport com.acme.billing.Invoice\nimport com.acme.util.*\nimport kotlin.math.max as mx\n",
			[]string{"2:com.acme.billing.Invoice", "3:com.acme.util.*", "4:kotlin.math.max"}},
	}
	for _, c := range cases {
		if got := importsOf(t, c.path, c.src); !slices.Equal(got, c.want) {
			t.Errorf("%s imports:\n got  %q\n want %q", c.path, got, c.want)
		}
	}
}

func TestKotlinTopLevelNames(t *testing.T) {
	f := parse(t, "testdata/x.kt", "package a\nclass Foo\ninterface Bar\nobject Baz\nfun qux() {}\ntypealias Q = Int\n")
	if got := TopLevelNames(f); !slices.Equal(got, []string{"Foo", "Bar", "Baz", "qux", "Q"}) {
		t.Errorf("names %v", got)
	}
}

// The grammar must keep a class with several annotations and no
// constructor: Spring configuration classes look like this.
func TestKotlinMultiAnnotatedClass(t *testing.T) {
	src := "package demo\n\n@Configuration\n@EnableWebSecurity\nclass SecurityConfig {\n    fun chain(): Int = 1\n}\n\nfun other() {}\n"
	assertUnits(t, unitsOf(t, "testdata/x.kt", src), []string{
		"method demo.SecurityConfig#chain 6-6",
		"function demo#other 9-9",
	})
}
