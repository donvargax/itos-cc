package domain

import (
	"context"
	"errors"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	selected "github.com/donvargax/itos-cc/internal/selection"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
	"slices"
)

// ListedSite identifies one actual current mutation site in a source file.
type ListedSite struct {
	Site
	File, Function string
}

// List returns sites in source order then line/column order. A later parse
// failure retains the earlier prefix, unlike Check's presentation admission.
func List(ctx context.Context, files []string, tree selport.Tree, surroundings langport.Surroundings, dir string) ([]ListedSite, error) {
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	if tree == nil || surroundings == nil {
		return nil, errors.New("mutation source tree binding is missing")
	}
	var out []ListedSite
	for _, path := range files {
		f, err := selected.Parse(ctx, tree, surroundings, path)
		if err != nil {
			return out, err
		}
		for _, site := range slices.SortedStableFunc(slices.Values(Sites(f)), LineOrder) {
			unit := f.Units[site.Unit]
			out = append(out, ListedSite{Site: site, File: relativeName(dir, path), Function: UnitID(unit.Namespace, unit.Name)})
		}
		f.Close()
	}
	return out, stopped(ctx, nil)
}
