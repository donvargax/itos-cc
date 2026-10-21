package domain

import (
	"context"
	"slices"

	selected "github.com/donvargax/itos-cc/internal/selection"
	"github.com/donvargax/itos-cc/internal/snapshot"
)

// plan parses each file, finds its sites, and keeps the outcomes the
// previous snapshot allows.
func plan(ctx context.Context, files []string, opt Options) ([]*Execution, error) {
	var states []*Execution
	for _, path := range files {
		f, err := selected.Parse(ctx, opt.Tree, opt.Surroundings, path)
		if err != nil {
			return states, err
		}
		rel, key := relativeName(opt.Dir, path), snapshot.Key(opt.Root, path)
		var tests []string
		if opt.Tests != nil {
			tests = opt.Tests(path)
		}
		c, failure := ObserveCommand(ctx, opt.Toolchain, CommandRequest{Path: path, Shell: opt.TestCommand, All: opt.AllTests, Tests: tests})
		if failure != nil {
			f.Close()
			return states, failure
		}

		s := &Execution{file: f, rel: rel, key: key, sites: Sites(f), command: c, currentSupport: opt.Support,
			result: &FileResult{Rel: rel}}
		states = append(states, s)
		snap, err := LoadSnapshot(ctx, opt.Store, key, opt.Toolchain)
		if err != nil {
			return states, SnapshotReadFailure{Key: key, Cause: err}
		}
		if err := s.follow(ctx, opt, path, &snap); err != nil {
			return states, err
		}
		s.tests, err = opt.Fingerprints.Tests(ctx, opt.Root, tests)
		if err != nil {
			return states, err
		}
		// Only a broad-scope outcome, judged now or recorded before,
		// depends on the Build root's tests.
		if BroadScope(ScopeForCommand(opt.TestCommand, opt.AllTests, s.command)) || snap != nil && HasBroadOutcome(snap.Units) {
			whole, err := opt.Fingerprints.Suite(ctx, path, opt.Root)
			if err != nil {
				return states, err
			}
			s.suite = SuiteOf(ctx, opt.Surroundings, opt.Root, path, whole)
		}
		s.stored, s.previous = snap, Usable(snap, s.tests)
		prev, prevScopes := RememberedWithScopes(s.previous, f)
		s.listedChange = ListedChangedWithInputs(ctx, s.stored, opt.Root, opt.Support, opt.Fingerprints)
		s.result.Functions = len(f.Units)
		var judge func(function string) bool
		if opt.Judge != nil {
			s.judged, s.result.Judged = map[int]bool{}, []string{}
			names := map[string]bool{}
			for i, u := range f.Units {
				if id := UnitID(u.Namespace, u.Name); opt.Judge(path, id, UnitHash(f, u)) {
					s.judged[i], names[id] = true, true
					s.result.Judged = append(s.result.Judged, id)
				}
			}
			// An exception names its function, so it counts when any
			// function of that name is judged.
			judge = func(function string) bool { return names[function] }
		}
		s.excepted = ApplyExceptions(opt.Exceptions, key, f, s.sites, judge)
		s.result.StaleExceptions = s.excepted.Stale
		s.outcomes = make([]string, len(s.sites))
		s.reused = make([]bool, len(s.sites))
		s.scopes = make([]string, len(s.sites))
		s.ran = make([][]string, len(s.sites))
		s.fresh = make([]freshOutcome, len(s.sites))
		scope := ScopeForCommand(opt.TestCommand, opt.AllTests, s.command)
		for i, site := range s.sites {
			// A kept outcome keeps the scope it was decided with.
			s.scopes[i] = scope
			if s.judged != nil && !s.judged[site.Unit] {
				// Not judged: it never runs, and keeps the outcome its
				// snapshot records for its unchanged function, if any.
				s.outcomes[i] = Skipped
				if outcome := prev[site.Unit][site.Key()]; outcome != "" {
					d := prevScopes[site.Unit][site.Key()]
					s.outcomes[i], s.scopes[i], s.ran[i] = outcome, d.Scope, d.Tests
				}
				continue
			}
			_, excepted := s.excepted.Held[i]
			d := prevScopes[site.Unit][site.Key()]
			// A listed outcome holds while the files it rests on do.
			restsOnChange := len(s.listedChange.Files([]Mutant{{Scope: d.Scope, Tests: d.Tests}})) > 0
			broad := Mutant{Scope: d.Scope, SuiteEvidence: d.Evidence}
			broadStale := len(BroadChanges(broad, s.suite.Of(broad.TestScope()), opt.Support)) > 0
			if outcome := prev[site.Unit][site.Key()]; outcome != "" && !restsOnChange && !broadStale {
				s.fresh[i] = freshOutcome{outcome: outcome, scope: d.Scope, tests: d.Tests}
			}
			if outcome, ok := prev.Kept(f, site, excepted); ok && !opt.MutateAll && !restsOnChange && !broadStale {
				s.outcomes[i], s.scopes[i], s.ran[i] = outcome, d.Scope, d.Tests
				s.reused[i] = true
				s.result.Reused++
			}
		}
	}
	return states, nil
}

