package main

import (
	"context"
	"fmt"
	"runtime"

	"github.com/donvargax/kong"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/crap"
	"github.com/donvargax/itos-cc/internal/dry"
	"github.com/donvargax/itos-cc/internal/mutation"
	"github.com/donvargax/itos-cc/internal/scrap"
	"github.com/donvargax/itos-cc/internal/units"
)

// Every command is a node of one kong model, which internal/cli's Dispatch
// reads a whole command line against: kong parses the command lines of the
// slices and of version, internal/cli reads them and reports their failures
// and help as every command's, and each runs as its Run, handed the context,
// the UI and its handler through kong's bindings. serve alone stays on its
// legacy handler and hand parser (cli.go, serve.go), its moves deferred
// while it leaves for a web app of its own: its node names it in the model,
// and dispatch hands its command line to that handler as it is.

// commandLine is the command line of every command: one field each, its
// kong struct, its help the line the top-level help lists it by. The fields
// are in their names' order, which the guess for an unknown command weighs
// a tie by, as the top-level help lists them.
type commandLine struct {
	Crap     crap.CLI     `cmd:"" help:"CRAP score per function: complexity × untested risk"`
	Dry      dry.CLI      `cmd:"" help:"functions similar enough to be duplicates"`
	Mutation mutation.CLI `cmd:"" help:"mutation testing: do the tests notice small changes?"`
	Scrap    scrap.CLI    `cmd:"" help:"test-code structure: which tests to leave, table-drive, refactor, or split"`
	Serve    legacyServe  `cmd:"" help:"live architecture graph and metrics over HTTP, for the viewer"`
	Units    units.CLI    `cmd:"" help:"list the functions and methods every tool measures"`
	Version  cli.Version  `cmd:"" help:"print the version"`
}

// legacyServe is serve's node in the model, which names it to the top-level
// help and the guess for an unknown command. kong never parses its command
// line: dispatch hands it whole to serve's legacy handler (serveCommand),
// whose own parser reads it and whose own help 'help serve' prints.
type legacyServe struct{}

// legacy are the commands still on the hand parser, by name: serve alone.
var legacy = map[string]*command{serveCommand.name: serveCommand}

// parserOf is the parser of c, which run builds once, as it parses one
// command line at most (cli.Parse): itos-cc's name, its flags held by
// internal/cli's Flags, help printed by internal/cli's Help, and the
// bindings each Run is handed, ctx and the UI among them, and providers
// (handlers) of every command's handler and of the version version prints.
// kong calls a provider only when the Run it calls asks for its value, so
// nothing a provider reads is read for help, a line kong refuses or another
// command. kong never exits the process: help is printed before it parses,
// and a failure is returned. A model kong refuses is the error returned, a
// bug run reports as an internal failure.
func parserOf(ctx context.Context, c *commandLine, ui *cli.UI, a adapters) (*kong.Kong, error) {
	return kong.New(c, append(append(cli.Flags(),
		kong.Name("itos-cc"),
		kong.Vars{"mutation_workers": fmt.Sprint(max(1, runtime.NumCPU()/2))},
		kong.Help(cli.Help),
		kong.Writers(ui.Stdout, ui.Stderr),
		kong.Exit(func(int) {}),
		kong.PostBuild(func(k *kong.Kong) error {
			k.Model.HelpFlag.Help = "print this help"
			return nil
		}),
		kong.BindTo(ctx, (*context.Context)(nil)),
		kong.Bind(ui),
	), handlers(ui, a)...)...)
}

// handlers are the providers of every command's handler, over a's ports,
// and of version's build, each called under the context of the Run that asks
// for it. The handlers of crap, units and dry hold the sources itos-cc.yaml
// excludes, which their providers read under ctx through the selection
// engine's settings, each provider reading them itself, so their failure is
// never wrapped as a dependent provider's: a file that cannot be read
// is config.invalid, the provider's error, which kong returns unwrapped from
// the call of that command's Run alone. scrap reads only tests, which nothing
// excludes, so it never reads the file, and version reads nothing but the
// build. mutation's handler is handed the flags its line gave as written
// (cli.Provided), read from the parsed line kong binds.
func handlers(ui *cli.UI, a adapters) []kong.Option {
	return []kong.Option{
		kong.BindToProvider(func(ctx context.Context) (crap.Handler, error) {
			exclude, err := exclusion(ctx, a.Selection)
			return crap.Handler{Tree: a.Tree, Surroundings: a.Surroundings, Store: a.Store, Coverage: a.Coverage, Name: ui.Rel, Exclude: exclude}, err
		}),
		kong.BindToProvider(func(ctx context.Context) (dry.Handler, error) {
			exclude, err := exclusion(ctx, a.Selection)
			return dry.Handler{Tree: a.Tree, Surroundings: a.Surroundings, Store: a.Store, Name: ui.Rel, Exclude: exclude}, err
		}),
		kong.BindToProvider(func() mutation.Handler {
			return mutation.Handler{Settings: a.Settings, Tree: a.Tree, Surroundings: a.Surroundings, Store: a.SnapshotReader, Catalog: a.Catalog, Presence: a.Presence, Status: a.SourceStatus, History: a.History, Toolchain: a.Toolchain, Fingerprints: a.Fingerprints, Reach: a.Reach, Workers: a.Workers, Exceptions: a.Exceptions, Dir: a.Dir,
				Snapshots: a.Snapshots, SourceEdits: a.SourceEdits, TestLister: a.TestLister, Coverage: a.MutationCoverage, Metrics: a.Store,
				Fresh: a.Fresh, FreshWorkers: a.FreshWorkers, Repository: a.Repository, PhysicalDir: a.PhysicalDir, Platform: a.Platform}
		}),
		kong.BindToProvider(func(parsed *kong.Context) mutation.Written { return mutation.Written(cli.Provided(parsed)) }),
		kong.BindToProvider(func() scrap.Handler {
			return scrap.Handler{Tree: a.Tree, Surroundings: a.Surroundings, Store: a.Store, Name: ui.Rel}
		}),
		kong.BindToProvider(func(ctx context.Context) (units.Handler, error) {
			exclude, err := exclusion(ctx, a.Selection)
			return units.Handler{Tree: a.Tree, Surroundings: a.Surroundings, Exclude: exclude}, err
		}),
		kong.BindToProvider(func() cli.Build { return cli.Build(versionString()) }),
	}
}

