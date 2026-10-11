package mutate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/metrics"
	"github.com/donvargax/itos-cc/project"
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
	Version  int    `json:"version"`
	File     string `json:"file"`
	Language string `json:"language"`
	// Tests is the SHA-256 of each test file that imports the file, by its
	// slash-separated path from the project root: the results hold while
	// they are the same. A snapshot written before snapshots recorded tests
	// has none, nil, which matches no tests.
	Tests map[string]string `json:"tests"`
	Units []UnitResult      `json:"units"`
	// Listed is the file that defines each listed test an outcome records
	// (Mutant.Tests), by its ID, as the list command names it, "" for none.
	// Left out when no outcome records any.
	Listed map[string]string `json:"listed,omitempty"`
	// ListedFiles is the SHA-256 of each file Listed names, and Support of
	// each support file (mutation.tests.support), by its slash-separated
	// path from the project root, when the outcomes were recorded: listed
	// outcomes hold while they are the same (ListedChanged). Left out when
	// no outcome is listed.
	ListedFiles map[string]string `json:"listed_files,omitempty"`
	Support     map[string]string `json:"support,omitempty"`
}

// UnitResult is one function's mutants. Hash is the function's source hash:
// while it is unchanged, killed mutants stay killed.
type UnitResult struct {
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Hash      string   `json:"hash"`
	StartLine int      `json:"start_line"`
	EndLine   int      `json:"end_line"`
	Killed    int      `json:"killed"`
	Survived  int      `json:"survived"`
	Uncovered int      `json:"uncovered"`
	Sites     int      `json:"sites"`
	Mutants   []Mutant `json:"mutants"`
	// Coverage is independent executable coverage for this function. Nil
	// means the snapshot predates strict coverage of its language or the
	// function was not measured by an admitted producer. Go evidence is
	// written under go_coverage, as it always was; evidence of a
	// line-precision language (CoverageEvidence.Language) under
	// line_coverage (MarshalJSON).
	Coverage *CoverageEvidence `json:"go_coverage,omitempty"`
	// Stale marks an entry decided under tests other than those the
	// snapshot records: a run that did not judge the function kept it,
	// outcomes and survivors included, after the tests that import the file
	// changed. It is never reused, and mutation check calls it stale, until
	// a run judges the function again. Left out of the file when false, so
	// every snapshot without such an entry reads and is written as before.
	Stale bool `json:"stale,omitempty"`
}

// unitLines is UnitResult as a snapshot writes a unit whose coverage
// evidence is of a line-precision language: under line_coverage rather
// than go_coverage. Converting between the two keeps their fields the same.
type unitLines struct {
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Hash      string            `json:"hash"`
	StartLine int               `json:"start_line"`
	EndLine   int               `json:"end_line"`
	Killed    int               `json:"killed"`
	Survived  int               `json:"survived"`
	Uncovered int               `json:"uncovered"`
	Sites     int               `json:"sites"`
	Mutants   []Mutant          `json:"mutants"`
	Coverage  *CoverageEvidence `json:"line_coverage,omitempty"`
	Stale     bool              `json:"stale,omitempty"`
}

