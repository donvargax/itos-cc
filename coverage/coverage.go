// Package coverage loads coverage reports in the formats each language's
// tools write and answers how much of a line range the tests executed: how
// many of its branches were taken where the report counts branches, and how
// many of its lines or statements ran otherwise.
package coverage

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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
	// from holds a bit for each Source whose data covered the segment.
	from uint8
}

// GoBlock is one positive-weight executable block in a Go cover profile.
// Span retains the complete measured line/column identity; Line is the
// current source line used for diagnostics.
type GoBlock struct {
	Span      string
	Line      int
	Column    int
	EndLine   int
	EndColumn int
	Weight    float64
	Covered   bool
}

// Entry is one file as a report names it, before it is matched to a source
// file on disk.
type Entry struct {
	Path     string
	Segments []Segment
	Branches []Segment
	// Source is where the report's data came from.
	Source Source
}

// Source is where coverage data came from.
type Source int

const (
	// InProcess is a report's own data: what the tests executed in their own
	// processes, or whatever a report named with --coverage-report holds.
	InProcess Source = iota
	// Integration is what Go binaries built with go build -cover wrote to
	// GOCOVERDIR while the tests ran them.
	Integration
)

var sourceNames = []string{"in-process", "integration"}

func (s Source) String() string { return sourceNames[s] }

// Report is coverage keyed by absolute source path.
type Report struct {
	files    map[string][]Segment
	branches map[string][]Segment
	missing  []Unmeasured
	// languages are those a coverage command measured: every command of its
	// plan exited 0 and every report it was to write was written and read.
	languages map[string]bool
	// tests is the coverage of each listed test, or nil.
	tests *TestCoverage
	// proven is each file whose executable lines the report lists
	// completely, at its format's line precision (Lines): a plan that
	// measured its language named it, or a static inventory listed it.
	proven map[string]bool
	// unloaded is each source of a plan that measured its language which
	// the report never names, as no test loaded it, with its plan's
	// language and build root: what Inventory lists.
	unloaded map[string]Unloaded
	// inventory is the executable lines a static inventory listed of
	// unloaded files, none of them executed.
	inventory map[string][]Segment
}

// Unloaded is a source of a measured language no test loaded.
type Unloaded struct {
	Language, Dir string
}

// Line is one executable line of a file at a line-precision format's
// precision: an LCOV DA line, a JaCoCo or Kover line. Covered is true when
// any measured copy of it executed: a JaCoCo line with both missed and
// covered instructions is covered.
type Line struct {
	Line    int
	Covered bool
}

// Cause is why coverage that was meant to be measured was not.
type Cause string

const (
	// ToolMissing: the project lacks a tool coverage needs, such as Vitest.
	ToolMissing Cause = "tool-missing"
	// MeasuredNothing: the coverage run ran and measured none of the files,
	// as when the tests do not compile.
	MeasuredNothing Cause = "measured-nothing"
	// NoReport: --use-existing-coverage found no report measuring the files.
	NoReport Cause = "no-report"
	// Unreadable: a report named on the command line cannot be read.
	Unreadable Cause = "unreadable"
)

// Unmeasured is a build root or report whose coverage was not measured.
type Unmeasured struct {
	Dir      string `json:"dir,omitempty"`
	Language string `json:"language,omitempty"`
	Report   string `json:"report,omitempty"`
	Cause    Cause  `json:"cause"`
	Reason   string `json:"reason"`
}

func (u Unmeasured) String() string {
	switch {
	case u.Report != "":
		return u.Report + ": " + u.Reason
	case u.Language != "":
		return fmt.Sprintf("%s (%s): %s", u.Dir, u.Language, u.Reason)
	}
	return u.Dir + ": " + u.Reason
}

