package main

import (
	"context"
	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/mutate"
	"github.com/donvargax/itos-cc/project"
)

// mutationCheckOptions contains actual read-only bindings and already admitted
// pure judge/reach facts. It supplies no disk fallback or snapshot write port.
func mutationCheckOptions(in *invocation, judge func(path, function, hash string) bool, tests func(path string) []string, support map[string]string, exceptions []config.Exception) mutants.CheckOptions {
	return mutants.CheckOptions{Store: in.adapters.SnapshotReader, Tree: in.adapters.Tree, Surroundings: in.adapters.Surroundings, Toolchain: in.adapters.Toolchain, Fingerprints: in.adapters.Fingerprints, Root: project.Root(), Dir: in.adapters.Dir, Judge: judge, Tests: tests, Support: support, Exceptions: exceptions}
}
func checkMutations(ctx context.Context, in *invocation, sources []string, judge func(path, function, hash string) bool, tests func(path string) []string, support map[string]string, exceptions []config.Exception) ([]mutate.FileCheck, error) {
	checks, err := mutants.Check(ctx, sources, mutationCheckOptions(in, judge, tests, support, exceptions))
	return checks, mutationRunError(err)
}
func checkMutationCoverage(ctx context.Context, in *invocation, sources []string, judge func(path, function, hash string) bool, languages []string, support map[string]string) ([]mutate.CoverageCheck, error) {
	enabled := map[string]bool{}
	for _, language := range languages {
		enabled[language] = true
	}
	checks, err := mutants.CheckCoverage(ctx, sources, mutationCheckOptions(in, judge, nil, support, nil), enabled, false, false)
	return checks, mutationRunError(err)
}
