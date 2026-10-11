package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/donvargax/kong"
)

// A command's help is its contract with the person and the script that run
// it (docs/CLI.md, rules 9, 10 and 14): its usage, what it does, its
// options, the shape of its --json object, its problem rules, its exit
// codes and examples, then where to report an issue. Every command prints
// it in one layout, Page's, whether kong parses its command line or not.

// Exit is an exit code a command can return, and when.
type Exit struct {
	Code    int
	Meaning string
}

// Contract is what a command's help says beyond its flags and arguments,
// which kong's model holds: what it does, the keys --json adds beside
// schema, ok and problems, the problem rules it reports ("rule: meaning"),
// its exit codes (0 and 2 among them; 70 is every command's), and examples.
type Contract struct {
	About    string
	JSON     string
	Rules    []string
	Exits    []Exit
	Examples []string
}

// Documented is a command whose help gives its contract: a slice's kong
// struct implements it, and Help prints it.
type Documented interface {
	Contract() Contract
}

// HelpFlag is one flag as help lists it: its name, its one-letter form, the
// placeholder of its value, its default, the values a switch may take after
// =, and whether it may be given again.
type HelpFlag struct {
	Name    string
	Short   rune
	Arg     string
	Default string
	Choices []string
	Repeat  bool
	Help    string
}

// Page is one command's help: the program and the command, the words after
// them in its usage line, its flags and its contract.
type Page struct {
	Program, Command, Synopsis string
	Flags                      []HelpFlag
	Contract
}

// String is the page as the command prints it.
func (p Page) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "usage: %s %s %s\n\n%s\n\nOptions:\n", p.Program, p.Command, p.Synopsis, strings.TrimSpace(p.About))
	WriteFlags(&b, p.Flags)
	fmt.Fprintf(&b, "\nOutput with --json, one object on stdout:\n  {\"schema\": 1, \"ok\": true|false, %s,\n   \"problems\": [{\"rule\", \"message\", \"fix\", and keys for its subject}]}\n", p.JSON)
	b.WriteString("  \"problems\" is there only when ok is false. Scripts read the exit code and\n  these keys, never \"message\", \"fix\", or the plain output.\n")
	if len(p.Rules) > 0 {
		b.WriteString("\nProblem rules:\n")
		for _, r := range p.Rules {
			fmt.Fprintf(&b, "  %s\n", r)
		}
	}
	b.WriteString("\nExit codes:\n")
	for _, e := range p.Exits {
		fmt.Fprintf(&b, "  %-3d %s\n", e.Code, e.Meaning)
	}
	fmt.Fprintf(&b, "  %-3d an internal error %s could not classify; please report it\n", CodeInternal, p.Program)
	b.WriteString("\nExamples:\n")
	for _, e := range p.Examples {
		fmt.Fprintf(&b, "  %s\n", e)
	}
	fmt.Fprintf(&b, "\nReport issues at %s\n", Issues)
	return b.String()
}

// WriteFlags lists flags as help does, one a line, its description in a
// column of its own, or on the next line when the flag is too long for it.
func WriteFlags(w io.Writer, flags []HelpFlag) {
	for _, f := range flags {
		name := "--" + f.Name
		if f.Short != 0 {
			name = "-" + string(f.Short) + ", " + name
		}
		if f.Arg != "" {
			name += " " + f.Arg
		}
		if len(f.Choices) > 0 {
			name += "[=" + strings.Join(f.Choices, "|") + "]"
		}
		help := f.Help
		if f.Default != "" {
			help += fmt.Sprintf(" (default %s)", f.Default)
		}
		if f.Repeat {
			help += "; repeatable"
		}
		if len(name) > 26 {
			_, _ = fmt.Fprintf(w, "  %s\n  %-26s %s\n", name, "", help)
		} else {
			_, _ = fmt.Fprintf(w, "  %-26s %s\n", name, help)
		}
	}
}

// Help is kong's help printer, kong.Help(cli.Help): the page of the command
// kong selected, from its model and its Contract, on kong's stdout. kong's
// own printer is replaced so every command's help keeps one layout.
func Help(_ kong.HelpOptions, ctx *kong.Context) error {
	node := ctx.Selected()
	if node == nil {
		return errors.New("help names no command")
	}
	page, err := PageOf(node)
	if err != nil {
		return err
	}
	_, err = io.WriteString(ctx.Stdout, page.String())
	return err
}

// PageOf is the help page of node, a command of kong's model: its flags,
// its own first and then those of the commands above it, its arguments, and
// the Contract its struct gives.
func PageOf(node *kong.Node) (Page, error) {
	documented, ok := node.Target.Interface().(Documented)
	if !ok {
		return Page{}, fmt.Errorf("the command %q gives no Contract for its help", commandPath(node))
	}
	page := Page{Program: programOf(node), Command: commandPath(node), Synopsis: synopsis(node), Contract: documented.Contract()}
	for _, f := range flagsOf(node) {
		one := HelpFlag{Name: f.Name, Short: f.Short, Help: f.Help, Repeat: f.IsCumulative()}
		if !f.IsBool() {
			one.Arg = f.FormatPlaceHolder()
		}
		if f.HasDefault {
			one.Default = f.Default
		}
		page.Flags = append(page.Flags, one)
	}
	return page, nil
}

// synopsis is the words after a command in its usage line: its options,
// then each argument, [name] when it may be left out, <name> when it may
// not, with ... when it may be given again.
func synopsis(node *kong.Node) string {
	words := []string{"[options]"}
	for _, p := range node.Positional {
		name := p.Name
		if p.IsCumulative() {
			name += " ..."
		}
		if p.Required {
			words = append(words, "<"+name+">")
		} else {
			words = append(words, "["+name+"]")
		}
	}
	return strings.Join(words, " ")
}

// flagsOf are the flags a command takes, in the order help lists them and
// an unknown flag's guess weighs them: the command's own as its struct
// declares them, then those of each command above it, the program's last.
// A hidden flag is left out.
func flagsOf(node *kong.Node) []*kong.Flag {
	var flags []*kong.Flag
	for n := node; n != nil; n = n.Parent {
		for _, f := range n.Flags {
			if !f.Hidden {
				flags = append(flags, f)
			}
		}
	}
	return flags
}

// commandPath is node's command as a person writes it after the program:
// units, or mutation run.
func commandPath(node *kong.Node) string {
	var words []string
	for n := node; n != nil && n.Type == kong.CommandNode; n = n.Parent {
		words = append([]string{n.Name}, words...)
	}
	return strings.Join(words, " ")
}

// programOf is the name of the program node is a command of.
func programOf(node *kong.Node) string {
	for node.Parent != nil {
		node = node.Parent
	}
	return node.Name
}
