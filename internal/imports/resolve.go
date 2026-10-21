// Package imports is the import-resolution engine (ADR-0051, T-162): it
// resolves each language's imports into dependencies between a repository's
// modules, a Go package or a TypeScript, Python or Kotlin file, and finds the
// test files that reach each source through them. graph draws its
// architecture from a Resolution, and mutation's importgraph adapter reaches
// TestsImporting. It is an adapter-side engine, not a domain: it parses
// files through project, finds the markers above them and reads
// tsconfig.json, and it imports nothing of ours but internal/lang, project
// and the coverage runner, so no slice's values reach it and a caller
// converts its results into its own terms. Each language is a resolver in a
// file of its own (golang.go, python.go, typescript.go, kotlin.go) and a line
// of languages.go's table (T-163), so Resolve, Reach and Of name none.
package imports

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
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
	languageOf(info.Language).facts(&info, f)
	return info
}

// Resolution is the module dependencies of some files: Edges counts each
// module's import statements of another, and Kotlin's uses of a name
// another module of its own package declares; External lists each module's
// packages imported from outside the repository. Modules are named as
// Module names them.
type Resolution struct {
	Edges     map[[2]string]int   // from, to → count
	External  map[string][]string // by module
	resolvers map[string]resolver // by language
}

// Resolve resolves the imports of files, root being the project root, which
// tsconfig aliases resolve against, looking under ctx for what surrounds
// each file. Each language's resolver indexes its own files.
func Resolve(ctx context.Context, root string, files []File) Resolution {
	byLanguage := map[string][]File{}
	for _, f := range files {
		byLanguage[f.Language] = append(byLanguage[f.Language], f)
	}
	r := Resolution{Edges: map[[2]string]int{}, External: map[string][]string{}, resolvers: map[string]resolver{}}
	for name, l := range languages {
		r.resolvers[name] = l.index(root, byLanguage[name])
	}
	r.resolveAll(ctx, files)
	return r
}

// Module is the module f belongs to, slash-separated from the root: its
// package directory in Go and for a Python __init__.py, "(root)" for one at
// the root, the file without its extension elsewhere.
func (Resolution) Module(f File) string { return moduleID(f) }

// moduleID is the module a file belongs to, as its language names it.
func moduleID(f File) string { return languageOf(f.Language).module(f) }

// resolver is the resolver of language, one resolving nothing for a
// language with none.
func (r *Resolution) resolver(language string) resolver {
	if res, ok := r.resolvers[language]; ok {
		return res
	}
	return unresolved{}
}

func (r *Resolution) resolveAll(ctx context.Context, files []File) {
	for i := range files {
		f := &files[i]
		from := moduleID(*f)
		res := r.resolver(f.Language)
		// A name the file uses with no import, such as Kotlin's from its
		// own package, is a dependency too.
		for _, to := range res.unimported(f) {
			if to != from {
				r.Edges[[2]string{from, to}]++
			}
		}
		for _, imp := range f.imports {
			targets, external := res.resolve(ctx, f, imp)
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
