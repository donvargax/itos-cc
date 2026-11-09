package mutation

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/selection"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// shownJSON is the mutant in show's --json object, the keys a counted run's
// selected site and an exception entry call the same things.
type shownJSON struct {
	Identity    string `json:"identity"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Function    string `json:"function"`
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
	LineText    string `json:"line_text"`
	MutatedText string `json:"mutated_text"`
}

// showResult is mutation show's --json object: a refusal holds no mutant.
type showResult struct {
	Mutant *shownJSON `json:"mutant,omitempty"`
}

// Show observes the settings, then the exclusion of the file id names, from
// the settings' root, then the file itself: nothing runs, nothing is written,
// and no snapshot or toolchain is asked, the shape of Rehash. An identity that
// names no mutant now is MutantUnmatched for argument, as the person wrote it.
func (h Handler) Show(ctx context.Context, argument string, id mutants.Identity) (mutants.ShownMutant, error) {
	root, cfg, err := h.settings(ctx)
	if err != nil {
		return mutants.ShownMutant{}, err
	}
	if selection.Exclusion(cfg.Selection).Excludes(sourcePath(root, filepath.FromSlash(id.File))) {
		return mutants.ShownMutant{}, MutantUnmatched{Argument: argument, Why: mutants.UnmatchedExcluded}
	}
	found, err := mutants.IdentifiedMutant(ctx, id, h.options(root, nil, nil, nil, nil, nil), h.Status)
	var stale *mutants.IdentityStaleError
	if errors.As(err, &stale) {
		return mutants.ShownMutant{}, MutantUnmatched{Argument: argument, Why: stale.Why, Hash: stale.Hash}
	}
	return found, err
}

// Run refuses a line that names no one identity before anything is read,
// then prints the mutant it names. A plain line names the site as SiteAt does,
// never as path:line:column, which CI problem matchers read as a compiler
// error (docs/CLI.md).
func (c Show) Run(ctx context.Context, ui *cli.UI, h Handler) int {
	refuse := func(p *cli.Problem) int { return ui.Emit(c.JSON, showResult{}, ui.Problems(p)) }
	switch len(c.Identity) {
	case 0:
		return refuse(cli.NewProblem(cli.CodeUsage, "args.missing", "mutation show needs the mutant's identity",
			"Copy the identity whole from the report of mutation run --count, such as its mutation.survived problem."))
	case 1:
	default:
		return refuse(cli.NewProblem(cli.CodeUsage, "args.unexpected", fmt.Sprintf("mutation show takes one identity, and was also given %q", c.Identity[1]),
			"Show one mutant at a time.").With("argument", c.Identity[1]))
	}
	argument := c.Identity[0]
	id, invalid := identityOf(argument)
	if invalid != nil {
		return refuse(invalid)
	}
	found, err := h.Show(ctx, argument, id)
	if err != nil {
		return ui.Emit(c.JSON, showResult{}, ui.Problems(readError(err, ui.Rel)))
	}
	s := found.Site
	if !c.JSON {
		_, _ = fmt.Fprintf(ui.Stdout, "mutant %s → %s in %s, %s\n  before: %s\n  after:  %s\n", Quote(s.Original), Quote(s.Replacement), found.Function,
			SiteAt(id.File, s.Line, s.Column), found.LineText, found.MutatedText)
	}
	return ui.Emit(c.JSON, showResult{Mutant: &shownJSON{Identity: argument, File: id.File, Line: s.Line, Column: s.Column, Function: found.Function,
		Original: s.Original, Replacement: s.Replacement, LineText: found.LineText, MutatedText: found.MutatedText}}, nil)
}
