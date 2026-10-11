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
	Rel       string // the file's path as output names it, from the working directory
	key       string // the file's path as its snapshot names it, from the project root
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
	Freshness
	// Entry is the snapshot's entry for the function, zero when Missing.
	Entry UnitResult
	// Mutants is what Entry records when Fresh, in site order, each at its
	// site's line and column now, a survivor itos-cc.yaml excepts with its
	// reason; nil otherwise.
	Mutants []Mutant
	unit    int // the function's index in its file's units
}

// CoverageCheck is one current function's independent coverage verdict.
type CoverageCheck struct {
	File, Function string
	// Language is the function's language.
	Language string
	Line     int
	State    string // "missing", "stale", "fresh", "unsupported"
	Changed  []string
	Blocks   []CoverageBlock
	Evidence *CoverageEvidence
}

// GoCoverageCheck is CoverageCheck, by its name before other languages had
// strict coverage.
type GoCoverageCheck = CoverageCheck

// CoverageInputs returns the current producer and input fingerprint of the
// strict coverage evidence of the source at path.
type CoverageInputs func(path string) (string, map[string]string, error)

// CheckGoCoverage is CheckCoverage of Go files alone.
func CheckGoCoverage(files []string, judge func(path, function, hash string) bool, current func(path string) (string, map[string]string, error)) ([]CoverageCheck, error) {
	return CheckCoverage(files, judge, map[string]CoverageInputs{"go": current})
}

// CheckCoverage checks independent coverage evidence for every selected
// function of a language current has inputs for, including functions with
// no mutation sites. It runs and writes nothing. current returns, by
// language, the current producer/input fingerprint per file. A file of
// another language is outside every evidence inventory, beneath a go.mod or
// not: it has no verdict here, and current is never asked about it.
func CheckCoverage(files []string, judge func(path, function, hash string) bool, current map[string]CoverageInputs) ([]CoverageCheck, error) {
	var out []CoverageCheck
	for _, path := range files {
		f, err := lang.ParseFile(path)
		if err != nil {
			return out, err
		}
		if f.Spec == nil || current[f.Spec.Name] == nil {
			f.Close()
			continue
		}
		language := f.Spec.Name
		unsupported := ""
		if language == "go" {
			unsupported, err = GoCoverageUnsupported(path)
		} else {
			unsupported, err = LineCoverageUnsupported(language, path, project.Root())
		}
		if err != nil {
			f.Close()
			return out, err
		}
		key := project.FromRoot(path)
		snap, err := LoadSnapshot(key)
		if err != nil {
			f.Close()
			return out, fmt.Errorf("%s: %w", SnapshotName(key), err)
		}
		_, inputs, err := current[language](path)
		if err != nil {
			f.Close()
			return out, err
		}
		var entries []UnitResult
		if snap != nil {
			entries = snap.Units
		}
		ids, hashes := fileKeys(f)
		pair := EntriesOf(ids, hashes, entries)
		for i, unit := range f.Units {
			if judge != nil && !judge(path, ids[i], hashes[i]) {
				continue
			}
			check := CoverageCheck{File: project.Rel(path), Function: ids[i], Language: language, Line: unit.StartLine, State: "missing"}
			if unsupported != "" {
				check.State = "unsupported"
				check.Changed = []string{unsupported}
				out = append(out, check)
				continue
			}
			if pair[i] >= 0 {
				if evidence := entries[pair[i]].Coverage; evidence != nil && evidence.Complete &&
					evidence.Language == evidenceKey(language) && evidence.Version == evidenceVersion(language) &&
					evidence.File == key && evidence.Function == ids[i] && evidence.Hash == hashes[i] {
					check.Blocks = evidence.Blocks
					check.Evidence = evidence
					check.State = "fresh"
					before, now := cloneHashes(evidence.Inputs), cloneHashes(inputs)
					delete(before, "@producer")
					delete(now, "@producer")
					check.Changed = GoCoverageChanges(before, now)
					if !validCoverageProducer(language, evidence.Producer) {
						check.Changed = append(check.Changed, "coverage producer/options")
					}
					if evidence.Producer == "" && len(check.Changed) == 0 {
						check.Changed = []string{"coverage producer/options"}
					}
					if len(check.Changed) > 0 {
						check.State = "stale"
					}
				}
			}
			out = append(out, check)
		}
		f.Close()
	}
	return out, nil
}

// NeedsMutationCoverage reports whether Run must test at least one mutant.
// Fresh kills, timeouts, uncovered outcomes and excepted survivors are
// reusable; a changed/missing function or ordinary survivor is not.
func NeedsMutationCoverage(checks []FileCheck, mutateAll bool) bool {
	if mutateAll {
		return true
	}
	for _, file := range checks {
		for _, function := range file.Functions {
			if function.State != Fresh {
				return true
			}
			for _, mutant := range function.Mutants {
				if mutant.Outcome == Survived && mutant.Excepted == "" {
					return true
				}
			}
		}
	}
	return false
}

// Freshness is how a function's cached results hold against the function
// as it is now, as mutation check calls them.
type Freshness struct {
	State string // Fresh, Stale, or Missing
	// Tests is how the tests that import the file changed since the
	// snapshot, when that alone makes the function Stale; nil otherwise.
	Tests *TestChange
	// Marked is true when the function is Stale because its entry is marked
	// Stale: a run that did not judge it kept it after the tests that
	// import the file changed.
	Marked bool
	// Listed is each file a listed outcome of the function rests on that
	// changed since the snapshot recorded it, when that makes the function
	// Stale: a file that defines one of its tests, or a support file; nil
	// otherwise.
	Listed []string
	// Broad is each module test or configured support input changed for a
	// Go all-tests or test-command outcome.
	Broad []string
	// Unrecorded is each site the function has now that its entry does not
	// record although its hash and tests match, in line order, as when a
	// newer itos-cc adds a mutation operator; such a function is Stale. nil
	// otherwise.
	Unrecorded []Site
}

