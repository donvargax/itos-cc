// Package units is the units command, a slice named for it: its kong struct
// and Run are the UI, and its Query and Handler the application.
//
// Run assembles the Query from the command line, the one copy of a command
// line's values, and hands it to Handle with the context cmd made. Handle
// only wires: the selection domain chooses the files through the Tree port
// it was given, the same port reads each one, and lang names the units of
// what was read, asking the Surroundings port it was given what surrounds
// each file. Run writes what came back, its paths from the working
// directory. The slice reads no file itself. A failure is the selection
// domain's, which cmd's classifier for that domain turns into a problem, or
// lang's, an internal one; Run reports it through the UI.
package units

import (
	"context"
	"fmt"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/lang"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	"github.com/donvargax/itos-cc/internal/selection"
	"github.com/donvargax/itos-cc/internal/selection/port"
)

// CLI is the units command's command line: the files it selects, as every
// command selects them, whether it lists their tests instead, and whether
// it prints lines or the one object a script reads.
type CLI struct {
	Changed bool     `help:"only files git reports as added or modified"`
	Tests   bool     `help:"list test code instead of production code"`
	JSON    bool     `name:"json" help:"print one JSON object on stdout instead of text (shape below)"`
	Paths   []string `arg:"" optional:"" name:"path" help:"files, directories, or fragments of a path under the working directory"`
}

// Contract is what units' help says beyond its flags: what it lists, the
// --json object a script reads, and every exit code it can return. A
// scenario checks the help and a script reads it.
func (CLI) Contract() cli.Contract {
	return cli.Contract{
		About: `
Lists the functions and methods of production code, or of test code with
--tests: the units crap, dry, and mutate measure.`,
		JSON: `"units": [{"file", "language", "namespace", "name", "kind", "private",
   "start_line", "end_line"}]`,
		Exits: []cli.Exit{
			{Code: 0, Meaning: "success"},
			{Code: 2, Meaning: "a usage error: a bad flag or path"},
			{Code: 3, Meaning: "--changed outside a git repository"},
		},
		Examples: []string{
			"itos-cc units src/billing",
			"itos-cc units --tests --json",
		},
	}
}

// Query is what the units command asks: the units of the files paths and
// changed select, of their tests instead with tests.
type Query struct {
	Paths   []string
	Changed bool
	Tests   bool
}

// Handler is the units command's application: the working tree it selects
// and reads files through, and what surrounds those files, which lang asks
// about to name them, each given by cmd, which chooses the adapters behind
// them. No value of a command line is read here.
type Handler struct {
	Tree         port.Tree
	Surroundings langport.Surroundings
}

// Handle lists the units of the files q selects, passing ctx on to the
// domain, lang and the ports: the selection is the domain's, and naming a
// file's units lang's. On a failure it returns the units listed before it.
func (h Handler) Handle(ctx context.Context, q Query) ([]lang.Unit, error) {
	files, err := selection.Select(ctx, h.Tree, q.Paths, q.Changed)
	if err != nil {
		return nil, err
	}
	list := files.Sources
	if q.Tests {
		list = files.Tests
	}
	var all []lang.Unit
	for _, path := range list {
		src, failure := h.Tree.Read(ctx, path)
		if failure != nil {
			return all, failure
		}
		found, err := lang.UnitsOf(ctx, h.Surroundings, path, src)
		if err != nil {
			return all, err
		}
		all = append(all, found...)
	}
	return all, nil
}

// listing is the object --json prints beside schema and ok: the units, an
// empty list rather than none when there are none.
type listing struct {
	Units []lang.Unit `json:"units"`
}

// Run lists the units the command line selects, through h and with the
// context cmd made, each a line or all in the object, or reports the
// failure it met, with the units listed before it in the object, and
// returns the exit code the UI gives it.
func (c CLI) Run(ctx context.Context, ui *cli.UI, h Handler) int {
	found, err := h.Handle(ctx, Query{Paths: c.Paths, Changed: c.Changed, Tests: c.Tests})
	result := listing{Units: []lang.Unit{}}
	for _, u := range found {
		u.File = ui.Rel(u.File)
		result.Units = append(result.Units, u)
	}
	if err != nil {
		return ui.Emit(c.JSON, result, ui.Problems(err))
	}
	if !c.JSON {
		for _, u := range result.Units {
			_, _ = fmt.Fprintf(ui.Stdout, "%s:%d-%d  %s#%s  %s\n", u.File, u.StartLine, u.EndLine, u.Namespace, u.Name, u.Kind)
		}
	}
	return ui.Emit(c.JSON, result, nil)
}
