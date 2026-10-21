package imports

import (
	"context"
	"path"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/project"
)

// File is what import resolution needs from one parsed source file.
type File struct {
	Rel       string // repository-relative, slash-separated
	Abs       string
	Language  string
	Namespace string
	imports   []lang.Import
	topLevel  []string // Kotlin declarations other files import
	refs      []string // Kotlin names used, for dependencies within a package
	test      bool     // test code, a runnable test or a test-support file
}

// Of is what resolution needs of a parsed file to resolve what it imports,
// and what imports it, root being the repository root its Rel is from.
func Of(root string, f *lang.File) File {
	info := File{Rel: rel(root, f.Path), Abs: f.Path, Language: f.Spec.Name, Namespace: f.Namespace, imports: f.Imports(),
		test: f.Spec.IsTest(f.Path)}
	if f.Spec.Name == "kotlin" {
		info.topLevel = lang.TopLevelNames(f)
		info.refs = lang.ReferencedNames(f)
	}
	return info
}

// Resolution is the module dependencies of some files: Edges counts each
// module's import statements of another, and Kotlin's uses of a name
// another module of its own package declares; External lists each module's
// packages imported from outside the repository. Modules are named as
// Module names them.
type Resolution struct {
	Edges    map[[2]string]int   // from, to → count
	External map[string][]string // by module
	idx      *index
}

// Resolve resolves the imports of files, root being the project root, which
// tsconfig aliases resolve against, looking under ctx for what surrounds
// each file.
func Resolve(ctx context.Context, root string, files []File) Resolution {
	r := Resolution{Edges: map[[2]string]int{}, External: map[string][]string{}, idx: newIndex(root, files)}
	r.resolveAll(ctx, files)
	return r
}

// Module is the module f belongs to, slash-separated from the root: its
// package directory in Go and for a Python __init__.py, "(root)" for one at
// the root, the file without its extension elsewhere.
func (Resolution) Module(f File) string { return moduleID(f) }

// moduleID is the module a file belongs to: its package directory in Go and
// for a Python __init__.py, the file without its extension elsewhere.
func moduleID(f File) string {
	if f.Language == "go" || path.Base(f.Rel) == "__init__.py" {
		dir := path.Dir(f.Rel)
		if dir == "." {
			return "(root)"
		}
		return dir
	}
	return strings.TrimSuffix(f.Rel, path.Ext(f.Rel))
}

func (r *Resolution) resolveAll(ctx context.Context, files []File) {
	idx := r.idx
	for i := range files {
		f := &files[i]
		from := moduleID(*f)
		// Kotlin needs no import for its own package: a name the file uses
		// that another file of the package declares is a dependency.
		for _, to := range idx.samePackage(f) {
			if to != from {
				r.Edges[[2]string{from, to}]++
			}
		}
		for _, imp := range f.imports {
			targets, external := idx.resolve(ctx, f, imp)
			for _, to := range targets {
				if to != from {
					r.Edges[[2]string{from, to}]++
				}
			}
			if external != "" {
				r.External[from] = appendUnique(r.External[from], external)
			}
		}
	}
}

// index maps the names imports use to module ids.
type index struct {
	root       string
	byFile     map[string]string   // repository-relative file without extension → module
	byDotted   map[string]string   // Python module and package names → module
	byImport   map[string]string   // Go import path → module
	byPackage  map[string][]string // Kotlin package → modules
	byKotlinFQ map[string]string   // Kotlin package.Name → module
	tsconfigs  map[string]*tsAliases
}

func newIndex(root string, files []File) *index {
	idx := &index{root: root, byFile: map[string]string{}, byDotted: map[string]string{}, byImport: map[string]string{},
		byPackage: map[string][]string{}, byKotlinFQ: map[string]string{}, tsconfigs: map[string]*tsAliases{}}
	for _, f := range files {
		id := moduleID(f)
		switch f.Language {
		case "typescript":
			idx.byFile[strings.TrimSuffix(f.Rel, path.Ext(f.Rel))] = id
		case "python":
			idx.byDotted[f.Namespace] = id
		case "go":
			idx.byImport[f.Namespace] = id
		case "kotlin":
			idx.byPackage[f.Namespace] = appendUnique(idx.byPackage[f.Namespace], id)
			for _, name := range f.topLevel {
				idx.byKotlinFQ[f.Namespace+"."+name] = id
			}
		}
	}
	return idx
}

// resolve returns the project modules an import names, or the external
// package it names, looking under ctx for what surrounds f.
func (idx *index) resolve(ctx context.Context, f *File, imp lang.Import) (targets []string, external string) {
	switch f.Language {
	case "typescript":
		return idx.typescript(ctx, f, imp.Path)
	case "python":
		return idx.python(f, imp)
	case "go":
		if id, ok := idx.byImport[imp.Path]; ok {
			return []string{id}, ""
		}
		return nil, imp.Path
	case "kotlin":
		return idx.kotlin(imp)
	}
	return nil, ""
}

