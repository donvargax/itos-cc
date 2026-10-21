// Package units is the units command, a slice named for it: its kong struct
// and Run are the UI, and its Query and Handler the application.
//
// Run assembles the Query from the command line, the one copy of a command
// line's values, and hands it to Handle with the context cmd made. Handle
// only wires: the selection domain chooses the files through the Tree port
// it was given and parses each one, read through the same port and parsed
// by lang, which asks the Surroundings port it was given what surrounds the
// file and names its units. Run writes what came back, its paths from the
// working directory. The slice reads no file itself. A failure is the
// selection domain's, which cmd's classifier for that domain turns into a
// problem, or lang's, an internal one; Run reports it through the UI.
//
// Command declares the command to cmd, which assembles it into its one
// model (ADR-0051), and Ports are the ports its handler is built over,
// which cmd's adapters satisfy.
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

// Command is the units command as cmd assembles it: its name, the line the
// top-level help lists it by, and a fresh command line for each parser.
var Command = cli.Declaration{
	Name: "units",
	Help: "list the functions and methods every tool measures",
	New:  func() any { return &CLI{} },
}

// CLI is the units command's command line: the files it selects, as every
// command selects them (cli.Selection, first), whether it lists their
// tests instead, and whether it prints lines or the one object a script
// reads.
type CLI struct {
	cli.Selection
	Tests bool `help:"list test code instead of production code"`
	JSON  bool `name:"json" help:"print one JSON object on stdout instead of text (shape below)"`
}

// Contract is what units' help says beyond its flags: what it lists, the
// --json object a script reads, and every exit code it can return. A
// scenario checks the help and a script reads it.
func (CLI) Contract() cli.Contract {
	return cli.Contract{
		About: `
Lists the functions and methods of production code, or of test code with
--tests: the units crap, dry, and mutate measure. The sources itos-cc.yaml's
exclude names are left out, however they are chosen; tests never are.`,
		JSON: `"units": [{"file", "language", "namespace", "name", "kind", "private",
   "start_line", "end_line"}]`,
		Rules: []string{
			"config.invalid  itos-cc.yaml cannot be read: file, and key for an exclude glob",
		},
		Exits: []cli.Exit{
			{Code: 0, Meaning: "success"},
			{Code: cli.CodeUsage, Meaning: "a usage or config error: a bad flag or path, or an itos-cc.yaml that cannot be read"},
			{Code: cli.CodeEnvironment, Meaning: "--changed outside a git repository"},
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
// and reads files through, what surrounds those files, which lang asks
// about to name them, and the sources the project's settings exclude, each
// given by cmd, which chooses the adapters behind them and reads the
// settings. No value of a command line is read here.
type Handler struct {
	Tree         port.Tree
	Surroundings langport.Surroundings
	Exclude      selection.Exclusion
}

// Ports are what the units command reaches the world through, which cmd's
// adapters give it: the working tree it selects and reads files through,
// what surrounds those files, and the selection engine's settings, whose
// exclude cmd reads, under the context of the Run that asks for the
// handler, into the Exclusion NewHandler is given.
type Ports interface {
	SelectionTree() port.Tree
	LangSurroundings() langport.Surroundings
	SelectionSettings() port.SettingsReader
}

// NewHandler is the handler over p's tree and surroundings, leaving out the
// sources exclude names.
func NewHandler(p Ports, exclude selection.Exclusion) Handler {
	return Handler{Tree: p.SelectionTree(), Surroundings: p.LangSurroundings(), Exclude: exclude}
}

// Found is what Handle found: the units listed, and Excluded, the count of
// the sources the selection left out by the handler's Exclude when it
// listed production code; 0 with tests, which nothing excludes.
type Found struct {
	Units    []lang.Unit
	Excluded int
}

// Handle lists the units of the files q selects, passing ctx on to the
// domain, lang and the ports: the selection and each file's parse are the
// domain's, the sources the handler's Exclude names left out, and naming a
// file's units lang's. On a failure it returns the units listed before
// it.
func (h Handler) Handle(ctx context.Context, q Query) (Found, error) {
	files, err := selection.Select(ctx, h.Tree, q.Paths, q.Changed, h.Exclude)
	if err != nil {
		return Found{}, err
	}
	list, found := files.Sources, Found{Excluded: files.Excluded}
	if q.Tests {
		list, found.Excluded = files.Tests, 0
	}
	for _, path := range list {
		f, err := selection.Parse(ctx, h.Tree, h.Surroundings, path)
		if err != nil {
			return found, err
		}
		found.Units = append(found.Units, f.Detached()...)
		f.Close()
	}
	return found, nil
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
	for _, u := range found.Units {
		u.File = ui.Rel(u.File)
		result.Units = append(result.Units, u)
	}
	if err != nil {
		return ui.Emit(c.JSON, result, ui.Problems(err))
	}
	if found.Excluded > 0 {
		ui.Line(fmt.Sprintf("%d sources left out by exclude in itos-cc.yaml", found.Excluded))
	}
	if !c.JSON {
		for _, u := range result.Units {
			_, _ = fmt.Fprintf(ui.Stdout, "%s:%d-%d  %s#%s  %s\n", u.File, u.StartLine, u.EndLine, u.Namespace, u.Name, u.Kind)
		}
	}
	return ui.Emit(c.JSON, result, nil)
}
