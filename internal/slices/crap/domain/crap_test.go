package domain

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
	covtest "github.com/donvargax/itos-cc/internal/coverage/port/porttest"
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/lang/port/porttest"
)

// parse is path's text parsed in memory through lang's fake, as cmd parses
// a selected file through the selection domain.
func parse(t *testing.T, path, src string) *lang.File {
	t.Helper()
	f, err := lang.Parse(t.Context(), porttest.Surroundings{}, lang.Detect(path), path, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.Close)
	return f
}

// report is the LCOV text's coverage of sources, matched by what its paths
// say.
func report(t *testing.T, sources []string, text string) *coverage.Report {
	t.Helper()
	entries, err := coverage.ParseLCOV(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	return coverage.Build(t.Context(), covtest.Files{}, sources, "/p", entries)
}

// A route's lines count toward the route alone. mount ran when the module
// loaded, so it is fully covered even though no request reached its route.
func TestRouteLinesAreNotTheEnclosingFunctions(t *testing.T) {
	path := "/p/routes.ts"
	f := parse(t, path, `export function mount(app) {
  const limit = 1;
  app.get("/x", (req, res) => {
    if (req.q > limit) { res.json(1); }
  });
}
`)
	r := report(t, []string{path}, "SF:"+path+"\nDA:2,1\nDA:3,1\nDA:4,0\nDA:5,1\nend_of_record\n")
	got := Analyze(f, "routes.ts", r, Measured(r, []string{path}))
	cov := map[string]float64{}
	for _, e := range got {
		cov[e.Name] = *e.Coverage
		if e.File != "routes.ts" {
			t.Errorf("%s is named %q, want the name its caller gave", e.Name, e.File)
		}
	}
	if cov["mount"] != 100 || cov["GET /x"] != 50 {
		t.Errorf("coverage = %v, want mount 100 and GET /x 50", cov)
	}
}

// A file the report never names is untested when its language is
// measured, by another of its files or by a run that wrote its report, and
// unknown otherwise; with no report nothing is scored.
func TestAFileTheReportNeverNamesIsUntestedOnlyWhereItsLanguageIsMeasured(t *testing.T) {
	a, b, c := "/p/a.py", "/p/b.py", "/p/c.go"
	files := []string{a, b, c, "/p/notes.txt"}
	r := report(t, files, "SF:a.py\nDA:1,1\nend_of_record\n")
	if got := Measured(r, files); !reflect.DeepEqual(got, map[string]bool{"python": true}) {
		t.Errorf("measured %v, want python alone", got)
	}
	src := "def f(x):\n    if x:\n        return 1\n    return 2\n"
	untested := Analyze(parse(t, b, src), "b.py", r, Measured(r, files))
	if len(untested) != 1 || untested[0].Coverage == nil || *untested[0].Coverage != 0 || *untested[0].CRAP != Score(2, 0) {
		t.Errorf("b's entries %+v, want f untested, scored at its complexity", untested)
	}
	goSrc := "package p\n\nfunc G() int { return 1 }\n"
	if unknown := Analyze(parse(t, c, goSrc), "c.go", r, Measured(r, files)); len(unknown) != 1 || unknown[0].Coverage != nil || unknown[0].CRAP != nil {
		t.Errorf("c's entries %+v, want G unscored: go was not measured", unknown)
	}
	r.MarkMeasured("go")
	if scored := Analyze(parse(t, c, goSrc), "c.go", r, Measured(r, files)); len(scored) != 1 || scored[0].Coverage == nil || *scored[0].Coverage != 0 {
		t.Errorf("c's entries %+v, want G untested: a go run wrote its report", scored)
	}
	if none := Analyze(parse(t, a, src), "a.py", nil, Measured(nil, files)); len(none) != 1 || none[0].Coverage != nil || none[0].Complexity != 2 {
		t.Errorf("a's entries with no report %+v, want f's complexity alone", none)
	}
}

// The worst come first: the highest CRAP, then the unscored by complexity.
func TestWorstSortsByCRAPThenComplexity(t *testing.T) {
	score := func(v float64) *float64 { return &v }
	entries := []Entry{
		{Name: "low", Complexity: 1, CRAP: score(1)},
		{Name: "unscored-simple", Complexity: 1},
		{Name: "high", Complexity: 3, CRAP: score(30)},
		{Name: "unscored-complex", Complexity: 5},
		{Name: "tie", Complexity: 4, CRAP: score(1)},
	}
	var names []string
	for _, e := range Worst(entries) {
		names = append(names, e.Name)
	}
	if want := []string{"high", "tie", "low", "unscored-complex", "unscored-simple"}; !reflect.DeepEqual(names, want) {
		t.Errorf("worst %v, want %v", names, want)
	}
	if entries[0].Name != "low" {
		t.Error("Worst sorted the caller's entries in place")
	}
	if got := Score(4, 0.5); math.Abs(got-6) > 1e-9 {
		t.Errorf("Score(4, 0.5) = %v, want 16 × 1/8 + 4 = 6", got)
	}
}
