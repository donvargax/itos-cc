package imports

import (
	"context"
	"path"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/project"
)

// python resolves Python's imports: a module is a file, or a package's
// directory for its __init__.py, and an import names a module or package by
// its dotted name, absolutely or relative to the importing file's package.
type python struct{ plain }

func (python) module(f File) string {
	if path.Base(f.Rel) == "__init__.py" {
		return dirModule(f)
	}
	return fileModule(f)
}

func (python) index(_ string, files []File) resolver {
	r := pyResolver{byDotted: map[string]string{}}
	for _, f := range files {
		r.byDotted[f.Namespace] = moduleID(f)
	}
	return r
}

type pyResolver struct {
	byDotted map[string]string // module and package names → module
}

func (r pyResolver) resolve(_ context.Context, f *File, imp lang.Import) ([]string, string) {
	module := imp.Path
	if strings.HasPrefix(module, ".") {
		dots := len(module) - len(strings.TrimLeft(module, "."))
		pkg := strings.Split(f.Namespace, ".")
		if !strings.HasSuffix(f.Rel, "__init__.py") {
			pkg = pkg[:len(pkg)-1]
		}
		up := dots - 1
		if up > len(pkg) {
			return nil, ""
		}
		parts := append([]string{}, pkg[:len(pkg)-up]...)
		if rest := strings.TrimLeft(module, "."); rest != "" {
			parts = append(parts, rest)
		}
		module = strings.Join(parts, ".")
	}
	targets := r.module(module, imp.Names)
	if len(targets) == 0 && !strings.HasPrefix(imp.Path, ".") && f.test {
		// pytest puts a test's directory on sys.path, so a test imports
		// the modules beside it, such as a helper, by their bare names.
		pkg := f.Namespace
		if !strings.HasSuffix(f.Rel, "__init__.py") {
			pkg = parentModule(pkg)
		}
		if pkg != "" {
			targets = r.module(join(pkg, module), imp.Names)
		}
	}
	if len(targets) == 0 && !strings.HasPrefix(imp.Path, ".") {
		return nil, strings.Split(module, ".")[0]
	}
	return targets, ""
}

func (pyResolver) unimported(*File) []string { return nil }

// module is the modules from module import names depends on: a name that is
// a submodule depends on it; otherwise the dependency is on module itself,
// or the nearest package above it the index holds.
func (r pyResolver) module(module string, names []string) []string {
	var targets []string
	onPackage := len(names) == 0
	for _, name := range names {
		if id, ok := r.byDotted[join(module, name)]; ok {
			targets = append(targets, id)
		} else {
			onPackage = true
		}
	}
	if onPackage {
		for m := module; m != ""; m = parentModule(m) {
			if id, ok := r.byDotted[m]; ok {
				targets = append(targets, id)
				break
			}
		}
	}
	return targets
}

// support: pytest loads every conftest.py in a test's directory and those
// above it, up to its build root, with no import.
func (python) support(tests []*File) supporter {
	conftests := map[string]*File{} // by directory
	for _, t := range tests {
		if filepath.Base(t.Abs) == "conftest.py" {
			conftests[filepath.Dir(t.Abs)] = t
		}
	}
	return func(ctx context.Context, root string, t *File) []*File {
		return applyingConftests(ctx, t, conftests, root)
	}
}

// applyingConftests is the conftest.py files pytest loads for t, a test file
// it runs or a conftest.py: those in its directory and each directory above
// it, up to its build root, or root when it has none; none for a helper. Its
// build root is found under ctx.
func applyingConftests(ctx context.Context, t *File, conftests map[string]*File, root string) []*File {
	if len(conftests) == 0 || !coverage.PythonTestFile(t.Abs) && filepath.Base(t.Abs) != "conftest.py" {
		return nil
	}
	top := project.FindUp(ctx, t.Abs, "pyproject.toml", "setup.py", "setup.cfg")
	if top == "" {
		top = root
	}
	var out []*File
	for dir := filepath.Dir(t.Abs); ; dir = filepath.Dir(dir) {
		if c := conftests[dir]; c != nil && c != t {
			out = append(out, c)
		}
		if rel, err := filepath.Rel(top, dir); err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.Dir(dir) == dir {
			return out
		}
	}
}
