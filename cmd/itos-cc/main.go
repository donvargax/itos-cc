// Command itos-cc measures code quality in TypeScript, Python, Kotlin, and Go.
package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

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
	run: func(in *invocation) (any, error) {
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
	for _, c := range []*command{crapCommand, dryCommand, mutationGroup, scrapCommand, serveCommand, unitsCommand, versionCommand} {
		commands[c.name] = c
	}
}

func usage() string {
	var b strings.Builder
	b.WriteString("usage: itos-cc <command> [options] [path ...]\n\nCommands:\n")
	for _, name := range sortedNames(commands) {
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

func main() {
	os.Exit(run(os.Args[1:]))
}

// classifiers are each domain's, which turn its sealed failures into
// problems for internal/cli: the sample count slice's, for the counting
// domain, until it is wired as a command or dropped. A problem itos-cc's
// commands report is its own and needs none.
var classifiers = []cli.Classifier{count.Classify}

// stdUI is the UI of this process: its standard streams as they are when it
// is called, so a test that swaps os.Stdout reads what it prints.
func stdUI() *cli.UI {
	return &cli.UI{In: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Classifiers: classifiers}
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

// run dispatches args to a command. --json, -h, --help, and --version may
// come before the command too.
func run(args []string) int {
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
			fmt.Print(usage())
			return 0
		case "--version":
			args = append([]string{"version"}, args...)
		default:
			p := cli.NewProblem(cli.CodeUsage, "flags.unknown", fmt.Sprintf("itos-cc takes no flag %s before a command", a),
				"Put a command's flags after its name; run 'itos-cc --help' for the commands.").With("flag", a)
			return stdUI().Emit(asJSON, nil, []*cli.Problem{p})
		}
	}
	if len(args) == 0 {
		if asJSON {
			p := cli.NewProblem(cli.CodeUsage, "command.missing", "no command given", "Name a command; run 'itos-cc --help' for them.")
			return stdUI().Emit(true, nil, []*cli.Problem{p})
		}
		fmt.Print(usage())
		return 0
	}
	name, rest := args[0], args[1:]
	if asJSON {
		rest = append(rest, "--json")
	}
	if name == "help" {
		return help(rest, asJSON)
	}
	cmd, ok := commands[name]
	if !ok {
		return stdUI().Emit(asJSON, nil, []*cli.Problem{unknownCommand(name)})
	}
	if cmd.subs != nil {
		return runGroup(cmd, rest)
	}
	return execute(cmd, rest)
}

// runGroup dispatches args to a subcommand of group g. Alone, or with -h or
// --help before a subcommand, it prints the group's help; with --json and
// no subcommand it is a usage error, as itos-cc --json alone is.
func runGroup(g *command, args []string) int {
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
	return execute(sub, args[1:])
}

// help prints the top-level help, or with a command name that command's.
func help(args []string, asJSON bool) int {
	var topics []string
	for _, a := range args {
		if a != "--json" {
			topics = append(topics, a)
		}
	}
	if len(topics) == 0 {
		fmt.Print(usage())
		return 0
	}
	cmd := commands[topics[0]]
	if cmd == nil {
		return stdUI().Emit(asJSON, nil, []*cli.Problem{unknownCommand(topics[0])})
	}
	// A group takes one more topic, its subcommand.
	most := 1
	if cmd.subs != nil {
		most = 2
	}
	switch {
	case len(topics) > most:
		return stdUI().Emit(asJSON, nil, []*cli.Problem{cli.NewProblem(cli.CodeUsage, "args.unexpected", fmt.Sprintf("help takes one command, and was given %q too", topics[most]),
			"Run 'itos-cc help <command>', or 'itos-cc help <group> <command>'.").With("argument", topics[most])})
	case len(topics) == 2 && cmd.sub(topics[1]) == nil:
		return stdUI().Emit(asJSON, nil, []*cli.Problem{unknownSubcommand(cmd, topics[1])})
	case len(topics) == 2:
		fmt.Print(cmd.sub(topics[1]).help())
	case cmd.subs != nil:
		fmt.Print(cmd.groupHelp())
	default:
		fmt.Print(cmd.help())
	}
	return 0
}

func unknownCommand(name string) *cli.Problem {
	p := cli.NewProblem(cli.CodeUsage, "command.unknown", fmt.Sprintf("there is no command %q", name), "Run 'itos-cc --help' for the commands.")
	if guess := closest(name, sortedNames(commands)); guess != "" {
		p.Fix = fmt.Sprintf("Did you mean '%s'? Run 'itos-cc --help' for the commands.", guess)
	}
	return p.With("command", name)
}

// unknownSubcommand is the problem of a subcommand that group g lacks: it
// names g's subcommands, and the one meant when it can guess it.
func unknownSubcommand(g *command, name string) *cli.Problem {
	full := g.name + " " + name
	fix := fmt.Sprintf("The subcommands of %s are %s. Run 'itos-cc %s --help' for them.", g.name, andList(g.subNames()), g.name)
	if guess := closest(name, g.subNames()); guess != "" {
		fix = fmt.Sprintf("Did you mean '%s %s'? ", g.name, guess) + fix
	}
	return cli.NewProblem(cli.CodeUsage, "command.unknown", fmt.Sprintf("there is no command %q", full), fix).With("command", full)
}

// leaves is every command that runs, a group's subcommands in its place.
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
