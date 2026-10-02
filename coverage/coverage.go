// Package coverage loads coverage reports in the formats each language's
// tools write and answers how much of a line range the tests executed.
package coverage

import (
	"path/filepath"
	"strings"
)

// Segment is a run of lines that a report measures as one piece: one LCOV
// line, one Go statement block, or one JaCoCo line. Total and Covered are in
// the report's own unit (lines, statements, or instructions).
type Segment struct {
	Start, End     int
	Total, Covered float64
}

// Entry is one file as a report names it, before it is matched to a source
// file on disk.
type Entry struct {
	Path     string
	Segments []Segment
}

// Report is coverage keyed by absolute source path.
type Report struct {
	files map[string][]Segment
}

// Fraction returns the covered share of the segments that start inside
// [start, end] of file, and false when the report has nothing to say about
// those lines: the file is missing or no measured segment starts there.
func (r *Report) Fraction(file string, start, end int) (float64, bool) {
	if r == nil {
		return 0, false
	}
	segs, ok := r.files[file]
	if !ok {
		return 0, false
	}
	var total, covered float64
	for _, s := range segs {
		if s.Start >= start && s.Start <= end {
			total += s.Total
			covered += s.Covered
		}
	}
	if total == 0 {
		return 0, false
	}
	return covered / total, true
}

// Has reports whether the report measured file at all.
func (r *Report) Has(file string) bool {
	if r == nil {
		return false
	}
	_, ok := r.files[file]
	return ok
}

// LineCovered reports whether a segment covering line was executed, and
// false for both outcomes when the line was not measured.
func (r *Report) LineCovered(file string, line int) (covered, measured bool) {
	if r == nil {
		return false, false
	}
	for _, s := range r.files[file] {
		if line >= s.Start && line <= s.End {
			measured = true
			if s.Covered > 0 {
				return true, true
			}
		}
	}
	return false, measured
}

// Build matches each entry to one of sources and merges entries that name the
// same file. base is the directory the report's relative paths start from.
// Entries that match no source are dropped: they are dependencies, tests, or
// generated code.
func Build(sources []string, base string, entries ...[]Entry) *Report {
	m := newMatcher(sources)
	r := &Report{files: map[string][]Segment{}}
	for _, list := range entries {
		for _, e := range list {
			if file := m.match(e.Path, base); file != "" {
				r.files[file] = append(r.files[file], e.Segments...)
			}
		}
	}
	return r
}

// Merge combines reports for different languages into one.
func Merge(reports ...*Report) *Report {
	out := &Report{files: map[string][]Segment{}}
	for _, r := range reports {
		if r == nil {
			continue
		}
		for f, segs := range r.files {
			out.files[f] = append(out.files[f], segs...)
		}
	}
	return out
}

// matcher resolves a report path to a source file: exactly when the path is
// absolute or relative to base, otherwise by the longest run of trailing path
// components it shares with exactly one source. Report tools disagree on
// where paths start (module paths, package directories, the working
// directory), but the tail of the path is always the file.
type matcher struct {
	sources map[string]bool
	byName  map[string][]string
}

func newMatcher(sources []string) *matcher {
	m := &matcher{sources: map[string]bool{}, byName: map[string][]string{}}
	for _, s := range sources {
		m.sources[s] = true
		name := filepath.Base(s)
		m.byName[name] = append(m.byName[name], s)
	}
	return m
}

func (m *matcher) match(path, base string) string {
	path = filepath.FromSlash(strings.TrimPrefix(path, "file://"))
	if filepath.IsAbs(path) && m.sources[filepath.Clean(path)] {
		return filepath.Clean(path)
	}
	if joined := filepath.Join(base, path); m.sources[joined] {
		return joined
	}
	want := components(path)
	best, bestLen, tied := "", 0, false
	for _, candidate := range m.byName[filepath.Base(path)] {
		n := sharedTail(want, components(candidate))
		switch {
		case n > bestLen:
			best, bestLen, tied = candidate, n, false
		case n == bestLen:
			tied = true
		}
	}
	if tied {
		return ""
	}
	return best
}

func components(path string) []string {
	return strings.Split(filepath.ToSlash(filepath.Clean(path)), "/")
}

func sharedTail(a, b []string) int {
	n := 0
	for n < len(a) && n < len(b) && a[len(a)-1-n] == b[len(b)-1-n] {
		n++
	}
	return n
}
