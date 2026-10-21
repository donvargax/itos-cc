package imports

import (
	"context"

	"github.com/donvargax/itos-cc/internal/lang"
)

// golang resolves Go's imports: a module is a package, its directory, and an
// import path names the package whose files declare it.
type golang struct{ plain }

func (golang) module(f File) string { return dirModule(f) }

func (golang) index(_ string, files []File) resolver {
	r := goResolver{byImport: map[string]string{}}
	for _, f := range files {
		r.byImport[f.Namespace] = moduleID(f)
	}
	return r
}

// oneHop: a Go test reaches its own package and the packages whose sources
// import it, and no further.
func (golang) oneHop() bool { return true }

type goResolver struct {
	byImport map[string]string // import path → module
}

func (r goResolver) resolve(_ context.Context, _ *File, imp lang.Import) ([]string, string) {
	if id, ok := r.byImport[imp.Path]; ok {
		return []string{id}, ""
	}
	return nil, imp.Path
}

func (goResolver) unimported(*File) []string { return nil }
