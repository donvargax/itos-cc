package domain

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/snapshot"
)

// Identity is a mutant as a counted run names it (AdmitFresh):
// <file>:<namespace#name>@<start>:<end>:<hash>:<site key>, the file from
// the project root, slash-separated, the function's lines and hash, and
// the site's key within it (Site.Key), its offset from the function's
// start, comments left out, then <original>><replacement>.
type Identity struct {
	File, Namespace, Name string
	StartLine, EndLine    int
	Hash, Key             string
}

// identityUnit is the part of an identity that tells it from a site,
// <file>:<line>:<column>: the function's lines and hash, and the site's
// offset.
var identityUnit = regexp.MustCompile(`@(\d+):(\d+):([0-9a-f]+):(\d+):`)

// IsIdentity is whether arg is written as an identity, holding its
// @<start>:<end>:<hash>:<offset>: part, rather than as a site.
func IsIdentity(arg string) bool { return identityUnit.MatchString(arg) }

// ParseIdentity reads arg as an identity, false when it is none. The site
// key is kept whole, never split at its >, which an original or a
// replacement may hold (`>` → `>=` is >>>=). The function's part is read
// from the last such part's @, its name after its last #, and the file
// before the last : that precedes it, which no namespace and no name holds;
// a file that is not local to the project root is none. The identity is
// text, its file slash-separated on every system, so the file is judged by
// the slash rules of path, never by the system's (filepath), whose Windows
// rules refuse a : that a file elsewhere may hold.
func ParseIdentity(arg string) (Identity, bool) {
	all := identityUnit.FindAllStringSubmatchIndex(arg, -1)
	if len(all) == 0 {
		return Identity{}, false
	}
	m := all[len(all)-1]
	function := arg[:m[0]]
	// With no #, hash is -1 and no : is looked for.
	hash := strings.LastIndex(function, "#")
	colon := strings.LastIndex(function[:hash+1], ":")
	if colon <= 0 || m[1] == len(arg) {
		return Identity{}, false
	}
	start, err1 := strconv.Atoi(arg[m[2]:m[3]])
	end, err2 := strconv.Atoi(arg[m[4]:m[5]])
	id := Identity{File: function[:colon], Namespace: function[colon+1 : hash], Name: function[hash+1:],
		StartLine: start, EndLine: end, Hash: arg[m[6]:m[7]], Key: arg[m[8]:]}
	if id.Name == "" || err1 != nil || err2 != nil || !local(id.File) {
		return Identity{}, false
	}
	return id, true
}

// local is whether file, slash-separated, names a path within the
// directory it is read from, neither absolute nor climbing out of it;
// ParseIdentity has refused an empty one.
func local(file string) bool {
	clean := path.Clean(file)
	return !path.IsAbs(file) && clean != ".." && !strings.HasPrefix(clean, "../")
}

// String is the identity as a counted run names it.
func (id Identity) String() string {
	return fmt.Sprintf("%s:%s@%d:%d:%s:%s", id.File, id.Function(), id.StartLine, id.EndLine, id.Hash, id.Key)
}

// Function is the identity's function, namespace#name.
func (id Identity) Function() string { return UnitID(id.Namespace, id.Name) }

// IdentityStaleError is an identity that names no site of the source as it
// is now: Why is ExceptionChanged when a function of its name is there
// with another hash, Hash, and ExceptionGone when its file, every function
// of its name, or its site in the function is gone.
type IdentityStaleError struct {
	Why, Hash string
}

func (e *IdentityStaleError) Error() string {
	if e.Why == ExceptionChanged {
		return "its function changed, its hash now " + e.Hash
	}
	return "its file, function or site is gone"
}

