package cli

import (
	"strings"
	"testing"

	"github.com/donvargax/kong"
)

// kong's printer is replaced: a command kong parses prints its help in the
// layout every command's has, from kong's model and the command's Contract.
func TestKongPrintsACommandsHelpAsItsPage(t *testing.T) {
	var out strings.Builder
	parser, err := kong.New(&probeApp{}, append(Flags(), kong.Name("itos-cc"), kong.Help(Help),
		kong.Writers(&out, &out), kong.Exit(func(int) {}),
		kong.PostBuild(func(k *kong.Kong) error {
			k.Model.HelpFlag.Help = "print this help"
			return nil
		}))...)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := kong.Trace(parser, []string{"probe"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.PrintUsage(false); err != nil {
		t.Fatal(err)
	}
	want := `usage: itos-cc probe [options] [path ...]

Probes things.

Options:
  --changed                  only changed files
  --top N                    print only the N worst
  --json                     print one JSON object
  -h, --help                 print this help

Output with --json, one object on stdout:
  {"schema": 1, "ok": true|false, "things": [],
   "problems": [{"rule", "message", "fix", "exit", and keys for its subject}]}
  "problems" is there only when ok is false. Scripts read the exit code and
  these keys, never "message", "fix", or the plain output.

Problem rules:
  probe.failed  a thing failed

Exit codes:
  0   success
  80  a usage error
  100 an internal error itos-cc could not classify; please report it
  130 interrupted by SIGINT or SIGTERM; on Linux and macOS it then ends by that signal, which a shell reads as 130 or 143

Examples:
  itos-cc probe src

Report issues at ` + Issues + "\n"
	if out.String() != want {
		t.Errorf("help:\n%s\nwant:\n%s", out.String(), want)
	}
}

// A command with no Contract has no help to print: a bug, said so.
func TestACommandWithNoContractHasNoPage(t *testing.T) {
	parser, err := kong.New(&probeApp{}, append(Flags(), kong.Name("itos-cc"))...)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := kong.Trace(parser, []string{"bare"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PageOf(ctx.Selected()); err == nil || !strings.Contains(err.Error(), `"bare"`) {
		t.Errorf("PageOf(bare) = %v, want the command named", err)
	}
	ctx, err = kong.Trace(parser, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := Help(kong.HelpOptions{}, ctx); err == nil {
		t.Error("help with no command printed")
	}
}

// A required argument is <name>, a flag's default and repetition are said,
// and a name too long for its column puts the description on a line of its
// own; a switch's choices follow it.
func TestAPageSaysWhatEachFlagAndArgumentIs(t *testing.T) {
	var app struct {
		Except struct {
			Reason string   `default:"none" help:"why"`
			Report []string `placeholder:"FILE" help:"a report"`
			Site   string   `arg:"" help:"the site"`
		} `cmd:""`
	}
	parser, err := kong.New(&app, append(Flags(), kong.Name("itos-cc"))...)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := kong.Trace(parser, []string{"except", "x"})
	if err != nil {
		t.Fatal(err)
	}
	node := ctx.Selected()
	if got := synopsis(node); got != "[options] <site>" {
		t.Errorf("synopsis %q", got)
	}
	var b strings.Builder
	WriteFlags(&b, []HelpFlag{
		{Name: "reason", Arg: "TEXT", Default: "none", Help: "why"},
		{Name: "report", Arg: "FILE", Repeat: true, Help: "a report"},
		{Name: "fail-uncovered", Choices: []string{"functions", "lines"}, Help: "say no"},
	})
	want := "  --reason TEXT              why (default none)\n" +
		"  --report FILE              a report; repeatable\n" +
		"  --fail-uncovered[=functions|lines]\n" +
		"                             say no\n"
	if b.String() != want {
		t.Errorf("flags:\n%s\nwant:\n%s", b.String(), want)
	}
	for _, f := range flagsOf(node) {
		if f.Name == "report" && !f.IsCumulative() {
			t.Error("a slice flag is not cumulative")
		}
	}
}
