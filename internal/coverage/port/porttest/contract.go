package porttest

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/coverage/port"
)

// FilesContract holds a Files to what every one, the fake and the disk's,
// must do: a regular file is the first of among that is the same file, by
// a path relative to the working directory, absolute or not clean, or no
// source when none of among is; a path that is not there, or a directory,
// is no regular file. open lays out files, by slash-separated paths
// relative to a directory, and returns the Files over them and that
// directory, absolute, the working directory; it may make that the test's,
// so no case runs in parallel. No case has a done context: the port's
// SameFile does not act on it. What only a disk can do (a symlinked
// directory, permissions) is the adapter's own tests'; the fake's links,
// the fake's.
func FilesContract(t *testing.T, open func(t *testing.T, files []string) (port.Files, string)) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, open func(t *testing.T, files []string) (port.Files, string))
	}{
		{"a regular file is the source among that names it, however its path is written", namesTheSource},
		{"the first of among that is the file is the one named", namesTheFirst},
		{"a regular file none of among names is no source", namesNoOtherFile},
		{"a path that is not there, or a directory, is no regular file", refusesWhatIsNoFile},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { c.run(t, open) })
	}
}

// sources are two files of one name in different directories, and one of
// another language.
var sources = []string{"src/a.go", "src/b/a.go", "lib/x.py"}

func namesTheSource(t *testing.T, open func(t *testing.T, files []string) (port.Files, string)) {
	files, dir := open(t, sources)
	abs := filepath.Join(dir, "src", "a.go")
	other := filepath.Join(dir, "src", "b", "a.go")
	for _, c := range []struct {
		path  string
		among []string
		want  string
	}{
		{"src/a.go", []string{other, abs}, abs},
		{filepath.Join("src", "a.go"), []string{abs}, abs},
		{abs, []string{other, filepath.Join("src", "a.go")}, filepath.Join("src", "a.go")},
		{filepath.Join(dir, "src", "b", "..", "a.go"), []string{other, abs}, abs},
		{"src/b/a.go", []string{abs, other}, other},
		{"lib/x.py", []string{filepath.Join(dir, "lib", "x.py")}, filepath.Join(dir, "lib", "x.py")},
	} {
		if same, regular := files.SameFile(t.Context(), c.path, c.among); same != c.want || !regular {
			t.Errorf("SameFile(%q, %q) = %q, %t; want %q, true", c.path, c.among, same, regular, c.want)
		}
	}
}

func namesTheFirst(t *testing.T, open func(t *testing.T, files []string) (port.Files, string)) {
	files, dir := open(t, sources)
	abs := filepath.Join(dir, "src", "a.go")
	among := []string{filepath.Join(dir, "absent", "a.go"), "src/a.go", abs}
	if same, regular := files.SameFile(t.Context(), abs, among); same != "src/a.go" || !regular {
		t.Errorf("SameFile(%q, %q) = %q, %t; want src/a.go, true: the first that is the file", abs, among, same, regular)
	}
}

func namesNoOtherFile(t *testing.T, open func(t *testing.T, files []string) (port.Files, string)) {
	files, dir := open(t, sources)
	for _, among := range [][]string{nil, {filepath.Join(dir, "src", "a.go")}, {filepath.Join(dir, "absent.go")}} {
		if same, regular := files.SameFile(t.Context(), "src/b/a.go", among); same != "" || !regular {
			t.Errorf("SameFile(src/b/a.go, %q) = %q, %t; want no source of a regular file", among, same, regular)
		}
	}
}

func refusesWhatIsNoFile(t *testing.T, open func(t *testing.T, files []string) (port.Files, string)) {
	files, dir := open(t, sources)
	for _, path := range []string{"absent.go", filepath.Join(dir, "src", "absent.go"), "src", "src/b", dir, ".", "src/a.go/x", "a.go"} {
		among := []string{filepath.Join(dir, "src", "a.go"), path}
		if same, regular := files.SameFile(t.Context(), path, among); same != "" || regular {
			t.Errorf("SameFile(%q, %q) = %q, %t; want no regular file", path, among, same, regular)
		}
	}
}

// opener lays out files, their text by slash-separated path relative to a
// directory, and returns the Measurer over them and that directory,
// absolute, the working directory; it may make that the test's.
type opener = func(t *testing.T, files map[string]string) (port.Measurer[*coverage.Report], string)

