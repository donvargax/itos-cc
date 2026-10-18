package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/metricsdir"
	mutation "github.com/donvargax/itos-cc/internal/mutants"
)

// The module the mutation tests of cmd share: a small Go module with a git
// repository of its own, where Board#place is tested, so its one mutant is
// killed, and Board#clear is not, so its one mutant survives whenever it
// runs. The commit tagged "base" holds the files as boardFiles writes them.
// The scenarios of "Rule: Judging the functions a range of commits changed"
// in features/mutate.feature hold what this file's tests held (T-101), all
// but the guard below, which no scenario reaches: a renamed file is narrowed
// by its new path.

const (
	placeID = "example.com/m/src.Board#place"
	clearID = "example.com/m/src.Board#clear"
)

var boardFiles = map[string]string{
	"go.mod": "module example.com/m\n\ngo 1.22\n",
	"src/board.go": `package board

type Board struct{}

func (b *Board) place(i int) bool {
	// past five
	return i > 5
}

func (b *Board) clear(i int) bool {
	return i < 3
}
`,
	"src/board_test.go": `package board

import "testing"

func TestPlace(t *testing.T) {
	var b Board
	if b.place(5) || !b.place(6) {
		t.Fatal("place")
	}
}
`,
	"README.md": "# board\n",
}

var boardSource = filepath.FromSlash("src/board.go")

// boardRepo makes the module in a git repository whose first commit is
// tagged "base", with extra files added to boardFiles, and makes it the
// test's directory (useDir).
func boardRepo(t *testing.T, extra map[string]string) string {
	t.Helper()
	for _, tool := range []string{"git", "go"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " is not installed")
		}
	}
	dir := t.TempDir()
	for name, text := range boardFiles {
		writeFile(t, filepath.Join(dir, name), text)
	}
	for name, text := range extra {
		writeFile(t, filepath.Join(dir, name), text)
	}
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "base")
	gitIn(t, dir, "tag", "base")
	useDir(t, dir)
	return dir
}

// The one mutant of Board#clear: `<` → `<=` at src/board.go:11:11.
const clearLine, clearColumn = 11, 11

// mutateCovered runs itos-cc mutation run with args and coverage measured, no
// source file annotated, and returns what it printed.
func mutateCovered(t *testing.T, args ...string) outcome {
	t.Helper()
	return itosCc(t, append([]string{"mutation", "run", "--no-annotate", "--workers", "1"}, args...)...)
}

// gitIn runs git with args in dir, a relative one in the test's directory.
// A repository it initialises never runs git's automatic maintenance or
// gc (noMaintenance), whichever git runs in it later.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "core.autocrlf=false"}, args...)...)
	cmd.Dir = inWD(t, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	if len(args) > 0 && args[0] == "init" {
		for _, setting := range noMaintenance {
			gitIn(t, dir, append([]string{"config"}, setting...)...)
		}
	}
}

// noMaintenance is the config that keeps git from maintaining a fixture
// repository in the background: its lock files come and go while a test
// copies or reads the tree.
var noMaintenance = [][]string{{"maintenance.auto", "false"}, {"gc.auto", "0"}}

// writeFile writes text to path, a relative one in the test's directory.
func writeFile(t *testing.T, path, text string) {
	t.Helper()
	path = inWD(t, path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// edit replaces old with new in the file name, a relative one in the
// test's directory.
func edit(t *testing.T, name, old, new string) {
	t.Helper()
	name = inWD(t, name)
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), old) {
		t.Fatalf("%s holds no %q", name, old)
	}
	writeFile(t, name, strings.Replace(string(data), old, new, 1))
}

// commitAll commits every change to a tracked file, so the snapshots under
// .metrics stay out of the range.
func commitAll(t *testing.T, message string) {
	t.Helper()
	gitIn(t, ".", "commit", "-qam", message)
}

type outcome struct {
	code           int
	stdout, stderr string
}

// mutateRun runs itos-cc mutation run with args, every mutant run whatever
// coverage says and no source file annotated, and returns what it printed.
func mutateRun(t *testing.T, args ...string) outcome {
	t.Helper()
	return itosCc(t, append([]string{"mutation", "run", "--no-coverage", "--no-annotate", "--workers", "1"}, args...)...)
}