// Missing is what was meant to be measured and was not. Its functions have
// no coverage, which is not the same as untested.
func (r *Report) Missing() []Unmeasured {
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

// Measures reports whether a coverage command ran successfully for language:
// every command exited 0 and wrote its report. A file of that language the
// report never names was then loaded by no test, even when the report names
// no file at all, as Vitest's is when no test imports the files it was given.
// A failed command, or one that wrote no report, measures nothing.
func (r *Report) Measures(language string) bool {
	return r != nil && r.languages[language]
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

// LineSources names the sources whose data executed a segment covering
// line, in the order of Source: "in-process", "integration", or both. It is
// empty when no segment covering line was executed.
func (r *Report) LineSources(file string, line int) []string {
	if r == nil {
		return nil
	}
	var from uint8
	for _, s := range r.files[file] {
		if line >= s.Start && line <= s.End && s.Covered > 0 {
			from |= s.from
		}
	}
	var out []string
	for i, name := range sourceNames {
		if from&(1<<i) != 0 {
			out = append(out, name)
		}
	}
	return out
}

// GoBlocks returns the union of measured Go statement spans for file across
// the primary report and any separately-attached listed-test measurements.
// A span is covered when any successful measurement covered that exact span.
func (r *Report) GoBlocks(file string) []GoBlock {
	if r == nil {
		return nil
	}
	bySpan := map[string]GoBlock{}
	add := func(report *Report) {
		if report == nil {
			return
		}
		for _, seg := range report.files[file] {
			startLine, startColumn, endLine, endColumn, ok := goSpanColumns(seg.Key)
			if !ok || seg.Total <= 0 {
				continue
			}
			block := bySpan[seg.Key]
			block.Span, block.Line, block.Column = seg.Key, startLine, startColumn
			block.EndLine, block.EndColumn = endLine, endColumn
			block.Weight = max(block.Weight, seg.Total)
			block.Covered = block.Covered || seg.Covered > 0
			bySpan[seg.Key] = block
		}
	}
	add(r)
	if r.tests != nil {
		for _, id := range r.tests.ids {
			add(r.tests.reports[id])
		}
	}
	out := make([]GoBlock, 0, len(bySpan))
	for _, block := range bySpan {
		out = append(out, block)
	}
	slices.SortFunc(out, func(a, b GoBlock) int {
		if a.Line != b.Line {
			return a.Line - b.Line
		}
		if a.Column != b.Column {
			return a.Column - b.Column
		}
		return strings.Compare(a.Span, b.Span)
	})
	return out
}

// Lines is the complete executable line inventory of file at its format's
// line precision, in line order, and whether the report proves it
// complete: a coverage command of its plan succeeded and its report named
// the file, or a static inventory listed the file no test loaded, every
// line then uncovered. Lines of the listed tests' measurements count as
// GoBlocks counts them. Without proof it returns false, whatever partial
// lines a failed command left: those are not evidence of what is
// executable.
func (r *Report) Lines(file string) ([]Line, bool) {
	if r == nil || !r.proven[file] {
		return nil, false
	}
	covered := map[int]bool{}
	add := func(segments []Segment) {
		for _, seg := range segments {
			if seg.Total <= 0 {
				continue
			}
			for line := seg.Start; line <= seg.End; line++ {
				covered[line] = covered[line] || seg.Covered > 0
			}
		}
	}
	add(r.files[file])
	add(r.inventory[file])
	if r.tests != nil {
		for _, id := range r.tests.ids {
			if report := r.tests.reports[id]; report != nil {
				add(report.files[file])
			}
		}
	}
	out := make([]Line, 0, len(covered))
	for line, c := range covered {
		out = append(out, Line{Line: line, Covered: c})
	}
	slices.SortFunc(out, func(a, b Line) int { return a.Line - b.Line })
	return out, true
}

// Unloaded is each source of a plan that measured language which no test
// loaded, so the report names none of its lines, sorted: what Inventory
// lists.
func (r *Report) Unloaded(language string) []string {
	if r == nil {
		return nil
	}
	var out []string
	for file, u := range r.unloaded {
		if u.Language == language && !r.proven[file] {
			out = append(out, file)
		}
	}
	slices.Sort(out)
	return out
}

// FromLines makes a report of language from previously complete line
// inventories, as FromGoBlocks does of Go blocks: every file proven.
func FromLines(language string, lines map[string][]Line) *Report {
	r := &Report{files: map[string][]Segment{}, branches: map[string][]Segment{}, languages: map[string]bool{language: true},
		proven: map[string]bool{}}
	for file, entries := range lines {
		r.proven[file] = true
		r.files[file] = []Segment{}
		for _, line := range entries {
			r.files[file] = append(r.files[file], Segment{Start: line.Line, End: line.Line, Total: 1,
				Covered: boolWeight(line.Covered, 1), Key: strconv.Itoa(line.Line)})
		}
	}
	return r
}

// FromGoBlocks makes a report from a previously complete Go block inventory.
// It is used only to classify mutation sites when strict cached evidence is
// fresh; it does not claim to be a newly measured producer.
func FromGoBlocks(blocks map[string][]GoBlock) *Report {
	r := &Report{files: map[string][]Segment{}, branches: map[string][]Segment{}, languages: map[string]bool{"go": true}}
	for file, entries := range blocks {
		for _, block := range entries {
			if block.Weight <= 0 {
				continue
			}
			covered := 0.0
			if block.Covered {
				covered = block.Weight
			}
			r.files[file] = append(r.files[file], Segment{Start: block.Line, End: block.Line, Total: block.Weight, Covered: covered, Key: block.Span})
		}
	}
	return r
}

func goSpanColumns(span string) (startLine, startColumn, endLine, endColumn int, ok bool) {
	from, to, found := strings.Cut(span, ",")
	if !found {
		return 0, 0, 0, 0, false
	}
	parse := func(pos string) (int, int, bool) {
		line, column, found := strings.Cut(pos, ".")
		if !found {
			return 0, 0, false
		}
		l, e1 := strconv.Atoi(line)
		c, e2 := strconv.Atoi(column)
		return l, c, e1 == nil && e2 == nil && l > 0 && c > 0
	}
	startLine, startColumn, ok1 := parse(from)
	endLine, endColumn, ok2 := parse(to)
	return startLine, startColumn, endLine, endColumn, ok1 && ok2
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
				r.files[file] = merged(r.files[file], e.Segments, e.Source, file+"\x00lines", seen)
				r.branches[file] = merged(r.branches[file], e.Branches, e.Source, file+"\x00branches", seen)
			}
		}
	}
	return r
}