// commandNodes are the commands of parser's model, by name.
func commandNodes(parser *kong.Kong) map[string]*kong.Node {
	all := map[string]*kong.Node{}
	for _, node := range parser.Model.Children {
		all[node.Name] = node
	}
	return all
}

// help prints on ui's stdout the help of topics, a command and a group's
// subcommand after it, which Dispatch found in parser's model, and returns
// 0: serve's from its legacy handler, every other's from the model.
func help(parser *kong.Kong, ui *cli.UI, topics []string) int {
	if cmd := legacy[topics[0]]; cmd != nil {
		// serve's legacy help has always exited 0 whether or not stdout takes
		// it; failing as modelHelp does would change its exit code.
		_, _ = fmt.Fprint(ui.Stdout, cmd.help())
		return 0
	}
	return modelHelp(parser, ui, topics...)
}

// dispatch runs args, a command's name and the rest of its command line,
// with parser, and returns its exit code. serve's line goes whole to its
// legacy handler. A group, a command with subcommands, first admits the
// words before its subcommand (cli.GroupLine), its help or problem winning
// there. For every other command help, -h or --help before any --, wins
// over anything else the line holds; a line kong cannot parse is a usage
// problem, with --json the failure's object. Once the line is parsed, the
// command kong selected runs (runSelected).
//
// No interrupt cancels the context parser binds yet: Ctrl-C ends the
// command by the system's default action, as it ends every command until
// one reports an interrupt (docs/CLI.md).
func dispatch(ctx context.Context, parser *kong.Kong, ui *cli.UI, a adapters, args []string) int {
	if cmd := legacy[args[0]]; cmd != nil {
		return execute(ctx, cmd, args[1:], a)
	}
	line := cli.Read(args)
	topics := []string{args[0]}
	if group := commandNodes(parser)[args[0]]; group != nil && len(group.Children) > 0 {
		admitted, help, problem := cli.GroupLine(group, args[1:])
		if problem != nil {
			return ui.Emit(line.JSON, nil, []*cli.Problem{problem})
		}
		if help {
			return modelHelp(parser, ui, args[0])
		}
		args = admitted
		line = cli.Read(args)
		topics = append(topics, args[1])
	}
	if line.Help {
		return modelHelp(parser, ui, topics...)
	}
	parsed, err := cli.Parse(parser, args)
	if err != nil {
		return ui.Emit(line.JSON, nil, []*cli.Problem{cli.ParseProblem(err, args)})
	}
	return runSelected(ui, parsed)
}

// runSelected calls the Run of the command parsed selected, through
// callSafely, with what it asks for bound by parserOf, and returns its exit
// code; a failure that keeps it from being called, such as a handler's
// config.invalid, is reported as --json, read from the parsed command, asks.
func runSelected(ui *cli.UI, parsed *kong.Context) int {
	node := parsed.Selected()
	asJSON := jsonOf(parsed, node)
	run := node.Target.Addr().MethodByName("Run")
	if !run.IsValid() {
		// A command kong took that has no Run: a bug.
		return ui.Fail(fmt.Errorf("no Run runs the command %q", parsed.Command()), asJSON)
	}
	return callSafely(ui, parsed, node.Path(), run.Interface(), asJSON)
}

// jsonOf is whether parsed gave node, the command it selected, --json.
func jsonOf(parsed *kong.Context, node *kong.Node) bool {
	for _, flag := range node.Flags {
		if flag.Name == "json" {
			asJSON, _ := parsed.FlagValue(flag).(bool)
			return asJSON
		}
	}
	return false
}

// callSafely calls run, a command's Run, with kong's bindings, and returns
// its exit code. A panic is an internal error, as it is in serve's legacy
// handler (runSafely), so even a bug ends in the --json object and exit 100.
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

// modelHelp prints the help of topics, a command of parser's model and a
// group's subcommand after it, internal/cli's Help printing it from the
// model, and returns 0.
func modelHelp(parser *kong.Kong, ui *cli.UI, topics ...string) int {
	traced, err := kong.Trace(parser, topics)
	if err == nil {
		err = traced.PrintUsage(false)
	}
	if err != nil {
		return ui.Fail(err, false)
	}
	return 0
}
