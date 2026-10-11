package main

import (
	"context"
	"fmt"

	"github.com/donvargax/kong"

	"github.com/donvargax/itos-cc/internal/cli"
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
	Units units.CLI `cmd:"" help:"list the functions and methods every tool measures"`
}

// parserOf makes parsers of c, one for each parse, as cli.Parse asks:
// itos-cc's name, its flags held by internal/cli's Flags, help printed by
// internal/cli's Help, and the bindings each Run is handed. kong never
// exits the process: help is printed before it parses, and a failure is
// returned.
func parserOf(ctx context.Context, c *commandLine, ui *cli.UI) func() (*kong.Kong, error) {
	return func() (*kong.Kong, error) {
		*c = commandLine{}
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
			kong.Bind(ui, units.Handler{Tree: workdir.Tree{}}),
		)...)
	}
}

// moved are the moved commands, as kong's model holds them, by name.
func moved() map[string]*kong.Node {
	parser, err := parserOf(context.Background(), &commandLine{}, stdUI())()
	if err != nil {
		// The model is this file's: one kong refuses is a bug, which every
		// run of a test meets.
		panic(err)
	}
	all := map[string]*kong.Node{}
	for _, node := range parser.Model.Children {
		all[node.Name] = node
	}
	return all
}

// runMoved runs args, a moved command's name and the rest of its command
// line, and returns its exit code. Help, -h or --help before any --, wins
// over anything else the line holds; a line kong cannot parse is a usage
// problem, with --json the failure's object.
//
// No interrupt cancels ctx yet: Ctrl-C ends the command by the system's
// default action, as it ends every command until one reports an interrupt
// (docs/CLI.md).
func runMoved(args []string) int {
	line := cli.Read(args)
	if line.Help {
		return movedHelp(args[0])
	}
	ui := stdUI()
	var c commandLine
	parsed, err := cli.Parse(parserOf(context.Background(), &c, ui), args)
	if err != nil {
		return ui.Emit(line.JSON, nil, []*cli.Problem{cli.ParseProblem(err, args)})
	}
	switch name := parsed.Selected().Name; name {
	case "units":
		return callSafely(ui, parsed, name, c.Units.Run, c.Units.JSON)
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
// printing it from kong's model, and returns 0.
func movedHelp(name string) int {
	ui := stdUI()
	parser, err := parserOf(context.Background(), &commandLine{}, ui)()
	if err == nil {
		var traced *kong.Context
		if traced, err = kong.Trace(parser, []string{name}); err == nil {
			err = traced.PrintUsage(false)
		}
	}
	if err != nil {
		return ui.Fail(err, false)
	}
	return 0
}
