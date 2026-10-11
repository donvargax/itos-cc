package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The contract with scripts is the exit code, the --json object less its
// "message" and "fix" keys, and the files under .metrics/. Plain output is
// for people and may change in any release. See docs/CLI.md.

// kind is what sort of outcome a problem is. It is the exit code.
type kind int

const (
	kindNo        kind = 1  // a check said no
	kindUsage     kind = 2  // a usage or config error
	kindMissing   kind = 3  // the environment lacks something
	kindInternal  kind = 70 // an error nothing classified
	kindTemporary kind = 75 // may pass when run again unchanged
)

// precedence picks the exit code of a run with problems of several kinds:
// the one that says the most about why nothing else could be trusted.
var precedence = []kind{kindUsage, kindInternal, kindMissing, kindTemporary, kindNo}

const issues = "https://github.com/donvargax/itos-cc/issues"

// problem is one reason a run did not succeed. Rule is a stable id scripts
// switch on; message and fix are sentences for people. Subject holds what
// the problem is about, such as file, line, and function, as keys of its
// own, so no script parses a message.
type problem struct {
	kind    kind
	rule    string
	message string
	fix     string
	subject map[string]any
	// shown is set when plain output already shows the problem, so it is not
	// repeated on stderr.
	shown bool
}

func fail(k kind, rule, message, fix string) *problem {
	return &problem{kind: k, rule: rule, message: message, fix: fix, subject: map[string]any{}}
}

func (p *problem) with(key string, value any) *problem {
	p.subject[key] = value
	return p
}

func (p *problem) Error() string { return p.message }

func (p *problem) MarshalJSON() ([]byte, error) {
	out := map[string]any{"rule": p.rule, "message": p.message}
	if p.fix != "" {
		out["fix"] = p.fix
	}
	for k, v := range p.subject {
		out[k] = v
	}
	return json.Marshal(out)
}

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
	code    kind
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
	problems []*problem
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

func (in *invocation) report(p *problem) { in.problems = append(in.problems, p) }

