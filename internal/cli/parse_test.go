package cli

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/kong"
)

// The scenarios of a command moved onto kong hold its usage errors by
// their rule, message and exit code (cli.feature's ID-CLI-07). These hold
// each kind of kong's failure and of Flags' as the problem it becomes, for
// a command line of every shape: a command with switches, a flag that takes
// a value and arguments, one with no argument, and a group.

type probe struct {
	Changed bool     `help:"only changed files"`
	Top     string   `placeholder:"N" help:"print only the N worst"`
	JSON    bool     `name:"json" help:"print one JSON object"`
	Paths   []string `arg:"" optional:"" name:"path" help:"files"`
}

func (probe) Contract() Contract {
	return Contract{
		About:    "Probes things.",
		JSON:     `"things": []`,
		Rules:    []string{"probe.failed  a thing failed"},
		Exits:    []Exit{{0, "success"}, {CodeUsage, "a usage error"}},
		Examples: []string{"itos-cc probe src"},
	}
}

type bare struct {
	JSON bool `name:"json" help:"print one JSON object"`
}

type group struct {
	Run  struct{} `cmd:"" help:"run it"`
	List struct{} `cmd:"" help:"list it"`
}

type probeApp struct {
	Probe probe `cmd:"" help:"probe things"`
	Bare  bare  `cmd:"" help:"take nothing"`
	Group group `cmd:"" help:"a group"`
}

// newProbe is a parser of probeApp, as cmd makes one, and the app it fills.
func newProbe(t *testing.T) (*kong.Kong, *probeApp) {
	t.Helper()
	app := &probeApp{}
	parser, err := kong.New(app, append(Flags(), kong.Name("itos-cc"), kong.Help(Help),
		kong.Exit(func(int) { t.Fatal("kong exited") }))...)
	if err != nil {
		t.Fatal(err)
	}
	return parser, app
}

func TestReadFindsJSONAndHelpBeforeDashDash(t *testing.T) {
	for args, want := range map[string]Line{
		"probe --json":          {JSON: true},
		"probe --bogus -h":      {Help: true},
		"probe a --help --json": {JSON: true, Help: true},
		"probe -- --json -h":    {},
		"probe --json=yes":      {},
	} {
		if got := Read(strings.Fields(args)); got != want {
			t.Errorf("%s: %+v, want %+v", args, got, want)
		}
	}
}

// Arguments may stand between flags, a value is its flag's, and a word
// after -- is an argument whatever it looks like.
func TestArgumentsMayStandBetweenFlags(t *testing.T) {
	parser, app := newProbe(t)
	if _, err := Parse(parser, strings.Fields("probe a --changed b --top 3 - c -- --json d")); err != nil {
		t.Fatal(err)
	}
	if got := app.Probe; !got.Changed || got.Top != "3" || got.JSON || !slices.Equal(got.Paths, []string{"a", "b", "-", "c", "--json", "d"}) {
		t.Errorf("got %+v", got)
	}
}

func TestADashDashAloneLeavesNoArgument(t *testing.T) {
	parser, app := newProbe(t)
	if _, err := Parse(parser, []string{"probe", "--changed", "--"}); err != nil || len(app.Probe.Paths) != 0 {
		t.Errorf("got %+v, %v", app.Probe, err)
	}
}

// problemOf is the problem the command line args is.
func problemOf(t *testing.T, args ...string) *Problem {
	t.Helper()
	parser, _ := newProbe(t)
	_, err := Parse(parser, args)
	if err == nil {
		t.Fatalf("%q parsed", args)
	}
	return ParseProblem(err, args)
}

// Each failure is the problem cli.feature's ID-CLI-07 pins, word for word,
// its subject what it names.
func TestEachFailureIsItsProblemWordForWord(t *testing.T) {
	for _, c := range []struct {
		args         string
		rule, line   string
		key, subject string
	}{
		{"probe --no-such-flag", "flags.unknown", "probe takes no flag --no-such-flag. Run 'itos-cc probe --help' for its flags.", "flag", "--no-such-flag"},
		{"probe --chnged=1", "flags.unknown", "probe takes no flag --chnged=1. Did you mean --changed? Run 'itos-cc probe --help' for its flags.", "flag", "--chnged=1"},
		{"probe -changed", "flags.unknown", "probe takes no flag -changed. Did you mean --changed? Run 'itos-cc probe --help' for its flags.", "flag", "-changed"},
		{"probe -x", "flags.unknown", "probe takes no flag -x. Run 'itos-cc probe --help' for its flags.", "flag", "-x"},
		{"probe -hello", "flags.unknown", "probe takes no flag -hello. Did you mean --help? Run 'itos-cc probe --help' for its flags.", "flag", "-hello"},
		{"probe --top", "flags.value-missing", "--top needs a value. Give it one, such as --top N.", "flag", "--top"},
		{"probe --top --json", "flags.value-missing", "--top needs a value. Give it one, such as --top N.", "flag", "--top"},
		{"probe --changed=yes", "flags.switch-value", "--changed takes no value, and was given \"yes\". Write --changed alone.", "flag", "--changed"},
		{"probe --json=", "flags.switch-value", "--json takes no value, and was given \"\". Write --json alone.", "flag", "--json"},
		{"probe --top 3 --top 5", "flags.repeated", "--top is given more than once. Give it once.", "flag", "--top"},
		{"probe --changed --changed=yes", "flags.repeated", "--changed is given more than once. Give it once.", "flag", "--changed"},
		{"bare extra", "args.unexpected", "bare takes no arguments, and was given \"extra\". Run 'itos-cc bare' alone.", "argument", "extra"},
		{"crapp", "command.unknown", "there is no command \"crapp\". Run 'itos-cc --help' for the commands.", "command", "crapp"},
		{"prob", "command.unknown", "there is no command \"prob\". Did you mean 'probe'? Run 'itos-cc --help' for the commands.", "command", "prob"},
		{"group", "command.missing", "group needs a subcommand. Name one of run and list; run 'itos-cc group --help' for them.", "", ""},
	} {
		p := problemOf(t, strings.Fields(c.args)...)
		want := CodeUsage
		if c.rule == "command.unknown" {
			want = CodeUnknownCommand
		}
		if p.Rule != c.rule || p.plain() != c.line || p.Code != want {
			t.Errorf("%s: %s %q (exit %d), want %s %q", c.args, p.Rule, p.plain(), p.Code, c.rule, c.line)
		}
		if c.key != "" && p.Subject(c.key) != c.subject {
			t.Errorf("%s: %s is %v, want %q", c.args, c.key, p.Subject(c.key), c.subject)
		}
	}
}

