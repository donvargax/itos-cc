package main

import (
	"context"

	mutation "github.com/donvargax/itos-cc/internal/mutants"
	mutationslice "github.com/donvargax/itos-cc/internal/mutation"
	"github.com/donvargax/itos-cc/project"
)

// The remaining cmd handlers share presentation with list/check until their
// own slice moves. These are genuine caller delegates, never a second printer.
func presentationFor(in *invocation) *mutationslice.Presentation {
	return &mutationslice.Presentation{UI: stdUI(), JSON: in.json, Root: project.Root(), Tree: in.adapters.Tree}
}
func quote(s string) string                       { return mutationslice.Quote(s) }
func languageName(s string) string                { return mutationslice.LanguageName(s) }
func coverageInventory(s string) string           { return mutationslice.CoverageInventory(s) }
func executableUnit(s string) string              { return mutationslice.ExecutableUnit(s) }
func exceptedIn(items []mutation.Mutant) int      { return mutationslice.ExceptedIn(items) }
func strictLanguagesOf(sources []string) []string { return mutationslice.StrictLanguages(sources) }
func tail(s string, lines int) string             { return mutationslice.Tail(s, lines) }
func reportFailed(in *invocation, rel, function string, items []mutation.Mutant, failUncovered func(int) bool) {
	p := presentationFor(in)
	p.Failed(rel, function, items, failUncovered)
	in.problems = append(in.problems, p.Problems...)
}
func reportLanguageCoverageProblem(in *invocation, language, file, function string, line int, rule, message string) {
	p := presentationFor(in)
	p.CoverageProblem(language, file, function, line, rule, message)
	in.problems = append(in.problems, p.Problems...)
}
func reportStaleExceptions(ctx context.Context, in *invocation, rel string, stale []mutation.StaleException) {
	p := presentationFor(in)
	p.StaleExceptions(ctx, rel, stale)
	in.problems = append(in.problems, p.Problems...)
}
func reportElsewhere(ctx context.Context, in *invocation, stale []mutation.StaleException) {
	p := presentationFor(in)
	p.Elsewhere(ctx, stale)
	in.problems = append(in.problems, p.Problems...)
}
func staleSubject(rel string, e mutation.StaleException) map[string]any {
	return (&mutationslice.Presentation{UI: stdUI(), Root: project.Root()}).StaleSubject(rel, e)
}
