package mutate

import (
	"context"
	"fmt"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/mutants"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
	"github.com/donvargax/itos-cc/project"
)

// CoverageInputs returns the current producer and input fingerprint of the
// strict coverage evidence of the source at path, reading under ctx.
type CoverageInputs func(ctx context.Context, path string) (string, map[string]string, error)

// CheckGoCoverage is CheckCoverage of Go files alone.
func CheckGoCoverage(ctx context.Context, store snapport.Store, files []string, judge func(path, function, hash string) bool, current CoverageInputs) ([]CoverageCheck, error) {
	return CheckCoverage(ctx, store, files, judge, map[string]CoverageInputs{"go": current})
}

// CheckCoverage checks independent coverage evidence for every selected
// function of a language current has inputs for, including functions with
// no mutation sites. It runs and writes nothing. current returns, by
// language, the current producer/input fingerprint per file. A file of
// another language is outside every evidence inventory, beneath a go.mod or
// not: it has no verdict here, and current is never asked about it. Each
// file's snapshot is read from store.
func CheckCoverage(ctx context.Context, store snapport.Store, files []string, judge func(path, function, hash string) bool, current map[string]CoverageInputs) ([]CoverageCheck, error) {
	var out []CoverageCheck
	for _, path := range files {
		f, err := project.ParseFile(ctx, path)
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
			unsupported, err = LineCoverageUnsupported(ctx, language, path, project.Root())
		}
		if err != nil {
			f.Close()
			return out, err
		}
		key := project.FromRoot(path)
		snap, err := LoadSnapshotOf(ctx, store, key)
		if err != nil {
			f.Close()
			return out, fmt.Errorf("%s: %w", SnapshotName(key), err)
		}
		_, inputs, err := current[language](ctx, path)
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
			check := CoverageCheck{File: project.Rel(path), Function: ids[i], Language: language, Line: unit.StartLine, EndLine: unit.EndLine, State: "missing"}
			if unsupported != "" {
				check.State = "unsupported"
				check.Changed = []string{unsupported}
				out = append(out, check)
				continue
			}
			if pair[i] >= 0 {
				check = mutants.CoverageFreshness(check, key, hashes[i], entries[pair[i]].Coverage, inputs)
			}
			out = append(out, check)
		}
		f.Close()
	}
	return out, stopped(ctx, nil)
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
// them. A mutant an entry of the function's hash records at a site the
// function no longer holds is left out, of its mutants and its counts
// (OnSites), and a fresh function counts them in LeftOut.
func Check(ctx context.Context, store snapport.Store, files []string, judge func(path, function, hash string) bool, tests func(path string) []string, support map[string]string, exceptions []config.Exception) ([]FileCheck, error) {
	var out []FileCheck
	for _, path := range files {
		c, err := checkFile(ctx, store, path, judge, tests, support, exceptions)
		if err != nil {
			return out, err
		}
		out = append(out, c)
	}
	return out, stopped(ctx, nil)
}

func checkFile(ctx context.Context, store snapport.Store, path string, judge func(path, function, hash string) bool, tests func(path string) []string, support map[string]string, exceptions []config.Exception) (FileCheck, error) {
	f, err := project.ParseFile(ctx, path)
	if err != nil {
		return FileCheck{}, err
	}
	defer f.Close()
	c, err := checkParsed(ctx, store, f, path, judge, tests, support, exceptions)
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
func checkParsed(ctx context.Context, store snapport.Store, f *lang.File, path string, judge func(path, function, hash string) bool, tests func(path string) []string, support map[string]string, exceptions []config.Exception) (FileCheck, error) {
	key := project.FromRoot(path)
	result := FileCheck{Rel: project.Rel(path), Key: key, Functions: []FunctionCheck{}}
	snap, err := LoadSnapshotOf(ctx, store, key)
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
		if moduleTests, err = SuiteTestHashes(ctx, path, project.Root()); err != nil {
			return result, err
		}
	}
	if snap != nil {
		changed = snap.TestsChanged(now)
	}
	listed := ListedChanged(snap, project.Root(), support)
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
		c := FunctionCheck{Function: id, StartLine: u.StartLine, Unit: i}
		var e *UnitResult
		var leftOut int
		if pair[i] >= 0 {
			e = OnSites(&entries[pair[i]], hashes[i], sites[i])
			c.Entry = *e
			leftOut = len(entries[pair[i]].Mutants) - len(e.Mutants)
		}
		var broad []string
		if e != nil {
			for _, m := range e.Mutants {
				broad = append(broad, BroadChanges(m, moduleTests, support)...)
			}
			broad = uniqueSorted(broad)
		}
		c.Freshness = FreshnessOf(e, hashes[i], changed, listed, broad, sites[i])
		if c.State == Fresh {
			c.LeftOut = leftOut
		}
		if c.Judged() {
			c.Mutants = placed(e.Mutants, sites[i])
		}
		result.Functions = append(result.Functions, c)
	}
	var judgeFunction func(function string) bool
	if judge != nil {
		judgeFunction = func(function string) bool { return names[function] }
	}
	x := applyExceptions(exceptions, key, f, all, judgeFunction)
	result.StaleExceptions = x.Stale
	at := map[string]int{}
	for i, s := range all {
		at[fmt.Sprint(s.Unit, "\x00", s.Key())] = i
	}
	for _, fn := range result.Functions {
		for j := range fn.Mutants {
			m := &fn.Mutants[j]
			i, ok := at[fmt.Sprint(fn.Unit, "\x00", m.Key())]
			if !ok {
				continue
			}
			reason, stale := x.Judge(i, all[i], m.Outcome)
			m.Excepted = reason
			if stale != nil {
				result.StaleExceptions = append(result.StaleExceptions, *stale)
			}
		}
	}
	return result, nil
}
