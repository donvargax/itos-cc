package coverage_test

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/coverage/port/porttest"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// build is Build over files on no disk: a report path names a source only
// by what it says.
func build(t *testing.T, sources []string, base string, entries ...[]coverage.Entry) *coverage.Report {
	t.Helper()
	return coverage.Build(t.Context(), porttest.Files{}, sources, base, entries...)
}

func fraction(t *testing.T, r *coverage.Report, file string, start, end int, skip ...[2]int) float64 {
	t.Helper()
	f, ok := r.Fraction(file, start, end, skip...)
	if !ok {
		t.Fatalf("no coverage for %s %d-%d", file, start, end)
	}
	return f
}

func TestLCOVLinesAreEqualWeight(t *testing.T) {
	entries, err := coverage.ParseLCOV(strings.NewReader(`TN:
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
	r := build(t, []string{"/p/src/demo/board.ts"}, "/p", entries)
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
	entries, err := coverage.ParseLCOV(strings.NewReader(coveragePy))
	if err != nil {
		t.Fatal(err)
	}
	r := build(t, []string{"/p/lib.py"}, "/p", entries)
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
	entries, err := coverage.ParseLCOV(strings.NewReader(v8))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries[0].Branches) != 0 {
		t.Errorf("branches %+v, want none", entries[0].Branches)
	}
	r := build(t, []string{"/p/lib.mjs"}, "/p", entries)
	if got := fraction(t, r, "/p/lib.mjs", 1, 9); !approx(got, 7.0/9) {
		t.Errorf("pick = %v, want 7/9 of its lines", got)
	}
}

func TestGoBlocksAreWeightedByStatements(t *testing.T) {
	entries, err := coverage.ParseGo(strings.NewReader(`mode: set
example.com/demo/board/board.go:5.24,7.2 1 1
example.com/demo/board/board.go:9.30,10.10 3 1
example.com/demo/board/board.go:10.10,12.3 1 0
`))
	if err != nil {
		t.Fatal(err)
	}
	r := build(t, []string{"/w/gomod/board/board.go"}, "/w/gomod", entries)
	if got := fraction(t, r, "/w/gomod/board/board.go", 9, 12); !approx(got, 3.0/4) {
		t.Errorf("Place = %v, want 3/4", got)
	}
	if covered, measured := r.LineCovered("/w/gomod/board/board.go", 11); covered || !measured {
		t.Errorf("line 11 covered=%v measured=%v, want false true", covered, measured)
	}
}

// An empty case clause is a block of no statements: one that ran covers its
// line, one that did not leaves it uncovered, and neither weighs in a
// share (#53).
func TestAGoBlockOfNoStatementsThatRanCoversItsLine(t *testing.T) {
	entries, err := coverage.ParseGo(strings.NewReader(`mode: set
example.com/k/kind.go:4.2,5.11 2 1
example.com/k/kind.go:6.9,6.9 0 1
example.com/k/kind.go:7.9,7.9 0 0
example.com/k/kind.go:8.3,8.12 1 0
example.com/k/kind.go:10.2,10.10 1 1
`))
	if err != nil {
		t.Fatal(err)
	}
	path := "/w/k/kind.go"
	r := build(t, []string{path}, "/w/k", entries)
	if covered, measured := r.LineCovered(path, 6); !covered || !measured {
		t.Errorf("line 6 covered=%v measured=%v, want true true: its empty clause ran", covered, measured)
	}
	if got := r.LineSources(path, 6); !slices.Equal(got, []string{"in-process"}) {
		t.Errorf("line 6 sources %v, want [in-process]", got)
	}
	if covered, measured := r.LineCovered(path, 7); covered || !measured {
		t.Errorf("line 7 covered=%v measured=%v, want false true: its empty clause never ran", covered, measured)
	}
	if got := fraction(t, r, path, 3, 11); !approx(got, 3.0/4) {
		t.Errorf("Kind = %v, want 3/4 of its statements", got)
	}
}

func TestGoBlocksRepeatedByEachTestBinaryAreOneBlock(t *testing.T) {
	entries, err := coverage.ParseGo(strings.NewReader(`mode: set
example.com/demo/board/board.go:9.30,10.10 3 0
example.com/demo/board/board.go:10.10,12.3 1 0
example.com/demo/board/board.go:9.30,10.10 3 1
example.com/demo/board/board.go:10.10,12.3 1 0
example.com/demo/board/board.go:9.30,10.10 3 0
`))
	if err != nil {
		t.Fatal(err)
	}
	r := build(t, []string{"/w/gomod/board/board.go"}, "/w/gomod", entries)
	if got := fraction(t, r, "/w/gomod/board/board.go", 9, 12); !approx(got, 3.0/4) {
		t.Errorf("Place = %v, want 3/4: the block one binary ran is covered", got)
	}
}

func TestGoBlocksRetainColumnsAndUnionListedMeasurements(t *testing.T) {
	path := "/w/gomod/board/board.go"
	primary, err := coverage.ParseGo(strings.NewReader("mode: set\nexample.com/demo/board/board.go:9.3,9.8 1 1\nexample.com/demo/board/board.go:9.12,9.17 1 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	listed, err := coverage.ParseGo(strings.NewReader("mode: set\nexample.com/demo/board/board.go:9.12,9.17 1 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	r := build(t, []string{path}, "/w/gomod", primary)
	var tests coverage.TestCoverage
	tests.Add("listed", build(t, []string{path}, "/w/gomod", listed))
	r.SetTests(&tests)
	blocks := r.GoBlocks(path)
	if len(blocks) != 2 {
		t.Fatalf("blocks %+v, want two distinct same-line spans", blocks)
	}
	if blocks[0].Span != "9.3,9.8" || !blocks[0].Covered {
		t.Errorf("first block %+v, want first span covered", blocks[0])
	}
	if blocks[1].Span != "9.12,9.17" || !blocks[1].Covered {
		t.Errorf("second block %+v, want separately listed-covered span", blocks[1])
	}
}

func TestReportsOfTheSameFileAreCombined(t *testing.T) {
	unit, err := coverage.ParseLCOV(strings.NewReader("SF:src/a.ts\nDA:1,1\nDA:2,0\nend_of_record\n"))
	if err != nil {
		t.Fatal(err)
	}
	integration, err := coverage.ParseLCOV(strings.NewReader("SF:src/a.ts\nDA:1,0\nDA:2,3\nend_of_record\n"))
	if err != nil {
		t.Fatal(err)
	}
	r := build(t, []string{"/p/src/a.ts"}, "/p", unit, integration)
	if got := fraction(t, r, "/p/src/a.ts", 1, 2); !approx(got, 1) {
		t.Errorf("covered share = %v, want 1: each line ran in one of the reports", got)
	}
}

func TestJaCoCoLinesAreWeightedByInstructions(t *testing.T) {
	entries, err := coverage.ParseJaCoCo(strings.NewReader(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
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
	r := build(t, []string{src}, "/k", entries)
	if got := fraction(t, r, src, 15, 17); !approx(got, 6.0/8) {
		t.Errorf("Board = %v, want 6/8", got)
	}
	if got := fraction(t, r, src, 21, 22); !approx(got, 1.0/4) {
		t.Errorf("a function with branches = %v, want 1/4 of them", got)
	}
}

func TestMatchingRefusesAmbiguousTails(t *testing.T) {
	entries := []coverage.Entry{{Path: "util.py", Segments: []coverage.Segment{{Start: 1, End: 1, Total: 1, Covered: 1}}}}
	r := build(t, []string{"/p/a/util.py", "/p/b/util.py"}, "/elsewhere", entries)
	if r.Has("/p/a/util.py") || r.Has("/p/b/util.py") {
		t.Error("a bare file name that matches two sources is not guessed")
	}
	r = build(t, []string{"/p/a/util.py", "/p/b/util.py"}, "/p/a", entries)
	if !r.Has("/p/a/util.py") {
		t.Error("a path relative to base resolves exactly")
	}
}

func TestReportPathsMatchWhicheverSeparatorTheyWereWrittenWith(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src", "demo", "board.ts")
	absolute := filepath.ToSlash(src)
	for _, path := range []string{
		`src\demo\board.ts`, "src/demo/board.ts", // relative to base
		`demo\board.ts`, "demo/board.ts", // by its tail
		strings.ReplaceAll(absolute, "/", `\`), absolute, // absolute
	} {
		entries := []coverage.Entry{{Path: path, Segments: []coverage.Segment{{Start: 1, End: 1, Total: 1, Covered: 1}}}}
		if r := build(t, []string{src}, root, entries); !r.Has(src) {
			t.Errorf("the report path %s does not match %s", path, src)
		}
	}
}

func TestMatchingNeverLendsAnotherFilesCoverage(t *testing.T) {
	root := t.TempDir()
	src, other := filepath.Join(root, "b", "main.go"), filepath.Join(root, "a", "b", "main.go")
	files := porttest.Files{Dir: root, Regular: []string{src, other}}
	for _, path := range []string{"a/b/main.go", other} {
		entries := []coverage.Entry{{Path: path, Segments: []coverage.Segment{{Start: 1, End: 1, Total: 1, Covered: 1}}}}
		if r := coverage.Build(t.Context(), files, []string{src}, root, entries); r.Has(src) {
			t.Errorf("%s is another file in the project, not b/main.go", path)
		}
	}
}

func TestModulePathsNameTheProjectFileTheirTailFinds(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "metrics", "metrics.go")
	files := porttest.Files{Dir: root, Regular: []string{src, filepath.Join(root, "graph", "metrics.go")}}
	entry := func(path string) []coverage.Entry {
		return []coverage.Entry{{Path: path, Segments: []coverage.Segment{{Start: 1, End: 1, Total: 1, Covered: 1}}}}
	}
	if r := coverage.Build(t.Context(), files, []string{src}, root, entry("example.com/m/graph/metrics.go")); r.Has(src) {
		t.Error("graph/metrics.go lent its coverage to metrics/metrics.go")
	}
	if r := coverage.Build(t.Context(), files, []string{src}, root, entry("example.com/m/metrics/metrics.go")); !r.Has(src) {
		t.Error("the module path of metrics/metrics.go did not match it")
	}
}

// A report that resolved a symlinked directory names the file the source
// is, as the port says; the runner's Disk is held to the same on a real
// symlink. A regular file that is no source lends it nothing.
func TestMatchingFollowsSymlinkedDirectories(t *testing.T) {
	dir := t.TempDir()
	real, link := filepath.Join(dir, "real"), filepath.Join(dir, "link")
	files := porttest.Files{Dir: dir, Regular: []string{filepath.Join(real, "lib.py"), filepath.Join(real, "other", "lib.py")},
		Links: map[string]string{link: real}}
	src := filepath.Join(link, "lib.py")
	entries := []coverage.Entry{{Path: filepath.Join(real, "lib.py"), Segments: []coverage.Segment{{Start: 1, End: 1, Total: 1, Covered: 1}}}}
	if r := coverage.Build(t.Context(), files, []string{src}, link, entries); !r.Has(src) {
		t.Error("a report that resolved the symlink still names the source")
	}
	entries[0].Path = filepath.Join(real, "other", "lib.py")
	if r := coverage.Build(t.Context(), files, []string{src}, link, entries); !r.Empty() {
		t.Error("a regular file that is no source lent its coverage to the one source of its name")
	}
}

func TestParseDetectsTheFormat(t *testing.T) {
	for name, want := range map[string]string{
		"testdata/go.out":     "example.com/demo/board/board.go",
		"testdata/lcov.info":  "src/demo/board.ts",
		"testdata/jacoco.xml": "demo/game/Board.kt",
	} {
		f, err := os.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		entries, err := coverage.Parse(f)
		if closeErr := f.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(entries) != 1 || entries[0].Path != want {
			t.Errorf("%s: entries %+v, want one for %s", name, entries, want)
		}
	}
}

func TestIntegrationBlocksFoldIntoTheTestsOwnAndSayWhichCoveredThem(t *testing.T) {
	parse := func(profile string, src coverage.Source) []coverage.Entry {
		t.Helper()
		entries, err := coverage.ParseGo(strings.NewReader(profile))
		if err != nil {
			t.Fatal(err)
		}
		for i := range entries {
			entries[i].Source = src
		}
		return entries
	}
	inProcess := parse(`mode: set
example.com/demo/main.go:5.2,6.10 1 1
example.com/demo/main.go:7.2,8.10 1 0
example.com/demo/main.go:9.2,10.10 1 1
example.com/demo/main.go:11.2,12.10 1 0
`, coverage.InProcess)
	integration := parse(`mode: set
example.com/demo/main.go:5.2,6.10 1 1
example.com/demo/main.go:7.2,8.10 1 1
example.com/demo/main.go:9.2,10.10 1 0
example.com/demo/main.go:11.2,12.10 1 0
`, coverage.Integration)
	file := "/w/demo/main.go"
	r := build(t, []string{file}, "/w/demo", inProcess, integration)
	if got := fraction(t, r, file, 5, 12); !approx(got, 3.0/4) {
		t.Errorf("fraction %v, want 3/4: a block either covers is covered, and counted once", got)
	}
	for line, want := range map[int][]string{
		5: {"in-process", "integration"}, 7: {"integration"}, 9: {"in-process"}, 11: nil,
	} {
		if got := r.LineSources(file, line); !slices.Equal(got, want) {
			t.Errorf("line %d sources %q, want %q", line, got, want)
		}
	}
}
