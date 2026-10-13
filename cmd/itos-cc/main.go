// Command itos-cc measures code quality in TypeScript, Python, Kotlin, and Go.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/donvargax/kong"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/count"
)

// version is set at release time with -ldflags '-X main.version=…'. A binary
// built by `go install …@<version>` reports the module version instead.
var version = "dev"

func versionString() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

var versionCommand = &command{
	name:     "version",
	summary:  "print the version",
	synopsis: "[options]",
	about:    "Prints \"itos-cc <version>\". --version is the same.",
	json:     `"version"`,
	exits: []exitDoc{
		{0, "success"},
		{2, "a usage error: a bad flag or an argument"},
	},
	examples: []string{"itos-cc version", "itos-cc --version --json"},
	run: func(_ context.Context, in *invocation) (any, error) {
		if len(in.args) > 0 {
			return nil, cli.NewProblem(cli.CodeUsage, "args.unexpected", fmt.Sprintf("version takes no arguments, and was given %q", in.args[0]),
				"Run 'itos-cc version' alone.").With("argument", in.args[0])
		}
		if !in.json {
			fmt.Println("itos-cc", versionString())
		}
		return struct {
			Version string `json:"version"`
		}{versionString()}, nil
	},
}

var commands = map[string]*command{}

func init() {
	for _, c := range []*command{crapCommand, mutationGroup, serveCommand, versionCommand} {
		commands[c.name] = c
	}
}

// commandNames are the names of every command, the moved ones' among them,
// as parser's model holds them, sorted.
func commandNames(parser *kong.Kong) []string {
	names := sortedNames(commands)
	for name := range moved(parser) {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// usage is the top-level help, the moved commands' lines from parser's
// model.
func usage(parser *kong.Kong) string {
	var b strings.Builder
	b.WriteString("usage: itos-cc <command> [options] [path ...]\n\nCommands:\n")
	nodes := moved(parser)
	for _, name := range commandNames(parser) {
		if node := nodes[name]; node != nil {
			fmt.Fprintf(&b, "  %-15s %s\n", name, node.Help)
			continue
		}
		c := commands[name]
		if c.subs == nil {
			fmt.Fprintf(&b, "  %-15s %s\n", name, c.summary)
		}
		for _, sub := range c.subs {
			fmt.Fprintf(&b, "  %-15s %s\n", sub.name, sub.summary)
		}
	}
	b.WriteString(`
Paths are files, directories, or fragments of a path under the working
directory. Without paths the working directory is analyzed.

Options, for every command and in any position:
  --json       print one JSON object on stdout: {"schema": 1, "ok": true|false,
               …, "problems": [{"rule", "message", "fix", …}]}
  -h, --help   print help; 'itos-cc help <command>' for a command's, such
               as 'itos-cc help crap' or 'itos-cc help mutation run'
  --version    print the version

Exit codes, for every command (each command's help says when):
  0   success
  1   a check said no: a mutant survived, a function is over a threshold
  2   a usage or config error
  3   the environment lacks something: a tool, a report, a git repository
  70  an internal error itos-cc could not classify; please report it
  75  a temporary failure: the same command may pass when run again

Scripts read the exit code and --json, less "message" and "fix"; plain
output is for people and may change in any release. See docs/CLI.md.

Examples:
  itos-cc crap --top 20
  itos-cc mutation run --changed --json

Report issues at ` + cli.Issues + "\n")
	return b.String()
}

// main makes the program's one context, which every command runs under,
// and gives run this process's standard streams and adapters. No interrupt
// cancels the context yet (docs/CLI.md). It alone exits.
func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr, diskAdapters(os.Stderr)))
}

// classifiers are each domain's, which turn its sealed failures into
// problems for internal/cli: the selection domain's, which every command
// selects its files through, the snapshot domain's, which the commands read
// and write .metrics/ through, the coverage domain's, which crap and
// mutation measure coverage through, and the sample count slice's, for the
// counting domain, until it is wired as a command or dropped. A problem
// itos-cc's commands report is its own and needs none.
var classifiers = []cli.Classifier{classifySelection, classifySnapshot, classifyCoverage, count.Classify}

