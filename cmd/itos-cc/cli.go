package main

import (
	"fmt"
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/donvargax/itos-cc/internal/cli"
)

// The command line's own flag parser and help. Every command reports
// through internal/cli: its problems, the --json object, the plain lines
// and the exit code.

// flagType is what a flag's value must parse as. A switch takes none.
type flagType int

const (
	switchFlag flagType = iota
	stringFlag
	intFlag
	floatFlag
	durationFlag
)

// flagSpec declares one long flag. Every command's flags are parsed from
// these, so every rule about flags holds in every command.
type flagSpec struct {
	name   string
	typ    flagType
	arg    string // the value's placeholder in help, such as N
	def    string // the default, shown in help when set
	repeat bool   // may be given more than once
	// choices, on a switch, are the values it may take after =, as in
	// --fail-uncovered=lines; alone, it is "true". Never the next argument.
	choices []string
	help    string
}

func sw(name, help string) flagSpec { return flagSpec{name: name, help: help} }

// swChoice is a switch that may also take one of choices after =.
func swChoice(name string, choices []string, help string) flagSpec {
	return flagSpec{name: name, choices: choices, help: help}
}

func opt(name string, typ flagType, arg, def, help string) flagSpec {
	return flagSpec{name: name, typ: typ, arg: arg, def: def, help: help}
}

// common flags every command takes.
var (
	jsonFlag = sw("json", "print one JSON object on stdout instead of text (shape below)")
	helpFlag = sw("help", "print this help")
)

// command is one subcommand: its help, its flags, and what it does.
type command struct {
	name     string
	summary  string // one line in the top-level help
	synopsis string // after "itos-cc <name> "
	about    string
	flags    []flagSpec
	json     string    // the keys --json adds beside schema, ok, and problems
	exits    []exitDoc // its exit codes, 0 and 2 included
	rules    []string  // the problem rules it reports, "rule: meaning"
	examples []string
	run      func(*invocation) (any, error)
	// subs makes the command a group, a noun whose subcommands are the
	// actions: mutation run, mutation list. A group has no run of its own;
	// alone it prints its help.
	subs []*command
}

type exitDoc struct {
	code    int
	meaning string
}

func (c *command) allFlags() []flagSpec {
	return append(append([]flagSpec{}, c.flags...), jsonFlag, helpFlag)
}

// invocation is one run of a command: its parsed flags and arguments, and
// the problems found so far.
type invocation struct {
	cmd      *command
	values   map[string][]string
	args     []string
	json     bool
	problems []*cli.Problem
}

func (in *invocation) set(name string) bool { return len(in.values[name]) > 0 }

func (in *invocation) str(name string) string {
	if v := in.values[name]; len(v) > 0 {
		return v[len(v)-1]
	}
	for _, f := range in.cmd.allFlags() {
		if f.name == name {
			return f.def
		}
	}
	return ""
}

func (in *invocation) strs(name string) []string { return in.values[name] }

// The typed readers cannot fail: parse checked every value.
func (in *invocation) integer(name string) int {
	n, _ := strconv.Atoi(in.str(name))
	return n
}

func (in *invocation) float(name string) float64 {
	f, _ := strconv.ParseFloat(in.str(name), 64)
	return f
}

func (in *invocation) duration(name string) time.Duration {
	d, _ := time.ParseDuration(in.str(name))
	return d
}

func (in *invocation) report(p *cli.Problem) { in.problems = append(in.problems, p) }

// errHelp is returned by parse when help was asked for.
var errHelp = cli.NewProblem(0, "help", "help", "")

