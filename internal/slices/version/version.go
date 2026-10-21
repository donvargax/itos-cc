// Package version is the version command, a slice named for it: its kong
// struct and Run are the UI, and its Handler the application.
//
// The command prints the version the binary says it is, which the handler
// holds: cmd's adapters give it through the slice's Ports, read from
// internal/version, the one source a release's stamp and Go's module
// version reach. It reads no file, settings or tool, so it never fails.
//
// Command declares the command to cmd, which assembles it into its one
// model (ADR-0051), and Ports are the ports its handler is built over,
// which cmd's adapters satisfy.
package version

import (
	"context"
	"fmt"

	"github.com/donvargax/itos-cc/internal/cli"
)

// Command is the version command as cmd assembles it: its name, the line
// the top-level help lists it by, and a fresh command line for each parser.
var Command = cli.Declaration{
	Name: "version",
	Help: "print the version",
	New:  func() any { return &CLI{} },
}

// CLI is the version command's command line: whether it prints the version
// as a person reads it, or the one object a script reads. It takes no
// arguments; --version before any command is the same command
// (cli.Dispatch).
type CLI struct {
	JSON bool `name:"json" help:"print one JSON object on stdout instead of text (shape below)"`
}

// Contract is what version's help says beyond its flags: the text it
// prints, the key --json adds and the exit codes it can return.
func (CLI) Contract() cli.Contract {
	return cli.Contract{
		About: `Prints "itos-cc <version>". --version is the same.`,
		JSON:  `"version"`,
		Exits: []cli.Exit{
			{Code: 0, Meaning: "success"},
			{Code: cli.CodeUsage, Meaning: "a usage error: a bad flag or an argument"},
		},
		Examples: []string{"itos-cc version", "itos-cc --version --json"},
	}
}

// Handler is the version command's application: Version, the version the
// binary says it is, which cmd gives through Ports (NewHandler).
type Handler struct {
	Version string
}

// Ports are what the version command reaches the world through, which
// cmd's adapters give it: BuildVersion, the version this binary says it
// is, from how it was built: what a release stamped, the module version go
// install recorded, a checkout's pseudo-version, or dev, each without a
// leading v.
type Ports interface {
	BuildVersion() string
}

// NewHandler is the handler over p's version. It reads nothing, so it never
// fails; its error is there for the one shape every slice's constructor
// has.
func NewHandler(_ context.Context, p Ports) (Handler, error) {
	return Handler{Version: p.BuildVersion()}, nil
}

// printed is the object --json prints beside schema and ok.
type printed struct {
	Version string `json:"version"`
}

// Run prints the version h holds: the line "itos-cc <version>", or with
// --json the object of its version alone. It cannot fail.
func (c CLI) Run(ui *cli.UI, h Handler) int {
	if !c.JSON {
		_, _ = fmt.Fprintln(ui.Stdout, "itos-cc", h.Version)
	}
	return ui.Emit(c.JSON, printed{Version: h.Version}, nil)
}
