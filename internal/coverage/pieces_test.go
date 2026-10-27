package coverage_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
)

// A report made of another's pieces answers what that one answers of its
// files: which lines ran, from which sources, a Go block's span and weight,
// a branch's share, and a piece of no weight that ran (slice-69's cache).
func TestPiecesKeepWhatAReportSaysOfItsFiles(t *testing.T) {
	inProcess, err := coverage.ParseGo(strings.NewReader("mode: set\nexample.com/demo/main.go:5.2,6.10 1 1\nexample.com/demo/main.go:7.9,7.9 0 1\nexample.com/demo/main.go:9.2,10.10 2 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	integration, err := coverage.ParseGo(strings.NewReader("mode: set\nexample.com/demo/main.go:9.2,10.10 2 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	for i := range integration {
		integration[i].Source = coverage.Integration
	}
	branches, err := coverage.ParseLCOV(strings.NewReader("SF:src/a.ts\nDA:1,1\nDA:2,0\nBRDA:1,0,0,1\nBRDA:1,0,1,0\nend_of_record\n"))
	if err != nil {
		t.Fatal(err)
	}
	main, ts := "/w/demo/main.go", "/w/demo/src/a.ts"
	original := build(t, []string{main, ts}, "/w/demo", inProcess, integration, branches)
	files := map[string][]coverage.Piece{}
	for _, file := range original.Files() {
		files[file] = original.Pieces(file)
	}
	if got := original.Files(); !slices.Equal(got, []string{main, ts}) {
		t.Fatalf("files %v, want both", got)
	}
	copied := coverage.FromPieces(files)
	for _, line := range []int{5, 7, 9} {
		if got, want := copied.LineSources(main, line), original.LineSources(main, line); !slices.Equal(got, want) {
			t.Errorf("line %d sources %v, want %v", line, got, want)
		}
	}
	if got, want := copied.GoBlocks(main), original.GoBlocks(main); !slices.Equal(got, want) {
		t.Errorf("Go blocks %+v, want %+v", got, want)
	}
	got, _ := copied.Fraction(ts, 1, 2)
	want, _ := original.Fraction(ts, 1, 2)
	if !approx(got, want) || !approx(got, 0.5) {
		t.Errorf("a.ts's share %v, want its one taken branch of two, %v", got, want)
	}
	if covered, measured := copied.LineCovered(ts, 2); covered || !measured {
		t.Errorf("a.ts line 2 covered=%v measured=%v, want false true", covered, measured)
	}
	if empty := coverage.FromPieces(map[string][]coverage.Piece{"/w/demo/b.go": nil}); !empty.Has("/w/demo/b.go") {
		t.Error("a file with no pieces is still one the report measured")
	}
}

// The listed tests' coverage a report holds is each test's, by its ID, in
// the list's order; none is nil throughout.
func TestTestsGiveEachListedTestsCoverage(t *testing.T) {
	var none *coverage.Report
	if none.Tests() != nil || none.Files() != nil || none.Pieces("x") != nil {
		t.Error("a nil report holds nothing")
	}
	var tc *coverage.TestCoverage
	if tc.IDs() != nil || tc.Of("ID-1") != nil {
		t.Error("no test coverage names no test")
	}
	r := coverage.FromPieces(nil)
	one := coverage.FromPieces(map[string][]coverage.Piece{"/w/a.go": {{Start: 3, End: 3, Total: 1, Covered: 1, Key: "3.1,3.9"}}})
	tests := &coverage.TestCoverage{}
	tests.Add("ID-2", one)
	tests.Add("ID-1", nil)
	r.SetTests(tests)
	if got := r.Tests().IDs(); !slices.Equal(got, []string{"ID-2", "ID-1"}) {
		t.Errorf("IDs %v, want the order they were added in", got)
	}
	if r.Tests().Of("ID-2") != one || r.Tests().Of("ID-3") != nil {
		t.Error("Of gives each test's own coverage, and none of a test not measured")
	}
}