// captured runs fn and returns what it printed on stdout and on stderr.
func captured(t *testing.T, fn func()) (string, string) {
	t.Helper()
	read := func(target **os.File) (func() string, func()) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		saved := *target
		*target = w
		done := make(chan string)
		go func() {
			out, _ := io.ReadAll(r)
			done <- string(out)
		}()
		return func() string {
			if err := w.Close(); err != nil {
				t.Error(err)
			}
			return <-done
		}, func() { *target = saved }
	}
	stdout, restoreOut := read(&os.Stdout)
	defer restoreOut()
	stderr, restoreErr := read(&os.Stderr)
	defer restoreErr()
	fn()
	return stdout(), stderr()
}

type mutateJSON struct {
	OK       bool             `json:"ok"`
	Files    []fileJSON       `json:"files"`
	Problems []map[string]any `json:"problems"`
}

type fileJSON struct {
	File      string `json:"file"`
	Killed    int    `json:"killed"`
	Survived  int    `json:"survived"`
	Uncovered int    `json:"uncovered"`
	// Excepted is nil when the key is missing.
	Excepted *int      `json:"excepted"`
	Ran      int       `json:"ran"`
	Baseline string    `json:"baseline"`
	Judged   *[]string `json:"judged"`
	// Mutants is nil when the key is missing or null.
	Mutants *[]mutantJSON `json:"mutants"`
}

func (o outcome) json(t *testing.T) mutateJSON {
	t.Helper()
	var out mutateJSON
	if err := json.Unmarshal([]byte(o.stdout), &out); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s\nstderr:\n%s", err, o.stdout, o.stderr)
	}
	return out
}

// problem is the first problem with rule, or nil.
func (m mutateJSON) problem(rule string) map[string]any {
	for _, p := range m.Problems {
		if p["rule"] == rule {
			return p
		}
	}
	return nil
}

// judged is the "judged" of file in the --json output, or nil when it has
// none.
func (m mutateJSON) judged(t *testing.T, file string) *[]string {
	t.Helper()
	return m.file(t, file).Judged
}

// file is the entry of file in "files".
func (m mutateJSON) file(t *testing.T, file string) fileJSON {
	t.Helper()
	for _, f := range m.Files {
		if f.File == file {
			return f
		}
	}
	t.Fatalf("no file %s in %+v", file, m.Files)
	return fileJSON{}
}

// boardUnits is the snapshot of src/board.go, by namespace#name.
func boardUnits(t *testing.T) map[string]mutation.UnitResult {
	t.Helper()
	snap, err := loadMutationSnapshot(t.Context(), metricsdir.Store{RootDir: wd(t)}, boardSource)
	if err != nil || snap == nil {
		t.Fatalf("no snapshot of %s: %v", boardSource, err)
	}
	units := map[string]mutation.UnitResult{}
	for _, u := range snap.Units {
		units[u.Namespace+"#"+u.Name] = u
	}
	return units
}

// changePlace commits a change to place's code that leaves what its
// mutants do: a change to its comments alone would be no change of it.
func changePlace(t *testing.T) {
	t.Helper()
	edit(t, boardSource, "return i > 5", "return (i > 5)")
	commitAll(t, "change place")
}

var gridSource = filepath.FromSlash("src/grid.go")

// Paths narrow the range by the path a renamed file has now: one renamed
// out from under the paths given is not selected, one renamed into them is.
func TestARenamedFileIsNarrowedByItsNewPath(t *testing.T) {
	t.Parallel()
	boardRepo(t, nil)
	moved := filepath.FromSlash("lib/grid.go")
	if err := os.MkdirAll(inWD(t, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, ".", "mv", filepath.ToSlash(boardSource), filepath.ToSlash(moved))
	gitIn(t, ".", "commit", "-qm", "move board.go to lib/grid.go")

	files := func(path string) []string {
		t.Helper()
		var names []string
		for name := range mutationCheck(t, "--json", "--since", "base", path).checkFunctions(t) {
			names = append(names, name)
		}
		return names
	}
	if got := files("src"); len(got) != 0 {
		t.Errorf("--since base src checks %q, want none: the file is now under lib", got)
	}
	if got := files("lib"); !slices.Equal(got, []string{moved}) {
		t.Errorf("--since base lib checks %q, want [%s]", got, moved)
	}
}
