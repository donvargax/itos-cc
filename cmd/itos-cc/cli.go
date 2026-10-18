package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/donvargax/itos-cc/internal/cli"
)

// The hand parser and help of serve, the one command still on its legacy
// handler: dispatch hands it serve's command line as it was written, and it
// reports through internal/cli, its problems, the --json object, the plain
// lines and the exit code, as every command does. It reads only what serve
// takes: switches, strings, whole numbers and durations, each given once.

// flagType is what a flag's value must parse as. A switch takes none.
type flagType int

const (
	switchFlag flagType = iota
	stringFlag
	intFlag
	durationFlag
)

// flagSpec declares one long flag, which parse reads by it.
type flagSpec struct {
	name string
	typ  flagType
	arg  string // the value's placeholder in help, such as N
	def  string // the default, shown in help when set
	help string
}

func sw(name, help string) flagSpec { return flagSpec{name: name, help: help} }

func opt(name string, typ flagType, arg, def, help string) flagSpec {
	return flagSpec{name: name, typ: typ, arg: arg, def: def, help: help}
}

// common flags every command takes.
var (
	jsonFlag = sw("json", "print one JSON object on stdout instead of text (shape below)")
	helpFlag = sw("help", "print this help")
)

// command is a command on the hand parser: its help, its flags, and what it
// does.
type command struct {
	name     string
	summary  string // one line in the top-level help
	synopsis string // after "itos-cc <name> "
	about    string
	flags    []flagSpec
	json     string    // the keys --json adds beside schema, ok, and problems
	exits    []exitDoc // its exit codes, 80 included
	rules    []string  // the problem rules it reports, "rule: meaning"
	examples []string
	run      func(context.Context, *invocation) (any, error)
}

type exitDoc struct {
	code    int
	meaning string
}

func (c *command) allFlags() []flagSpec {
	return append(append([]flagSpec{}, c.flags...), jsonFlag, helpFlag)
}

// invocation is one run of a command: its parsed flags and arguments, the
// problems found so far, and the adapters run was given, which serve reads
// its snapshots and toolchain through.
type invocation struct {
	cmd      *command
	values   map[string][]string
	args     []string
	json     bool
	problems []*cli.Problem
	adapters adapters
}

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

// The typed readers cannot fail: parse checked every value.
func (in *invocation) integer(name string) int {
	n, _ := strconv.Atoi(in.str(name))
	return n
}

func (in *invocation) duration(name string) time.Duration {
	d, _ := time.ParseDuration(in.str(name))
	return d
}

// errHelp is returned by parse when help was asked for.
var errHelp = cli.NewProblem(0, "help", "help", "")

// parse reads args against the command's flags: --flag value and
// --flag=value, flags in any position, -- to end them, and -h or --help
// anywhere for help. It refuses an unknown flag, a value missing or of the
// wrong type, a value given to a switch, and a flag given twice.
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
		if len(in.values[name]) > 0 {
			return in, cli.NewProblem(cli.CodeUsage, "flags.repeated", fmt.Sprintf("--%s is given more than once", name),
				"Give it once.").With("flag", "--"+name)
		}
		// The argument after this one, which an option without =value takes.
		next, hasNext := "", i+1 < len(args)
		if hasNext {
			next = args[i+1]
		}
		switch {
		case f.typ == switchFlag && hasValue:
			return in, cli.NewProblem(cli.CodeUsage, "flags.switch-value", fmt.Sprintf("--%s takes no value, and was given %q", name, value),
				fmt.Sprintf("Write --%s alone.", name)).With("flag", "--"+name)
		case f.typ == switchFlag:
			value = "true"
		case !hasValue && (!hasNext || (strings.HasPrefix(next, "-") && next != "-")):
			// An option's value is never read as a flag: --port --json is a
			// --port with no value.
			return in, cli.NewProblem(cli.CodeUsage, "flags.value-missing", fmt.Sprintf("--%s needs a value", name),
				fmt.Sprintf("Give it one, such as --%s %s.", name, f.arg)).With("flag", "--"+name)
		case !hasValue:
			i++
			value = next
		}
		if err := checkValue(f, value); err != nil {
			return in, err
		}
		in.values[name] = append(in.values[name], value)
	}
	return in, nil
}

func checkValue(f flagSpec, value string) error {
	var err error
	what := ""
	switch f.typ {
	case intFlag:
		_, err = strconv.Atoi(value)
		what = "a whole number"
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

// execute runs cmd with args under ctx, with the adapters a, and returns
// its exit code. Problems print on stderr, or with --json in the one object
// on stdout.
func execute(ctx context.Context, cmd *command, args []string, a adapters) int {
	in, err := parse(cmd, args)
	in.adapters = a
	if err == errHelp {
		fmt.Print(cmd.help())
		return 0
	}
	var data any
	if err == nil {
		data, err = runSafely(ctx, cmd, in)
	}
	ui := stdUI()
	if err != nil {
		in.problems = append(in.problems, ui.Problems(err)...)
	}
	return ui.Emit(in.json, data, in.problems)
}

// runSafely runs cmd under ctx, turning a panic into an internal error, so
// even a bug ends in the --json object and exit 100.
func runSafely(ctx context.Context, cmd *command, in *invocation) (data any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%s panicked: %v", cmd.name, r)
		}
	}()
	return cmd.run(ctx, in)
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

// helpFlags are flags as help lists them, help's own with -h.
func helpFlags(flags []flagSpec) []cli.HelpFlag {
	var out []cli.HelpFlag
	for _, f := range flags {
		one := cli.HelpFlag{Name: f.name, Arg: f.arg, Default: f.def, Help: f.help}
		if f.name == "help" {
			one.Short = 'h'
		}
		out = append(out, one)
	}
	return out
}
