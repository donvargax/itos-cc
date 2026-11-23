// Package project finds the source and test files the tools analyze.
package project

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/selection"
)

// skipDirs are never walked: dependencies, caches, and fixtures that are
// not part of the program. Build output is skipped by IsBuildOutput.
var skipDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true, ".idea": true, ".vscode": true,
	"node_modules": true, "vendor": true, ".gradle": true, ".venv": true, "venv": true,
	"__pycache__": true, ".mypy_cache": true, ".pytest_cache": true, ".tox": true,
	".metrics": true, "testdata": true,
}

// buildDirs are the names build and test tools write their output to. They
// are also ordinary package names, such as Go's internal/out.
var buildDirs = map[string]bool{"target": true, "build": true, "dist": true, "out": true, "coverage": true}

// buildManifests mark a directory whose tool writes buildDirs beside them:
// npm and its bundlers, Maven, Gradle, Cargo, Python packaging, and Dart's
// pub, whose flutter and dart write build and coverage. Go writes no
// output into the module.
var buildManifests = []string{
	"package.json", "pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle",
	"settings.gradle.kts", "Cargo.toml", "pyproject.toml", "setup.py", "setup.cfg",
	"pubspec.yaml",
}

// IsBuildOutput reports whether dir is build or test output rather than
// source: it has a build output name, and it sits beside a manifest of a
// tool that writes there, or git ignores it, asked under ctx.
func IsBuildOutput(ctx context.Context, dir string) bool {
	return IsBuildOutputWithIgnore(ctx, dir, gitIgnores)
}

// gitIgnores says whether git ignores dir. Once ctx is done it starts no
// command and says no.
func gitIgnores(ctx context.Context, dir string) bool {
	if ctx.Err() != nil {
		return false
	}
	return gitCommand(filepath.Dir(dir), "check-ignore", "-q", dir).Run() == nil
}

// IsBuildOutputWithIgnore reports whether dir is a known build directory
// with a nearby build manifest or whether ignored, asked under ctx, says it
// is ignored. The callback lets committed-input callers evaluate ignore
// rules against a frozen tree instead of the live checkout.
func IsBuildOutputWithIgnore(ctx context.Context, dir string, ignored func(ctx context.Context, dir string) bool) bool {
	return isBuildOutputIn(ctx, OS{}, dir, ignored)
}

// isBuildOutputIn is IsBuildOutputWithIgnore over the files of fsys.
func isBuildOutputIn(ctx context.Context, fsys FS, dir string, ignored func(ctx context.Context, dir string) bool) bool {
	if !buildDirs[filepath.Base(dir)] {
		return false
	}
	for _, m := range buildManifests {
		if _, err := fsys.Stat(filepath.Join(filepath.Dir(dir), m)); err == nil {
			return true
		}
	}
	return ignored != nil && ignored(ctx, dir)
}

// Files are the supported source files under some roots, split into
// production code and test code. Paths are absolute and sorted.
type Files = selection.Files

// Discover walks roots (files or directories) under ctx and classifies
// every file in a supported language. It never walks into a skipped,
// hidden or build output directory, or a Flutter app's platform folder,
// and takes no generated Dart. A root that names a file is taken even if
// it would be skipped while walking. Once ctx is done it returns its
// cause.
func Discover(ctx context.Context, roots []string) (Files, error) {
	return DiscoverWithBuildOutput(ctx, roots, IsBuildOutput)
}

// DiscoverWithBuildOutput is Discover with an explicit build-output
// classifier, asked under ctx. All other source/test, hidden-directory,
// fixture, dependency, Flutter and generated-Dart rules remain the same.
func DiscoverWithBuildOutput(ctx context.Context, roots []string, isBuildOutput func(ctx context.Context, dir string) bool) (Files, error) {
	if isBuildOutput == nil {
		isBuildOutput = IsBuildOutput
	}
	return DiscoverIn(ctx, OS{}, roots, isBuildOutput)
}

// DiscoverIn is Discover over the files of fsys, whose build output
// isBuildOutput tells, asked under ctx: the one set of rules that chooses
// what a command measures, on the disk or in a commit's tree.
func DiscoverIn(ctx context.Context, fsys FS, roots []string, isBuildOutput func(ctx context.Context, dir string) bool) (Files, error) {
	seen := map[string]bool{}
	flutter := newFlutterRulesIn(fsys)
	var files Files
	add := func(path string) {
		spec := lang.Detect(path)
		if spec == nil || seen[path] {
			return
		}
		seen[path] = true
		if spec.IsTest(path) {
			files.Tests = append(files.Tests, path)
		} else {
			files.Sources = append(files.Sources, path)
		}
	}
	for _, root := range roots {
		abs, err := filepath.Abs(root)
		if err != nil {
			return files, err
		}
		info, err := fsys.Stat(abs)
		if err != nil {
			return files, err
		}
		if !info.IsDir() {
			add(abs)
			continue
		}
		err = WalkDirIn(ctx, fsys, abs, func(ctx context.Context, path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != abs && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".") || isBuildOutput(ctx, path) || flutter.platformDir(path)) {
					return filepath.SkipDir
				}
				return nil
			}
			if !flutter.generated(path) {
				add(path)
			}
			return nil
		})
		if err != nil {
			return files, err
		}
	}
	sort.Strings(files.Sources)
	sort.Strings(files.Tests)
	return files, nil
}

// Changed returns the supported files under the working directory that git
// reports as added or modified, staged or not, plus untracked files, as paths
// relative to the working directory. Git lists them NUL-terminated, so a name
// with spaces or non-ASCII letters arrives unquoted.
func Changed() ([]string, error) {
	var paths []string
	for _, args := range [][]string{
		{"diff", "--name-only", "-z", "--relative", "--diff-filter=AMR", "HEAD"},
		{"ls-files", "-z", "--others", "--exclude-standard"},
	} {
		out, err := gitCommand("", args...).Output()
		if err != nil {
			// A repository without commits has no HEAD; fall back to the index.
			if args[0] == "diff" {
				out, err = gitCommand("", "diff", "--name-only", "-z", "--relative", "--cached").Output()
			}
			if err != nil {
				return nil, err
			}
		}
		for _, name := range strings.Split(string(out), "\x00") {
			if name != "" && lang.Detect(name) != nil {
				if _, err := os.Stat(name); err == nil {
					paths = append(paths, name)
				}
			}
		}
	}
	return paths, nil
}

// BuildOutputIn is the build-output classifier of the files of fsys, which
// have no ignore rules to ask: a build directory beside a manifest of a tool
// that writes there.
func BuildOutputIn(fsys FS) func(ctx context.Context, dir string) bool {
	return func(ctx context.Context, dir string) bool { return isBuildOutputIn(ctx, fsys, dir, nil) }
}