// MeasurerContract holds a Measurer to what every one, the fake and
// internal/coveragetools', must do with the reports a request names: each
// format read for the lines it covers, a report's paths matched to the
// sources however they are written, reports naming the same file merged,
// a report that cannot be read Missing as Unreadable while the rest are
// read, one a command was to write and did not Missing as MeasuredNothing,
// and a done context Cancelled with no report. open lays out the files, the
// sources and the reports among them. The command is one every platform's
// shell runs and that writes nothing. Running the project's own tools, and
// reading what they left, is the adapter's own tests'; the reports a fake
// is given for them, the fake's.
func MeasurerContract(t *testing.T, open opener) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, open opener)
	}{
		{"each format names the lines it covers", readsEachFormat},
		{"a report's paths name the sources however they are written", matchesTheSources},
		{"reports naming the same file merge, a piece they both name counted once", mergesTheReports},
		{"a report that cannot be read is unreadable, and the others are read", missesWhatCannotBeRead},
		{"a report the command did not write measured nothing", missesWhatTheCommandDidNotWrite},
		{"a done context is cancelled and gives no report", aDoneContextMeasuresNothing},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { c.run(t, open) })
	}
}

// measure is m's report of reports, the sources by slash-separated path
// relative to dir, made absolute as the selection discovers them; a
// failure fails t.
func measure(t *testing.T, m port.Measurer[*coverage.Report], dir string, req port.Request, sources ...string) *coverage.Report {
	t.Helper()
	for _, s := range sources {
		req.Sources = append(req.Sources, filepath.Join(dir, filepath.FromSlash(s)))
	}
	r, failure := m.Measure(t.Context(), req)
	if failure != nil || r == nil {
		t.Fatalf("Measure(%+v) = %v, %v; want a report", req, r, failure)
	}
	return r
}

// covered fails t unless r says of each line of the source at path,
// relative to dir, whether it was covered, as want says.
func covered(t *testing.T, r *coverage.Report, dir, path string, want map[int]bool) {
	t.Helper()
	file := filepath.Join(dir, filepath.FromSlash(path))
	for line, c := range want {
		if got, measured := r.LineCovered(file, line); got != c || !measured {
			t.Errorf("%s:%d covered %t, measured %t; want covered %t", path, line, got, measured, c)
		}
	}
}

// fraction fails t unless r gives the lines start to end of the source at
// path, relative to dir, the covered share want.
func fraction(t *testing.T, r *coverage.Report, dir, path string, start, end int, want float64) {
	t.Helper()
	if got, ok := r.Fraction(filepath.Join(dir, filepath.FromSlash(path)), start, end); got != want || !ok {
		t.Errorf("%s:%d-%d covered %v, %t; want %v", path, start, end, got, ok, want)
	}
}

// missing fails t unless r's Missing is the reports order names, in that
// order, each with the gap want gives it and a reason.
func missing(t *testing.T, r *coverage.Report, want map[string]port.Gap, order ...string) {
	t.Helper()
	var reports []string
	for _, m := range r.Missing() {
		reports = append(reports, m.Report)
		if m.Cause != want[m.Report] || m.Reason == "" || m.Dir != "" || m.Language != "" {
			t.Errorf("missing %+v; want report %s %v, with a reason", m, m.Report, want[m.Report])
		}
	}
	if !slices.Equal(reports, order) {
		t.Errorf("missing reports %q, want %q", reports, order)
	}
}

func readsEachFormat(t *testing.T, open opener) {
	m, dir := open(t, map[string]string{
		"src/a.go": "package a\n", "src/b/a.go": "package b\n", "web/app.ts": "export {}\n",
		"kt/src/main/kotlin/com/example/Board.kt": "package com.example\n",
		"go.out":    "mode: set\nexample.com/m/src/a.go:1.1,2.2 1 1\nexample.com/m/src/a.go:3.1,4.2 1 0\n",
		"lcov.info": "SF:web/app.ts\nDA:1,1\nDA:2,0\nBRDA:3,0,0,1\nBRDA:3,0,1,0\nend_of_record\n",
		"reports/jacoco.xml": `<?xml version="1.0"?><report name="r"><package name="com/example"><sourcefile name="Board.kt">` +
			`<line nr="5" mi="0" ci="3" mb="0" cb="0"/><line nr="6" mi="2" ci="0" mb="1" cb="1"/></sourcefile></package></report>`,
	})
	r := measure(t, m, dir, port.Request{Reports: []string{"go.out", "lcov.info", filepath.Join("reports", "jacoco.xml")}},
		"src/a.go", "src/b/a.go", "web/app.ts", "kt/src/main/kotlin/com/example/Board.kt")
	missing(t, r, nil)
	covered(t, r, dir, "src/a.go", map[int]bool{1: true, 3: false})
	if r.Has(filepath.Join(dir, "src", "b", "a.go")) {
		t.Error("the Go profile names src/a.go alone, and src/b/a.go has coverage")
	}
	covered(t, r, dir, "web/app.ts", map[int]bool{1: true, 2: false})
	fraction(t, r, dir, "web/app.ts", 3, 3, 0.5)
	covered(t, r, dir, "kt/src/main/kotlin/com/example/Board.kt", map[int]bool{5: true})
	fraction(t, r, dir, "kt/src/main/kotlin/com/example/Board.kt", 6, 6, 0.5)
}