// MarshalJSON writes u with its coverage evidence under go_coverage, Go's
// key, or under line_coverage for a line-precision language, so a Go
// snapshot reads and is written as before.
func (u UnitResult) MarshalJSON() ([]byte, error) {
	type plain UnitResult
	var v any = plain(u)
	if u.Coverage != nil && u.Coverage.Language != "" {
		v = unitLines(u)
	}
	// As metrics.Write writes a snapshot: the encoder that embeds this
	// escapes what its own settings say.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// UnmarshalJSON reads a unit, its coverage evidence under go_coverage or
// line_coverage.
func (u *UnitResult) UnmarshalJSON(data []byte) error {
	type plain UnitResult
	var raw struct {
		plain
		Lines *CoverageEvidence `json:"line_coverage"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*u = UnitResult(raw.plain)
	if u.Coverage == nil {
		u.Coverage = raw.Lines
	}
	return nil
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
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Offset      int    `json:"offset"`
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
	Outcome     string `json:"outcome"`
	// Scope is the scope of the tests that decided Outcome: ScopeAllTests,
	// a --test-command line, or "" for ScopeOwn, which is also how an
	// outcome recorded before scopes reads. See TestScope.
	Scope string `json:"scope,omitempty"`
	// Tests is, for an outcome of ScopeListed, the IDs of the listed tests
	// that decided it after the file's own tests survived the mutant: those
	// whose coverage reaches its line.
	Tests []string `json:"tests,omitempty"`
	// SuiteEvidence is the test and configured-support input of a
	// broad-scope outcome, of any language. Nil means legacy evidence was
	// not recorded. Go outcomes recorded it under go_evidence before every
	// language did; that key reads as this one, and is never written.
	SuiteEvidence *SuiteEvidence `json:"suite_evidence,omitempty"`
	// Excepted is the reason itos-cc.yaml gives for a survivor it excepts,
	// as a run or a check judged it; never written to the snapshot, which
	// records the survivor as it is.
	Excepted string `json:"-"`
}

// SuiteEvidence is the complete freshness evidence of a broad-scope
// outcome: the hashes of every test file beneath its source's build root
// (SuiteTestHashes) and of the configured support files.
type SuiteEvidence struct {
	Tests   map[string]string `json:"tests"`
	Support map[string]string `json:"support"`
}

// UnmarshalJSON reads a mutant, its evidence under suite_evidence or, as Go
// outcomes recorded it before, go_evidence.
func (m *Mutant) UnmarshalJSON(data []byte) error {
	type plain Mutant
	var raw struct {
		plain
		GoEvidence *SuiteEvidence `json:"go_evidence"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*m = Mutant(raw.plain)
	if m.SuiteEvidence == nil {
		m.SuiteEvidence = raw.GoEvidence
	}
	return nil
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
func recordedScope(scope string) string {
	if scope == ScopeOwn {
		return ""
	}
	return scope
}

// scopeCommand is the command that runs path's tests in scope, tests the
// test files that reach it.
func scopeCommand(path, scope string, tests []string) Command {
	switch scope {
	case ScopeOwn, ScopeListed, "":
		return TestCommand(path, "", false, tests)
	case ScopeAllTests:
		return TestCommand(path, "", true, nil)
	}
	return TestCommand(path, scope, false, nil)
}

func (m Mutant) key() string {
	return Site{Offset: m.Offset, Original: m.Original, Replacement: m.Replacement}.Key()
}

// SnapshotName is where the snapshot of the file at rel, its path from the
// project root (project.FromRoot), lives under .metrics.
func SnapshotName(rel string) string {
	return filepath.Join("mutate", filepath.ToSlash(rel)+".json")
}

// LoadSnapshot reads the snapshot of the file at rel, its path from the
// project root, or returns nil when there is none.
func LoadSnapshot(rel string) (*Snapshot, error) {
	return LoadSnapshotOf(project.Root(), rel)
}

// LoadSnapshotOf is LoadSnapshot for the project whose root is root, which
// need not be the working directory's: the snapshot at SnapshotName(rel)
// under its .metrics, which holds while it names the file rel. One that
// names another file is none, so the file's functions are missing. Every
// reader looks a file's snapshot up here, mutation check and the
// architecture graph alike.
//
// An outcome it records as its file's own tests', where those run the whole
// suite (OwnScope), reads as ScopeAllTests, the scope that decided it: one
// recorded before that rule has no whole-suite evidence, so it reads stale
// once.
func LoadSnapshotOf(root, rel string) (*Snapshot, error) {
	var s Snapshot
	ok, err := metrics.ReadIn(metrics.DirOf(root), SnapshotName(rel), &s)
	if err != nil || !ok || s.File != filepath.ToSlash(rel) {
		return nil, err
	}
	if OwnScope(filepath.Join(root, filepath.FromSlash(rel))) == ScopeAllTests {
		for i := range s.Units {
			for j := range s.Units[i].Mutants {
				if m := &s.Units[i].Mutants[j]; m.TestScope() == ScopeOwn {
					m.Scope = ScopeAllTests
				}
			}
		}
	}
	return &s, nil
}

// TestHashes is what a snapshot records of the test files tests: the
// SHA-256 of each one's content, by its slash-separated path from the
// project root (project.FromRoot).
func TestHashes(tests []string) (map[string]string, error) {
	return testHashes(tests, project.FromRoot)
}

// TestHashesUnder is TestHashes for the project at root, which need not be
// the working directory's: each test file by its slash-separated path from
// root, as a run in that project records it.
func TestHashesUnder(root string, tests []string) (map[string]string, error) {
	return testHashes(tests, func(path string) string {
		if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
		return path
	})
}

func testHashes(tests []string, rel func(string) string) (map[string]string, error) {
	out := map[string]string{}
	for _, path := range tests {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		out[filepath.ToSlash(rel(path))] = hex.EncodeToString(sum[:])
	}
	return out, nil
}

// testsMatch reports whether s recorded exactly tests, the same paths with
// the same hashes. A snapshot that records no tests matches none.
func testsMatch(s *Snapshot, tests map[string]string) bool {
	return s.Tests != nil && maps.Equal(s.Tests, tests)
}

// TestsChanged is how now, the hashes of the test files that import the
// snapshot's file as they are, differ from those it recorded, or nil while
// they are the same: only then do the results of a function whose hash is
// unchanged hold. A snapshot that records no tests has changed tests.
func (s *Snapshot) TestsChanged(now map[string]string) *TestChange {
	if testsMatch(s, now) {
		return nil
	}
	return compareTests(s.Tests, now)
}

// TestChange is how the tests that import a file differ from those its
// snapshot recorded, each list sorted. Unrecorded is true when the
// snapshot records no tests at all.
type TestChange struct {
	Unrecorded              bool
	Added, Removed, Changed []string
}

func compareTests(recorded, now map[string]string) *TestChange {
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
type previous map[int]map[string]string

// previousScopes are the scopes of previous outcomes, keyed as they are.
type previousScopes map[int]map[string]decidedBy

// decidedBy is the scope that decided an outcome, and with ScopeListed the
// listed tests.
type decidedBy struct {
	scope    string
	tests    []string
	evidence *SuiteEvidence
}

func unitID(namespace, name string) string { return namespace + "#" + name }

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
func fileKeys(f *lang.File) (ids, hashes []string) {
	for _, u := range f.Units {
		ids, hashes = append(ids, unitID(u.Namespace, u.Name)), append(hashes, UnitHash(f, u))
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
		id := unitID(e.Namespace, e.Name)
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
func remembered(s *Snapshot, f *lang.File) previous {
	prev, _ := rememberedWithScopes(s, f)
	return prev
}

// rememberedWithScopes is remembered, and the scope each of its outcomes
// was decided with.
func rememberedWithScopes(s *Snapshot, f *lang.File) (previous, previousScopes) {
	prev, scopes := previous{}, previousScopes{}
	if s == nil {
		return prev, scopes
	}
	ids, hashes := fileKeys(f)
	for i, j := range EntriesOf(ids, hashes, s.Units) {
		if j < 0 {
			continue
		}
		u := s.Units[j]
		if u.Hash != hashes[i] || u.Stale {
			continue
		}
		outcomes, decided := map[string]string{}, map[string]decidedBy{}
		for _, m := range u.Mutants {
			outcomes[m.key()] = m.Outcome
			decided[m.key()] = decidedBy{scope: m.TestScope(), tests: m.Tests, evidence: m.SuiteEvidence}
		}
		prev[i], scopes[i] = outcomes, decided
	}
	return prev, scopes
}

// usable is s when its results still hold for the tests that import its
// file now, tests, and nil otherwise: results the tests may no longer earn
// count for nothing, neither reused nor kept for a function not judged.
func usable(s *Snapshot, tests map[string]string) *Snapshot {
	if s == nil || !testsMatch(s, tests) {
		return nil
	}
	return s
}

// kept reports the previous outcome of site when it can be reused without
// running: a killed or timed-out mutant in an unchanged unit, or a survivor
// there that excepted says itos-cc.yaml excepts. Other survivors are always
// retried, since new tests may kill them.
func (p previous) kept(f *lang.File, s Site, excepted bool) (string, bool) {
	outcome := p[s.Unit][s.Key()]
	return outcome, outcome == Killed || outcome == Timeout || excepted && outcome == Survived
}

// markExcepted sets Excepted on each mutant of units that mutants, the
// mutants decided, says is excepted.
func markExcepted(units []UnitResult, mutants []MutantResult) {
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
			u.Mutants[j].Excepted = reasons[unitID(u.Namespace, u.Name)+"\x00"+u.Mutants[j].key()]
		}
	}
}

// build assembles a snapshot from every site's outcome, against tests, the
// tests that import the file. A skipped site is left out, and so is one
// with no outcome, which a fail-fast stop left undecided. Every outcome
// records ScopeOwn; see buildScoped.
func build(f *lang.File, rel string, tests map[string]string, sites []Site, outcomes []string) Snapshot {
	return buildScoped(f, rel, tests, sites, outcomes, nil, nil)
}

// buildScoped is build with the scope each outcome was decided with, by
// site, or ScopeOwn for every one when scopes is nil, and the listed tests
// that decided it, by site, when ran is not nil.
func buildScoped(f *lang.File, rel string, tests map[string]string, sites []Site, outcomes, scopes []string, ran [][]string) Snapshot {
	if tests == nil {
		tests = map[string]string{}
	}
	snap := Snapshot{Version: metrics.Version, File: filepath.ToSlash(rel), Language: f.Spec.Name, Tests: tests, Units: []UnitResult{}}
	for _, u := range f.Units {
		snap.Units = append(snap.Units, UnitResult{
			Namespace: u.Namespace, Name: u.Name, Hash: UnitHash(f, u),
			StartLine: u.StartLine, EndLine: u.EndLine, Mutants: []Mutant{},
		})
	}
	for i, s := range sites {
		if outcomes[i] == skipped || outcomes[i] == "" {
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
			m.Scope = recordedScope(scopes[i])
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
func listedOf(units []UnitResult, files map[string]string, previous *Snapshot) map[string]string {
	var out map[string]string
	for _, u := range units {
		for _, m := range u.Mutants {
			for _, id := range m.Tests {
				if out == nil {
					out = map[string]string{}
				}
				file, ok := files[id]
				if !ok && previous != nil {
					file = previous.Listed[id]
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
func keepUnjudged(units []UnitResult, judged map[int]bool, previous *Snapshot, testsHold bool) []UnitResult {
	var entries []UnitResult
	if previous != nil {
		entries = previous.Units
	}
	var ids, hashes []string
	for _, u := range units {
		ids, hashes = append(ids, unitID(u.Namespace, u.Name)), append(hashes, u.Hash)
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
