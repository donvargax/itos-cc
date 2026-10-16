package mutants

import (
	"context"
	"errors"

	langport "github.com/donvargax/itos-cc/internal/lang/port"
	selected "github.com/donvargax/itos-cc/internal/selection"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
	"github.com/donvargax/itos-cc/internal/snapshot"
)

// CheckOptions binds read-only observations and pure selection facts.
type CheckOptions struct {
	Store        MutationSnapshotReader
	Tree         selport.Tree
	Surroundings langport.Surroundings
	Toolchain    Toolchain
	Fingerprints Fingerprints
	Root, Dir    string
	Judge        func(path, function, hash string) bool
	Tests        func(path string) []string
	Support      map[string]string
	Exceptions   []Exception
}

func (opt CheckOptions) bound() error {
	if opt.Tree == nil || opt.Surroundings == nil {
		return errors.New("mutation source tree binding is missing")
	}
	return opt.observationsBound()
}
func (opt CheckOptions) observationsBound() error {
	if opt.Store == nil {
		return errors.New("mutation snapshots reader binding is missing")
	}
	if opt.Toolchain == nil {
		return errors.New("mutation Toolchain binding is missing")
	}
	if opt.Fingerprints == nil {
		return errors.New("mutation Fingerprints binding is missing")
	}
	return nil
}

// Check compares each admitted function with mutation sites against its stored
// snapshot, running and writing nothing. Functions sharing a name are paired by
// hash, so a moved function remains fresh. Own-test, listed-test and broad-suite
// evidence must still hold. A current site absent from its entry is unrecorded;
// a recorded site no longer present is left out of both mutants and counts.
// Exceptions apply only to judged functions and retain stale exception reasons.
// Only this entry hides stale mutants; CheckParsed retains judged ones for
// Sample and exception operations.
func Check(ctx context.Context, files []string, opt CheckOptions) ([]FileCheck, error) {
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	if err := opt.bound(); err != nil {
		return nil, err
	}
	var out []FileCheck
	for _, path := range files {
		f, err := selected.Parse(ctx, opt.Tree, opt.Surroundings, path)
		if err != nil {
			return out, err
		}
		c, err := CheckParsed(ctx, f, path, opt)
		f.Close()
		for i := range c.Functions {
			if fn := &c.Functions[i]; fn.State != Fresh {
				fn.Mutants = nil
			}
		}
		if err != nil {
			return out, err
		}
		out = append(out, c)
	}
	return out, stopped(ctx, nil)
}

// CheckCoverage inventories every admitted function, including zero-site units.
// Unsupported scope, stored evidence and current inputs are observed in order.
func CheckCoverage(ctx context.Context, files []string, opt CheckOptions, languages map[string]bool, all, shell bool) ([]CoverageCheck, error) {
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	if err := opt.bound(); err != nil {
		return nil, err
	}
	var out []CoverageCheck
	for _, path := range files {
		f, err := selected.Parse(ctx, opt.Tree, opt.Surroundings, path)
		if err != nil {
			return out, err
		}
		if f.Spec == nil || !languages[f.Spec.Name] {
			f.Close()
			continue
		}
		language := f.Spec.Name
		unsupported, failure := opt.Fingerprints.Unsupported(ctx, language, path, opt.Root)
		err = failure
		if err != nil {
			f.Close()
			return out, err
		}
		key := snapshot.Key(opt.Root, absoluteSource(opt.Dir, path))
		snap, err := LoadSnapshot(ctx, opt.Store, key, opt.Toolchain)
		if err != nil {
			f.Close()
			return out, SnapshotReadFailure{Key: key, Cause: err}
		}
		_, inputs, err := CoverageInputsFor(ctx, opt.Toolchain, opt.Fingerprints, path, opt.Root, all, shell, opt.Support)
		if err != nil {
			f.Close()
			return out, err
		}
		var entries []UnitResult
		if snap != nil {
			entries = snap.Units
		}
		ids, hashes := FileKeys(f)
		pair := EntriesOf(ids, hashes, entries)
		for i, unit := range f.Units {
			if opt.Judge != nil && !opt.Judge(path, ids[i], hashes[i]) {
				continue
			}
			check := CoverageCheck{File: relativeName(opt.Dir, path), Function: ids[i], Language: language, Line: unit.StartLine, EndLine: unit.EndLine, State: "missing"}
			if unsupported != "" {
				check.State = "unsupported"
				check.Changed = []string{unsupported}
				out = append(out, check)
				continue
			}
			if pair[i] >= 0 {
				check = CoverageFreshness(check, key, hashes[i], entries[pair[i]].Coverage, inputs)
			}
			out = append(out, check)
		}
		f.Close()
	}
	return out, stopped(ctx, nil)
}
