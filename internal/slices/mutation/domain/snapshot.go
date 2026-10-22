package domain

import (
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/snapshot"
)

// Outcomes of a mutant.
const (
	Killed    = "killed"
	Survived  = "survived"
	Timeout   = "timeout" // counts as killed: the tests noticed
	Uncovered = "uncovered"
)

// Snapshot is one source file's results, stored at
// .metrics/mutate/<path>.json and meant to be committed with the code.
type Snapshot struct {
	Version  int
	File     string
	Language string
	// Tests is the SHA-256 of each test file that imports the file, by its
	// slash-separated path from the project root: the results hold while
	// they are the same. A snapshot written before snapshots recorded tests
	// has none, nil, which matches no tests.
	Tests map[string]string
	Units []UnitResult
	// Listed is the file that defines each listed test an outcome records
	// (Mutant.Tests), by its ID, as the list command names it, "" for none.
	// Left out when no outcome records any.
	Listed map[string]string
	// ListedFiles is the SHA-256 of each file Listed names, and Support of
	// each support file (mutation.tests.support), by its slash-separated
	// path from the project root, when the outcomes were recorded: listed
	// outcomes hold while they are the same (ListedChanged). Left out when
	// no outcome is listed.
	ListedFiles map[string]string
	Support     map[string]string
	// ListedFingerprints is the fingerprint of each test Listed names that
	// the list command printed with one, by its ID: a listed outcome resting
	// on such a test holds while the list prints the test with it, whatever
	// its file holds, and its file's hash unchanged means it does. A test
	// without one is dated by its file. Left out when none has one, as in a
	// snapshot written before itos-cc read fingerprints.
	ListedFingerprints map[string]string
}

// UnitResult is one function's mutants. Hash is the function's source hash:
// while it is unchanged, killed mutants stay killed.
type UnitResult struct {
	Namespace string
	Name      string
	Hash      string
	StartLine int
	EndLine   int
	Killed    int
	Survived  int
	Uncovered int
	Sites     int
	Mutants   []Mutant
	// Coverage is independent executable coverage for this function. Nil
	// means the snapshot predates strict coverage of its language or the
	// function was not measured by an admitted producer. Go evidence is
	// written under go_coverage, as it always was; evidence of a
	// line-precision language (CoverageEvidence.Language) under
	// line_coverage (MarshalJSON).
	Coverage *CoverageEvidence
	// Stale marks an entry decided under tests other than those the
	// snapshot records: a run that did not judge the function kept it,
	// outcomes and survivors included, after the tests that import the file
	// changed. It is never reused, and mutation check calls it stale, until
	// a run judges the function again. Left out of the file when false, so
	// every snapshot without such an entry reads and is written as before.
	Stale bool
}

// CoverageEvidence is a function's complete measured executable coverage
// inventory and the producer/input boundary that makes it reusable. Go
// evidence, Language "", holds positive-weight blocks at cover-profile
// precision; that of a line-precision language, such as Python's from
// coverage.py's LCOV, holds a block per executable line of the function,
// Span its line number, Weight 1 and Column 0.
type CoverageEvidence struct {
	Version  int               `json:"version"`
	Language string            `json:"language,omitempty"`
	File     string            `json:"file"`
	Function string            `json:"function"`
	Hash     string            `json:"function_hash"`
	Producer string            `json:"producer"`
	Inputs   map[string]string `json:"inputs"`
	Complete bool              `json:"complete"`
	Blocks   []CoverageBlock   `json:"blocks"`
}

// GoCoverageEvidence is Go's CoverageEvidence, by the name it had before
// other languages had any.
type GoCoverageEvidence = CoverageEvidence

// CoverageBlock is a positive-weight executable span at its format's
// precision: a Go coverage-profile block, whose Span distinguishes blocks
// sharing a line, or an executable line.
type CoverageBlock struct {
	Span    string  `json:"span"`
	Line    int     `json:"line"`
	Column  int     `json:"column"`
	Weight  float64 `json:"weight"`
	Covered bool    `json:"covered"`
}

