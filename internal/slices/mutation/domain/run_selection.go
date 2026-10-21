package mutants

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"
)

// A selection is the set of listed tests a mutant runs once its own tests
// survive it: those whose coverage reaches its line. The first time a
// mutant needs a selection, the selection runs once without any mutant, in
// that mutant's worker's copy, through the same command. Passing proves the
// tests hold alone, and its time sets the timeout of every mutant run of the
// selection, as a file's baseline does for its own tests. A selection that
// fails decides none of the mutants that would run it.

// selections holds the baseline of each selection a run needed, by its set
// of IDs, for every worker: each runs once, while the other workers that
// need it wait for it.
type SelectionBaselines struct {
	listed   *Listed
	factor   float64
	progress Progress
	mu       sync.Mutex
	byKey    map[string]*selection
}

// selection is the baseline of one selection: the timeout of its mutant
// runs, or how it failed. ran says whether it ran, which mu guards, so it
// runs once while the other workers that need it wait.
type selection struct {
	mu      sync.Mutex
	ran     bool
	timeout time.Duration
	failed  *FailedSelection
	err     error
}

func NewSelectionBaselines(listed *Listed, factor float64, progress Progress) *SelectionBaselines {
	return &SelectionBaselines{listed: listed, factor: factor, progress: progress, byKey: map[string]*selection{}}
}

// baseline is the baseline of the selection ids, run in w's copy under ctx,
// with no mutant, the first time any worker needs it: a baseline that ctx
// cancels is an error, never a selection that fails.
func (s *SelectionBaselines) baseline(ctx context.Context, w Worker, ids []string) *selection {
	key := SelectionKey(ids)
	s.mu.Lock()
	sel := s.byKey[key]
	if sel == nil {
		sel = &selection{}
		s.byKey[key] = sel
	}
	s.mu.Unlock()
	sel.mu.Lock()
	defer sel.mu.Unlock()
	if !sel.ran {
		sel.ran = true
		s.run(ctx, sel, w, ids)
	}
	return sel
}

// run runs sel, the selection ids, in w's copy under ctx, with no mutant,
// and records its timeout, or how it failed.
func (s *SelectionBaselines) run(ctx context.Context, sel *selection, w Worker, ids []string) {
	c := s.listed.Command(ids)
	report(s.progress, RunEvent{Kind: "baseline", Command: c})
	r, err := w.Run(ctx, c, nil, 0)
	if err != nil {
		sel.err = err
		return
	}
	if r.Cancelled || ctx.Err() != nil {
		sel.err = fmt.Errorf("cancelled: %w", context.Cause(ctx))
		return
	}
	if !r.Passed {
		sel.failed = &FailedSelection{IDs: slices.Clone(ids), Command: c.Shell, ExitCode: r.ExitCode, Output: r.Output}
		report(s.progress, RunEvent{Kind: "selection-failed", IDs: ids})
		return
	}
	sel.timeout = TimeoutOf(r.Elapsed, s.factor)
	report(s.progress, RunEvent{Kind: "selection-passed", IDs: ids, Elapsed: r.Elapsed, Timeout: sel.timeout})
}

// timeoutAllowance is added to every mutant's timeout. A mutant's run
// builds the mutated code, which its baseline may have found built, so on
// a slow machine a mutant timed from the baseline alone could time out
// before its tests ran, and a survivor would count as killed. It also
// keeps a fast suite from timing out on scheduling noise.
const timeoutAllowance = 5 * time.Second

// timeoutOf is how long a mutant run may take whose baseline took elapsed:
// factor times it, plus timeoutAllowance.
func TimeoutOf(elapsed time.Duration, factor float64) time.Duration {
	return time.Duration(float64(elapsed)*factor) + timeoutAllowance
}

// Baseline returns the actual once-per-ID-set baseline observation.
func (s *SelectionBaselines) Baseline(ctx context.Context, w Worker, ids []string) (time.Duration, *FailedSelection, error) {
	result := s.baseline(ctx, w, ids)
	return result.timeout, result.failed, result.err
}
