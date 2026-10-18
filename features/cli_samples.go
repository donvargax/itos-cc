package features

import (
	"fmt"
	"strings"
)

// The sample commands of the CLI contract scenarios, and the fixture projects
// they run in.

// The projects of the scenarios that only say When I run, by their ID.
var cliSamples = map[string]func(*cliState) error{
	"@ID-CLI-05": (*cliState).flagSample,
	"@ID-CLI-06": (*cliState).pathSample,
	"@ID-CLI-09": (*cliState).goSample,
	"@ID-CLI-13": (*cliState).mutationSample,
	"@ID-CLI-14": (*cliState).listSample,
}

// flagSample is a Go module in the scratch folder: src/board.go of eight
// functions, f1 to f8, each of complexity its number, the first two
// covered by its test, and lib/billing/invoice.go, one function of
// complexity 7 no test runs; go test measures it.
func (c *cliState) flagSample() error {
	var fns []goFunction
	for i := 1; i <= 8; i++ {
		fns = append(fns, goFunction{name: fmt.Sprintf("f%d", i), cc: i, covered: i <= 2})
	}
	board, _ := goFile("board.go", fns)
	invoice, _ := goFile("invoice.go", []goFunction{{name: "g", cc: 7}})
	for name, text := range map[string]string{
		"go.mod": goModule, "src/board.go": board, "src/board_test.go": goTest(fns), "lib/billing/invoice.go": invoice,
	} {
		if err := c.w.write(name, text); err != nil {
			return err
		}
	}
	return c.w.d.Coverage(c.w.dir, Coverage{Language: "go"})
}

// pathSample is a folder called --tests holding a production file, and a
// test file elsewhere.
func (c *cliState) pathSample() error {
	if err := c.w.write("--tests/board.ts", sourceFor("board.ts")); err != nil {
		return err
	}
	return c.w.write("src/board.test.ts", sourceFor("board.test.ts"))
}

// goSample is a Go module whose test covers the first of its functions,
// and what go test prints when it measures them.
func (c *cliState) goSample() error {
	project := &crapState{w: c.w}
	if err := project.goProject(".", []goFunction{{name: "place", cc: 2, covered: true}, {name: "clear", cc: 3}}); err != nil {
		return err
	}
	return c.w.d.Coverage(c.w.dir, Coverage{Language: "go", Report: project.report, Output: "ok  \tboard\t0.004s\tcoverage: 40.0% of statements in ./...\n"})
}

// The Vitest project of ID-CLI-13: src/board.ts's 17 mutation sites, of
// which its tests kill 14, leave one alive (score's 1 to 0, as score(0)
// and score(1) both give 0) and never reach two (unused's).
const (
	boardTS = `export function inside(n: number): boolean {
  return n > 0 && n < 9;
}

export function blocked(a: boolean, b: boolean): boolean {
  return a || b;
}

export function digits(n: number): number {
  if (n < 0) {
    return 0;
  }
  return String(n).length;
}

export function score(n: number): number {
  if (n === 1) {
    return 0;
  }
  return n - 1;
}

export function either(a: boolean, b: boolean): boolean {
  return a && !b;
}

export function unused(n: number): boolean {
  return n !== 0;
}
`
	boardTestTS = `import { expect, test } from "vitest";
import { inside, blocked, digits, score, either } from "./board";

test("inside", () => {
  expect(inside(0)).toBe(false);
  expect(inside(1)).toBe(true);
  expect(inside(8)).toBe(true);
  expect(inside(9)).toBe(false);
});

test("blocked", () => {
  expect(blocked(true, false)).toBe(true);
  expect(blocked(false, false)).toBe(false);
});

test("digits", () => {
  expect(digits(-1)).toBe(0);
  expect(digits(0)).toBe(1);
  expect(digits(42)).toBe(2);
});

test("score", () => {
  expect(score(1)).toBe(0);
  expect(score(5)).toBe(4);
});

test("either", () => {
  expect(either(true, false)).toBe(true);
  expect(either(true, true)).toBe(false);
  expect(either(false, false)).toBe(false);
});
`
)

