package worktrees

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/donvargax/itos-cc/internal/processes"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// Workers is the real mutation worker factory. Prepare adjusts the actual
// command at the boundary (frozen dependency/environment setup), never a domain
// callback. Owner shares preparation's abort budget with subsequent trials.
type Workers struct {
	Platform string
	Prepare  func(*exec.Cmd)
	Owner    *processes.OwnedExecutor
	// Hold marks the owned process groups of the pool's commands being
	// killed and joined, as OwnedExecutor's does; with Owner, Owner's.
	Hold func() (release func())
	// Dir is the working directory, whose project root bounds the
	// workspace a worker's copy of a TypeScript package may hold; "", each
	// package's own project root, as a counted run's frozen export has.
	Dir string
	// Settings, read when a pool opens, gives mutation.copy, the paths a
	// copy brings back of what it leaves out; nil brings back none.
	Settings mutants.SettingsReader
	// TempDir is the directory a pool's worker directories are made in,
	// swept first of those killed runs left there; "", the system's
	// temporary directory. Tests give their own, so that no other run's
	// directories there decide their outcome (T-219).
	TempDir string
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
	rules, failure := f.rules(ctx, purpose)
	if failure != nil {
		return nil, failure
	}
	scratch, err := newWorkerScratch(f.TempDir, prefix, processes.PlatformAtBoundary(f.Platform))
	if err != nil {
		return nil, mutants.WorkerFailed{Cause: err}
	}
	budget := processes.NewCleanupBudget(purpose == mutants.MutationRun)
	hold := f.Hold
	if f.Owner != nil {
		budget, hold = f.Owner.Budget(), f.Owner.Hold
	}
	p := &pool{scratch: scratch, budget: budget, closing: make(chan struct{}), done: make(chan struct{})}
	for i := 0; i < max(1, count); i++ {
		p.workers = append(p.workers, &boundWorker{pool: p, worker: &worker{dir: filepath.Join(scratch.dir, fmt.Sprintf("w%d", i)), copies: map[string]*workerCopy{}, project: f.Dir, rules: rules, runner: processes.Runner{Budget: budget, Platform: processes.PlatformAtBoundary(f.Platform), Hold: hold}, prepare: f.Prepare}})
	}
	return p, nil
}

// rules are what the copies of a pool opened for purpose leave out as git
// ignores it and bring back as mutation.copy names: none for a counted
// run's, whose frozen export holds the commit's tracked files alone.
func (f Workers) rules(ctx context.Context, purpose mutants.WorkPurpose) (*copyRules, mutants.WorkerFailure) {
	if purpose == mutants.FrozenTrials {
		return nil, nil
	}
	rules := &copyRules{trees: map[string]ignoredPaths{}}
	if f.Settings != nil {
		config, failure := f.Settings.Read(ctx)
		if failure != nil {
			return nil, failed(ctx, failure)
		}
		rules.root, rules.globs = config.Selection.Root, config.Copy
	}
	return rules, nil
}

type pool struct {
	scratch       *workerScratch
	budget        *processes.CleanupBudget
	workers       []*boundWorker
	mu            sync.Mutex
	active        sync.WaitGroup
	closed        bool
	lifetime      inputLifetime
	closeOnce     sync.Once
	closing, done chan struct{}
}

func (p *pool) Worker(index int) mutants.Worker { return p.workers[index] }
func (p *pool) Abort()                          { p.budget.Begin() }
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

// enter admits one command to the pool, which Close then waits for: a
// cancelled caller or a closed pool admits none. Its done ends it.
func (p *pool) enter(ctx context.Context) (done func(), failure mutants.WorkerFailure) {
	if ctx.Err() != nil {
		return nil, mutants.WorkerCancelled{Cause: context.Cause(ctx)}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, mutants.WorkerFailed{Cause: errors.New("worker pool is closed")}
	}
	p.active.Add(1)
	return p.active.Done, nil
}

// InProject runs c in c.Dir itself, through the first worker's runner and
// preparation, with no copy, Ready or timeout, and the system's temporary
// folder, as the coverage run has, and names what git ignores that a copy
// of its tree leaves out.
func (p *pool) InProject(ctx context.Context, c mutants.Command) (mutants.ProjectRun, mutants.WorkerFailure) {
	w := p.workers[0].worker
	tree, _ := w.tree(c)
	run := mutants.ProjectRun{Copied: tree, LeftOut: w.rules.leftOut(ctx, tree)}
	done, failure := p.enter(ctx)
	if failure != nil {
		return run, failure
	}
	defer done()
	r, err := w.runIn(ctx, c, c.Root, "", mutants.Limits{})
	run.Result = r
	return run, failed(ctx, err)
}

func (w *boundWorker) Run(ctx context.Context, c mutants.Command, change *mutants.Mutation, limits mutants.Limits) (mutants.CommandResult, mutants.WorkerFailure) {
	done, failure := w.pool.enter(ctx)
	if failure != nil {
		return result{}, failure
	}
	defer done()
	var r result
	// The copy is put back as made before the mutant is written, never
	// between, and before the command's timeout starts.
	err := w.worker.fresh(ctx, c)
	switch {
	case err != nil:
	case change == nil:
		r, err = w.worker.run(ctx, c, limits)
	default:
		r, err = w.worker.withMutant(ctx, c, change.Path, change.Source, change.Site.Apply(change.Source), func(ctx context.Context) (result, error) { return w.worker.run(ctx, c, limits) })
	}
	return r, failed(ctx, err)
}

// failed is err, a command's under ctx, as the port's failure: nil for
// none.
func failed(ctx context.Context, err error) mutants.WorkerFailure {
	if err == nil {
		return nil
	}
	var cleanup *processes.CleanupFailed
	if errors.As(err, &cleanup) {
		return mutants.WorkerCleanupFailed{ProcessGroup: cleanup.ProcessGroup, Command: cleanup.Command, Cause: err}
	}
	if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		return mutants.WorkerCancelled{Cause: context.Cause(ctx)}
	}
	return mutants.WorkerFailed{Cause: err}
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
