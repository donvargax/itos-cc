package cli

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/donvargax/kong"
)

// A command line kong parses is read as every command's is (docs/CLI.md,
// rules 9 and 15 to 25): --json and help before anything is judged, -h the
// only short flag, flags in any position, and each failure a problem of its
// own rule in the person's words. kong holds most of it; the rest is here,
// beside Flags, and told apart by the types of the kong fork's failures,
// never by their text (q-55).

// Line is what a command line asks for before kong reads it: the --json
// object, even for a usage error, and help, which -h or --help anywhere
// before -- asks for whatever else the line holds.
type Line struct {
	JSON, Help bool
}

// Read is what args ask for, read up to --.
func Read(args []string) Line {
	var line Line
	for _, a := range args {
		switch a {
		case "--":
			return line
		case "--json":
			line.JSON = true
		case "-h", "--help":
			line.Help = true
		}
	}
	return line
}

// MistypedFlagError is a word of one dash before --, other than - and -h:
// a long flag written with one dash, such as -changed, since -h is the only
// short flag. kong would read it as short flags, -c first, so Parse finds
// it first, against kong's model.
type MistypedFlagError struct {
	Word string
	// Command is the command whose flags the word is weighed against.
	Command *kong.Node
}

func (e *MistypedFlagError) Error() string { return "unknown flag " + e.Word }

// Parse parses args, a command line naming a command of the parser's model
// (units --changed src), with parser, which has parsed nothing before:
// Flags holds the state of one parse, so a parser parses once. It reads the
// line as every command does, where kong would not:
//
//   - A word of one dash before -- is a mistyped long flag
//     (MistypedFlagError), unless what comes before it fails first.
//   - Arguments may stand between flags: kong reads a command's arguments
//     in one run, so they are moved behind its flags, after a --, keeping
//     what each word is.
func Parse(parser *kong.Kong, args []string) (*kong.Context, error) {
	if i := slices.IndexFunc(beforeDashes(args), mistyped); i >= 0 {
		parsed, err := Parse(parser, args[:i])
		if err != nil {
			return nil, err
		}
		node := parsed.Selected()
		if node == nil {
			node = parsed.Model.Node
		}
		return nil, &MistypedFlagError{Word: args[i], Command: node}
	}
	return parser.Parse(flagsFirst(parser.Model, args))
}

// beforeDashes is args up to a --.
func beforeDashes(args []string) []string {
	if i := slices.Index(args, "--"); i >= 0 {
		return args[:i]
	}
	return args
}

// mistyped is whether word is a word of one dash that is no flag: neither
// -, an argument, nor -h.
func mistyped(word string) bool {
	return strings.HasPrefix(word, "-") && !strings.HasPrefix(word, "--") && word != "-" && word != "-h"
}

// flagsFirst is args with the arguments of the command they name moved
// behind its flags and a --: the command's words, then each flag with the
// value it takes as the next word, then the arguments in their order, then
// what followed a -- in args.
func flagsFirst(model *kong.Application, args []string) []string {
	node, i := model.Node, 0
	for ; i < len(args); i++ {
		child := command(node, args[i])
		if child == nil {
			break
		}
		node = child
	}
	out := append([]string{}, args[:i]...)
	var arguments []string
	rest, dashes := args[i:], false
	for j := 0; j < len(rest); j++ {
		word := rest[j]
		switch {
		case word == "--":
			arguments, dashes = append(arguments, rest[j+1:]...), true
			j = len(rest)
		case !strings.HasPrefix(word, "-") || word == "-":
			arguments = append(arguments, word)
		default:
			out = append(out, word)
			if takesNext(node, word) && j+1 < len(rest) && (!strings.HasPrefix(rest[j+1], "-") || rest[j+1] == "-") {
				j++
				out = append(out, rest[j])
			}
		}
	}
	if len(arguments) > 0 || dashes {
		out = append(append(out, "--"), arguments...)
	}
	return out
}