// coverageEvidence is the measured coverage evidence of each unit of the
// file, of a strict language, as its snapshot records it: from the run's
// statement coverage, or from a matching independent cache, which wins;
// nil where neither has any.
func (s *Execution) coverageEvidence(ctx context.Context, opt Options) ([]*CoverageEvidence, error) {
	out := make([]*CoverageEvidence, len(s.file.Units))
	if !StrictCoverage(s.file.Spec.Name) {
		return out, nil
	}
	if opt.StatementCoverage != nil {
		producer, inputs := opt.CoverageProducer, map[string]string{}
		if opt.RecordCoverageInputs {
			var err error
			producer, inputs, err = CoverageInputsFor(ctx, opt.Toolchain, opt.Fingerprints, s.file.Path, opt.Root, opt.AllTests, opt.TestCommand != "", opt.Support)
			if err != nil {
				return nil, err
			}
		}
		for i, unit := range s.file.Units {
			out[i] = CoverageEvidenceFor(s.key, s.file, unit, opt.StatementCoverage, producer, inputs)
		}
	}
	if opt.CachedCoverage != nil {
		ids, hashes := FileKeys(s.file)
		for i := range s.file.Units {
			if evidence := opt.CachedCoverage(s.file.Path, ids[i], hashes[i]); evidence != nil {
				out[i] = evidence
			}
		}
	}
	return out, nil
}

// decided lists the mutants of the functions judged with their outcomes, in
// site order, each excepted survivor with its reason. A function not judged
// was not decided in this run, whatever its snapshot keeps for it. An
// exception whose mutant the tests noticed is added to the result's stale
// ones. In a fail-fast run each has its state, and one with no outcome has
// no scope either.
func (s *Execution) decided() []MutantResult {
	out := []MutantResult{}
	for i, site := range s.sites {
		u := s.file.Units[site.Unit]
		id := UnitID(u.Namespace, u.Name)
		if s.judged != nil && !s.judged[site.Unit] {
			continue
		}
		reason, stale := s.excepted.Judge(i, site, s.outcomes[i])
		if stale != nil {
			s.result.StaleExceptions = append(s.result.StaleExceptions, *stale)
		}
		var covered []string
		if s.coverage != nil && s.outcomes[i] != Uncovered {
			covered = s.coverage[i]
		}
		var ran []string
		if s.scopes[i] == ScopeListed {
			ran = s.ran[i]
		}
		scope, state := s.scopes[i], ""
		if s.failFast {
			state = s.stateOf(i)
			if s.outcomes[i] == "" {
				scope, ran = "", nil
			}
		}
		out = append(out, MutantResult{Site: site, Function: id, Outcome: s.outcomes[i], Reused: s.reused[i], Scope: scope,
			Excepted: reason, Coverage: covered, Tests: ran, State: state})
	}
	slices.SortStableFunc(out, func(a, b MutantResult) int { return LineOrder(a.Site, b.Site) })
	return out
}

// markUncovered settles pending sites on lines no test executes, and records
// which sources of coverage, and which listed tests, executed each site's
// line. A file the report never mentions is uncovered when coverage
// measured its language: a coverage command for it succeeded, even with a
// report that names no file of the run, or the report measured other files
// of the run in it. The tests ran and never loaded it. A line a listed test
// executes is never uncovered.
func markUncovered(ctx context.Context, states []*Execution, opt Options) error {
	if opt.Coverage == nil && opt.Measurement == nil {
		return nil
	}
	var sources []string
	for _, s := range states {
		sources = append(sources, s.file.Path)
	}
	measuredReport, err := opt.Measure(ctx, sources, false)
	if err != nil || measuredReport == nil {
		return err
	}
	measured := map[string]bool{}
	for _, s := range states {
		if measuredReport.Has(s.file.Path) || measuredReport.Measures(s.file.Spec.Name) {
			measured[s.file.Spec.Name] = true
		}
	}
	for _, s := range states {
		has := measuredReport.Has(s.file.Path)
		if !has && !measured[s.file.Spec.Name] {
			report(opt.Progress, RunEvent{Kind: "no-coverage", File: s.rel})
			continue
		}
		s.coverage = make([][]string, len(s.sites))
		s.reach = make([][]string, len(s.sites))
		for i, site := range s.sites {
			s.coverage[i] = measuredReport.LineSources(s.file.Path, site.Line)
			if opt.Listed != nil {
				s.reach[i] = measuredReport.LineTests(s.file.Path, site.Line)
			}
			if s.outcomes[i] != "" || len(s.reach[i]) > 0 {
				continue
			}
			covered, lineMeasured := measuredReport.LineCovered(s.file.Path, site.Line)
			if !has || (lineMeasured && !covered) {
				s.outcomes[i] = Uncovered
			}
		}
	}
	return nil
}
