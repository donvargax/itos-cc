package domain

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	selport "github.com/donvargax/itos-cc/internal/selection/port"
)

// FreshTrial is one selected mutant to judge with the own-test command whose
// clean baseline preparation ran, then, when it survives that, with the
// listed tests that reach its line. A file whose own tests run nothing
// (Command.RunsNothing), as no own test reaches it, has a Baseline with no
// command and never ran: its mutant goes straight to the listed tests.
type FreshTrial struct {
	Candidate FreshCandidate
	Baseline  FreshBaseline
	// Reach is the IDs of the listed tests whose measured coverage reaches
	// the mutant's line; none when no listed test does.
	Reach []string
}

// FreshTrialResult is how one fresh trial went. Outcome is Killed,
// Survived, Timeout or OverLimit, with Limit the cap it went over, when its
// tests ran to an end; it is empty when they could not:
// with Err set, a parent cancellation included, or with FailedSelection set
// when the listed tests it needed fail without any mutant. Such a mutant has
// no outcome. Stages is each stage of the trial that ran, in order.
type FreshTrialResult struct {
	Trial   FreshTrial
	Outcome string
	Limit   string
	// Limits are those of the run that decided Outcome.
	Limits Limits
	// Scope is the scope of the tests that decided Outcome: the own-test
	// scope of the options, or ScopeListed, with Tests the listed IDs.
	Scope           string
	Tests           []string
	Stages          []FreshStage
	FailedSelection *FailedSelection
	Elapsed         time.Duration
	Err             error
}

// FreshStage is one stage of a fresh trial: "own", the mutant under its own
// tests; "listed-baseline", the clean baseline of the listed selection it
// needs, run once per run; and "listed", the mutant under that selection.
// State is "complete", "failed", or "aborted" when a run abort cut it off;
// a stage that never ran is not listed.
type FreshStage struct {
	Name    string
	State   string
	Outcome string
	Tests   []string
	Error   string
}

// FreshTrialOptions control RunFreshTrials.
type FreshTrialOptions struct {
	Workers       int
	WorkerFactory Workers
	TimeoutFactor float64
	// OutputFactor and MemoryFactor size each mutant's output and memory
	// caps from its baseline's, as TimeoutFactor its timeout (Factors).
	OutputFactor, MemoryFactor float64
	// Scope is the scope of the own-test command, RunScope's; a file whose
	// own tests run the whole suite judges in OwnScope's.
	Scope string
	// Listed runs a selection of the listed tests at the frozen root; nil
	// when the frozen config lists none.
	Listed *Listed
	// Tree observes actual frozen bytes after preparation, never plan-time copies.
	Tree      selport.Tree
	Toolchain Toolchain
	Progress  Progress
}

// RunFreshTrials applies each selected mutant to a private worker copy of
// the frozen root and runs its own tests once, under the same owned process
// supervision as complete runs, workers at a time; a survivor that listed
// tests reach then runs them, as complete runs do, after their selection's
// clean baseline the first time any mutant needs it. Both stages are one
// trial. A mutant times out after its baseline's time times the factor,
// plus the usual allowance, and its output and memory are capped from its
// baseline's (Factors). Results are in the order of trials. Nothing is
// written outside the worker copies, which are removed before it returns.
func RunFreshTrials(ctx context.Context, plan *FreshPlan, trials []FreshTrial, opt FreshTrialOptions) (results []FreshTrialResult, resultErr error) {
	if plan == nil || plan.FrozenRoot == "" {
		return nil, errors.New("fresh trials need a live frozen plan")
	}
	results = make([]FreshTrialResult, len(trials))
	if len(trials) == 0 {
		return results, nil
	}
	factory := opt.WorkerFactory
	if factory == nil || opt.Tree == nil || opt.Toolchain == nil {
		return nil, errors.New("fresh trial bindings are missing")
	}
	count := min(max(1, opt.Workers), len(trials))
	pool, err := factory.Open(ctx, FrozenTrials, count)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeFailure := pool.Close(ctx); closeFailure != nil {
			resultErr = errors.Join(resultErr, closeFailure)
			if cause := context.Cause(ctx); cause != nil {
				resultErr = errors.Join(cause, resultErr)
			}
		}
	}()

	workers := make([]Worker, count)
	for i := range workers {
		workers[i] = pool.Worker(i)
	}

	var sels *SelectionBaselines
	if opt.Listed != nil {
		sels = NewSelectionBaselines(opt.Listed, opt.factors(), opt.Progress)
	}
	for _, trial := range trials {
		if !trial.Baseline.Command.RunsNothing() && !trial.Baseline.MemoryMeasured {
			// No memory cap holds on this system: said once.
			report(opt.Progress, RunEvent{Kind: MemoryUnmeasured})
			break
		}
	}
	queue := make(chan int)
	var mu sync.Mutex
	done := 0
	var wg sync.WaitGroup
	for _, w := range workers {
		wg.Add(1)
		go func(ctx context.Context, w Worker) {
			defer wg.Done()
			for i := range queue {
				r := runFreshTrial(ctx, w, plan, trials[i], opt, sels)
				mu.Lock()
				results[i] = r
				done++
				if opt.Progress != nil {
					opt.Progress.Report(RunEvent{Kind: "fresh-trial", File: trials[i].Candidate.Path, Site: trials[i].Candidate.Site, Done: done, Total: len(trials), FreshTrial: &r})
				}

				mu.Unlock()
			}
		}(ctx, w)
	}
	for i := range trials {
		if ctx.Err() != nil {
			results[i] = FreshTrialResult{Trial: trials[i], Err: ctx.Err()}
			continue
		}
		queue <- i
	}
	close(queue)
	wg.Wait()
	return results, nil
}

