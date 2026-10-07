package mutate

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"os"
	"path/filepath"
	"sort"
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
	// Stale marks an entry decided under tests other than those the
	// snapshot records: a run that did not judge the function kept it,
	// outcomes and survivors included, after the tests that import the file
	// changed. It is never reused, and mutation check calls it stale, until
	// a run judges the function again. Left out of the file when false, so
	// every snapshot without such an entry reads and is written as before.
	Stale bool `json:"stale,omitempty"`
}

// Mutant is one site's outcome.
type Mutant struct {
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Offset      int    `json:"offset"`
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
	Outcome     string `json:"outcome"`
	// Excepted is the reason itos-cc.yaml gives for a survivor it excepts,
	// as a run or a check judged it; never written to the snapshot, which
	// records the survivor as it is.
	Excepted string `json:"-"`
}

func (m Mutant) key() string {
	return Site{Offset: m.Offset, Original: m.Original, Replacement: m.Replacement}.Key()
}

// SnapshotName is where path's snapshot lives under .metrics.
func SnapshotName(rel string) string {
	return filepath.Join("mutate", filepath.ToSlash(rel)+".json")
}

// LoadSnapshot reads rel's snapshot, or returns nil when there is none.
func LoadSnapshot(rel string) (*Snapshot, error) {
	var s Snapshot
	ok, err := metrics.Read(SnapshotName(rel), &s)
	if err != nil || !ok {
		return nil, err
	}
	return &s, nil
}

// TestHashes is what a snapshot records of the test files tests: the
// SHA-256 of each one's content, by its slash-separated path from the
// working directory, the project root.
func TestHashes(tests []string) (map[string]string, error) {
	return testHashes(tests, project.Rel)
}

// TestHashesUnder is TestHashes for the project at root, which need not be
// the working directory: each test file by its slash-separated path from
// root, as a run from root records it.
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

// Previous outcomes of one file, by unit identity and site key.
type previous map[string]map[string]string

func unitID(namespace, name string) string { return namespace + "#" + name }

// remembered returns the outcomes a run may keep: those of units whose hash
// is unchanged and whose entry is not marked Stale. s holds only while the
// tests that import f are those it recorded; see usable.
func remembered(s *Snapshot, f *lang.File) previous {
	prev := previous{}
	if s == nil {
		return prev
	}
	hashes := map[string]string{}
	for _, u := range f.Units {
		hashes[unitID(u.Namespace, u.Name)] = UnitHash(f, u)
	}
	for _, u := range s.Units {
		id := unitID(u.Namespace, u.Name)
		if hashes[id] != u.Hash || u.Stale {
			continue
		}
		outcomes := map[string]string{}
		for _, m := range u.Mutants {
			outcomes[m.key()] = m.Outcome
		}
		prev[id] = outcomes
	}
	return prev
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
	u := f.Units[s.Unit]
	outcome := p[unitID(u.Namespace, u.Name)][s.Key()]
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
// tests that import the file. A skipped site is left out.
func build(f *lang.File, rel string, tests map[string]string, sites []Site, outcomes []string) Snapshot {
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
		if outcomes[i] == skipped {
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
		r.Mutants = append(r.Mutants, Mutant{
			Line: s.Line, Column: s.Column, Offset: s.Offset,
			Original: s.Original, Replacement: s.Replacement, Outcome: outcomes[i],
		})
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

// keepUnjudged keeps the functions of units that judged holds and puts in
// place of each other one what previous records for it, so a function not
// judged does not change in the snapshot. Unchanged, it keeps its outcomes
// at its current lines; changed since, it keeps its entry as recorded, old
// hash included, so a later run still sees the change; and one previous
// does not record gets no entry. testsHold says whether previous recorded
// the tests that import the file as they are now: when not, the snapshot
// written records the new ones, so each entry kept is marked Stale, as one
// already marked stays.
func keepUnjudged(units []UnitResult, judged map[string]bool, previous *Snapshot, testsHold bool) []UnitResult {
	recorded := map[string]UnitResult{}
	if previous != nil {
		for _, u := range previous.Units {
			recorded[unitID(u.Namespace, u.Name)] = u
		}
	}
	out := []UnitResult{}
	for _, u := range units {
		id := unitID(u.Namespace, u.Name)
		was, ok := recorded[id]
		switch {
		case judged[id]:
			out = append(out, u)
		case !ok:
		case !testsHold || was.Stale:
			was.Stale = true
			out = append(out, was)
		case was.Hash == u.Hash:
			out = append(out, u)
		default:
			out = append(out, was)
		}
	}
	return out
}
