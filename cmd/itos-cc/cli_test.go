package main

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/donvargax/kong"

	"github.com/donvargax/itos-cc/internal/cli"
)

// testCommand takes what serve's flags are: a switch, a string, a whole
// number and a duration, the kinds the hand parser reads.
var testCommand = &command{
	name: "probe",
	flags: []flagSpec{
		sw("quiet", ""),
		opt("ui", stringFlag, "DIR", "", ""),
		opt("port", intFlag, "N", "7070", ""),
		opt("interval", durationFlag, "D", "500ms", ""),
	},
}

func TestFlagsAreReadByTheirSpec(t *testing.T) {
	t.Parallel()
	in, err := parse(testCommand, []string{"a", "--port", "3", "--interval=2s", "b", "--ui", "x", "--quiet", "--", "--port"})
	if err != nil {
		t.Fatal(err)
	}
	if in.integer("port") != 3 || in.duration("interval") != 2*time.Second || in.str("ui") != "x" || in.str("quiet") != "true" ||
		!slices.Equal(in.args, []string{"a", "b", "--port"}) {
		t.Errorf("values %v args %q", in.values, in.args)
	}
	// A flag not given is its default.
	in, err = parse(testCommand, nil)
	if err != nil || in.integer("port") != 7070 || in.duration("interval") != 500*time.Millisecond || in.str("ui") != "" {
		t.Errorf("defaults: %v, values %v", err, in.values)
	}
}

func TestBadFlagsAreUsageErrors(t *testing.T) {
	t.Parallel()
	for args, rule := range map[string]string{
		"--bogus":             "flags.unknown",
		"-quiet":              "flags.unknown",
		"--port":              "flags.value-missing",
		"--port --quiet":      "flags.value-missing",
		"--port=x":            "flags.value-invalid",
		"--interval 5":        "flags.value-invalid",
		"--quiet=yes":         "flags.switch-value",
		"--port 1 --port 2":   "flags.repeated",
		"--quiet --quiet":     "flags.repeated",
		"--ui a --ui=b":       "flags.repeated",
		"--json --json":       "flags.repeated",
		"--interval=1s --top": "flags.unknown",
	} {
		_, err := parse(testCommand, strings.Fields(args))
		p, ok := err.(*cli.Problem)
		if !ok || p.Rule != rule || p.Code != cli.CodeUsage {
			t.Errorf("%s: %v, want %s, a usage error", args, err, rule)
		}
	}
	_, err := parse(testCommand, []string{"-quiet"})
	if p := err.(*cli.Problem); !strings.Contains(p.Fix, "Did you mean --quiet?") {
		t.Errorf("fix %q, want the flag that was meant", p.Fix)
	}
}

func TestHelpAnywhereButAfterDashDash(t *testing.T) {
	t.Parallel()
	if _, err := parse(testCommand, []string{"a", "--port", "1", "-h"}); err != errHelp {
		t.Errorf("-h after other flags: %v, want help", err)
	}
	if _, err := parse(testCommand, []string{"--", "-h"}); err == errHelp {
		t.Error("-h after -- is an argument, not help")
	}
}

// stdout runs fn and returns what it printed on stdout. The pipe is read
// while fn writes, as a Windows pipe holds less than a long help.
func stdout(t *testing.T, fn func()) string {
	t.Helper()
	out, _ := captured(t, fn)
	return out
}

