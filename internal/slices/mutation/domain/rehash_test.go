package domain_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/donvargax/itos-cc/internal/lang"
	selected "github.com/donvargax/itos-cc/internal/selection"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// rehashSource holds Count, whose comment line its hash before 0.18.0
// kept, and Over, with no comment.
const rehashSource = "package board\n\nfunc Count(x int) bool {\n\t// below five counts\n\treturn x < 5\n}\n\n" +
	"func Over(x int) bool {\n\treturn x > 10\n}\n"

// rehashParsed is board.go of f parsed, with its sites, and the unit of
// each function by name.
func rehashParsed(t *testing.T, f *runFixture) (*lang.File, []mutants.Site, map[string]int) {
	t.Helper()
	parsed, err := selected.Parse(t.Context(), f.options.Tree, f.inputs, f.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(parsed.Close)
	units := map[string]int{}
	for i, u := range parsed.Units {
		units[u.Name] = i
	}
	return parsed, mutants.Sites(parsed), units
}

// The hash before 0.18.0 keeps a function's comments, as b214f151's parent
// took it; today's leaves them out. The literals are those of slice-68's
// scenarios, measured with sha256sum.
func TestPreviousUnitHashKeepsComments(t *testing.T) {
	f := newRunFixtureOf(t, rehashSource)
	parsed, _, units := rehashParsed(t, f)
	count, over := parsed.Units[units["Count"]], parsed.Units[units["Over"]]
	if got := mutants.PreviousUnitHash(parsed, count); got != "af13bea3308d9ebf" {
		t.Errorf("Count's previous hash %s, want af13bea3308d9ebf", got)
	}
	if got := mutants.UnitHash(parsed, count); got != "b1a8f41702cd975f" {
		t.Errorf("Count's hash %s, want b1a8f41702cd975f", got)
	}
	if mutants.PreviousUnitHash(parsed, over) != mutants.UnitHash(parsed, over) {
		t.Error("a function with no comment hashes differently under the two")
	}
}

// slice-68: --rehash carries over an entry its function's previous hash
// and its site at its place match, and leaves every other stale entry as
// Locate judges it, or gone when its file is no source.
func TestRehashCarriesOverOnlyWhatThePreviousHashMatches(t *testing.T) {
	f := newRunFixtureOf(t, rehashSource)
	f.files[filepath.Join(f.root, "notes.txt")] = "notes"
	f.edits.Directories = map[string]bool{filepath.Join(f.root, "pkg.go"): true}
	parsed, _, units := rehashParsed(t, f)
	id := func(name string) string {
		u := parsed.Units[units[name]]
		return mutants.UnitID(u.Namespace, u.Name)
	}
	count := mutants.Exception{File: "board.go", Function: id("Count"), Hash: "af13bea3308d9ebf", LineInFunction: 3, Column: 11,
		Original: "<", Replacement: "<=", Reason: "x is never 5"}
	over := mutants.Exception{File: "board.go", Function: id("Over"), Hash: mutants.UnitHash(parsed, parsed.Units[units["Over"]]),
		LineInFunction: 2, Column: 11, Original: ">", Replacement: ">=", Reason: "x is never 10"}
	with := func(e mutants.Exception, change func(*mutants.Exception)) mutants.Exception {
		change(&e)
		return e
	}
	entries := []mutants.Exception{
		count,
		over,
		with(count, func(e *mutants.Exception) { e.Hash = "0000000000000000" }),
		with(count, func(e *mutants.Exception) { e.Column = 12 }),
		with(count, func(e *mutants.Exception) { e.LineInFunction = 2 }),
		with(count, func(e *mutants.Exception) { e.Original = ">" }),
		with(count, func(e *mutants.Exception) { e.Replacement = ">" }),
		with(count, func(e *mutants.Exception) { e.Function = "board#Below" }),
		with(count, func(e *mutants.Exception) { e.File = "gone.go" }),
		with(count, func(e *mutants.Exception) { e.File = "notes.txt" }),
		with(count, func(e *mutants.Exception) { e.File = "pkg.go" }),
	}
	rehashed, left, err := mutants.Rehash(t.Context(), entries, exceptionOptions(f), f.edits)
	if err != nil {
		t.Fatal(err)
	}
	want := []mutants.Renewal{{Old: count, New: with(count, func(e *mutants.Exception) { e.Hash = "b1a8f41702cd975f" }), Line: 5}}
	if !reflect.DeepEqual(rehashed, want) {
		t.Errorf("rehashed %+v, want %+v", rehashed, want)
	}
	var whys []string
	for _, s := range left {
		whys = append(whys, s.Why+"@"+strconv.Itoa(s.Line))
	}
	wantWhys := []string{"changed@5", "changed@5", "changed@4", "gone@5", "gone@5", "gone@0", "gone@0", "gone@0", "gone@0"}
	if !reflect.DeepEqual(whys, wantWhys) {
		t.Errorf("left %v, want %v", whys, wantWhys)
	}
	if got, _, _ := mutants.Rehash(t.Context(), nil, mutants.CheckOptions{}, nil); got != nil {
		t.Errorf("no entries rehashed %+v", got)
	}
	if _, _, err := mutants.Rehash(t.Context(), entries, exceptionOptions(f), nil); err == nil {
		t.Error("no status binding accepted")
	}
	denied := errors.New("stat denied")
	f.edits.StatErrors = map[string]error{f.path: denied}
	if _, _, err := mutants.Rehash(t.Context(), entries, exceptionOptions(f), f.edits); !errors.Is(err, denied) {
		t.Errorf("status failure: %v", err)
	}
	f.edits.StatErrors = nil
	if _, _, err := mutants.Rehash(t.Context(), entries, mutants.CheckOptions{Root: f.root}, f.edits); err == nil {
		t.Error("no parser accepted")
	}
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("stopped")
	cancel(cause)
	if _, _, err := mutants.Rehash(ctx, entries, exceptionOptions(f), f.edits); !errors.Is(err, cause) {
		t.Errorf("cancelled: %v", err)
	}
}

// run and check say an entry the previous hash matches is rehashable;
// another stale entry is not.
func TestApplyExceptionsSaysWhichStaleEntryIsRehashable(t *testing.T) {
	f := newRunFixtureOf(t, rehashSource)
	parsed, sites, units := rehashParsed(t, f)
	u := parsed.Units[units["Count"]]
	count := mutants.Exception{File: "board.go", Function: mutants.UnitID(u.Namespace, u.Name), Hash: "af13bea3308d9ebf", LineInFunction: 3, Column: 11, Original: "<", Replacement: "<="}
	other := count
	other.Hash = "0000000000000000"
	got := mutants.ApplyExceptions([]mutants.Exception{count, other}, "board.go", parsed, sites, nil).Stale
	if len(got) != 2 || !got[0].Rehashable || got[1].Rehashable || got[0].Why != mutants.ExceptionChanged {
		t.Errorf("stale %+v, want the first alone rehashable, both changed", got)
	}
}

// --renew says an entry left before its outcome was read is rehashable
// when the previous hash matches its function in its own file, and in no
// other file it found the function in.
func TestRenewSaysWhichLeftEntryIsRehashable(t *testing.T) {
	commented := "package board\nfunc Value(x int) bool {\n\t// one\n\treturn x > 1\n}\n"
	f := newRunFixtureOf(t, commented)
	parsed, sites, _ := rehashParsed(t, f)
	u, s := parsed.Units[0], sites[0]
	entry := mutants.Exception{File: "board.go", Function: mutants.UnitID(u.Namespace, u.Name), Hash: mutants.PreviousUnitHash(parsed, u),
		LineInFunction: s.Line - u.StartLine + 1, Column: s.Column, Original: s.Original, Replacement: s.Replacement}
	_, left, err := mutants.Renew(t.Context(), []mutants.Exception{entry}, []string{f.path}, exceptionOptions(f), f.edits)
	if err != nil || len(left) != 1 || left[0].Why != mutants.RenewNoResults || !left[0].Rehashable {
		t.Fatalf("no results: %+v %v", left, err)
	}
	// Elsewhere: a function with no comment, in a file the entry does not name.
	g := newRunFixture(t)
	parsed, sites, _ = rehashParsed(t, g)
	u, s = parsed.Units[0], sites[0]
	moved := mutants.Exception{File: "old.go", Function: mutants.UnitID(u.Namespace, u.Name), Hash: mutants.UnitHash(parsed, u),
		LineInFunction: s.Line - u.StartLine + 1, Column: s.Column, Original: s.Original, Replacement: s.Replacement}
	_, left, err = mutants.Renew(t.Context(), []mutants.Exception{moved}, []string{g.path}, exceptionOptions(g), g.edits)
	if err != nil || len(left) != 1 || left[0].Why != mutants.RenewNoResults || left[0].Rehashable {
		t.Fatalf("moved: %+v %v", left, err)
	}
	// With fresh results, an entry whose line text changed is left changed.
	f, _ = survivingFixtureOf(t, commented)
	entry.LineText = "return x >= 1"
	_, left, err = mutants.Renew(t.Context(), []mutants.Exception{entry}, []string{f.path}, exceptionOptions(f), f.edits)
	if err != nil || len(left) != 1 || left[0].Why != mutants.ExceptionChanged || !left[0].Rehashable {
		t.Fatalf("changed: %+v %v", left, err)
	}
}
