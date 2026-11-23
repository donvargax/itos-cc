package workdir

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/donvargax/itos-cc/internal/lang"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	"github.com/donvargax/itos-cc/internal/past"
	"github.com/donvargax/itos-cc/internal/project"
	"github.com/donvargax/itos-cc/internal/selection"
	"github.com/donvargax/itos-cc/internal/selection/port"
)

// A commit's tree, as git's objects hold it, is read through the same ports
// the working directory is: Commits opens a commit as a past.Commit whose
// Tree and Surroundings answer from the tree git recorded, never from the
// disk, so a slice cannot tell where the files come from.

// Entry is one file of a commit's tree: its path from the repository's top
// level, slash-separated, the id of its content and its size.
type Entry struct {
	Path, ID string
	Size     int64
}

// Objects is git's object database, as the commits of one repository are
// read from it. A failure to name a commit is *past.BadRev, no repository
// *past.NoRepository.
type Objects interface {
	// Top is the project root of the working directory, absolute: where
	// the commit's paths start.
	Top(ctx context.Context) (string, error)
	// CommonDir is the git common directory, absolute, which every
	// worktree of the repository shares.
	CommonDir(ctx context.Context) (string, error)
	// Resolve is the full id of the commit rev names.
	Resolve(ctx context.Context, rev string) (string, error)
	// Entries are the regular files of the commit's tree, sorted by path.
	Entries(ctx context.Context, commit string) ([]Entry, error)
	// Blobs are the contents the ids name, by id.
	Blobs(ctx context.Context, ids []string) (map[string][]byte, error)
}

// Keeper is the files under the git common directory the kept results are
// written to.
type Keeper interface {
	// Read is the file at path, absolute, false when it is not there or
	// cannot be read.
	Read(ctx context.Context, path string) ([]byte, bool)
	// Write writes data to path, absolute, making the folders above it,
	// whole and atomically: a reader sees the file as it was, or all of
	// data, never a part. A file already there, one a run found corrupt,
	// is replaced.
	Write(ctx context.Context, path string, data []byte) error
}

// Commits opens the commits of the repository holding Dir, the working
// directory, read through Objects.
type Commits struct {
	Dir     string
	Objects Objects
	Keeper  Keeper
	// Settings is the reader of the selection settings the text of a
	// commit's itos-cc.yaml, nil when it has none, at root, gives.
	Settings func(root string, text []byte) port.SettingsReader
}

var _ past.Source = Commits{}

// Open is the commit rev names, its files named as they would be under the
// project root of Dir, and the itos-cc.yaml it holds, if it holds one, its
// settings. A done ctx opens nothing.
func (c Commits) Open(ctx context.Context, rev string) (past.Commit, error) {
	if ctx.Err() != nil {
		return past.Commit{}, context.Cause(ctx)
	}
	id, err := c.Objects.Resolve(ctx, rev)
	if err != nil {
		return past.Commit{}, err
	}
	top, err := c.Objects.Top(ctx)
	if err != nil {
		return past.Commit{}, err
	}
	common, err := c.Objects.CommonDir(ctx)
	if err != nil {
		return past.Commit{}, err
	}
	entries, err := c.Objects.Entries(ctx, id)
	if err != nil {
		return past.Commit{}, err
	}
	tree := newCommitTree(top, c.Dir, id, entries, c.Objects)
	text, _ := tree.Read(ctx, filepath.Join(top, "itos-cc.yaml"))
	settings, failure := c.Settings(top, text).ReadSelection(ctx)
	if failure != nil {
		return past.Commit{}, failure
	}
	return past.Commit{
		ID: id, Root: top, Dir: c.Dir,
		Tree: tree, Surroundings: commitSurroundings{tree},
		Exclude: selection.Exclusion(settings),
		Store:   past.Nothing{RootDir: top},
		Results: kept{dir: filepath.Join(common, "itos-cc", "history"), keeper: c.Keeper},
	}, nil
}

// kept is the results kept under dir, one folder for each commit.
type kept struct {
	dir    string
	keeper Keeper
}

var _ past.Results = kept{}

func (k kept) path(commit, command, key string) string {
	return filepath.Join(k.dir, commit, command+"-"+key+".json")
}

