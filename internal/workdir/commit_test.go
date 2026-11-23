package workdir

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"maps"
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
// done context writes nothing, and a write that cannot be made leaves no
// staging file behind.
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
	// A folder where the file belongs cannot be replaced by it.
	folder := filepath.Join(dir, "folder")
	if err := os.MkdirAll(folder, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := disk.Write(t.Context(), folder, []byte("x")); err == nil {
		t.Errorf("a file cannot replace a folder")
	}
	if _, err := os.Stat(fmt.Sprintf("%s.staging-%d", folder, os.Getpid())); err == nil {
		t.Errorf("a failed write left its staging file")
	}
	// A folder where the staging file belongs cannot be written to.
	blocked := filepath.Join(dir, "blocked")
	if err := os.MkdirAll(fmt.Sprintf("%s.staging-%d", blocked, os.Getpid()), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := disk.Write(t.Context(), blocked, []byte("x")); err == nil {
		t.Errorf("a staging file that cannot be written should fail the write")
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

// What cat-file --batch says is read as it says it: a blob, whole; anything
// else, a size that is none, or a content cut short, is an error naming the id.
func TestReadBlobTakesWhatBatchSays(t *testing.T) {
	into := map[string][]byte{}
	r := bufio.NewReader(strings.NewReader("a1 blob 3\nabc\nb2 missing\nc3 blob x\nd4 blob 9\nshort"))
	if err := readBlob(r, "a1", into); err != nil || string(into["a1"]) != "abc" {
		t.Errorf("a blob: %q, %v", into["a1"], err)
	}
	for _, id := range []string{"b2", "c3", "d4"} {
		if err := readBlob(r, id, into); err == nil || !strings.Contains(err.Error(), id) {
			t.Errorf("%s: %v, want an error naming it", id, err)
		}
	}
}

// fake is the objects of a commit held in memory, each call able to fail.
type fake struct {
	top   string
	files map[string]string
	fail  map[string]error
	// cancel is called when Blobs is, to end the run it is asked in.
	cancel func()
	asked  [][]string
}

func (f *fake) Top(context.Context) (string, error)       { return f.top, f.fail["top"] }
func (f *fake) CommonDir(context.Context) (string, error) { return f.top + "/.git", f.fail["common"] }
func (f *fake) Resolve(context.Context, string) (string, error) {
	return "c0ffee", f.fail["resolve"]
}

func (f *fake) Entries(context.Context, string) ([]Entry, error) {
	var out []Entry
	for _, name := range slices.Sorted(maps.Keys(f.files)) {
		out = append(out, Entry{Path: name, ID: "id-" + name, Size: int64(len(f.files[name]))})
	}
	return out, f.fail["entries"]
}

func (f *fake) Blobs(_ context.Context, ids []string) (map[string][]byte, error) {
	f.asked = append(f.asked, ids)
	if f.cancel != nil {
		f.cancel()
	}
	if err := f.fail["blobs"]; err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, id := range ids {
		if text, ok := f.files[strings.TrimPrefix(id, "id-")]; ok && !strings.HasPrefix(text, "<missing>") {
			out[id] = []byte(text)
		}
	}
	return out, nil
}

func opened(t *testing.T, f *fake) (past.Commit, error) {
	t.Helper()
	return Commits{Dir: f.top, Objects: f, Keeper: &memoryKeeper{}, Settings: config}.Open(t.Context(), "HEAD")
}

// Each failure of the objects ends Open as the objects gave it, except the
// contents of itos-cc.yaml, which are read apart: a commit whose files cannot
// be loaded is opened, and fails where they are read.
func TestOpenEndsWhereTheObjectsFail(t *testing.T) {
	base := map[string]string{"a.ts": alpha}
	for _, call := range []string{"resolve", "top", "common", "entries"} {
		boom := errors.New(call + " failed")
		if _, err := opened(t, &fake{top: "/p", files: base, fail: map[string]error{call: boom}}); !errors.Is(err, boom) {
			t.Errorf("%s failing: %v, want it returned", call, err)
		}
	}
	commit, err := opened(t, &fake{top: "/p", files: base, fail: map[string]error{"blobs": errors.New("no objects")}})
	if err != nil {
		t.Fatalf("a commit whose contents cannot be loaded still opens: %v", err)
	}
	if _, _, failure := commit.Tree.Discover(t.Context(), []string{"."}); failure == nil {
		t.Errorf("Discover should fail where the contents cannot be loaded")
	}
}

// A cancellation that ends the loading is a cancellation, not an unwalkable
// tree; a file outside the loaded set is asked for alone and kept; a file git
// holds nothing for is unreadable, as is one whose objects fail.
func TestTreeReadsWhatTheObjectsGive(t *testing.T) {
	files := map[string]string{"a.ts": alpha, "notes.txt": "hello", "gone.txt": "<missing>"}
	f := &fake{top: t.TempDir(), files: files}
	commit, err := opened(t, f)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.top, "notes.txt")
	for range 2 {
		if text, failure := commit.Tree.Read(t.Context(), path); failure != nil || string(text) != "hello" {
			t.Errorf("Read(notes.txt) = %q, %v", text, failure)
		}
	}
	asked := 0
	for _, ids := range f.asked {
		if slices.Contains(ids, "id-notes.txt") {
			asked++
		}
	}
	if asked != 1 {
		t.Errorf("notes.txt was asked of git %d times, want once", asked)
	}
	if _, failure := commit.Tree.Read(t.Context(), path+".none"); failure == nil {
		t.Errorf("a file the commit lacks is unreadable")
	}
	if _, failure := commit.Tree.Read(t.Context(), filepath.Join(f.top, "gone.txt")); failure == nil {
		t.Errorf("a file git holds no content for is unreadable")
	}
	f2 := &fake{top: f.top, files: files, fail: map[string]error{}}
	commit2, _ := opened(t, f2)
	if _, failure := commit2.Tree.Read(t.Context(), filepath.Join(f.top, "a.ts")); failure != nil {
		t.Fatalf("Read(a.ts) = %v", failure)
	}
	f2.fail["blobs"] = errors.New("no objects")
	if _, failure := commit2.Tree.Read(t.Context(), filepath.Join(f.top, "notes.txt")); failure == nil {
		t.Errorf("a file asked of objects that fail is unreadable")
	}
	ctx, cancel := context.WithCancelCause(t.Context())
	f3 := &fake{top: f.top, files: files, fail: map[string]error{}}
	commit3, _ := opened(t, f3)
	f3.cancel, f3.fail["blobs"] = func() { cancel(errors.New("stop")) }, errors.New("cut off")
	f3.files["late.txt"] = "x"
	if _, failure := commit3.Tree.Read(ctx, filepath.Join(f.top, "notes.txt")); failure == nil {
		t.Errorf("a read cut off by a cancellation fails")
	}
	ctx2, cancel2 := context.WithCancelCause(t.Context())
	f4 := &fake{top: f.top, files: files, fail: map[string]error{}}
	commit4 := Commits{Dir: f.top, Objects: f4, Keeper: &memoryKeeper{}, Settings: config}
	opened4, err := commit4.Open(t.Context(), "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	// Loading happens at the first read of the read set: end it there.
	fresh := newCommitTree(f.top, f.top, "c0ffee", mustEntries(t, f4), f4)
	f4.cancel, f4.fail["blobs"] = func() { cancel2(errors.New("stop")) }, errors.New("cut off")
	if _, _, failure := fresh.Discover(ctx2, []string{"."}); failure == nil || !strings.Contains(failure.Error(), "cancelled") {
		t.Errorf("a discovery cut off by a cancellation = %v, want Cancelled", failure)
	}
	if _, _, failure := opened4.Tree.Discover(ctx2, []string{"."}); failure == nil {
		t.Errorf("a done context discovers nothing")
	}
	if _, failure := opened4.Tree.Read(ctx2, path); failure == nil {
		t.Errorf("a done context reads nothing")
	}
}

func mustEntries(t *testing.T, o Objects) []Entry {
	t.Helper()
	entries, err := o.Entries(t.Context(), "c0ffee")
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

// The commit's folders and files answer as a disk's do: names in order, a
// file's size and kind, a folder's, what is not there not found, and a
// file's content only where it is loaded.
func TestCommitFilesAnswerAsADiskDoes(t *testing.T) {
	top := filepath.FromSlash("/p")
	f := newCommitFS(top, "c0ffee", []Entry{{Path: "src/a.ts", ID: "1", Size: 4}, {Path: "src/b.ts", ID: "2", Size: 5}, {Path: "z.txt", ID: "3", Size: 1}}, &fake{})
	entries, err := f.ReadDir(top)
	if err != nil || len(entries) != 2 || entries[0].Name() != "src" || !entries[0].IsDir() || entries[1].Name() != "z.txt" {
		t.Errorf("ReadDir(top) = %v, %v", entries, err)
	}
	if _, err := f.ReadDir(filepath.Join(top, "none")); err == nil {
		t.Errorf("ReadDir of a missing folder should fail")
	}
	info, err := f.Lstat(filepath.Join(top, "src", "a.ts"))
	if err != nil || info.Size() != 4 || info.IsDir() || info.Mode().IsDir() || !info.ModTime().IsZero() || info.Sys() != nil || info.Name() != "a.ts" {
		t.Errorf("Lstat(a.ts) = %+v, %v", info, err)
	}
	if dir, err := f.Stat(filepath.Join(top, "src")); err != nil || !dir.IsDir() || !dir.Mode().IsDir() {
		t.Errorf("Stat(src) = %+v, %v", dir, err)
	}
	if _, err := f.Stat(filepath.Join(top, "none")); err == nil {
		t.Errorf("Stat of a missing file should fail")
	}
	if _, err := f.ReadFile(filepath.Join(top, "none")); err == nil {
		t.Errorf("ReadFile of a missing file should fail")
	}
	if _, err := f.ReadFile(filepath.Join(top, "z.txt")); err == nil {
		t.Errorf("ReadFile of a file not loaded should fail")
	}
	f.content["3"] = []byte("z")
	if text, err := f.ReadFile(filepath.Join(top, "z.txt")); err != nil || string(text) != "z" {
		t.Errorf("ReadFile of a loaded file = %q, %v", text, err)
	}
}

// git ls-tree lists links and submodules, which are no files of the commit,
// beside its files.
func TestEntriesLeaveOutLinksAndSubmodules(t *testing.T) {
	dir, _ := committed(t, map[string]string{"a.ts": alpha})
	blob := gitOutput(t, dir, "hash-object", "-w", "--stdin")
	_ = blob
	git(t, "update-index", "--add", "--cacheinfo", "120000,"+gitOutput(t, dir, "rev-parse", "HEAD:a.ts")+",link")
	git(t, "update-index", "--add", "--cacheinfo", "160000,"+gitOutput(t, dir, "rev-parse", "HEAD")+",sub")
	git(t, "commit", "-q", "-m", "links")
	entries, err := Git{Dir: dir}.Entries(t.Context(), "HEAD")
	if err != nil || len(entries) != 1 || entries[0].Path != "a.ts" {
		t.Errorf("Entries = %+v, %v; want a.ts alone", entries, err)
	}
}

// The git failures are told apart: a done context is its cause, a revision
// git cannot name BadRev, and no repository NoRepository.
func TestGitFailuresAreTold(t *testing.T) {
	dir, _ := committed(t, map[string]string{"a.ts": alpha})
	done, cancel := context.WithCancelCause(t.Context())
	cancel(errors.New("stop"))
	g := Git{Dir: dir}
	if _, err := g.Resolve(done, "HEAD"); err == nil || err.Error() != "stop" {
		t.Errorf("Resolve with a done context: %v", err)
	}
	if _, err := g.Blobs(done, []string{"x"}); err == nil || err.Error() != "stop" {
		t.Errorf("Blobs with a done context: %v", err)
	}
	if _, err := (Git{Dir: filepath.Join(dir, "none")}).Blobs(t.Context(), []string{"x"}); err == nil {
		t.Errorf("Blobs outside a folder should fail")
	}
	if _, err := g.Blobs(t.Context(), []string{"0000000000000000000000000000000000000000"}); err == nil {
		t.Errorf("Blobs of an id git lacks should fail")
	}
	top, err := g.Top(t.Context())
	if err != nil || top != dir {
		t.Errorf("Top = %q, %v; want %q", top, err, dir)
	}
}
