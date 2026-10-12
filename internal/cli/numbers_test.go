package cli

import (
	"strings"
	"testing"

	"github.com/donvargax/kong"
)

// The number flags read their values as the hand parser read dry's,
// crap's and mutation run's: these hold each edge of the command line it
// answered, the answer it gave, to the rule, the sentences and the subject.
// No binary is compared per move (docs/go.md, step 9); the same lines join
// the refactor's final comparison.

type numbers struct {
	Changed   bool     `help:"only files git reports as added or modified"`
	Threshold float64  `placeholder:"N" default:"0.82" help:"minimum similarity, 0–1"`
	MinLines  int      `name:"min-lines" placeholder:"N" default:"4" help:"skip functions shorter than N lines"`
	JSON      bool     `name:"json" help:"print one JSON object"`
	Paths     []string `arg:"" optional:"" name:"path" help:"files"`
}

func (numbers) Contract() Contract { return Contract{About: "Measures numbers."} }

type numbersApp struct {
	Dry numbers `cmd:"" help:"functions similar enough to be duplicates"`
}

// parseNumbers is args parsed as numbersApp, as cmd parses a moved command,
// and the problem its failure is, or nil.
func parseNumbers(t *testing.T, args string) (numbers, *Problem) {
	t.Helper()
	app := &numbersApp{}
	parser, err := kong.New(app, append(Flags(), kong.Name("itos-cc"), kong.Help(Help),
		kong.Exit(func(int) { t.Fatal("kong exited") }))...)
	if err != nil {
		t.Fatal(err)
	}
	words := strings.Fields(args)
	if _, err := Parse(parser, words); err != nil {
		return app.Dry, ParseProblem(err, words)
	}
	return app.Dry, nil
}

func TestNumberFlagsReadValuesAsTheHandParserDid(t *testing.T) {
	for _, c := range []struct {
		args      string
		threshold float64
		minLines  int
	}{
		{"dry", 0.82, 4},
		{"dry --threshold=-1", -1, 4},
		{"dry --threshold 1e3", 1000, 4},
		{"dry --threshold .5 src", 0.5, 4},
		{"dry --min-lines 010", 0.82, 10},
		{"dry --min-lines=-3", 0.82, -3},
		{"dry --min-lines +7", 0.82, 7},
	} {
		got, p := parseNumbers(t, c.args)
		if p != nil || got.Threshold != c.threshold || got.MinLines != c.minLines {
			t.Errorf("%s: threshold %v, min-lines %d, %v; want %v and %d", c.args, got.Threshold, got.MinLines, p, c.threshold, c.minLines)
		}
	}
}

func TestNumberFlagsRefuseAsTheHandParserDid(t *testing.T) {
	for _, c := range []struct {
		args, rule, message, fix, flag string
		value                          any
	}{
		{"dry --threshold -1", "flags.value-missing", "--threshold needs a value", "Give it one, such as --threshold N.", "--threshold", nil},
		{"dry --threshold", "flags.value-missing", "--threshold needs a value", "Give it one, such as --threshold N.", "--threshold", nil},
		{"dry --min-lines --json", "flags.value-missing", "--min-lines needs a value", "Give it one, such as --min-lines N.", "--min-lines", nil},
		{"dry --threshold=", "flags.value-invalid", `--threshold needs a number, and was given ""`, "Give it a number.", "--threshold", ""},
		{"dry --threshold abc", "flags.value-invalid", `--threshold needs a number, and was given "abc"`, "Give it a number.", "--threshold", "abc"},
		{"dry --threshold -", "flags.value-invalid", `--threshold needs a number, and was given "-"`, "Give it a number.", "--threshold", "-"},
		{"dry --min-lines 1.5", "flags.value-invalid", `--min-lines needs a whole number, and was given "1.5"`, "Give it a whole number.", "--min-lines", "1.5"},
		{"dry --min-lines=0x10", "flags.value-invalid", `--min-lines needs a whole number, and was given "0x10"`, "Give it a whole number.", "--min-lines", "0x10"},
		{"dry --min-lines 1_000", "flags.value-invalid", `--min-lines needs a whole number, and was given "1_000"`, "Give it a whole number.", "--min-lines", "1_000"},
		{"dry --threshold 0.5 --threshold 0.6", "flags.repeated", "--threshold is given more than once", "Give it once.", "--threshold", nil},
		{"dry --threshold 0.5 --threshold x", "flags.repeated", "--threshold is given more than once", "Give it once.", "--threshold", nil},
		{"dry -threshold", "flags.unknown", "dry takes no flag -threshold", "Did you mean --threshold? Run 'itos-cc dry --help' for its flags.", "-threshold", nil},
	} {
		_, p := parseNumbers(t, c.args)
		if p == nil {
			t.Errorf("%s: parsed, want %s", c.args, c.rule)
			continue
		}
		if p.Code != CodeUsage || p.Rule != c.rule || p.Message != c.message || p.Fix != c.fix || p.Subject("flag") != c.flag || p.Subject("value") != c.value {
			t.Errorf("%s: %d %s %q %q flag %v value %v; want %s %q %q flag %s value %v",
				c.args, p.Code, p.Rule, p.Message, p.Fix, p.Subject("flag"), p.Subject("value"), c.rule, c.message, c.fix, c.flag, c.value)
		}
	}
}

// Help shows a number flag's placeholder and default, as the hand parser's
// help listed them.
func TestHelpShowsANumberFlagsDefault(t *testing.T) {
	app := &numbersApp{}
	parser, err := kong.New(app, append(Flags(), kong.Name("itos-cc"))...)
	if err != nil {
		t.Fatal(err)
	}
	page, err := PageOf(parser.Model.Children[0])
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	WriteFlags(&b, page.Flags)
	for _, want := range []string{
		"  --threshold N              minimum similarity, 0–1 (default 0.82)\n",
		"  --min-lines N              skip functions shorter than N lines (default 4)\n",
	} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("help\n%s\nlacks %q", b.String(), want)
		}
	}
}
