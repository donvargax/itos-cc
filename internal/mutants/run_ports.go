package mutants

import (
	"context"

	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// MutationSnapshotReader observes complete model values without write authority.
// Concrete codecs and physical names belong to the adapter.
type MutationSnapshotReader interface {
	Root(ctx context.Context) (string, snapport.Failure)
	Read(ctx context.Context, key string) (*Snapshot, snapport.Failure)
}

// MutationSnapshots adds publication to the read-only contract.
type MutationSnapshots interface {
	MutationSnapshotReader
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

// MutationCatalog inventories regular JSON records using store-relative names.
// RecordedFile observes only permissive file metadata, not a complete snapshot.
type MutationCatalog interface {
	Root(ctx context.Context) (string, snapport.Failure)
	List(ctx context.Context) ([]string, snapport.Failure)
	RecordedFile(ctx context.Context, name string) (string, snapport.Failure)
}

// SourcePresence observes Lstat presence. Only not-exist is absent; directories,
// dangling links and ordinary observation errors do not prove a source gone.
type SourcePresence interface {
	Present(ctx context.Context, path string) (bool, InputFailure)
	// RenameTarget requires successful Lstat evidence, unlike conservative Present.
	RenameTarget(ctx context.Context, path string) (bool, InputFailure)
}