func (k kept) Get(ctx context.Context, commit, command, key string) ([]byte, bool) {
	return k.keeper.Read(ctx, k.path(commit, command, key))
}

func (k kept) Put(ctx context.Context, commit, command, key string, data []byte) {
	_ = k.keeper.Write(ctx, k.path(commit, command, key), data)
}

// commitTree is a commit's tree at the working directory dir: selection's
// Tree and lang's Surroundings over the files of the commit.
type commitTree struct {
	dir   string
	files *commitFS
}

var (
	_ port.Tree             = (*commitTree)(nil)
	_ langport.Surroundings = commitSurroundings{}
)

func newCommitTree(top, dir, commit string, entries []Entry, objects Objects) *commitTree {
	return &commitTree{dir: dir, files: newCommitFS(top, commit, entries, objects)}
}

// abs is path made absolute from the working directory.
func (t *commitTree) abs(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(t.dir, path)
}

func (t *commitTree) Discover(ctx context.Context, roots []string) (sources, tests []string, failure port.Failure) {
	if ctx.Err() != nil {
		return nil, nil, &port.Cancelled{Cause: context.Cause(ctx)}
	}
	if err := t.files.load(ctx); err != nil {
		return nil, nil, failed(ctx, err)
	}
	absolute := make([]string, 0, len(roots))
	for _, r := range roots {
		absolute = append(absolute, t.abs(r))
	}
	found, err := project.DiscoverIn(ctx, t.files, absolute, project.BuildOutputIn(t.files))
	if err != nil && ctx.Err() != nil {
		return nil, nil, &port.Cancelled{Cause: context.Cause(ctx)}
	}
	if err != nil {
		return nil, nil, &port.Unwalkable{Reason: err.Error() + " in commit " + t.files.commit}
	}
	return found.Sources, found.Tests, nil
}

// failed is err as the port's failure: a cancellation is one, anything else
// an unwalkable tree.
func failed(ctx context.Context, err error) port.Failure {
	if ctx.Err() != nil {
		return &port.Cancelled{Cause: context.Cause(ctx)}
	}
	return &port.Unwalkable{Reason: err.Error()}
}

// Changed is never asked: a commit has no changes of the working tree's.
func (t *commitTree) Changed(ctx context.Context) ([]string, port.Failure) {
	return nil, &port.NoGit{Reason: "--at measures a commit, not the working tree's changes"}
}

func (t *commitTree) Exists(_ context.Context, path string) bool {
	_, err := t.files.Stat(t.abs(path))
	return err == nil
}

func (t *commitTree) Read(ctx context.Context, path string) ([]byte, port.Failure) {
	if ctx.Err() != nil {
		return nil, &port.Cancelled{Cause: context.Cause(ctx)}
	}
	text, err := t.files.fileContent(ctx, t.abs(path))
	if err != nil {
		return nil, &port.Unreadable{Path: path, Reason: err.Error()}
	}
	return text, nil
}

// commitSurroundings are what surrounds the files of a commit, as lang's
// port asks for them.
type commitSurroundings struct{ tree *commitTree }

func (s commitSurroundings) Exists(ctx context.Context, path string) bool {
	return s.tree.Exists(ctx, path)
}

func (s commitSurroundings) Read(ctx context.Context, path string) ([]byte, bool) {
	text, failure := s.tree.Read(ctx, path)
	return text, failure == nil
}

func (s commitSurroundings) WorkingDir(context.Context) (string, bool) { return s.tree.dir, true }

func (s commitSurroundings) Abs(_ context.Context, path string) (string, bool) {
	return s.tree.abs(path), true
}

// commitFS is project.FS over the files of one commit: its entries named
// absolutely from top, the contents of the ones a command reads loaded
// together once, and of any other when asked.
type commitFS struct {
	top, commit string
	objects     Objects
	entries     map[string]Entry
	dirs        map[string][]string

	mu      sync.Mutex
	loaded  bool
	content map[string][]byte // by entry id
}

var _ project.FS = (*commitFS)(nil)

