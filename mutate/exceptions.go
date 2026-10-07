package mutate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/lang"
)

// Why an exception in itos-cc.yaml no longer holds.
const (
	ExceptionKilled  = "killed"  // the tests now kill its mutant
	ExceptionChanged = "changed" // its function changed since it was written
	ExceptionGone    = "gone"    // its function, or its site in the function, is gone
)

// StaleException is an exception that no longer holds, and why.
type StaleException struct {
	config.Exception
	Why string // ExceptionKilled, ExceptionChanged, or ExceptionGone
	// Line is where the site is in the file now, or would be within its
	// function; 0 when the function is gone.
	Line int
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
// column, fresh as Check judges it with tests. A site with no such
// survivor, whether its mutant is recorded killed or uncovered, its
// function's results are stale or missing, or there is no site there, is a
// *NoSurvivorError.
func Survivor(path string, line, column int, tests func(path string) []string) (config.Exception, error) {
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
	c, err := checkParsed(f, path, nil, tests, nil)
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
			switch fn.State {
			case Missing:
				return config.Exception{}, &NoSurvivorError{fn.Function + " has no mutation results"}
			case Stale:
				return config.Exception{}, &NoSurvivorError{fn.Function + " changed since its mutation results, or the tests that import its file did"}
			}
			for _, m := range fn.Mutants {
				if m.key() != s.Key() {
					continue
				}
				if m.Outcome != Survived {
					return config.Exception{}, &NoSurvivorError{fmt.Sprintf("its mutant is recorded %s, and only a survivor is excepted", m.Outcome)}
				}
				return config.Exception{File: filepath.ToSlash(c.Rel), Function: fn.Function, Hash: UnitHash(f, u),
					LineInFunction: s.Line - u.StartLine + 1, Column: s.Column, Original: s.Original, Replacement: s.Replacement}, nil
			}
			return config.Exception{}, &NoSurvivorError{"the results of " + fn.Function + " record no mutant there"}
		}
	}
	return config.Exception{}, &NoSurvivorError{"there is no mutation site there"}
}
