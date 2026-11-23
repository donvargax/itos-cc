package workdir

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/past"
	"github.com/donvargax/itos-cc/internal/project"
	"github.com/donvargax/itos-cc/internal/selection/port"
)

// committed lays files out as the working directory of a repository, commits
// them, and then changes the working tree, so the commit and the disk
// differ; it gives the commit's id and the directory.
func committed(t *testing.T, files map[string]string) (dir, commit string) {
	t.Helper()
	dir = workingDir(t, files)
	git(t, "init", "-q", ".")
	git(t, "add", "-A")
	git(t, "commit", "-q", "-m", "fixture")
	id := gitOutput(t, dir, "rev-parse", "HEAD")
	return dir, id
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := GitFor(dir).run(t.Context(), args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(out)
}

// run is git's output, for a test.
func (g Git) run(ctx context.Context, args ...string) (string, error) {
	cmd := project.GitIn(ctx, g.Dir, args...)
	out, err := cmd.Output()
	return string(out), err
}

// GitFor is the Git of dir.
func GitFor(dir string) Git { return Git{Dir: dir} }

// config is a settings reader of its own: the text "exclude: [x]" excludes x,
// "exclude: {a: b}" is invalid, and no text excludes nothing.
func config(root string, text []byte) port.SettingsReader {
	return reader{root: root, text: string(text)}
}

type reader struct{ root, text string }

func (r reader) ReadSelection(context.Context) (port.Settings, port.SettingsFailure) {
	switch {
	case strings.Contains(r.text, "{a: b}"):
		return port.Settings{}, &port.SettingsInvalid{Reason: "exclude must be a list", Key: "exclude"}
	case strings.HasPrefix(r.text, "exclude: ["):
		glob := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(r.text), "exclude: ["), "]")
		return port.Settings{Root: r.root, Exclude: []string{glob}}, nil
	}
	return port.Settings{Root: r.root}, nil
}

const alpha = "export function alpha(): number {\n  return 1;\n}\n"

