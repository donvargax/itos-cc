package cli

import (
	"fmt"
	"strings"

	"github.com/donvargax/kong"
)

// GroupHelp preserves a group's help layout, using the same parser model
// as its children. A group describes itself through the usual Contract.
func GroupHelp(node *kong.Node) (string, error) {
	documented, ok := node.Target.Interface().(Documented)
	if !ok {
		return "", fmt.Errorf("the group %q gives no Contract for its help", commandPath(node))
	}
	c := documented.Contract()
	name := commandPath(node)
	var b strings.Builder
	fmt.Fprintf(&b, "usage: %s %s <subcommand> [options] [path ...]\n\n%s\n\nSubcommands:\n", programOf(node), name, strings.TrimSpace(c.About))
	width := 0
	for _, sub := range node.Children {
		width = max(width, len(sub.Name))
	}
	for _, sub := range node.Children {
		fmt.Fprintf(&b, "  %-*s  %s\n", width, sub.Name, sub.Help)
	}
	fmt.Fprintf(&b, `
Run '%[1]s %[2]s <subcommand> --help', or '%[1]s help %[2]s <subcommand>',
for a subcommand's options, --json output, problem rules, and exit codes.

Output with --json and no subcommand, one object on stdout:
  {"schema": 1, "ok": false, "problems": [{"rule": "command.missing", …}]}

Problem rules:
  command.unknown  no subcommand by that name: command
  command.missing  --json with no subcommand

Exit codes:
  0   help printed
  80  a usage error: an unknown flag, or --json with no subcommand
  81  an unknown subcommand
  130 %[3]s
`, programOf(node), name, InterruptedMeaning)
	b.WriteString("\nExamples:\n")
	for _, e := range c.Examples {
		fmt.Fprintf(&b, "  %s\n", e)
	}
	fmt.Fprintf(&b, "\nReport issues at %s\n", Issues)
	return b.String(), nil
}

// GroupLine admits the words before a group's child, before its flags are
// parsed. Only the common JSON/help switches are accepted there. The
// selected child is always resolved from this parser's model; its options
// are parsed once by Parse. Help after an unknown child cannot hide it.
func GroupLine(node *kong.Node, args []string) (child []string, help bool, problem *Problem) {
	json := Read(args).JSON
	i := 0
	for ; i < len(args) && strings.HasPrefix(args[i], "-") && args[i] != "--"; i++ {
		switch word := args[i]; word {
		case "--json":
		case "-h", "--help":
			return nil, true, nil
		default:
			name := commandPath(node)
			return nil, false, NewProblem(CodeUsage, "flags.unknown", fmt.Sprintf("%s takes no flag %s before a subcommand", name, word), fmt.Sprintf("Put a subcommand's flags after its name; run '%s %s --help' for the subcommands.", programOf(node), name)).With("flag", word)
		}
	}
	if i == len(args) {
		if !json {
			return nil, true, nil
		}
		return nil, false, NewProblem(CodeUsage, "command.missing", fmt.Sprintf("%s needs a subcommand", commandPath(node)), fmt.Sprintf("Name one of %s; run '%s %s --help' for them.", andList(commandNames(node)), programOf(node), commandPath(node)))
	}
	if command(node, args[i]) == nil {
		return nil, false, UnknownCommand(node, args[i])
	}
	child = append([]string{node.Name}, args[i:]...)
	// Leading group JSON affects group errors only, as the original group
	// dispatcher did; a child-level occurrence is parsed by the child.
	return child, false, nil
}
