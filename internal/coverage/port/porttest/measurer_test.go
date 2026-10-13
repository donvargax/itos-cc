package porttest

import (
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/coverage/port"
)

// The fake keeps the contract every Measurer keeps, internal/coveragetools'
// included. Its directory is a name only: the fake never touches it.
func TestMeasurerKeepsTheContract(t *testing.T) {
	MeasurerContract(t, func(t *testing.T, files map[string]string) (port.Measurer[*coverage.Report], string) {
		dir := t.TempDir()
		held := map[string]string{}
		for name, text := range files {
			held[filepath.FromSlash(name)] = text
		}
		return Measurer{Dir: dir, Files: held}, dir
	})
}

// A request that names no report, running the tests, is given the reports
// of the roots nearest its sources, merged, a root nested in another
// measuring its own; reading what an earlier run left, those of Left; a
// root nearest to none of them gives nothing.
func TestMeasurerGivesTheReportsOfTheSourcesRoots(t *testing.T) {
	report := func(language, dir string, gap port.Gap) *coverage.Report {
		r := &coverage.Report{}
		r.MarkMeasured(language)
		r.SetMissing(coverage.Unmeasured{Dir: dir, Language: language, Cause: gap, Reason: "no " + language})
		return r
	}
	m := Measurer{Dir: "/work", Roots: map[string]*coverage.Report{
		".": report("go", "/work", port.MeasuredNothing{}), "web": report("typescript", "/work/web", port.ToolMissing{}),
		"py": report("python", "/work/py", port.ToolMissing{}),
	}, Left: map[string]*coverage.Report{".": report("go", "/work", port.NoReport{})}}
	r, failure := m.Measure(t.Context(), port.Request{Sources: []string{"/work/a.go", "web/src/app.ts"}})
	if failure != nil {
		t.Fatalf("Measure() failed: %v", failure)
	}
	if !r.Measures("go") || !r.Measures("typescript") || r.Measures("python") {
		t.Errorf("measures go %t, typescript %t, python %t; want the two roots nearest a source",
			r.Measures("go"), r.Measures("typescript"), r.Measures("python"))
	}
	if got := r.Missing(); len(got) != 2 || got[0].Language != "go" || got[1].Language != "typescript" {
		t.Errorf("Missing() = %+v, want the root's then web's, in the roots' order", got)
	}
	r, _ = m.Measure(t.Context(), port.Request{Sources: []string{"/work/web/src/app.ts"}})
	if got := r.Missing(); len(got) != 1 || got[0].Language != "typescript" {
		t.Errorf("a source of web alone: Missing() = %+v, want web's alone, the nearest", got)
	}
	r, _ = m.Measure(t.Context(), port.Request{Sources: []string{"/work/a.go", "/work/web/app.ts"}, Existing: true})
	if got := r.Missing(); len(got) != 1 || got[0].Cause != (port.NoReport{}) {
		t.Errorf("existing: Missing() = %+v, want what the root left", got)
	}
	if r, failure := (Measurer{Dir: "/work"}).Measure(t.Context(), port.Request{Sources: []string{"/work/a.go"}}); failure != nil || !r.Empty() || len(r.Missing()) != 0 {
		t.Errorf("Measure() with no roots = %+v, %v; want an empty report", r, failure)
	}
}
