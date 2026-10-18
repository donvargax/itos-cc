package main

import (
	"errors"

	"github.com/donvargax/itos-cc/internal/cli"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// classifySnapshot is the snapshot domain's classifier, which cmd hands to
// internal/cli: a failure of the port's sealed set is its problem, and any
// other error is not the snapshot domain's to claim. Several commands share
// the domain, so cmd holds it, as it holds selection's.
func classifySnapshot(err error) ([]*cli.Problem, bool) {
	var failure snapport.Failure
	if !errors.As(err, &failure) {
		return nil, false
	}
	return []*cli.Problem{snapshotProblem(err, failure)}, true
}

// snapshotProblem is the problem of err, which holds failure: a snapshot
// that could not be read, written or removed, or a folder of them listed,
// is the internal 100, its message err's text whole, the
// library's with whatever a caller wrapped it in, such as the snapshot's
// name mutation gives, as when no classifier claimed it; and an interrupt
// is 130 (decision 31). A command that can go on without a snapshot, as
// scrap without its last one, says so and never brings the failure here.
// None has a default; a kind with no case here is the 100 of a bug.
func snapshotProblem(err error, failure snapport.Failure) *cli.Problem {
	p := cli.Internal(err)
	switch failure.(type) {
	case *snapport.Unreadable:
		p = cli.Internal(err)
	case *snapport.Unwritable:
		p = cli.Internal(err)
	case *snapport.Unremovable:
		p = cli.Internal(err)
	case *snapport.Unlistable:
		p = cli.Internal(err)
	case *snapport.Cancelled:
		p = cli.Interrupted()
	}
	return p
}
