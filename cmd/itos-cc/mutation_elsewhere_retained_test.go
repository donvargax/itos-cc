package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/internal/lang"
	mutation "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/workdir"
)

func TestAnEntryWhoseFileIsGoneHasMovedToTheOneFileHoldingItsFunction(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	opt := mutation.CheckOptions{Root: dir, Dir: dir, Tree: workdir.Tree{}, Surroundings: workdir.Surroundings{}}
	const b = "def b(x):\n    return x < 1\n"
	e := retainedExceptionAt(t, parseScopeFixture(t, "x.py", b), 2, 14, "<")
	write := func(name, src string) string {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		return filepath.Join(dir, name)
	}
	one := write("one.py", "def a(x):\n    return x > 0\n\n"+b)
	two := write("two.py", b)
	changed := write("changed.py", "def b(x):\n    return x < 2\n")
	all := func(string) bool { return true }
	nothing := func(string) bool { return false }

	for _, c := range []struct {
		name    string
		sources []string
		why     string
	}{
		{"in one file", []string{one, changed}, mutation.ExceptionMoved},
		{"in two files", []string{one, two}, mutation.ExceptionGone},
		{"changed", []string{changed}, mutation.ExceptionGone},
	} {
		stale, moved, err := mutation.Elsewhere(t.Context(), []config.Exception{e}, c.sources, all, nothing, opt)
		if err != nil {
			t.Fatal(err)
		}
		if len(stale) != 1 || stale[0].Why != c.why || stale[0].Line != 0 {
			t.Errorf("%s: stale %+v, want one, %s, with no line", c.name, stale, c.why)
			continue
		}
		if c.why != mutation.ExceptionMoved {
			if len(moved) != 0 || stale[0].NewFile != "" {
				t.Errorf("%s: moved %+v, new file %q, want none", c.name, moved, stale[0].NewFile)
			}
			continue
		}
		// In Python a function's namespace is its file's module, so the
		// entry reads there with one.py's.
		if stale[0].NewFile != "one.py" || len(moved) != 1 || moved[0].File != "one.py" || moved[0].Hash != e.Hash ||
			!strings.HasSuffix(moved[0].Function, "#b") || moved[0].Function == e.Function {
			t.Errorf("%s: new file %q, moved %+v, want one.py and the entry as it reads there", c.name, stale[0].NewFile, moved)
		}
	}

	// An entry for a file that is still selected, or one judge leaves out,
	// is not judged here.
	if stale, _, _ := mutation.Elsewhere(t.Context(), []config.Exception{e}, []string{write("x.py", b)}, all, nothing, opt); len(stale) != 0 {
		t.Errorf("an entry of a selected file: %+v, want none", stale)
	}
	if stale, _, _ := mutation.Elsewhere(t.Context(), []config.Exception{e}, []string{one}, nothing, all, opt); len(stale) != 0 {
		t.Errorf("an entry judge leaves out: %+v, want none", stale)
	}

	// An entry for a file itos-cc.yaml excludes is stale as excluded, even
	// where its function is in one of sources, and never moves.
	stale, moved, err := mutation.Elsewhere(t.Context(), []config.Exception{e}, []string{one}, all, func(file string) bool { return file == e.File }, opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 1 || stale[0].Why != mutation.ExceptionExcluded || stale[0].Line != 0 || stale[0].NewFile != "" || len(moved) != 0 {
		t.Errorf("an entry of an excluded file: stale %+v, moved %+v, want one, excluded, with no line, moved nowhere", stale, moved)
	}
}

func retainedExceptionAt(t *testing.T, f *lang.File, line, column int, original string) config.Exception {
	t.Helper()
	for _, s := range mutation.Sites(f) {
		if s.Line == line && s.Column == column && s.Original == original {
			u := f.Units[s.Unit]
			return config.Exception{File: "x.py", Function: mutation.UnitID(u.Namespace, u.Name), Hash: mutation.UnitHash(f, u),
				LineInFunction: s.Line - u.StartLine + 1, Column: s.Column, Original: s.Original, Replacement: s.Replacement, Reason: "r"}
		}
	}
	t.Fatalf("no site %d:%d %s", line, column, original)
	return config.Exception{}
}
