package mutate

import (
	"fmt"
	"slices"
	"sort"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/project"
)

// States of a function's cached results against its source now.
const (
	Fresh   = "fresh"   // the snapshot records the function as it is
	Stale   = "stale"   // the function, or the tests that import its file, changed since the snapshot recorded it, or it has a site the snapshot never recorded
	Missing = "missing" // the snapshot does not record the function
)

// FileCheck is one source file's functions checked against its snapshot.
type FileCheck struct {
	Rel       string
	Functions []FunctionCheck
	// StaleExceptions is each exception of the functions checked that no
	// longer holds: those whose function changed or whose site is gone,
	// then those whose mutant a fresh entry records killed.
	StaleExceptions []StaleException
}

// FunctionCheck is one function's cached results.
type FunctionCheck struct {
	Function  string // namespace#name
	StartLine int    // where it starts now
	State     string // Fresh, Stale, or Missing
	// Entry is the snapshot's entry for the function, zero when Missing.
	Entry UnitResult
	// Tests is how the tests that import the file changed since the
	// snapshot, when that alone makes the function Stale; nil otherwise.
	Tests *TestChange
	// Marked is true when the function is Stale because Entry is marked
	// Stale: a run that did not judge it kept it after the tests that
	// import the file changed.
	Marked bool
	// Mutants is what Entry records when Fresh, in site order, each at its
	// site's line and column now, a survivor itos-cc.yaml excepts with its
	// reason; nil otherwise.
	Mutants []Mutant
	// Unrecorded is each site the function has now that Entry does not
	// record although its hash and tests match, in line order, as when a
	// newer itos-cc adds a mutation operator; such a function is Stale. nil
	// otherwise.
	Unrecorded []Site
	unit       int // the function's index in its file's units
}

// Check compares each function of files that has a mutation site with its
// file's snapshot, running nothing and writing nothing. judge, when set,
// says which functions of the file at path to check, by namespace#name and
// hash, as Options.Judge does. tests, when set, lists the test files that
// import the file at path, as Options.Tests does for a run. Each function
// is paired with an entry of its name by hash, functions sharing a name
// each with their own (PairByHash), and is fresh while that entry has its
// hash, so a function that only moved is fresh, as it is for reuse, and
// while the snapshot records the tests that import its file as they are
// now. exceptions are the survivors itos-cc.yaml excepts, judged
// as a run judges them: a fresh entry's survivor that one excepts fails
// nothing, and one that no longer holds is stale. An entry whose hash and
// tests match but which lacks a site the function has now is stale too,
// naming those sites in Unrecorded: no run judged them, and a run runs only
// them.
func Check(files []string, judge func(path, function, hash string) bool, tests func(path string) []string, exceptions []config.Exception) ([]FileCheck, error) {
	var out []FileCheck
	for _, path := range files {
		c, err := checkFile(path, judge, tests, exceptions)
		if err != nil {
			return out, err
		}
		out = append(out, c)
	}
	return out, nil
}

func checkFile(path string, judge func(path, function, hash string) bool, tests func(path string) []string, exceptions []config.Exception) (FileCheck, error) {
	f, err := lang.ParseFile(path)
	if err != nil {
		return FileCheck{}, err
	}
	defer f.Close()
	c, err := checkParsed(f, path, judge, tests, exceptions)
	for i := range c.Functions {
		if fn := &c.Functions[i]; fn.State == Fresh && len(fn.Unrecorded) > 0 {
			fn.State, fn.Mutants = Stale, nil
		}
	}
	return c, err
}

