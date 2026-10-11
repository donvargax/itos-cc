package mutate

import (
	"testing"

	"github.com/donvargax/itos-cc/config"
)

func TestARenewalMatchesTheMutantOnItsLineOrAtItsPlace(t *testing.T) {
	t.Parallel()
	// The function starts at line 4 now; the entry was written when its
	// site was at line_in_function 3, column 7, on "if n > 0 {".
	src := []byte("package p\n\n// F.\nfunc F(n int) int {\n\tname := 1\n\tif n > 0 {\n\t\treturn n\n\t}\n\tif n > 0 { // again\n\t\treturn 2\n\t}\n\treturn name\n}\n")
	entry := config.Exception{LineInFunction: 3, Column: 7, Original: ">", Replacement: ">=", LineText: "if n > 0 {"}
	legacy := entry
	legacy.LineText = ""
	at := func(line, column int) Mutant {
		return Mutant{Line: line, Column: column, Original: ">", Replacement: ">=", Outcome: Survived}
	}
	for _, c := range []struct {
		name       string
		e          config.Exception
		start      int
		mutants    []Mutant
		line       int
		match, why string
	}{
		{"on its line, at its place", entry, 4, []Mutant{at(6, 7), at(9, 7)}, 6, MatchLine, ""},
		// A line added above moved it: its text finds it.
		{"moved down a line", entry, 3, []Mutant{at(6, 7), at(9, 7)}, 6, MatchLine, ""},
		{"its line changed", config.Exception{LineInFunction: 3, Column: 7, Original: ">", Replacement: ">=", LineText: "if n > 1 {"}, 4,
			[]Mutant{at(6, 7), at(9, 7)}, 0, "", ExceptionChanged},
		{"two sites on lines of its text, neither at its place", entry, 1, []Mutant{at(6, 7), {Line: 6, Column: 9, Original: ">", Replacement: ">="}}, 0, "", RenewAmbiguous},
		{"no site of its change", entry, 4, []Mutant{{Line: 6, Column: 7, Original: ">", Replacement: "<"}}, 0, "", ExceptionGone},
		{"legacy, at its place", legacy, 4, []Mutant{at(6, 7), at(9, 7)}, 6, MatchPlace, ""},
		{"legacy, the only site of its change", legacy, 2, []Mutant{at(9, 7)}, 9, MatchOnly, ""},
		{"legacy, moved, with two sites of its change", legacy, 2, []Mutant{at(6, 7), at(9, 7)}, 0, "", RenewAmbiguous},
	} {
		m, match, why := matchMutant(c.e, c.start, c.mutants, src)
		if why != c.why || match != c.match || (why == "" && m.Line != c.line) {
			t.Errorf("%s: line %d, match %q, why %q; want line %d, match %q, why %q", c.name, m.Line, match, why, c.line, c.match, c.why)
		}
	}
	for outcome, want := range map[string]string{Survived: "", Killed: ExceptionKilled, Timeout: ExceptionKilled, Uncovered: RenewUncovered, "": RenewNoResults} {
		if got := survivedWhy(Mutant{Outcome: outcome}); got != want {
			t.Errorf("a mutant recorded %q: %q, want %q", outcome, got, want)
		}
	}
	if got := LineText([]byte("a\n\t  b c  \r\nd"), 2); got != "b c" {
		t.Errorf("LineText = %q, want %q", got, "b c")
	}
	if withoutSpace("if n>0 {") != withoutSpace("if n > 0 {") {
		t.Error("a reformatted line reads as the line it was")
	}
}
