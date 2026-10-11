package mutate

import (
	"os"
	"strings"
	"unicode"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/project"
)

// Why mutation except --renew left a stale exception as it was, beside
// ExceptionKilled, ExceptionChanged and ExceptionGone.
const (
	// RenewNoResults: its function has no fresh results in its snapshot,
	// so nothing shows its mutant survived the function as it is now.
	RenewNoResults = "no-results"
	// RenewAmbiguous: more than one fresh site could be its mutant.
	RenewAmbiguous = "ambiguous"
	// RenewUncovered: its mutant is recorded uncovered, which no exception
	// excuses.
	RenewUncovered = "uncovered"
)

// How a renewal found its mutant (Renewal.Match).
const (
	// MatchLine: the one site of the entry's change on a line whose text,
	// less its space, is the line the entry recorded (config.Exception.
	// LineText), or of several such the one at the entry's place.
	MatchLine = "line"
	// MatchPlace: an entry with no line text, at its place in the
	// function: the same line_in_function and column.
	MatchPlace = "place"
	// MatchOnly: an entry with no line text whose place has no such site,
	// and whose function has exactly one site of its change.
	MatchOnly = "only"
)

// Renewal is a stale exception renewed: Old as itos-cc.yaml has it, and
// New, at its function's current hash, place and line text, with Old's
// reason. Line is the site's line in New's file.
type Renewal struct {
	Old, New config.Exception
	Line     int
	Match    string
}

// Unrenewed is a stale exception mutation except --renew left as it was,
// and why: ExceptionKilled, ExceptionChanged (no site of its change is on
// its line, whose text changed), ExceptionGone (its file, its function, or
// any site of its change in it), RenewNoResults, RenewAmbiguous or
// RenewUncovered. Line is where its function's matching site is now, when
// there is one, else 0.
type Unrenewed struct {
	config.Exception
	Why  string
	Line int
}

// Renew renews each exception of exceptions that no longer holds when its
// mutant is unchanged: its function, found by its namespace#name in its
// file, else by its name alone there when exactly one function has it, or,
// its file gone, in exactly one of sources with its name and hash (a move),
// has fresh results in its file's snapshot, as Check judges them with tests
// and support, and they hold exactly one site of the entry's original and
// replacement that is its mutant (Match), recorded survived. An exception
// that holds is left alone, unless its fresh results record its mutant
// killed, which no renewal can mend. It writes nothing; the caller writes
// the renewals (config.Renew).
func Renew(exceptions []config.Exception, sources []string, tests func(path string) []string, support map[string]string) ([]Renewal, []Unrenewed, error) {
	var renewed []Renewal
	var left []Unrenewed
	files := map[string]*renewFile{}
	defer func() {
		for _, f := range files {
			f.file.Close()
		}
	}()
	open := func(path string) (*renewFile, error) {
		if f, ok := files[path]; ok {
			return f, nil
		}
		f, err := lang.ParseFile(path)
		if err != nil {
			return nil, err
		}
		c, err := checkParsed(f, path, nil, tests, support, nil)
		if err != nil {
			f.Close()
			return nil, err
		}
		files[path] = &renewFile{file: f, check: c, sites: Sites(f)}
		return files[path], nil
	}
	for _, e := range exceptions {
		path := project.AtRoot(e.File)
		var target *renewFile
		unit := -1
		why := ExceptionGone
		if info, err := os.Stat(path); err == nil && !info.IsDir() && lang.Detect(path) != nil {
			f, err := open(path)
			if err != nil {
				return nil, nil, err
			}
			target = f
			if i, ok := f.unitOf(e); ok {
				unit = i
			} else if i >= 0 {
				why = RenewAmbiguous
			}
		} else if err == nil || os.IsNotExist(err) {
			for _, source := range sources {
				if project.FromRoot(source) == e.File {
					continue
				}
				f, err := open(source)
				if err != nil {
					return nil, nil, err
				}
				_, name, _ := strings.Cut(e.Function, "#")
				for i, u := range f.file.Units {
					if u.Name == name && UnitHash(f.file, u) == e.Hash {
						if target != nil && (target != f || unit != i) {
							why = RenewAmbiguous
						}
						target, unit = f, i
					}
				}
			}
			if why == RenewAmbiguous {
				target, unit = nil, -1
			}
		} else {
			return nil, nil, err
		}
		if target == nil || unit < 0 {
			left = append(left, Unrenewed{Exception: e, Why: why})
			continue
		}
		f := target
		u := f.file.Units[unit]
		if project.FromRoot(f.file.Path) == e.File && f.holds(e) {
			// It holds: only a fresh kill makes it stale, and nothing
			// renews that.
			if m, ok := f.recorded(unit, e.LineInFunction+u.StartLine-1, e.Column, e.Original, e.Replacement); ok && (m.Outcome == Killed || m.Outcome == Timeout) {
				left = append(left, Unrenewed{Exception: e, Why: ExceptionKilled, Line: m.Line})
			}
			continue
		}
		fn, ok := f.function(unit)
		if !ok {
			left = append(left, Unrenewed{Exception: e, Why: RenewNoResults})
			continue
		}
		m, match, why := matchMutant(e, u.StartLine, fn.Mutants, f.file.Src)
		if why != "" {
			left = append(left, Unrenewed{Exception: e, Why: why})
			continue
		}
		if why := survivedWhy(m); why != "" {
			left = append(left, Unrenewed{Exception: e, Why: why, Line: m.Line})
			continue
		}
		renewed = append(renewed, Renewal{Old: e, Line: m.Line, Match: match, New: config.Exception{
			File: project.FromRoot(f.file.Path), Function: unitID(u.Namespace, u.Name), Hash: UnitHash(f.file, u),
			LineInFunction: m.Line - u.StartLine + 1, Column: m.Column, Original: e.Original, Replacement: e.Replacement,
			Reason: e.Reason, LineText: LineText(f.file.Src, m.Line),
		}})
	}
	return renewed, left, nil
}

