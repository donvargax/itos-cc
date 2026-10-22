package domain

import (
	"context"
	"time"
)

// Workers opens the private copies for one run. Fail-fast and frozen pools
// share a post-abort cleanup deadline; ordinary runs keep local command budgets. The caller joins its work before Close; Close
// also waits for already-entered commands before releasing their inputs.
type Workers interface {
	Open(ctx context.Context, purpose WorkPurpose, count int) (WorkerPool, WorkerFailure)
}

// WorkerPool owns isolated workers and their lifetime, not the run context.
type WorkerPool interface {
	Worker(index int) Worker
	// Abort starts the shared cleanup deadline when a fail-fast stop is published.
	Abort()
	// Close joins entered work and removes the copies, even on a done context.
	// Cleanup must not abandon owned commands because their caller cancelled.
	Close(ctx context.Context) WorkerFailure
	// InProject runs command once, unmutated and with no timeout, in the
	// project's own folder, command.Dir, never a copy, under the pool's
	// process ownership: whether tests whose baseline failed in a worker's
	// copy pass in the project. Ready never runs there. It fails as Run
	// does, and names the tree a worker's copy of command.Root holds.
	InProject(ctx context.Context, command Command) (ProjectRun, WorkerFailure)
}

// ProjectRun is a command's run in the project's own folder (InProject):
// its result, and Copied, the tree a worker's copy holds for the command,
// which may hold more than its Root, such as the workspace it belongs to.
type ProjectRun struct {
	Result CommandResult
	Copied string
}

// Worker executes a command in its private copy of Command.Root, or of the
// tree around it the command needs, such as the workspace it belongs to,
// running it in Root's place there. Each run
// starts from the copy as it was made: what an earlier run wrote, changed or
// removed there is put back first, outside the run's timeout and Elapsed,
// while the build output, caches and linked dependencies the copy leaves to
// the tools stay as the runs leave them. A nil change runs a clean baseline;
// otherwise the changed source is restored after return. A worker is used
// serially; separate workers may execute concurrently.
type Worker interface {
	Run(ctx context.Context, command Command, change *Mutation, timeout time.Duration) (CommandResult, WorkerFailure)
}

// WorkPurpose distinguishes ordinary judgments from trials of frozen inputs.
// Concrete adapters own physical directory names for those lifetimes.
type WorkPurpose int

const (
	MutationRun WorkPurpose = iota
	FrozenTrials
	// FailFastJudgments shares cleanup on any parent abort, even before a stop.
	FailFastJudgments
)

// Mutation is the source being judged, in the command's original tree.
// Source restores the private file after the command, never the live tree.
type Mutation struct {
	Path   string
	Source []byte
	Site   Site
}

// CommandResult preserves the actual test command's outcome and combined
// stdout/stderr. ExitCode is -1 when it gave none; a parent cancellation is
// distinct from a mutant's own deadline and from a completed nonzero exit.
type CommandResult struct {
	Passed, TimedOut, Cancelled bool
	Elapsed                     time.Duration
	Output                      string
	ExitCode                    int
}

// WorkerFailure is a command/copy/lifetime failure, never a test judgment.
//
//sumtype:decl
type WorkerFailure interface {
	error
	workerFailure()
}

// WorkerCancelled forwards the caller's cause, including custom stop causes.
type WorkerCancelled struct{ Cause error }

func (WorkerCancelled) workerFailure()  {}
func (f WorkerCancelled) Error() string { return f.Cause.Error() }
func (f WorkerCancelled) Unwrap() error { return f.Cause }

// WorkerCleanupFailed is a command whose owned process group's cleanup did
// not finish by its deadline (ADR-0021): ProcessGroup and Command name the
// group and the command. Its judgment has no outcome, and the run reports
// the failure beside its other problems, never as a kill.
type WorkerCleanupFailed struct {
	ProcessGroup int
	Command      string
	Cause        error
}

func (WorkerCleanupFailed) workerFailure()  {}
func (f WorkerCleanupFailed) Error() string { return f.Cause.Error() }
func (f WorkerCleanupFailed) Unwrap() error { return f.Cause }

// WorkerFailed preserves an actual setup, copy, start, wait or cleanup error.
type WorkerFailed struct{ Cause error }

func (WorkerFailed) workerFailure()  {}
func (f WorkerFailed) Error() string { return f.Cause.Error() }
func (f WorkerFailed) Unwrap() error { return f.Cause }