func TestDispatch(t *testing.T) {
	t.Parallel()
	useDir(t, t.TempDir())
	for _, c := range []struct {
		args []string
		code int
		want string
	}{
		{nil, 0, "Commands:"},
		{[]string{"--help"}, 0, "Commands:"},
		{[]string{"help", "crap"}, 0, "usage: itos-cc crap"},
		{[]string{"crap", "--help"}, 0, "Exit codes:"},
		{[]string{"help", "nosuch"}, 81, ""},
		{[]string{"--json", "crapp"}, 81, `"Did you mean 'crap'?`},
		{[]string{"--json", "version"}, 0, `"version"`},
		{[]string{"version", "extra"}, 80, ""},
		{nil, 0, "mutation run "},
		{[]string{"mutation", "-h"}, 0, "Subcommands:"},
		{[]string{"mutation", "--help"}, 0, "Subcommands:"},
		{[]string{"help", "mutation"}, 0, "Subcommands:"},
		{[]string{"mutation", "run", "--help"}, 0, "usage: itos-cc mutation run"},
		{[]string{"mutation", "list", "-h"}, 0, "usage: itos-cc mutation list"},
		{[]string{"help", "mutation", "run"}, 0, "usage: itos-cc mutation run"},
		{[]string{"mutation", "check", "--help"}, 0, "usage: itos-cc mutation check"},
		{[]string{"help", "mutation", "check"}, 0, "usage: itos-cc mutation check"},
		{nil, 0, "mutation check "},
		{[]string{"mutation"}, 0, "  check "},
		{[]string{"mutation", "sample", "--help"}, 0, "usage: itos-cc mutation sample"},
		{[]string{"help", "mutation", "sample"}, 0, "usage: itos-cc mutation sample"},
		{nil, 0, "mutation sample "},
		{[]string{"mutation"}, 0, "  sample "},
		{[]string{"--json", "mutation", "sampel"}, 81, `"Did you mean 'mutation sample'?`},
		{[]string{"mutation", "except", "--help"}, 0, "usage: itos-cc mutation except"},
		{[]string{"help", "mutation", "except"}, 0, "usage: itos-cc mutation except"},
		{nil, 0, "mutation except "},
		{[]string{"mutation"}, 0, "  except "},
		{[]string{"--json", "mutation", "excpet"}, 81, `"Did you mean 'mutation except'?`},
		{[]string{"--json", "mutation", "chek"}, 81, `"Did you mean 'mutation check'?`},
		{[]string{"help", "mutation", "nosuch"}, 81, ""},
		{[]string{"help", "crap", "run"}, 80, ""},
		{[]string{"--json", "mutation"}, 80, `"command.missing"`},
		{[]string{"mutation", "--json"}, 80, `"command.missing"`},
		{[]string{"--json", "mutation", "rn"}, 81, `"Did you mean 'mutation run'?`},
		{[]string{"--json", "mutation", "--changed", "run"}, 80, `"flags.unknown"`},
		{nil, 0, "  units           list the functions"},
		{[]string{"units", "--help"}, 0, "usage: itos-cc units [options] [path ...]"},
		{[]string{"help", "units"}, 0, "usage: itos-cc units [options] [path ...]"},
		{[]string{"help", "units", "extra"}, 80, ""},
		{[]string{"--json", "unit"}, 81, `"Did you mean 'units'?`},
		{[]string{"--json", "units", "-changed"}, 80, `"Did you mean --changed?`},
		{nil, 0, "  serve           live architecture graph"},
		{nil, 0, "  version         print the version"},
		{[]string{"help", "serve"}, 0, "usage: itos-cc serve [options] [repo ...]"},
		{[]string{"help", "serve", "extra"}, 80, ""},
		{[]string{"--json", "serve", "--port", "x"}, 80, `"flags.value-invalid"`},
		{[]string{"--json", "serv"}, 81, `"Did you mean 'serve'?`},
		{[]string{"help", "version"}, 0, "usage: itos-cc version [options]"},
		{[]string{"version", "-h", "extra"}, 0, "usage: itos-cc version [options]"},
		{[]string{"--json", "version", "--bogus"}, 80, `"flags.unknown"`},
		{[]string{"--json", "--version"}, 0, `"version"`},
		{[]string{"--json", "vresion"}, 81, `"Did you mean 'version'?`},
	} {
		o := itosCc(t, c.args...)
		code, out := o.code, o.stdout
		if code != c.code || !strings.Contains(out, c.want) {
			t.Errorf("itos-cc %q: exit %d, want %d; stdout %q, want it to contain %q", c.args, code, c.code, out, c.want)
		}
	}
}

