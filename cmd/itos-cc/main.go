// Command itos-cc measures code quality in TypeScript, Python, Kotlin, and Go.
package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"
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
			return nil, fail(kindUsage, "args.unexpected", fmt.Sprintf("version takes no arguments, and was given %q", in.args[0]),
				"Run 'itos-cc version' alone.").with("argument", in.args[0])
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
	for _, c := range []*command{crapCommand, dryCommand, mutateCommand, scrapCommand, serveCommand, unitsCommand, versionCommand} {
		commands[c.name] = c
	}
}

func usage() string {
	var b strings.Builder
	b.WriteString("usage: itos-cc <command> [options] [path ...]\n\nCommands:\n")
	for _, name := range sortedNames(commands) {
		fmt.Fprintf(&b, "  %-8s %s\n", name, commands[name].summary)
	}
	b.WriteString(`
Paths are files, directories, or fragments of a path under the working
directory. Without paths the working directory is analyzed.

Options, for every command and in any position:
  --json       print one JSON object on stdout: {"schema": 1, "ok": true|false,
               …, "problems": [{"rule", "message", "fix", …}]}
  -h, --help   print help; 'itos-cc help <command>' for a command's
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
  itos-cc mutate --changed --json

Report issues at ` + issues + "\n")
	return b.String()
}

func main() {
	os.Exit(run(os.Args[1:]))
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
			p := fail(kindUsage, "flags.unknown", fmt.Sprintf("itos-cc takes no flag %s before a command", a),
				"Put a command's flags after its name; run 'itos-cc --help' for the commands.").with("flag", a)
			return emit(asJSON, nil, []*problem{p})
		}
	}
	if len(args) == 0 {
		if asJSON {
			p := fail(kindUsage, "command.missing", "no command given", "Name a command; run 'itos-cc --help' for them.")
			return emit(true, nil, []*problem{p})
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
		return emit(asJSON, nil, []*problem{unknownCommand(name)})
	}
	return execute(cmd, rest)
}

// help prints the top-level help, or with a command name that command's.
func help(args []string, asJSON bool) int {
	var topics []string
	for _, a := range args {
		if a != "--json" {
			topics = append(topics, a)
		}
	}
	switch {
	case len(topics) == 0:
		fmt.Print(usage())
	case len(topics) > 1:
		return emit(asJSON, nil, []*problem{fail(kindUsage, "args.unexpected", "help takes one command",
			"Run 'itos-cc help <command>'.").with("argument", topics[1])})
	case commands[topics[0]] == nil:
		return emit(asJSON, nil, []*problem{unknownCommand(topics[0])})
	default:
		fmt.Print(commands[topics[0]].help())
	}
	return 0
}

func unknownCommand(name string) *problem {
	p := fail(kindUsage, "command.unknown", fmt.Sprintf("there is no command %q", name), "Run 'itos-cc --help' for the commands.")
	if guess := closest(name, sortedNames(commands)); guess != "" {
		p.fix = fmt.Sprintf("Did you mean '%s'? Run 'itos-cc --help' for the commands.", guess)
	}
	return p.with("command", name)
}
