package mutate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// FreshTrial is one selected mutant to judge with the own-test command whose
// clean baseline preparation ran, then, when it survives that, with the
// listed tests that reach its line.
type FreshTrial struct {
	Candidate FreshCandidate
	Baseline  FreshBaseline
	// Reach is the IDs of the listed tests whose measured coverage reaches
	// the mutant's line; none when no listed test does.
	Reach []string
}

// FreshTrialResult is how one fresh trial went. Outcome is Killed, Survived
// or Timeout when its tests ran to an end; it is empty when they could not:
// with Err set, a parent cancellation included, or with FailedSelection set
// when the listed tests it needed fail without any mutant. Such a mutant has
// no outcome. Stages is each stage of the trial that ran, in order.
type FreshTrialResult struct {
	Trial   FreshTrial
	Outcome string
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
	TimeoutFactor float64
	// Scope is the scope of the own-test command, RunScope's; a file whose
	// own tests run the whole suite judges in OwnScope's.
	Scope string
	// Listed runs a selection of the listed tests at the frozen root; nil
	// when the frozen config lists none.
	Listed *Listed
	// Prepare adjusts each command before it runs, such as its environment.
	Prepare func(*exec.Cmd)
	// Owner, when set, is the executor whose run-abort cleanup deadline the
	// trials share, so that preparation and trials have one deadline after
	// a run abort; unset, the trials have one of their own.
	Owner *OwnedExecutor
	Log   io.Writer
}

