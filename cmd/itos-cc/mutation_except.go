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
	synopsis: "<file>:<line>:<column> --reason TEXT [options] | --renew [options]",
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
replacement, the reason, and the text of the site's line less the space
around it (line_text), so a function that only moves keeps it.
Excepting a site again replaces its entry; the file's other keys and its
comments are kept. An entry stops holding, and mutation run and mutation
check fail it as mutation.exception-stale, when its function changes, when
its function or site is gone, when its file is deleted or renamed, or when
the tests now kill its mutant. Only mutation except writes itos-cc.yaml.

--renew, run after mutation run, carries each entry that no longer holds
across a change that leaves its mutant the same, such as a rename, a
literal or comment edit, reformatting, or a line added elsewhere in the
function. It takes no site and no --reason. An entry is renewed only when
its function, found by namespace#name in its file, else by its name alone
there when exactly one function has it (a renamed module or package), or,
its file gone, in exactly one source with its name and hash, has fresh
results in its snapshot, and they hold exactly one site of its original
and replacement on a line whose text, less its space, is its line_text,
or of several such the one at its place; an entry with no line_text,
written before entries had one, matches the site at its line_in_function
and column, else the function's only site of its change. That mutant must
be recorded survived. The renewed entry names the function's current
hash, name, line_in_function, column and line text, and keeps its reason
and comments; nothing else is written, and an entry that holds is left
alone. Each renewal is reported with its old and new hash and the site.
Each entry left stale is mutation.exception-stale with why: killed (its
fresh results record its mutant killed), changed (no site of its change
is on a line of its text), gone (its file, function or change is gone),
ambiguous, no-results (its function has no fresh results) or uncovered.

Plain output is one line, "excepted again" when it replaced an entry:
  excepted <file>:<line>:<column> ` + "`original` → `replacement`" + ` in <namespace#name> in itos-cc.yaml
and with --renew a line per renewal and per entry left stale:
  renewed <file>:<line>:<column> ` + "`original` → `replacement`" + ` in <namespace#name> (hash <old> → <new>) in itos-cc.yaml
  not renewed <file>[:<line>:<column>] ` + "`original` → `replacement`" + ` in <namespace#name> (<why>)`,
	flags: []flagSpec{
		opt("reason", stringFlag, "TEXT", "", "why no test can kill the mutant; required, but with --renew"),
		sw("renew", "renew each stale entry whose mutant is unchanged, after mutation run, keeping its reason"),
	},
	json: `"exception": {"file", "function", "hash",
   "line_in_function", "column", "original", "replacement", "reason",
   "line_text"}, "replaced": true|false; with --renew instead
   "renewed": [{"file", "function", "line", "column", "line_in_function",
   "original", "replacement", "reason", "new_hash", "old_hash", "old_file",
   "old_function", "old_line_in_function", "old_column",
   "match": "line"|"place"|"only"}], "not_renewed": [{the entry's keys,
   "why", "line" when its site is found}]`,
	rules: []string{
		"exception.no-survivor     the site is no survivor a fresh snapshot records: file, line, column",
		"mutation.exception-stale  with --renew, an entry that no longer holds was not renewed: file, function, line (when its site is found), column, original, replacement, why: killed|changed|gone|ambiguous|no-results|uncovered",
		"flags.conflict            --renew with --reason: flag",
		"config.invalid            itos-cc.yaml cannot be read: file",
		"flags.value-missing       no --reason: flag",
		"flags.value-invalid       an empty --reason: flag, value",
		"args.missing              no site given",
		"args.invalid              the site is not <file>:<line>:<column>: argument",
		"args.unexpected           more than one site, or a site with --renew: argument",
	},
	exits: []exitDoc{
		{0, "the survivor is excepted in itos-cc.yaml; with --renew, no entry is left stale"},
		{1, "with --renew, an entry that no longer holds could not be renewed"},
		{2, "a usage or config error: no site or no --reason, a site that is no recorded survivor, --renew with a site or --reason, or an itos-cc.yaml that cannot be read"},
	},
	examples: []string{
		"itos-cc mutation except src/board.ts:3:13 --reason 'c is set again before it is read'",
		"itos-cc mutation except src/billing/invoice.go:42:9 --reason 'the 0 is never read' --json",
		"itos-cc mutation except --renew  # after mutation run, carry stale entries whose mutant is unchanged",
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
	LineText       string `json:"line_text,omitempty"`
}

