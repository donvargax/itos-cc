package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/kong"
)

// picker is a command that selects its files as every command does, by
// the Selection it embeds first, with a flag of its own after it.
type picker struct {
	Selection
	Top  string `placeholder:"N" help:"print only the N worst"`
	JSON bool   `name:"json" help:"print one JSON object"`
}

func (picker) Contract() Contract {
	return Contract{About: "Picks files.", Exits: []Exit{{0, "success"}}}
}

// An embedded Selection reads --changed and the paths wherever they stand,
// between the command's own flags too, and its help lists --changed first
// and the paths last on the usage line, as the commands that declared
// them in their own structs listed them.
func TestAnEmbeddedSelectionIsTheCommandsFirstFlagAndItsArguments(t *testing.T) {
	var app struct {
		Pick picker `cmd:"" help:"pick files"`
	}
	var out strings.Builder
	parser, err := kong.New(&app, append(Flags(), kong.Name("itos-cc"), kong.Help(Help),
		kong.Writers(&out, &out), kong.Exit(func(int) { t.Fatal("kong exited") }))...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(parser, strings.Fields("pick src --top 3 --changed lib")); err != nil {
		t.Fatal(err)
	}
	if got := app.Pick; !got.Changed || got.Top != "3" || !slices.Equal(got.Paths, []string{"src", "lib"}) {
		t.Errorf("parsed %+v, want --changed, --top 3 and src, lib", got)
	}
	traced, err := kong.Trace(parser, []string{"pick"})
	if err != nil {
		t.Fatal(err)
	}
	if err := traced.PrintUsage(false); err != nil {
		t.Fatal(err)
	}
	if want := "usage: itos-cc pick [options] [path ...]\n"; !strings.HasPrefix(out.String(), want) {
		t.Errorf("help:\n%s\nwant it to start %q", out.String(), want)
	}
	if want := "Options:\n  --changed                  only files git reports as added or modified\n  --top N"; !strings.Contains(out.String(), want) {
		t.Errorf("help:\n%s\nwant it to hold %q", out.String(), want)
	}
}
