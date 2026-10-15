package mutants

import (
	"context"
	"time"
)

// Workers opens the private copies for one run. Each pool owns one shared
// post-abort cleanup deadline. The caller joins its work before Close; Close
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
}

// Worker executes a command in its private copy of Command.Root. A nil change
// runs a clean baseline; otherwise the changed source is restored after return.
// A worker is used serially; separate workers may execute concurrently.
type Worker interface {
	Run(ctx context.Context, command Command, change *Mutation, timeout time.Duration) (CommandResult, WorkerFailure)
}

// WorkPurpose distinguishes ordinary judgments from trials of frozen inputs.
// Concrete adapters own physical directory names for those lifetimes.
type WorkPurpose int

const (
	MutationRun WorkPurpose = iota
	FrozenTrials
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

// WorkerFailed preserves an actual setup, copy, start, wait or cleanup error.
type WorkerFailed struct{ Cause error }

func (WorkerFailed) workerFailure()  {}
func (f WorkerFailed) Error() string { return f.Cause.Error() }
func (f WorkerFailed) Unwrap() error { return f.Cause }