// command is node's command named word, or nil.
func command(node *kong.Node, word string) *kong.Node {
	for _, child := range node.Children {
		if child.Type == kong.CommandNode && (child.Name == word || slices.Contains(child.Aliases, word)) {
			return child
		}
	}
	return nil
}

// takesNext is whether word, a long flag of node's written without =,
// takes the next word as its value: a flag that is no switch.
func takesNext(node *kong.Node, word string) bool {
	name, _, hasValue := strings.Cut(strings.TrimPrefix(word, "--"), "=")
	if hasValue || !strings.HasPrefix(word, "--") {
		return false
	}
	for _, f := range flagsOf(node) {
		if f.Name == name {
			return !f.IsBool()
		}
	}
	return false
}

// ParseProblem is err, Parse's failure to parse args, as the problem
// docs/CLI.md names it, each kind told by its type:
//
//   - flags.unknown: kong's UnknownFlagError, or a MistypedFlagError,
//     naming the flag as written and the one meant when it can guess it.
//   - flags.switch-value, flags.value-missing and flags.repeated: Flags'
//     own refusals, and kong's MissingValueError of a flag.
//   - flags.value-invalid: a number flag's value that is no number of its
//     kind (NumberError), in the hand parser's words; a value kong's own
//     mapper refused, or not of its enum, in kong's words until a command
//     meets one.
//   - command.unknown, args.unexpected and command.missing: kong's
//     UnexpectedArgumentError, for a command or an argument, and its
//     MissingCommandError.
//
// Another failure of the command line, which no command can meet yet, is
// a usage problem in kong's words; any other error, such as a model kong
// refuses, is internal, a bug.
func ParseProblem(err error, args []string) *Problem {
	var (
		parse          *kong.ParseError
		mistyped       *MistypedFlagError
		unknown        *kong.UnknownFlagError
		repeated       *RepeatedFlagError
		switchValue    *SwitchValueError
		valueMissing   *ValueMissingError
		number         *NumberError
		missing        *kong.MissingValueError
		invalid        *kong.InvalidValueError
		enum           *kong.EnumError
		unexpected     *kong.UnexpectedArgumentError
		missingCommand *kong.MissingCommandError
	)
	if errors.As(err, &mistyped) {
		return unknownFlag(mistyped.Command, mistyped.Word)
	}
	if !errors.As(err, &parse) || parse.Context == nil {
		return Internal(err)
	}
	node := parse.Context.Selected()
	if node == nil {
		node = parse.Context.Model.Node
	}
	switch {
	case errors.As(err, &unknown):
		return unknownFlag(node, written(args, unknown.Flag))
	case errors.As(err, &repeated):
		return NewProblem(CodeUsage, "flags.repeated", fmt.Sprintf("--%s is given more than once", repeated.Flag.Name),
			"Give it once.").With("flag", "--"+repeated.Flag.Name)
	case errors.As(err, &switchValue):
		name := switchValue.Flag.Name
		return NewProblem(CodeUsage, "flags.switch-value", fmt.Sprintf("--%s takes no value, and was given %q", name, switchValue.Value),
			fmt.Sprintf("Write --%s alone.", name)).With("flag", "--"+name)
	case errors.As(err, &valueMissing):
		return needsAValue(valueMissing.Flag)
	case errors.As(err, &missing) && missing.Value != nil && missing.Value.Flag != nil:
		return needsAValue(missing.Value.Flag)
	case errors.As(err, &number) && number.Flag != nil:
		name, what := number.Flag.Name, number.What()
		return NewProblem(CodeUsage, "flags.value-invalid", fmt.Sprintf("--%s needs %s, and was given %q", name, what, number.Value),
			fmt.Sprintf("Give it %s.", what)).With("flag", "--"+name).With("value", number.Value)
	case errors.As(err, &invalid) && invalid.Value.Flag != nil:
		return valueInvalid(node, invalid.Value, err)
	case errors.As(err, &enum) && enum.Value.Flag != nil:
		return valueInvalid(node, enum.Value, err).With("value", enum.Got)
	case errors.As(err, &unexpected) && unexpected.ExpectedCommand:
		return unknownCommand(node, unexpected.Arg)
	case errors.As(err, &unexpected) && len(node.Positional) == 0:
		name := commandPath(node)
		return NewProblem(CodeUsage, "args.unexpected", fmt.Sprintf("%s takes no arguments, and was given %q", name, unexpected.Arg),
			fmt.Sprintf("Run '%s %s' alone.", programOf(node), name)).With("argument", unexpected.Arg)
	case errors.As(err, &missingCommand):
		return NewProblem(CodeUsage, "command.missing", fmt.Sprintf("%s needs a subcommand", commandPath(node)),
			fmt.Sprintf("Name one of %s; run '%s --help' for them.", andList(commandNames(node)), strings.TrimSpace(programOf(node)+" "+commandPath(node))))
	}
	return NewProblem(CodeUsage, "usage", err.Error(), fmt.Sprintf("Run '%s --help' for the commands and the flags each takes.", programOf(node)))
}

