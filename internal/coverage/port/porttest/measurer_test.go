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

// A request that names no report, running the tests or reading what they
// left, is given the reports of every root holding one of its sources,
// merged; a root holding none of them gives nothing.
func TestMeasurerGivesTheReportsOfTheSourcesRoots(t *testing.T) {
	report := func(language, dir string) *coverage.Report {
		r := &coverage.Report{}
		r.MarkMeasured(language)
		r.SetMissing(coverage.Unmeasured{Dir: dir, Language: language, Cause: port.ToolMissing{}, Reason: "no " + language})
		return r
	}
	m := Measurer{Dir: "/work", Roots: map[string]*coverage.Report{
		"web": report("typescript", "/work/web"), "/work/svc": report("go", "/work/svc"), "py": report("python", "/work/py"),
	}}
	for _, existing := range []bool{false, true} {
		r, failure := m.Measure(t.Context(), port.Request{Sources: []string{"/work/svc/a.go", "web/src/app.ts"}, Existing: existing})
		if failure != nil {
			t.Fatalf("Measure() failed: %v", failure)
		}
		if !r.Measures("go") || !r.Measures("typescript") || r.Measures("python") {
			t.Errorf("existing %t: measures go %t, typescript %t, python %t; want the two roots holding a source",
				existing, r.Measures("go"), r.Measures("typescript"), r.Measures("python"))
		}
		if got := r.Missing(); len(got) != 2 || got[0].Language != "go" || got[1].Language != "typescript" {
			t.Errorf("existing %t: Missing() = %+v, want svc's then web's, in the roots' order", existing, got)
		}
	}
	if r, failure := (Measurer{Dir: "/work"}).Measure(t.Context(), port.Request{Sources: []string{"/work/a.go"}}); failure != nil || !r.Empty() || len(r.Missing()) != 0 {
		t.Errorf("Measure() with no roots = %+v, %v; want an empty report", r, failure)
	}
}
