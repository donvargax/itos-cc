package mutation

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/donvargax/itos-cc/internal/cli"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// exceptResult is mutation except's --json object: the entry of
// itos-cc.yaml, with the same keys, and whether it replaced one.
type exceptResult struct {
	Exception *mutants.Exception `json:"exception,omitempty"`
	Replaced  bool               `json:"replaced"`
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
	mutants.Exception
	Why  string `json:"why"`
	Line int    `json:"line,omitempty"`
}

type renewResult struct {
	Renewed    []renewedJSON    `json:"renewed"`
	NotRenewed []notRenewedJSON `json:"not_renewed"`
}

// pruneResult is --prune's --json object: each entry it removed, and why.
type pruneResult struct {
	Pruned []notRenewedJSON `json:"pruned"`
}

// rehashedJSON is an entry --rehash carried over: the entry, its site's
// line, and its hash before and now.
type rehashedJSON struct {
	File           string `json:"file"`
	Function       string `json:"function"`
	Line           int    `json:"line"`
	Column         int    `json:"column"`
	LineInFunction int    `json:"line_in_function"`
	Original       string `json:"original"`
	Replacement    string `json:"replacement"`
	Reason         string `json:"reason"`
	OldHash        string `json:"old_hash"`
	NewHash        string `json:"new_hash"`
}

// rehashResult is --rehash's --json object: each entry it carried over,
// and each it left stale, with why.
type rehashResult struct {
	Rehashed    []rehashedJSON   `json:"rehashed"`
	NotRehashed []notRenewedJSON `json:"not_rehashed"`
}

// Run writes the entry for a recorded survivor into itos-cc.yaml, with
// --renew renews the stale entries whose mutant is unchanged, with --prune
// removes those nothing can mend, or with --rehash carries over those
// written under itos-cc's previous hash. --rehash's, --prune's and --renew's branches
// come before the reason is looked at; an absent --reason and an empty one
// are refused apart, then the site's count and form, before the query is
// assembled once.
func (c Except) Run(ctx context.Context, ui *cli.UI, h Handler) int {
	if c.Rehash {
		return c.rehash(ctx, ui, h)
	}
	if c.Prune {
		return c.prune(ctx, ui, h)
	}
	if c.Renew {
		return c.renew(ctx, ui, h)
	}
	var result exceptResult
	// With no --reason only a site that re-points a stale entry is
	// excepted: every refusal is the missing --reason.
	refuse := func(p *cli.Problem) int {
		if c.Reason == nil {
			p = reasonMissing()
		}
		return ui.Emit(c.JSON, result, ui.Problems(p))
	}
	if c.Reason != nil && strings.TrimSpace(*c.Reason) == "" {
		return ui.Emit(c.JSON, result, ui.Problems(cli.NewProblem(cli.CodeUsage, "flags.value-invalid", fmt.Sprintf("--reason needs a reason, and was given %q", *c.Reason),
			"Say why no test can kill the mutant.").With("flag", "--reason").With("value", *c.Reason)))
	}
	switch len(c.Site) {
	case 0:
		return refuse(cli.NewProblem(cli.CodeUsage, "args.missing", "mutation except needs the site to except, as <file>:<line>:<column>",
			"Name a survivor as mutation run lists it, such as src/board.ts:7:19."))
	case 1:
	default:
		return refuse(cli.NewProblem(cli.CodeUsage, "args.unexpected", fmt.Sprintf("mutation except takes one site, and was also given %q", c.Site[1]),
			"Except one site at a time.").With("argument", c.Site[1]))
	}
	q, invalid := exceptQuery(c.Site[0], c.Reason)
	if invalid != nil {
		return refuse(invalid)
	}
	found, err := h.Except(ctx, q)
	if err != nil {
		return ui.Emit(c.JSON, result, ui.Problems(exceptError(err, ui.Rel, "Fix it, then except the site again.")))
	}
	e := found.Exception
	result = exceptResult{Exception: &e, Replaced: found.Replaced}
	if !c.JSON {
		what := "excepted"
		if found.Replaced {
			what = "excepted again"
		}
		_, _ = fmt.Fprintf(ui.Stdout, "%s %s:%d:%d %s → %s in %s in %s\n", what, named(found.File, found.Path, ui.Rel), found.Line, found.Column,
			Quote(e.Original), Quote(e.Replacement), e.Function, "itos-cc.yaml")
	}
	return ui.Emit(c.JSON, result, nil)
}

