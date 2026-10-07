// Package coverage loads coverage reports in the formats each language's
// tools write and answers how much of a line range the tests executed: how
// many of its branches were taken where the report counts branches, and how
// many of its lines or statements ran otherwise.
package coverage

import (
	"os"
	"path/filepath"
	"strings"
)

// Segment is a run of lines that a report measures as one piece: one LCOV
// line, one Go statement block, or one JaCoCo line. Total and Covered are in
// the report's own unit (lines, statements, or instructions). A branch
// segment is one decision, and counts its branches. Key names the piece
// within its file, so Build counts a piece that several test binaries or
// reports list once, as covered when any of them covered it. For a decision
// that is its most-taken copy's branches: block counts cannot tell whether
// two runs took the same branches or different ones.
type Segment struct {
	Start, End     int
	Total, Covered float64
	Key            string
}

// Entry is one file as a report names it, before it is matched to a source
// file on disk.
type Entry struct {
	Path     string
	Segments []Segment
	Branches []Segment
}

// Report is coverage keyed by absolute source path.
type Report struct {
	files    map[string][]Segment
	branches map[string][]Segment
	missing  []string
}

// Missing says, one line each, what was meant to be measured and was not: a
// build root whose coverage run wrote no report or whose tool is missing, or
// a report that could not be read. Its functions have no coverage, which is
// not the same as untested.
func (r *Report) Missing() []string {
	if r == nil {
		return nil
	}
	return r.missing
}

// Fraction returns the covered share of [start, end] of file, leaving out
// the skip ranges: the share of branches taken when the report counts
// branches there, and of lines or statements run otherwise. It returns false
// when the report has nothing to say about those lines: the file is missing
// or no measured segment starts there.
func (r *Report) Fraction(file string, start, end int, skip ...[2]int) (float64, bool) {
	if r == nil {
		return 0, false
	}
	segs, ok := r.files[file]
	if !ok {
		return 0, false
	}
	if f, ok := share(r.branches[file], start, end, skip); ok {
		return f, true
	}
	return share(segs, start, end, skip)
}

// share is the covered share of the segments that start inside [start, end]
// but not inside any of the skip ranges.
func share(segs []Segment, start, end int, skip [][2]int) (float64, bool) {
	var total, covered float64
	for _, s := range segs {
		if s.Start >= start && s.Start <= end && !inAny(s.Start, skip) {
			total += s.Total
			covered += s.Covered
		}
	}
	if total == 0 {
		return 0, false
	}
	return covered / total, true
}

func inAny(line int, ranges [][2]int) bool {
	for _, r := range ranges {
		if line >= r[0] && line <= r[1] {
			return true
		}
	}
	return false
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
// same file, and segments that name the same piece of it. base is the
// directory the report's relative paths start from. Entries that match no
// source are dropped: they are dependencies, tests, or generated code.
func Build(sources []string, base string, entries ...[]Entry) *Report {
	m := newMatcher(sources)
	r := &Report{files: map[string][]Segment{}, branches: map[string][]Segment{}}
	seen := map[string]int{} // file, kind, and key → index into its segments
	for _, list := range entries {
		for _, e := range list {
			if file := m.match(e.Path, base); file != "" {
				r.files[file] = merged(r.files[file], e.Segments, file+"\x00lines", seen)
				r.branches[file] = merged(r.branches[file], e.Branches, file+"\x00branches", seen)
			}
		}
	}
	return r
}

// merged appends more to segs, folding a segment whose key segs already
// holds into that one.
func merged(segs, more []Segment, prefix string, seen map[string]int) []Segment {
	for _, s := range more {
		if s.Key == "" {
			segs = append(segs, s)
			continue
		}
		key := prefix + "\x00" + s.Key
		if i, ok := seen[key]; ok {
			segs[i].Covered = max(segs[i].Covered, s.Covered)
			continue
		}
		seen[key] = len(segs)
		segs = append(segs, s)
	}
	return segs
}

// Merge combines reports for different languages into one.
func Merge(reports ...*Report) *Report {
	out := &Report{files: map[string][]Segment{}, branches: map[string][]Segment{}}
	for _, r := range reports {
		if r == nil {
			continue
		}
		out.missing = append(out.missing, r.missing...)
		for f, segs := range r.files {
			out.files[f] = append(out.files[f], segs...)
		}
		for f, segs := range r.branches {
			out.branches[f] = append(out.branches[f], segs...)
		}
	}
	return out
}

// matcher resolves a report path to a source file: exactly when the path is
// absolute or relative to base, otherwise by the longest run of trailing path
// components it shares with exactly one source. Report tools disagree on
// where paths start (module paths, package directories, the working
// directory), but the tail of the path is always the file. A path that names
// a file on disk, as written or by a tail under base, is that file, so when
// only some files are scored, a/b/x.go never lends its coverage to b/x.go.
type matcher struct {
	sources map[string]string // cleaned path → the path as the caller gave it
	byName  map[string][]string
}

func newMatcher(sources []string) *matcher {
	m := &matcher{sources: map[string]string{}, byName: map[string][]string{}}
	for _, s := range sources {
		m.sources[filepath.Clean(s)] = s
		name := filepath.Base(s)
		m.byName[name] = append(m.byName[name], s)
	}
	return m
}

func (m *matcher) match(path, base string) string {
	path = filepath.FromSlash(strings.TrimPrefix(path, "file://"))
	exact := filepath.Join(base, path)
	if filepath.IsAbs(path) {
		exact = filepath.Clean(path)
	}
	want := components(path)
	// The path, then each shorter tail of it under base: a module path or
	// another machine's checkout names the project file its longest tail
	// finds.
	for i := range want {
		candidate := exact
		if i > 0 {
			candidate = filepath.Join(append([]string{base}, want[i:]...)...)
		}
		if s, ok := m.sources[candidate]; ok {
			return s
		}
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return m.sameFile(info)
		}
	}
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

// sameFile is the source that info describes, reached through another path
// such as a symlinked directory's target, or "" when it is not a source.
func (m *matcher) sameFile(info os.FileInfo) string {
	for _, candidate := range m.byName[info.Name()] {
		if other, err := os.Stat(candidate); err == nil && os.SameFile(info, other) {
			return candidate
		}
	}
	return ""
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