// A commit is read from git's objects: its files, its itos-cc.yaml's
// exclude and each file's content are the commit's, not the disk's, which
// holds others by then.
func TestCommitsOpenAReadsTheTreeGitRecorded(t *testing.T) {
	dir, id := committed(t, map[string]string{"src/a.ts": alpha, "src/skip.ts": alpha, "itos-cc.yaml": "exclude: [src/skip.ts]\n", "src/a.test.ts": "test('x', () => {});\n"})
	if err := os.WriteFile(filepath.Join(dir, "src", "a.ts"), []byte("export function beta(): number {\n  return 2;\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	keeper := &memoryKeeper{}
	commits := Commits{Dir: dir, Objects: Git{Dir: dir}, Keeper: keeper, Settings: config}
	commit, err := commits.Open(t.Context(), "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if commit.ID != id || commit.Root != dir {
		t.Errorf("Open(HEAD) = %q at %q, want %q at %q", commit.ID, commit.Root, id, dir)
	}
	sources, tests, failure := commit.Tree.Discover(t.Context(), []string{"."})
	if failure != nil || !slices.Equal(sources, []string{filepath.Join(dir, "src", "a.ts"), filepath.Join(dir, "src", "skip.ts")}) || !slices.Equal(tests, []string{filepath.Join(dir, "src", "a.test.ts")}) {
		t.Fatalf("Discover = %q, %q, %v", sources, tests, failure)
	}
	text, failure := commit.Tree.Read(t.Context(), "src/a.ts")
	if failure != nil || string(text) != alpha {
		t.Errorf("Read(src/a.ts) = %q, %v; want the commit's text, not the disk's", text, failure)
	}
	if !slices.Equal(commit.Settings.Exclude, []string{"src/skip.ts"}) || commit.Settings.Root != dir {
		t.Errorf("the commit's own itos-cc.yaml should exclude skip.ts alone: %+v", commit.Settings)
	}
	if !commit.Tree.Exists(t.Context(), "src") || commit.Tree.Exists(t.Context(), "src/none.ts") {
		t.Errorf("Exists holds the commit's folders and files only")
	}
	if _, failure := commit.Tree.Read(t.Context(), "src/none.ts"); failure == nil {
		t.Errorf("Read of a file the commit lacks should fail")
	}
	if _, _, failure := commit.Tree.Discover(t.Context(), []string{"nosuch"}); failure == nil || !strings.Contains(failure.Error(), id) {
		t.Errorf("Discover of a missing root = %v, want Unwalkable naming the commit", failure)
	}
	if _, failure := commit.Tree.Changed(t.Context()); failure == nil {
		t.Errorf("Changed should be refused: a commit has no changes")
	}
	around := commit.Surroundings
	if wd, ok := around.WorkingDir(t.Context()); !ok || wd != dir {
		t.Errorf("WorkingDir = %q", wd)
	}
	if abs, ok := around.Abs(t.Context(), "src/a.ts"); !ok || abs != filepath.Join(dir, "src", "a.ts") {
		t.Errorf("Abs = %q", abs)
	}
	if !around.Exists(t.Context(), "src/a.ts") {
		t.Errorf("Exists(src/a.ts) = false")
	}
	if read, ok := around.Read(t.Context(), "src/a.ts"); !ok || string(read) != alpha {
		t.Errorf("Read = %q, %v", read, ok)
	}
	// The result is kept in the git common directory, under the commit.
	commit.Results.Put(t.Context(), commit.ID, "units", "k", []byte("x"))
	want := filepath.Join(dir, ".git", "itos-cc", "history", id, "units-k.json")
	if _, ok := keeper.files[want]; !ok {
		t.Errorf("kept %v, want %s", keys(keeper.files), want)
	}
	if got, ok := commit.Results.Get(t.Context(), commit.ID, "units", "k"); !ok || string(got) != "x" {
		t.Errorf("Get = %q, %v", got, ok)
	}
}

func keys(m map[string][]byte) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

type memoryKeeper struct{ files map[string][]byte }

func (k *memoryKeeper) Read(_ context.Context, path string) ([]byte, bool) {
	text, ok := k.files[path]
	return text, ok
}

func (k *memoryKeeper) Write(_ context.Context, path string, data []byte) error {
	if k.files == nil {
		k.files = map[string][]byte{}
	}
	k.files[path] = data
	return nil
}

// An itos-cc.yaml that cannot be read is the settings' own failure, a
// revision git cannot name BadRev, and a done context opens nothing.
func TestCommitsOpenFails(t *testing.T) {
	dir, _ := committed(t, map[string]string{"src/a.ts": alpha, "itos-cc.yaml": "exclude: {a: b}\n"})
	commits := Commits{Dir: dir, Objects: Git{Dir: dir}, Keeper: &memoryKeeper{}, Settings: config}
	var invalid port.SettingsFailure
	if _, err := commits.Open(t.Context(), "HEAD"); !errors.As(err, &invalid) {
		t.Errorf("an invalid itos-cc.yaml: %v, want the settings' failure", err)
	}
	var bad *past.BadRev
	if _, err := commits.Open(t.Context(), "nosuch"); !errors.As(err, &bad) || bad.Rev != "nosuch" {
		t.Errorf("an unknown revision: %v, want BadRev", err)
	}
	if _, err := commits.Open(t.Context(), "--output=x"); !errors.As(err, &bad) {
		t.Errorf("a revision that is a flag: %v, want BadRev", err)
	}
	done, cancel := context.WithCancelCause(t.Context())
	cancel(errors.New("stop"))
	if _, err := commits.Open(done, "HEAD"); err == nil || err.Error() != "stop" {
		t.Errorf("a done context: %v, want its cause", err)
	}
}

// Outside a repository there is no commit to open.
func TestCommitsOpenWithoutARepository(t *testing.T) {
	dir := workingDir(t, map[string]string{"a.ts": alpha})
	commits := Commits{Dir: dir, Objects: Git{Dir: dir}, Keeper: &memoryKeeper{}, Settings: config}
	var none *past.NoRepository
	if _, err := commits.Open(t.Context(), "HEAD"); !errors.As(err, &none) {
		t.Errorf("no repository: %v, want NoRepository", err)
	}
	if _, err := (Git{Dir: dir}).CommonDir(t.Context()); !errors.As(err, &none) {
		t.Errorf("CommonDir without a repository: %v, want NoRepository", err)
	}
}

// A linked worktree and the main one share one git common directory, and
// ls-tree names only regular files, never a link.
func TestGitCommonDirIsSharedByLinkedWorktrees(t *testing.T) {
	dir, _ := committed(t, map[string]string{"a.ts": alpha})
	linked := filepath.Join(filepath.Dir(dir), filepath.Base(dir)+"-linked")
	git(t, "worktree", "add", "-q", "--detach", linked, "HEAD")
	main, err := Git{Dir: dir}.CommonDir(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	other, err := Git{Dir: linked}.CommonDir(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	same, _ := filepath.EvalSymlinks(main)
	again, _ := filepath.EvalSymlinks(other)
	if same != again {
		t.Errorf("common dirs %q and %q differ", main, other)
	}
}

// Blobs of ids git has not are an error, and none asked is none read.
func TestGitBlobs(t *testing.T) {
	dir, _ := committed(t, map[string]string{"a.ts": alpha})
	g := Git{Dir: dir}
	if got, err := g.Blobs(t.Context(), nil); err != nil || len(got) != 0 {
		t.Errorf("Blobs(nil) = %v, %v", got, err)
	}
	if _, err := g.Blobs(t.Context(), []string{"0000000000000000000000000000000000000000"}); err == nil {
		t.Errorf("Blobs of a missing id should fail")
	}
	entries, err := g.Entries(t.Context(), "HEAD")
	if err != nil || len(entries) != 1 || entries[0].Path != "a.ts" || entries[0].Size != int64(len(alpha)) {
		t.Fatalf("Entries = %+v, %v", entries, err)
	}
	got, err := g.Blobs(t.Context(), []string{entries[0].ID})
	if err != nil || string(got[entries[0].ID]) != alpha {
		t.Errorf("Blobs = %q, %v", got, err)
	}
	if _, err := g.Entries(t.Context(), "nosuch"); err == nil {
		t.Errorf("Entries of a missing commit should fail")
	}
}

// Disk writes a file whole, making its folders, and replaces one there; a
// done context writes nothing.
func TestDiskWritesWholeFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "x.json")
	var disk Disk
	if err := disk.Write(t.Context(), path, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := disk.Write(t.Context(), path, []byte("two")); err != nil {
		t.Fatal(err)
	}
	if got, ok := disk.Read(t.Context(), path); !ok || string(got) != "two" {
		t.Errorf("Read = %q, %v; want two", got, ok)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("the folder holds %d files, want the one: no staging file left", len(entries))
	}
	if _, ok := disk.Read(t.Context(), filepath.Join(dir, "none")); ok {
		t.Errorf("Read of a missing file should say so")
	}
	if err := disk.Write(t.Context(), filepath.Join(path, "under-a-file"), []byte("x")); err == nil {
		t.Errorf("a folder that cannot be made should fail the write")
	}
	done, cancel := context.WithCancelCause(t.Context())
	cancel(errors.New("stop"))
	if err := disk.Write(done, filepath.Join(dir, "late"), []byte("x")); err == nil {
		t.Errorf("a done context should write nothing")
	}
	if _, ok := disk.Read(done, path); ok {
		t.Errorf("a done context should read nothing")
	}
}