// RunFreshTrials applies each selected mutant to a private worker copy of
// the frozen root and runs its own tests once, under the same owned process
// supervision as complete runs, workers at a time; a survivor that listed
// tests reach then runs them, as complete runs do, after their selection's
// clean baseline the first time any mutant needs it. Both stages are one
// trial. A mutant times out after its baseline's time times the factor,
// plus the usual allowance. Results are in the order of trials. Nothing is
// written outside the worker copies, which are removed before it returns.
func RunFreshTrials(ctx context.Context, plan *FreshPlan, trials []FreshTrial, opt FreshTrialOptions) ([]FreshTrialResult, error) {
	if ctx == nil {
		return nil, errors.New("fresh trials need a context")
	}
	if plan == nil || plan.FrozenRoot == "" {
		return nil, errors.New("fresh trials need a live frozen plan")
	}
	log := opt.Log
	if log == nil {
		log = io.Discard
	}
	results := make([]FreshTrialResult, len(trials))
	if len(trials) == 0 {
		return results, nil
	}
	base, err := os.MkdirTemp("", "itos-cc-fresh-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(base)
	budget := &cleanupBudget{} // one run-abort deadline, shared by every worker
	if opt.Owner != nil {
		budget = &opt.Owner.cleanup
	}
	workers := make([]*worker, min(max(1, opt.Workers), len(trials)))
	for i := range workers {
		workers[i] = &worker{dir: filepath.Join(base, fmt.Sprintf("w%d", i)), copies: map[string]string{},
			runner: commandRunner{cleanupBudget: budget}, prepare: opt.Prepare}
	}
	var sels *selections
	if opt.Listed != nil {
		sels = newSelections(opt.Listed, opt.TimeoutFactor, log)
	}
	queue := make(chan int)
	var mu sync.Mutex
	done := 0
	var wg sync.WaitGroup
	for _, w := range workers {
		wg.Add(1)
		go func(w *worker) {
			defer wg.Done()
			for i := range queue {
				r := runFreshTrial(ctx, w, plan, trials[i], opt, sels)
				mu.Lock()
				results[i] = r
				done++
				c := trials[i].Candidate
				said := r.Outcome
				switch {
				case r.Err != nil:
					said = "not judged: " + r.Err.Error()
				case r.FailedSelection != nil:
					said = fmt.Sprintf("survived its own tests, then not judged: %s fail without any mutant", strings.Join(r.FailedSelection.IDs, " "))
				case r.Scope == ScopeListed:
					said = fmt.Sprintf("survived its own tests, then %s with %s", r.Outcome, strings.Join(r.Tests, " "))
				}
				fmt.Fprintf(log, "itos-cc: [%d/%d] %s:%d %s → %s %s (%.1fs)\n", done, len(trials),
					c.Path, c.Site.Line, show(c.Site.Original), show(c.Site.Replacement), said, r.Elapsed.Seconds())
				mu.Unlock()
			}
		}(w)
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
func runFreshTrial(ctx context.Context, w *worker, plan *FreshPlan, trial FreshTrial, opt FreshTrialOptions, sels *selections) FreshTrialResult {
	out := FreshTrialResult{Trial: trial}
	if err := ctx.Err(); err != nil {
		out.Err = err
		return out
	}
	c := trial.Baseline.Command
	if len(c.Args) == 0 {
		out.Err = errors.New("no own-test command")
		return out
	}
	site := trial.Candidate.Site
	path := filepath.Join(plan.FrozenRoot, filepath.FromSlash(trial.Candidate.Path))
	src, err := os.ReadFile(path)
	if err != nil {
		out.Err = err
		return out
	}
	if int(site.End) > len(src) || string(src[site.Start:site.End]) != site.Original {
		out.Err = fmt.Errorf("frozen %s no longer holds %q at its selected site", trial.Candidate.Path, site.Original)
		return out
	}
	mutated := site.Apply(src)
	timeout := timeoutOf(trial.Baseline.Elapsed, opt.TimeoutFactor)
	r, err := w.withMutant(c.Root, path, src, mutated, func() (result, error) {
		return w.runContext(ctx, c, timeout)
	})
	out.Elapsed = r.elapsed
	if out.Err = stageError(ctx, r, err); out.Err != nil {
		out.Stages = append(out.Stages, FreshStage{Name: "own", State: failedState(ctx), Error: out.Err.Error()})
		return out
	}
	own := judged(r)
	out.Stages = append(out.Stages, FreshStage{Name: "own", State: "complete", Outcome: own})
	if own != Survived || len(trial.Reach) == 0 {
		out.Outcome, out.Scope = own, opt.Scope
		if opt.Scope == ScopeOwn {
			out.Scope = OwnScope(path)
		}
		return out
	}
	if sels == nil {
		out.Err = fmt.Errorf("listed tests %s reach its line, and no listed tests are configured to run them", strings.Join(trial.Reach, " "))
		return out
	}
	ids := slices.Clone(trial.Reach)
	sel := sels.baselineContext(ctx, w, ids)
	switch {
	case sel.err != nil:
		out.Err = sel.err
		out.Stages = append(out.Stages, FreshStage{Name: "listed-baseline", State: failedState(ctx), Tests: ids, Error: sel.err.Error()})
		return out
	case sel.failed != nil:
		out.FailedSelection = sel.failed
		out.Stages = append(out.Stages, FreshStage{Name: "listed-baseline", State: "failed", Tests: ids,
			Error: fmt.Sprintf("the listed tests fail without any mutant (exit %d)", sel.failed.ExitCode)})
		return out
	}
	out.Stages = append(out.Stages, FreshStage{Name: "listed-baseline", State: "complete", Tests: ids})
	listed, err := w.withMutant(sels.listed.Root, path, src, mutated, func() (result, error) {
		return w.runContext(ctx, sels.listed.command(ids), sel.timeout)
	})
	out.Elapsed += listed.elapsed
	if out.Err = stageError(ctx, listed, err); out.Err != nil {
		out.Stages = append(out.Stages, FreshStage{Name: "listed", State: failedState(ctx), Tests: ids, Error: out.Err.Error()})
		return out
	}
	out.Outcome, out.Scope, out.Tests = judged(listed), ScopeListed, ids
	out.Stages = append(out.Stages, FreshStage{Name: "listed", State: "complete", Outcome: out.Outcome, Tests: ids})
	return out
}

// stageError is why a stage of a fresh trial has no outcome: the error its
// command gave, or its cancellation; nil when its tests ran to an end.
func stageError(ctx context.Context, r result, err error) error {
	switch {
	case err != nil:
		return err
	case r.cancelled || ctx.Err() != nil:
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