// parse reads args against the command's flags: --flag value and
// --flag=value, flags in any position, -- to end them, and -h or --help
// anywhere for help. It refuses an unknown flag, a value missing or of the
// wrong type, a value given to a switch, unless it is one of the switch's
// choices, given after =, and a once-only flag given twice.
func parse(cmd *command, args []string) (*invocation, error) {
	in := &invocation{cmd: cmd, values: map[string][]string{}}
	specs := map[string]flagSpec{}
	for _, f := range cmd.allFlags() {
		specs[f.name] = f
	}
	// --json applies to a usage error too, so it is found before any flag
	// is checked.
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "--json" {
			in.json = true
		}
		if a == "-h" || a == "--help" {
			return in, errHelp
		}
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			in.args = append(in.args, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			in.args = append(in.args, a)
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		f, known := specs[name]
		if !known || !strings.HasPrefix(a, "--") {
			p := cli.NewProblem(cli.CodeUsage, "flags.unknown", fmt.Sprintf("%s takes no flag %s", cmd.name, a), "")
			if guess := cli.Closest("--"+name, flagNames(cmd)); guess != "" {
				p.Fix = fmt.Sprintf("Did you mean %s? Run 'itos-cc %s --help' for its flags.", guess, cmd.name)
			} else {
				p.Fix = fmt.Sprintf("Run 'itos-cc %s --help' for its flags.", cmd.name)
			}
			return in, p.With("flag", a)
		}
		if len(in.values[name]) > 0 && !f.repeat {
			return in, cli.NewProblem(cli.CodeUsage, "flags.repeated", fmt.Sprintf("--%s is given more than once", name),
				"Give it once.").With("flag", "--"+name)
		}
		switch {
		case f.typ == switchFlag && hasValue && len(f.choices) > 0 && !slices.Contains(f.choices, value):
			return in, cli.NewProblem(cli.CodeUsage, "flags.value-invalid", fmt.Sprintf("--%s takes %s, and was given %q", name, choiceList(f.choices), value),
				fmt.Sprintf("Write --%s alone, or --%s=%s.", name, name, f.choices[0])).With("flag", "--"+name).With("value", value)
		case f.typ == switchFlag && hasValue && len(f.choices) > 0:
		case f.typ == switchFlag && hasValue:
			return in, cli.NewProblem(cli.CodeUsage, "flags.switch-value", fmt.Sprintf("--%s takes no value, and was given %q", name, value),
				fmt.Sprintf("Write --%s alone.", name)).With("flag", "--"+name)
		case f.typ == switchFlag:
			value = "true"
		case !hasValue && (i+1 == len(args) || (strings.HasPrefix(args[i+1], "-") && args[i+1] != "-")):
			// An option's value is never read as a flag: --top --json is a
			// --top with no value.
			return in, cli.NewProblem(cli.CodeUsage, "flags.value-missing", fmt.Sprintf("--%s needs a value", name),
				fmt.Sprintf("Give it one, such as --%s %s.", name, f.arg)).With("flag", "--"+name)
		case !hasValue:
			i++
			value = args[i]
		}
		if err := checkValue(f, value); err != nil {
			return in, err
		}
		in.values[name] = append(in.values[name], value)
	}
	return in, nil
}

// choiceList names a switch's choices, as "=a or =b".
func choiceList(choices []string) string {
	var out []string
	for _, c := range choices {
		out = append(out, "="+c)
	}
	return strings.Join(out, " or ")
}

func checkValue(f flagSpec, value string) error {
	var err error
	what := ""
	switch f.typ {
	case intFlag:
		_, err = strconv.Atoi(value)
		what = "a whole number"
	case floatFlag:
		_, err = strconv.ParseFloat(value, 64)
		what = "a number"
	case durationFlag:
		_, err = time.ParseDuration(value)
		what = "a duration such as 500ms or 2s"
	}
	if err != nil {
		return cli.NewProblem(cli.CodeUsage, "flags.value-invalid", fmt.Sprintf("--%s needs %s, and was given %q", f.name, what, value),
			fmt.Sprintf("Give it %s.", what)).With("flag", "--"+f.name).With("value", value)
	}
	return nil
}

func flagNames(cmd *command) []string {
	var names []string
	for _, f := range cmd.allFlags() {
		names = append(names, "--"+f.name)
	}
	return names
}

