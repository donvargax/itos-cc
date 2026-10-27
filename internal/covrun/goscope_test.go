package covrun

import (
	"path/filepath"
	"slices"
	"testing"
)

// A package of no dependencies that go list prints last, as it does where
// no package has tests, is read as any other (bug-11): its line ends in the
// tab before its empty dependencies, and on Windows in \r\n besides, as
// go.bat's echo writes it. ID-COV-51 holds it with a real go, but only the
// binary driver runs it, so a go of the test's own lists it here.
func TestGoScopeReadsAPackageOfNoDependenciesListedLast(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	fake := t.TempDir()
	writeFiles(t, fake, map[string]string{
		"go":     "#!/bin/sh\nprintf 'example.com/m/a\\t%s\\t\\nexample.com/m/b\\t%s\\t\\n' '" + a + "' '" + b + "'\n",
		"go.bat": "@echo off\r\necho example.com/m/a\t" + a + "\t\r\necho example.com/m/b\t" + b + "\t\r\n",
	})
	t.Setenv("PATH", fake)
	want := []string{"example.com/m/b"}
	if pkgs, tests, why := GoScope(dir, []string{filepath.Join(b, "b.go")}); !slices.Equal(pkgs, want) || !slices.Equal(tests, want) || why != "" {
		t.Errorf("GoScope: %q tested by %q (%q), want %q by %q", pkgs, tests, why, want, want)
	}
}
