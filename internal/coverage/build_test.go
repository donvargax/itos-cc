package coverage

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage/port"
	"github.com/donvargax/itos-cc/internal/coverage/port/porttest"
)

func lcov(t *testing.T, text string) []Entry {
	t.Helper()
	entries, err := ParseLCOV(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

// What a run found, the runner records through the setters: what it could
// not measure, the languages it measured, and the files it proved.
func TestARunRecordsWhatItMissedMeasuredAndProved(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.py"), filepath.Join(dir, "b.py")
	r := build(t, []string{a, b}, dir, lcov(t, "SF:a.py\nDA:2,1\nDA:3,0\nend_of_record\n"))
	tool := Unmeasured{Dir: dir, Language: "python", Cause: port.ToolMissing{}, Reason: "no coverage.py"}
	r.SetMissing(tool)
	if got := r.Missing(); !reflect.DeepEqual(got, []Unmeasured{tool}) {
		t.Errorf("missing %v, want the tool", got)
	}
	r.SetMissing()
	if got := r.Missing(); got != nil {
		t.Errorf("missing %v after it was cleared, want none", got)
	}
	r.SetIntegrationMissing(tool)
	if got := r.IntegrationMissing(); !reflect.DeepEqual(got, []Unmeasured{tool}) {
		t.Errorf("integration missing %v, want the tool", got)
	}
	var none *Report
	if none.IntegrationMissing() != nil || none.Missing() != nil || !none.Empty() || none.Has(a) {
		t.Error("a nil report holds something")
	}
	r.MarkMeasured("python")
	r.MarkMeasured("go")
	if !r.Measures("python") || !r.Measures("go") || r.Measures("kotlin") {
		t.Errorf("measures python %v, go %v, kotlin %v; want the two marked", r.Measures("python"), r.Measures("go"), r.Measures("kotlin"))
	}
	r.Prove("python", dir, []string{a, b})
	if lines, ok := r.Lines(a); !ok || !reflect.DeepEqual(lines, []Line{{Line: 2, Covered: true}, {Line: 3}}) {
		t.Errorf("a's lines %v %v, want 2 covered and 3 not, proven", lines, ok)
	}
	if lines, ok := r.Lines(b); ok || lines != nil {
		t.Errorf("b's lines %v %v, want none proven", lines, ok)
	}
	if got := r.UnloadedByDir("python"); !reflect.DeepEqual(got, map[string][]string{dir: {b}}) {
		t.Errorf("unloaded by build root %v, want b in its own", got)
	}
	if got := r.UnloadedByDir("go"); len(got) != 0 {
		t.Errorf("unloaded go files %v, want none", got)
	}
	if r.Empty() || !(&Report{}).Empty() {
		t.Error("a report naming a file is empty, or the zero report is not")
	}
	r.Prove("python", dir, nil)
	if _, ok := r.Lines(a); ok || r.Unloaded("python") != nil {
		t.Error("proving no sources kept what an earlier proof recorded")
	}
}

// A static inventory's lines, or a module's report listing every class,
// prove a file no test loaded, every line unexecuted; what it names that
// is no such file is left out.
func TestListProvesTheLinesOfFilesNoTestLoaded(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.py"), filepath.Join(dir, "b.py")
	r := build(t, []string{a, b}, dir, lcov(t, "SF:a.py\nDA:1,1\nend_of_record\n"))
	r.Prove("python", dir, []string{a, b})
	inventory := lcov(t, "SF:b.py\nDA:1,1\nDA:4,0\nend_of_record\nSF:c.py\nDA:1,0\nend_of_record\n")
	r.List(t.Context(), porttest.Files{}, []string{b}, dir, inventory)
	if lines, ok := r.Lines(b); !ok || !reflect.DeepEqual(lines, []Line{{Line: 1}, {Line: 4}}) {
		t.Errorf("b's lines %v %v, want 1 and 4 unexecuted, proven", lines, ok)
	}
	if got := r.Unloaded("python"); got != nil {
		t.Errorf("unloaded %v, want none once listed", got)
	}
	if _, ok := r.Lines(filepath.Join(dir, "c.py")); ok {
		t.Error("a file the inventory was not given is proven")
	}
	var fresh Report
	fresh.List(t.Context(), porttest.Files{}, []string{b}, dir, inventory)
	if lines, ok := fresh.Lines(b); !ok || len(lines) != 2 {
		t.Errorf("listing into an empty report gave %v %v, want b's two lines proven", lines, ok)
	}
}

// Integration data is marked as such, and in a file the in-process report
// names it keeps to the pieces that report counts there; in a file the
// report does not name, it is kept whole.
func TestIntegratedKeepsToWhatTheInProcessReportCounts(t *testing.T) {
	sources := []string{"/p/src/a.ts", "/p/src/b.ts"}
	in := build(t, sources, "/p", lcov(t, "SF:src/a.ts\nDA:1,0\nDA:2,0\nBRDA:2,0,0,0\nBRDA:2,0,1,0\nend_of_record\n"))
	entries := []Entry{
		{Path: "src/a.ts", Segments: []Segment{{Start: 1, End: 1, Total: 1, Covered: 1, Key: "1"}, {Start: 3, End: 3, Total: 1, Covered: 1, Key: "3"},
			{Start: 5, End: 5, Total: 1, Covered: 1}}, Branches: []Segment{{Start: 2, End: 2, Total: 2, Covered: 1, Key: "2,0"}, {Start: 7, End: 7, Total: 2, Covered: 2, Key: "7,0"}}},
		{Path: "src/b.ts", Segments: []Segment{{Start: 9, End: 9, Total: 1, Covered: 1, Key: "9"}}},
	}
	got := in.Integrated(t.Context(), porttest.Files{}, sources, "/p", entries)
	keys := func(segs []Segment) []string {
		var out []string
		for _, s := range segs {
			out = append(out, s.Key)
		}
		return out
	}
	if len(got) != 2 || got[0].Source != Integration || got[1].Source != Integration {
		t.Fatalf("entries %+v, want both, marked integration", got)
	}
	if k := keys(got[0].Segments); !slices.Equal(k, []string{"1", ""}) {
		t.Errorf("a's lines %q, want the one the report counts and the unkeyed one", k)
	}
	if k := keys(got[0].Branches); !slices.Equal(k, []string{"2,0"}) {
		t.Errorf("a's branches %q, want the decision the report counts", k)
	}
	if k := keys(got[1].Segments); !slices.Equal(k, []string{"9"}) {
		t.Errorf("b's lines %q, want them whole: the report does not name b", k)
	}
	r := build(t, sources, "/p", lcov(t, "SF:src/a.ts\nDA:1,0\nDA:2,0\nend_of_record\n"), got)
	if sources := r.LineSources("/p/src/a.ts", 1); !slices.Equal(sources, []string{"integration"}) {
		t.Errorf("line 1 ran in %q, want integration", sources)
	}
}

// Unmeasured is written as --json writes it, its gap its cause's name, and
// said as a person reads it.
func TestUnmeasuredIsWrittenWithItsGapAsItsCause(t *testing.T) {
	for gap, want := range map[port.Gap]string{
		port.ToolMissing{}: "tool-missing", port.MeasuredNothing{}: "measured-nothing", port.NoReport{}: "no-report",
		port.Unreadable{}: "unreadable", port.Cancelled{}: "cancelled",
	} {
		data, err := json.Marshal(Unmeasured{Dir: "d", Language: "go", Cause: gap, Reason: "r"})
		if text := `{"dir":"d","language":"go","cause":"` + want + `","reason":"r"}`; err != nil || string(data) != text {
			t.Errorf("JSON %s, %v; want %s", data, err, text)
		}
	}
	for u, want := range map[Unmeasured]string{
		{Report: "r.info", Dir: "d", Reason: "gone"}: "r.info: gone",
		{Dir: "d", Language: "go", Reason: "broke"}:  "d (go): broke",
		{Dir: "d", Reason: "broke"}:                  "d: broke",
	} {
		if got := u.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	}
}

// Each listed test's coverage is kept in the list's order, and a line
// names the tests that ran it.
func TestLineTestsNameTheListedTestsThatRanALine(t *testing.T) {
	file := "/p/a.py"
	r := build(t, []string{file}, "/p", lcov(t, "SF:a.py\nDA:1,0\nDA:2,0\nend_of_record\n"))
	if r.LineTests(file, 1) != nil || (*Report)(nil).LineTests(file, 1) != nil {
		t.Error("a report without listed tests names some")
	}
	var tests TestCoverage
	tests.Add("second", build(t, []string{file}, "/p", lcov(t, "SF:a.py\nDA:1,1\nDA:2,1\nend_of_record\n")))
	tests.Add("first", build(t, []string{file}, "/p", lcov(t, "SF:a.py\nDA:1,1\nDA:2,0\nend_of_record\n")))
	r.SetTests(&tests)
	(*Report)(nil).SetTests(&tests)
	if got := r.LineTests(file, 1); !slices.Equal(got, []string{"second", "first"}) {
		t.Errorf("line 1 ran in %q, want both in the list's order", got)
	}
	if got := r.LineTests(file, 2); !slices.Equal(got, []string{"second"}) {
		t.Errorf("line 2 ran in %q, want second alone", got)
	}
}

// Merging keeps what each report held: its files, what it missed, the
// languages it measured, what it proved and listed, and the first's listed
// tests.
func TestMergeKeepsWhatEachReportHeld(t *testing.T) {
	dir := t.TempDir()
	a, b, c := filepath.Join(dir, "a.py"), filepath.Join(dir, "b.py"), filepath.Join(dir, "c.go")
	python := build(t, []string{a, b}, dir, lcov(t, "SF:a.py\nDA:1,1\nend_of_record\n"))
	python.MarkMeasured("python")
	python.Prove("python", dir, []string{a, b})
	python.List(t.Context(), porttest.Files{}, []string{b}, dir, lcov(t, "SF:b.py\nDA:3,0\nend_of_record\n"))
	gone := Unmeasured{Dir: dir, Language: "go", Cause: port.MeasuredNothing{}, Reason: "broke"}
	failed := &Report{}
	failed.SetMissing(gone)
	failed.SetIntegrationMissing(gone)
	var tests TestCoverage
	tests.Add("t", python)
	failed.SetTests(&tests)
	merged := Merge(nil, failed, python, FromGoBlocks(map[string][]GoBlock{c: {{Span: "2.1,2.9", Line: 2, Weight: 1, Covered: true}, {Span: "3.1,3.9", Line: 3}}}))
	if !merged.Measures("python") || !merged.Measures("go") || !merged.Has(a) || !merged.Has(c) {
		t.Errorf("merged measures python %v, go %v, has a %v, c %v; want all", merged.Measures("python"), merged.Measures("go"), merged.Has(a), merged.Has(c))
	}
	if !reflect.DeepEqual(merged.Missing(), []Unmeasured{gone}) || !reflect.DeepEqual(merged.IntegrationMissing(), []Unmeasured{gone}) {
		t.Errorf("merged missing %v and %v, want the failed run's", merged.Missing(), merged.IntegrationMissing())
	}
	if lines, ok := merged.Lines(b); !ok || !reflect.DeepEqual(lines, []Line{{Line: 3}}) {
		t.Errorf("b's lines %v %v, want the listed one, proven", lines, ok)
	}
	if got := merged.LineTests(a, 1); !slices.Equal(got, []string{"t"}) {
		t.Errorf("line 1 of a ran in %q, want t", got)
	}
	if blocks := merged.GoBlocks(c); len(blocks) != 1 || blocks[0].Span != "2.1,2.9" || !blocks[0].Covered {
		t.Errorf("c's blocks %+v, want the one of positive weight", blocks)
	}
	if got := Integration.String() + " " + InProcess.String(); got != "integration in-process" {
		t.Errorf("sources are named %q", got)
	}
}

// Line inventories cached complete make a report of their language that
// proves them, as one measured anew would.
func TestFromLinesProvesTheCachedLines(t *testing.T) {
	file := "/p/a.py"
	r := FromLines("python", map[string][]Line{file: {{Line: 2, Covered: true}, {Line: 5}}})
	if !r.Measures("python") || r.Measures("go") {
		t.Error("the report measures another language than its lines'")
	}
	if lines, ok := r.Lines(file); !ok || !reflect.DeepEqual(lines, []Line{{Line: 2, Covered: true}, {Line: 5}}) {
		t.Errorf("lines %v %v, want the cached ones, proven", lines, ok)
	}
	if got := fraction(t, r, file, 1, 5); !approx(got, 0.5) {
		t.Errorf("fraction %v, want 1/2", got)
	}
}
