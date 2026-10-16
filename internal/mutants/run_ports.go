package mutants

import (
	"context"

	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// MutationSnapshots stores complete model values. Concrete codecs and physical
// snapshot names belong to the adapter; admission remains domain policy.
type MutationSnapshots interface {
	Root(ctx context.Context) (string, snapport.Failure)
	Read(ctx context.Context, key string) (*Snapshot, snapport.Failure)
	Write(ctx context.Context, key string, snapshot Snapshot) snapport.Failure
	Remove(ctx context.Context, key string) snapport.Failure
}

// SourceEdits observes rename-follow absence and updates only a current source's
// summary. Missing uses follow's Stat semantics, distinct from orphan presence.
type SourceEdits interface {
	Missing(ctx context.Context, path string) (bool, InputFailure)
	Annotate(ctx context.Context, path, comment, block string) InputFailure
}

// TestLister executes the configured listing command. Only stdout names tests;
// the concrete adapter streams stderr to its composed output as it arrives.
type TestLister interface {
	List(ctx context.Context, command Command) (TestListing, InputFailure)
}

// TestListing keeps ordinary unsuccessful commands separate from observation
// failure. Reason is the original start/wait error; ExitCode is -1 without one.
type TestListing struct {
	Stdout   string
	ExitCode int
	Reason   string
}

const (
	OrphanGone     = "gone"
	OrphanExcluded = "excluded"
)

// OrphanReason applies the shared rule to actual presence/exclusion/scope facts.
// Observers use Lstat presence, including dangling links and conservative errors.
func OrphanReason(present, excluded, covered bool) string {
	if !covered {
		return ""
	}
	if !present {
		return OrphanGone
	}
	if excluded {
		return OrphanExcluded
	}
	return ""
}