// execute runs cmd with args and returns its exit code. Problems print on
// stderr, or with --json in the one object on stdout.
func execute(cmd *command, args []string) int {
	in, err := parse(cmd, args)
	if err == errHelp {
		fmt.Print(cmd.help())
		return 0
	}
	var data any
	if err == nil {
		data, err = runSafely(cmd, in)
	}
	ui := stdUI()
	if err != nil {
		in.problems = append(in.problems, ui.Problems(err)...)
	}
	return ui.Emit(in.json, data, in.problems)
}

// runSafely runs cmd, turning a panic into an internal error, so even a bug
// ends in the --json object and exit 70.
func runSafely(cmd *command, in *invocation) (data any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%s panicked: %v", cmd.name, r)
		}
	}()
	return cmd.run(in)
}

// help is the command's help: usage, what it does, its flags, its --json
// shape, its problem rules, its exit codes, and examples, in the layout
// every command's help has (cli.Page).
func (c *command) help() string {
	page := cli.Page{Program: "itos-cc", Command: c.name, Synopsis: c.synopsis, Flags: helpFlags(c.allFlags()),
		Contract: cli.Contract{About: c.about, JSON: c.json, Rules: c.rules, Examples: c.examples}}
	for _, e := range c.exits {
		page.Exits = append(page.Exits, cli.Exit{Code: e.code, Meaning: e.meaning})
	}
	return page.String()
}

func writeFlags(w io.Writer, flags []flagSpec) { cli.WriteFlags(w, helpFlags(flags)) }

// helpFlags are flags as help lists them, help's own with -h.
func helpFlags(flags []flagSpec) []cli.HelpFlag {
	var out []cli.HelpFlag
	for _, f := range flags {
		one := cli.HelpFlag{Name: f.name, Arg: f.arg, Default: f.def, Choices: f.choices, Repeat: f.repeat, Help: f.help}
		if f.name == "help" {
			one.Short = 'h'
		}
		out = append(out, one)
	}
	return out
}

// andList joins words as "a", "a and b", or "a, b, and c".
func andList(words []string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	case 2:
		return words[0] + " and " + words[1]
	}
	return strings.Join(words[:len(words)-1], ", ") + ", and " + words[len(words)-1]
}

func sortedNames(m map[string]*command) []string {
	var names []string
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// sub is the group's subcommand named name, or nil.
func (c *command) sub(name string) *command {
	for _, s := range c.subs {
		if s.verb() == name {
			return s
		}
	}
	return nil
}

// verb is a subcommand's own name: run for "mutation run".
func (c *command) verb() string {
	return c.name[strings.LastIndex(c.name, " ")+1:]
}

func (c *command) subNames() []string {
	var names []string
	for _, s := range c.subs {
		names = append(names, s.verb())
	}
	return names
}

// groupHelp is a group's help: its usage, what it is for, its subcommands,
// and where each one's contract is.
func (c *command) groupHelp() string {
	var b strings.Builder
	fmt.Fprintf(&b, "usage: itos-cc %s <subcommand> [options] [path ...]\n\n%s\n\nSubcommands:\n", c.name, strings.TrimSpace(c.about))
	width := 0
	for _, s := range c.subs {
		width = max(width, len(s.verb()))
	}
	for _, s := range c.subs {
		fmt.Fprintf(&b, "  %-*s  %s\n", width, s.verb(), s.summary)
	}
	fmt.Fprintf(&b, `
Run 'itos-cc %[1]s <subcommand> --help', or 'itos-cc help %[1]s <subcommand>',
for a subcommand's options, --json output, problem rules, and exit codes.

Output with --json and no subcommand, one object on stdout:
  {"schema": 1, "ok": false, "problems": [{"rule": "command.missing", …}]}

Problem rules:
  command.unknown  no subcommand by that name: command
  command.missing  --json with no subcommand

Exit codes:
  0   help printed
  2   a usage error: an unknown subcommand or flag, or --json with no subcommand
`, c.name)
	b.WriteString("\nExamples:\n")
	for _, e := range c.examples {
		fmt.Fprintf(&b, "  %s\n", e)
	}
	fmt.Fprintf(&b, "\nReport issues at %s\n", cli.Issues)
	return b.String()
}