// IdentifiedSurvivor is the exception, less its reason, for the mutant id
// names, and the line of its site, found in the source as it is now with
// no mutation run: its file, from opt.Root, the function of its name
// whose hash is its hash, the namespace compared where it is the unit's
// own (in Python and TypeScript a counted run names the module from the
// committed tree, stableFunction), its lines choosing among several such,
// then the site of its key. A function that only moved is found. One
// that changed, or a file, function or site that is gone, is an
// *IdentityStaleError, and so is a file the system cannot name within
// opt.Root (filepath.Localize), such as one holding a : on Windows: the
// identity's slash-separated file becomes the system's path only here,
// where it is read. A missing or stale snapshot of the function is no
// refusal: the identity is the evidence that it survived. A fresh one that
// records the mutant killed, timed out, over the limit or uncovered is a
// *NoSurvivorError, as Survivor's, at the site.
func IdentifiedSurvivor(ctx context.Context, id Identity, opt CheckOptions, status SourceStatus) (Exception, int, error) {
	if ctx.Err() != nil {
		return Exception{}, 0, context.Cause(ctx)
	}
	gone := &IdentityStaleError{Why: ExceptionGone}
	file, err := filepath.Localize(path.Clean(id.File))
	if err != nil {
		return Exception{}, 0, gone
	}
	source := filepath.Join(opt.Root, file)
	if lang.Detect(source) == nil {
		return Exception{}, 0, gone
	}
	if status == nil {
		return Exception{}, 0, errors.New("mutation source status binding is missing")
	}
	kind, failure := status.Kind(ctx, source)
	var canceled InputCancelled
	if errors.As(failure, &canceled) {
		return Exception{}, 0, failure
	}
	if failure != nil || kind != SourceNonDirectory {
		return Exception{}, 0, gone
	}
	f, err := parseExceptionSource(ctx, source, opt)
	if err != nil {
		return Exception{}, 0, err
	}
	defer f.Close()
	unit, err := identifiedUnit(f, id)
	if err != nil {
		return Exception{}, 0, err
	}
	u := f.Units[unit]
	for _, s := range Sites(f) {
		if s.Unit != unit || s.Key() != id.Key {
			continue
		}
		if err := recordedSurvivor(ctx, f, source, unit, s, opt); err != nil {
			return Exception{}, 0, err
		}
		return Exception{File: snapshot.Key(opt.Root, source), Function: UnitID(u.Namespace, u.Name), Hash: UnitHash(f, u),
			LineInFunction: s.Line - u.StartLine + 1, Column: s.Column, Original: s.Original, Replacement: s.Replacement,
			LineText: LineText(f.Src, s.Line)}, s.Line, nil
	}
	return Exception{}, 0, gone
}

// identifiedUnit is the index of f's unit id names: of those of its name,
// and namespace where it is the unit's own, the one at its lines among
// those with its hash, else the nearest. None of its name is gone; none
// with its hash is changed, with the hash of the nearest.
func identifiedUnit(f *lang.File, id Identity) (int, error) {
	own := f.Spec.Name != "python" && f.Spec.Name != "typescript"
	named, hashed := -1, -1
	for i, u := range f.Units {
		if u.Name != id.Name || own && u.Namespace != id.Namespace {
			continue
		}
		if UnitHash(f, u) != id.Hash {
			named = nearer(f.Units, id, named, i)
			continue
		}
		if u.StartLine == id.StartLine && u.EndLine == id.EndLine {
			return i, nil
		}
		hashed = nearer(f.Units, id, hashed, i)
	}
	switch {
	case hashed >= 0:
		return hashed, nil
	case named >= 0:
		return -1, &IdentityStaleError{Why: ExceptionChanged, Hash: UnitHash(f, f.Units[named])}
	}
	return -1, &IdentityStaleError{Why: ExceptionGone}
}

// nearer is whichever of the units at best and i starts nearer id's
// start, best when they are as near; i when best is -1.
func nearer(units []lang.Unit, id Identity, best, i int) int {
	if best < 0 || distance(units[i].StartLine, id.StartLine) < distance(units[best].StartLine, id.StartLine) {
		return i
	}
	return best
}

func distance(a, b int) int { return max(a-b, b-a) }

// recordedSurvivor refuses the site s of f's unit when the fresh results of
// its function, fresh as Check judges them with tests, record its mutant
// other than survived: a missing or stale entry, or one that never recorded
// the site, refuses nothing.
func recordedSurvivor(ctx context.Context, f *lang.File, path string, unit int, s Site, opt CheckOptions) error {
	c, err := CheckParsed(ctx, f, path, opt)
	if err != nil {
		return err
	}
	for _, fn := range c.Functions {
		if fn.Unit != unit || fn.State == Missing || !fn.Judged() {
			continue
		}
		for _, m := range fn.Mutants {
			if m.Key() == s.Key() {
				if err := survived(m.Outcome, m.Flaky); err != nil {
					err.Line, err.Column = s.Line, s.Column
					return err
				}
			}
		}
	}
	return nil
}

// survived refuses a mutant recorded as outcome unless it survived: an
// uncovered one on purpose, as no exception covers it, and a flaky one,
// whose survival is not trusted.
func survived(outcome string, flaky bool) *NoSurvivorError {
	switch {
	case flaky:
		return &NoSurvivorError{Reason: "its mutant is recorded flaky, which its tests must settle, and only a survivor is excepted"}
	case outcome == Survived:
		return nil
	case outcome == Uncovered:
		return &NoSurvivorError{Reason: "its mutant is recorded uncovered", Uncovered: true}
	}
	return &NoSurvivorError{Reason: fmt.Sprintf("its mutant is recorded %s, and only a survivor is excepted", outcome)}
}