// GoCoverageBlock is Go's CoverageBlock, by its name before other
// languages had any.
type GoCoverageBlock = CoverageBlock

// Lines is the first and last line of the block, both included: a Go
// block's span "line.column,line.column", less its last line when the span
// ends at that line's first column, before any of it, and an executable
// line's own line.
func (b CoverageBlock) Lines() (start, end int) {
	from, to, ok := strings.Cut(b.Span, ",")
	if !ok {
		return b.Line, b.Line
	}
	startText, _, _ := strings.Cut(from, ".")
	endText, endColumn, _ := strings.Cut(to, ".")
	start, err1 := strconv.Atoi(startText)
	end, err2 := strconv.Atoi(endText)
	if err1 != nil || err2 != nil || end < start {
		return b.Line, b.Line
	}
	if endColumn == "1" && end > start {
		end--
	}
	return start, end
}

// LineScope says whether strict coverage judges the lines from start to
// end, both included, of the source at path: with --fail-uncovered=lines,
// whether the --since range added or changed any of them. A nil LineScope
// judges every line, as --fail-uncovered alone does.
type LineScope func(path string, start, end int) bool

// Judges reports whether s judges any line from start to end of path.
func (s LineScope) Judges(path string, start, end int) bool {
	return s == nil || s(path, start, end)
}

// JudgesBlock reports whether s judges any line of block, of path.
func (s LineScope) JudgesBlock(path string, block CoverageBlock) bool {
	start, end := block.Lines()
	return s.Judges(path, start, end)
}

// Mutant is one site's outcome.
type Mutant struct {
	Line        int
	Column      int
	Offset      int
	Original    string
	Replacement string
	Outcome     string
	// Scope is the scope of the tests that decided Outcome: ScopeAllTests,
	// a --test-command line, or "" for ScopeOwn, which is also how an
	// outcome recorded before scopes reads. See TestScope.
	Scope string
	// Tests is, for an outcome of ScopeListed, the IDs of the listed tests
	// that decided it after the file's own tests survived the mutant: those
	// whose coverage reaches its line.
	Tests []string
	// SuiteEvidence is the test and configured-support input of a
	// broad-scope outcome, of any language. Nil means legacy evidence was
	// not recorded. Go outcomes recorded it under go_evidence before every
	// language did; that key reads as this one, and is never written.
	SuiteEvidence *SuiteEvidence
	// Excepted is the reason itos-cc.yaml gives for a survivor it excepts,
	// as a run or a check judged it; never written to the snapshot, which
	// records the survivor as it is.
	Excepted string
}

// SuiteEvidence is the complete freshness evidence of a broad-scope
// outcome: the hashes of every test file beneath its source's build root
// (SuiteTestHashes) and of the configured support files.
type SuiteEvidence struct {
	Tests   map[string]string `json:"tests"`
	Support map[string]string `json:"support"`
}

// Scopes of the tests that decide an outcome. Any other scope is the
// --test-command line that decided it.
const (
	ScopeOwn      = "own"       // the file's own tests, the default
	ScopeAllTests = "all-tests" // the whole suite of its build root, --all-tests
	// ScopeListed is the file's own tests, then, as they survived the
	// mutant, the listed tests that reach its line (Mutant.Tests).
	ScopeListed = "listed"
)

// RunScope is the scope of a run with the --test-command shell and
// --all-tests all, as TestCommand picks its command: shell wins over all.
func RunScope(shell string, all bool) string {
	switch {
	case shell != "":
		return shell
	case all:
		return ScopeAllTests
	}
	return ScopeOwn
}

// TestScope is the scope that decided the mutant's outcome, ScopeOwn when
// it records none.
func (m Mutant) TestScope() string {
	if m.Scope == "" {
		return ScopeOwn
	}
	return m.Scope
}

// recordedScope is scope as a snapshot records it: ScopeOwn left out.
func RecordedScope(scope string) string {
	if scope == ScopeOwn {
		return ""
	}
	return scope
}

func (m Mutant) Key() string {
	return Site{Offset: m.Offset, Original: m.Original, Replacement: m.Replacement}.Key()
}

