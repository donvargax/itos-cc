package project

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// held is an FS over files held in memory, named from /r by slash-separated
// path, as a commit's tree is held.
type held struct{ fs fstest.MapFS }

func (h held) inside(path string) string {
	return strings.TrimPrefix(filepath.ToSlash(filepath.Clean(path)), "/")
}

func (h held) Lstat(path string) (fs.FileInfo, error) { return h.Stat(path) }
func (h held) Stat(path string) (fs.FileInfo, error)  { return fs.Stat(h.fs, h.inside(path)) }
func (h held) ReadDir(path string) ([]fs.DirEntry, error) {
	return fs.ReadDir(h.fs, h.inside(path))
}
func (h held) ReadFile(path string) ([]byte, error) { return fs.ReadFile(h.fs, h.inside(path)) }

func files(names ...string) held {
	m := fstest.MapFS{}
	for _, n := range names {
		m[n] = &fstest.MapFile{Data: []byte("x")}
	}
	return held{m}
}

// Discovery over an FS chooses by the rules the disk's does: tests told from
// sources, hidden, dependency and fixture folders skipped, a build folder
// beside its tool's manifest left out, a Flutter app's platform folder and
// its generated Dart too, and a root that is a file taken as it is.
func TestDiscoverInChoosesByTheRulesOfTheDisk(t *testing.T) {
	h := files(
		"r/src/a.ts", "r/src/a.test.ts", "r/.hidden/b.ts", "r/node_modules/c.ts", "r/testdata/d.ts",
		"r/web/package.json", "r/web/dist/e.ts", "r/lib/dist/f.ts",
		"r/app/pubspec.yaml", "r/app/android/g.kt", "r/app/lib/m.dart", "r/app/lib/m.g.dart", "r/README.md",
	)
	h.fs["r/app/pubspec.yaml"] = &fstest.MapFile{Data: []byte("dependencies:\n  flutter:\n    sdk: flutter\n")}
	h.fs["r/web/package.json"] = &fstest.MapFile{Data: []byte("{}")}
	found, err := DiscoverIn(t.Context(), h, []string{"/r", "/r/src/a.ts"}, BuildOutputIn(h))
	if err != nil {
		t.Fatal(err)
	}
	wantSources := []string{"/r/app/lib/m.dart", "/r/lib/dist/f.ts", "/r/src/a.ts"}
	got := make([]string, 0, len(found.Sources))
	for _, s := range found.Sources {
		got = append(got, filepath.ToSlash(s))
	}
	if !slices.Equal(got, wantSources) || len(found.Tests) != 1 || filepath.ToSlash(found.Tests[0]) != "/r/src/a.test.ts" {
		t.Errorf("discovered %q and %q, want %q and the one test", got, found.Tests, wantSources)
	}
	if _, err := DiscoverIn(t.Context(), h, []string{"/r/none"}, BuildOutputIn(h)); err == nil {
		t.Errorf("a root that is not there should fail")
	}
}

// A walk calls back for each entry in order, reads what an FS holds, ends
// early where it is told to and where its context is done, and tells a
// failure to read a folder to the callback.
func TestWalkDirInVisitsEntriesInOrder(t *testing.T) {
	h := files("r/a/x.go", "r/a/y.go", "r/b.go", "r/skip/z.go")
	var seen []string
	err := WalkDirIn(t.Context(), h, "/r", func(_ context.Context, path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		seen = append(seen, filepath.ToSlash(path))
		if d.Name() == "skip" {
			return filepath.SkipDir
		}
		return nil
	})
	want := []string{"/r", "/r/a", "/r/a/x.go", "/r/a/y.go", "/r/b.go", "/r/skip"}
	if err != nil || !slices.Equal(seen, want) {
		t.Errorf("walked %q, %v; want %q", seen, err, want)
	}
	stop := errors.New("stop")
	if err := WalkDirIn(t.Context(), h, "/r", func(context.Context, string, fs.DirEntry, error) error { return stop }); err != stop {
		t.Errorf("a callback's error: %v, want it returned", err)
	}
	var missing error
	_ = WalkDirIn(t.Context(), h, "/none", func(_ context.Context, _ string, _ fs.DirEntry, err error) error { missing = err; return nil })
	if missing == nil {
		t.Errorf("a root that is not there should reach the callback as an error")
	}
	done, cancel := context.WithCancelCause(t.Context())
	cancel(stop)
	if err := WalkDirIn(done, h, "/r", func(context.Context, string, fs.DirEntry, error) error { return nil }); err != stop {
		t.Errorf("a done context: %v, want its cause", err)
	}
	if err := WalkDirIn(t.Context(), h, "/r", func(_ context.Context, _ string, d fs.DirEntry, _ error) error {
		if d.IsDir() && d.Name() == "a" {
			return filepath.SkipAll
		}
		return nil
	}); err != nil {
		t.Errorf("SkipAll should end the walk without an error: %v", err)
	}
}

// On the disk the same walk, the OS's FS and the disk's Discover and
// classifiers answer as they always have.
func TestTheDiskIsAnFS(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src", "a.go"), "package a\n")
	write(t, filepath.Join(dir, "dist", "b.go"), "package b\n")
	write(t, filepath.Join(dir, "package.json"), "{}")
	var names []string
	if err := WalkDir(t.Context(), dir, func(_ context.Context, path string, d fs.DirEntry, _ error) error {
		if !d.IsDir() {
			names = append(names, filepath.Base(path))
		}
		return nil
	}); err != nil || !slices.Equal(names, []string{"b.go", "package.json", "a.go"}) {
		t.Errorf("walked %q, %v", names, err)
	}
	info, err := (OS{}).Lstat(dir)
	if err != nil || !info.IsDir() {
		t.Errorf("Lstat(dir) = %v, %v", info, err)
	}
	if _, err := (OS{}).Stat(filepath.Join(dir, "none")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat of a missing file: %v", err)
	}
	if entries, err := (OS{}).ReadDir(dir); err != nil || len(entries) != 3 {
		t.Errorf("ReadDir = %v, %v", entries, err)
	}
	if text, err := (OS{}).ReadFile(filepath.Join(dir, "package.json")); err != nil || string(text) != "{}" {
		t.Errorf("ReadFile = %q, %v", text, err)
	}
	found, err := DiscoverWithBuildOutput(t.Context(), []string{dir}, nil)
	if err != nil || len(found.Sources) != 1 || filepath.Base(found.Sources[0]) != "a.go" {
		t.Errorf("Discover = %+v, %v; want a.go alone, dist left out beside package.json", found, err)
	}
	if !BuildOutputIn(OS{})(t.Context(), filepath.Join(dir, "dist")) || BuildOutputIn(OS{})(t.Context(), filepath.Join(dir, "src")) {
		t.Errorf("dist beside package.json is build output, src is not")
	}
	if !GeneratedDart("lib/m.g.dart") || GeneratedDart("lib/m.dart") || FlutterPlatformDir(filepath.Join(dir, "web")) {
		t.Errorf("the disk's Dart rules: m.g.dart is generated, m.dart is not, no web folder here is a Flutter app's")
	}
	if !Supported("a.go") || Supported("README.md") {
		t.Errorf("Supported tells a Go file from a Markdown one")
	}
}
