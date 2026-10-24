package domain

import (
	"context"
	"sync"
	"time"
)

type job struct {
	state *Execution
	site  int
}

// execute runs each command's baseline, then every pending mutant across
// the workers, each selection of listed tests' baseline the first time a
// mutant needs it. With ff, it stops at the first actionable final
// judgment, which it returns: every command runs under one context the
// stop cancels, and the workers share one cleanup deadline from the stop.
// The worker copies are removed only once every worker returned, its
// owned commands joined.
func execute(ctx context.Context, states []*Execution, opt Options, ff *FailFast) (stop *Stop, err error) {
	purpose := MutationRun
	if ff != nil {
		purpose = FailFastJudgments
	}
	pool, err := opt.WorkerFactory.Open(ctx, purpose, max(1, opt.Workers))
	if err != nil {
		return nil, err
	}
	// Close receives the original caller, not the fail-fast execution child.
	// Preserve a primary run failure/cancellation; a separate lifetime failure
	// prevents publication when execution otherwise completed successfully.
	defer func(ctx context.Context) {
		if failure := pool.Close(ctx); failure != nil && err == nil {
			err = failure
			if cause := context.Cause(ctx); cause != nil {
				err = cause
			}
		}
	}(ctx)
	// A parent's abort, such as an interrupt, starts the cleanup deadline
	// every worker shares, as a fail-fast stop does.
	defer context.AfterFunc(ctx, pool.Abort)()
	var st *stopper
	if ff != nil {
		ctx, st = newStopper(ctx, opt.Progress)
		defer st.cancel(nil)
		st.abort = pool.Abort
	}
	workers := make([]Worker, max(1, opt.Workers))
	for i := range workers {
		workers[i] = pool.Worker(i)
	}

	// One baseline per distinct command, run inside a worker's copy: passing
	// proves the copy is a working tree, and what it used sets the limits
	// of its mutants' runs.
	factors := Factors{Timeout: opt.TimeoutFactor, Output: opt.OutputFactor, Memory: opt.MemoryFactor}
	limits := map[string]Limits{}
	failed := map[string]*Output{}
	unmeasured := false
	// copies is what a worker's copy holds and leaves out for each command
	// whose baseline failed there but passes in the project's own folder.
	copies := map[string]ProjectRun{}
	var jobs []job
	for _, s := range states {
		for i, o := range s.outcomes {
			if o != "" {
				continue
			}
			if s.command.RunsNothing() {
				// No test reaches the file: none runs, so its mutant is
				// uncovered, unless listed tests reach its line, which then
				// judge it alone.
				if s.reach == nil || len(s.reach[i]) == 0 {
					s.outcomes[i] = Uncovered
					continue
				}
				if !st.stopped() {
					jobs = append(jobs, job{s, i})
				}
				continue
			}
			key := s.command.Key()
			if _, done := limits[key]; !done && failed[key] == nil && !st.stopped() {
				report(opt.Progress, RunEvent{Kind: "baseline", Command: s.command})
				r, err := workers[0].Run(ctx, s.command, nil, Limits{})
				if cancelledBy(ctx, err) {
					// The run stopped during the baseline: nothing of
					// the file is admitted.
					continue
				}
				if err != nil {
					return nil, err
				}
				if !r.Passed {
					kept := r.Kept()
					failed[key] = &kept
					if copies[key], err = copyOnly(ctx, pool, s.command, opt.Progress); err != nil {
						return nil, err
					}
				} else {
					limits[key] = factors.LimitsOf(r.Usage())
					if !r.MemoryMeasured && !unmeasured {
						// No memory cap holds on this system: said once.
						unmeasured = true
						report(opt.Progress, RunEvent{Kind: MemoryUnmeasured})
					}
				}
			}
			if out, bad := failed[key]; bad {
				s.result.BaselineFailed, s.result.BaselineOutput = true, *out
				s.result.BaselineCopy, s.result.BaselineLeftOut = copies[key].Copied, copies[key].LeftOut
				if st != nil {
					st.trigger(&Stop{Rule: "mutation.baseline-failed", File: s.rel})
				}
				continue
			}
			if _, ran := limits[key]; !ran {
				// The run stopped before this baseline: nothing of the
				// file is admitted.
				continue
			}
			jobs = append(jobs, job{s, i})
		}
	}
	// A baseline that ran is every file's sharing its command, whether or
	// not one of its own mutants needed it.
	for _, s := range states {
		if s.command.RunsNothing() {
			// No test reaches the file: no baseline runs for it in any
			// run, and run says BaselineNotRun.
			continue
		}
		if _, ran := limits[s.command.Key()]; ran {
			s.result.Baseline = "passed"
		} else if s.result.BaselineFailed {
			s.result.Baseline = "failed"
		}
	}

	// Each selection of listed tests runs without a mutant the first time
	// a mutant needs it, in that mutant's worker.
	var sels *SelectionBaselines
	if opt.Listed != nil {
		sels = NewSelectionBaselines(opt.Listed, factors, opt.Progress)
	}

	queue := make(chan job)
	var mu sync.Mutex
	var firstErr error
	done := 0
	var wg sync.WaitGroup
	for _, w := range workers {
		wg.Add(1)
		go func(ctx context.Context, w Worker) {
			defer wg.Done()
			for j := range queue {
				r, err := runMutant(ctx, w, j, limits[j.state.command.Key()], sels)
				mu.Lock()
				if err != nil && firstErr == nil {
					firstErr = err
				}
				site := j.state.sites[j.site]
				if r.cleanup != nil {
					j.state.result.CleanupFailures = append(j.state.result.CleanupFailures, *r.cleanup)
				}
				if r.cancelled {
					// The stop came first, or the judgment's owned cleanup
					// failed: no outcome, and a judgment that never started
					// stays unattempted.
					if r.started {
						j.state.leave(j.site, StateCancelled)
						report(opt.Progress, RunEvent{Kind: "cancelled", File: j.state.rel, Site: site, Failure: context.Cause(ctx), Elapsed: r.elapsed})
					}
					mu.Unlock()
					continue
				}
				switch {
				case r.failed != nil:
					// Undecided: its outcome stays pending, it counts as no
					// mutant run, and its file is not written, but in a
					// fail-fast run, which keeps what else it judged.
					j.state.result.AddFailed(r.failed)
					if st != nil {
						j.state.leave(j.site, StateBlocked)
					}
				case r.ids != nil:
					j.state.outcomes[j.site], j.state.limits[j.site] = r.outcome, r.limit
					j.state.scopes[j.site], j.state.ran[j.site] = ScopeListed, r.ids
				default:
					j.state.outcomes[j.site], j.state.limits[j.site] = r.outcome, r.limit
				}
				if r.failed == nil {
					j.state.result.Ran++
				}
				done++
				kind := "judged"
				if r.failed != nil {
					kind = "selection-blocked"
				}
				report(opt.Progress, RunEvent{Kind: kind, Done: done, Total: len(jobs), File: j.state.rel, Site: site, Outcome: r.outcome, Limit: r.limit, Limits: r.limits,
					IDs: r.ids, Elapsed: r.elapsed, Listed: r.listed})
				if st != nil && err == nil {
					if r.failed != nil {
						st.trigger(&Stop{Rule: "tests.selection-failed", File: j.state.rel, Selection: r.failed})
					} else if stop := j.state.actionable(j.site); stop != nil {
						st.trigger(stop)
					}
				}
				mu.Unlock()
			}
		}(ctx, w)
	}
	// After a stop no job is admitted; the Done of a run without fail-fast
	// never closes.
dispatch:
	for _, j := range jobs {
		select {
		case queue <- j:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(queue)
	wg.Wait()
	return st.result(), firstErr
}

// mutantRun is how a mutant's runs went.
type mutantRun struct {
	outcome string
	// limit names the cap an OverLimit outcome went over, of limits, those
	// of the run that decided it.
	limit   string
	limits  Limits
	elapsed time.Duration // its runs' time in all
	listed  time.Duration // the listed tests' time
	// ids is the listed tests it ran once its own tests survived it, or
	// nil when it ran none.
	ids []string
	// failed, when set, is the selection ids, which fails without any
	// mutant: the mutant is not decided.
	failed *FailedSelection
	// cancelled is set when a fail-fast stop cut the mutant's judgment
	// short, or came before it: it has no outcome. started says whether
	// any of its commands had been admitted.
	cancelled, started bool
	// cleanup is the failure of the judgment's owned cleanup, which
	// leaves it cancelled.
	cleanup *WorkerCleanupFailed
}

// runMutant runs the file's own tests on the mutant, then, when it survives
// them, the listed tests that reach its line, after their selection's
// baseline the first time any mutant needs it. Every command runs under
// ctx: once a fail-fast stop cancels it, none starts, and a judgment it cut
// short is cancelled, never killed or timed out.
func runMutant(ctx context.Context, w Worker, j job, limits Limits, sels *SelectionBaselines) (mutantRun, error) {
	if ctx.Err() != nil {
		return mutantRun{cancelled: true}, nil
	}
	s := j.state
	site := s.sites[j.site]
	// Own tests that run nothing survive every mutant without running.
	run, err := mutantRun{outcome: Survived, started: true}, error(nil)
	if !s.command.RunsNothing() {
		var r CommandResult
		r, err = w.Run(ctx, s.command, &Mutation{Path: s.file.Path, Source: s.file.Src, Site: site}, limits)
		run = mutantRun{outcome: Judged(r), limit: r.OverLimit, limits: limits, elapsed: r.Elapsed, started: true}
		if c, ok := cleanupFailed(err); ok {
			return mutantRun{cancelled: true, started: true, elapsed: r.Elapsed, cleanup: &c}, nil
		}
		if r.Cancelled || cancelledBy(ctx, err) {
			return mutantRun{cancelled: true, started: true, elapsed: r.Elapsed}, nil
		}
	}
	if err != nil || run.outcome != Survived || sels == nil || s.reach == nil || len(s.reach[j.site]) == 0 {
		return run, err
	}
	run.ids = s.reach[j.site]
	cut := mutantRun{cancelled: true, started: true, elapsed: run.elapsed}
	if ctx.Err() != nil {
		return cut, nil
	}
	sel := sels.baseline(ctx, w, run.ids)
	if cancelledBy(ctx, sel.err) {
		return cut, nil
	}
	if sel.err != nil || sel.failed != nil {
		run.failed = sel.failed
		return run, sel.err
	}
	listed, err := w.Run(ctx, sels.listed.Command(run.ids), &Mutation{Path: s.file.Path, Source: s.file.Src, Site: site}, sel.limits)
	if c, ok := cleanupFailed(err); ok {
		cut.elapsed += listed.Elapsed
		cut.cleanup = &c
		return cut, nil
	}
	if listed.Cancelled || cancelledBy(ctx, err) {
		cut.elapsed += listed.Elapsed
		return cut, nil
	}
	run.outcome, run.limit, run.limits, run.listed = Judged(listed), listed.OverLimit, sel.limits, listed.Elapsed
	run.elapsed += listed.Elapsed
	return run, err
}

// judged is the outcome of a test run on a mutant: OverLimit when it went
// over a cap of its limits, whose name r.OverLimit gives.
func Judged(r CommandResult) string {
	switch {
	case r.OverLimit != "":
		return OverLimit
	case r.TimedOut:
		return Timeout
	case r.Passed:
		return Survived
	}
	return Killed
}

// command runs the listed tests ids.
func (l *Listed) Command(ids []string) Command {
	return Command{Root: l.Root, Dir: l.Root, Shell: l.Select(ids)}
}

// copyOnly runs c, whose baseline failed in a worker's copy, once in the
// project's own folder, as coverage runs there: that run, naming the tree
// the copy holds and what it leaves out, when it passes there, and none
// when it fails there too. A run the caller stopped decides nothing, so
// the failure stays the tests'.
func copyOnly(ctx context.Context, pool WorkerPool, c Command, progress Progress) (ProjectRun, WorkerFailure) {
	report(progress, RunEvent{Kind: "baseline-project", Command: c})
	there, err := pool.InProject(ctx, c)
	if cancelledBy(ctx, err) {
		return ProjectRun{}, nil
	}
	if err != nil {
		return ProjectRun{}, err
	}
	if !there.Result.Passed || there.Result.Cancelled {
		return ProjectRun{}, nil
	}
	return there, nil
}
