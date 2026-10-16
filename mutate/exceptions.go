package mutate

import (
	"context"
	"path/filepath"
	"slices"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/internal/mutants"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
	"github.com/donvargax/itos-cc/internal/workdir"
	"github.com/donvargax/itos-cc/project"
)

// Elsewhere retains concrete setup for legacy callers, using the domain rule.
func Elsewhere(ctx context.Context, exceptions []config.Exception, sources []string, judge, excluded func(string) bool) ([]StaleException, []config.Exception, error) {
	return mutants.Elsewhere(ctx, exceptions, sources, judge, excluded, legacyCheckOptions(nil, nil, nil, nil, nil, nil, nil))
}

type NoSurvivorError = mutants.NoSurvivorError

func Survivor(ctx context.Context, store snapport.Store, path string, line, column int, tests func(string) []string, support map[string]string) (config.Exception, error) {
	return SurvivorWithInputs(ctx, store, path, line, column, tests, support, nil, nil)
}
func SurvivorWithInputs(ctx context.Context, store snapport.Store, path string, line, column int, tests func(string) []string, support map[string]string, tools mutants.Toolchain, fingerprints mutants.Fingerprints) (config.Exception, error) {
	e, err := mutants.Survivor(ctx, path, line, column, legacyCheckOptions(store, nil, tests, support, nil, tools, fingerprints), workdir.SourceEdits{})
	return e, runError(err)
}

// FreshExceptions is how a project's exceptions bear on the selection of a
// fresh plan. Only the exceptions of the functions with a selected site
// count, as a --since run counts only those of the functions it judges, and
// an exception never takes a site out of the selection or skips its trial.
type FreshExceptions struct {
	held map[string]config.Exception // by the identity of the candidate each excepts
	// Stale is each counted exception that no longer holds without running
	// anything: its function changed since it was written, or its site is
	// gone.
	Stale []StaleException
}

// PlanExceptions places exceptions on the selected sites of plan, reading
// each selected file from its frozen root.
func PlanExceptions(ctx context.Context, plan *FreshPlan, exceptions []config.Exception) (*FreshExceptions, error) {
	out := &FreshExceptions{held: map[string]config.Exception{}}
	if len(exceptions) == 0 || plan == nil {
		return out, nil
	}
	byPath := map[string][]FreshCandidate{}
	var paths []string
	for _, c := range plan.Selected {
		if _, ok := byPath[c.Path]; !ok {
			paths = append(paths, c.Path)
		}
		byPath[c.Path] = append(byPath[c.Path], c)
	}
	slices.Sort(paths)
	for _, rel := range paths {
		f, err := project.ParseFile(ctx, filepath.Join(plan.FrozenRoot, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		sites := Sites(f)
		names := map[string]bool{}
		for _, c := range byPath[rel] {
			if c.Site.Unit < len(f.Units) {
				u := f.Units[c.Site.Unit]
				names[unitID(u.Namespace, u.Name)] = true
			}
		}
		x := applyExceptions(exceptions, rel, f, sites, func(function string) bool { return names[function] })
		out.Stale = append(out.Stale, x.Stale...)
		for i, e := range x.Held {
			for _, c := range byPath[rel] {
				if c.Site.Unit == sites[i].Unit && c.Site.Start == sites[i].Start && c.Site.Key() == sites[i].Key() {
					out.held[c.Identity] = e
				}
			}
		}
		f.Close()
	}
	return out, nil
}

// Judge is how the exception of the selected site c, if it has one, bears
// on its fresh outcome, as a complete run judges it: a survivor is excepted
// with the entry's reason, a killed or timed-out mutant makes the entry
// stale, and an uncovered one is neither.
func (x *FreshExceptions) Judge(c FreshCandidate, outcome string) (reason string, stale *StaleException) {
	if x == nil {
		return "", nil
	}
	e, ok := x.held[c.Identity]
	if !ok {
		return "", nil
	}
	return fileExceptions{Held: map[int]config.Exception{0: e}}.Judge(0, c.Site, outcome)
}
