package lang

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/lang/port"
	"github.com/donvargax/itos-cc/internal/lang/port/porttest"
)

// These tests hold how a file is named from what surrounds it: the go.mod
// above a Go file, the markers above a TypeScript or Python one, the working
// directory where there is none, and a context that is done. Each asks
// porttest's fake, never the disk.

// boardGo is a Go source under testdata/gomod's go.mod, in a package of
// its own.
const boardGo = `package board

type Board[T any] struct{ cells []T }

func New() *Board[int] {
	return &Board[int]{}
}

func (b *Board[T]) Place(v T) {
	add := func() { b.cells = append(b.cells, v) }
	add()
}

func (b Board[T]) size() int { return len(b.cells) }

func helper() {}
`

func TestGo(t *testing.T) {
	assertUnits(t, unitsOf(t, "testdata/gomod/board/board.go", boardGo), []string{
		"function example.com/demo/board#New 5-7",
		"method example.com/demo/board.Board#Place 9-12",
		"method example.com/demo/board.Board#size 14-14 private",
		"function example.com/demo/board#helper 16-16 private",
	})
}

// A Go file is named by the nearest go.mod with a module line, joined with
// the file's directory below it, and by its package clause where none is
// found or none can be read.
func TestGoNamespaceIsTheNearestModuleLine(t *testing.T) {
	src := "package board\n\nfunc New() {}\n"
	long := strings.Repeat("x", 70000) + "\nmodule example.com/hidden\n"
	for _, c := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"at the module's root", map[string]string{"/m/go.mod": "module example.com/m\n"}, "example.com/m/a/board"},
		{"a module line after others, quoted", map[string]string{"/m/a/go.mod": "// the module\n\n  module \"example.com/q\"  \n"}, "example.com/q/board"},
		{"past a go.mod with no module line", map[string]string{"/m/a/go.mod": "go 1.22\n", "/m/go.mod": "module example.com/m\n"}, "example.com/m/a/board"},
		{"in the file's own directory", map[string]string{"/m/a/board/go.mod": "module example.com/b\n", "/m/go.mod": "module example.com/m\n"}, "example.com/b"},
		// The scanner stops at a line too long for it, as it always has.
		{"behind a line too long to scan", map[string]string{"/m/go.mod": long}, "board"},
		{"with no go.mod", nil, "board"},
	} {
		s := porttest.Surroundings{Dir: "/m", Files: c.files}
		f, err := Parse(t.Context(), s, Detect("x.go"), filepath.FromSlash("a/board/board.go"), []byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if f.Namespace != c.want {
			t.Errorf("%s: namespace %q, want %q", c.name, f.Namespace, c.want)
		}
		f.Close()
	}
	// No working directory, so no directory to walk up from: the package
	// clause.
	f, err := Parse(t.Context(), porttest.Surroundings{Files: map[string]string{"/m/go.mod": "module example.com/m\n"}}, Detect("x.go"), "board.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if f.Namespace != "board" {
		t.Errorf("with no working directory: namespace %q, want board", f.Namespace)
	}
}

// UnitsOf finds a file's units in the source its caller read, reading none
// itself: what it is given, not what is on disk, decides them.
func TestUnitsOfParsesTheSourceItIsGiven(t *testing.T) {
	units, err := UnitsOf(t.Context(), around, "src/billing/total.py", []byte("def total(xs):\n    return sum(xs)\n\n\ndef _round(x):\n    return x\n"))
	if err != nil {
		t.Fatal(err)
	}
	assertUnits(t, describe(units), []string{
		"function billing.total#total 1-2",
		"function billing.total#_round 5-6 private",
	})
	for _, u := range units {
		if u.Node != nil {
			t.Errorf("%s keeps its node", u.Name)
		}
	}
	if _, err := UnitsOf(t.Context(), around, "notes.txt", nil); err == nil || err.Error() != "unsupported language: notes.txt" {
		t.Errorf("UnitsOf(notes.txt) = %v, want unsupported language", err)
	}
}

// cancelling cancels its context the first time it is asked to make a
// path absolute, as an interrupt arriving while a file is named would.
type cancelling struct {
	porttest.Surroundings
	cancel context.CancelFunc
}

