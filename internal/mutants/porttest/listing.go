package porttest

import (
	"context"
	"fmt"
	"io"

	"github.com/donvargax/itos-cc/internal/mutants"
)

// Listing is independently observed fixture stdout/stderr and command status.
// Facts are keyed by actual root and command, never by expected selected IDs.
type Listing struct {
	Stdout, Stderr, Reason string
	ExitCode               int
}
type Lister struct {
	Listings map[string]map[string]Listing
	Log      io.Writer
}

func (l *Lister) List(ctx context.Context, c mutants.Command) (mutants.TestListing, mutants.InputFailure) {
	if ctx.Err() != nil {
		return mutants.TestListing{}, mutants.InputCancelled{Cause: context.Cause(ctx)}
	}
	fact, ok := l.Listings[c.Root][c.String()]
	if !ok {
		return mutants.TestListing{}, mutants.InputFailed{Cause: fmt.Errorf("no declared listing command %s in %s", c, c.Root)}
	}
	if l.Log != nil && fact.Stderr != "" {
		if _, err := io.WriteString(l.Log, fact.Stderr); err != nil {
			return mutants.TestListing{}, mutants.InputFailed{Cause: err}
		}
	}
	return mutants.TestListing{Stdout: fact.Stdout, ExitCode: fact.ExitCode, Reason: fact.Reason}, nil
}