func matchesTheSources(t *testing.T, open opener) {
	m, dir := open(t, map[string]string{"src/a.go": "", "src/b/a.go": "", "b/x.go": "", "lib/b/x.go": "",
		"paths.info": "SF:example.com/m/src/b/a.go\nDA:1,1\nend_of_record\n" +
			"SF:src\\a.go\nDA:2,1\nend_of_record\n" +
			"SF:lib/b/x.go\nDA:3,1\nend_of_record\n" +
			"SF:other/a.go\nDA:4,1\nend_of_record\n" +
			"SF:vendor/other.go\nDA:5,1\nend_of_record\n"})
	r := measure(t, m, dir, port.Request{Reports: []string{filepath.Join(dir, "paths.info")}}, "src/a.go", "src/b/a.go", "b/x.go")
	covered(t, r, dir, "src/b/a.go", map[int]bool{1: true})
	covered(t, r, dir, "src/a.go", map[int]bool{2: true})
	if r.Has(filepath.Join(dir, "b", "x.go")) {
		t.Error("lib/b/x.go, a file the run does not score, lent its coverage to b/x.go")
	}
	for _, source := range []string{"src/a.go", "src/b/a.go"} {
		if _, measured := r.LineCovered(filepath.Join(dir, filepath.FromSlash(source)), 4); measured {
			t.Errorf("other/a.go, whose tail fits two sources alike, lent its coverage to %s", source)
		}
	}
}

func mergesTheReports(t *testing.T, open opener) {
	m, dir := open(t, map[string]string{
		"src/a.ts": "", "src/c.go": "",
		"one.info": "SF:src/a.ts\nDA:1,1\nDA:2,0\nend_of_record\n",
		"two.info": "SF:src/a.ts\nDA:1,0\nDA:2,1\nend_of_record\n",
		"c.out":    "mode: set\nsrc/c.go:1.1,2.2 2 0\nsrc/c.go:1.1,2.2 2 1\nsrc/c.go:3.1,3.5 2 0\n",
	})
	r := measure(t, m, dir, port.Request{Reports: []string{"one.info", "two.info", "c.out"}}, "src/a.ts", "src/c.go")
	covered(t, r, dir, "src/a.ts", map[int]bool{1: true, 2: true})
	fraction(t, r, dir, "src/a.ts", 1, 2, 1)
	fraction(t, r, dir, "src/c.go", 1, 3, 0.5)
}

func missesWhatCannotBeRead(t *testing.T, open opener) {
	m, dir := open(t, map[string]string{
		"src/a.ts": "", "good.info": "SF:src/a.ts\nDA:1,1\nend_of_record\n", "bad.info": "SF:src/a.ts\nDA:x,1\nend_of_record\n",
	})
	r := measure(t, m, dir, port.Request{Reports: []string{"absent.info", "good.info", "bad.info", "src"}}, "src/a.ts")
	unreadable := port.Unreadable{}
	missing(t, r, map[string]port.Gap{"absent.info": unreadable, "bad.info": unreadable, "src": unreadable}, "absent.info", "bad.info", "src")
	covered(t, r, dir, "src/a.ts", map[int]bool{1: true})
}

func missesWhatTheCommandDidNotWrite(t *testing.T, open opener) {
	m, dir := open(t, map[string]string{"src/a.ts": "", "good.info": "SF:src/a.ts\nDA:1,1\nend_of_record\n"})
	r := measure(t, m, dir, port.Request{Command: "exit 0", Reports: []string{"absent.info", "good.info"}}, "src/a.ts")
	missing(t, r, map[string]port.Gap{"absent.info": port.MeasuredNothing{}}, "absent.info")
	if why := r.Missing()[0].Reason; !strings.HasPrefix(why, "--coverage-command wrote no readable report: ") {
		t.Errorf("the reason is %q, want it to say the command wrote no readable report", why)
	}
	covered(t, r, dir, "src/a.ts", map[int]bool{1: true})
}

func aDoneContextMeasuresNothing(t *testing.T, open opener) {
	m, dir := open(t, map[string]string{"src/a.ts": "", "good.info": "SF:src/a.ts\nDA:1,1\nend_of_record\n"})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	source := []string{filepath.Join(dir, "src", "a.ts")}
	for _, req := range []port.Request{{Sources: source, Reports: []string{"good.info"}}, {Sources: source}} {
		if r, failure := m.Measure(ctx, req); r != nil || failure != (port.Cancelled{}) {
			t.Errorf("Measure(%+v) on a done context = %v, %v; want no report, cancelled", req, r, failure)
		}
	}
}
