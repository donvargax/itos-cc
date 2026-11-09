package mutation

import "github.com/donvargax/itos-cc/internal/cli"

// Contract is the show command's help contract.
func (Show) Contract() cli.Contract {
	return cli.Contract{
		About: `
Says where the mutant an identity names is and what it changes, running
nothing and writing nothing: no test, no git, no toolchain, no snapshot. An
identity, as mutation run --count names a mutant in its report, is
<file>:<namespace#name>@<start>:<end>:<hash>:<offset>:<original>><replacement>,
its offset counted with comments left out, so it cannot be read by counting
bytes from the function's start.

It is resolved as mutation except and mutation run --site resolve it, in the
working tree: its file from the project root, its function the one of that
name whose hash is the identity's, its lines choosing among several, so one
that only moved is still found, and its site the one whose whole key, the
offset and the change, is the identity's. A function that changed since, a
file, function or site that is gone, a test file, or a source the exclude of
itos-cc.yaml leaves out is site.unmatched. A file only another system builds
is shown: only running its tests needs it built.

Plain output is three lines:
  mutant ` + "`original` → `replacement`" + ` in <namespace#name>, <file> line <line> column <column>
    before: <the site's line, less the space around it>
    after:  <that line with the change made>`,
		JSON: `"mutant": {"identity", "file", "line", "column", "function",
   "original", "replacement", "line_text", "mutated_text"}, none on a refusal`,
		Rules: []string{
			"site.unmatched   the identity names no mutant of the code as it is now: argument, why: changed (with hash, the function's now)|gone|excluded",
			"config.invalid   itos-cc.yaml cannot be read: file, and key for an exclude glob",
			"args.missing     no identity given",
			"args.invalid     the argument is not a mutant's identity: argument",
			"args.unexpected  more than one identity: argument",
		},
		Exits: []cli.Exit{
			{Code: 0, Meaning: "the mutant is shown"},
			{Code: cli.CodeUsage, Meaning: "a usage or config error: a bad flag, no identity or more than one, an argument that is no identity, an identity the code as it is now does not have, or an itos-cc.yaml that cannot be read"},
		},
		Examples: []string{
			"itos-cc mutation show 'board/board.go:example.com/fx/board#Count@4:6:b1a8f41702cd975f:35:<><='",
			"itos-cc mutation show 'board/board.go:example.com/fx/board#Count@4:6:b1a8f41702cd975f:35:<><=' --json",
		},
	}
}