// exceptQuery is the query of arg, a site or, holding an identity's
// @<start>:<end>:<hash>:<offset>: part, a survivor's identity as a counted
// run reports it, with reason; args.invalid when it is neither.
func exceptQuery(arg string, reason *string) (ExceptQuery, *cli.Problem) {
	if mutants.IsIdentity(arg) {
		id, ok := mutants.ParseIdentity(arg)
		if !ok {
			return ExceptQuery{}, cli.NewProblem(cli.CodeUsage, "args.invalid", fmt.Sprintf("%q is not a mutant's identity, <file>:<namespace#name>@<start>:<end>:<hash>:<offset>:<original>><replacement>", arg),
				"Copy the identity whole from the report of mutation run --count, such as its mutation.survived problem.").With("argument", arg)
		}
		return ExceptQuery{Identity: &id, Reason: reason}, nil
	}
	path, line, column, ok := parseSite(arg)
	if !ok {
		return ExceptQuery{}, cli.NewProblem(cli.CodeUsage, "args.invalid", fmt.Sprintf("%q is not a site, <file>:<line>:<column>", arg),
			"Name a survivor as mutation run lists it, such as src/board.ts:7:19.").With("argument", arg)
	}
	return ExceptQuery{Path: path, Line: line, Column: column, Reason: reason}, nil
}

// renew is mutation except --renew: each entry of itos-cc.yaml that no
// longer holds is renewed when its mutant is unchanged (mutants.Renew), and
// only those entries are written. Every renewal is reported, and every
// stale entry left as it was is mutation.exception-stale, with why.
func (c Except) renew(ctx context.Context, ui *cli.UI, h Handler) int {
	result := renewResult{Renewed: []renewedJSON{}, NotRenewed: []notRenewedJSON{}}
	if c.Reason != nil {
		return ui.Emit(c.JSON, result, ui.Problems(cli.NewProblem(cli.CodeUsage, "flags.conflict", "--renew keeps each entry's own reason, and was given --reason",
			"Drop --reason; to except a survivor with a new reason, name its site without --renew.").With("flag", "--reason")))
	}
	if len(c.Site) > 0 {
		return ui.Emit(c.JSON, result, ui.Problems(cli.NewProblem(cli.CodeUsage, "args.unexpected", fmt.Sprintf("--renew renews every stale entry of %s, and takes no site, but was given %q", "itos-cc.yaml", c.Site[0]),
			"Drop the site, or drop --renew to except that survivor.").With("argument", c.Site[0])))
	}
	found, err := h.Renew(ctx)
	if err != nil {
		return ui.Emit(c.JSON, result, ui.Problems(exceptError(err, ui.Rel, "Fix it, then renew again.")))
	}
	for _, r := range found.Renewed {
		rel := ui.Rel(sourcePath(found.Root, r.New.File))
		result.Renewed = append(result.Renewed, renewedJSON{File: rel, Function: r.New.Function, Line: r.Line, Column: r.New.Column,
			LineInFunction: r.New.LineInFunction, Original: r.New.Original, Replacement: r.New.Replacement, Reason: r.New.Reason,
			NewHash: r.New.Hash, OldHash: r.Old.Hash, OldFile: r.Old.File, OldFunction: r.Old.Function,
			OldLineInFunction: r.Old.LineInFunction, OldColumn: r.Old.Column, Match: r.Match})
		if !c.JSON {
			_, _ = fmt.Fprintf(ui.Stdout, "renewed %s:%d:%d %s → %s in %s (hash %s → %s) in %s\n", rel, r.Line, r.New.Column,
				Quote(r.New.Original), Quote(r.New.Replacement), r.New.Function, r.Old.Hash, r.New.Hash, "itos-cc.yaml")
		}
	}
	var problems []*cli.Problem
	for _, u := range found.Left {
		rel := ui.Rel(sourcePath(found.Root, u.File))
		result.NotRenewed = append(result.NotRenewed, notRenewedJSON{Exception: u.Exception, Why: u.Why, Line: u.Line})
		at := rel
		if u.Line != 0 {
			at = fmt.Sprintf("%s:%d:%d", rel, u.Line, u.Column)
		}
		change := fmt.Sprintf("%s → %s in %s", Quote(u.Original), Quote(u.Replacement), u.Function)
		if !c.JSON {
			_, _ = fmt.Fprintf(ui.Stdout, "  not renewed %s %s (%s)\n", at, change, u.Why)
		}
		because, fix := notRenewedBecause(u.Why), notRenewedFix(u.Why)
		if u.Rehashable {
			because, fix = because+"; "+previousHash, rehashFix+" Otherwise: "+fix
		}
		p := cli.NewProblem(cli.CodeNo, "mutation.exception-stale",
			fmt.Sprintf("%s: the entry in %s for %s was not renewed: %s", at, "itos-cc.yaml", change, because), fix)
		p = p.With("file", rel).With("function", u.Function).With("column", u.Column).
			With("original", u.Original).With("replacement", u.Replacement).With("why", u.Why)
		if u.Line != 0 {
			p = p.With("line", u.Line)
		}
		p.Shown = true
		problems = append(problems, p)
	}
	return ui.Emit(c.JSON, result, problems)
}

