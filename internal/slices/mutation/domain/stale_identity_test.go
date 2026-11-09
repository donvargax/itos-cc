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