// stdUI is the UI of this process, which the hand-parsed commands report
// through until they move: its standard streams as they are when it is
// called.
func stdUI() *cli.UI {
	dir, _ := os.Getwd()
	return &cli.UI{In: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Classifiers: classifiers, Dir: dir}
}

// init makes this executable the one go test runs each test binary through
// for Go coverage, and runs a test binary when go test does. An init rather
// than main, so the test binary of this package does the same and its tests
// run coverage as itos-cc does.
func init() {
	if len(os.Args) > 1 && os.Args[1] == coverage.ExecArg {
		os.Exit(coverage.ExecTest(os.Args[2:]))
	}
	coverage.Executable, _ = os.Executable()
}

// run dispatches args to a command, run under ctx. --json, -h, --help, and
// --version may come before the command too. The top-level help and
// problems and the moved commands read stdin and write stdout and stderr,
// and the moved commands reach the world through a; the hand-parsed ones
// keep this process's streams and disk until they move, but for crap's
// coverage, measured through a's. It builds the moved
// commands' parser once, which a run reads their names and help from and
// parses their command line with; a model kong refuses is a bug, an
// internal failure.
func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, a adapters) int {
	ui := &cli.UI{In: stdin, Stdout: stdout, Stderr: stderr, Classifiers: classifiers, Dir: a.Dir}
	var c commandLine
	parser, err := parserOf(ctx, &c, ui, a)
	if err != nil {
		return ui.Fail(err, cli.Read(args).JSON)
	}
	var leading []string
	for len(args) > 0 && strings.HasPrefix(args[0], "-") && args[0] != "--" {
		leading, args = append(leading, args[0]), args[1:]
	}
	asJSON := false
	for _, a := range leading {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprint(ui.Stdout, usage(parser))
			return 0
		case "--version":
			args = append([]string{"version"}, args...)
		default:
			p := cli.NewProblem(cli.CodeUsage, "flags.unknown", fmt.Sprintf("itos-cc takes no flag %s before a command", a),
				"Put a command's flags after its name; run 'itos-cc --help' for the commands.").With("flag", a)
			return ui.Emit(asJSON, nil, []*cli.Problem{p})
		}
	}
	if len(args) == 0 {
		if asJSON {
			p := cli.NewProblem(cli.CodeUsage, "command.missing", "no command given", "Name a command; run 'itos-cc --help' for them.")
			return ui.Emit(true, nil, []*cli.Problem{p})
		}
		fmt.Fprint(ui.Stdout, usage(parser))
		return 0
	}
	name, rest := args[0], args[1:]
	if asJSON {
		rest = append(rest, "--json")
	}
	if name == "help" {
		return help(parser, ui, rest, asJSON)
	}
	if moved(parser)[name] != nil {
		return runMoved(ctx, parser, &c, ui, a, append([]string{name}, rest...))
	}
	cmd, ok := commands[name]
	if !ok {
		return ui.Emit(asJSON, nil, []*cli.Problem{unknownCommand(parser, name)})
	}
	if cmd.subs != nil {
		return runGroup(ctx, cmd, rest, a)
	}
	return execute(ctx, cmd, rest, a)
}

// runGroup dispatches args to a subcommand of group g, run under ctx with
// the adapters a.
// Alone, or with -h or --help before a subcommand, it prints the group's
// help; with --json and no subcommand it is a usage error, as itos-cc
// --json alone is.
func runGroup(ctx context.Context, g *command, args []string, a adapters) int {
	asJSON := false
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "--json" {
			asJSON = true
		}
	}
	for len(args) > 0 && strings.HasPrefix(args[0], "-") && args[0] != "--" {
		switch a := args[0]; a {
		case "--json":
		case "-h", "--help":
			fmt.Print(g.groupHelp())
			return 0
		default:
			p := cli.NewProblem(cli.CodeUsage, "flags.unknown", fmt.Sprintf("%s takes no flag %s before a subcommand", g.name, a),
				fmt.Sprintf("Put a subcommand's flags after its name; run 'itos-cc %s --help' for the subcommands.", g.name)).With("flag", a)
			return stdUI().Emit(asJSON, nil, []*cli.Problem{p})
		}
		args = args[1:]
	}
	if len(args) == 0 {
		if asJSON {
			p := cli.NewProblem(cli.CodeUsage, "command.missing", fmt.Sprintf("%s needs a subcommand", g.name),
				fmt.Sprintf("Name one of %s; run 'itos-cc %s --help' for them.", andList(g.subNames()), g.name))
			return stdUI().Emit(true, nil, []*cli.Problem{p})
		}
		fmt.Print(g.groupHelp())
		return 0
	}
	sub := g.sub(args[0])
	if sub == nil {
		return stdUI().Emit(asJSON, nil, []*cli.Problem{unknownSubcommand(g, args[0])})
	}
	return execute(ctx, sub, args[1:], a)
}