// testsMatch reports whether s recorded exactly tests, the same paths with
// the same hashes. A snapshot that records no tests matches none.
func TestsMatch(s *Snapshot, tests map[string]string) bool {
	return s.Tests != nil && maps.Equal(s.Tests, tests)
}

// TestsChanged is how now, the hashes of the test files that import the
// snapshot's file as they are, differ from those it recorded, or nil while
// they are the same: only then do the results of a function whose hash is
// unchanged hold. A snapshot that records no tests has changed tests.
func (s *Snapshot) TestsChanged(now map[string]string) *TestChange {
	if TestsMatch(s, now) {
		return nil
	}
	return CompareTests(s.Tests, now)
}

// TestChange is how the tests that import a file differ from those its
// snapshot recorded, each list sorted. Unrecorded is true when the
// snapshot records no tests at all.
type TestChange struct {
	Unrecorded              bool
	Added, Removed, Changed []string
}

func CompareTests(recorded, now map[string]string) *TestChange {
	c := &TestChange{Unrecorded: recorded == nil}
	for path, hash := range now {
		switch was, ok := recorded[path]; {
		case !ok:
			c.Added = append(c.Added, path)
		case was != hash:
			c.Changed = append(c.Changed, path)
		}
	}
	for path := range recorded {
		if _, ok := now[path]; !ok {
			c.Removed = append(c.Removed, path)
		}
	}
	sort.Strings(c.Added)
	sort.Strings(c.Removed)
	sort.Strings(c.Changed)
	return c
}

// Previous outcomes of one file, by the index of its unit and site key.
type Previous map[int]map[string]string

// previousScopes are the scopes of previous outcomes, keyed as they are.
type PreviousScopes map[int]map[string]DecidedBy

// decidedBy is the scope that decided an outcome, and with ScopeListed the
// listed tests.
type DecidedBy struct {
	Scope    string
	Tests    []string
	Evidence *SuiteEvidence
}

func UnitID(namespace, name string) string { return namespace + "#" + name }

// PairByHash pairs the functions of one file that share a namespace#name
// with the snapshot's entries of that name: hashes are the functions'
// hashes now, in file order, and recorded the entries', in the snapshot's
// order. Each function takes the first entry left with its hash, so
// reordering such functions changes nothing and functions with one hash
// take their entries in file order; then each function left takes the
// first entry left, which records it as it was before it changed. It
// returns the index into recorded of each function's entry, -1 for none.
func PairByHash(hashes, recorded []string) []int {
	out := make([]int, len(hashes))
	used := make([]bool, len(recorded))
	for i, h := range hashes {
		out[i] = -1
		for j, r := range recorded {
			if !used[j] && r == h {
				out[i], used[j] = j, true
				break
			}
		}
	}
	for i := range hashes {
		if out[i] >= 0 {
			continue
		}
		if j := slices.Index(used, false); j >= 0 {
			out[i], used[j] = j, true
		}
	}
	return out
}

// fileKeys is the namespace#name and the hash of each unit of f, in order.
func FileKeys(f *lang.File) (ids, hashes []string) {
	for _, u := range f.Units {
		ids, hashes = append(ids, UnitID(u.Namespace, u.Name)), append(hashes, UnitHash(f, u))
	}
	return ids, hashes
}

// EntriesOf pairs each function of a file, by its namespace#name in ids
// and its hash in hashes, with its entry among entries, as PairByHash pairs
// those sharing a name. It returns the index into entries of each one's
// entry, -1 for none.
func EntriesOf(ids, hashes []string, entries []UnitResult) []int {
	recorded := map[string][]int{}
	for j, e := range entries {
		id := UnitID(e.Namespace, e.Name)
		recorded[id] = append(recorded[id], j)
	}
	units := map[string][]int{}
	for i, id := range ids {
		units[id] = append(units[id], i)
	}
	out := make([]int, len(ids))
	for id, us := range units {
		var now, was []string
		for _, i := range us {
			now = append(now, hashes[i])
		}
		for _, j := range recorded[id] {
			was = append(was, entries[j].Hash)
		}
		for k, p := range PairByHash(now, was) {
			out[us[k]] = -1
			if p >= 0 {
				out[us[k]] = recorded[id][p]
			}
		}
	}
	return out
}

