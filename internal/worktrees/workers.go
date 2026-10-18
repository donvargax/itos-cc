package worktrees

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/donvargax/itos-cc/internal/mutants"
)

// Workers is the real mutation worker factory. Prepare adjusts the actual
// command at the boundary (frozen dependency/environment setup), never a domain
// callback. Owner shares preparation's abort budget with subsequent trials.
type Workers struct {
	Platform string
	Prepare  func(*exec.Cmd)
	Owner    *OwnedExecutor
	// Hold marks the owned process groups of the pool's commands being
	// killed and joined, as OwnedExecutor's does; with Owner, Owner's.
	Hold func() (release func())
}

var _ mutants.Workers = Workers{}

func (f Workers) Open(ctx context.Context, purpose mutants.WorkPurpose, count int) (mutants.WorkerPool, mutants.WorkerFailure) {
	if err := ctx.Err(); err != nil {
		return nil, mutants.WorkerCancelled{Cause: context.Cause(ctx)}
	}
	prefix := "itos-cc-mutate-"
	switch purpose {
	case mutants.MutationRun, mutants.FailFastJudgments:
	case mutants.FrozenTrials:
		prefix = "itos-cc-fresh-"
	default:
		return nil, mutants.WorkerFailed{Cause: errors.New("unknown worker purpose")}
	}
	scratch, err := newWorkerScratch(prefix, platformAtBoundary(f.Platform))
	if err != nil {
		return nil, mutants.WorkerFailed{Cause: err}
	}
	budget := &cleanupBudget{localUntilAbort: purpose == mutants.MutationRun}
	hold := f.Hold
	if f.Owner != nil {
		budget, hold = &f.Owner.cleanup, f.Owner.Hold
	}
	p := &pool{scratch: scratch, budget: budget, closing: make(chan struct{}), done: make(chan struct{})}
	for i := 0; i < max(1, count); i++ {
		p.workers = append(p.workers, &boundWorker{pool: p, worker: &worker{dir: filepath.Join(scratch.dir, fmt.Sprintf("w%d", i)), copies: map[string]string{}, runner: commandRunner{cleanupBudget: budget, platform: platformAtBoundary(f.Platform), hold: hold}, prepare: f.Prepare}})
	}
	return p, nil
}

type pool struct {
	scratch       *workerScratch
	budget        *cleanupBudget
	workers       []*boundWorker
	mu            sync.Mutex
	active        sync.WaitGroup
	closed        bool
	lifetime      inputLifetime
	closeOnce     sync.Once
	closing, done chan struct{}
}

func (p *pool) Worker(index int) mutants.Worker { return p.workers[index] }
func (p *pool) Abort()                          { p.budget.begin() }
func (p *pool) Close(context.Context) mutants.WorkerFailure {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		close(p.closing)
		p.mu.Unlock()
		p.lifetime.finish(p.active.Wait, func() {
			// Historical scratch removal is best effort; preserve that surfaced-error
			// policy instead of turning a cleanup warning into a new fatal CLI result.
			p.scratch.remove()
		})
		close(p.done)
	})
	<-p.done
	return nil
}

type boundWorker struct {
	pool   *pool
	worker *worker
}

func (w *boundWorker) Run(ctx context.Context, c mutants.Command, change *mutants.Mutation, timeout time.Duration) (mutants.CommandResult, mutants.WorkerFailure) {
	if ctx.Err() != nil {
		return result{}, mutants.WorkerCancelled{Cause: context.Cause(ctx)}
	}
	w.pool.mu.Lock()
	if w.pool.closed {
		w.pool.mu.Unlock()
		return result{}, mutants.WorkerFailed{Cause: errors.New("worker pool is closed")}
	}
	w.pool.active.Add(1)
	w.pool.mu.Unlock()
	defer w.pool.active.Done()
	var r result
	var err error
	if change == nil {
		r, err = w.worker.run(ctx, c, timeout)
	} else {
		r, err = w.worker.withMutant(ctx, c.Root, change.Path, change.Source, change.Site.Apply(change.Source), func(ctx context.Context) (result, error) { return w.worker.run(ctx, c, timeout) })
	}
	if err != nil {
		var cleanup *CleanupFailed
		if errors.As(err, &cleanup) {
			return r, mutants.WorkerCleanupFailed{ProcessGroup: cleanup.ProcessGroup, Command: cleanup.Command, Cause: err}
		}
		if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
			return r, mutants.WorkerCancelled{Cause: context.Cause(ctx)}
		}
		return r, mutants.WorkerFailed{Cause: err}
	}
	return r, nil
}

// inputLifetime keeps the adapter's actual join/release boundary explicit.
// Hooks are instance-local adapter guard observations, never domain callbacks.
type inputLifetime struct {
	join    func(wait func())
	release func(remove func())
}

func (l inputLifetime) finish(wait, remove func()) {
	if l.join != nil {
		l.join(wait)
	} else {
		wait()
	}
	if l.release != nil {
		l.release(remove)
	} else {
		remove()
	}
}
