package mutation

import (
	"context"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// notBuilt parts sources into those this platform builds and the Go files
// only another system builds (mutants.SplitBuilt), which run, check and
// sample leave out: no test here runs them, and the outcomes another
// system recorded of them are that system's to judge (slice-25, #18).
// Without the bindings it reads through, every source is built, and the
// command says which binding is missing where it always did.
func (h Handler) notBuilt(ctx context.Context, sources []string) ([]string, []mutants.NotBuilt, error) {
	if len(sources) == 0 || h.Surroundings == nil || h.Toolchain == nil {
		return sources, nil, nil
	}
	return mutants.SplitBuilt(ctx, sources, h.Surroundings, h.Toolchain)
}

// notBuiltFile is a file of "not_built": a source only another system
// builds, and the platform this one is.
type notBuiltFile struct {
	File     string `json:"file"`
	Platform string `json:"platform"`
}

// sayNotBuilt says on stderr, once for each, that a file is not built on
// this platform, so not judged here, naming it slash-separated, and gives
// them as --json lists them, nil for none.
func (p *Presentation) sayNotBuilt(files []mutants.NotBuilt) []notBuiltFile {
	var out []notBuiltFile
	for _, f := range files {
		rel := p.UI.Rel(f.Path)
		p.UI.Line("not built on " + f.Platform.String() + ", so " + p.plain(rel) + " is not judged here")
		out = append(out, notBuiltFile{File: rel, Platform: f.Platform.String()})
	}
	return out
}