type exceptResult struct {
	Exception *exceptionJSON `json:"exception,omitempty"`
	Replaced  bool           `json:"replaced"`
}

// runMutationExcept writes the entry for a recorded survivor into
// itos-cc.yaml, or with --renew renews the stale entries whose mutant is
// unchanged.
func runMutationExcept(in *invocation) (any, error) {
	if in.set("renew") {
		return runRenewExceptions(in)
	}
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
	cfg, err := loadConfig()
	if err != nil {
		return result, err
	}
	support, err := supportNow(cfg)
	if err != nil {
		return result, err
	}
	tests, err := importingTests()
	if err != nil {
		return result, err
	}
	e, err := mutate.Survivor(path, line, column, tests, support)
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
		Column: e.Column, Original: e.Original, Replacement: e.Replacement, Reason: e.Reason, LineText: e.LineText}, Replaced: replaced}
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

// renewedJSON is a renewal in --renew's --json object: the entry as it
// reads now, and the keys it had.
type renewedJSON struct {
	File              string `json:"file"`
	Function          string `json:"function"`
	Line              int    `json:"line"`
	Column            int    `json:"column"`
	LineInFunction    int    `json:"line_in_function"`
	Original          string `json:"original"`
	Replacement       string `json:"replacement"`
	Reason            string `json:"reason"`
	NewHash           string `json:"new_hash"`
	OldHash           string `json:"old_hash"`
	OldFile           string `json:"old_file"`
	OldFunction       string `json:"old_function"`
	OldLineInFunction int    `json:"old_line_in_function"`
	OldColumn         int    `json:"old_column"`
	Match             string `json:"match"`
}

// notRenewedJSON is an entry --renew left as it was, and why.
type notRenewedJSON struct {
	exceptionJSON
	Why  string `json:"why"`
	Line int    `json:"line,omitempty"`
}

type renewResult struct {
	Renewed    []renewedJSON    `json:"renewed"`
	NotRenewed []notRenewedJSON `json:"not_renewed"`
}

