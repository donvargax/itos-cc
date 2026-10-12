package porttest

import (
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/lang/port"
)

// SurroundingsContract holds a Surroundings to what every one, the fake
// and the disk's, must do: what is there, what a file holds, the working
// directory and a path made absolute, a relative path always from the
// working directory. open lays out files, by slash-separated paths
// relative to a directory with their text, and returns the Surroundings
// over them and that directory, absolute, the working directory; it may
// make that the test's, so no case runs in parallel. No case has a done
// context: the port says no answer given after it matters. What only a
// disk can do (symlinks, permissions, a working directory that is gone) is
// the adapters' own tests'; a working directory the fake cannot tell, the
// fake's.
func SurroundingsContract(t *testing.T, open func(t *testing.T, files map[string]string) (port.Surroundings, string)) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, open func(t *testing.T, files map[string]string) (port.Surroundings, string))
	}{
		{"a file and the directories above it are there", existsWhereAFileIs},
		{"read gives a file's bytes and nothing for an absent path or a directory", readsAFile},
		{"the working directory is the directory", hasTheWorkingDirectory},
		{"abs joins a relative path to the directory and cleans an absolute one", makesPathsAbsolute},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { c.run(t, open) })
	}
}

// module is a directory holding a Go module below it and a file of its
// own.
var module = map[string]string{
	"mod/go.mod":   "module example.com/m\n",
	"mod/pkg/a.go": "package pkg\n",
	"notes.txt":    "notes\n",
}

func existsWhereAFileIs(t *testing.T, open func(t *testing.T, files map[string]string) (port.Surroundings, string)) {
	around, dir := open(t, module)
	for path, want := range map[string]bool{
		"mod/go.mod": true, filepath.Join("mod", "go.mod"): true, filepath.Join(dir, "mod", "go.mod"): true,
		"notes.txt": true, "mod/pkg": true, "mod": true, ".": true, dir: true, filepath.Dir(dir): true,
		"absent": false, "mod/absent.go": false, filepath.Join(dir, "absent"): false, "mod/go.mod/x": false, "mo": false,
	} {
		if got := around.Exists(t.Context(), path); got != want {
			t.Errorf("Exists(%q) = %t, want %t", path, got, want)
		}
	}
}

func readsAFile(t *testing.T, open func(t *testing.T, files map[string]string) (port.Surroundings, string)) {
	around, dir := open(t, module)
	for _, path := range []string{"mod/go.mod", filepath.Join("mod", "go.mod"), filepath.Join(dir, "mod", "go.mod"), "mod/pkg/../go.mod"} {
		if text, ok := around.Read(t.Context(), path); !ok || string(text) != module["mod/go.mod"] {
			t.Errorf("Read(%q) = %q, %t; want what it holds", path, text, ok)
		}
	}
	for _, path := range []string{"absent", filepath.Join(dir, "mod", "absent.go"), "mod", dir, "mod/go.mod/x"} {
		if text, ok := around.Read(t.Context(), path); ok || text != nil {
			t.Errorf("Read(%q) = %q, %t; want no answer", path, text, ok)
		}
	}
}

func hasTheWorkingDirectory(t *testing.T, open func(t *testing.T, files map[string]string) (port.Surroundings, string)) {
	around, dir := open(t, module)
	if got, ok := around.WorkingDir(t.Context()); !ok || got != dir {
		t.Errorf("WorkingDir() = %q, %t; want %q", got, ok, dir)
	}
}

func makesPathsAbsolute(t *testing.T, open func(t *testing.T, files map[string]string) (port.Surroundings, string)) {
	around, dir := open(t, module)
	sep := string(filepath.Separator)
	for path, want := range map[string]string{
		"x.ts":            filepath.Join(dir, "x.ts"),
		"mod/pkg/../b.go": filepath.Join(dir, "mod", "b.go"),
		".":               dir,
		"../up.ts":        filepath.Join(filepath.Dir(dir), "up.ts"),
		dir + sep + "mod" + sep + "." + sep + "z.ts": filepath.Join(dir, "mod", "z.ts"),
		dir + sep + "mod" + sep + sep + "y.ts":       filepath.Join(dir, "mod", "y.ts"),
		filepath.Join(dir, "mod", "pkg", "a.go"):     filepath.Join(dir, "mod", "pkg", "a.go"),
	} {
		if got, ok := around.Abs(t.Context(), path); !ok || got != want {
			t.Errorf("Abs(%q) = %q, %t; want %q", path, got, ok, want)
		}
	}
}
