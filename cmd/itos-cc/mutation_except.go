package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/internal/cli"
	mutation "github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/selection"
	"github.com/donvargax/itos-cc/project"
)

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
func runMutationExcept(ctx context.Context, in *invocation) (any, error) {
	if in.set("renew") {
		return runRenewExceptions(ctx, in)
	}
	var result exceptResult
	if !in.set("reason") {
		return result, cli.NewProblem(cli.CodeUsage, "flags.value-missing", "--reason is required: an exception says why no test can kill its mutant",
			"Give it, such as --reason 'the loop only counts up'.").With("flag", "--reason")
	}
	reason := in.str("reason")
	if strings.TrimSpace(reason) == "" {
		return result, cli.NewProblem(cli.CodeUsage, "flags.value-invalid", fmt.Sprintf("--reason needs a reason, and was given %q", reason),
			"Say why no test can kill the mutant.").With("flag", "--reason").With("value", reason)
	}
	switch len(in.args) {
	case 0:
		return result, cli.NewProblem(cli.CodeUsage, "args.missing", "mutation except needs the site to except, as <file>:<line>:<column>",
			"Name a survivor as mutation run lists it, such as src/board.ts:7:19.")
	case 1:
	default:
		return result, cli.NewProblem(cli.CodeUsage, "args.unexpected", fmt.Sprintf("mutation except takes one site, and was also given %q", in.args[1]),
			"Except one site at a time.").With("argument", in.args[1])
	}
	path, line, column, ok := parseSite(in.args[0])
	if !ok {
		return result, cli.NewProblem(cli.CodeUsage, "args.invalid", fmt.Sprintf("%q is not a site, <file>:<line>:<column>", in.args[0]),
			"Name a survivor as mutation run lists it, such as src/board.ts:7:19.").With("argument", in.args[0])
	}
	// An unreadable file is refused before any work, and so is a site of
	// a source the file excludes, which nothing mutates.
	cfg, err := loadConfig()
	if err != nil {
		return result, err
	}
	root := project.Root()
	if exclude := (selection.Exclusion{Root: root, Globs: cfg.Exclude}); exclude.Excludes(absOrSame(path)) {
		rel := project.Rel(absOrSame(path))
		return result, cli.NewProblem(cli.CodeUsage, "exception.excluded",
			fmt.Sprintf("%s:%d:%d is in %s, a source the exclude of %s leaves out, so none of its mutants is judged or excepted", rel, line, column, rel, config.File),
			fmt.Sprintf("Except a survivor of a source mutation run judges, or take %s out of exclude in %s first.", rel, config.File)).
			With("file", rel)
	}
	if err := mutationInputsBound(in.adapters); err != nil {
		return result, err
	}
	support, err := supportNow(ctx, in, cfg)
	if err != nil {
		return result, err
	}
	tests, err := importingTests(ctx, in)
	if err != nil {
		return result, err
	}
	e, err := mutation.Survivor(ctx, path, line, column, mutationCheckOptions(in, nil, tests, support, nil), in.adapters.SourceStatus)
	var none *mutation.NoSurvivorError
	if errors.As(err, &none) {
		rel := project.Rel(absOrSame(path))
		return result, cli.NewProblem(cli.CodeUsage, "exception.no-survivor", fmt.Sprintf("%s:%d:%d is no survivor to except: %s", rel, line, column, none.Reason),
			fmt.Sprintf("Except a site that mutation run lists as survived; run 'itos-cc mutation run %s' first when its results are stale or missing.", rel)).
			With("file", rel).With("line", line).With("column", column)
	}
	if err != nil {
		return result, mutationRunError(err)
	}
	e.Reason = reason
	if in.adapters.Exceptions == nil {
		return result, errors.New("mutation Exceptions binding is missing")
	}
	writer := in.adapters.Exceptions(root)
	if writer == nil {
		return result, errors.New("mutation Exceptions binding is missing")
	}
	replaced, failure := writer.Except(ctx, e)
	err = exceptionUpdateError(failure)
	var invalid *config.InvalidError
	if errors.As(err, &invalid) {
		return result, configProblem(invalid, "Fix it, then except the site again.")
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
// (mutation.Renew), and only those entries are written. Every renewal is
// reported, and every stale entry left as it was is
// mutation.exception-stale, with why.
func runRenewExceptions(ctx context.Context, in *invocation) (any, error) {
	result := renewResult{Renewed: []renewedJSON{}, NotRenewed: []notRenewedJSON{}}
	if in.set("reason") {
		return result, cli.NewProblem(cli.CodeUsage, "flags.conflict", "--renew keeps each entry's own reason, and was given --reason",
			"Drop --reason; to except a survivor with a new reason, name its site without --renew.").With("flag", "--reason")
	}
	if len(in.args) > 0 {
		return result, cli.NewProblem(cli.CodeUsage, "args.unexpected", fmt.Sprintf("--renew renews every stale entry of %s, and takes no site, but was given %q", config.File, in.args[0]),
			"Drop the site, or drop --renew to except that survivor.").With("argument", in.args[0])
	}
	cfg, err := loadConfig()
	if err != nil {
		return result, err
	}
	if len(cfg.Exceptions) == 0 {
		return result, nil
	}
	if err := mutationInputsBound(in.adapters); err != nil {
		return result, err
	}
	support, err := supportNow(ctx, in, cfg)
	if err != nil {
		return result, err
	}
	tests, err := importingTests(ctx, in)
	if err != nil {
		return result, err
	}
	root := project.Root()
	all, err := project.Discover(ctx, []string{root})
	if err != nil {
		return result, err
	}
	renewed, left, err := mutation.Renew(ctx, cfg.Exceptions, all.Sources, mutationCheckOptions(in, nil, tests, support, nil), in.adapters.SourceStatus)
	if err != nil {
		return result, mutationRunError(err)
	}
	if in.adapters.Exceptions == nil {
		return result, errors.New("mutation Exceptions binding is missing")
	}
	writer := in.adapters.Exceptions(root)
	if writer == nil {
		return result, errors.New("mutation Exceptions binding is missing")
	}
	err = exceptionUpdateError(writer.Renew(ctx, renewed))
	var invalid *config.InvalidError
	if errors.As(err, &invalid) {
		return result, configProblem(invalid, "Fix it, then renew again.")
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
		p := cli.NewProblem(cli.CodeNo, "mutation.exception-stale",
			fmt.Sprintf("%s: the entry in %s for %s was not renewed: %s", at, config.File, change, notRenewedBecause(u.Why)),
			notRenewedFix(u.Why))
		p.With("file", rel).With("function", u.Function).With("column", u.Column).
			With("original", u.Original).With("replacement", u.Replacement).With("why", u.Why)
		if u.Line != 0 {
			p.With("line", u.Line)
		}
		p.Shown = true
		in.report(p)
	}
	return result, nil
}

// notRenewedBecause says why --renew left a stale entry as it was.
func notRenewedBecause(why string) string {
	switch why {
	case mutation.ExceptionKilled:
		return "the fresh results record its mutant killed"
	case mutation.ExceptionChanged:
		return "no site of its change is on a line with the text it recorded: its line changed"
	case mutation.ExceptionGone:
		return "its file, its function, or any site of its change in it is gone"
	case mutation.RenewNoResults:
		return "its function has no fresh mutation results to show its mutant survived"
	case mutation.RenewAmbiguous:
		return "more than one site could be its mutant"
	case mutation.RenewUncovered:
		return "its mutant is recorded uncovered, which no exception excuses"
	}
	return why
}

// notRenewedFix says what to do about a stale entry --renew left.
func notRenewedFix(why string) string {
	switch why {
	case mutation.RenewNoResults:
		return "Run 'itos-cc mutation run' on its file, then renew again."
	case mutation.ExceptionKilled:
		return "Remove its entry from " + config.File + ": the tests show that the mutant changes behaviour."
	case mutation.RenewUncovered:
		return "Add a test that executes its line, run mutation run, then renew again, or remove its entry."
	}
	return "Review the mutant again: except it with 'itos-cc mutation except <file>:<line>:<column> --reason …' if it still survives and changes no behaviour, and remove the old entry from " + config.File + "."
}

// exceptionUpdateError preserves config.invalid versus original I/O/cancel causes.
func exceptionUpdateError(failure mutation.ExceptionFailure) error {
	switch f := failure.(type) {
	case nil:
		return nil
	case mutation.ExceptionInvalid:
		return &config.InvalidError{Reason: f.Reason, Key: f.Key}
	case mutation.ExceptionFailed:
		return f.Cause
	case mutation.ExceptionCancelled:
		return f.Cause
	}
	return failure
}
