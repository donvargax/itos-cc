package worktrees

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// copySettings is itos-cc.yaml as a pool reads it when it opens: the
// project root and mutation.copy's globs, or failure.
type copySettings struct {
	root    string
	globs   []string
	failure mutants.SettingsFailure
}

func (s copySettings) Read(context.Context) (*mutants.Config, mutants.SettingsFailure) {
	if s.failure != nil {
		return nil, s.failure
	}
	c := &mutants.Config{Copy: s.globs}
	c.Selection.Root = s.root
	return c, nil
}

// gitIn runs git with args in dir, failing the test when it fails.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...) //nolint:gosec // git, run on the test's own fixture
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// writeTree writes each file, by its slash-separated path, in root.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, text := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// ignoredPool opens a pool whose settings are settings, closed when the
// test ends.
func ignoredPool(t *testing.T, settings mutants.SettingsReader) *pool {
	t.Helper()
	opened, failure := (Workers{Platform: runtime.GOOS, Settings: settings}).Open(t.Context(), mutants.MutationRun, 1)
	if failure != nil {
		t.Fatal(failure)
	}
	p := opened.(*pool)
	t.Cleanup(func() {
		if f := p.Close(t.Context()); f != nil {
			t.Error(f)
		}
	})
	return p
}

// ignoredRun runs c's baseline on p's worker, failing the test unless it
// passes, and returns the worker's copy of tree.
func ignoredRun(t *testing.T, p *pool, c mutants.Command, tree string) string {
	t.Helper()
	if r, failure := p.workers[0].Run(t.Context(), c, nil, mutants.Limits{}); failure != nil || !r.Passed {
		t.Fatalf("run: %+v %v", r, failure)
	}
	return p.workers[0].worker.copies[tree].dir
}

// holds checks that the copy at dir holds each path, slash-separated, the
// map says it holds, and none it says it does not.
func holds(t *testing.T, dir string, want map[string]bool) {
	t.Helper()
	for name, held := range want {
		if _, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(name))); (err == nil) != held {
			t.Errorf("the copy holds %s: %v, want %v", name, err == nil, held)
		}
	}
}

// ignoredCommand is the helper's success mode run in root, whose Ready
// writes .idea/workspace.xml, a folder every copy leaves out.
func ignoredCommand(t *testing.T, root string) mutants.Command {
	t.Setenv(runnerFixtureMode, "ignored")
	args := commandRunnerHelperArgs()
	return mutants.Command{Root: root, Dir: root, Args: args, Ready: append(args[:len(args):len(args)], "-test.short")}
}

