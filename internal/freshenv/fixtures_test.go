package freshenv

import (
	"context"
	"io"
	"runtime"
	"testing"

	"github.com/donvargax/itos-cc/internal/importgraph"
	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/testinputs"
	"github.com/donvargax/itos-cc/internal/testtools"
	"github.com/donvargax/itos-cc/internal/workdir"
	"github.com/donvargax/itos-cc/internal/worktrees"
)

func fixtureEnvironment(root string) Environment {
	owner := &worktrees.OwnedExecutor{Platform: runtime.GOOS}
	return Environment{Platform: runtime.GOOS, Log: io.Discard, Execute: owner.Run, Command: owner.Command, Toolchain: testtools.Toolchain{Dir: root}, Fingerprints: testinputs.Fingerprints{Runtime: testinputs.CurrentRuntime()}, Reach: importgraph.Reach{}, Tree: workdir.Tree{}, Surroundings: workdir.Surroundings{}}
}
func PlanFresh(ctx context.Context, root string, paths []string, since string, count int, seed string) (*FreshPlan, error) {
	// Retained physical holders exercise the real port; no legacy implementation remains here.
	plan, failure := fixtureEnvironment(root).Plan(ctx, mutants.FreshRequest{Root: root, Paths: paths, Since: since, Count: count, Seed: seed})
	return plan, failure
}

func cleanupFreshPlan(t *testing.T, p *FreshPlan) {
	t.Helper()
	t.Cleanup(func() {
		if err := releasePlan(p); err != nil {
			t.Error(err)
		}
	})
}
