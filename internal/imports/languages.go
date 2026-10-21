package imports

import (
	"context"
	"path"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
)

// languages are the languages whose imports resolve, by their lang.Spec
// name. Adding one is a resolver in a file of its own and a line here; a
// language with no line is plain.
var languages = map[string]language{
	"dart":       dart{},
	"go":         golang{},
	"kotlin":     kotlin{},
	"python":     python{},
	"typescript": typescript{},
}

// languageOf is the language named name, or plain when none is.
func languageOf(name string) language {
	if l, ok := languages[name]; ok {
		return l
	}
	return plain{}
}

// language is how one language's files import each other.
type language interface {
	// facts keeps in f what resolution needs of src beyond its imports.
	facts(f *File, src *lang.File)
	// module is the module f belongs to, slash-separated from the root.
	module(f File) string
	// index is a resolver over files, this language's files of one Resolve
	// call, root being the project root.
	index(root string, files []File) resolver
	// oneHop says a test reaches only its own module and the modules whose
	// sources import it, rather than all its imports reach.
	oneHop() bool
	// support finds, among tests, this language's tests that reach
	// transitively, the support files each one loads with no import, or is
	// nil when the language has none.
	support(tests []*File) supporter
}

// resolver resolves the imports of one language's files.
type resolver interface {
	// resolve returns the project modules imp names, or the external
	// package it names, looking under ctx for what surrounds f.
	resolve(ctx context.Context, f *File, imp lang.Import) (modules []string, external string)
	// unimported lists the modules f uses with no import, once each.
	unimported(f *File) []string
}

// supporter returns the support files test t loads with no import, root
// being the project root, looking under ctx for what surrounds it.
type supporter func(ctx context.Context, root string, t *File) []*File

// plain is a language with nothing of its own: its module is the file
// without its extension, nothing it imports resolves, and its tests reach
// transitively with no support files. A language embeds it for what it does
// not change.
type plain struct{}

func (plain) facts(*File, *lang.File) {}

func (plain) module(f File) string { return fileModule(f) }

func (plain) index(string, []File) resolver { return unresolved{} }

func (plain) oneHop() bool { return false }

func (plain) support([]*File) supporter { return nil }

// unresolved resolves nothing.
type unresolved struct{}

func (unresolved) resolve(context.Context, *File, lang.Import) ([]string, string) { return nil, "" }

func (unresolved) unimported(*File) []string { return nil }

// fileModule is the file without its extension.
func fileModule(f File) string { return strings.TrimSuffix(f.Rel, path.Ext(f.Rel)) }

// dirModule is the file's directory, "(root)" at the root.
func dirModule(f File) string {
	dir := path.Dir(f.Rel)
	if dir == "." {
		return "(root)"
	}
	return dir
}