// unknownFlag is the problem of word, a flag node's command does not take,
// as it was written: the flag meant, when one is near enough to guess.
func unknownFlag(node *kong.Node, word string) *Problem {
	name, _, _ := strings.Cut(strings.TrimLeft(word, "-"), "=")
	var names []string
	for _, f := range flagsOf(node) {
		names = append(names, "--"+f.Name)
	}
	command := commandPath(node)
	fix := fmt.Sprintf("Run '%s %s --help' for its flags.", programOf(node), command)
	if guess := Closest("--"+name, names); guess != "" {
		fix = fmt.Sprintf("Did you mean %s? ", guess) + fix
	}
	return NewProblem(CodeUsage, "flags.unknown", fmt.Sprintf("%s takes no flag %s", command, word), fix).With("flag", word)
}

// written is flag, a long flag as kong names it (--name), as args wrote it:
// the first word before -- naming it, with its =value and every dash it was
// written with.
func written(args []string, flag string) string {
	name, ok := strings.CutPrefix(flag, "--")
	if !ok {
		return flag
	}
	for _, a := range beforeDashes(args) {
		if before, _, _ := strings.Cut(a, "="); strings.HasPrefix(a, "--") && strings.TrimLeft(before, "-") == name {
			return a
		}
	}
	return flag
}

// needsAValue is the problem of flag, which takes a value, given none.
func needsAValue(flag *kong.Flag) *Problem {
	return NewProblem(CodeUsage, "flags.value-missing", fmt.Sprintf("--%s needs a value", flag.Name),
		fmt.Sprintf("Give it one, such as --%s %s.", flag.Name, flag.FormatPlaceHolder())).With("flag", "--"+flag.Name)
}

// valueInvalid is the problem of a value a flag cannot take, in kong's
// words.
func valueInvalid(node *kong.Node, value *kong.Value, err error) *Problem {
	var parse *kong.ParseError
	if errors.As(err, &parse) {
		err = parse.Unwrap()
	}
	return NewProblem(CodeUsage, "flags.value-invalid", err.Error(),
		fmt.Sprintf("Run '%s %s --help' for its flags.", programOf(node), commandPath(node))).With("flag", "--"+value.Name)
}

// unknownCommand is the problem of word, no command of node's: the one
// meant, when one is near enough to guess.
func unknownCommand(node *kong.Node, word string) *Problem {
	full := strings.TrimSpace(commandPath(node) + " " + word)
	fix := fmt.Sprintf("Run '%s --help' for the commands.", strings.TrimSpace(programOf(node)+" "+commandPath(node)))
	if guess := Closest(word, commandNames(node)); guess != "" {
		fix = fmt.Sprintf("Did you mean '%s'? ", strings.TrimSpace(commandPath(node)+" "+guess)) + fix
	}
	return NewProblem(CodeUsage, "command.unknown", fmt.Sprintf("there is no command %q", full), fix).With("command", full)
}

// commandNames are the commands of node a person may name.
func commandNames(node *kong.Node) []string {
	var names []string
	for _, child := range node.Children {
		if child.Type == kong.CommandNode && !child.Hidden {
			names = append(names, child.Name)
		}
	}
	return names
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
