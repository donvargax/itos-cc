package coverage

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func fraction(t *testing.T, r *Report, file string, start, end int, skip ...[2]int) float64 {
	t.Helper()
	f, ok := r.Fraction(file, start, end, skip...)
	if !ok {
		t.Fatalf("no coverage for %s %d-%d", file, start, end)
	}
	return f
}

func TestLCOVLinesAreEqualWeight(t *testing.T) {
	entries, err := ParseLCOV(strings.NewReader(`TN:
SF:src/demo/board.ts
DA:2,1
DA:3,0
DA:4,5
DA:9,0
end_of_record
SF:node_modules/lib/index.js
DA:1,1
end_of_record
`))
	if err != nil {
		t.Fatal(err)
	}
	r := Build([]string{"/p/src/demo/board.ts"}, "/p", entries)
	if got := fraction(t, r, "/p/src/demo/board.ts", 1, 5); !approx(got, 2.0/3) {
		t.Errorf("lines 1-5 = %v, want 2/3", got)
	}
	if _, ok := r.Fraction("/p/src/demo/board.ts", 5, 8); ok {
		t.Error("lines with no measured statement have no coverage, not zero")
	}
	if r.Has("/p/node_modules/lib/index.js") {
		t.Error("files that are not sources are dropped")
	}
}

// pick runs once with x > 0, so one of its two branches is taken; never is
// not called. Both reports are trimmed from real runs.
func TestLCOVBranchesScoreFunctionsThatHaveThem(t *testing.T) {
	coveragePy := `SF:lib.py
DA:1,1
DA:2,1
DA:3,1
DA:4,1
DA:6,0
DA:7,1
DA:9,1
DA:10,0
DA:11,0
DA:12,0
BRDA:3,0,jump to line 4,1
BRDA:3,0,jump to line 6,0
BRDA:10,0,jump to line 11,-
BRDA:10,0,jump to line 12,-
end_of_record
`
	entries, err := ParseLCOV(strings.NewReader(coveragePy))
	if err != nil {
		t.Fatal(err)
	}
	r := Build([]string{"/p/lib.py"}, "/p", entries)
	if got := fraction(t, r, "/p/lib.py", 2, 7); !approx(got, 0.5) {
		t.Errorf("pick = %v, want 1/2 of its branches", got)
	}
	if got := fraction(t, r, "/p/lib.py", 10, 12); got != 0 {
		t.Errorf("never = %v, want 0", got)
	}
	if got := fraction(t, r, "/p/lib.py", 4, 4); !approx(got, 1) {
		t.Errorf("a range without branches = %v, want its line coverage, 1", got)
	}
	if got := fraction(t, r, "/p/lib.py", 2, 7, [2]int{3, 3}); !approx(got, 3.0/4) {
		t.Errorf("pick without its decision = %v, want 3/4 of its lines", got)
	}
}

// Node's test runner, c8, and Vitest's v8 provider before AST-aware
// remapping turn V8 blocks into one-branch blocks: the function body is one,
// and the arm that ran is missing. They are not decisions, so lines decide.
func TestLCOVOneBranchBlocksAreNotDecisions(t *testing.T) {
	v8 := `SF:lib.mjs
BRDA:1,0,0,1
BRDA:1,1,0,1
BRDA:5,2,0,0
DA:1,1
DA:2,1
DA:3,1
DA:4,1
DA:5,1
DA:6,0
DA:7,0
DA:8,1
DA:9,1
end_of_record
`
	entries, err := ParseLCOV(strings.NewReader(v8))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries[0].Branches) != 0 {
		t.Errorf("branches %+v, want none", entries[0].Branches)
	}
	r := Build([]string{"/p/lib.mjs"}, "/p", entries)
	if got := fraction(t, r, "/p/lib.mjs", 1, 9); !approx(got, 7.0/9) {
		t.Errorf("pick = %v, want 7/9 of its lines", got)
	}
}

func TestGoBlocksAreWeightedByStatements(t *testing.T) {
	entries, err := ParseGo(strings.NewReader(`mode: set
example.com/demo/board/board.go:5.24,7.2 1 1
example.com/demo/board/board.go:9.30,10.10 3 1
example.com/demo/board/board.go:10.10,12.3 1 0
`))
	if err != nil {
		t.Fatal(err)
	}
	r := Build([]string{"/w/gomod/board/board.go"}, "/w/gomod", entries)
	if got := fraction(t, r, "/w/gomod/board/board.go", 9, 12); !approx(got, 3.0/4) {
		t.Errorf("Place = %v, want 3/4", got)
	}
	if covered, measured := r.LineCovered("/w/gomod/board/board.go", 11); covered || !measured {
		t.Errorf("line 11 covered=%v measured=%v, want false true", covered, measured)
	}
}

func TestGoBlocksRepeatedByEachTestBinaryAreOneBlock(t *testing.T) {
	entries, err := ParseGo(strings.NewReader(`mode: set
example.com/demo/board/board.go:9.30,10.10 3 0
example.com/demo/board/board.go:10.10,12.3 1 0
example.com/demo/board/board.go:9.30,10.10 3 1
example.com/demo/board/board.go:10.10,12.3 1 0
example.com/demo/board/board.go:9.30,10.10 3 0
`))
	if err != nil {
		t.Fatal(err)
	}
	r := Build([]string{"/w/gomod/board/board.go"}, "/w/gomod", entries)
	if got := fraction(t, r, "/w/gomod/board/board.go", 9, 12); !approx(got, 3.0/4) {
		t.Errorf("Place = %v, want 3/4: the block one binary ran is covered", got)
	}
}

