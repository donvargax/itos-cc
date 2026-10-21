package imports

import (
	"context"
	"path"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/project"
)

// typescript resolves TypeScript's and JavaScript's imports: a module is a
// file, and an import names one relatively, through the aliases of the
// nearest tsconfig.json, or else is an external package.
type typescript struct{ plain }

// index's tsconfig cache lasts the one Resolve call it serves.
func (typescript) index(root string, files []File) resolver {
	r := &tsResolver{root: root, byFile: map[string]string{}, tsconfigs: map[string]*tsAliases{}}
	for _, f := range files {
		r.byFile[strings.TrimSuffix(f.Rel, path.Ext(f.Rel))] = moduleID(f)
	}
	return r
}

type tsResolver struct {
	root      string
	byFile    map[string]string // repository-relative file without extension → module
	tsconfigs map[string]*tsAliases
}

var tsExtensions = []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts"}

func (r *tsResolver) resolve(ctx context.Context, f *File, imp lang.Import) ([]string, string) {
	spec := imp.Path
	if strings.HasPrefix(spec, ".") {
		if id, ok := r.file(path.Join(path.Dir(f.Rel), spec)); ok {
			return []string{id}, ""
		}
		return nil, ""
	}
	// An alias from tsconfig paths or baseUrl names a project file.
	for _, abs := range r.aliases(ctx, f).resolve(spec) {
		if id, ok := r.file(rel(r.root, abs)); ok {
			return []string{id}, ""
		}
	}
	parts := strings.SplitN(spec, "/", 3)
	if strings.HasPrefix(spec, "@") && len(parts) > 1 {
		return nil, parts[0] + "/" + parts[1]
	}
	return nil, parts[0]
}

func (*tsResolver) unimported(*File) []string { return nil }

// file finds the module a repository-relative import target names: the
// file itself, or the index of a directory. ESM imports name the compiled
// file, so ./a.js means a.ts.
func (r *tsResolver) file(target string) (string, bool) {
	for _, ext := range tsExtensions {
		target = strings.TrimSuffix(target, ext)
	}
	for _, candidate := range []string{target, target + "/index"} {
		if id, ok := r.byFile[candidate]; ok {
			return id, true
		}
	}
	return "", false
}

// aliases are the tsconfig aliases that apply to f: those of the nearest
// tsconfig.json above it, found under ctx.
func (r *tsResolver) aliases(ctx context.Context, f *File) *tsAliases {
	dir := project.FindUp(ctx, f.Abs, "tsconfig.json")
	a, ok := r.tsconfigs[dir]
	if !ok {
		a = &tsAliases{}
		if dir != "" {
			a = loadTSAliases(dir)
		}
		r.tsconfigs[dir] = a
	}
	return a
}
