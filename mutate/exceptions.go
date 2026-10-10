package mutate

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/project"
)

// Why an exception in itos-cc.yaml no longer holds.
const (
	ExceptionKilled  = "killed"  // the tests now kill its mutant
	ExceptionChanged = "changed" // its function changed since it was written
	ExceptionGone    = "gone"    // its function, or its site in the function, is gone, or its file is with no trace of it
	ExceptionMoved   = "moved"   // its file is gone, and its function, unchanged, is in one other file now
)

// StaleException is an exception that no longer holds, and why.
type StaleException struct {
	config.Exception
	Why string // ExceptionKilled, ExceptionChanged, or ExceptionGone
	// Line is where the site is in the file now, or would be within its
	// function; 0 when the function or its file is gone.
	Line int
	// NewFile is, with ExceptionMoved, the file that holds the function
	// now, from the project root, as an entry names its file.
	NewFile string
}

// exceptionsOf is the entries of exceptions for the file rel.
func exceptionsOf(exceptions []config.Exception, rel string) []config.Exception {
	var out []config.Exception
	for _, e := range exceptions {
		if e.File == filepath.ToSlash(rel) {
			out = append(out, e)
		}
	}
	return out
}

// locate finds the site e excepts among sites, f's sites. It returns the
// site's index when e holds for f as it is now. Otherwise the index is -1
// and why says why it no longer holds, with the line it names: a function
// whose hash differs but that still has the site at its place has changed,
// and its mutant is judged as if it had no entry; one with no such site, or
// no function of e's name, is gone. A function that only moved keeps its
// hash and the site's place within it.
func locate(e config.Exception, f *lang.File, sites []Site) (site int, why string, line int) {
	why, site = ExceptionGone, -1
	for i, u := range f.Units {
		if unitID(u.Namespace, u.Name) != e.Function {
			continue
		}
		hash := UnitHash(f, u)
		at := u.StartLine + e.LineInFunction - 1
		if why == ExceptionGone && line == 0 {
			line = at
		}
		for j, s := range sites {
			if s.Unit != i || s.Line != at || s.Column != e.Column || s.Original != e.Original || s.Replacement != e.Replacement {
				continue
			}
			if hash == e.Hash {
				return j, "", s.Line
			}
			why, line = ExceptionChanged, s.Line
		}
	}
	return site, why, line
}

