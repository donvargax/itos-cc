package count

import (
	"errors"
	"fmt"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/counting/port"
)

// Classify is the counting domain's classifier, which cmd hands to
// internal/cli: a failure of the port's sealed set is its problem, and any
// other error is not counting's to claim. The slice holds it, as it knows
// both the domain and the UI, so the UI imports no domain and removing the
// sample touches no file of internal/cli.
func Classify(err error) ([]*cli.Problem, bool) {
	var failure port.ReadFailure
	if !errors.As(err, &failure) {
		return nil, false
	}
	return []*cli.Problem{readProblem(failure)}, true
}

// readProblem is failure's problem. Each kind below is one the count
// command's scenarios read: a missing file and an unreadable one, a
// directory among the latter, both the environment's and exit 3, and a read
// an interrupt cancelled, exit 130 (decision 31). None has a default; a kind
// with no case here is the 70 of a bug.
func readProblem(failure port.ReadFailure) *cli.Problem {
	p := cli.Internal(failure)
	switch e := failure.(type) {
	case *port.Missing:
		p = cli.NewProblem(cli.CodeEnvironment, "count.input-missing",
			fmt.Sprintf("the file %s is not there", e.Path), "Check the path, or give - to count standard input.")
	case *port.Unreadable:
		p = cli.NewProblem(cli.CodeEnvironment, "count.input-unreadable",
			fmt.Sprintf("the file %s cannot be read as text", e.Path), "Give a file this program can read, not a folder.")
	case *port.Cancelled:
		p = cli.Interrupted()
	}
	return p
}
