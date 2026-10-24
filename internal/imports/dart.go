package imports

import (
	"context"
	"path"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/project"
)

// dart resolves Dart's imports: a module is a library's file, and a
// directive names another by a URI, relative to the naming file or
// package:<name>/<path>, lib/<path> of the project's package whose
// pubspec.yaml has that name, a pub workspace member's sibling among them.
// lang lists each import's, export's and part's URI, every target of a
// conditional one, so a library depends on what it exports and on its
// parts, and a part's own imports count for the library that holds it. dart:
// libraries and other packages are external.
type dart struct{ plain }

func (dart) index(_ string, files []File) resolver {
	r := dartResolver{byFile: map[string]string{}, byLib: map[string][]File{}}
	for _, f := range files {
		r.byFile[f.Rel] = moduleID(f)
		// Every lib/ folder above the file may be its package's.
		for rest := f.Rel; ; {
			i := strings.Index(rest, "lib/")
			if i < 0 {
				break
			}
			if i == 0 || rest[i-1] == '/' {
				r.byLib[rest[i+len("lib/"):]] = append(r.byLib[rest[i+len("lib/"):]], f)
			}
			rest = rest[i+len("lib/"):]
		}
	}
	return r
}

type dartResolver struct {
	byFile map[string]string // repository-relative file → module
	byLib  map[string][]File // path under a lib/ folder → the files there
}

func (r dartResolver) resolve(_ context.Context, f *File, imp lang.Import) ([]string, string) {
	uri := imp.Path
	if rest, ok := strings.CutPrefix(uri, "package:"); ok {
		pkg, file, _ := strings.Cut(rest, "/")
		// lang names a file under lib/ by its package's pubspec.yaml name
		// and its path under lib/, dotted.
		want := pkg + "." + strings.ReplaceAll(strings.TrimSuffix(file, path.Ext(file)), "/", ".")
		for _, c := range r.byLib[file] {
			if c.Namespace == want {
				return []string{moduleID(c)}, ""
			}
		}
		return nil, pkg
	}
	if scheme, _, ok := strings.Cut(uri, ":"); ok && !strings.Contains(scheme, "/") {
		return nil, uri // dart:async, or another scheme's
	}
	if id, ok := r.byFile[path.Join(path.Dir(f.Rel), uri)]; ok {
		return []string{id}, ""
	}
	return nil, ""
}

func (dartResolver) unimported(*File) []string { return nil }

// dartTestConfig is the file flutter test loads around each test file.
const dartTestConfig = "flutter_test_config.dart"

// support: flutter test loads the nearest flutter_test_config.dart in a
// test file's directory or one above it, up to its package's, with no
// import. A test file is one flutter test runs, a _test.dart file.
func (dart) support(tests []*File) supporter {
	configs := map[string]*File{} // by directory
	for _, t := range tests {
		if filepath.Base(t.Abs) == dartTestConfig {
			configs[filepath.Dir(t.Abs)] = t
		}
	}
	if len(configs) == 0 {
		return nil
	}
	return func(ctx context.Context, root string, t *File) []*File {
		if !strings.HasSuffix(filepath.Base(t.Abs), "_test.dart") {
			return nil
		}
		top := project.FindUp(ctx, t.Abs, "pubspec.yaml")
		if top == "" {
			top = root
		}
		for dir := filepath.Dir(t.Abs); ; dir = filepath.Dir(dir) {
			if c := configs[dir]; c != nil {
				return []*File{c}
			}
			if rel, err := filepath.Rel(top, dir); err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.Dir(dir) == dir {
				return nil
			}
		}
	}
}