// runRenewExceptions is mutation except --renew: each entry of itos-cc.yaml
// that no longer holds is renewed when its mutant is unchanged
// (mutate.Renew), and only those entries are written. Every renewal is
// reported, and every stale entry left as it was is
// mutation.exception-stale, with why.
func runRenewExceptions(in *invocation) (any, error) {
	result := renewResult{Renewed: []renewedJSON{}, NotRenewed: []notRenewedJSON{}}
	if in.set("reason") {
		return result, fail(kindUsage, "flags.conflict", "--renew keeps each entry's own reason, and was given --reason",
			"Drop --reason; to except a survivor with a new reason, name its site without --renew.").with("flag", "--reason")
	}
	if len(in.args) > 0 {
		return result, fail(kindUsage, "args.unexpected", fmt.Sprintf("--renew renews every stale entry of %s, and takes no site, but was given %q", config.File, in.args[0]),
			"Drop the site, or drop --renew to except that survivor.").with("argument", in.args[0])
	}
	cfg, err := loadConfig()
	if err != nil {
		return result, err
	}
	if len(cfg.Exceptions) == 0 {
		return result, nil
	}
	support, err := supportNow(cfg)
	if err != nil {
		return result, err
	}
	tests, err := importingTests()
	if err != nil {
		return result, err
	}
	all, err := project.Discover([]string{project.Root()})
	if err != nil {
		return result, err
	}
	renewed, left, err := mutate.Renew(cfg.Exceptions, all.Sources, tests, support)
	if err != nil {
		return result, err
	}
	var writes []config.Renewal
	for _, r := range renewed {
		writes = append(writes, config.Renewal{Old: r.Old, New: r.New})
	}
	err = config.Renew(writes)
	var invalid *config.InvalidError
	if errors.As(err, &invalid) {
		return result, fail(kindUsage, "config.invalid", invalid.Error(), "Fix it, then renew again.").with("file", config.File)
	}
	if err != nil {
		return result, err
	}
	for _, r := range renewed {
		rel := project.Rel(project.AtRoot(r.New.File))
		result.Renewed = append(result.Renewed, renewedJSON{File: rel, Function: r.New.Function, Line: r.Line, Column: r.New.Column,
			LineInFunction: r.New.LineInFunction, Original: r.New.Original, Replacement: r.New.Replacement, Reason: r.New.Reason,
			NewHash: r.New.Hash, OldHash: r.Old.Hash, OldFile: r.Old.File, OldFunction: r.Old.Function,
			OldLineInFunction: r.Old.LineInFunction, OldColumn: r.Old.Column, Match: r.Match})
		if !in.json {
			fmt.Printf("renewed %s:%d:%d %s → %s in %s (hash %s → %s) in %s\n", rel, r.Line, r.New.Column,
				quote(r.New.Original), quote(r.New.Replacement), r.New.Function, r.Old.Hash, r.New.Hash, config.File)
		}
	}
	for _, u := range left {
		rel := project.Rel(project.AtRoot(u.File))
		result.NotRenewed = append(result.NotRenewed, notRenewedJSON{exceptionJSON: exceptionJSON{File: u.File, Function: u.Function,
			Hash: u.Hash, LineInFunction: u.LineInFunction, Column: u.Column, Original: u.Original, Replacement: u.Replacement,
			Reason: u.Reason, LineText: u.LineText}, Why: u.Why, Line: u.Line})
		at := rel
		if u.Line != 0 {
			at = fmt.Sprintf("%s:%d:%d", rel, u.Line, u.Column)
		}
		change := fmt.Sprintf("%s → %s in %s", quote(u.Original), quote(u.Replacement), u.Function)
		if !in.json {
			fmt.Printf("  not renewed %s %s (%s)\n", at, change, u.Why)
		}
		p := fail(kindNo, "mutation.exception-stale",
			fmt.Sprintf("%s: the entry in %s for %s was not renewed: %s", at, config.File, change, notRenewedBecause(u.Why)),
			notRenewedFix(u.Why))
		p.subject = map[string]any{"file": rel, "function": u.Function, "column": u.Column,
			"original": u.Original, "replacement": u.Replacement, "why": u.Why}
		if u.Line != 0 {
			p.subject["line"] = u.Line
		}
		p.shown = true
		in.report(p)
	}
	return result, nil
}

// notRenewedBecause says why --renew left a stale entry as it was.
func notRenewedBecause(why string) string {
	switch why {
	case mutate.ExceptionKilled:
		return "the fresh results record its mutant killed"
	case mutate.ExceptionChanged:
		return "no site of its change is on a line with the text it recorded: its line changed"
	case mutate.ExceptionGone:
		return "its file, its function, or any site of its change in it is gone"
	case mutate.RenewNoResults:
		return "its function has no fresh mutation results to show its mutant survived"
	case mutate.RenewAmbiguous:
		return "more than one site could be its mutant"
	case mutate.RenewUncovered:
		return "its mutant is recorded uncovered, which no exception excuses"
	}
	return why
}

// notRenewedFix says what to do about a stale entry --renew left.
func notRenewedFix(why string) string {
	switch why {
	case mutate.RenewNoResults:
		return "Run 'itos-cc mutation run' on its file, then renew again."
	case mutate.ExceptionKilled:
		return "Remove its entry from " + config.File + ": the tests show that the mutant changes behaviour."
	case mutate.RenewUncovered:
		return "Add a test that executes its line, run mutation run, then renew again, or remove its entry."
	}
	return "Review the mutant again: except it with 'itos-cc mutation except <file>:<line>:<column> --reason …' if it still survives and changes no behaviour, and remove the old entry from " + config.File + "."
}