func newCommitFS(top, commit string, entries []Entry, objects Objects) *commitFS {
	f := &commitFS{top: top, commit: commit, objects: objects, entries: map[string]Entry{}, dirs: map[string][]string{top: nil}, content: map[string][]byte{}}
	seen := map[string]bool{top: true}
	for _, e := range entries {
		abs := filepath.Join(top, filepath.FromSlash(e.Path))
		f.entries[abs] = e
		for child := abs; child != filepath.Dir(child) && !seen[child]; child = filepath.Dir(child) {
			seen[child] = true
			parent := filepath.Dir(child)
			f.dirs[parent] = append(f.dirs[parent], filepath.Base(child))
		}
	}
	for dir := range f.dirs {
		sort.Strings(f.dirs[dir])
	}
	return f
}

// readSet is whether a command may read the file at path without being
// asked for it by name: a file of a language itos-cc measures, or one the
// names around such files are looked up in.
func readSet(path string) bool {
	if lang.Detect(path) != nil {
		return true
	}
	switch strings.ToLower(filepath.Base(path)) {
	case "go.mod", "package.json", "tsconfig.json", "pyproject.toml", "setup.py", "setup.cfg",
		"pubspec.yaml", "l10n.yaml", "itos-cc.yaml", "build.gradle", "build.gradle.kts",
		"settings.gradle", "settings.gradle.kts", "pom.xml":
		return true
	}
	return false
}

// load reads every file of the read set at once, the first time it is
// asked, in one run of git.
func (f *commitFS) load(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.loaded {
		return nil
	}
	var ids []string
	for abs, e := range f.entries {
		if readSet(abs) {
			ids = append(ids, e.ID)
		}
	}
	sort.Strings(ids)
	blobs, err := f.objects.Blobs(ctx, ids)
	if err != nil {
		return err
	}
	for id, text := range blobs {
		f.content[id] = text
	}
	f.loaded = true
	return nil
}

// content is the file at path, from the commit.
func (f *commitFS) fileContent(ctx context.Context, path string) ([]byte, error) {
	e, ok := f.entries[path]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}
	if err := f.load(ctx); err != nil {
		return nil, err
	}
	f.mu.Lock()
	text, ok := f.content[e.ID]
	f.mu.Unlock()
	if ok {
		return text, nil
	}
	blobs, err := f.objects.Blobs(ctx, []string{e.ID})
	if err != nil {
		return nil, err
	}
	text, ok = blobs[e.ID]
	if !ok {
		return nil, &fs.PathError{Op: "read", Path: path, Err: errors.New("git holds no such content")}
	}
	f.mu.Lock()
	f.content[e.ID] = text
	f.mu.Unlock()
	return text, nil
}

func (f *commitFS) Lstat(path string) (fs.FileInfo, error) { return f.Stat(path) }

func (f *commitFS) Stat(p string) (fs.FileInfo, error) {
	p = filepath.Clean(p)
	if e, ok := f.entries[p]; ok {
		return info{name: path.Base(e.Path), size: e.Size}, nil
	}
	if _, ok := f.dirs[p]; ok {
		return info{name: filepath.Base(p), dir: true}, nil
	}
	return nil, &fs.PathError{Op: "stat", Path: p, Err: fs.ErrNotExist}
}

func (f *commitFS) ReadDir(p string) ([]fs.DirEntry, error) {
	p = filepath.Clean(p)
	names, ok := f.dirs[p]
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: p, Err: fs.ErrNotExist}
	}
	out := make([]fs.DirEntry, 0, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		child, err := f.Stat(filepath.Join(p, name))
		if err != nil {
			return nil, err
		}
		out = append(out, fs.FileInfoToDirEntry(child))
	}
	return out, nil
}

// ReadFile is the file at path as the commit holds it, when its content is
// loaded: discovery loads the read set first, and a path outside it is read
// through the tree, which asks git.
func (f *commitFS) ReadFile(p string) ([]byte, error) {
	p = filepath.Clean(p)
	e, ok := f.entries[p]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if text, ok := f.content[e.ID]; ok {
		return text, nil
	}
	return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
}

// info is the FileInfo of a file or a folder of a commit.
type info struct {
	name string
	size int64
	dir  bool
}

func (i info) Name() string { return i.name }
func (i info) Size() int64  { return i.size }
func (i info) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}
func (i info) ModTime() time.Time { return time.Time{} }
func (i info) IsDir() bool        { return i.dir }
func (i info) Sys() any           { return nil }