// The first failure of the line is the one reported, left to right: a
// mistyped flag after another failure is not the one named.
func TestTheFirstFailureIsReported(t *testing.T) {
	for args, rule := range map[string]string{
		"probe --bogus -changed":    "flags.unknown",
		"probe --changed=1 -x":      "flags.switch-value",
		"probe -x --changed=1":      "flags.unknown",
		"probe --top -5":            "flags.value-missing",
		"probe --changed --changed": "flags.repeated",
	} {
		p := problemOf(t, strings.Fields(args)...)
		if p.Rule != rule {
			t.Errorf("%s: %s %q, want %s", args, p.Rule, p.plain(), rule)
		}
	}
	if p := problemOf(t, "probe", "--bogus", "-changed"); p.Subject("flag") != "--bogus" {
		t.Errorf("--bogus -changed names %v, want --bogus", p.Subject("flag"))
	}
}

// Another failure of the command line is a usage error in kong's words,
// and an error that is no parse failure is a bug.
func TestOtherFailuresAreUsageErrorsAndTheRestBugs(t *testing.T) {
	type required struct {
		Name string `required:"" help:"a name"`
	}
	var app struct {
		Need required `cmd:""`
	}
	parser, err := kong.New(&app, append(Flags(), kong.Name("itos-cc"), kong.Exit(func(int) {}))...)
	if err != nil {
		t.Fatal(err)
	}
	_, err = parser.Parse([]string{"need"})
	var missingFlags *kong.MissingFlagsError
	if p := ParseProblem(err, []string{"need"}); !errors.As(err, &missingFlags) || p.Rule != "usage" || p.Code != CodeUsage {
		t.Errorf("a missing required flag: %s %q, want a usage error", p.Rule, p.plain())
	}
	if p := ParseProblem(errors.New("no model"), nil); p.Rule != "internal" || p.Code != CodeInternal {
		t.Errorf("a model kong refuses: %s, want internal", p.Rule)
	}
}

// A kind no mapper of ours reads, such as a uint (a number flag is a float64
// or an int, numbers.go), is refused in kong's words.
func TestAValueKongsMapperRefusesIsInvalid(t *testing.T) {
	var app struct {
		Count struct {
			Top  uint   `help:"top"`
			Kind string `enum:"a,b" default:"a" help:"kind"`
		} `cmd:""`
	}
	for args, value := range map[string]any{"count --top=x": nil, "count --kind c": "c"} {
		parser, err := kong.New(&app, append(Flags(), kong.Name("itos-cc"), kong.Exit(func(int) {}))...)
		if err != nil {
			t.Fatal(err)
		}
		_, err = parser.Parse(strings.Fields(args))
		p := ParseProblem(err, strings.Fields(args))
		if p.Rule != "flags.value-invalid" || p.Subject("value") != value || !strings.Contains(p.Fix, "itos-cc count --help") {
			t.Errorf("%s: %s %q, value %v", args, p.Rule, p.plain(), p.Subject("value"))
		}
	}
}

func TestClosestGuessesWhatWasMeant(t *testing.T) {
	for word, want := range map[string]string{"crapp": "crap", "lint": "", "mutate": "mutation", "dr": "dry", "scrapers": ""} {
		if got := Closest(word, []string{"crap", "dry", "mutation", "scrap", "serve"}); got != want {
			t.Errorf("Closest(%q) = %q, want %q", word, got, want)
		}
	}
}

func TestRelNamesAPathFromTheWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	ui := &UI{Dir: dir}
	if got, want := ui.Rel(filepath.Join(dir, "src", "a.go")), filepath.Join("src", "a.go"); got != want {
		t.Errorf("Rel(a file under it) = %q, want %q", got, want)
	}
	outside := filepath.Join(filepath.Dir(dir), "elsewhere", "a.go")
	for _, path := range []string{outside, "b.go"} {
		if got := ui.Rel(path); got != path {
			t.Errorf("Rel(%q) = %q, want it as it is", path, got)
		}
	}
	if got := (&UI{}).Rel(outside); got != outside {
		t.Errorf("Rel with no working directory = %q, want %q", got, outside)
	}
}