// FreshnessOf is the freshness of entry, the snapshot's entry paired with a
// function by name and hash (PairByHash), nil for none, against the
// function as it is now: its hash, its sites, and tests, how the tests that
// import its file changed since the snapshot (Snapshot.TestsChanged), and
// listed, how the files its listed outcomes rest on did
// (Snapshot.ListedChanged). It is
// mutation check's verdict, which the architecture graph shows too, so the
// two cannot drift. The entry holds while it has the function's hash, so a
// function that only moved is fresh, the tests are those it recorded, no
// run kept it marked stale, the files its listed outcomes rest on are
// unchanged, and it records every site the function has.
func FreshnessOf(entry *UnitResult, hash string, tests *TestChange, listed *ListedChange, broad []string, sites []Site) Freshness {
	switch {
	case entry == nil:
		return Freshness{State: Missing}
	case entry.Hash != hash:
		return Freshness{State: Stale}
	case tests != nil:
		return Freshness{State: Stale, Tests: tests}
	case entry.Stale:
		return Freshness{State: Stale, Marked: true}
	}
	if files := listed.files(entry.Mutants); len(files) > 0 {
		return Freshness{State: Stale, Listed: files}
	}
	if len(broad) > 0 {
		return Freshness{State: Stale, Broad: broad}
	}
	if u := unrecorded(entry.Mutants, sites); len(u) > 0 {
		return Freshness{State: Stale, Unrecorded: u}
	}
	return Freshness{State: Fresh}
}

// judged is true while Entry's mutants hold for the sites it records: the
// function is Fresh, or Stale only for sites Entry never recorded. Mutation
// sample and mutation except read only those mutants, so they judge such a
// function as they did before check called it stale.
func (c FunctionCheck) judged() bool {
	return c.State == Fresh || len(c.Unrecorded) > 0
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
// now, and while the files its listed outcomes rest on are unchanged,
// support being the support files' hashes now (SupportHashes). exceptions
// are the survivors itos-cc.yaml excepts, judged
// as a run judges them: a fresh entry's survivor that one excepts fails
// nothing, and one that no longer holds is stale. An entry whose hash and
// tests match but which lacks a site the function has now is stale too,
// naming those sites in Unrecorded: no run judged them, and a run runs only
// them.
func Check(files []string, judge func(path, function, hash string) bool, tests func(path string) []string, support map[string]string, exceptions []config.Exception) ([]FileCheck, error) {
	var out []FileCheck
	for _, path := range files {
		c, err := checkFile(path, judge, tests, support, exceptions)
		if err != nil {
			return out, err
		}
		out = append(out, c)
	}
	return out, nil
}

func checkFile(path string, judge func(path, function, hash string) bool, tests func(path string) []string, support map[string]string, exceptions []config.Exception) (FileCheck, error) {
	f, err := lang.ParseFile(path)
	if err != nil {
		return FileCheck{}, err
	}
	defer f.Close()
	c, err := checkParsed(f, path, judge, tests, support, exceptions)
	for i := range c.Functions {
		if fn := &c.Functions[i]; fn.State != Fresh {
			fn.Mutants = nil
		}
	}
	return c, err
}

// checkParsed is checkFile of f, the file at path, parsed. It leaves the
// Mutants of a function Stale only for sites its entry never recorded
// (judged), so mutation sample and mutation except, which read only the
// mutants an entry records, judge it as before; checkFile drops them.
func checkParsed(f *lang.File, path string, judge func(path, function, hash string) bool, tests func(path string) []string, support map[string]string, exceptions []config.Exception) (FileCheck, error) {
	key := project.FromRoot(path)
	result := FileCheck{Rel: project.Rel(path), key: key, Functions: []FunctionCheck{}}
	snap, err := LoadSnapshot(key)
	if err != nil {
		return result, fmt.Errorf("%s: %w", SnapshotName(key), err)
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
	var moduleTests map[string]string
	if snap != nil && HasBroadOutcome(snap.Units) {
		if moduleTests, err = SuiteTestHashes(path, project.Root()); err != nil {
			return result, err
		}
	}
	if snap != nil {
		changed = snap.TestsChanged(now)
	}
	listed := snap.ListedChanged(project.Root(), support)
	var entries []UnitResult
	if snap != nil {
		entries = snap.Units
	}
	ids, hashes := fileKeys(f)
	pair := EntriesOf(ids, hashes, entries)
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
		c := FunctionCheck{Function: id, StartLine: u.StartLine, unit: i}
		var e *UnitResult
		if pair[i] >= 0 {
			e = &entries[pair[i]]
			c.Entry = *e
		}
		var broad []string
		if e != nil {
			for _, m := range e.Mutants {
				broad = append(broad, BroadChanges(m, moduleTests, support)...)
			}
			broad = uniqueSorted(broad)
		}
		c.Freshness = FreshnessOf(e, hashes[i], changed, listed, broad, sites[i])
		if c.judged() {
			c.Mutants = placed(e.Mutants, sites[i])
		}
		result.Functions = append(result.Functions, c)
	}
	var judgeFunction func(function string) bool
	if judge != nil {
		judgeFunction = func(function string) bool { return names[function] }
	}
	x := applyExceptions(exceptions, key, f, all, judgeFunction)
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
