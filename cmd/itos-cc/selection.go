package main

import (
	"errors"
	"fmt"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/selection/port"
)

// classifySelection is the selection domain's classifier, which cmd hands to
// internal/cli: a failure of the port's sealed set is its problem, and any
// other error is not selection's to claim.
func classifySelection(err error) ([]*cli.Problem, bool) {
	var failure port.Failure
	if !errors.As(err, &failure) {
		return nil, false
	}
	return []*cli.Problem{selectionProblem(failure)}, true
}

// selectionProblem is failure's problem: --changed with no git is the
// environment's, exit 82, and an argument that names nothing a usage error,
// exit 80, as file_selection.feature reads them; a walk or a read that
// failed is the internal 100, its own message whole; a cancellation an
// interrupt caused, cli.ErrInterrupted its cause, is 130 (decision 38), and
// any other the internal 100. None has a default; a kind with no case here is the 100 of a bug.
func selectionProblem(failure port.Failure) *cli.Problem {
	p := cli.Internal(failure)
	switch e := failure.(type) {
	case *port.NoGit:
		p = cli.NewProblem(cli.CodeEnvironment, "changed.no-git", "--changed needs a git repository: "+e.Reason,
			"Run it inside a git repository, or name the paths instead.")
	case *port.Unmatched:
		p = cli.NewProblem(cli.CodeUsage, "paths.unmatched", fmt.Sprintf("%q is no file, and no file's path under the working directory contains it", e.Fragment),
			"Name a file, a directory, or a fragment of a source's path.").With("argument", e.Fragment)
	case *port.Unwalkable:
		p = cli.Internal(e)
	case *port.Unreadable:
		p = cli.Internal(e)
	case *port.Cancelled:
		if errors.Is(e.Cause, cli.ErrInterrupted) {
			p = cli.Interrupted()
		}
	}
	return p
}