// remembered returns the outcomes a run may keep: those of units whose hash
// is unchanged and whose entry is not marked Stale. s holds only while the
// tests that import f are those it recorded; see usable.
func Remembered(s *Snapshot, f *lang.File) Previous {
	prev, _ := RememberedWithScopes(s, f)
	return prev
}

// rememberedWithScopes is remembered, and the scope each of its outcomes
// was decided with.
func RememberedWithScopes(s *Snapshot, f *lang.File) (Previous, PreviousScopes) {
	prev, scopes := Previous{}, PreviousScopes{}
	if s == nil {
		return prev, scopes
	}
	ids, hashes := FileKeys(f)
	for i, j := range EntriesOf(ids, hashes, s.Units) {
		if j < 0 {
			continue
		}
		u := s.Units[j]
		if u.Hash != hashes[i] || u.Stale {
			continue
		}
		outcomes, decided := map[string]string{}, map[string]DecidedBy{}
		for _, m := range u.Mutants {
			outcomes[m.Key()] = m.Outcome
			decided[m.Key()] = DecidedBy{Scope: m.TestScope(), Tests: m.Tests, Evidence: m.SuiteEvidence}
		}
		prev[i], scopes[i] = outcomes, decided
	}
	return prev, scopes
}

// usable is s when its results still hold for the tests that import its
// file now, tests, and nil otherwise: results the tests may no longer earn
// count for nothing, neither reused nor kept for a function not judged.
func Usable(s *Snapshot, tests map[string]string) *Snapshot {
	if s == nil || !TestsMatch(s, tests) {
		return nil
	}
	return s
}

// kept reports the previous outcome of site when it can be reused without
// running: a killed or timed-out mutant in an unchanged unit, or a survivor
// there that excepted says itos-cc.yaml excepts. Other survivors are always
// retried, since new tests may kill them.
func (p Previous) Kept(f *lang.File, s Site, excepted bool) (string, bool) {
	outcome := p[s.Unit][s.Key()]
	return outcome, outcome == Killed || outcome == Timeout || excepted && outcome == Survived
}

// markExcepted sets Excepted on each mutant of units that mutants, the
// mutants decided, says is excepted.
func MarkExcepted(units []UnitResult, mutants []MutantResult) {
	reasons := map[string]string{}
	for _, m := range mutants {
		if m.Excepted != "" {
			reasons[m.Function+"\x00"+m.Key()] = m.Excepted
		}
	}
	if len(reasons) == 0 {
		return
	}
	for i := range units {
		u := &units[i]
		for j := range u.Mutants {
			u.Mutants[j].Excepted = reasons[UnitID(u.Namespace, u.Name)+"\x00"+u.Mutants[j].Key()]
		}
	}
}

// build assembles a snapshot from every site's outcome, against tests, the
// tests that import the file. A skipped site is left out, and so is one
// with no outcome, which a fail-fast stop left undecided. Every outcome
// records ScopeOwn; see buildScoped.
func Build(f *lang.File, rel string, tests map[string]string, sites []Site, outcomes []string) Snapshot {
	return BuildScoped(f, rel, tests, sites, outcomes, nil, nil)
}