var tsExtensions = []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts"}

func (idx *index) typescript(ctx context.Context, f *File, spec string) ([]string, string) {
	if strings.HasPrefix(spec, ".") {
		if id, ok := idx.tsFile(path.Join(path.Dir(f.Rel), spec)); ok {
			return []string{id}, ""
		}
		return nil, ""
	}
	// An alias from tsconfig paths or baseUrl names a project file.
	for _, abs := range idx.aliases(ctx, f).resolve(spec) {
		if id, ok := idx.tsFile(rel(idx.root, abs)); ok {
			return []string{id}, ""
		}
	}
	parts := strings.SplitN(spec, "/", 3)
	if strings.HasPrefix(spec, "@") && len(parts) > 1 {
		return nil, parts[0] + "/" + parts[1]
	}
	return nil, parts[0]
}

// tsFile finds the module a repository-relative import target names: the
// file itself, or the index of a directory. ESM imports name the compiled
// file, so ./a.js means a.ts.
func (idx *index) tsFile(target string) (string, bool) {
	for _, ext := range tsExtensions {
		target = strings.TrimSuffix(target, ext)
	}
	for _, candidate := range []string{target, target + "/index"} {
		if id, ok := idx.byFile[candidate]; ok {
			return id, true
		}
	}
	return "", false
}

// aliases are the tsconfig aliases that apply to f: those of the nearest
// tsconfig.json above it, found under ctx.
func (idx *index) aliases(ctx context.Context, f *File) *tsAliases {
	dir := project.FindUp(ctx, f.Abs, "tsconfig.json")
	a, ok := idx.tsconfigs[dir]
	if !ok {
		a = &tsAliases{}
		if dir != "" {
			a = loadTSAliases(dir)
		}
		idx.tsconfigs[dir] = a
	}
	return a
}

func (idx *index) python(f *File, imp lang.Import) ([]string, string) {
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
	targets := idx.pythonModule(module, imp.Names)
	if len(targets) == 0 && !strings.HasPrefix(imp.Path, ".") && f.test {
		// pytest puts a test's directory on sys.path, so a test imports
		// the modules beside it, such as a helper, by their bare names.
		pkg := f.Namespace
		if !strings.HasSuffix(f.Rel, "__init__.py") {
			pkg = parentModule(pkg)
		}
		if pkg != "" {
			targets = idx.pythonModule(join(pkg, module), imp.Names)
		}
	}
	if len(targets) == 0 && !strings.HasPrefix(imp.Path, ".") {
		return nil, strings.Split(module, ".")[0]
	}
	return targets, ""
}

// pythonModule is the modules from module import names depends on: a name
// that is a submodule depends on it; otherwise the dependency is on module
// itself, or the nearest package above it the index holds.
func (idx *index) pythonModule(module string, names []string) []string {
	var targets []string
	onPackage := len(names) == 0
	for _, name := range names {
		if id, ok := idx.byDotted[join(module, name)]; ok {
			targets = append(targets, id)
		} else {
			onPackage = true
		}
	}
	if onPackage {
		for m := module; m != ""; m = parentModule(m) {
			if id, ok := idx.byDotted[m]; ok {
				targets = append(targets, id)
				break
			}
		}
	}
	return targets
}

// samePackage lists the modules of f's own Kotlin package that declare a
// name f uses, once each.
func (idx *index) samePackage(f *File) []string {
	if f.Language != "kotlin" {
		return nil
	}
	var out []string
	for _, name := range f.refs {
		if id, ok := idx.byKotlinFQ[f.Namespace+"."+name]; ok {
			out = appendUnique(out, id)
		}
	}
	return out
}

func (idx *index) kotlin(imp lang.Import) ([]string, string) {
	if imp.Wildcard {
		if ids, ok := idx.byPackage[imp.Path]; ok {
			return ids, ""
		}
	}
	// a.b.Outer.Inner imports from the file that declares a.b.Outer.
	for name := imp.Path; name != ""; name = parentModule(name) {
		if id, ok := idx.byKotlinFQ[name]; ok {
			return []string{id}, ""
		}
	}
	parts := strings.Split(imp.Path, ".")
	if len(parts) > 2 {
		return nil, strings.Join(parts[:2], ".")
	}
	return nil, imp.Path
}

func join(module, name string) string {
	if module == "" {
		return name
	}
	return module + "." + name
}

func parentModule(m string) string {
	if i := strings.LastIndex(m, "."); i >= 0 {
		return m[:i]
	}
	return ""
}

func appendUnique(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}

// rel is path relative to root with forward slashes.
func rel(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(r)
}
