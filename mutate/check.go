package mutate

import (
	"fmt"
	"sort"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/project"
)

// States of a function's cached results against its source now.
const (
	Fresh   = "fresh"   // the snapshot records the function as it is
	Stale   = "stale"   // the function, or the tests that import its file, changed since the snapshot recorded it
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
	// Mutants is what Entry records when Fresh, in site order, each at its
	// site's line and column now, a survivor itos-cc.yaml excepts with its
	// reason; nil otherwise.
	Mutants []Mutant
	unit    int // the function's index in its file's units
}

// Check compares each function of files that has a mutation site with its
// file's snapshot, running nothing and writing nothing. judge, when set,
// says which functions of the file at path to check, by namespace#name.
// tests, when set, lists the test files that import the file at path, as
// Options.Tests does for a run. A function is fresh while one entry under
// its name has its hash, so a function that only moved is fresh, as it is
// for reuse, and while the snapshot records the tests that import its file
// as they are now. exceptions are the survivors itos-cc.yaml excepts, judged
// as a run judges them: a fresh entry's survivor that one excepts fails
// nothing, and one that no longer holds is stale.
func Check(files []string, judge func(path, function string) bool, tests func(path string) []string, exceptions []config.Exception) ([]FileCheck, error) {
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

func checkFile(path string, judge func(path, function string) bool, tests func(path string) []string, exceptions []config.Exception) (FileCheck, error) {
	f, err := lang.ParseFile(path)
	if err != nil {
		return FileCheck{}, err
	}
	defer f.Close()
	return checkParsed(f, path, judge, tests, exceptions)
}

// checkParsed is checkFile of f, the file at path, parsed.
func checkParsed(f *lang.File, path string, judge func(path, function string) bool, tests func(path string) []string, exceptions []config.Exception) (FileCheck, error) {
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
	if snap != nil && !testsMatch(snap, now) {
		changed = compareTests(snap.Tests, now)
	}
	recorded := map[string][]UnitResult{}
	if snap != nil {
		for _, u := range snap.Units {
			id := unitID(u.Namespace, u.Name)
			recorded[id] = append(recorded[id], u)
		}
	}
	all := Sites(f)
	sites := map[int][]Site{}
	for _, s := range all {
		sites[s.Unit] = append(sites[s.Unit], s)
	}
	for i, u := range f.Units {
		id := unitID(u.Namespace, u.Name)
		if len(sites[i]) == 0 || (judge != nil && !judge(path, id)) {
			continue
		}
		c := FunctionCheck{Function: id, StartLine: u.StartLine, State: Missing, unit: i}
		entries := recorded[id]
		if len(entries) > 0 {
			c.State, c.Entry = Stale, entries[0]
		}
		hash := UnitHash(f, u)
		for _, e := range entries {
			if e.Hash == hash && changed != nil {
				c.State, c.Entry, c.Tests = Stale, e, changed
				break
			}
			if e.Hash == hash {
				c.State, c.Entry, c.Mutants = Fresh, e, placed(e.Mutants, sites[i])
				break
			}
		}
		result.Functions = append(result.Functions, c)
	}
	var judgeFunction func(function string) bool
	if judge != nil {
		judgeFunction = func(function string) bool { return judge(path, function) }
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
