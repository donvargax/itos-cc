package mutate

import (
	"context"

	"slices"
	"sort"

	"github.com/donvargax/itos-cc/project"
)

// Sample runs again count mutants drawn by seed from those the fresh
// entries of files' snapshots record as killed, timeout, or survived, all
// of them when there are fewer, after each test command's baseline, in a
// worker's copy as Run does. Fresh is as Check judges it, save that an
// entry Stale only for sites it never recorded offers the mutants it
// records. It runs no coverage and writes nothing. The draw depends only
// on the seed and on each candidate's file, function, and site, so one
// seed draws the same mutants from the same snapshots wherever the code
// moved. Each mutant runs
// in the scope its snapshot records, one baseline per scope's command,
// unless opt's TestCommand or AllTests is set, which then sets every
// mutant's. A mutant of ScopeListed runs, once its own tests survive it,
// the listed tests its outcome records, through opt's Listed, after the
// baseline of that selection, which sets its timeout, as in Run. opt's Store, Workers, TimeoutFactor, Judge, Tests, Listed, and Log
// apply as in Run; the rest is not used.
func Sample(ctx context.Context, files []string, count int, seed string, opt Options) (Sampled, error) {
	var states []*fileState
	defer func() {
		for _, s := range states {
			s.file.Close()
		}
	}()
	type candidate struct {
		state    *fileState
		site     int
		function string
		recorded string
		scope    string
		tests    []string // the listed tests it runs after its own, in ScopeListed
		rank     string
	}
	override := opt.TestCommand != "" || opt.AllTests
	var candidates []candidate
	for _, path := range files {
		f, err := project.ParseFile(ctx, path)
		if err != nil {
			return Sampled{}, err
		}
		s := &fileState{file: f, sites: Sites(f)}
		states = append(states, s)
		c, err := checkParsed(ctx, opt.Store, f, path, opt.Judge, opt.Tests, opt.Support, nil)
		if err != nil {
			return Sampled{}, err
		}
		s.rel, s.key, s.result = c.Rel, c.Key, &FileResult{Rel: c.Rel}
		at := map[int]map[string]int{}
		for i, site := range s.sites {
			if at[site.Unit] == nil {
				at[site.Unit] = map[string]int{}
			}
			at[site.Unit][site.Key()] = i
		}
		for _, fn := range c.Functions {
			if !fn.Judged() {
				continue
			}
			for _, m := range fn.Mutants {
				i, ok := at[fn.Unit][m.Key()]
				if !ok || (m.Outcome != Killed && m.Outcome != Timeout && m.Outcome != Survived) {
					continue
				}
				scope, tests := m.TestScope(), m.Tests
				if override {
					scope, tests = RunScope(opt.TestCommand, opt.AllTests), nil
				}
				candidates = append(candidates, candidate{state: s, site: i, function: fn.Function, recorded: m.Outcome,
					scope: scope, tests: tests, rank: rank(seed, s.key, fn.Function, m.Key())})
			}
		}
	}
	result := Sampled{Cached: len(candidates), Files: []SampledFile{}}
	sort.SliceStable(candidates, func(a, b int) bool { return candidates[a].rank < candidates[b].rank })
	candidates = candidates[:min(count, len(candidates))]
	if len(candidates) == 0 {
		return result, nil
	}
	// Each file's drawn mutants run in one state per scope, with that
	// scope's command, so execute runs each command's baseline once.
	type scoped struct {
		state *fileState
		scope string
	}
	runs := map[scoped]*fileState{}
	var order []*fileState
	drawn := map[*fileState][]candidate{}
	for _, c := range candidates {
		k := scoped{c.state, c.scope}
		r := runs[k]
		if r == nil {
			s := c.state
			var tests []string
			if opt.Tests != nil {
				tests = opt.Tests(s.file.Path)
			}
			r = &fileState{file: s.file, rel: s.rel, key: s.key, sites: s.sites, command: scopeCommand(ctx, s.file.Path, c.scope, tests),
				result: &FileResult{Rel: s.rel}, outcomes: slices.Repeat([]string{skipped}, len(s.sites)),
				scopes: make([]string, len(s.sites)), ran: make([][]string, len(s.sites))}
			if c.scope == ScopeListed {
				r.reach = make([][]string, len(s.sites))
			}
			runs[k] = r
			order = append(order, r)
		}
		r.outcomes[c.site] = ""
		if r.reach != nil {
			r.reach[c.site] = c.tests
		}
		drawn[c.state] = append(drawn[c.state], c)
	}
	if _, err := execute(ctx, order, opt, nil); err != nil {
		return result, stopped(ctx, err)
	}
	if err := stopped(ctx, nil); err != nil {
		return result, err
	}
	for _, s := range states {
		if drawn[s] == nil {
			continue
		}
		f := SampledFile{Rel: s.rel, Mutants: []SampledMutant{}}
		failed := &FileResult{}
		for _, c := range drawn[s] {
			r := runs[scoped{s, c.scope}]
			if r.result.BaselineFailed && !f.BaselineFailed {
				f.BaselineFailed, f.BaselineOutput = true, r.result.BaselineOutput
			}
			for _, sel := range r.result.FailedSelections {
				failed.addFailed(&sel)
			}
		}
		f.FailedSelections = failed.FailedSelections
		if f.BaselineFailed || len(f.FailedSelections) > 0 {
			result.Files = append(result.Files, f)
			continue
		}
		for _, c := range drawn[s] {
			outcome := runs[scoped{s, c.scope}].outcomes[c.site]
			f.Mutants = append(f.Mutants, SampledMutant{Site: s.sites[c.site], Function: c.function, Recorded: c.recorded,
				Outcome: outcome, Scope: c.scope})
		}
		sort.Slice(f.Mutants, func(a, b int) bool {
			ma, mb := f.Mutants[a], f.Mutants[b]
			if ma.Line != mb.Line {
				return ma.Line < mb.Line
			}
			if ma.Column != mb.Column {
				return ma.Column < mb.Column
			}
			return ma.Replacement < mb.Replacement
		})
		result.Files = append(result.Files, f)
	}
	return result, nil
}
