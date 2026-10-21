package mutants

import (
	"path/filepath"

	"github.com/donvargax/itos-cc/internal/lang"
)

// Why an exception in itos-cc.yaml no longer holds.
const (
	ExceptionKilled   = "killed"   // the tests now kill its mutant
	ExceptionChanged  = "changed"  // its function changed since it was written
	ExceptionGone     = "gone"     // its function, or its site in the function, is gone, or its file is with no trace of it
	ExceptionMoved    = "moved"    // its file is gone, and its function, unchanged, is in one other file now
	ExceptionExcluded = "excluded" // its file is one itos-cc.yaml excludes, which no command mutates
)

// StaleException is an exception that no longer holds, and why.
type StaleException struct {
	Exception
	Why string // ExceptionKilled, ExceptionChanged, ExceptionGone, ExceptionMoved, or ExceptionExcluded
	// Line is where the site is in the file now, or would be within its
	// function; 0 when the function or its file is gone.
	Line int
	// NewFile is, with ExceptionMoved, the file that holds the function
	// now, from the project root, as an entry names its file.
	NewFile string
}

// exceptionsOf is the entries of exceptions for the file rel.
func ExceptionsOf(exceptions []Exception, rel string) []Exception {
	var out []Exception
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
func Locate(e Exception, f *lang.File, sites []Site) (site int, why string, line int) {
	why, site = ExceptionGone, -1
	for i, u := range f.Units {
		if UnitID(u.Namespace, u.Name) != e.Function {
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
type FileExceptions struct {
	Held  map[int]Exception // by the index of the site each excepts
	Stale []StaleException  // those that no longer hold without running anything
}

// applyExceptions places the exceptions of the file rel, f with sites,
// counting only those of the functions judge says to judge, all when it is
// nil.
func ApplyExceptions(exceptions []Exception, rel string, f *lang.File, sites []Site, judge func(function string) bool) FileExceptions {
	out := FileExceptions{Held: map[int]Exception{}}
	for _, e := range ExceptionsOf(exceptions, rel) {
		if judge != nil && !judge(e.Function) {
			continue
		}
		i, why, line := Locate(e, f, sites)
		if why != "" {
			out.Stale = append(out.Stale, StaleException{Exception: e, Why: why, Line: line})
			continue
		}
		out.Held[i] = e
	}
	return out
}

// exceptedKey names the mutant at the site whose Key is site in the
// function with namespace#name function and hash hash.
func ExceptedKey(function, hash, site string) string {
	return function + "\x00" + hash + "\x00" + site
}

// heldReasons is the reason of each exception of the file rel that holds
// for f, with sites, as it is now, by the function, the hash and the site
// it excepts (exceptedKey): an entry holds while its function has its hash
// and its site, which needs no run, so it is the same whichever functions a
// run judges. A stale entry is not there: it excepts nothing.
func HeldReasons(exceptions []Exception, rel string, f *lang.File, sites []Site) map[string]string {
	out := map[string]string{}
	for i, e := range ApplyExceptions(exceptions, rel, f, sites, nil).Held {
		out[ExceptedKey(e.Function, e.Hash, sites[i].Key())] = e.Reason
	}
	return out
}

// judge is how an exception's outcome now bears on the mutant it excepts:
// a survivor is excepted, with the entry's reason; a mutant the tests
// noticed makes the entry stale; and an uncovered one is neither, since an
// exception covers survivors only.
func (x FileExceptions) Judge(site int, s Site, outcome string) (reason string, stale *StaleException) {
	e, ok := x.Held[site]
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
