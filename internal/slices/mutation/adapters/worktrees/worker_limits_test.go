package worktrees

import (
	"testing"

	"github.com/donvargax/itos-cc/internal/processes"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// TestTheEnginesCapNamesAreTheDomains: a worker converts the engine's
// Result into the domain's CommandResult field for field, so the cap a run
// went over is named alike on both sides.
func TestTheEnginesCapNamesAreTheDomains(t *testing.T) {
	if processes.OverOutput != mutants.LimitOutput || processes.OverMemory != mutants.LimitMemory {
		t.Fatalf("the engine names its caps %q and %q, the domain %q and %q",
			processes.OverOutput, processes.OverMemory, mutants.LimitOutput, mutants.LimitMemory)
	}
}
