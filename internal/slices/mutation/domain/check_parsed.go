package domain

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/snapshot"
)

// CheckParsed is checkFile of f, the file at path, parsed. It leaves the
// Mutants of a function Stale only for sites its entry never recorded
// (judged), so mutation sample and mutation except, which read only the
// mutants an entry records, judge it as before; checkFile drops them.
func CheckParsed(ctx context.Context, f *lang.File, path string, opt CheckOptions) (FileCheck, error) {
	if ctx.Err() != nil {
		return FileCheck{}, context.Cause(ctx)
	}
	if err := opt.observationsBound(); err != nil {
		return FileCheck{}, err
	}
	key := snapshot.Key(opt.Root, absoluteSource(opt.Dir, path))
	result := FileCheck{Rel: relativeName(opt.Dir, path), Key: key, Functions: []FunctionCheck{}}
	snap, err := LoadSnapshot(ctx, opt.Store, key, opt.Toolchain)
	if err != nil {
		return result, SnapshotReadFailure{Key: key, Cause: err}
	}
	var paths []string
	if opt.Tests != nil {
		paths = opt.Tests(path)
	}
	now, err := opt.Fingerprints.Tests(ctx, opt.Root, paths)
	if err != nil {
		return result, err
	}
	var changed *TestChange
	var moduleTests map[string]string
	if snap != nil && HasBroadOutcome(snap.Units) {
		moduleTests, err = opt.Fingerprints.Suite(ctx, path, opt.Root)
		if err != nil {
			return result, err
		}
	}
	if snap != nil {
		changed = snap.TestsChanged(now)
	}
	listed := ListedChangedWithInputs(ctx, snap, opt.Root, opt.Support, opt.Fingerprints)
	var entries []UnitResult
	if snap != nil {
		entries = snap.Units
	}
	ids, hashes := FileKeys(f)
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
		if opt.Judge != nil && !opt.Judge(path, id, hashes[i]) {
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
				broad = append(broad, BroadChanges(m, moduleTests, opt.Support)...)
			}
			broad = UniqueSorted(broad)
		}
		c.Freshness = FreshnessOf(e, hashes[i], changed, listed, broad, sites[i])
		if c.State == SnapshotFresh {
			c.LeftOut = leftOut
		}
		if c.Judged() {
			c.Mutants = Placed(e.Mutants, sites[i])
		}
		result.Functions = append(result.Functions, c)
	}
	var judgeFunction func(function string) bool
	if opt.Judge != nil {
		judgeFunction = func(function string) bool { return names[function] }
	}
	x := ApplyExceptions(opt.Exceptions, key, f, all, judgeFunction)
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

func absoluteSource(dir, source string) string {
	if filepath.IsAbs(source) {
		return source
	}
	return filepath.Join(dir, source)
}