// help prints on ui's stdout the top-level help, or with a command name
// that command's, the moved commands' from parser's model, and reports
// through ui.
func help(parser *kong.Kong, ui *cli.UI, args []string, asJSON bool) int {
	var topics []string
	for _, a := range args {
		if a != "--json" {
			topics = append(topics, a)
		}
	}
	if len(topics) == 0 {
		fmt.Fprint(ui.Stdout, usage(parser))
		return 0
	}
	if moved(parser)[topics[0]] != nil {
		if len(topics) > 1 {
			return ui.Emit(asJSON, nil, []*cli.Problem{helpTakesOne(topics[1])})
		}
		return movedHelp(parser, ui, topics[0])
	}
	cmd := commands[topics[0]]
	if cmd == nil {
		return ui.Emit(asJSON, nil, []*cli.Problem{unknownCommand(parser, topics[0])})
	}
	// A group takes one more topic, its subcommand.
	most := 1
	if cmd.subs != nil {
		most = 2
	}
	switch {
	case len(topics) > most:
		return ui.Emit(asJSON, nil, []*cli.Problem{helpTakesOne(topics[most])})
	case len(topics) == 2 && cmd.sub(topics[1]) == nil:
		return ui.Emit(asJSON, nil, []*cli.Problem{unknownSubcommand(cmd, topics[1])})
	case len(topics) == 2:
		fmt.Fprint(ui.Stdout, cmd.sub(topics[1]).help())
	case cmd.subs != nil:
		fmt.Fprint(ui.Stdout, cmd.groupHelp())
	default:
		fmt.Fprint(ui.Stdout, cmd.help())
	}
	return 0
}

// helpTakesOne is the problem of help given topic beyond the command it
// takes.
func helpTakesOne(topic string) *cli.Problem {
	return cli.NewProblem(cli.CodeUsage, "args.unexpected", fmt.Sprintf("help takes one command, and was given %q too", topic),
		"Run 'itos-cc help <command>', or 'itos-cc help <group> <command>'.").With("argument", topic)
}

// unknownCommand is the problem of a command itos-cc lacks, naming the one
// meant, the moved commands among them (parser's), when it can guess it.
func unknownCommand(parser *kong.Kong, name string) *cli.Problem {
	p := cli.NewProblem(cli.CodeUsage, "command.unknown", fmt.Sprintf("there is no command %q", name), "Run 'itos-cc --help' for the commands.")
	if guess := cli.Closest(name, commandNames(parser)); guess != "" {
		p.Fix = fmt.Sprintf("Did you mean '%s'? Run 'itos-cc --help' for the commands.", guess)
	}
	return p.With("command", name)
}

// unknownSubcommand is the problem of a subcommand that group g lacks: it
// names g's subcommands, and the one meant when it can guess it.
func unknownSubcommand(g *command, name string) *cli.Problem {
	full := g.name + " " + name
	fix := fmt.Sprintf("The subcommands of %s are %s. Run 'itos-cc %s --help' for them.", g.name, andList(g.subNames()), g.name)
	if guess := cli.Closest(name, g.subNames()); guess != "" {
		fix = fmt.Sprintf("Did you mean '%s %s'? ", g.name, guess) + fix
	}
	return cli.NewProblem(cli.CodeUsage, "command.unknown", fmt.Sprintf("there is no command %q", full), fix).With("command", full)
}

// leaves is every hand-parsed command that runs, a group's subcommands in
// its place.
func leaves() []*command {
	var all []*command
	for _, name := range sortedNames(commands) {
		if c := commands[name]; c.subs != nil {
			all = append(all, c.subs...)
		} else {
			all = append(all, c)
		}
	}
	return all
}
