// Package count is the count command, a slice named for it: its kong struct
// and Run are the UI, and its Query and Handler the application.
//
// Run assembles the Query from the command line, the one copy of a command
// line's values, and hands it to Handle with the context cmd made; Handle
// only wires the Query to the domain through the Files port it was given,
// and Run writes what came back. The slice counts nothing and reads no file.
// A failure is the domain's: Classify, which cmd hands to internal/cli,
// turns its kind into a problem and its exit code, and Run reports it
// through the UI.
package count

import (
	"context"
	"fmt"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/counting"
	"github.com/donvargax/itos-cc/internal/counting/port"
)

// CLI is the count command's command line: the file it counts, and whether
// it prints the counts or the one object a script reads.
type CLI struct {
	File string `arg:"" help:"The file to count, or - for standard input."`
	JSON bool   `name:"json" negatable:"" env:"ITOS_CC_JSON" help:"Print one JSON object instead of the counts."`
}

// Help is count's own help: the shape of its --json object and every exit
// code it can return, so the two agree without a reader holding the source.
// The wording is what the help must hold, because a scenario checks it and a
// script reads it.
//
// The block is indented, so it is preformatted and holds its lines whole at
// any terminal width; only the sentence above it is re-wrapped.
func (CLI) Help() string {
	return "\nThe counts are for a person to read; a script reads --json and the exit code.\n\n" +
		"    --json prints one object: schema 1, ok true, file the argument, lines and words.\n" +
		"    Exit codes:\n" +
		"      0  success\n" +
		"      2  the command line is wrong\n" +
		"      3  the input is missing or cannot be read\n" +
		"      130  interrupted\n"
}

// counts is the one object --json prints on success, the contract a script
// reads: the argument as given, - for standard input, and what the domain
// counted of it.
type counts struct {
	Schema int    `json:"schema"`
	OK     bool   `json:"ok"`
	File   string `json:"file"`
	Lines  int    `json:"lines"`
	Words  int    `json:"words"`
}

// Query is what the count command asks of the domain: the file to count,
// its path as the argument gave it, - for standard input.
type Query struct {
	Path string
}

// Handler is the count command's application: the Files port the domain
// reads through, given by cmd, which chooses the adapters behind it. No
// value of a command line is read here.
type Handler struct {
	Files port.Files
}

// Handle counts the file q names through the domain and the Files port,
// passing ctx on to them. It only wires: the counting is the domain's.
func (h Handler) Handle(ctx context.Context, q Query) (counting.Result, error) {
	return counting.File(ctx, h.Files, q.Path)
}

// Run counts the file the command names, through h and with the context
// cmd made, or reports the failure it met, and returns the exit code the UI
// gives it.
func (c CLI) Run(ctx context.Context, ui *cli.UI, h Handler) int {
	result, failure := h.Handle(ctx, Query{Path: c.File})
	if failure != nil {
		return ui.Fail(failure, c.JSON)
	}
	if c.JSON {
		ui.JSON(counts{Schema: 1, OK: true, File: c.File, Lines: result.Lines, Words: result.Words})
		return 0
	}
	// The counts, and the path as given, never what was read: an input
	// holding a secret must not reach the output.
	_, _ = fmt.Fprintf(ui.Stdout, "%s: lines %d, words %d\n", c.File, result.Lines, result.Words)
	ui.Line("for scripts, use --json")
	return 0
}
