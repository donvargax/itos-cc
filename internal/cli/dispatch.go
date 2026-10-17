package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/donvargax/kong"
)

// The whole command line, before any command reads its own: the flags that
// may come before a command (--json, -h, --help and --version), the command
// it names or help, each read against kong's model of every command, so the
// top-level help, 'help <command>' and the guess for an unknown command know
// every command one way (docs/CLI.md, rules 3, 4, 7 and 9). Nothing here
// reads the configuration or runs a command.

// Route is what a whole command line asks for, as Dispatch reads it: a
// problem, the top-level help, a command's help, or a command to run.
type Route struct {
	// JSON is whether --json came before the command, which the route's
	// problem is printed by.
	JSON bool
	// Problem, when set, is the usage problem the line is.
	Problem *Problem
	// Usage is whether the line asks for the top-level help.
	Usage bool
	// Help, when set, is the command whose help 'help' asks for: its name,
	// and a group's subcommand after it, each known to the model.
	Help []string
	// Line, when set, is the command line of the command named: its name,
	// then the words after it, --json last when it came before the command.
	Line []string
}

// Dispatch reads args, a whole command line after the program's name,
// against app, kong's model of every command. The flags before a command are
// read in turn: --json asks for the object, -h or --help for the top-level
// help, at once, --version for the version command, before whatever command
// follows, and any other is a usage problem. No command is the top-level
// help, or with --json a usage problem; help is a command's help, or the top
// level's; any other word must name one of app's commands, and the one
// meant is guessed when it does not.
func Dispatch(app *kong.Application, args []string) Route {
	var leading []string
	for len(args) > 0 && strings.HasPrefix(args[0], "-") && args[0] != "--" {
		leading, args = append(leading, args[0]), args[1:]
	}
	var r Route
	for _, word := range leading {
		switch word {
		case "--json":
			r.JSON = true
		case "-h", "--help":
			return Route{JSON: r.JSON, Usage: true}
		case "--version":
			args = slices.Concat([]string{"version"}, args)
		default:
			r.Problem = NewProblem(CodeUsage, "flags.unknown", fmt.Sprintf("%s takes no flag %s before a command", app.Name, word),
				fmt.Sprintf("Put a command's flags after its name; run '%s --help' for the commands.", app.Name)).With("flag", word)
			return r
		}
	}
	if len(args) == 0 {
		if r.JSON {
			r.Problem = NewProblem(CodeUsage, "command.missing", "no command given", fmt.Sprintf("Name a command; run '%s --help' for them.", app.Name))
			return r
		}
		r.Usage = true
		return r
	}
	name, rest := args[0], slices.Clone(args[1:])
	if r.JSON {
		rest = append(rest, "--json")
	}
	if name == "help" {
		return helpRoute(app, rest, r.JSON)
	}
	if command(app.Node, name) == nil {
		r.Problem = UnknownCommand(app.Node, name)
		return r
	}
	r.Line = append([]string{name}, rest...)
	return r
}

// helpRoute is the route of 'help' followed by args: the top-level help with
// no command named, else that command's, a group taking one of its
// subcommands after it. A command or subcommand app lacks, or a word past
// the ones help takes, is a usage problem, printed as asJSON says.
func helpRoute(app *kong.Application, args []string, asJSON bool) Route {
	var topics []string
	for _, a := range args {
		if a != "--json" {
			topics = append(topics, a)
		}
	}
	r := Route{JSON: asJSON}
	if len(topics) == 0 {
		r.Usage = true
		return r
	}
	node := command(app.Node, topics[0])
	if node == nil {
		r.Problem = UnknownCommand(app.Node, topics[0])
		return r
	}
	most := 1
	if len(node.Children) > 0 {
		most = 2
	}
	switch {
	case len(topics) > most:
		r.Problem = NewProblem(CodeUsage, "args.unexpected", fmt.Sprintf("help takes one command, and was given %q too", topics[most]),
			fmt.Sprintf("Run '%[1]s help <command>', or '%[1]s help <group> <command>'.", app.Name)).With("argument", topics[most])
	case len(topics) == 2 && command(node, topics[1]) == nil:
		r.Problem = UnknownCommand(node, topics[1])
	default:
		r.Help = topics
	}
	return r
}

// Usage is the top-level help of app, kong's model of every command: each
// command a line, sorted by name, a group's subcommands each a line in its
// place in the group's order, then the options every command takes, the exit
// codes and examples.
func Usage(app *kong.Application) string {
	program := app.Name
	var b strings.Builder
	fmt.Fprintf(&b, "usage: %s <command> [options] [path ...]\n\nCommands:\n", program)
	nodes := slices.Clone(app.Children)
	slices.SortStableFunc(nodes, func(a, b *kong.Node) int { return strings.Compare(a.Name, b.Name) })
	for _, node := range nodes {
		if node.Type != kong.CommandNode || node.Hidden {
			continue
		}
		if len(node.Children) == 0 {
			fmt.Fprintf(&b, "  %-15s %s\n", node.Name, node.Help)
			continue
		}
		for _, sub := range node.Children {
			fmt.Fprintf(&b, "  %-15s %s\n", node.Name+" "+sub.Name, sub.Help)
		}
	}
	fmt.Fprintf(&b, `
Paths are files, directories, or fragments of a path under the working
directory. Without paths the working directory is analyzed.

Options, for every command and in any position:
  --json       print one JSON object on stdout: {"schema": 1, "ok": true|false,
               …, "problems": [{"rule", "message", "fix", …}]}
  -h, --help   print help; '%[1]s help <command>' for a command's, such
               as '%[1]s help crap' or '%[1]s help mutation run'
  --version    print the version

Exit codes, for every command (each command's help says when):
  0   success
  1   a check said no: a mutant survived, a function is over a threshold
  2   a usage or config error
  3   the environment lacks something: a tool, a report, a git repository
  70  an internal error %[1]s could not classify; please report it
  75  a temporary failure: the same command may pass when run again

Scripts read the exit code and --json, less "message" and "fix"; plain
output is for people and may change in any release. See docs/CLI.md.

Examples:
  %[1]s crap --top 20
  %[1]s mutation run --changed --json

Report issues at %[2]s
`, program, Issues)
	return b.String()
}
