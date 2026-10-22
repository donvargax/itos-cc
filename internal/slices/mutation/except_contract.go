package mutation

import "github.com/donvargax/itos-cc/internal/cli"

// Contract preserves the except command’s existing help contract.
func (Except) Contract() cli.Contract {
	return cli.Contract{
		About: `
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
not a reason. So is a site in a source that the exclude of itos-cc.yaml
leaves out, which nothing mutates: exception.excluded.

The entry, under mutation.exceptions, holds the file, the function, the
function's hash, the site's line counted from the function's first line
(line_in_function, 1 for the first), its column, the original, the
replacement, the reason, and the text of the site's line less the space
around it (line_text), so a function that only moves keeps it.
Excepting a site again replaces its entry. Only the lines of the entry
written change: a new one is added in block style after the list's last
entry, a replaced one is written in its place, keeping the comment above
it and the one after its first line, and every other byte of the file is
kept. A list written in flow style is rewritten whole in block style. An entry stops holding, and mutation run and mutation
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
and comments; only the renewed entries' lines change, and an entry that
holds is left alone. Each renewal is reported with its old and new hash and the site.
Each entry left stale is mutation.exception-stale with why: killed (its
fresh results record its mutant killed), changed (no site of its change
is on a line of its text), gone (its file, function or change is gone),
ambiguous, no-results (its function has no fresh results) or uncovered.

Plain output is one line, "excepted again" when it replaced an entry:
  excepted <file>:<line>:<column> ` + "`original` → `replacement`" + ` in <namespace#name> in itos-cc.yaml
and with --renew a line per renewal and per entry left stale:
  renewed <file>:<line>:<column> ` + "`original` → `replacement`" + ` in <namespace#name> (hash <old> → <new>) in itos-cc.yaml
  not renewed <file>[:<line>:<column>] ` + "`original` → `replacement`" + ` in <namespace#name> (<why>)`,
		JSON: `"exception": {"file", "function", "hash",
   "line_in_function", "column", "original", "replacement", "reason",
   "line_text"}, "replaced": true|false; with --renew instead
   "renewed": [{"file", "function", "line", "column", "line_in_function",
   "original", "replacement", "reason", "new_hash", "old_hash", "old_file",
   "old_function", "old_line_in_function", "old_column",
   "match": "line"|"place"|"only"}], "not_renewed": [{the entry's keys,
   "why", "line" when its site is found}]`,
		Rules: []string{
			"exception.no-survivor     the site is no survivor a fresh snapshot records: file, line, column",
			"exception.excluded        the site is in a source itos-cc.yaml excludes: file",
			"mutation.exception-stale  with --renew, an entry that no longer holds was not renewed: file, function, line (when its site is found), column, original, replacement, why: killed|changed|gone|ambiguous|no-results|uncovered",
			"flags.conflict            --renew with --reason: flag",
			"config.invalid            itos-cc.yaml cannot be read: file, and key for an exclude glob",
			"flags.value-missing       no --reason: flag",
			"flags.value-invalid       an empty --reason: flag, value",
			"args.missing              no site given",
			"args.invalid              the site is not <file>:<line>:<column>: argument",
			"args.unexpected           more than one site, or a site with --renew: argument",
		},
		Exits: []cli.Exit{
			{Code: 0, Meaning: "the survivor is excepted in itos-cc.yaml; with --renew, no entry is left stale"},
			{Code: 1, Meaning: "with --renew, an entry that no longer holds could not be renewed"},
			{Code: cli.CodeUsage, Meaning: "a usage or config error: no site or no --reason, a site that is no recorded survivor or is in an excluded source, --renew with a site or --reason, or an itos-cc.yaml that cannot be read"},
		},
		Examples: []string{
			"itos-cc mutation except src/board.ts:3:13 --reason 'c is set again before it is read'",
			"itos-cc mutation except src/billing/invoice.go:42:9 --reason 'the 0 is never read' --json",
			"itos-cc mutation except --renew  # after mutation run, carry stale entries whose mutant is unchanged",
		},
	}
}
