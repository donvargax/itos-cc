package coverage

import (
	"math"
	"strings"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func fraction(t *testing.T, r *Report, file string, start, end int) float64 {
	t.Helper()
	f, ok := r.Fraction(file, start, end)
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

func TestJaCoCoLinesAreWeightedByInstructions(t *testing.T) {
	entries, err := ParseJaCoCo(strings.NewReader(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<!DOCTYPE report PUBLIC "-//JACOCO//DTD Report 1.1//EN" "report.dtd">
<report name="demo">
  <package name="demo/game">
    <sourcefile name="Board.kt">
      <line nr="15" mi="0" ci="6" mb="0" cb="0"/>
      <line nr="17" mi="2" ci="0" mb="0" cb="0"/>
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
