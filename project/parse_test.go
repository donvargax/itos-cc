package project

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// unitsIn describes the units of a parsed file as "kind namespace#name
// lines [private]".
func unitsIn(t *testing.T, path string) []string {
	t.Helper()
	f, err := ParseFile(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []string
	for _, u := range f.Units {
		s := fmt.Sprintf("%s %s#%s %d-%d", u.Kind, u.Namespace, u.Name, u.StartLine, u.EndLine)
		if u.Private {
			s += " private"
		}
		out = append(out, s)
	}
	return out
}

// A Go file read from disk is named by the go.mod above it.
func TestParseFileNamesAGoFileByItsGoMod(t *testing.T) {
	got := unitsIn(t, filepath.Join("testdata", "gomod", "board", "board.go"))
	want := []string{
		"function example.com/demo/board#New 5-7",
		"method example.com/demo/board.Board#Place 9-12",
		"method example.com/demo/board.Board#size 14-14 private",
		"function example.com/demo/board#helper 16-16 private",
	}
	if !slices.Equal(got, want) {
		t.Errorf("units:\n got  %q\n want %q", got, want)
	}
}

// A language lang does not know is refused before anything is read, and a
// file that cannot be read fails with the read's own error.
func TestParseFileRefusesBeforeItReads(t *testing.T) {
	if _, err := ParseFile(t.Context(), "notes.txt"); err == nil || err.Error() != "unsupported language: notes.txt" {
		t.Errorf("ParseFile(notes.txt) = %v, want unsupported language before any read", err)
	}
	absent := filepath.Join(t.TempDir(), "absent.go")
	_, err := ParseFile(t.Context(), absent)
	if _, readErr := os.ReadFile(absent); err == nil || err.Error() != readErr.Error() {
		t.Errorf("ParseFile(absent) = %v, want %v", err, readErr)
	}
}

// FindUp finds the nearest directory up holding a marker on disk.
func TestFindUpFindsTheNearestMarkedDirectory(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "settings.gradle"), "")
	write(t, filepath.Join(dir, "app", "build.gradle"), "")
	source := filepath.Join(dir, "app", "src", "Main.kt")
	write(t, source, "")
	if got := FindUp(t.Context(), source, "build.gradle", "settings.gradle"); got != filepath.Join(dir, "app") {
		t.Errorf("FindUp(build.gradle) = %q, want %q", got, filepath.Join(dir, "app"))
	}
	if got := FindUp(t.Context(), source, "settings.gradle"); got != dir {
		t.Errorf("FindUp(settings.gradle) = %q, want %q", got, dir)
	}
	if got := FindUp(t.Context(), source, "no-directory-holds-this.marker"); got != "" {
		t.Errorf("FindUp(a marker nowhere) = %q, want none", got)
	}
}

// Outside any root a TypeScript file read from disk is named by its path
// from the working directory, so the name never depends on where the
// checkout lives; under a root the working directory plays no part.
func TestParseFileNamesFromTheWorkingDirectoryOutsideAnyRoot(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	wd := filepath.Join(base, "work")
	if root := FindUp(t.Context(), filepath.Join(wd, "x.ts"), "package.json", "tsconfig.json"); root != "" {
		t.Skipf("%s holds a project marker", root)
	}
	for _, name := range []string{"work/x.ts", "work/src/demo/board.ts", "other/y.ts"} {
		write(t, filepath.Join(base, filepath.FromSlash(name)), "export function f() {}\n")
	}
	t.Chdir(wd)
	for path, want := range map[string]string{
		"x.ts":                                  "x",
		filepath.Join(wd, "x.ts"):               "x",
		filepath.FromSlash("src/demo/board.ts"): "demo.board",
		filepath.FromSlash("../other/y.ts"):     "other.y",
		filepath.Join(base, "other", "y.ts"):    "other.y",
	} {
		f, err := ParseFile(t.Context(), path)
		if err != nil {
			t.Fatal(err)
		}
		if f.Namespace != want {
			t.Errorf("ParseFile(%q) namespace %q, want %q", path, f.Namespace, want)
		}
		f.Close()
	}

	write(t, filepath.Join(base, "package.json"), "{}\n")
	f, err := ParseFile(t.Context(), filepath.FromSlash("src/demo/board.ts"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if f.Namespace != "work.src.demo.board" {
		t.Errorf("under package.json: %q, want work.src.demo.board", f.Namespace)
	}
}
