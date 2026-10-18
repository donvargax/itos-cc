package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/kong"
)

// dispatched is a model of a program's commands, out of their names' order:
// a group of two subcommands, a command and version.
type dispatched struct {
	Units struct{} `cmd:"" help:"list the units"`
	Group struct {
		Run  struct{} `cmd:"" help:"run it"`
		List struct{} `cmd:"" help:"list it"`
	} `cmd:"" help:"a group"`
	Crap    struct{} `cmd:"" help:"score it"`
	Version struct{} `cmd:"" help:"print the version"`
}

func dispatchModel(t *testing.T) *kong.Application {
	t.Helper()
	parser, err := kong.New(&dispatched{}, kong.Name("itos-cc"), kong.Exit(func(int) {}))
	if err != nil {
		t.Fatal(err)
	}
	return parser.Model
}

// A command's line is its name and the words after it, --json last when it
// came before the command; --version is the version command, before
// whatever followed it.
func TestDispatchRoutesACommandsLine(t *testing.T) {
	app := dispatchModel(t)
	for args, want := range map[string][]string{
		"units src --changed":    {"units", "src", "--changed"},
		"--json units src":       {"units", "src", "--json"},
		"--json group run --top": {"group", "run", "--top", "--json"},
		"--version":              {"version"},
		"--json --version":       {"version", "--json"},
		"--version extra":        {"version", "extra"},
		"--version --version":    {"version", "version"},
		"units -- --json":        {"units", "--", "--json"},
	} {
		r := Dispatch(app, strings.Fields(args))
		if r.Problem != nil || r.Usage || r.Help != nil || !slices.Equal(r.Line, want) {
			t.Errorf("%s: %+v, want the line %q", args, r, want)
		}
	}
}

// No command, -h or --help before one, and help alone are the top-level
// help; --json alone is command.missing, as its object.
func TestDispatchFindsTheTopLevelHelp(t *testing.T) {
	app := dispatchModel(t)
	for _, args := range []string{"", "-h", "--help", "--json --help", "--version --help", "help", "--json help", "help --json"} {
		if r := Dispatch(app, strings.Fields(args)); !r.Usage || r.Problem != nil || r.Line != nil {
			t.Errorf("%q: %+v, want the top-level help", args, r)
		}
	}
	r := Dispatch(app, []string{"--json"})
	if r.Problem == nil || r.Problem.Rule != "command.missing" || !r.JSON || r.Problem.Fix != "Name a command; run 'itos-cc --help' for them." {
		t.Errorf("--json alone: %+v, want command.missing as its object", r)
	}
}

// help names a command known to the model, a group one of its subcommands
// after it, and nothing more.
func TestDispatchReadsHelpsTopics(t *testing.T) {
	app := dispatchModel(t)
	for args, want := range map[string][]string{
		"help units":          {"units"},
		"help group":          {"group"},
		"help group run":      {"group", "run"},
		"help --json crap":    {"crap"},
		"--json help crap":    {"crap"},
		"help version":        {"version"},
		"help group --json":   {"group"},
		"help --json group l": nil,
	} {
		r := Dispatch(app, strings.Fields(args))
		if want == nil {
			if r.Problem == nil || r.Problem.Rule != "command.unknown" {
				t.Errorf("%s: %+v, want command.unknown", args, r)
			}
			continue
		}
		if r.Problem != nil || !slices.Equal(r.Help, want) {
			t.Errorf("%s: %+v, want the help of %q", args, r, want)
		}
	}
	for args, extra := range map[string]string{"help units extra": "extra", "help group run more": "more"} {
		r := Dispatch(app, strings.Fields(args))
		if r.Problem == nil || r.Problem.Rule != "args.unexpected" || r.Problem.Subject("argument") != extra ||
			r.Problem.Message != `help takes one command, and was given "`+extra+`" too` {
			t.Errorf("%s: %+v, want args.unexpected naming %q", args, r.Problem, extra)
		}
	}
	r := Dispatch(app, strings.Fields("--json help group rn"))
	if r.Problem == nil || !r.JSON || r.Problem.Fix != "Did you mean 'group run'? The subcommands of group are run and list. Run 'itos-cc group --help' for them." {
		t.Errorf("help group rn: %+v, want the subcommand meant, as the object", r.Problem)
	}
}

// A word no command of the model has is command.unknown, naming the one
// meant when it is near enough; a flag before the command other than
// --json, help and --version is flags.unknown, whatever follows it.
func TestDispatchRefusesWhatTheModelLacks(t *testing.T) {
	app := dispatchModel(t)
	for args, fix := range map[string]string{
		"crapp":       "Did you mean 'crap'? Run 'itos-cc --help' for the commands.",
		"lint":        "Run 'itos-cc --help' for the commands.",
		"help nosuch": "Run 'itos-cc --help' for the commands.",
		"-- crap":     "Run 'itos-cc --help' for the commands.",
		"vresion":     "Did you mean 'version'? Run 'itos-cc --help' for the commands.",
	} {
		r := Dispatch(app, strings.Fields(args))
		if r.Problem == nil || r.Problem.Rule != "command.unknown" || r.Problem.Code != CodeUnknownCommand || r.Problem.Fix != fix {
			t.Errorf("%s: %+v, want command.unknown with %q", args, r.Problem, fix)
		}
	}
	for _, args := range []string{"--bogus units", "--json --bogus", "--version --bogus", "-x"} {
		r := Dispatch(app, strings.Fields(args))
		word := strings.Fields(args)[slices.IndexFunc(strings.Fields(args), func(w string) bool { return w == "--bogus" || w == "-x" })]
		if r.Problem == nil || r.Problem.Rule != "flags.unknown" || r.Problem.Message != "itos-cc takes no flag "+word+" before a command" ||
			r.Problem.Fix != "Put a command's flags after its name; run 'itos-cc --help' for the commands." {
			t.Errorf("%s: %+v, want flags.unknown naming %s", args, r.Problem, word)
		}
	}
	if r := Dispatch(app, []string{"--json", "--bogus"}); !r.JSON {
		t.Errorf("--json --bogus: %+v, want the problem as the object", r)
	}
	if r := Dispatch(app, []string{"--bogus", "--json"}); r.JSON {
		t.Errorf("--bogus --json: %+v, want the problem plain: --json came after it", r)
	}
}

// The top-level help lists each command by its help, sorted by name, a
// group's subcommands in its place in the group's order.
func TestUsageListsEveryCommandSorted(t *testing.T) {
	usage := Usage(dispatchModel(t))
	want := "usage: itos-cc <command> [options] [path ...]\n\nCommands:\n" +
		"  crap            score it\n" +
		"  group run       run it\n" +
		"  group list      list it\n" +
		"  units           list the units\n" +
		"  version         print the version\n\n"
	if !strings.HasPrefix(usage, want) {
		t.Errorf("the usage begins\n%s\nwant\n%s", usage[:min(len(usage), len(want))], want)
	}
	for _, part := range []string{"'itos-cc help <command>'", "  100 an internal error itos-cc could not classify", "  itos-cc mutation run --changed --json\n", "\nReport issues at " + Issues + "\n"} {
		if !strings.Contains(usage, part) {
			t.Errorf("the usage lacks %q", part)
		}
	}
}
