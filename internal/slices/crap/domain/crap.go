// Package domain is the crap command's domain, scoring functions by the
// CRAP formula: complexity that tests do not exercise is risk. It is the
// crap slice's own (ADR-0051), kept apart from its CLI and application
// code, and a domain (decision 37).
//
//	CRAP = CC² × (1 − coverage)³ + CC
//
// It takes files internal/lang parsed, with the names its caller gives
// them, and a coverage Report, and reads nothing, so it has no port.
package domain

import (
	"math"
	"sort"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/lang"
)

// Entry is one function's score. Coverage and CRAP are nil when no coverage
// applies to it.
type Entry struct {
	Namespace  string   `json:"namespace"`
	Name       string   `json:"name"`
	Language   string   `json:"language"`
	File       string   `json:"file"`
	StartLine  int      `json:"start_line"`
	EndLine    int      `json:"end_line"`
	Complexity int      `json:"complexity"`
	Coverage   *float64 `json:"coverage"` // percent, 0–100
	CRAP       *float64 `json:"crap"`
}

// Score is the CRAP formula with coverage as a fraction from 0 to 1.
func Score(cc int, coverage float64) float64 {
	c, u := float64(cc), 1-coverage
	return c*c*(u*u*u) + c
}

// Measured is the languages of files, the paths a run scores, whose
// coverage report speaks for: a language is measured when the report names
// one of its files, or its coverage run succeeded and wrote its report
// though it names none (Report.Measures). Either way, a file of it the
// report never names is untested. A path of no language lang knows has
// none.
func Measured(report *coverage.Report, files []string) map[string]bool {
	measured := map[string]bool{}
	for _, f := range files {
		spec := lang.Detect(f)
		if spec != nil && (report.Has(f) || report.Measures(spec.Name)) {
			measured[spec.Name] = true
		}
	}
	return measured
}

// Analyze scores every unit of f, the file its caller names name, by
// report. With a nil report every coverage is nil. A file the report never
// mentions counts as 0% covered when its language is measured (Measured,
// over every file the run scores), because the tests ran and never loaded
// it; otherwise its coverage is unknown. The caller parsed f and closes it.
func Analyze(f *lang.File, name string, report *coverage.Report, measured map[string]bool) []Entry {
	var entries []Entry
	for _, u := range f.Units {
		e := Entry{
			Namespace:  u.Namespace,
			Name:       u.Name,
			Language:   u.Language,
			File:       name,
			StartLine:  u.StartLine,
			EndLine:    u.EndLine,
			Complexity: f.Complexity(u),
		}
		if cov, ok := unitCoverage(report, measured, f, u); ok {
			pct := round(cov*100, 1)
			score := round(Score(e.Complexity, cov), 1)
			e.Coverage, e.CRAP = &pct, &score
		}
		entries = append(entries, e)
	}
	return entries
}

func unitCoverage(r *coverage.Report, measured map[string]bool, f *lang.File, u lang.Unit) (float64, bool) {
	if r == nil {
		return 0, false
	}
	if !r.Has(f.Path) {
		return 0, measured[u.Language]
	}
	return r.Fraction(f.Path, u.BodyLine, u.EndLine, innerLines(f, u)...)
}

// innerLines are the lines of u's inline units, which their own entries
// measure. The line an inline unit starts on stays u's: it runs when u does.
func innerLines(f *lang.File, u lang.Unit) [][2]int {
	var out [][2]int
	for _, i := range u.Inner {
		in := f.Units[i]
		out = append(out, [2]int{max(in.BodyLine, in.StartLine+1), in.EndLine})
	}
	return out
}

// Worst sorts entries by CRAP, highest first; unscored entries sort last by
// complexity.
func Worst(entries []Entry) []Entry {
	out := append([]Entry(nil), entries...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case a.CRAP != nil && b.CRAP != nil && *a.CRAP != *b.CRAP:
			return *a.CRAP > *b.CRAP
		case (a.CRAP == nil) != (b.CRAP == nil):
			return a.CRAP != nil
		}
		return a.Complexity > b.Complexity
	})
	return out
}

func round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}