// checkParsed is checkFile of f, the file at path, parsed. It leaves a
// function whose entry lacks some of its sites Fresh, with its Mutants and
// its Unrecorded sites, so mutation sample and mutation except, which read
// only the mutants an entry records, judge those as before; checkFile makes
// it Stale.
func checkParsed(f *lang.File, path string, judge func(path, function, hash string) bool, tests func(path string) []string, exceptions []config.Exception) (FileCheck, error) {
	rel := project.Rel(path)
	result := FileCheck{Rel: rel, Functions: []FunctionCheck{}}
	snap, err := LoadSnapshot(rel)
	if err != nil {
		return result, fmt.Errorf("%s: %w", SnapshotName(rel), err)
	}
	var paths []string
	if tests != nil {
		paths = tests(path)
	}
	now, err := TestHashes(paths)
	if err != nil {
		return result, err
	}
	var changed *TestChange
	if snap != nil {
		changed = snap.TestsChanged(now)
	}
	var entries []UnitResult
	if snap != nil {
		entries = snap.Units
	}
	ids, hashes := fileKeys(f)
	pair := entriesOf(ids, hashes, entries)
	all := Sites(f)
	sites := map[int][]Site{}
	for _, s := range all {
		sites[s.Unit] = append(sites[s.Unit], s)
	}
	// An exception names its function, so it counts when any function of
	// that name is judged.
	names := map[string]bool{}
	for i, u := range f.Units {
		id := ids[i]
		if judge != nil && !judge(path, id, hashes[i]) {
			continue
		}
		names[id] = true
		if len(sites[i]) == 0 {
			continue
		}
		// The function's entry is the one of its name with its hash, so
		// functions sharing a name are told apart; one paired with an entry
		// of another hash changed since.
		c := FunctionCheck{Function: id, StartLine: u.StartLine, State: Missing, unit: i}
		if pair[i] >= 0 {
			e := entries[pair[i]]
			switch {
			case e.Hash != hashes[i]:
				c.State, c.Entry = Stale, e
			case changed != nil:
				c.State, c.Entry, c.Tests = Stale, e, changed
			case e.Stale:
				c.State, c.Entry, c.Marked = Stale, e, true
			default:
				c.State, c.Entry, c.Mutants = Fresh, e, placed(e.Mutants, sites[i])
				c.Unrecorded = unrecorded(e.Mutants, sites[i])
			}
		}
		result.Functions = append(result.Functions, c)
	}
	var judgeFunction func(function string) bool
	if judge != nil {
		judgeFunction = func(function string) bool { return names[function] }
	}
	x := applyExceptions(exceptions, rel, f, all, judgeFunction)
	result.StaleExceptions = x.stale
	at := map[string]int{}
	for i, s := range all {
		at[fmt.Sprint(s.Unit, "\x00", s.Key())] = i
	}
	for _, fn := range result.Functions {
		for j := range fn.Mutants {
			m := &fn.Mutants[j]
			i, ok := at[fmt.Sprint(fn.unit, "\x00", m.key())]
			if !ok {
				continue
			}
			reason, stale := x.judge(i, all[i], m.Outcome)
			m.Excepted = reason
			if stale != nil {
				result.StaleExceptions = append(result.StaleExceptions, *stale)
			}
		}
	}
	return result, nil
}

// unrecorded is each of sites, in line order, that recorded has no mutant
// for.
func unrecorded(recorded []Mutant, sites []Site) []Site {
	has := map[string]bool{}
	for _, m := range recorded {
		has[m.key()] = true
	}
	var out []Site
	for _, s := range sites {
		if !has[s.Key()] {
			out = append(out, s)
		}
	}
	slices.SortStableFunc(out, LineOrder)
	return out
}

// placed is recorded at the line and column of its site now, which differ
// from those recorded when the unchanged function moved.
func placed(recorded []Mutant, sites []Site) []Mutant {
	at := map[string]Site{}
	for _, s := range sites {
		at[s.Key()] = s
	}
	out := []Mutant{}
	for _, m := range recorded {
		if s, ok := at[m.key()]; ok {
			m.Line, m.Column = s.Line, s.Column
		}
		out = append(out, m)
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Line != out[b].Line {
			return out[a].Line < out[b].Line
		}
		return out[a].Column < out[b].Column
	})
	return out
}
