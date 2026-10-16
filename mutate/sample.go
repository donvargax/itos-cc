package mutate

import (
	"context"

	"github.com/donvargax/itos-cc/internal/mutants"
)

// Sample retains disk composition for direct legacy callers; policy is domain-owned.
func Sample(ctx context.Context, files []string, count int, seed string, opt Options) (Sampled, error) {
	o := domainOptions(opt)
	result, err := mutants.Sample(ctx, files, count, seed, mutants.SampleOptions{CheckOptions: mutants.CheckOptions{Store: o.Store, Root: o.Root, Dir: o.Dir, Tree: o.Tree, Surroundings: o.Surroundings, Toolchain: o.Toolchain, Fingerprints: o.Fingerprints, Judge: o.Judge, Tests: o.Tests, Support: o.Support}, WorkerFactory: o.WorkerFactory, Workers: o.Workers, TimeoutFactor: o.TimeoutFactor, TestCommand: o.TestCommand, AllTests: o.AllTests, Listed: o.Listed, Progress: o.Progress})
	return result, runError(err)
}