func TestEveryCommandsHelpGivesItsContract(t *testing.T) {
	t.Parallel()
	contract := []string{"Options:", "--json", `"schema": 1`, "Exit codes:", "  0 ", "  80 ", "  100 ", "Examples:", cli.Issues}
	// serve, on its legacy handler, prints its own, in the same layout.
	for _, c := range legacy {
		h := c.help()
		for _, part := range append([]string{"usage: itos-cc " + c.name}, contract...) {
			if !strings.Contains(h, part) {
				t.Errorf("%s --help lacks %q", c.name, part)
			}
		}
	}
	// Every other command prints its from kong's model and its Contract.
	parser, err := parserOf(t.Context(), &commandLine{}, stdUI(), diskAdapters(io.Discard))
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range modelLeaves(parser) {
		name := node.Path()
		if legacy[name] != nil {
			continue
		}
		page, err := cli.PageOf(node)
		if err != nil {
			t.Fatal(err)
		}
		h := page.String()
		for _, part := range append([]string{"usage: itos-cc " + name}, contract...) {
			if !strings.Contains(h, part) {
				t.Errorf("%s --help lacks %q", name, part)
			}
		}
	}
	for name, node := range commandNodes(parser) {
		if len(node.Children) == 0 {
			continue
		}
		h, err := cli.GroupHelp(node)
		if err != nil {
			t.Fatal(err)
		}
		for _, part := range []string{"usage: itos-cc " + name + " <subcommand>", "Subcommands:", "--json", "command.missing", "Exit codes:", "  0 ", "  80 ", "  81 ", "Examples:", cli.Issues} {
			if !strings.Contains(h, part) {
				t.Errorf("%s --help lacks %q", name, part)
			}
		}
	}
}

// The model names every command once, in their names' order, which the
// guess for an unknown command weighs a tie by, and the line the top-level
// help lists serve by is its legacy handler's summary.
func TestTheModelNamesEveryCommandInOrder(t *testing.T) {
	t.Parallel()
	parser, err := parserOf(t.Context(), &commandLine{}, stdUI(), adapters{})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, node := range parser.Model.Children {
		names = append(names, node.Name)
	}
	if want := []string{"crap", "dry", "mutation", "scrap", "serve", "units", "version"}; !slices.Equal(names, want) {
		t.Errorf("the model's commands are %q, want %q", names, want)
	}
	for name, cmd := range legacy {
		if node := commandNodes(parser)[name]; node == nil || node.Help != cmd.summary {
			t.Errorf("the model lists %s as %v, want its summary %q", name, node, cmd.summary)
		}
	}
}

func TestAPanicIsAnInternalError(t *testing.T) {
	// Serial: it swaps os.Stdout to read what execute prints.
	boom := &command{name: "boom", run: func(context.Context, *invocation) (any, error) { panic("oops") }}
	var code int
	out := stdout(t, func() { code = execute(t.Context(), boom, []string{"--json"}, diskAdapters(io.Discard)) })
	if code != 100 || !strings.Contains(out, `"rule": "internal"`) || !strings.Contains(out, "boom panicked: oops") {
		t.Errorf("exit %d, stdout %q: want exit 100 and an internal problem", code, out)
	}
}

// modelLeaves are the executable command nodes, including group children.
func modelLeaves(parser *kong.Kong) []*kong.Node {
	var out []*kong.Node
	var visit func(*kong.Node)
	visit = func(node *kong.Node) {
		if len(node.Children) == 0 {
			out = append(out, node)
			return
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	for _, node := range parser.Model.Children {
		visit(node)
	}
	slices.SortFunc(out, func(a, b *kong.Node) int { return strings.Compare(a.Path(), b.Path()) })
	return out
}
