package mutate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FreshTrial is one selected mutant to judge with the own-test command whose
// clean baseline preparation ran.
type FreshTrial struct {
	Candidate FreshCandidate
	Baseline  FreshBaseline
}

// FreshTrialResult is how one fresh trial went. Outcome is Killed, Survived
// or Timeout when the mutant's tests ran to an end; it is empty, with Err
// set, when they could not, a parent cancellation included: such a mutant
// has no outcome.
type FreshTrialResult struct {
	Trial   FreshTrial
	Outcome string
	Elapsed time.Duration
	Err     error
}

// FreshTrialOptions control RunFreshTrials.
type FreshTrialOptions struct {
	Workers       int
	TimeoutFactor float64
	// Prepare adjusts each command before it runs, such as its environment.
	Prepare func(*exec.Cmd)
	Log     io.Writer
}

// RunFreshTrials applies each selected mutant to a private worker copy of
// the frozen root and runs its own tests once, under the same owned process
// supervision as complete runs, workers at a time. A mutant times out after
// its baseline's time times the factor, plus the usual allowance. Results
// are in the order of trials. Nothing is written outside the worker copies,
// which are removed before it returns.
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
	workers := make([]*worker, min(max(1, opt.Workers), len(trials)))
	for i := range workers {
		workers[i] = &worker{dir: filepath.Join(base, fmt.Sprintf("w%d", i)), copies: map[string]string{},
			runner: commandRunner{cleanupBudget: budget}}
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
				r := runFreshTrial(ctx, w, plan, trials[i], opt)
				mu.Lock()
				results[i] = r
				done++
				c := trials[i].Candidate
				said := r.Outcome
				if r.Err != nil {
					said = "not judged: " + r.Err.Error()
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

func runFreshTrial(ctx context.Context, w *worker, plan *FreshPlan, trial FreshTrial, opt FreshTrialOptions) FreshTrialResult {
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
	timeout := timeoutOf(trial.Baseline.Elapsed, opt.TimeoutFactor)
	r, err := w.withMutant(c.Root, path, src, site.Apply(src), func() (result, error) {
		root, err := w.copyOf(c.Root)
		if err != nil {
			return result{}, err
		}
		trialCtx, cancel := context.WithTimeoutCause(ctx, timeout, errMutationTimeout)
		defer cancel()
		cmd := exec.CommandContext(trialCtx, c.Args[0], c.Args[1:]...)
		cmd.Dir = filepath.Join(root, strings.TrimPrefix(c.Dir, c.Root))
		cmd.Env = os.Environ()
		if c.PathEnv != "" {
			var paths []string
			for _, p := range c.PathDirs {
				paths = append(paths, filepath.Join(root, p))
			}
			if old := os.Getenv(c.PathEnv); old != "" {
				paths = append(paths, old)
			}
			cmd.Env = append(cmd.Env, c.PathEnv+"="+strings.Join(paths, string(os.PathListSeparator)))
		}
		if opt.Prepare != nil {
			opt.Prepare(cmd)
		}
		var output bytes.Buffer
		cmd.Stdout, cmd.Stderr = &output, &output
		return w.runner.run(trialCtx, cmd, &output)
	})
	out.Elapsed = r.elapsed
	switch {
	case err != nil:
		out.Err = err
	case r.cancelled || ctx.Err() != nil:
		out.Err = fmt.Errorf("cancelled: %w", context.Cause(ctx))
	default:
		out.Outcome = judged(r)
	}
	return out
}
