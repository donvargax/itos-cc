package mutation

import (
	"context"
	"fmt"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/mutants"
)

// ListFound retains the actual ordered domain prefix and selection notices.
type ListFound struct {
	Sites     []mutants.ListedSite
	Excluded  int
	NoSources bool
}

func (h Handler) List(ctx context.Context, q SelectionQuery) (ListFound, error) {
	files, err := h.files(ctx, q)
	if err != nil {
		return ListFound{}, err
	}
	found := ListFound{Excluded: files.Excluded, NoSources: len(files.Sources) == 0}
	if found.NoSources {
		return found, nil
	}
	found.Sites, err = mutants.List(ctx, files.Sources, h.Tree, h.Surroundings, h.Dir)
	return found, err
}

// Site is one mutation site in the command's existing JSON representation.
type Site struct {
	File        string `json:"file"`
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Function    string `json:"function"`
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
}
type listResult struct {
	Sites []Site `json:"sites"`
}

// Run assembles selection once, then presents even the prefix before a later failure.
func (c List) Run(ctx context.Context, ui *cli.UI, h Handler) int {
	found, err := h.List(ctx, SelectionQuery{Paths: c.Paths, Changed: c.Changed})
	result := listResult{Sites: []Site{}}
	sayExcluded(ui, found.Excluded)
	if found.NoSources {
		ui.Line("no source files to list")
	}
	for _, s := range found.Sites {
		site := Site{File: s.File, Line: s.Line, Column: s.Column, Function: s.Function, Original: s.Original, Replacement: s.Replacement}
		result.Sites = append(result.Sites, site)
		if !c.JSON {
			_, _ = fmt.Fprintf(ui.Stdout, "%s:%d:%d %s → %s in %s\n", site.File, site.Line, site.Column, Quote(site.Original), Quote(site.Replacement), site.Function)
		}
	}
	return ui.Emit(c.JSON, result, ui.Problems(readError(err)))
}
func sayExcluded(ui *cli.UI, n int) {
	if n > 0 {
		ui.Line(fmt.Sprintf("%d sources left out by exclude in itos-cc.yaml", n))
	}
}
