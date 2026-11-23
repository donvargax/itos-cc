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
	"encoding/json"
	"fmt"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/lang"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	"github.com/donvargax/itos-cc/internal/past"
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
	cli.Past
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
exclude names are left out, however they are chosen; tests never are.

With --at COMMIT the command measures that commit's tree, read from git's
objects, as if it were checked out: the working tree is never read, nothing
is written to .metrics/, and the commit's own itos-cc.yaml is the settings.
Each result is kept per commit in the git common directory, shared by every
worktree.`,
		JSON: `"units": [{"file", "language", "namespace", "name", "kind", "private",
   "start_line", "end_line"}]`,
		Rules: []string{
			"config.invalid  itos-cc.yaml cannot be read: file, and key for an exclude glob",
			"at.bad-rev      --at names no commit of the repository: rev",
			"at.no-git       --at outside a git repository",
			"flags.conflict  --at with --changed: flag",
		},
		Exits: []cli.Exit{
			{Code: 0, Meaning: "success"},
			{Code: cli.CodeUsage, Meaning: "a usage or config error: a bad flag, path or revision, or an itos-cc.yaml that cannot be read"},
			{Code: cli.CodeEnvironment, Meaning: "--changed or --at outside a git repository"},
		},
		Examples: []string{
			"itos-cc units --at HEAD~10",
			"itos-cc units src/billing",
			"itos-cc units --tests --json",
		},
	}
}

// Query is what the units command asks: the units of the files paths and
// changed select, of their tests instead with tests, of the commit at
// instead of the working tree when at names one.
type Query struct {
	Paths   []string
	Changed bool
	Tests   bool
	At      string
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
	// Past opens the commit a query names with At, and Version is the
	// itos-cc that measures it, which a kept result is keyed by.
	Past    past.Source
	Version string
}

// Ports are what the units command reaches the world through, which cmd's
// adapters give it: the working tree it selects and reads files through,
// what surrounds those files, and the sources the project's settings
// exclude, which cmd reads under the context of the Run that asks for the
// handler and whose failure it words as a person reads it.
type Ports interface {
	SelectionTree() port.Tree
	LangSurroundings() langport.Surroundings
	SelectionExclusion(ctx context.Context) (selection.Exclusion, error)
	PastCommits() past.Source
	BuildVersion() string
}

// NewHandler is the handler over p's tree and surroundings, leaving out the
// sources p's settings exclude, read under ctx; their failure is returned
// as p gave it, never wrapped, so the problem cmd worded is the one the
// person reads.
func NewHandler(ctx context.Context, p Ports) (Handler, error) {
	exclude, err := p.SelectionExclusion(ctx)
	if err != nil {
		return Handler{}, err
	}
	return Handler{Tree: p.SelectionTree(), Surroundings: p.LangSurroundings(), Exclude: exclude, Past: p.PastCommits(), Version: p.BuildVersion()}, nil
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
	if q.At != "" {
		return h.handleAt(ctx, q)
	}
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

// handleAt lists the units of the commit q.At names, its tree read through
// the ports the commit gives and its own itos-cc.yaml leaving sources out,
// or the list an earlier run kept for the same question.
func (h Handler) handleAt(ctx context.Context, q Query) (Found, error) {
	commit, err := h.Past.Open(ctx, q.At)
	if err != nil {
		return Found{}, err
	}
	at := Handler{Tree: commit.Tree, Surroundings: commit.Surroundings, Exclude: selection.Exclusion(commit.Settings)}
	q.At = ""
	return past.Kept(ctx, commit, Command.Name, commit.Key(q, h.Version),
		func() (Found, error) { return at.Handle(ctx, q) },
		func(f Found) ([]byte, error) {
			kept := keptFound{Excluded: f.Excluded, Units: []lang.Unit{}}
			for _, u := range f.Units {
				u.File = commit.Rel(u.File)
				kept.Units = append(kept.Units, u)
			}
			return json.Marshal(kept)
		},
		func(data []byte) (Found, error) {
			var kept keptFound
			if err := json.Unmarshal(data, &kept); err != nil {
				return Found{}, err
			}
			f := Found{Excluded: kept.Excluded}
			for _, u := range kept.Units {
				u.File = commit.Abs(u.File)
				f.Units = append(f.Units, u)
			}
			return f, nil
		})
}

// keptFound is what a run keeps of a commit's units: each unit's file named
// from the project root, never from where this worktree is.
type keptFound struct {
	Units    []lang.Unit `json:"units"`
	Excluded int         `json:"excluded"`
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
	result := listing{Units: []lang.Unit{}}
	if conflict := c.Conflict(c.Changed); conflict != nil {
		return ui.Emit(c.JSON, result, []*cli.Problem{conflict})
	}
	found, err := h.Handle(ctx, Query{Paths: c.Paths, Changed: c.Changed, Tests: c.Tests, At: c.At})
	for _, u := range found.Units {
		u.File = ui.Show(u.File).System()
		result.Units = append(result.Units, u)
	}
	if err != nil {
		return ui.Emit(c.JSON, result, ui.Problems(err))
	}
	if found.Excluded > 0 {
		ui.Line(fmt.Sprintf("%d sources left out by exclude in itos-cc.yaml", found.Excluded))
	}
	if !c.JSON {
		for _, u := range found.Units {
			_, _ = fmt.Fprintf(ui.Stdout, "%s:%d-%d  %s#%s  %s\n", ui.Show(u.File), u.StartLine, u.EndLine, u.Namespace, u.Name, u.Kind)
		}
	}
	return ui.Emit(c.JSON, result, nil)
}