// errHelp is returned by parse when help was asked for.
var errHelp = fail(0, "help", "help", "")

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
			p := fail(kindUsage, "flags.unknown", fmt.Sprintf("%s takes no flag %s", cmd.name, a), "")
			if guess := closest("--"+name, flagNames(cmd)); guess != "" {
				p.fix = fmt.Sprintf("Did you mean %s? Run 'itos-cc %s --help' for its flags.", guess, cmd.name)
			} else {
				p.fix = fmt.Sprintf("Run 'itos-cc %s --help' for its flags.", cmd.name)
			}
			return in, p.with("flag", a)
		}
		if len(in.values[name]) > 0 && !f.repeat {
			return in, fail(kindUsage, "flags.repeated", fmt.Sprintf("--%s is given more than once", name),
				"Give it once.").with("flag", "--"+name)
		}
		switch {
		case f.typ == switchFlag && hasValue && len(f.choices) > 0 && !slices.Contains(f.choices, value):
			return in, fail(kindUsage, "flags.value-invalid", fmt.Sprintf("--%s takes %s, and was given %q", name, choiceList(f.choices), value),
				fmt.Sprintf("Write --%s alone, or --%s=%s.", name, name, f.choices[0])).with("flag", "--"+name).with("value", value)
		case f.typ == switchFlag && hasValue && len(f.choices) > 0:
		case f.typ == switchFlag && hasValue:
			return in, fail(kindUsage, "flags.switch-value", fmt.Sprintf("--%s takes no value, and was given %q", name, value),
				fmt.Sprintf("Write --%s alone.", name)).with("flag", "--"+name)
		case f.typ == switchFlag:
			value = "true"
		case !hasValue && (i+1 == len(args) || (strings.HasPrefix(args[i+1], "-") && args[i+1] != "-")):
			// An option's value is never read as a flag: --top --json is a
			// --top with no value.
			return in, fail(kindUsage, "flags.value-missing", fmt.Sprintf("--%s needs a value", name),
				fmt.Sprintf("Give it one, such as --%s %s.", name, f.arg)).with("flag", "--"+name)
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
		return fail(kindUsage, "flags.value-invalid", fmt.Sprintf("--%s needs %s, and was given %q", f.name, what, value),
			fmt.Sprintf("Give it %s.", what)).with("flag", "--"+f.name).with("value", value)
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
	if err != nil {
		in.report(asProblem(err))
	}
	return emit(in.json, data, in.problems)
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

// asProblem keeps a problem as it is and makes any other error internal.
func asProblem(err error) *problem {
	if p, ok := err.(*problem); ok {
		return p
	}
	return fail(kindInternal, "internal", err.Error(), "This is a bug in itos-cc; please report it at "+issues+".")
}

// emit prints the outcome and returns the exit code: with asJSON the object
// {"schema": 1, "ok": …, data's keys…, "problems": […]} on stdout,
// otherwise each problem on stderr as "itos-cc: <message>. <fix>".
func emit(asJSON bool, data any, problems []*problem) int {
	if asJSON {
		out := map[string]any{}
		if data != nil {
			raw, err := json.Marshal(data)
			if err == nil {
				json.Unmarshal(raw, &out)
			}
		}
		out["schema"] = 1
		out["ok"] = len(problems) == 0
		if len(problems) > 0 {
			out["problems"] = problems
		}
		printJSON(out)
	} else {
		for _, p := range problems {
			if !p.shown {
				fmt.Fprintln(os.Stderr, plainProblem(p))
			}
		}
	}
	for _, k := range precedence {
		for _, p := range problems {
			if p.kind == k {
				return int(k)
			}
		}
	}
	return 0
}

func plainProblem(p *problem) string {
	line := "itos-cc: " + p.message
	if p.fix != "" {
		line += ". " + p.fix
	}
	return line
}

// help is the command's help: usage, what it does, its flags, its --json
// shape, its problem rules, its exit codes, and examples.
func (c *command) help() string {
	var b strings.Builder
	fmt.Fprintf(&b, "usage: itos-cc %s %s\n\n%s\n\nOptions:\n", c.name, c.synopsis, strings.TrimSpace(c.about))
	writeFlags(&b, c.allFlags())
	fmt.Fprintf(&b, "\nOutput with --json, one object on stdout:\n  {\"schema\": 1, \"ok\": true|false, %s,\n   \"problems\": [{\"rule\", \"message\", \"fix\", and keys for its subject}]}\n", c.json)
	b.WriteString("  \"problems\" is there only when ok is false. Scripts read the exit code and\n  these keys, never \"message\", \"fix\", or the plain output.\n")
	if len(c.rules) > 0 {
		b.WriteString("\nProblem rules:\n")
		for _, r := range c.rules {
			fmt.Fprintf(&b, "  %s\n", r)
		}
	}
	b.WriteString("\nExit codes:\n")
	for _, e := range c.exits {
		fmt.Fprintf(&b, "  %-3d %s\n", e.code, e.meaning)
	}
	b.WriteString("  70  an internal error itos-cc could not classify; please report it\n")
	b.WriteString("\nExamples:\n")
	for _, e := range c.examples {
		fmt.Fprintf(&b, "  %s\n", e)
	}
	fmt.Fprintf(&b, "\nReport issues at %s\n", issues)
	return b.String()
}

func writeFlags(w io.Writer, flags []flagSpec) {
	for _, f := range flags {
		name := "--" + f.name
		if f.name == "help" {
			name = "-h, --help"
		}
		if f.arg != "" {
			name += " " + f.arg
		}
		if len(f.choices) > 0 {
			name += "[=" + strings.Join(f.choices, "|") + "]"
		}
		help := f.help
		if f.def != "" {
			help += fmt.Sprintf(" (default %s)", f.def)
		}
		if f.repeat {
			help += "; repeatable"
		}
		if len(name) > 26 {
			fmt.Fprintf(w, "  %s\n  %-26s %s\n", name, "", help)
		} else {
			fmt.Fprintf(w, "  %-26s %s\n", name, help)
		}
	}
}

// closest is the candidate nearest to word, or "" when none is near enough
// to be what was meant. A word that only ends differently, such as mutate
// for mutation, is near: all but its last letter, four letters at least,
// begin exactly one candidate.
func closest(word string, candidates []string) string {
	best, bestDist := "", 3
	for _, c := range candidates {
		if d := distance(word, c); d < bestDist {
			best, bestDist = c, d
		}
	}
	if best != "" || len(word) < 5 {
		return best
	}
	stem := word[:len(word)-1]
	for _, c := range candidates {
		if strings.HasPrefix(c, stem) {
			if best != "" {
				return ""
			}
			best = c
		}
	}
	return best
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

// distance is the Levenshtein distance between a and b.
func distance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
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
	fmt.Fprintf(&b, "\nReport issues at %s\n", issues)
	return b.String()
}
