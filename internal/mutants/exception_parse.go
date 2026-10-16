package mutants

import (
	"context"
	"errors"

	"github.com/donvargax/itos-cc/internal/lang"
	selected "github.com/donvargax/itos-cc/internal/selection"
)

func parseExceptionSource(ctx context.Context, path string, opt CheckOptions) (*lang.File, error) {
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	if opt.Tree == nil || opt.Surroundings == nil {
		return nil, errors.New("mutation source tree binding is missing")
	}
	return selected.Parse(ctx, opt.Tree, opt.Surroundings, path)
}
