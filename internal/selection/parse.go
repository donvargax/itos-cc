package selection

import (
	"context"
	"fmt"

	"github.com/donvargax/itos-cc/internal/lang"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	"github.com/donvargax/itos-cc/internal/selection/port"
)

// Parse is the selected file at path parsed, passing ctx on to tree and to
// lang: a language lang does not know is refused before anything is read,
// then tree, the one the file was selected from, reads it, and lang parses
// it, asking around what surrounds it. Every command that measures what it
// selects parses it here, so the order of its failures has one copy: the
// language, then the read's port.Failure, then lang's. The caller closes
// the file.
func Parse(ctx context.Context, tree port.Tree, around langport.Surroundings, path string) (*lang.File, error) {
	spec := lang.Detect(path)
	if spec == nil {
		return nil, fmt.Errorf("unsupported language: %s", path)
	}
	src, failure := tree.Read(ctx, path)
	if failure != nil {
		return nil, failure
	}
	return lang.Parse(ctx, around, spec, path, src)
}
