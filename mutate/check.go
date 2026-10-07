package mutate

import (
	"fmt"
	"sort"

	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/project"
)

// States of a function's cached results against its source now.
const (
	Fresh   = "fresh"   // the snapshot records the function as it is
	Stale   = "stale"   // the function changed since the snapshot recorded it
	Missing = "missing" // the snapshot does not record the function
)

// FileCheck is one source file's functions checked against its snapshot.
type FileCheck struct {
	Rel       string
	Functions []FunctionCheck
}

// FunctionCheck is one function's cached results.
type FunctionCheck struct {
	Function  string // namespace#name
	StartLine int    // where it starts now
	State     string // Fresh, Stale, or Missing
	// Entry is the snapshot's entry for the function, zero when Missing.
	Entry UnitResult
	// Mutants is what Entry records when Fresh, in site order, each at its
	// site's line and column now; nil otherwise.
	Mutants []Mutant
}

// Check compares each function of files that has a mutation site with its
// file's snapshot, running nothing and writing nothing. judge, when set,
// says which functions of the file at path to check, by namespace#name. A
// function is fresh while one entry under its name has its hash, so a
// function that only moved is fresh, as it is for reuse.
func Check(files []string, judge func(path, function string) bool) ([]FileCheck, error) {
	var out []FileCheck
	for _, path := range files {
		c, err := checkFile(path, judge)
		if err != nil {
			return out, err
		}
		out = append(out, c)
	}
	return out, nil
}

func checkFile(path string, judge func(path, function string) bool) (FileCheck, error) {
	f, err := lang.ParseFile(path)
	if err != nil {
		return FileCheck{}, err
	}
	defer f.Close()
	rel := project.Rel(path)
	result := FileCheck{Rel: rel, Functions: []FunctionCheck{}}
	snap, err := LoadSnapshot(rel)
	if err != nil {
		return result, fmt.Errorf("%s: %w", SnapshotName(rel), err)
	}
	recorded := map[string][]UnitResult{}
	if snap != nil {
		for _, u := range snap.Units {
			id := unitID(u.Namespace, u.Name)
			recorded[id] = append(recorded[id], u)
		}
	}
	sites := map[int][]Site{}
	for _, s := range Sites(f) {
		sites[s.Unit] = append(sites[s.Unit], s)
	}
	for i, u := range f.Units {
		id := unitID(u.Namespace, u.Name)
		if len(sites[i]) == 0 || (judge != nil && !judge(path, id)) {
			continue
		}
		c := FunctionCheck{Function: id, StartLine: u.StartLine, State: Missing}
		entries := recorded[id]
		if len(entries) > 0 {
			c.State, c.Entry = Stale, entries[0]
		}
		hash := UnitHash(f, u)
		for _, e := range entries {
			if e.Hash == hash {
				c.State, c.Entry, c.Mutants = Fresh, e, placed(e.Mutants, sites[i])
				break
			}
		}
		result.Functions = append(result.Functions, c)
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