// TestACopyLeavesOutWhatGitIgnores holds slice-43's decisions 1, 3, 4, 5
// and 10 on every system's go test with git: a copy leaves out the
// untracked files the committed .gitignore files ignore but keeps a
// tracked file in an ignored folder, brings back a folder mutation.copy
// names whole and a file it names in a build-output folder, both put back
// before the next run while what a run wrote beside the file stays, links
// the ignored node_modules, and copies no folder git does not track that
// holds its own .git, a folder or a file, but the files of one
// mutation.copy names; a submodule's files are copied, never its .git.
// The run in the project names what the copy left out, a folder git names
// with what it holds (logs/ and logs/a.log) once.
func TestACopyLeavesOutWhatGitIgnores(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeTree(t, root, map[string]string{
		"package.json": "{}\n", "source.txt": "source\n",
		".gitignore":          "testdata/\ngen/\ntmp/\nnode_modules/\n*.log\n",
		"testdata/keep.txt":   "kept\n",
		"node_modules/dep.js": "dep\n",
	})
	gitIn(t, root, "init", "-q")
	gitIn(t, root, "add", ".gitignore", "package.json", "source.txt")
	gitIn(t, root, "add", "-f", "testdata/keep.txt")
	gitIn(t, root, "commit", "-qm", "project")
	source := t.TempDir()
	writeTree(t, source, map[string]string{"data.txt": "data\n"})
	gitIn(t, source, "init", "-q")
	gitIn(t, source, "add", "data.txt")
	gitIn(t, source, "commit", "-qm", "data")
	gitIn(t, root, "-c", "protocol.file.allow=always", "submodule", "add", "-q", source, "sub")
	gitIn(t, root, "commit", "-qm", "submodule")
	writeTree(t, root, map[string]string{
		"testdata/left-behind": "left\n", "gen/table.txt": "table\n", "gen/sub/more.txt": "more\n", "tmp/stray.log": "stray\n", "logs/a.log": "a\n",
		"dist/types/index.d.ts": "types\n", "dist/out.js": "out\n", "dist/chunks/a.js": "a\n", "dist/node_modules/x.js": "x\n",
		"coverage/lcov.info": "lcov\n", "nested/marker.txt": "nested\n", "linked/marker.txt": "linked\n", "linked/.git": "gitdir: elsewhere\n",
		"brought/x.txt": "x\n",
	})
	gitIn(t, filepath.Join(root, "nested"), "init", "-q")
	gitIn(t, filepath.Join(root, "brought"), "init", "-q")
	if err := os.Symlink("source.txt", filepath.Join(root, "alias.txt")); err != nil {
		t.Fatal(err)
	}
	p := ignoredPool(t, copySettings{root: root, globs: []string{"gen", "dist/types/index.d.ts", "brought"}})
	c := ignoredCommand(t, root)
	copied := ignoredRun(t, p, c, root)
	holds(t, copied, map[string]bool{
		"source.txt": true, "alias.txt": true, "testdata/keep.txt": true, "testdata/left-behind": false,
		"gen/table.txt": true, "gen/sub/more.txt": true, "tmp": false,
		"dist/types/index.d.ts": true, "dist/out.js": false, "dist/chunks": false, "dist/node_modules": false, "coverage": false,
		"nested": false, "linked": false, ".idea/workspace.xml": true,
		"sub/data.txt": true, "sub/.git": false, "brought/x.txt": true, "brought/.git": false,
	})
	if link, err := os.Readlink(filepath.Join(copied, "node_modules")); err != nil || link != filepath.Join(root, "node_modules") {
		t.Errorf("the copy's node_modules links %q, %v, not the project's", link, err)
	}
	// What a run leaves: a changed file mutation.copy brought, output
	// beside it in the build-output folder, and an ignored folder anew.
	later := time.Now().Add(time.Hour)
	for name, text := range map[string]string{"dist/types/index.d.ts": "changed!\n", "gen/table.txt": "changed!\n", "dist/run.js": "run\n", "tmp/new.log": "new\n", "coverage/run.txt": "run\n"} {
		path := filepath.Join(copied, filepath.FromSlash(name))
		writeTree(t, copied, map[string]string{name: text})
		if err := os.Chtimes(path, later, later); err != nil {
			t.Fatal(err)
		}
	}
	ignoredRun(t, p, c, root)
	for name, want := range map[string]string{"dist/types/index.d.ts": "types\n", "gen/table.txt": "table\n", "dist/run.js": "run\n", "coverage/run.txt": "run\n"} {
		if got, err := os.ReadFile(filepath.Join(copied, filepath.FromSlash(name))); err != nil || string(got) != want { //nolint:gosec // a fixture's own file
			t.Errorf("after the restore the copy's %s holds %q, %v, not %q", name, got, err, want)
		}
	}
	holds(t, copied, map[string]bool{"tmp": false})
	run, failure := p.InProject(t.Context(), c)
	if failure != nil || !run.Result.Passed {
		t.Fatalf("the run in the project: %+v %v", run, failure)
	}
	if want := []string{"linked/", "logs/", "nested/", "testdata/left-behind", "tmp/"}; !slices.Equal(run.LeftOut, want) {
		t.Errorf("the copy left out %q, not %q", run.LeftOut, want)
	}
}

// TestACopyKeepsGeneratedDartGitIgnores holds decision 8: a Dart
// package's copy keeps the generated Dart git ignores, recognised as
// discovery recognises it, even in an ignored folder, and leaves out the
// rest of what git ignores.
func TestACopyKeepsGeneratedDartGitIgnores(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"pubspec.yaml": "name: app\n", "l10n.yaml": "arb-dir: lib/l10n\n", "lib/app.dart": "void main() {}\n",
		".gitignore": "*.g.dart\nlib/l10n/\nbuild/\n",
	})
	gitIn(t, root, "init", "-q")
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-qm", "app")
	writeTree(t, root, map[string]string{
		"lib/user.g.dart": "// generated\n", "lib/l10n/app_localizations.dart": "// generated\n",
		"lib/l10n/notes.txt": "notes\n", "lib/l10n/sub/x.txt": "x\n", "build/out.txt": "out\n",
	})
	p := ignoredPool(t, nil)
	copied := ignoredRun(t, p, ignoredCommand(t, root), root)
	holds(t, copied, map[string]bool{
		"lib/app.dart": true, "lib/user.g.dart": true, "lib/l10n/app_localizations.dart": true,
		"lib/l10n/notes.txt": false, "lib/l10n/sub": false, "build": false,
	})
}