// renewFile is a source Renew reads: parsed, checked against its snapshot,
// and its sites.
type renewFile struct {
	file  *lang.File
	check FileCheck
	sites []Site
}

// unitOf is the index of the function of f e names: by its namespace#name,
// else by its name alone when exactly one function of f has it, as when a
// renamed module or package changed its namespace. When several have it,
// the index is that of one of them, and false says it is ambiguous; -1
// when none has.
func (f *renewFile) unitOf(e config.Exception) (int, bool) {
	for i, u := range f.file.Units {
		if unitID(u.Namespace, u.Name) == e.Function {
			return i, true
		}
	}
	_, name, _ := strings.Cut(e.Function, "#")
	found := -1
	for i, u := range f.file.Units {
		if u.Name != name {
			continue
		}
		if found >= 0 {
			return found, false
		}
		found = i
	}
	return found, found >= 0
}

// holds says whether e holds for f as it is now (locate), so it needs no
// renewal.
func (f *renewFile) holds(e config.Exception) bool {
	_, why, _ := locate(e, f.file, f.sites)
	return why == ""
}

// function is the check of f's function at unit when its results are fresh
// (FunctionCheck.judged), and false otherwise.
func (f *renewFile) function(unit int) (FunctionCheck, bool) {
	for _, fn := range f.check.Functions {
		if fn.unit == unit {
			return fn, fn.judged() && fn.State != Missing
		}
	}
	return FunctionCheck{}, false
}

// recorded is the fresh result of the mutant of f's function at unit at
// line and column with original and replacement.
func (f *renewFile) recorded(unit, line, column int, original, replacement string) (Mutant, bool) {
	fn, ok := f.function(unit)
	if !ok {
		return Mutant{}, false
	}
	for _, m := range fn.Mutants {
		if m.Line == line && m.Column == column && m.Original == original && m.Replacement == replacement {
			return m, true
		}
	}
	return Mutant{}, false
}

// matchMutant finds e's mutant among mutants, the fresh results of its
// function, which starts at start now, in a file of src: the one result of
// e's original and replacement that Match describes. Otherwise why says why
// none is: ExceptionGone when the function has no result of that change,
// ExceptionChanged when an entry with line text finds none on a line with
// that text, RenewAmbiguous when more than one could be it.
func matchMutant(e config.Exception, start int, mutants []Mutant, src []byte) (m Mutant, match, why string) {
	var change []Mutant
	for _, m := range mutants {
		if m.Original == e.Original && m.Replacement == e.Replacement {
			change = append(change, m)
		}
	}
	if len(change) == 0 {
		return Mutant{}, "", ExceptionGone
	}
	atPlace := func(ms []Mutant) []Mutant {
		var out []Mutant
		for _, m := range ms {
			if m.Line-start+1 == e.LineInFunction && m.Column == e.Column {
				out = append(out, m)
			}
		}
		return out
	}
	if e.LineText != "" {
		want := withoutSpace(e.LineText)
		var onLine []Mutant
		for _, m := range change {
			if withoutSpace(LineText(src, m.Line)) == want {
				onLine = append(onLine, m)
			}
		}
		switch {
		case len(onLine) == 0:
			return Mutant{}, "", ExceptionChanged
		case len(onLine) == 1:
			return onLine[0], MatchLine, ""
		}
		if placed := atPlace(onLine); len(placed) == 1 {
			return placed[0], MatchLine, ""
		}
		return Mutant{}, "", RenewAmbiguous
	}
	if placed := atPlace(change); len(placed) == 1 {
		return placed[0], MatchPlace, ""
	}
	if len(change) == 1 {
		return change[0], MatchOnly, ""
	}
	return Mutant{}, "", RenewAmbiguous
}

// survivedWhy is "" for a mutant recorded survived, the one outcome that
// renews its exception, and why it does not otherwise: ExceptionKilled for
// a kill or a timeout, RenewUncovered, or RenewNoResults with no outcome.
func survivedWhy(m Mutant) string {
	switch m.Outcome {
	case Survived:
		return ""
	case Killed, Timeout:
		return ExceptionKilled
	case Uncovered:
		return RenewUncovered
	}
	return RenewNoResults
}

// LineText is line of src, counted from 1, less the space around it: what
// an exception records of its site's line (config.Exception.LineText).
func LineText(src []byte, line int) string {
	lines := strings.SplitN(string(src), "\n", line+1)
	if line < 1 || line > len(lines) {
		return ""
	}
	return strings.TrimSpace(lines[line-1])
}

// withoutSpace is s with every space removed, so a reformatted line reads
// as the line it was.
func withoutSpace(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}