// boardLCOV is the LCOV Vitest's V8 coverage writes for src/board.ts, as
// it was written and once score and either have their comments: every
// function's lines covered but unused's, line 28 or, a line under each
// comment, 30.
var boardLCOV = map[bool]string{
	false: `TN:
SF:src/board.ts
FN:1,inside
FN:5,blocked
FN:9,digits
FN:16,score
FN:23,either
FN:27,unused
FNF:6
FNH:5
FNDA:4,inside
FNDA:2,blocked
FNDA:3,digits
FNDA:2,score
FNDA:3,either
FNDA:0,unused
DA:2,4
DA:6,2
DA:10,3
DA:11,1
DA:13,2
DA:17,2
DA:18,1
DA:20,1
DA:24,3
DA:28,0
LF:10
LH:9
BRDA:2,0,0,4
BRDA:2,0,1,3
BRDA:6,1,0,2
BRDA:6,1,1,1
BRDA:10,2,0,1
BRDA:10,2,1,2
BRDA:17,3,0,1
BRDA:17,3,1,1
BRDA:24,4,0,3
BRDA:24,4,1,2
BRF:10
BRH:10
end_of_record
`,
	true: `TN:
SF:src/board.ts
FN:1,inside
FN:5,blocked
FN:9,digits
FN:16,score
FN:24,either
FN:29,unused
FNF:6
FNH:5
FNDA:4,inside
FNDA:2,blocked
FNDA:3,digits
FNDA:2,score
FNDA:3,either
FNDA:0,unused
DA:2,4
DA:6,2
DA:10,3
DA:11,1
DA:13,2
DA:18,2
DA:19,1
DA:21,1
DA:26,3
DA:30,0
LF:10
LH:9
BRDA:2,0,0,4
BRDA:2,0,1,3
BRDA:6,1,0,2
BRDA:6,1,1,1
BRDA:10,2,0,1
BRDA:10,2,1,2
BRDA:18,3,0,1
BRDA:18,3,1,1
BRDA:26,4,0,3
BRDA:26,4,1,2
BRF:10
BRH:10
end_of_record
`,
}

// boardTests are the Vitest project's tests as mutation run runs them over
// src/board.ts, which kill every mutant they reach but score's 1 to 0 on
// the line given, score's if.
func boardTests(scoreIf int) Tests {
	return Tests{Language: "typescript", Command: vitestCommand + "src/board.ts",
		Surviving: []Site{{File: "src/board.ts", Line: scoreIf, Original: "1", Replacement: "0"}}}
}

// mutationSample writes the Vitest project, runs mutation run over
// src/board.ts once, which caches every outcome, then changes score and
// either by a comment each: the next run reuses the eight killed mutants
// of the three functions unchanged and runs the seven of those two again,
// score's survivor among them, its if a line further down.
func (c *cliState) mutationSample() error {
	files := map[string]string{
		"package.json":      `{"name": "m", "type": "module", "devDependencies": {"vitest": "5.0.2", "@vitest/coverage-v8": "5.0.2"}}` + "\n",
		"src/board.ts":      boardTS,
		"src/board.test.ts": boardTestTS,
	}
	for name, text := range files {
		if err := c.w.write(name, text); err != nil {
			return err
		}
	}
	if err := c.w.d.Coverage(c.w.dir, Coverage{Language: "typescript", Report: boardLCOV[false]}); err != nil {
		return err
	}
	if err := c.w.d.Tests(c.w.dir, boardTests(17)); err != nil {
		return err
	}
	code, stdout, stderr, err := c.w.capture(c.w.dir, c.w.env(), "mutation", "run", "src/board.ts")
	if err != nil {
		return err
	}
	if want := "(ran 15, reused 0)"; code != 1 || !strings.Contains(stdout, want) {
		return fmt.Errorf("the first mutation run did not run the 15 mutants it reaches: exit %d\n%s%s", code, stdout, stderr)
	}
	changed := strings.NewReplacer("  if (n === 1) {", "  // one scores nothing\n  if (n === 1) {",
		"  return a && !b;", "  // a, unless b\n  return a && !b;").Replace(boardTS)
	if err := c.w.write("src/board.ts", changed); err != nil {
		return err
	}
	if err := c.w.d.Coverage(c.w.dir, Coverage{Language: "typescript", Report: boardLCOV[true]}); err != nil {
		return err
	}
	return c.w.d.Tests(c.w.dir, boardTests(18))
}

// listSample is x.ts, of two mutation sites, in a project whose test
// script leaves test-ran behind if it runs.
func (c *cliState) listSample() error {
	if err := c.w.write("package.json", `{"scripts": {"test": "echo ran > test-ran"}}`+"\n"); err != nil {
		return err
	}
	return c.w.write("x.ts", "export function place(a: number): boolean {\n  return a > 0;\n}\n")
}

// The sites of listSample's x.ts: the file, line and column, function,
// original and replacement of each.
var xSites = []string{"x.ts 2 12 x#place > >=", "x.ts 2 14 x#place 0 1"}