// TestATreeGitCannotDescribeIsCopiedAsBefore holds decision 6: a tree in
// no git repository, or one git ignores whole, is copied whole, and the
// run in the project names nothing left out.
func TestATreeGitCannotDescribeIsCopiedAsBefore(t *testing.T) {
	alone := t.TempDir()
	writeTree(t, alone, map[string]string{".gitignore": "gen/\n", "gen/table.txt": "table\n"})
	repository := t.TempDir()
	writeTree(t, repository, map[string]string{".gitignore": "pkg/\n", "pkg/gen/table.txt": "table\n"})
	gitIn(t, repository, "init", "-q")
	for _, tree := range []string{alone, filepath.Join(repository, "pkg")} {
		p := ignoredPool(t, nil)
		c := ignoredCommand(t, tree)
		holds(t, ignoredRun(t, p, c, tree), map[string]bool{"gen/table.txt": true})
		if run, failure := p.InProject(t.Context(), c); failure != nil || run.LeftOut != nil {
			t.Errorf("%s: the copy left out %q, %v", tree, run.LeftOut, failure)
		}
	}
}

// TestAPoolWhoseSettingsFailOpensNone: settings that cannot be read when
// a pool opens fail it, before any copy; a counted run's pool reads none.
func TestAPoolWhoseSettingsFailOpensNone(t *testing.T) {
	settings := copySettings{failure: mutants.InputFailed{Cause: errors.New("unreadable")}}
	var failed mutants.WorkerFailed
	if _, failure := (Workers{Platform: runtime.GOOS, Settings: settings}).Open(t.Context(), mutants.MutationRun, 1); !errors.As(failure, &failed) {
		t.Errorf("a pool whose settings fail: %v, not WorkerFailed", failure)
	}
	opened, failure := (Workers{Platform: runtime.GOOS, Settings: settings}).Open(t.Context(), mutants.FrozenTrials, 1)
	if failure != nil {
		t.Fatalf("a counted run's pool: %v", failure)
	}
	p := opened.(*pool)
	if f := p.Close(t.Context()); f != nil {
		t.Error(f)
	}
	if p.workers[0].worker.rules != nil {
		t.Error("a counted run's copies leave out what git ignores")
	}
}

// TestMutationCopyGlobsMatchFromTheProjectRoot: a glob matches a path by
// its path from the project root, a folder's glob may match what it
// leads to, and nothing outside the root matches.
func TestMutationCopyGlobsMatchFromTheProjectRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	r := &copyRules{root: root, globs: []string{"gen/**", "dist/*.d.ts", "docs"}}
	at := func(rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }
	for rel, want := range map[string]bool{"gen": true, "gen/a/b.txt": true, "dist/x.d.ts": true, "dist/x.js": false, "docs": true, "src": false, "../gen": false, ".": false} {
		if got := r.brings(at(rel)); got != want {
			t.Errorf("brings %s: %v, want %v", rel, got, want)
		}
	}
	for rel, want := range map[string]bool{"dist": true, "dist/sub": true, "docs": true, "src": false, "../dist": false} {
		if got := r.mayBring(at(rel)); got != want {
			t.Errorf("may bring within %s: %v, want %v", rel, got, want)
		}
	}
	if !(&copyRules{root: root, globs: []string{"**/x"}}).mayBring(at("any")) {
		t.Error("a glob that begins with ** may match within any folder")
	}
	if (&copyRules{root: root}).brings(at("gen")) || (*copyRules)(nil).mayBring(at("gen")) {
		t.Error("no glob brings anything")
	}
}

// fakeEntry is a directory entry of the type kind, named name.
type fakeEntry struct {
	name string
	kind fs.FileMode
}

func (e fakeEntry) Name() string               { return e.name }
func (e fakeEntry) IsDir() bool                { return e.kind.IsDir() }
func (e fakeEntry) Type() fs.FileMode          { return e.kind }
func (e fakeEntry) Info() (fs.FileInfo, error) { return nil, fs.ErrInvalid }

// TestCopyEntryCopiesFoldersFilesAndLinksAlone: an entry neither a
// folder, a file nor a link, such as a named pipe, is not copied; folders
// it cannot make above the target and a link it cannot read fail.
func TestCopyEntryCopiesFoldersFilesAndLinksAlone(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	writeTree(t, dir, map[string]string{"file": "x\n"})
	if err := copyEntry(file, filepath.Join(dir, "pipe"), fakeEntry{"pipe", fs.ModeNamedPipe}, false); err != nil {
		t.Errorf("a named pipe: %v", err)
	}
	holds(t, dir, map[string]bool{"pipe": false})
	if err := copyEntry(file, filepath.Join(file, "sub", "x"), fakeEntry{"x", 0}, true); err == nil {
		t.Error("the folders above a target where a file stands were made")
	}
	if err := copyEntry(file, filepath.Join(dir, "link"), fakeEntry{"link", fs.ModeSymlink}, false); err == nil {
		t.Error("a link that cannot be read was copied")
	}
}
