package mutate

import (
	"testing"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/lang"
)

// exceptionAt is the exception for the site of f's unit u at line and
// column with original, as mutation except writes it.
func exceptionAt(t *testing.T, f *lang.File, line, column int, original string) config.Exception {
	t.Helper()
	for _, s := range Sites(f) {
		if s.Line == line && s.Column == column && s.Original == original {
			u := f.Units[s.Unit]
			return config.Exception{File: "x.py", Function: unitID(u.Namespace, u.Name), Hash: UnitHash(f, u),
				LineInFunction: s.Line - u.StartLine + 1, Column: s.Column, Original: s.Original, Replacement: s.Replacement, Reason: "r"}
		}
	}
	t.Fatalf("no site %d:%d %s", line, column, original)
	return config.Exception{}
}

func TestAnExceptionIsFoundByItsFunctionAndPlace(t *testing.T) {
	before := parse(t, "x.py", "def a(x):\n    return x > 0\n\ndef b(x):\n    return x < 1\n")
	e := exceptionAt(t, before, 5, 14, "<")

	for _, c := range []struct {
		name, src string
		why       string
		line      int
	}{
		{"unchanged", "def a(x):\n    return x > 0\n\ndef b(x):\n    return x < 1\n", "", 5},
		{"moved", "import os\n\ndef a(x):\n    return x > 0\n\ndef b(x):\n    return x < 1\n", "", 7},
		{"changed", "def a(x):\n    return x > 0\n\ndef b(x):\n    return x < 2\n", ExceptionChanged, 5},
		{"site gone", "def a(x):\n    return x > 0\n\ndef b(x):\n    return x == 1\n", ExceptionGone, 5},
		{"site moved within", "def a(x):\n    return x > 0\n\ndef b(x):\n    y = 1\n    return x < 1\n", ExceptionGone, 5},
		{"function gone", "def a(x):\n    return x > 0\n", ExceptionGone, 0},
	} {
		f := parse(t, "x.py", c.src)
		sites := Sites(f)
		i, why, line := locate(e, f, sites)
		if why != c.why || line != c.line {
			t.Errorf("%s: why %q at line %d, want %q at line %d", c.name, why, line, c.why, c.line)
		}
		if why == "" && (i < 0 || sites[i].Original != "<" || sites[i].Line != c.line) {
			t.Errorf("%s: site %d, want the `<` of b", c.name, i)
		}
		if why != "" && i != -1 {
			t.Errorf("%s: site %d for a stale exception, want -1", c.name, i)
		}
	}
}

func TestAnExceptionCoversASurvivorOnly(t *testing.T) {
	f := parse(t, "x.py", "def b(x):\n    return x < 1\n")
	sites := Sites(f)
	e := exceptionAt(t, f, 2, 14, "<")
	x := applyExceptions([]config.Exception{e}, "x.py", f, sites, nil)
	i := -1
	for j := range sites {
		if _, ok := x.held[j]; ok {
			i = j
		}
	}
	if i < 0 || len(x.stale) != 0 {
		t.Fatalf("held %v, stale %v, want the exception held", x.held, x.stale)
	}
	if reason, stale := x.judge(i, sites[i], Survived); reason != "r" || stale != nil {
		t.Errorf("a survivor: reason %q, stale %v, want it excepted", reason, stale)
	}
	for _, outcome := range []string{Killed, Timeout} {
		if reason, stale := x.judge(i, sites[i], outcome); reason != "" || stale == nil || stale.Why != ExceptionKilled || stale.Line != 2 {
			t.Errorf("%s: reason %q, stale %+v, want the exception stale, killed", outcome, reason, stale)
		}
	}
	if reason, stale := x.judge(i, sites[i], Uncovered); reason != "" || stale != nil {
		t.Errorf("uncovered: reason %q, stale %v, want neither", reason, stale)
	}
	// Another file's exceptions, and those of functions not judged, do not apply.
	if x := applyExceptions([]config.Exception{e}, "y.py", f, sites, nil); len(x.held)+len(x.stale) != 0 {
		t.Errorf("another file: %+v, want nothing", x)
	}
	stale := e
	stale.Hash = "other"
	if x := applyExceptions([]config.Exception{stale}, "x.py", f, sites, func(string) bool { return false }); len(x.held)+len(x.stale) != 0 {
		t.Errorf("a function not judged: %+v, want nothing", x)
	}
}
