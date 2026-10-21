package version

import (
	"fmt"

	"github.com/donvargax/itos-cc/internal/cli"
)

// Version is the version command's command line: whether it prints the
// version as a person reads it, or the one object a script reads. It takes
// no arguments; --version before any command is the same command (Dispatch).
type Version struct {
	JSON bool `name:"json" help:"print one JSON object on stdout instead of text (shape below)"`
}

// Build is the version the binary says it is, which cmd works out from how
// it was built and binds for Version's Run: what a release stamped, the
// module version go install recorded, a checkout's pseudo-version, or dev.
type Build string

// Contract is what version's help says beyond its flags: the text it
// prints, the key --json adds and the exit codes it can return.
func (Version) Contract() cli.Contract {
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

// version is the object --json prints beside schema and ok.
type version struct {
	Version string `json:"version"`
}

// Run prints the version the binary says it is, build: the line
// "itos-cc <version>", or with --json the object of its version alone. It
// cannot fail.
func (v Version) Run(ui *cli.UI, build Build) int {
	if !v.JSON {
		_, _ = fmt.Fprintln(ui.Stdout, "itos-cc", string(build))
	}
	return ui.Emit(v.JSON, version{Version: string(build)}, nil)
}