// buildScoped is build with the scope each outcome was decided with, by
// site, or ScopeOwn for every one when scopes is nil, and the listed tests
// that decided it, by site, when ran is not nil.
func BuildScoped(f *lang.File, rel string, tests map[string]string, sites []Site, outcomes, scopes []string, ran [][]string) Snapshot {
	if tests == nil {
		tests = map[string]string{}
	}
	snap := Snapshot{Version: snapshot.Version, File: filepath.ToSlash(rel), Language: f.Spec.Name, Tests: tests, Units: []UnitResult{}}
	for _, u := range f.Units {
		snap.Units = append(snap.Units, UnitResult{
			Namespace: u.Namespace, Name: u.Name, Hash: UnitHash(f, u),
			StartLine: u.StartLine, EndLine: u.EndLine, Mutants: []Mutant{},
		})
	}
	for i, s := range sites {
		if outcomes[i] == Skipped || outcomes[i] == "" {
			continue
		}
		r := &snap.Units[s.Unit]
		r.Sites++
		switch outcomes[i] {
		case Killed, Timeout:
			r.Killed++
		case Survived:
			r.Survived++
		case Uncovered:
			r.Uncovered++
		}
		m := Mutant{
			Line: s.Line, Column: s.Column, Offset: s.Offset,
			Original: s.Original, Replacement: s.Replacement, Outcome: outcomes[i],
		}
		if scopes != nil {
			m.Scope = RecordedScope(scopes[i])
		}
		if ran != nil && m.Scope == ScopeListed {
			m.Tests = ran[i]
		}
		r.Mutants = append(r.Mutants, m)
	}
	for i := range snap.Units {
		sort.Slice(snap.Units[i].Mutants, func(a, b int) bool {
			ma, mb := snap.Units[i].Mutants[a], snap.Units[i].Mutants[b]
			if ma.Offset != mb.Offset {
				return ma.Offset < mb.Offset
			}
			return ma.Replacement < mb.Replacement
		})
	}
	return snap
}

// listedOf is the file of each listed test the mutants of units record, by
// its ID: as files, the list now, names it, else as previous recorded it.
func ListedOf(units []UnitResult, files map[string]string, Previous *Snapshot) map[string]string {
	var out map[string]string
	for _, u := range units {
		for _, m := range u.Mutants {
			for _, id := range m.Tests {
				if out == nil {
					out = map[string]string{}
				}
				file, ok := files[id]
				if !ok && Previous != nil {
					file = Previous.Listed[id]
				}
				out[id] = file
			}
		}
	}
	return out
}

// keepUnjudged keeps the functions of units, a file's in order, whose index
// judged holds and puts in place of each other one what previous records
// for it, its entry paired by name and hash (EntriesOf), so a function not
// judged does not change in the snapshot. Unchanged, it keeps its outcomes
// at its current lines; changed since, it keeps its entry as recorded, old
// hash included, so a later run still sees the change; and one previous
// does not record gets no entry. testsHold says whether previous recorded
// the tests that import the file as they are now: when not, the snapshot
// written records the new ones, so each entry kept is marked Stale, as one
// already marked stays.
func KeepUnjudged(units []UnitResult, judged map[int]bool, Previous *Snapshot, testsHold bool) []UnitResult {
	var entries []UnitResult
	if Previous != nil {
		entries = Previous.Units
	}
	var ids, hashes []string
	for _, u := range units {
		ids, hashes = append(ids, UnitID(u.Namespace, u.Name)), append(hashes, u.Hash)
	}
	pair := EntriesOf(ids, hashes, entries)
	out := []UnitResult{}
	for i, u := range units {
		var was UnitResult
		ok := pair[i] >= 0
		if ok {
			was = entries[pair[i]]
		}
		switch {
		case judged[i]:
			out = append(out, u)
		case !ok:
		case !testsHold || was.Stale:
			was.Stale = true
			out = append(out, was)
		case was.Hash == u.Hash:
			// Current positions belong to this source, but the judgment and
			// its dependencies still belong to the run that decided it.
			recorded := map[string]Mutant{}
			for _, m := range was.Mutants {
				recorded[m.Key()] = m
			}
			for j, m := range u.Mutants {
				if original, ok := recorded[m.Key()]; ok {
					original.Line, original.Column = m.Line, m.Column
					u.Mutants[j] = original
				}
			}
			// The run may have measured file-wide statement coverage, but an
			// unjudged function cannot inherit its new inventory or input
			// fingerprints. Keep the independent evidence exactly as recorded.
			u.Coverage = was.Coverage
			out = append(out, u)
		default:
			out = append(out, was)
		}
	}
	return out
}
