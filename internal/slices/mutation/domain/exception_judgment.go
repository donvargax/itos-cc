package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/snapshot"
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
func Elsewhere(ctx context.Context, exceptions []Exception, sources []string, judge, excluded func(file string) bool, opt CheckOptions) (stale []StaleException, moved []Exception, err error) {
	if ctx.Err() != nil {
		return nil, nil, context.Cause(ctx)
	}
	selectedFiles := map[string]bool{}
	for _, path := range sources {
		selectedFiles[snapshot.Key(opt.Root, absoluteSource(opt.Dir, path))] = true
	}
	var orphans []Exception
	for _, e := range exceptions {
		switch {
		case selectedFiles[e.File] || !judge(e.File):
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
	found := make([][]Exception, len(orphans))
	for _, path := range sources {
		f, err := parseExceptionSource(ctx, path, opt)
		if err != nil {
			return nil, nil, err
		}
		key := snapshot.Key(opt.Root, absoluteSource(opt.Dir, path))
		for i, e := range orphans {
			_, name, _ := strings.Cut(e.Function, "#")
			for _, u := range f.Units {
				if u.Name == name && UnitHash(f, u) == e.Hash {
					there := e
					there.File, there.Function = key, UnitID(u.Namespace, u.Name)
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

// Repointed is the index of the entry of entries that excepting survivor,
// the exception Survivor gives for a site of the file at path, re-points:
// the one entry that names survivor's file, its function by namespace#name,
// and its original and replacement, and that no longer holds for the file
// as it is now (Locate), its function changed or its site moved. An entry
// that holds, as one stale as killed does, and one whose file moved name no
// such entry. -1 when there is none, or more than one.
func Repointed(ctx context.Context, entries []Exception, survivor Exception, path string, opt CheckOptions) (int, error) {
	if ctx.Err() != nil {
		return -1, context.Cause(ctx)
	}
	var candidates []int
	for i, e := range entries {
		if e.File == survivor.File && e.Function == survivor.Function && e.Original == survivor.Original && e.Replacement == survivor.Replacement {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		return -1, nil
	}
	f, err := parseExceptionSource(ctx, absoluteSource(opt.Dir, path), opt)
	if err != nil {
		return -1, err
	}
	defer f.Close()
	sites := Sites(f)
	found := -1
	for _, i := range candidates {
		if _, why, _ := Locate(entries[i], f, sites); why == "" {
			continue
		}
		if found >= 0 {
			return -1, nil
		}
		found = i
	}
	return found, nil
}

// NoSurvivorError is a site Survivor cannot except: Reason says why, and
// Uncovered is true when its mutant is recorded uncovered, which an
// exception never covers, on purpose: it needs a test.
type NoSurvivorError struct {
	Reason    string
	Uncovered bool
}

func (e *NoSurvivorError) Error() string { return e.Reason }

// Survivor is the exception, less its reason, for the survivor that the
// fresh entry of the snapshot of the file at path records at line and
// column, fresh as Check judges it with tests, or stale only for sites it
// never recorded. A site with no such survivor, whether its mutant is
// recorded killed or uncovered, its function's results are stale or
// missing, or there is no site there, is a *NoSurvivorError.
func Survivor(ctx context.Context, path string, line, column int, opt CheckOptions, status SourceStatus) (Exception, error) {
	if ctx.Err() != nil {
		return Exception{}, context.Cause(ctx)
	}
	path = absoluteSource(opt.Dir, path)
	if lang.Detect(path) == nil {
		return Exception{}, &NoSurvivorError{Reason: "it is no source file itos-cc mutates"}
	}
	if status == nil {
		return Exception{}, errors.New("mutation source status binding is missing")
	}
	kind, failure := status.Kind(ctx, path)
	if failure != nil {
		var canceled InputCancelled
		if errors.As(failure, &canceled) {
			return Exception{}, failure
		}
	}
	if failure != nil || kind != SourceNonDirectory {
		return Exception{}, &NoSurvivorError{Reason: "there is no such file"}
	}
	f, err := parseExceptionSource(ctx, path, opt)
	if err != nil {
		return Exception{}, err
	}
	defer f.Close()
	c, err := CheckParsed(ctx, f, path, opt)
	if err != nil {
		return Exception{}, err
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
				return Exception{}, &NoSurvivorError{Reason: fn.Function + " has no mutation results"}
			case !fn.Judged():
				return Exception{}, &NoSurvivorError{Reason: fn.Function + " changed since its mutation results, or the tests that import its file did"}
			}
			for _, m := range fn.Mutants {
				if m.Key() != s.Key() {
					continue
				}
				switch m.Outcome {
				case Survived:
				case Uncovered:
					return Exception{}, &NoSurvivorError{Reason: "its mutant is recorded uncovered", Uncovered: true}
				default:
					return Exception{}, &NoSurvivorError{Reason: fmt.Sprintf("its mutant is recorded %s, and only a survivor is excepted", m.Outcome)}
				}
				return Exception{File: c.Key, Function: fn.Function, Hash: UnitHash(f, u),
					LineInFunction: s.Line - u.StartLine + 1, Column: s.Column, Original: s.Original, Replacement: s.Replacement,
					LineText: LineText(f.Src, s.Line)}, nil
			}
			return Exception{}, &NoSurvivorError{Reason: "the results of " + fn.Function + " record no mutant there"}
		}
	}
	return Exception{}, &NoSurvivorError{Reason: "there is no mutation site there"}
}