// Elsewhere judges the exceptions whose file is none of sources, of those
// judge says to judge by the file they name. One whose function, by its
// name less its namespace and by its hash, exactly one of sources holds
// has moved there: it is stale, ExceptionMoved, naming that file, and
// moved holds it as it reads there, so a run or a check still excepts its
// mutant. The namespace is left out because a file's path can be part of
// it, as in TypeScript. Any other is stale as ExceptionGone, with no line.
// Neither changes itos-cc.yaml.
func Elsewhere(exceptions []config.Exception, sources []string, judge func(file string) bool) (stale []StaleException, moved []config.Exception, err error) {
	selected := map[string]bool{}
	for _, path := range sources {
		selected[project.FromRoot(path)] = true
	}
	var orphans []config.Exception
	for _, e := range exceptions {
		if !selected[e.File] && judge(e.File) {
			orphans = append(orphans, e)
		}
	}
	if len(orphans) == 0 {
		return nil, nil, nil
	}
	// Where each orphan's function is now: one entry per file holding it.
	found := make([][]config.Exception, len(orphans))
	for _, path := range sources {
		f, err := lang.ParseFile(path)
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
	return stale, moved, nil
}

// fileExceptions is how the exceptions of one file apply to its sites now.
type fileExceptions struct {
	held  map[int]config.Exception // by the index of the site each excepts
	stale []StaleException         // those that no longer hold without running anything
}

// applyExceptions places the exceptions of the file rel, f with sites,
// counting only those of the functions judge says to judge, all when it is
// nil.
func applyExceptions(exceptions []config.Exception, rel string, f *lang.File, sites []Site, judge func(function string) bool) fileExceptions {
	out := fileExceptions{held: map[int]config.Exception{}}
	for _, e := range exceptionsOf(exceptions, rel) {
		if judge != nil && !judge(e.Function) {
			continue
		}
		i, why, line := locate(e, f, sites)
		if why != "" {
			out.stale = append(out.stale, StaleException{Exception: e, Why: why, Line: line})
			continue
		}
		out.held[i] = e
	}
	return out
}

// exceptedKey names the mutant at the site whose Key is site in the
// function with namespace#name function and hash hash.
func exceptedKey(function, hash, site string) string {
	return function + "\x00" + hash + "\x00" + site
}

// heldReasons is the reason of each exception of the file rel that holds
// for f, with sites, as it is now, by the function, the hash and the site
// it excepts (exceptedKey): an entry holds while its function has its hash
// and its site, which needs no run, so it is the same whichever functions a
// run judges. A stale entry is not there: it excepts nothing.
func heldReasons(exceptions []config.Exception, rel string, f *lang.File, sites []Site) map[string]string {
	out := map[string]string{}
	for i, e := range applyExceptions(exceptions, rel, f, sites, nil).held {
		out[exceptedKey(e.Function, e.Hash, sites[i].Key())] = e.Reason
	}
	return out
}

// judge is how an exception's outcome now bears on the mutant it excepts:
// a survivor is excepted, with the entry's reason; a mutant the tests
// noticed makes the entry stale; and an uncovered one is neither, since an
// exception covers survivors only.
func (x fileExceptions) judge(site int, s Site, outcome string) (reason string, stale *StaleException) {
	e, ok := x.held[site]
	if !ok {
		return "", nil
	}
	switch outcome {
	case Survived:
		return e.Reason, nil
	case Killed, Timeout:
		return "", &StaleException{Exception: e, Why: ExceptionKilled, Line: s.Line}
	}
	return "", nil
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
func Survivor(path string, line, column int, tests func(path string) []string, support map[string]string) (config.Exception, error) {
	if lang.Detect(path) == nil {
		return config.Exception{}, &NoSurvivorError{"it is no source file itos-cc mutates"}
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return config.Exception{}, &NoSurvivorError{"there is no such file"}
	}
	f, err := lang.ParseFile(path)
	if err != nil {
		return config.Exception{}, err
	}
	defer f.Close()
	c, err := checkParsed(f, path, nil, tests, support, nil)
	if err != nil {
		return config.Exception{}, err
	}
	for _, s := range Sites(f) {
		if s.Line != line || s.Column != column {
			continue
		}
		u := f.Units[s.Unit]
		for _, fn := range c.Functions {
			if fn.unit != s.Unit {
				continue
			}
			switch {
			case fn.State == Missing:
				return config.Exception{}, &NoSurvivorError{fn.Function + " has no mutation results"}
			case !fn.judged():
				return config.Exception{}, &NoSurvivorError{fn.Function + " changed since its mutation results, or the tests that import its file did"}
			}
			for _, m := range fn.Mutants {
				if m.key() != s.Key() {
					continue
				}
				if m.Outcome != Survived {
					return config.Exception{}, &NoSurvivorError{fmt.Sprintf("its mutant is recorded %s, and only a survivor is excepted", m.Outcome)}
				}
				return config.Exception{File: c.key, Function: fn.Function, Hash: UnitHash(f, u),
					LineInFunction: s.Line - u.StartLine + 1, Column: s.Column, Original: s.Original, Replacement: s.Replacement}, nil
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
func PlanExceptions(plan *FreshPlan, exceptions []config.Exception) (*FreshExceptions, error) {
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
		f, err := lang.ParseFile(filepath.Join(plan.FrozenRoot, filepath.FromSlash(rel)))
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
		out.Stale = append(out.Stale, x.stale...)
		for i, e := range x.held {
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
	return fileExceptions{held: map[int]config.Exception{0: e}}.judge(0, c.Site, outcome)
}