func (c cancelling) Abs(ctx context.Context, path string) (string, bool) {
	c.cancel()
	return c.Surroundings.Abs(ctx, path)
}

var _ port.Surroundings = cancelling{}

// Once the context is done, Parse and UnitsOf return its error and no
// file, whether it was done before they began or while the file was named.
func TestADoneContextParsesNothing(t *testing.T) {
	done, cancel := context.WithCancel(t.Context())
	cancel()
	if f, err := Parse(done, around, Detect("x.py"), "x.py", []byte("def f():\n    pass\n")); f != nil || !errors.Is(err, context.Canceled) {
		t.Errorf("Parse(done) = %v, %v; want no file and context.Canceled", f, err)
	}
	if units, err := UnitsOf(done, around, "x.py", []byte("def f():\n    pass\n")); units != nil || !errors.Is(err, context.Canceled) {
		t.Errorf("UnitsOf(done) = %v, %v; want no units and context.Canceled", units, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if f, err := Parse(ctx, cancelling{around, cancel}, Detect("x.ts"), "x.ts", []byte("function f() {}\n")); f != nil || !errors.Is(err, context.Canceled) {
		t.Errorf("Parse(cancelled while naming) = %v, %v; want no file and context.Canceled", f, err)
	}
}

func TestModulePathOutsideAnyRootIsFromTheWorkingDirectory(t *testing.T) {
	s := porttest.Surroundings{Dir: "/base/work"}
	for path, want := range map[string]string{
		"x.ts":                       "x",
		"/base/work/x.ts":            "x",
		"src/demo/board.ts":          "demo.board",
		"../other/y.ts":              "other.y",
		"/base/other/y.ts":           "other.y",
		"lib/util/strings.ts":        "util.strings",
		"./src/demo/board.ts":        "demo.board",
		"/base/work/src/lib/deep.ts": "deep", // src/ and then lib/ come off
	} {
		if got := modulePath(t.Context(), s, filepath.FromSlash(path), "package.json", "tsconfig.json"); got != want {
			t.Errorf("modulePath(%q) = %q, want %q", path, got, want)
		}
	}

	// Under a root the working directory plays no part.
	s.Files = map[string]string{"/base/package.json": "{}\n"}
	if got := modulePath(t.Context(), s, filepath.FromSlash("/base/work/src/x.ts"), "package.json"); got != "work.src.x" {
		t.Errorf("under package.json: %q, want work.src.x", got)
	}
}

// A working directory that cannot be told reads as a failed os.Getwd once
// did: a relative path cannot be made absolute, so it names the module as
// written, and an absolute one outside any root is named by its file name.
func TestModulePathWithoutAWorkingDirectory(t *testing.T) {
	s := porttest.Surroundings{}
	for path, want := range map[string]string{
		"src/demo/board.ts": "demo.board",
		"../other/y.ts":     "other.y",
		"/base/other/y.ts":  "y",
	} {
		if got := modulePath(t.Context(), s, filepath.FromSlash(path), "package.json"); got != want {
			t.Errorf("modulePath(%q) = %q, want %q", path, got, want)
		}
	}
}

// Nearest walks up from a path's directory, its own first, to the first
// directory holding any of the markers, and finds nothing past the top.
func TestNearestIsTheFirstMarkedDirectoryUp(t *testing.T) {
	s := porttest.Surroundings{Dir: "/w", Files: map[string]string{
		"/w/settings.gradle": "", "/w/app/build.gradle": "", "/w/app/src/Main.kt": "",
	}}
	for path, want := range map[string]string{
		"app/src/Main.kt":  "/w/app",
		"app/build.gradle": "/w/app",
		"other/x.kt":       "/w",
	} {
		if got := Nearest(t.Context(), s, filepath.FromSlash(path), "build.gradle", "settings.gradle"); got != filepath.FromSlash(want) {
			t.Errorf("Nearest(%q) = %q, want %q", path, got, want)
		}
	}
	if got := Nearest(t.Context(), s, filepath.FromSlash("app/src/Main.kt"), "pom.xml"); got != "" {
		t.Errorf("Nearest(no marker) = %q, want none", got)
	}
	if got := Nearest(t.Context(), porttest.Surroundings{}, "x.kt", "build.gradle"); got != "" {
		t.Errorf("Nearest(no working directory) = %q, want none", got)
	}
}
