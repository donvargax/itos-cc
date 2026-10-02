package mutate

import (
	"fmt"
	"slices"
	"testing"

	"itos-cc/lang"
)

func parse(t *testing.T, path, src string) *lang.File {
	t.Helper()
	f, err := lang.Parse(lang.Detect(path), path, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.Close)
	return f
}

// describe renders sites as "line:original>replacement".
func describe(sites []Site) []string {
	var out []string
	for _, s := range sites {
		out = append(out, fmt.Sprintf("%d:%s>%s", s.Line, s.Original, s.Replacement))
	}
	return out
}

func assertSites(t *testing.T, path, src string, want []string) {
	t.Helper()
	if got := describe(Sites(parse(t, path, src))); !slices.Equal(got, want) {
		t.Errorf("%s sites:\n got  %q\n want %q", path, got, want)
	}
}

func TestTypeScriptSites(t *testing.T) {
	assertSites(t, "x.ts", `const LIMIT = 1 + 2;
function f(xs: Array<number>, a: number): boolean {
  // a < b in a comment
  const s = "x > y";
  if (a > 0 && !done) return a === 1;
  return -a <= 10 || true;
}
`, []string{
		"5:>>>=", "5:0>1", "5:&&>||", "5:!>", "5:===>!==", "5:1>0",
		"6:->", "6:<=><", "6:||>&&", "6:true>false",
	})
}

func TestPythonSites(t *testing.T) {
	assertSites(t, "x.py", `LIMIT = 1 + 2

def f(a, b):
    """a < b"""
    if a >= 0 and not b:
        return a * 2 == 1
    return -a < b or False
`, []string{
		"5:>=>>", "5:0>1", "5:and>or", "5:not>",
		"6:*>/", "6:==>!=", "6:1>0",
		"7:->", "7:<><=", "7:or>and", "7:False>True",
	})
}

func TestGoSites(t *testing.T) {
	assertSites(t, "x.go", `package p

var limit = 1 + 2

func f(a int, xs []int) bool {
	var m map[string][]int
	_ = m
	if a != 0 || !ok(xs) {
		return a-1 > len(xs)
	}
	return true
}
`, []string{
		"8:!=>==", "8:0>1", "8:||>&&", "8:!>",
		"9:->+", "9:1>0", "9:>>>=",
		"11:true>false",
	})
}

func TestKotlinSites(t *testing.T) {
	assertSites(t, "x.kt", `val limit = 1 + 2

fun f(a: Int, xs: List<Int>): Boolean {
    if (a == 0 && !xs.isEmpty()) return false
    return a * 2 >= xs.size
}
`, []string{
		"4:==>!=", "4:0>1", "4:&&>||", "4:!>", "4:false>true",
		"5:*>/", "5:>=>>",
	})
}

func TestApplyReplacesOnlyTheSite(t *testing.T) {
	f := parse(t, "x.py", "def f(a):\n    return a < 1\n")
	sites := Sites(f)
	if got := string(sites[0].Apply(f.Src)); got != "def f(a):\n    return a <= 1\n" {
		t.Errorf("applied %q", got)
	}
	if string(f.Src) != "def f(a):\n    return a < 1\n" {
		t.Error("Apply changed the original source")
	}
}

func TestSiteKeysSurviveCodeAboveTheUnit(t *testing.T) {
	a := parse(t, "x.py", "def f(a):\n    return a < 1\n")
	b := parse(t, "x.py", "import os\n\n\ndef f(a):\n    return a < 1\n")
	if Sites(a)[0].Key() != Sites(b)[0].Key() {
		t.Error("moving a function down the file changed its site keys")
	}
	if UnitHash(a, a.Units[0]) != UnitHash(b, b.Units[0]) {
		t.Error("moving a function down the file changed its hash")
	}
}
