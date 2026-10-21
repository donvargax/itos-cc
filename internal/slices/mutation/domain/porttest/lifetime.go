package porttest

import (
	"context"
	"sync"

	"github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// Invocation observes an actual entered request and its current virtual copy.
type Invocation struct {
	Command mutants.Command
	Changed bool
	Files   map[string]string
}

// WorkGate holds actual entered commands for deterministic ownership fixtures.
// Cancellation is observed before an optional held join is allowed to return.
// It supplies no outcome or expected event sequence.
type WorkGate struct {
	MutationsOnly   bool
	Entered         chan<- Invocation
	Release         <-chan struct{}
	JoinAfterCancel <-chan struct{}
	Cancelled       chan<- Invocation
}

func (g *WorkGate) wait(ctx context.Context, entry Invocation) bool {
	if g.Entered != nil {
		select {
		case g.Entered <- entry:
		case <-ctx.Done():
			return true
		}
	}
	select {
	case <-g.Release:
		return false
	case <-ctx.Done():
		if g.Cancelled != nil {
			g.Cancelled <- entry
		}
		if g.JoinAfterCancel != nil {
			<-g.JoinAfterCancel
		}
		return true
	}
}

type InputState struct{ JoinReturned, Released bool }
type InputLifetime struct {
	mu    sync.Mutex
	state InputState
}

func (l *InputLifetime) State() InputState { l.mu.Lock(); defer l.mu.Unlock(); return l.state }