// prune is mutation except --prune: each entry of itos-cc.yaml stale as
// gone, killed or excluded is removed, and each removal reported. It
// takes no site, no --reason and no --renew.
func (c Except) prune(ctx context.Context, ui *cli.UI, h Handler) int {
	result := pruneResult{Pruned: []notRenewedJSON{}}
	switch {
	case c.Renew:
		return ui.Emit(c.JSON, result, ui.Problems(cli.NewProblem(cli.CodeUsage, "flags.conflict", "--prune removes the entries nothing can mend, and --renew carries others over: they are not given together",
			"Run 'itos-cc mutation except --renew' first, then 'itos-cc mutation except --prune'.").With("flag", "--renew")))
	case c.Reason != nil:
		return ui.Emit(c.JSON, result, ui.Problems(cli.NewProblem(cli.CodeUsage, "flags.conflict", "--prune removes entries, and was given --reason",
			"Drop --reason; to except a survivor with a reason, name its site without --prune.").With("flag", "--reason")))
	case len(c.Site) > 0:
		return ui.Emit(c.JSON, result, ui.Problems(cli.NewProblem(cli.CodeUsage, "args.unexpected", fmt.Sprintf("--prune removes every entry of %s that nothing can mend, and takes no site, but was given %q", "itos-cc.yaml", c.Site[0]),
			"Drop the site, or drop --prune to except that survivor.").With("argument", c.Site[0])))
	}
	found, err := h.Prune(ctx)
	if err != nil {
		return ui.Emit(c.JSON, result, ui.Problems(exceptError(err, ui.Rel, "Fix it, then prune again.")))
	}
	for _, s := range found.Pruned {
		result.Pruned = append(result.Pruned, notRenewedJSON{Exception: s.Exception, Why: s.Why, Line: s.Line})
		if !c.JSON {
			at := ui.Rel(sourcePath(found.Root, s.File))
			if s.Line != 0 {
				at = fmt.Sprintf("%s:%d:%d", at, s.Line, s.Column)
			}
			_, _ = fmt.Fprintf(ui.Stdout, "pruned %s %s → %s in %s (%s) from %s\n", at, Quote(s.Original), Quote(s.Replacement), s.Function, s.Why, "itos-cc.yaml")
		}
	}
	return ui.Emit(c.JSON, result, nil)
}

