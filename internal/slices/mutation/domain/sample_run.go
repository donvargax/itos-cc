package domain

import (
	"context"
	"errors"
	"sort"

	"github.com/donvargax/itos-cc/internal/lang"
)

// Sample runs again count mutants drawn by seed from those the fresh
// entries of files' snapshots record as killed, timeout, over-limit, or
// survived, all
// of them when there are fewer, after each test command's baseline, in a
// worker's copy as Run does. Freshness is as Check judges it, save that an
// entry Stale only for sites it never recorded offers the mutants it
// records. It runs no coverage and writes nothing. The draw depends only
// on the seed and on each candidate's file, function, and site, so one
// seed draws the same mutants from the same snapshots wherever the code
// moved. Each mutant runs
// in the scope its snapshot records, one baseline per scope's command,
// unless opt's TestCommand or AllTests is set, which then sets every
// mutant's. A mutant whose outcome disagrees with the one recorded runs
// again, in the same worker and scope, up to Reruns times, until one run
// agrees (Classify); with opt's Repeat it runs that many times first, and
// is flaky when those outcomes are not all alike (Repeated); each keeps
// every outcome in run order. A mutant of
// ScopeListed runs, once its own tests survive it,
// the listed tests its outcome records, through opt's Listed, after the
// baseline of that selection, which sets its timeout, as in Run. Its read
// and execution bindings apply as in Run; it holds no publication capability.
func Sample(ctx context.Context, files []string, count int, seed string, opt SampleOptions) (Sampled, error) {
	if ctx.Err() != nil {
		return Sampled{}, context.Cause(ctx)
	}
	if err := opt.bound(); err != nil {
		return Sampled{}, err
	}
	type sampleSource struct {
		file     *lang.File
		sites    []Site
		rel, key string
	}

	var states []*sampleSource
	defer func() {
		for _, s := range states {
			s.file.Close()
		}
	}()
	type candidate struct {
		state    *sampleSource
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
		f, err := parseExceptionSource(ctx, path, opt.CheckOptions)
		if err != nil {
			return Sampled{}, err
		}
		s := &sampleSource{file: f, sites: Sites(f)}
		states = append(states, s)
		c, err := CheckParsed(ctx, f, path, opt.CheckOptions)
		if err != nil {
			return Sampled{}, err
		}
		s.rel, s.key = c.Rel, c.Key
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
				if !ok || (!Noticed(m.Outcome) && m.Outcome != Survived) {
					continue
				}
				scope, tests := m.TestScope(), m.Tests
				if override {
					scope, tests = RunScope(opt.TestCommand, opt.AllTests), nil
				}
				candidates = append(candidates, candidate{state: s, site: i, function: fn.Function, recorded: m.Outcome,
					scope: scope, tests: tests, rank: Rank(seed, s.key, fn.Function, m.Key())})
			}
		}
	}
	result := Sampled{Cached: len(candidates), Files: []SampledFile{}}
	sort.SliceStable(candidates, func(a, b int) bool { return candidates[a].rank < candidates[b].rank })
	candidates = candidates[:min(count, len(candidates))]
	if len(candidates) == 0 {
		return result, nil
	}
	if opt.WorkerFactory == nil {
		return result, errors.New("mutation Workers binding is missing")
	}
	// Each file's drawn mutants run in one state per scope, with that
	// scope's command, so execute runs each command's baseline once.
	type scoped struct {
		state *sampleSource
		scope string
	}
	runs := map[scoped]*Execution{}
	var order []*Execution
	drawn := map[*sampleSource][]candidate{}
	for _, c := range candidates {
		k := scoped{c.state, c.scope}
		r := runs[k]
		if r == nil {
			s := c.state
			var tests []string
			if opt.Tests != nil {
				tests = opt.Tests(s.file.Path)
			}
			command, err := scopeCommand(ctx, s.file.Path, c.scope, tests, opt.Toolchain)
			if err != nil {
				return result, err
			}
			r = NewExecution(s.file, s.rel, s.key, command)
			r.Repeat(opt.Repeat)
			runs[k] = r
			order = append(order, r)
		}
		ids := c.tests
		if c.scope != ScopeListed {
			ids = nil
		}
		r.Pending(c.site, ids)
		r.Recheck(c.site, c.recorded)
		drawn[c.state] = append(drawn[c.state], c)
	}
	if _, err := Execute(ctx, order, Options{WorkerFactory: opt.WorkerFactory, Workers: opt.Workers, TimeoutFactor: opt.TimeoutFactor,
		OutputFactor: opt.OutputFactor, MemoryFactor: opt.MemoryFactor, Listed: opt.Listed, Progress: opt.Progress}, nil); err != nil {
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
			if r.Result().BaselineFailed && !f.BaselineFailed {
				f.BaselineFailed, f.BaselineOutput = true, r.Result().BaselineOutput
				f.BaselineCopy, f.BaselineLeftOut = r.Result().BaselineCopy, r.Result().BaselineLeftOut
			}
			for _, sel := range r.Result().FailedSelections {
				failed.AddFailed(&sel)
			}
		}
		f.FailedSelections = failed.FailedSelections
		if f.BaselineFailed || len(f.FailedSelections) > 0 {
			result.Files = append(result.Files, f)
			continue
		}
		for _, c := range drawn[s] {
			outcomes := runs[scoped{s, c.scope}].Outcomes(c.site)
			f.Mutants = append(f.Mutants, SampledMutant{Site: s.sites[c.site], Function: c.function, Recorded: c.recorded,
				Outcome: outcomes[0], Outcomes: outcomes, Scope: c.scope, Repeat: opt.Repeat})
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

// SampleOptions grants read-only inputs and execution, never publication authority.
type SampleOptions struct {
	CheckOptions
	WorkerFactory Workers
	Workers       int
	TimeoutFactor float64
	// OutputFactor and MemoryFactor size each mutant's output and memory
	// caps from its baseline's, as TimeoutFactor its timeout (Factors).
	OutputFactor, MemoryFactor float64
	TestCommand                string
	AllTests                   bool
	Listed                     *Listed
	Progress                   Progress
	// Repeat is how many times each sampled mutant runs at least,
	// --repeat; below 2 it runs once, unless it disagrees.
	Repeat int
}

// scopeCommand preserves the scope that actually decided a recorded outcome.
func scopeCommand(ctx context.Context, path, scope string, tests []string, tools Toolchain) (Command, error) {
	request := CommandRequest{Path: path, Tests: tests}
	switch scope {
	case ScopeOwn, ScopeListed, "":
	case ScopeAllTests:
		request.All = true
		request.Tests = nil
	default:
		request.Shell = scope
		request.Tests = nil
	}
	return ObserveCommand(ctx, tools, request)
}
