package mutate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/internal/lang"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
	"github.com/donvargax/itos-cc/project"
)

// Elsewhere judges the exceptions whose file is none of sources, of those
// judge says to judge by the file they name. One whose function, by its
// name less its namespace and by its hash, exactly one of sources holds
// has moved there: it is stale, ExceptionMoved, naming that file, and
// moved holds it as it reads there, so a run or a check still excepts its
// mutant. The namespace is left out because a file's path can be part of
// it, as in TypeScript. One whose file excluded says itos-cc.yaml leaves
// out, by the path an entry names it by, is stale as ExceptionExcluded,
// with no line, wherever its function is. Any other is stale as
// ExceptionGone, with no line. Neither changes itos-cc.yaml.
func Elsewhere(ctx context.Context, exceptions []config.Exception, sources []string, judge, excluded func(file string) bool) (stale []StaleException, moved []config.Exception, err error) {
	selected := map[string]bool{}
	for _, path := range sources {
		selected[project.FromRoot(path)] = true
	}
	var orphans []config.Exception
	for _, e := range exceptions {
		switch {
		case selected[e.File] || !judge(e.File):
		case excluded(e.File):
			stale = append(stale, StaleException{Exception: e, Why: ExceptionExcluded})
		default:
			orphans = append(orphans, e)
		}
	}
	if len(orphans) == 0 {
		return stale, nil, nil
	}
	// Where each orphan's function is now: one entry per file holding it.
	found := make([][]config.Exception, len(orphans))
	for _, path := range sources {
		f, err := project.ParseFile(ctx, path)
		if err != nil {
			return nil, nil, err
		}
		key := project.FromRoot(path)
		for i, e := range orphans {
			_, name, _ := strings.Cut(e.Function, "#")
			for _, u := range f.Units {
				if u.Name == name && UnitHash(f, u) == e.Hash {
					there := e
					there.File, there.Function = key, unitID(u.Namespace, u.Name)
					found[i] = append(found[i], there)
					break
				}
			}
		}
		f.Close()
	}
	for i, e := range orphans {
		if len(found[i]) != 1 {
			stale = append(stale, StaleException{Exception: e, Why: ExceptionGone})
			continue
		}
		stale = append(stale, StaleException{Exception: e, Why: ExceptionMoved, NewFile: found[i][0].File})
		moved = append(moved, found[i][0])
	}
	return stale, moved, stopped(ctx, nil)
}

// NoSurvivorError is a site Survivor cannot except: Reason says why.
type NoSurvivorError struct{ Reason string }

func (e *NoSurvivorError) Error() string { return e.Reason }

// Survivor is the exception, less its reason, for the survivor that the
// fresh entry of the snapshot of the file at path records at line and
// column, fresh as Check judges it with tests, or stale only for sites it
// never recorded. A site with no such survivor, whether its mutant is
// recorded killed or uncovered, its function's results are stale or
// missing, or there is no site there, is a *NoSurvivorError.
func Survivor(ctx context.Context, store snapport.Store, path string, line, column int, tests func(path string) []string, support map[string]string) (config.Exception, error) {
	if lang.Detect(path) == nil {
		return config.Exception{}, &NoSurvivorError{"it is no source file itos-cc mutates"}
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return config.Exception{}, &NoSurvivorError{"there is no such file"}
	}
	f, err := project.ParseFile(ctx, path)
	if err != nil {
		return config.Exception{}, err
	}
	defer f.Close()
	c, err := checkParsed(ctx, store, f, path, nil, tests, support, nil)
	if err != nil {
		return config.Exception{}, err
	}
	for _, s := range Sites(f) {
		if s.Line != line || s.Column != column {
			continue
		}
		u := f.Units[s.Unit]
		for _, fn := range c.Functions {
			if fn.Unit != s.Unit {
				continue
			}
			switch {
			case fn.State == Missing:
				return config.Exception{}, &NoSurvivorError{fn.Function + " has no mutation results"}
			case !fn.Judged():
				return config.Exception{}, &NoSurvivorError{fn.Function + " changed since its mutation results, or the tests that import its file did"}
			}
			for _, m := range fn.Mutants {
				if m.Key() != s.Key() {
					continue
				}
				if m.Outcome != Survived {
					return config.Exception{}, &NoSurvivorError{fmt.Sprintf("its mutant is recorded %s, and only a survivor is excepted", m.Outcome)}
				}
				return config.Exception{File: c.Key, Function: fn.Function, Hash: UnitHash(f, u),
					LineInFunction: s.Line - u.StartLine + 1, Column: s.Column, Original: s.Original, Replacement: s.Replacement,
					LineText: LineText(f.Src, s.Line)}, nil
			}
			return config.Exception{}, &NoSurvivorError{"the results of " + fn.Function + " record no mutant there"}
		}
	}
	return config.Exception{}, &NoSurvivorError{"there is no mutation site there"}
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
