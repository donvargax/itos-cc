package domain_test

import (
	"testing"

	"github.com/donvargax/itos-cc/internal/lang"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// staleTwiceSource holds Twice beside Count, its two sites of `>` → `>=` on lines
// of one text, so an entry can read as either.
const staleTwiceSource = "package board\n\nfunc Count(x int) bool {\n\treturn x < 5\n}\n\n" +
	"func Twice(x int) bool {\n\tif x == 0 {\n\t\treturn x > 1\n\t}\n\treturn x > 1\n}\n"

// staleEntry is an entry for the function name of parsed, at the given place,
// stale by its hash.
func staleEntry(parsed *lang.File, units map[string]int, name string, line, column int, original, replacement, text string) mutants.Exception {
	u := parsed.Units[units[name]]
	return mutants.Exception{File: "sub/board.go", Function: mutants.UnitID(u.Namespace, u.Name), Hash: "0000000000000000",
		LineInFunction: line, Column: column, Original: original, Replacement: replacement, Reason: "kept", LineText: text}
}

// slice-78: an entry stale as changed names the identity of the one site
// --renew's rule says is its mutant, from the entry's file, which IdentifiedSite
// reads back as that site; any other names none.
func TestApplyExceptionsNamesTheIdentityThatRepointsAChangedEntry(t *testing.T) {
	f := newRunFixtureOf(t, staleTwiceSource)
	parsed, sites, units := rehashParsed(t, f)
	for _, c := range []struct {
		name  string
		entry mutants.Exception
		line  int
	}{
		{"its line's text", staleEntry(parsed, units, "Count", 7, 11, "<", "<=", "return x < 5"), 4},
		{"its line's text, reformatted", staleEntry(parsed, units, "Count", 2, 11, "<", "<=", "return  x<5"), 4},
		{"its place, with no line text", staleEntry(parsed, units, "Count", 2, 11, "<", "<=", ""), 4},
		{"the only site, with no line text", staleEntry(parsed, units, "Count", 9, 11, "<", "<=", ""), 4},
		{"the one at its place, of two of its text", staleEntry(parsed, units, "Twice", 3, 12, ">", ">=", "return x > 1"), 9},
		{"its place, of two with no line text", staleEntry(parsed, units, "Twice", 5, 11, ">", ">=", ""), 11},
	} {
		got := mutants.ApplyExceptions([]mutants.Exception{c.entry}, "sub/board.go", parsed, sites, nil).Stale
		if len(got) != 1 || got[0].Why != mutants.ExceptionChanged || got[0].Identity == "" {
			t.Errorf("%s: stale %+v, want changed, with an identity", c.name, got)
			continue
		}
		id, ok := mutants.ParseIdentity(got[0].Identity)
		if !ok || id.File != "sub/board.go" {
			t.Errorf("%s: %q is no identity from the entry's file", c.name, got[0].Identity)
			continue
		}
		site, err := mutants.IdentifiedSite(parsed, id)
		if err != nil || site.Line != c.line || site.Original != c.entry.Original {
			t.Errorf("%s: %q reads back as %+v, %v, want line %d", c.name, got[0].Identity, site, err, c.line)
		}
	}
	for _, c := range []struct {
		name  string
		entry mutants.Exception
		why   string
	}{
		{"a changed line", staleEntry(parsed, units, "Count", 2, 11, "<", "<=", "return x < 6"), mutants.ExceptionChanged},
		{"two of its text, none at its place", staleEntry(parsed, units, "Twice", 7, 11, ">", ">=", "return x > 1"), mutants.ExceptionChanged},
		{"two with no line text, none at its place", staleEntry(parsed, units, "Twice", 7, 11, ">", ">=", ""), mutants.ExceptionChanged},
		{"no site of its change", staleEntry(parsed, units, "Count", 2, 11, ">", ">=", "return x < 5"), mutants.ExceptionGone},
		{"no such function", mutants.Exception{File: "sub/board.go", Function: "board#Below", Hash: "0000000000000000", LineInFunction: 2, Column: 11, Original: "<", Replacement: "<="}, mutants.ExceptionGone},
	} {
		got := mutants.ApplyExceptions([]mutants.Exception{c.entry}, "sub/board.go", parsed, sites, nil).Stale
		if len(got) != 1 || got[0].Why != c.why || got[0].Identity != "" {
			t.Errorf("%s: stale %+v, want %s, with no identity", c.name, got, c.why)
		}
	}
}

// What --rehash leaves as changed names the identity, and what it leaves as
// gone none. A rehashable entry is carried over.
func TestRehashLeavesAChangedEntryItsIdentity(t *testing.T) {
	f := newRunFixtureOf(t, rehashSource)
	parsed, _, units := rehashParsed(t, f)
	changed := staleEntry(parsed, units, "Count", 3, 11, "<", "<=", "")
	changed.File = "board.go"
	gone := changed
	gone.Original = ">"
	_, left, err := mutants.Rehash(t.Context(), []mutants.Exception{changed, gone}, exceptionOptions(f), f.edits)
	if err != nil || len(left) != 2 {
		t.Fatalf("left %+v, %v", left, err)
	}
	if left[0].Why != mutants.ExceptionChanged || !mutants.IsIdentity(left[0].Identity) {
		t.Errorf("changed: %+v", left[0])
	}
	if left[1].Why != mutants.ExceptionGone || left[1].Identity != "" {
		t.Errorf("gone: %+v", left[1])
	}
}

// What --renew leaves for want of results names the identity, in the entry's
// own file and by its own namespace#name alone: an identity of another file or
// namespace re-points no entry. With fresh results, what it leaves names none.
func TestRenewLeavesANoResultsEntryItsIdentity(t *testing.T) {
	source := "package board\nfunc Value(x int) bool {\n\treturn x > 1\n}\n"
	f := newRunFixtureOf(t, source)
	parsed, sites, _ := rehashParsed(t, f)
	u, s := parsed.Units[0], sites[0]
	entry := mutants.Exception{File: "board.go", Function: mutants.UnitID(u.Namespace, u.Name), Hash: "0000000000000000",
		LineInFunction: s.Line - u.StartLine + 1, Column: s.Column, Original: s.Original, Replacement: s.Replacement}
	_, left, err := mutants.Renew(t.Context(), []mutants.Exception{entry}, []string{f.path}, exceptionOptions(f), f.edits)
	if err != nil || len(left) != 1 || left[0].Why != mutants.RenewNoResults || !mutants.IsIdentity(left[0].Identity) {
		t.Fatalf("no results: %+v %v", left, err)
	}
	renamed := entry
	renamed.Function = "another/namespace#" + u.Name
	_, left, err = mutants.Renew(t.Context(), []mutants.Exception{renamed}, []string{f.path}, exceptionOptions(f), f.edits)
	if err != nil || len(left) != 1 || left[0].Why != mutants.RenewNoResults || left[0].Identity != "" {
		t.Fatalf("another namespace: %+v %v", left, err)
	}
	moved := entry
	moved.File, moved.Hash = "old.go", mutants.UnitHash(parsed, u)
	_, left, err = mutants.Renew(t.Context(), []mutants.Exception{moved}, []string{f.path}, exceptionOptions(f), f.edits)
	if err != nil || len(left) != 1 || left[0].Why != mutants.RenewNoResults || left[0].Identity != "" {
		t.Fatalf("moved: %+v %v", left, err)
	}
	f, _ = survivingFixtureOf(t, source)
	changed := entry
	changed.LineText = "return x >= 1"
	_, left, err = mutants.Renew(t.Context(), []mutants.Exception{changed}, []string{f.path}, exceptionOptions(f), f.edits)
	if err != nil || len(left) != 1 || left[0].Why != mutants.ExceptionChanged || left[0].Identity != "" {
		t.Fatalf("changed: %+v %v", left, err)
	}
}

// twoMovedSource is Count with a line added above two sites of `<` → `<=`,
// "a := x < 5" now at line_in_function 3 and "b := y < 7" at 4, and two more
// on one line, at 5.
const twoMovedSource = "package board\n\nfunc Count(x, y int) bool {\n\t_ = x\n\ta := x < 5\n\tb := y < 7\n\treturn a && b && x < 9 && y < 9\n}\n"

// slice-80: of the stale entries of the survivor's change, the one whose
// mutant --renew's rule names as the survivor's site is re-pointed, so the
// entry an identity names is the one it re-points, even over an entry at the
// survivor's place whose own site moved. Only when none, or more than one,
// names the site, the entry at the place, else the one stale entry.
func TestRepointedIsTheStaleEntryWhoseMutantIsTheSurvivor(t *testing.T) {
	f := newRunFixtureOf(t, twoMovedSource)
	parsed, sites, units := rehashParsed(t, f)
	u := parsed.Units[units["Count"]]
	var changes []mutants.Site
	for _, s := range sites {
		if s.Original == "<" && s.Replacement == "<=" {
			changes = append(changes, s)
		}
	}
	if len(changes) != 4 {
		t.Fatalf("Count has %d sites of `<` → `<=`, not 4: %+v", len(changes), changes)
	}
	survivor := func(s mutants.Site) mutants.Exception {
		return mutants.Exception{File: "sub/board.go", Function: mutants.UnitID(u.Namespace, u.Name), Hash: mutants.UnitHash(parsed, u),
			LineInFunction: s.Line - u.StartLine + 1, Column: s.Column, Original: s.Original, Replacement: s.Replacement, LineText: mutants.LineText(parsed.Src, s.Line)}
	}
	a, b, x9 := survivor(changes[0]), survivor(changes[1]), survivor(changes[2])
	entry := func(line, column int, text string) mutants.Exception {
		return staleEntry(parsed, units, "Count", line, column, "<", "<=", text)
	}
	// first and second are a's and b's entries from before the line was added:
	// second is at a's place now.
	first, second := entry(2, 9, "a := x < 5"), entry(3, 9, "b := y < 7")
	placeGone := entry(3, 9, "c := x < 5")
	rehashed := entry(3, 9, "a := x < 5")
	firstAgain := entry(6, 9, "a := x < 5")
	secondFar, gone := entry(6, 9, "b := y < 7"), entry(7, 9, "c := z < 1")
	otherColumn := entry(5, x9.Column+9, "return a && b && x < 9 && y < 9")
	holdsB := b
	holdsB.Reason = "kept"
	for _, c := range []struct {
		name     string
		entries  []mutants.Exception
		survivor mutants.Exception
		want     int
	}{
		{"the first moved, over the second at its place", []mutants.Exception{first, second}, a, 0},
		{"the second moved, none at its place", []mutants.Exception{first, second}, b, 1},
		{"the first moved, after the second", []mutants.Exception{second, first}, a, 1},
		{"one at the place that names it", []mutants.Exception{second, rehashed}, a, 1},
		{"one at the place when none names it", []mutants.Exception{secondFar, placeGone}, a, 1},
		{"several, none naming it", []mutants.Exception{secondFar, gone}, a, -1},
		{"two that name it", []mutants.Exception{first, firstAgain}, a, -1},
		{"the one stale, beside one that holds elsewhere", []mutants.Exception{holdsB, gone}, a, 1},
		{"one on its line at another column", []mutants.Exception{otherColumn, secondFar}, x9, -1},
	} {
		got, err := mutants.Repointed(t.Context(), c.entries, c.survivor, f.path, exceptionOptions(f))
		if err != nil || got != c.want {
			t.Errorf("%s: %d, %v, want %d", c.name, got, err, c.want)
		}
	}
}
