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
	return []*cli.Problem{snapshotProblem(failure)}, true
}

// snapshotProblem is failure's problem: a snapshot that could not be read
// or written is the 70 it has always been, the library's message whole,
// and an interrupt is 130 (decision 31). A command that can go on without
// a snapshot, as scrap without its last one, says so and never brings the
// failure here. None has a default; a kind with no case here is the 70 of
// a bug.
func snapshotProblem(failure snapport.Failure) *cli.Problem {
	p := cli.Internal(failure)
	switch e := failure.(type) {
	case *snapport.Unreadable:
		p = cli.Internal(e)
	case *snapport.Unwritable:
		p = cli.Internal(e)
	case *snapport.Cancelled:
		p = cli.Interrupted()
	}
	return p
}
