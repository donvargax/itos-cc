package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/mutate"
	"github.com/donvargax/itos-cc/project"
)

var mutationExceptCommand = &command{
	name:     "mutation except",
	summary:  "except an equivalent mutant, a survivor no test can kill, with a reason",
	synopsis: "<file>:<line>:<column> --reason TEXT [options]",
	about: `
Excepts the survivor at <file>:<line>:<column> in itos-cc.yaml, at the
project root, with the reason no test can kill it: an equivalent mutant,
one whose change changes no behaviour, such as a 0 set again before it is
ever read. itos-cc.yaml is meant to be committed and reviewed like
the code. mutation run and mutation check then count the survivor as
excepted, not survived, and it fails nothing while its entry holds.

The site must be a survivor that its file's snapshot in .metrics/mutate/
records, fresh as mutation check judges it: run mutation run first. A site
recorded killed or uncovered, one whose function's results are stale or
missing, or no site at all, is refused: an uncovered mutant needs a test,
not a reason.

The entry, under mutation.exceptions, holds the file, the function, the
function's hash, the site's line counted from the function's first line
(line_in_function, 1 for the first), its column, the original, the
replacement, and the reason, so a function that only moves keeps it.
Excepting a site again replaces its entry; the file's other keys and its
comments are kept. An entry stops holding, and mutation run and mutation
check fail it as mutation.exception-stale, when its function changes, when
its function or site is gone, or when the tests now kill its mutant.

Plain output is one line, "excepted again" when it replaced an entry:
  excepted <file>:<line>:<column> ` + "`original` → `replacement`" + ` in <namespace#name> in itos-cc.yaml`,
	flags: []flagSpec{
		opt("reason", stringFlag, "TEXT", "", "why no test can kill the mutant; required"),
	},
	json: `"exception": {"file", "function", "hash",
   "line_in_function", "column", "original", "replacement", "reason"},
   "replaced": true|false`,
	rules: []string{
		"exception.no-survivor     the site is no survivor a fresh snapshot records: file, line, column",
		"config.invalid            itos-cc.yaml cannot be read: file",
		"flags.value-missing       no --reason: flag",
		"flags.value-invalid       an empty --reason: flag, value",
		"args.missing              no site given",
		"args.invalid              the site is not <file>:<line>:<column>: argument",
		"args.unexpected           more than one site: argument",
	},
	exits: []exitDoc{
		{0, "the survivor is excepted in itos-cc.yaml"},
		{2, "a usage or config error: no site or no --reason, a site that is no recorded survivor, or an itos-cc.yaml that cannot be read"},
	},
	examples: []string{
		"itos-cc mutation except src/board.ts:3:13 --reason 'c is set again before it is read'",
		"itos-cc mutation except src/billing/invoice.go:42:9 --reason 'the 0 is never read' --json",
	},
	run: runMutationExcept,
}

// exceptionJSON is an entry of itos-cc.yaml, with the same keys.
type exceptionJSON struct {
	File           string `json:"file"`
	Function       string `json:"function"`
	Hash           string `json:"hash"`
	LineInFunction int    `json:"line_in_function"`
	Column         int    `json:"column"`
	Original       string `json:"original"`
	Replacement    string `json:"replacement"`
	Reason         string `json:"reason"`
}

type exceptResult struct {
	Exception *exceptionJSON `json:"exception,omitempty"`
	Replaced  bool           `json:"replaced"`
}

// runMutationExcept writes the entry for a recorded survivor into
// itos-cc.yaml.
func runMutationExcept(in *invocation) (any, error) {
	var result exceptResult
	if !in.set("reason") {
		return result, fail(kindUsage, "flags.value-missing", "--reason is required: an exception says why no test can kill its mutant",
			"Give it, such as --reason 'the loop only counts up'.").with("flag", "--reason")
	}
	reason := in.str("reason")
	if strings.TrimSpace(reason) == "" {
		return result, fail(kindUsage, "flags.value-invalid", fmt.Sprintf("--reason needs a reason, and was given %q", reason),
			"Say why no test can kill the mutant.").with("flag", "--reason").with("value", reason)
	}
	switch len(in.args) {
	case 0:
		return result, fail(kindUsage, "args.missing", "mutation except needs the site to except, as <file>:<line>:<column>",
			"Name a survivor as mutation run lists it, such as src/board.ts:7:19.")
	case 1:
	default:
		return result, fail(kindUsage, "args.unexpected", fmt.Sprintf("mutation except takes one site, and was also given %q", in.args[1]),
			"Except one site at a time.").with("argument", in.args[1])
	}
	path, line, column, ok := parseSite(in.args[0])
	if !ok {
		return result, fail(kindUsage, "args.invalid", fmt.Sprintf("%q is not a site, <file>:<line>:<column>", in.args[0]),
			"Name a survivor as mutation run lists it, such as src/board.ts:7:19.").with("argument", in.args[0])
	}
	// An unreadable file is refused before any work.
	if _, err := loadExceptions(); err != nil {
		return result, err
	}
	tests, err := importingTests()
	if err != nil {
		return result, err
	}
	e, err := mutate.Survivor(path, line, column, tests)
	var none *mutate.NoSurvivorError
	if errors.As(err, &none) {
		rel := project.Rel(absOrSame(path))
		return result, fail(kindUsage, "exception.no-survivor", fmt.Sprintf("%s:%d:%d is no survivor to except: %s", rel, line, column, none.Reason),
			fmt.Sprintf("Except a site that mutation run lists as survived; run 'itos-cc mutation run %s' first when its results are stale or missing.", rel)).
			with("file", rel).with("line", line).with("column", column)
	}
	if err != nil {
		return result, err
	}
	e.Reason = reason
	replaced, err := config.Except(e)
	var invalid *config.InvalidError
	if errors.As(err, &invalid) {
		return result, fail(kindUsage, "config.invalid", invalid.Error(), "Fix it, then except the site again.").with("file", config.File)
	}
	if err != nil {
		return result, err
	}
	result = exceptResult{Exception: &exceptionJSON{File: e.File, Function: e.Function, Hash: e.Hash, LineInFunction: e.LineInFunction,
		Column: e.Column, Original: e.Original, Replacement: e.Replacement, Reason: e.Reason}, Replaced: replaced}
	if !in.json {
		what := "excepted"
		if replaced {
			what = "excepted again"
		}
		fmt.Printf("%s %s:%d:%d %s → %s in %s in %s\n", what, project.Rel(absOrSame(path)), line, column,
			quote(e.Original), quote(e.Replacement), e.Function, config.File)
	}
	return result, nil
}

// parseSite reads <file>:<line>:<column>, the numbers from the right so a
// Windows drive letter stays in the file.
func parseSite(arg string) (path string, line, column int, ok bool) {
	rest, col, found := cutLast(arg, ":")
	if !found {
		return "", 0, 0, false
	}
	path, ln, found := cutLast(rest, ":")
	if !found || path == "" {
		return "", 0, 0, false
	}
	line, err1 := strconv.Atoi(ln)
	column, err2 := strconv.Atoi(col)
	if err1 != nil || err2 != nil || line < 1 || column < 1 {
		return "", 0, 0, false
	}
	return path, line, column, true
}

func cutLast(s, sep string) (before, after string, found bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}
