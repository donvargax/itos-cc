package lang

import (
	"fmt"
	"slices"
	"testing"
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

func unitsOf(t *testing.T, path, src string) []string {
	t.Helper()
	spec := Detect(path)
	if spec == nil {
		t.Fatalf("no spec for %s", path)
	}
	units, err := Units(spec, path, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return describe(units)
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
	assertUnits(t, unitsOf(t, "src/demo/board.ts", src), []string{
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
	assertUnits(t, unitsOf(t, "src/ui/cell.tsx", src), []string{
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
	assertUnits(t, unitsOf(t, "src/demo/board.py", src), []string{
		"function demo.board#place 3-6",
		"function demo.board#_hidden 9-10 private",
		"method demo.board.Board#__init__ 13-14",
		"method demo.board.Board#size 17-18",
		"method demo.board.Board#_clear 20-21 private",
		"method demo.board.Board.Cell#value 24-25",
	})
}

func TestPythonPackageInit(t *testing.T) {
	assertUnits(t, unitsOf(t, "src/demo/__init__.py", "def main():\n    pass\n"), []string{
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
