package main

import (
	"context"
	"fmt"

	"github.com/donvargax/kong"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/dry"
	"github.com/donvargax/itos-cc/internal/scrap"
	"github.com/donvargax/itos-cc/internal/units"
	"github.com/donvargax/itos-cc/internal/workdir"
)

// The commands moved onto kong (T-32), each a slice of its own: kong parses
// their command lines, internal/cli reads them and reports their failures
// and help as every command's, and each runs as its slice's Run, handed the
// context, the UI and its handler through kong's bindings. run routes a
// moved command here and every other to the hand parser (cli.go), until it
// moves too.

// commandLine is the moved commands' command line: one field each, its
// slice's kong struct, its help the line usage lists it by.
type commandLine struct {
	Dry   dry.CLI   `cmd:"" help:"functions similar enough to be duplicates"`
	Scrap scrap.CLI `cmd:"" help:"test-code structure: which tests to leave, table-drive, refactor, or split"`
	Units units.CLI `cmd:"" help:"list the functions and methods every tool measures"`
}

// parserOf is the parser of c, which run builds once, as it parses one
// command line at most (cli.Parse): itos-cc's name, its flags held by
// internal/cli's Flags, help printed by internal/cli's Help, and the
// bindings each Run is handed, ctx among them. The handlers of units and
// dry are not among them: they hold what itos-cc.yaml excludes, which
// runMoved reads once the line is parsed, so a file that cannot be read
// never breaks help. kong never exits the process: help is printed before
// it parses, and a failure is returned. A model kong refuses is the error
// returned, a bug run reports as an internal failure.
func parserOf(ctx context.Context, c *commandLine, ui *cli.UI) (*kong.Kong, error) {
	return kong.New(c, append(cli.Flags(),
		kong.Name("itos-cc"),
		kong.Help(cli.Help),
		kong.Writers(ui.Stdout, ui.Stderr),
		kong.Exit(func(int) {}),
		kong.PostBuild(func(k *kong.Kong) error {
			k.Model.HelpFlag.Help = "print this help"
			return nil
		}),
		kong.BindTo(ctx, (*context.Context)(nil)),
		kong.Bind(ui, scrap.Handler{Tree: workdir.Tree{}, Surroundings: workdir.Surroundings{}, Store: snapshots, Name: ui.Rel}),
	)...)
}

// moved are the moved commands, as parser's model holds them, by name.
func moved(parser *kong.Kong) map[string]*kong.Node {
	all := map[string]*kong.Node{}
	for _, node := range parser.Model.Children {
		all[node.Name] = node
	}
	return all
}

// runMoved runs args, a moved command's name and the rest of its command
// line, with parser, which fills c, and returns its exit code. Help, -h or
// --help before any --, wins over anything else the line holds; a line kong
// cannot parse is a usage problem, with --json the failure's object. Once
// the line is parsed, units and dry read itos-cc.yaml for the sources it
// excludes, bound into their handlers: a file that cannot be read is
// config.invalid. scrap reads only tests, which nothing excludes, so it
// never reads the file.
//
// No interrupt cancels the context parser binds yet: Ctrl-C ends the
// command by the system's default action, as it ends every command until
// one reports an interrupt (docs/CLI.md).
func runMoved(parser *kong.Kong, c *commandLine, ui *cli.UI, args []string) int {
	line := cli.Read(args)
	if line.Help {
		return movedHelp(parser, ui, args[0])
	}
	parsed, err := cli.Parse(parser, args)
	if err != nil {
		return ui.Emit(line.JSON, nil, []*cli.Problem{cli.ParseProblem(err, args)})
	}
	switch name := parsed.Selected().Name; name {
	case "dry", "units":
		exclude, err := exclusion()
		if err != nil {
			return ui.Fail(err, line.JSON)
		}
		parsed.Bind(units.Handler{Tree: workdir.Tree{}, Surroundings: workdir.Surroundings{}, Exclude: exclude},
			dry.Handler{Tree: workdir.Tree{}, Surroundings: workdir.Surroundings{}, Store: snapshots, Name: ui.Rel, Exclude: exclude})
		if name == "units" {
			return callSafely(ui, parsed, name, c.Units.Run, c.Units.JSON)
		}
		return callSafely(ui, parsed, name, c.Dry.Run, c.Dry.JSON)
	case "scrap":
		return callSafely(ui, parsed, name, c.Scrap.Run, c.Scrap.JSON)
	}
	// A command kong took that has no case here: a bug.
	return ui.Fail(fmt.Errorf("no case runs the command %q", parsed.Command()), line.JSON)
}

// callSafely calls run, a moved command's Run, with kong's bindings, and
// returns its exit code. A panic is an internal error, as it is in a hand
// parsed command (runSafely), so even a bug ends in the --json object and
// exit 70.
func callSafely(ui *cli.UI, parsed *kong.Context, name string, run any, asJSON bool) (code int) {
	defer func() {
		if r := recover(); r != nil {
			code = ui.Fail(fmt.Errorf("%s panicked: %v", name, r), asJSON)
		}
	}()
	out, err := parsed.Call(run)
	if err != nil {
		// Run asks for something nothing bound: a bug.
		return ui.Fail(err, asJSON)
	}
	return out[0].(int)
}

// movedHelp prints the help of the moved command name, internal/cli's Help
// printing it from parser's model, and returns 0.
func movedHelp(parser *kong.Kong, ui *cli.UI, name string) int {
	traced, err := kong.Trace(parser, []string{name})
	if err == nil {
		err = traced.PrintUsage(false)
	}
	if err != nil {
		return ui.Fail(err, false)
	}
	return 0
}