// runFreshTrial runs one trial in w: the mutant under its own tests, then,
// when it survives them and listed tests reach its line, the selection's
// clean baseline, if no mutant needed it yet, and the mutant under the
// selection. A selection whose baseline fails decides nothing.
func runFreshTrial(ctx context.Context, w Worker, plan *FreshPlan, trial FreshTrial, opt FreshTrialOptions, sels *SelectionBaselines) FreshTrialResult {
	out := FreshTrialResult{Trial: trial}
	if err := ctx.Err(); err != nil {
		out.Err = err
		return out
	}
	c := trial.Baseline.Command
	switch {
	case c.RunsNothing() && len(trial.Reach) == 0:
		out.Err = errors.New("no own test reaches its file, and no listed test reaches its line")
		return out
	case !c.RunsNothing() && len(c.Args) == 0:
		out.Err = errors.New("no own-test command")
		return out
	}
	site := trial.Candidate.Site
	path := filepath.Join(plan.FrozenRoot, filepath.FromSlash(trial.Candidate.Path))
	src, readFailure := opt.Tree.Read(ctx, path)
	if readFailure != nil {
		out.Err = readFailure
		return out
	}
	if int(site.End) > len(src) || string(src[site.Start:site.End]) != site.Original {
		out.Err = fmt.Errorf("frozen %s no longer holds %q at its selected site", trial.Candidate.Path, site.Original)
		return out
	}
	// A file no own test reaches has no own stage: the mutant survives its
	// own tests without running them, as in a complete run, and the listed
	// tests that reach its line judge it.
	own := Survived
	if !c.RunsNothing() {
		limits := opt.factors().LimitsOf(trial.Baseline.Usage())
		r, err := w.Run(ctx, c, &Mutation{Path: path, Source: src, Site: site}, limits)
		out.Elapsed = r.Elapsed
		if out.Err = stageError(ctx, r, err); out.Err != nil {
			out.Stages = append(out.Stages, FreshStage{Name: "own", State: failedState(ctx), Error: out.Err.Error()})
			return out
		}
		own = Judged(r)
		out.Limit, out.Limits = r.OverLimit, limits
		out.Stages = append(out.Stages, FreshStage{Name: "own", State: "complete", Outcome: own})
	}
	if own != Survived || len(trial.Reach) == 0 {
		out.Outcome, out.Scope = own, opt.Scope
		if opt.Scope == ScopeOwn {
			command, failure := ObserveCommand(ctx, opt.Toolchain, CommandRequest{Path: path})
			if failure != nil {
				out.Err = failure
				return out
			}
			out.Scope = ScopeForCommand("", false, command)
		}
		return out
	}
	if sels == nil {
		out.Err = fmt.Errorf("listed tests %s reach its line, and no listed tests are configured to run them", strings.Join(trial.Reach, " "))
		return out
	}
	ids := slices.Clone(trial.Reach)
	limits, failed, selectionErr := sels.Baseline(ctx, w, ids)
	switch {
	case selectionErr != nil:
		out.Err = selectionErr
		out.Stages = append(out.Stages, FreshStage{Name: "listed-baseline", State: failedState(ctx), Tests: ids, Error: selectionErr.Error()})
		return out
	case failed != nil:
		out.FailedSelection = failed
		out.Stages = append(out.Stages, FreshStage{Name: "listed-baseline", State: "failed", Tests: ids,
			Error: fmt.Sprintf("the listed tests fail without any mutant (exit %d)", failed.ExitCode)})
		return out
	}
	out.Stages = append(out.Stages, FreshStage{Name: "listed-baseline", State: "complete", Tests: ids})
	listed, err := w.Run(ctx, opt.Listed.Command(ids), &Mutation{Path: path, Source: src, Site: site}, limits)
	out.Elapsed += listed.Elapsed
	if out.Err = stageError(ctx, listed, err); out.Err != nil {
		out.Stages = append(out.Stages, FreshStage{Name: "listed", State: failedState(ctx), Tests: ids, Error: out.Err.Error()})
		return out
	}
	out.Outcome, out.Limit, out.Limits, out.Scope, out.Tests = Judged(listed), listed.OverLimit, limits, ScopeListed, ids
	out.Stages = append(out.Stages, FreshStage{Name: "listed", State: "complete", Outcome: out.Outcome, Tests: ids})
	return out
}

// factors size the trials' limits from their baselines'.
func (opt FreshTrialOptions) factors() Factors {
	return Factors{Timeout: opt.TimeoutFactor, Output: opt.OutputFactor, Memory: opt.MemoryFactor}
}

// RanOwn says whether the trial ran the mutant under its own tests: not
// for a file no own test reaches, which the listed tests alone judge.
func (r FreshTrialResult) RanOwn() bool {
	return len(r.Stages) > 0 && r.Stages[0].Name == "own"
}

// stageError is why a stage of a fresh trial has no outcome: the error its
// command gave, or its cancellation; nil when its tests ran to an end.
func stageError(ctx context.Context, r CommandResult, err error) error {
	switch {
	case err != nil:
		return err
	case r.Cancelled || ctx.Err() != nil:
		return fmt.Errorf("cancelled: %w", context.Cause(ctx))
	}
	return nil
}

// failedState is the state of a stage that ended without an outcome:
// "aborted" once ctx, the run's, is cancelled, "failed" otherwise.
func failedState(ctx context.Context) string {
	if ctx.Err() != nil {
		return "aborted"
	}
	return "failed"
}
