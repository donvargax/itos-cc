package mutate

import (
	"path/filepath"
	"sort"

	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/metrics"
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
	Version  int          `json:"version"`
	File     string       `json:"file"`
	Language string       `json:"language"`
	Units    []UnitResult `json:"units"`
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
}

// Mutant is one site's outcome.
type Mutant struct {
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Offset      int    `json:"offset"`
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
	Outcome     string `json:"outcome"`
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

// Previous outcomes of one file, by unit identity and site key.
type previous map[string]map[string]string

func unitID(namespace, name string) string { return namespace + "#" + name }

// remembered returns the outcomes a run may keep: those of units whose hash
// is unchanged.
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
		if hashes[id] != u.Hash {
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

// kept reports the previous outcome of site when it can be reused without
// running: a killed or timed-out mutant in an unchanged unit. Survivors are
// always retried, since new tests may kill them.
func (p previous) kept(f *lang.File, s Site) (string, bool) {
	u := f.Units[s.Unit]
	outcome := p[unitID(u.Namespace, u.Name)][s.Key()]
	return outcome, outcome == Killed || outcome == Timeout
}

// build assembles a snapshot from every site's outcome. A skipped site is
// left out.
func build(f *lang.File, rel string, sites []Site, outcomes []string) Snapshot {
	snap := Snapshot{Version: metrics.Version, File: filepath.ToSlash(rel), Language: f.Spec.Name, Units: []UnitResult{}}
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
// does not record gets no entry.
func keepUnjudged(units []UnitResult, judged map[string]bool, previous *Snapshot) []UnitResult {
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
		case judged[id], ok && was.Hash == u.Hash:
			out = append(out, u)
		case ok:
			out = append(out, was)
		}
	}
	return out
}
