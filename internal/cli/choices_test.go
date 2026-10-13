package cli

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/kong"
)

// A switch of choices reads as the hand parser read mutation run's
// --fail-uncovered: these hold each edge of the command line it answered,
// the answer it gave, to the rule, the sentences and the subject. No binary
// is compared per move (docs/go.md, step 9); the same lines join the
// refactor's final comparison.

type choices struct {
	FailUncovered *Choice  `enum:"functions,lines" help:"fail on uncovered mutants"`
	JSON          bool     `name:"json" help:"print one JSON object"`
	Paths         []string `arg:"" optional:"" name:"path" help:"files"`
}

func (choices) Contract() Contract { return Contract{About: "Runs mutants."} }

type choicesApp struct {
	Mutation struct {
		Run choices `cmd:"" help:"run mutants"`
	} `cmd:"" help:"mutation testing"`
}

// parseChoices is args parsed as choicesApp, as cmd parses a moved command,
// and the problem its failure is, or nil.
func parseChoices(t *testing.T, args string) (choices, *Problem) {
	t.Helper()
	app := &choicesApp{}
	parser, err := kong.New(app, append(Flags(), kong.Name("itos-cc"), kong.Help(Help),
		kong.Exit(func(int) { t.Fatal("kong exited") }))...)
	if err != nil {
		t.Fatal(err)
	}
	words := strings.Fields(args)
	if _, err := Parse(parser, words); err != nil {
		return app.Mutation.Run, ParseProblem(err, words)
	}
	return app.Mutation.Run, nil
}

func TestASwitchOfChoicesReadsAsTheHandParserDid(t *testing.T) {
	for _, c := range []struct {
		args   string
		choice string
		paths  []string
	}{
		{"mutation run", "", nil},
		{"mutation run --fail-uncovered", "functions", nil},
		{"mutation run --fail-uncovered=functions", "functions", nil},
		{"mutation run --fail-uncovered=lines src", "lines", []string{"src"}},
		{"mutation run --fail-uncovered lines", "functions", []string{"lines"}},
		{"mutation run a --fail-uncovered --json b", "functions", []string{"a", "b"}},
	} {
		got, p := parseChoices(t, c.args)
		choice := ""
		if got.FailUncovered != nil {
			choice = string(*got.FailUncovered)
		}
		if p != nil || choice != c.choice || !slices.Equal(got.Paths, c.paths) {
			t.Errorf("%s: choice %q, paths %q, %v; want %q and %q", c.args, choice, got.Paths, p, c.choice, c.paths)
		}
	}
}

func TestASwitchOfChoicesRefusesAsTheHandParserDid(t *testing.T) {
	invalid := "--fail-uncovered takes =functions or =lines, and was given %q"
	fix := "Write --fail-uncovered alone, or --fail-uncovered=functions."
	for _, c := range []struct {
		args, rule, message, fix string
		value                    any
	}{
		{"mutation run --fail-uncovered=x", "flags.value-invalid", fmt.Sprintf(invalid, "x"), fix, "x"},
		{"mutation run --fail-uncovered=", "flags.value-invalid", fmt.Sprintf(invalid, ""), fix, ""},
		{"mutation run --fail-uncovered=LINES src", "flags.value-invalid", fmt.Sprintf(invalid, "LINES"), fix, "LINES"},
		{"mutation run --fail-uncovered=lines --fail-uncovered", "flags.repeated", "--fail-uncovered is given more than once", "Give it once.", nil},
		{"mutation run --fail-uncovered --fail-uncovered=x", "flags.repeated", "--fail-uncovered is given more than once", "Give it once.", nil},
	} {
		_, p := parseChoices(t, c.args)
		if p == nil {
			t.Errorf("%s: parsed, want %s", c.args, c.rule)
			continue
		}
		if p.Code != CodeUsage || p.Rule != c.rule || p.Message != c.message || p.Fix != c.fix || p.Subject("flag") != "--fail-uncovered" || p.Subject("value") != c.value {
			t.Errorf("%s: %d %s %q %q flag %v value %v; want %s %q %q value %v",
				c.args, p.Code, p.Rule, p.Message, p.Fix, p.Subject("flag"), p.Subject("value"), c.rule, c.message, c.fix, c.value)
		}
	}
}

// Help lists a switch's choices from kong's model, after the switch, as
// the hand parser's help listed them.
func TestHelpListsASwitchsChoices(t *testing.T) {
	parser, err := kong.New(&choicesApp{}, append(Flags(), kong.Name("itos-cc"))...)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := kong.Trace(parser, []string{"mutation", "run"})
	if err != nil {
		t.Fatal(err)
	}
	page, err := PageOf(ctx.Selected())
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	WriteFlags(&b, page.Flags)
	want := "  --fail-uncovered[=functions|lines]\n                             fail on uncovered mutants\n"
	if !strings.HasPrefix(b.String(), want) {
		t.Errorf("help\n%s\nlacks %q", b.String(), want)
	}
}

// A switch of choices that names none has no choice to take alone: a bug
// in the model, refused before anything is parsed.
func TestASwitchOfChoicesNamesItsChoices(t *testing.T) {
	var app struct {
		Run struct {
			Fail *Choice `help:"fail"`
		} `cmd:""`
	}
	if _, err := kong.New(&app, append(Flags(), kong.Name("itos-cc"))...); err == nil || !strings.Contains(err.Error(), "fail is a switch of choices") {
		t.Errorf("kong.New = %v, want the switch named", err)
	}
}

// Completion offers a switch's choices after its =, and, the switch taking
// no next word, files after it.
func TestCompletionOffersASwitchsChoices(t *testing.T) {
	parser, err := kong.New(&choicesApp{}, append(Flags(), kong.Name("itos-cc"))...)
	if err != nil {
		t.Fatal(err)
	}
	for words, want := range map[string][]string{
		"mutation run --fail-uncovered=l": {"lines", ":none"},
		"mutation run --fail-uncovered=":  {"functions", "lines", ":none"},
		"mutation run --fail-uncovered ":  {":files"},
	} {
		if got := Complete(parser.Model, strings.Split(words, " ")); !slices.Equal(got, want) {
			t.Errorf("%q: %q, want %q", words, got, want)
		}
	}
}
