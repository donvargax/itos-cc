package cli

import "fmt"

// Version is the version command: whether it prints the text --version
// prints, or the one object a script reads. --version is kong's flag, an
// action that prints the text alone, so only the command takes --json.
type Version struct {
	JSON bool `name:"json" negatable:"" env:"ITOS_CC_JSON" help:"Print one JSON object instead of the version."`
}

// Help is version's own help: the shape of its --json object and the exit
// codes it can return, as count's help gives its own. A scenario checks the
// wording and a script reads it.
//
// The block is indented, so it is preformatted and holds its lines whole at
// any terminal width; only the sentence above it is re-wrapped.
func (Version) Help() string {
	return "\nThe version is for a person to read; a script reads --json and the exit code.\n\n" +
		"    --json prints one object: schema 1, ok true, version, and commit when the build knows it.\n" +
		"    Exit codes:\n" +
		"      0  success\n" +
		"      2  the command line is wrong\n"
}

// build is the one object --json prints, the contract a script reads: the
// version the binary says it is and the commit it was built from, the key
// left out when the build knows none, as the text then has no second line.
type build struct {
	Schema  int    `json:"schema"`
	OK      bool   `json:"ok"`
	Version string `json:"version"`
	Commit  string `json:"commit,omitempty"`
}

// Run prints what the binary says it is: text, as --version prints it, or
// with --json the object of version and commit, commit "" when the build
// knows none. It cannot fail.
func (v Version) Run(ui *UI, text, version, commit string) int {
	if v.JSON {
		ui.JSON(build{Schema: 1, OK: true, Version: version, Commit: commit})
		return 0
	}
	_, _ = fmt.Fprintln(ui.Stdout, text)
	return 0
}