// merged appends more, from src, to segs, folding a segment whose key segs
// already holds into that one: covered when either is, by the sources of
// both.
func merged(segs, more []Segment, src Source, prefix string, seen map[string]int) []Segment {
	for _, s := range more {
		if s.Covered > 0 {
			s.from |= 1 << src
		}
		if s.Key == "" {
			segs = append(segs, s)
			continue
		}
		key := prefix + "\x00" + s.Key
		if i, ok := seen[key]; ok {
			segs[i].Covered = max(segs[i].Covered, s.Covered)
			segs[i].from |= s.from
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
		if out.tests == nil {
			out.tests = r.tests
		}
		for l := range r.languages {
			if out.languages == nil {
				out.languages = map[string]bool{}
			}
			out.languages[l] = true
		}
		for f, segs := range r.files {
			out.files[f] = append(out.files[f], segs...)
		}
		for f, segs := range r.branches {
			out.branches[f] = append(out.branches[f], segs...)
		}
		for f := range r.proven {
			if out.proven == nil {
				out.proven = map[string]bool{}
			}
			out.proven[f] = true
		}
		for f, u := range r.unloaded {
			if out.unloaded == nil {
				out.unloaded = map[string]Unloaded{}
			}
			out.unloaded[f] = u
		}
		for f, segs := range r.inventory {
			if out.inventory == nil {
				out.inventory = map[string][]Segment{}
			}
			out.inventory[f] = append(out.inventory[f], segs...)
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
