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

	"github.com/donvargax/itos-cc/mutate"
)

// The scenarios of "Rule: Judging the functions a range of commits changed"
// in features/mutate.feature. Each runs mutate in a small Go module with a
// git repository of its own: Board#place is tested, so its one mutant is
// killed, and Board#clear is not, so its one mutant survives whenever it
// runs. The commit tagged "base" holds the files as boardFiles writes them.

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
// working directory.
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
	t.Chdir(dir)
	return dir
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "core.autocrlf=false"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// edit replaces old with new in the working directory's file name.
func edit(t *testing.T, name, old, new string) {
	t.Helper()
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
	var o outcome
	o.stdout, o.stderr = captured(t, func() {
		o.code = run(append([]string{"mutation", "run", "--no-coverage", "--no-annotate", "--workers", "1"}, args...))
	})
	return o
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
		return func() string { w.Close(); return <-done }, func() { *target = saved }
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
	File      string    `json:"file"`
	Killed    int       `json:"killed"`
	Survived  int       `json:"survived"`
	Uncovered int       `json:"uncovered"`
	Ran       int       `json:"ran"`
	Baseline  string    `json:"baseline"`
	Judged    *[]string `json:"judged"`
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
func boardUnits(t *testing.T) map[string]mutate.UnitResult {
	t.Helper()
	snap, err := mutate.LoadSnapshot(boardSource)
	if err != nil || snap == nil {
		t.Fatalf("no snapshot of %s: %v", boardSource, err)
	}
	units := map[string]mutate.UnitResult{}
	for _, u := range snap.Units {
		units[u.Namespace+"#"+u.Name] = u
	}
	return units
}

func changePlace(t *testing.T) {
	t.Helper()
	edit(t, boardSource, "// past five\n", "// past five, and not at five\n")
	commitAll(t, "change place")
}

// @ID-MUT-37
func TestOnlyTheFunctionsTheCommitsSinceARefChangedAreJudged(t *testing.T) {
	boardRepo(t, nil)
	changePlace(t)

	o := mutateRun(t, "--since", "base")
	units := boardUnits(t)
	if u, ok := units[placeID]; !ok || u.Killed != 1 || len(u.Mutants) != 1 {
		t.Errorf("place in the snapshot: %+v, want its one mutant run and killed", u)
	}
	if u, ok := units[clearID]; ok {
		t.Errorf("clear in the snapshot: %+v, want no entry: it did not change, and never ran before", u)
	}
	line := ""
	for _, l := range strings.Split(o.stdout, "\n") {
		if strings.HasPrefix(l, boardSource+": ") {
			line = l
		}
	}
	if !strings.HasSuffix(line, " (judged 1 of 2 functions)") {
		t.Errorf("summary %q, want it to end \"(judged 1 of 2 functions)\"; stdout:\n%s", line, o.stdout)
	}
}

// @ID-MUT-38
func TestFunctionsNotJudgedKeepWhatTheirSnapshotHolds(t *testing.T) {
	boardRepo(t, nil)
	if o := mutateRun(t); o.code != 1 {
		t.Fatalf("the first run: exit %d, want 1 for clear's survivor\n%s%s", o.code, o.stdout, o.stderr)
	}
	changePlace(t)

	o := mutateRun(t, "--since", "base")
	u := boardUnits(t)[clearID]
	if u.Survived != 1 || len(u.Mutants) != 1 || u.Mutants[0].Outcome != mutate.Survived {
		t.Errorf("clear in the snapshot: %+v, want the survivor it recorded", u)
	}
	if strings.Contains(o.stdout, "survived "+boardSource) || o.code != 0 {
		t.Errorf("exit %d, stdout:\n%s\nwant exit 0 and clear's survivor not reported", o.code, o.stdout)
	}
}

// @ID-MUT-39
func TestUncommittedChangesAreNotInTheRange(t *testing.T) {
	boardRepo(t, nil)
	changePlace(t)
	edit(t, boardSource, "i < 3", "i < 4")

	m := mutateRun(t, "--json", "--since", "base").json(t)
	if j := m.judged(t, boardSource); j == nil || !slices.Equal(*j, []string{placeID}) {
		t.Errorf("judged %v, want only %s: clear's change is not committed", j, placeID)
	}
}

// @ID-MUT-40
func TestDeletingLinesChangesTheFunctionAroundThem(t *testing.T) {
	boardRepo(t, nil)
	edit(t, boardSource, "\t// past five\n", "")
	commitAll(t, "delete a comment in place")

	m := mutateRun(t, "--json", "--since", "base").json(t)
	if j := m.judged(t, boardSource); j == nil || !slices.Equal(*j, []string{placeID}) {
		t.Errorf("judged %v, want %s, whose line was deleted", j, placeID)
	}
}

// @ID-MUT-41
func TestPathsNarrowTheRange(t *testing.T) {
	boardRepo(t, map[string]string{
		"src/rest.go": "package board\n\nfunc rest(i int) bool {\n\treturn i == 1\n}\n",
		"lib/util.go": "package util\n\nfunc Half(i int) bool {\n\treturn i > 2\n}\n",
	})
	edit(t, boardSource, "// past five\n", "// past five, and not at five\n")
	edit(t, filepath.FromSlash("lib/util.go"), "i > 2", "i > 3")
	commitAll(t, "change place and Half")

	m := mutateRun(t, "--json", "--since", "base", "src").json(t)
	var files []string
	for _, f := range m.Files {
		files = append(files, f.File)
	}
	if !slices.Equal(files, []string{boardSource}) {
		t.Errorf("mutated %q, want only %s: src/rest.go did not change, lib/util.go is not under src", files, boardSource)
	}
}

// @ID-MUT-42
func TestNothingChangedSinceTheRef(t *testing.T) {
	boardRepo(t, nil)
	edit(t, "README.md", "# board\n", "# board\n\nA board.\n")
	commitAll(t, "describe the board")

	o := mutateRun(t, "--since", "base")
	if !strings.Contains(o.stderr, "itos-cc: no source files to mutate") || o.code != 0 {
		t.Errorf("exit %d, stderr:\n%s\nwant exit 0 and no source files to mutate", o.code, o.stderr)
	}
}

// @ID-MUT-43
func TestARefGitCannotResolveIsAUsageError(t *testing.T) {
	boardRepo(t, nil)

	o := mutateRun(t, "--since", "nosuch")
	if !strings.Contains(o.stderr, "itos-cc: --since nosuch: not a commit in this repository") || o.code != 2 {
		t.Errorf("exit %d, stderr:\n%s\nwant exit 2 and the ref named", o.code, o.stderr)
	}
	o = mutateRun(t, "--json", "--since", "nosuch")
	if p := o.json(t).problem("since.bad-ref"); p == nil || p["ref"] != "nosuch" || o.code != 2 {
		t.Errorf("exit %d, stdout:\n%s\nwant exit 2 and since.bad-ref with ref nosuch", o.code, o.stdout)
	}
}

// @ID-MUT-44
func TestSinceOutsideAGitRepositoryIsAMissingEnvironment(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	for name, text := range boardFiles {
		writeFile(t, filepath.Join(dir, name), text)
	}
	t.Chdir(dir)

	o := mutateRun(t, "--json", "--since", "main")
	if p := o.json(t).problem("since.no-git"); p == nil || o.code != 3 {
		t.Errorf("exit %d, stdout:\n%s\nwant exit 3 and since.no-git", o.code, o.stdout)
	}
}

// @ID-MUT-45
func TestSinceAndChangedAreNotCombined(t *testing.T) {
	boardRepo(t, nil)

	o := mutateRun(t, "--json", "--since", "main", "--changed")
	if p := o.json(t).problem("flags.conflict"); p == nil || p["flag"] != "--changed" || o.code != 2 {
		t.Errorf("exit %d, stdout:\n%s\nwant exit 2 and flags.conflict with flag --changed", o.code, o.stdout)
	}
}

// @ID-MUT-46
func TestTheFunctionsJudgedAsJSON(t *testing.T) {
	boardRepo(t, nil)
	changePlace(t)

	m := mutateRun(t, "--json", "--since", "base").json(t)
	if len(m.Files) == 0 {
		t.Fatalf("no files in %+v", m)
	}
	for _, f := range m.Files {
		if f.Judged == nil || !slices.Equal(*f.Judged, []string{placeID}) {
			t.Errorf("%s: judged %v, want [%s]", f.File, f.Judged, placeID)
		}
	}
	m = mutateRun(t, "--json").json(t)
	for _, f := range m.Files {
		if f.Judged != nil {
			t.Errorf("%s: judged %v without --since, want no key", f.File, *f.Judged)
		}
	}
}
