package imports

import (
	"context"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
)

// kotlin resolves Kotlin's imports: a module is a file, an import names the
// file that declares a package's top-level name, or every file of a package
// for a wildcard, and a file needs no import for its own package.
type kotlin struct{ plain }

func (kotlin) facts(f *File, src *lang.File) {
	f.topLevel = lang.TopLevelNames(src)
	f.refs = lang.ReferencedNames(src)
}

func (kotlin) index(_ string, files []File) resolver {
	r := kotlinResolver{byPackage: map[string][]string{}, byFQ: map[string]string{}}
	for _, f := range files {
		id := moduleID(f)
		r.byPackage[f.Namespace] = appendUnique(r.byPackage[f.Namespace], id)
		for _, name := range f.topLevel {
			r.byFQ[f.Namespace+"."+name] = id
		}
	}
	return r
}

type kotlinResolver struct {
	byPackage map[string][]string // package → modules
	byFQ      map[string]string   // package.Name → module
}

func (r kotlinResolver) resolve(_ context.Context, _ *File, imp lang.Import) ([]string, string) {
	if imp.Wildcard {
		if ids, ok := r.byPackage[imp.Path]; ok {
			return ids, ""
		}
	}
	// a.b.Outer.Inner imports from the file that declares a.b.Outer.
	for name := imp.Path; name != ""; name = parentModule(name) {
		if id, ok := r.byFQ[name]; ok {
			return []string{id}, ""
		}
	}
	parts := strings.Split(imp.Path, ".")
	if len(parts) > 2 {
		return nil, strings.Join(parts[:2], ".")
	}
	return nil, imp.Path
}

// unimported lists the modules of f's own package that declare a name f
// uses, once each.
func (r kotlinResolver) unimported(f *File) []string {
	var out []string
	for _, name := range f.refs {
		if id, ok := r.byFQ[f.Namespace+"."+name]; ok {
			out = appendUnique(out, id)
		}
	}
	return out
}
