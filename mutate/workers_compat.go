package mutate

import (
	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/worktrees"
)

type result = mutants.CommandResult
type OwnedExecutor = worktrees.OwnedExecutor

// legacyWorkers keeps direct library fixture callers working until T92 moves
// orchestration. cmd always supplies its actual Workers binding; the in-memory
// handler must never select this disk fallback.
func legacyWorkers(factory mutants.Workers) mutants.Workers {
	if factory != nil {
		return factory
	}
	return worktrees.Workers{}
}