func TestReportsOfTheSameFileAreCombined(t *testing.T) {
	unit, err := ParseLCOV(strings.NewReader("SF:src/a.ts\nDA:1,1\nDA:2,0\nend_of_record\n"))
	if err != nil {
		t.Fatal(err)
	}
	integration, err := ParseLCOV(strings.NewReader("SF:src/a.ts\nDA:1,0\nDA:2,3\nend_of_record\n"))
	if err != nil {
		t.Fatal(err)
	}
	r := Build([]string{"/p/src/a.ts"}, "/p", unit, integration)
	if got := fraction(t, r, "/p/src/a.ts", 1, 2); !approx(got, 1) {
		t.Errorf("covered share = %v, want 1: each line ran in one of the reports", got)
	}
}

func TestJaCoCoLinesAreWeightedByInstructions(t *testing.T) {
	entries, err := ParseJaCoCo(strings.NewReader(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<!DOCTYPE report PUBLIC "-//JACOCO//DTD Report 1.1//EN" "report.dtd">
<report name="demo">
  <package name="demo/game">
    <sourcefile name="Board.kt">
      <line nr="15" mi="0" ci="6" mb="0" cb="0"/>
      <line nr="17" mi="2" ci="0" mb="0" cb="0"/>
      <line nr="21" mi="0" ci="4" mb="0" cb="0"/>
      <line nr="22" mi="1" ci="3" mb="3" cb="1"/>
    </sourcefile>
  </package>
</report>`))
	if err != nil {
		t.Fatal(err)
	}
	src := "/k/src/main/kotlin/demo/game/Board.kt"
	r := Build([]string{src}, "/k", entries)
	if got := fraction(t, r, src, 15, 17); !approx(got, 6.0/8) {
		t.Errorf("Board = %v, want 6/8", got)
	}
	if got := fraction(t, r, src, 21, 22); !approx(got, 1.0/4) {
		t.Errorf("a function with branches = %v, want 1/4 of them", got)
	}
}

func TestMatchingRefusesAmbiguousTails(t *testing.T) {
	entries := []Entry{{Path: "util.py", Segments: []Segment{{Start: 1, End: 1, Total: 1, Covered: 1}}}}
	r := Build([]string{"/p/a/util.py", "/p/b/util.py"}, "/elsewhere", entries)
	if r.Has("/p/a/util.py") || r.Has("/p/b/util.py") {
		t.Error("a bare file name that matches two sources is not guessed")
	}
	r = Build([]string{"/p/a/util.py", "/p/b/util.py"}, "/p/a", entries)
	if !r.Has("/p/a/util.py") {
		t.Error("a path relative to base resolves exactly")
	}
}

func TestMatchingNeverLendsAnotherFilesCoverage(t *testing.T) {
	root := t.TempDir()
	src, other := filepath.Join(root, "b", "main.go"), filepath.Join(root, "a", "b", "main.go")
	for _, f := range []string{src, other} {
		os.MkdirAll(filepath.Dir(f), 0o755)
		os.WriteFile(f, nil, 0o644)
	}
	for _, path := range []string{"a/b/main.go", other} {
		entries := []Entry{{Path: path, Segments: []Segment{{Start: 1, End: 1, Total: 1, Covered: 1}}}}
		if r := Build([]string{src}, root, entries); r.Has(src) {
			t.Errorf("%s is another file in the project, not b/main.go", path)
		}
	}
}

func TestModulePathsNameTheProjectFileTheirTailFinds(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "metrics", "metrics.go")
	for _, f := range []string{src, filepath.Join(root, "graph", "metrics.go")} {
		os.MkdirAll(filepath.Dir(f), 0o755)
		os.WriteFile(f, nil, 0o644)
	}
	entry := func(path string) []Entry {
		return []Entry{{Path: path, Segments: []Segment{{Start: 1, End: 1, Total: 1, Covered: 1}}}}
	}
	if r := Build([]string{src}, root, entry("example.com/m/graph/metrics.go")); r.Has(src) {
		t.Error("graph/metrics.go lent its coverage to metrics/metrics.go")
	}
	if r := Build([]string{src}, root, entry("example.com/m/metrics/metrics.go")); !r.Has(src) {
		t.Error("the module path of metrics/metrics.go did not match it")
	}
}

func TestMatchingFollowsSymlinkedDirectories(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	os.WriteFile(filepath.Join(real, "lib.py"), nil, 0o644)
	src := filepath.Join(link, "lib.py")
	entries := []Entry{{Path: filepath.Join(real, "lib.py"), Segments: []Segment{{Start: 1, End: 1, Total: 1, Covered: 1}}}}
	if r := Build([]string{src}, link, entries); !r.Has(src) {
		t.Error("a report that resolved the symlink still names the source")
	}
}

func TestLoadDetectsTheFormat(t *testing.T) {
	for name, want := range map[string]string{
		"testdata/go.out":     "example.com/demo/board/board.go",
		"testdata/lcov.info":  "src/demo/board.ts",
		"testdata/jacoco.xml": "demo/game/Board.kt",
	} {
		entries, err := Load(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(entries) != 1 || entries[0].Path != want {
			t.Errorf("%s: entries %+v, want one for %s", name, entries, want)
		}
	}
}