// rehash is mutation except --rehash: each entry of itos-cc.yaml that no
// longer holds, written under itos-cc's previous hash, whose function
// changed only in what the hash now leaves out, takes its function's hash
// now (mutants.Rehash), and only those hashes are written. Every rehash is
// reported, and every stale entry left as it was is
// mutation.exception-stale, with why. It takes no site, no --reason, and
// neither --renew nor --prune.
func (c Except) rehash(ctx context.Context, ui *cli.UI, h Handler) int {
	result := rehashResult{Rehashed: []rehashedJSON{}, NotRehashed: []notRenewedJSON{}}
	conflict := func(flag, fix string) int {
		return ui.Emit(c.JSON, result, ui.Problems(cli.NewProblem(cli.CodeUsage, "flags.conflict",
			"--rehash carries over each entry written under itos-cc's previous hash, and was given "+flag, fix).With("flag", flag)))
	}
	switch {
	case c.Renew:
		return conflict("--renew", "Run 'itos-cc mutation except --rehash' first, then 'itos-cc mutation run' on what is still stale, then 'itos-cc mutation except --renew'.")
	case c.Prune:
		return conflict("--prune", "Run 'itos-cc mutation except --rehash' first, then 'itos-cc mutation except --prune' once mutation run has judged what is still stale.")
	case c.Reason != nil:
		return conflict("--reason", "Drop --reason: a rehashed entry keeps its reason. To except a survivor with a reason, name its site without --rehash.")
	case len(c.Site) > 0:
		return ui.Emit(c.JSON, result, ui.Problems(cli.NewProblem(cli.CodeUsage, "args.unexpected", fmt.Sprintf("--rehash carries over every entry of %s written under itos-cc's previous hash, and takes no site, but was given %q", "itos-cc.yaml", c.Site[0]),
			"Drop the site, or drop --rehash to except that survivor.").With("argument", c.Site[0])))
	}
	found, err := h.Rehash(ctx)
	if err != nil {
		return ui.Emit(c.JSON, result, ui.Problems(exceptError(err, ui.Rel, "Fix it, then rehash again.")))
	}
	for _, r := range found.Rehashed {
		rel := ui.Rel(sourcePath(found.Root, r.Old.File))
		result.Rehashed = append(result.Rehashed, rehashedJSON{File: rel, Function: r.Old.Function, Line: r.Line, Column: r.Old.Column,
			LineInFunction: r.Old.LineInFunction, Original: r.Old.Original, Replacement: r.Old.Replacement, Reason: r.Old.Reason,
			OldHash: r.Old.Hash, NewHash: r.New.Hash})
		if !c.JSON {
			_, _ = fmt.Fprintf(ui.Stdout, "rehashed %s:%d:%d %s → %s in %s (hash %s → %s) in %s\n", rel, r.Line, r.Old.Column,
				Quote(r.Old.Original), Quote(r.Old.Replacement), r.Old.Function, r.Old.Hash, r.New.Hash, "itos-cc.yaml")
		}
	}
	var problems []*cli.Problem
	for _, s := range found.Left {
		rel := ui.Rel(sourcePath(found.Root, s.File))
		result.NotRehashed = append(result.NotRehashed, notRenewedJSON{Exception: s.Exception, Why: s.Why, Line: s.Line})
		at := rel
		if s.Line != 0 {
			at = fmt.Sprintf("%s:%d:%d", rel, s.Line, s.Column)
		}
		change := fmt.Sprintf("%s → %s in %s", Quote(s.Original), Quote(s.Replacement), s.Function)
		if !c.JSON {
			_, _ = fmt.Fprintf(ui.Stdout, "  not rehashed %s %s (%s)\n", at, change, s.Why)
		}
		because, fix := "its function changed in more than what the hash now leaves out", notRehashedChangedFix
		if s.Why == mutants.ExceptionGone {
			because, fix = "its file, its function, or any site of its change in it is gone", pruneFix
		}
		p := cli.NewProblem(cli.CodeNo, "mutation.exception-stale",
			fmt.Sprintf("%s: the entry in %s for %s was not rehashed: %s", at, "itos-cc.yaml", change, because), fix)
		p = p.With("file", rel).With("function", s.Function).With("column", s.Column).
			With("original", s.Original).With("replacement", s.Replacement).With("why", s.Why)
		if s.Line != 0 {
			p = p.With("line", s.Line)
		}
		// Not Shown: stderr gives its fix, which the line above does not.
		problems = append(problems, p)
	}
	return ui.Emit(c.JSON, result, problems)
}

// notRehashedChangedFix is the fix of an entry --rehash left as changed.
const notRehashedChangedFix = "Run 'itos-cc mutation run' on its file, then 'itos-cc mutation except --renew', which carries the entry over, keeping its reason, when its mutant is unchanged."

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

// notRenewedBecause says why --renew left a stale entry as it was.
func notRenewedBecause(why string) string {
	switch why {
	case mutants.ExceptionKilled:
		return "the fresh results record its mutant killed"
	case mutants.ExceptionChanged:
		return "no site of its change is on a line with the text it recorded: its line changed"
	case mutants.ExceptionGone:
		return "its file, its function, or any site of its change in it is gone"
	case mutants.RenewNoResults:
		return "its function has no fresh mutation results to show its mutant survived"
	case mutants.RenewAmbiguous:
		return "more than one site could be its mutant"
	case mutants.RenewUncovered:
		return "its mutant is recorded uncovered, which no exception excuses"
	}
	return why
}

// The fixes of an entry stale as killed, and as gone or excluded, which
// mutation except --prune removes, and the word and the fix of one written
// under itos-cc's previous hash whose function changed only in what the
// hash now leaves out, which mutation except --rehash carries over.
const (
	previousHash   = "it was written under itos-cc's previous hash, and its function changed only in what the hash now leaves out"
	rehashFix      = "Run 'itos-cc mutation except --rehash': it carries the entry over to its function's hash now, running nothing."
	pruneKilledFix = "Remove its entry with 'itos-cc mutation except --prune': the tests show that the mutant changes behaviour."
	pruneFix       = "Remove its entry with 'itos-cc mutation except --prune'."
)

// notRenewedFix says what to do about a stale entry --renew left.
func notRenewedFix(why string) string {
	switch why {
	case mutants.RenewNoResults:
		return "Run 'itos-cc mutation run' on its file, then renew again."
	case mutants.ExceptionKilled:
		return pruneKilledFix
	case mutants.RenewUncovered:
		return "Add a test that executes its line, run mutation run, then renew again, or remove its entry."
	}
	return "Review the mutant again: if it still survives and changes no behaviour, except it at its site now with 'itos-cc mutation except <file>:<line>:<column>', " +
		"which replaces this entry and keeps its reason; once its function or every site of its change is gone, 'itos-cc mutation except --prune' removes the entry."
}
