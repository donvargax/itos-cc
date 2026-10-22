package mutation

import "github.com/donvargax/itos-cc/internal/cli"

// Contract preserves the check command’s existing help contract.
func (Check) Contract() cli.Contract {
	return cli.Contract{
		About: `
Says, from the results mutation run cached in .metrics/mutate/ alone, whether
each function has passing results for its code as it is now. It runs no test
and no coverage command, and writes nothing, so a gate such as a commit hook
can prove mutation run was run on the code being committed.

For each chosen function with at least one mutation site, its entry in its
file's snapshot is compared with its source now. No entry is missing; an
entry whose function has changed since is stale, while a function that only
moved is fresh; an entry that lacks a site the function has now, as when a
newer itos-cc adds a mutation operator or a mutation run --fail-fast stop
left the site undecided, is stale, naming each such site, and
mutation run runs only those; a mutant an entry records at a site the
unchanged function no longer holds, as when a newer itos-cc removes a
mutation operator, is not in the code: it is left out of the verdict and
the counts, and mutation run drops it from the entry, reusing the rest; and
a fresh entry fails on each survivor it records, and
with --fail-uncovered on each uncovered mutant it records. A function with no
mutation site needs no mutation results, but with --fail-uncovered every
judged Go function also needs fresh independent executable coverage evidence,
including zero-site functions. Uncovered executable blocks and missing or
stale evidence fail without running tests, coverage or list commands. The
snapshot also records the hash of each test file that reaches its file: in
TypeScript, Python, Kotlin and Dart through its imports, transitively, through the
test-support files a test imports and, in Python, the conftest.py files
above it, in Dart only the files under its package's test/ that its own
tests run, never those under integration_test/ or test_driver/; in Go its
package's tests and those of the packages that import it. When one was
added, changed, or removed since, every function of the file is stale, as
it is when the snapshot predates recording tests. A run with --since that finds them changed keeps the
entries of the functions it does not judge marked stale, until a run judges
them again.

A kill by tests itos-cc.yaml lists (mutation.tests) holds while the files
those tests are defined in, and the support files, are unchanged, with no
command run. A test the list command printed with a fingerprint, a third
column, is dated by it instead: its kill holds while the list prints it
with that fingerprint, whatever else its file holds and whichever file
names it, and its kill is stale, naming the test, when the fingerprint
changed or the test is no longer listed. The list command runs, once, only
when the file of such a test changed; one that fails is tests.list-failed.

A survivor that itos-cc.yaml excepts (see mutation except) fails nothing.
An exception that no longer holds fails as mutation.exception-stale, as in
mutation run: its function changed, its function or site is gone, or a fresh
entry records its mutant killed. A check of the whole project, or with
--since one whose range deleted or renamed an entry's file, judges the
entries of files it does not select as mutation run does: excluded, when
itos-cc.yaml's exclude leaves the file out, moved, when one source checked
holds the function unchanged, or gone. An itos-cc.yaml that cannot be read
is config.invalid.

A snapshot under the paths checked whose source file is gone, nothing at
its path in the working tree, as when the file was deleted, is an orphan,
judged as mutation run judges it: one whose file git shows renamed to a
file that exists, in the --since range or staged against HEAD (git mv,
or git add after a mv), is none, kept for the --since run that moves it,
and outside a git repository none is named; the snapshot of a file
outside the project root, named by its absolute path, is judged by that
file, so it is none while the file exists. The snapshot of a source that
itos-cc.yaml's exclude leaves out is an orphan too, though its file is
there. check names an orphan, a line "<file>: gone, its snapshot
<snapshot> is an orphan, which mutation run removes", excluded in place of
gone for an excluded source, and in --json under "orphans", and judges
and fails nothing for it. With no paths it looks under the working directory. The next
mutation run over its directory removes it.

It chooses as mutation run does: paths, --changed, and --since REF, which
checks only the functions the commits since REF changed (git diff
REF...HEAD), as mutation run judges them.

--fail-uncovered=lines, which needs --since, checks coverage as mutation
run --fail-uncovered=lines judges it: only an executable block or line
that overlaps a line the range added or changed, and an uncovered mutant
on such a line, fail. Missing, stale or unsupported evidence fails for a
judged function with such a line, whose changed lines cannot be judged
without fresh evidence, and a function with none needs no evidence.
Survivors and exceptions are checked as before. --fail-uncovered alone is
--fail-uncovered=functions; without --since, =lines is flags.conflict.

Plain output is a line per file, "<file>: N fresh, N stale, N missing",
which names the test files that changed when they made functions stale, then
each problem, a line each:
  missing <file>:<line> in <namespace#name>
  stale <file>:<line> in <namespace#name>
  survived <file>:<line>:<column> ` + "`original` → `replacement`" + ` in <namespace#name>
  uncovered <file>:<line>:<column> ` + "`original` → `replacement`" + ` in <namespace#name>
  exception-stale <file>:<line>:<column> ` + "`original` → `replacement`" + ` in <namespace#name> (<why>)
the place being the file alone when the function or the file is gone, and
the why "moved to <file>" when the function is in another file now. The line of a missing or stale function is where it starts now.
A fresh function whose entry records mutants at sites it no longer holds
gets a note, which fails nothing:
  left out <file>:<line> in <namespace#name>: <n> recorded mutants at sites gone, which the next mutation run drops
the line being where it starts now. With --json,
"functions" holds the functions checked, in the file's order, with the
counts their entry records, less mutants at sites gone, zero when missing; an excepted survivor counts
in "excepted", not "survived"; and "left_out" counts a fresh function's
mutants at sites gone, when it has any.`,
		JSON: `"files": [{"file", "functions": [{"function",
   "state": "fresh"|"stale"|"missing", "killed", "survived", "excepted",
   "uncovered", and when there are any "left_out", the mutants its fresh
   entry records at sites gone}]}], and when there is any, "orphans": [{"file",
   "snapshot", "why"}], each snapshot under the paths whose source file is
   gone or excluded, "snapshot" its path from the project root, "why"
   "gone"|"excluded"`,
		Rules: []string{
			"mutation.missing          a function with a mutation site has no results: file, line, function",
			"mutation.stale            a function, or a test that imports its file, changed since its results, or they never recorded one of its sites: file, line, function",
			"mutation.survived         its results record a survivor: file, line, column, function, original, replacement",
			"mutation.uncovered        with --fail-uncovered, its results record an uncovered mutant: file, line, column, function, original, replacement",
			"mutation.uncovered-statement with --fail-uncovered, a measured Go coverage block or Python, TypeScript, Kotlin or Dart line is uncovered: file, function, line",
			"mutation.coverage-missing  with --fail-uncovered, a Go, Python, TypeScript, Kotlin or Dart function lacks complete measured coverage evidence: file, function, line",
			"mutation.coverage-stale    with --fail-uncovered, Go, Python, TypeScript, Kotlin or Dart coverage inputs changed since measurement: file, function, line",
			"mutation.coverage-unsupported with --fail-uncovered, strict coverage reaches beyond the inventoried root: a Go workspace or outside replacement, an npm or yarn workspace, a pnpm workspace root package or a member depending on another, or a local dependency, pnpm override, includeBuild, Maven module or pub path dependency or override outside it: file, function, line",
			"mutation.exception-stale  an exception in itos-cc.yaml no longer holds: file, function, line (none when the function or its file is gone, or its file excluded), column, original, replacement, why: killed|changed|gone|moved|excluded, and with moved new_file",
			"tests.list-failed         the list command of mutation.tests, run to date a kill by a test listed with a fingerprint, failed: command, exit_code",
			"config.invalid            itos-cc.yaml cannot be read: file, and key for an exclude glob",
			"since.bad-ref             --since names no commit: ref",
			"since.no-git              --since outside a git repository",
			"flags.conflict            --since with --changed, or --fail-uncovered=lines without --since: flag",
		},
		Exits: []cli.Exit{
			{Code: 0, Meaning: "every function checked has fresh results, and none records a survivor"},
			{Code: 1, Meaning: "a function's results are missing or stale, or record a survivor itos-cc.yaml does not except, or an uncovered mutant, Go executable block or Python, TypeScript, Kotlin or Dart line, or missing/stale Go, Python, TypeScript, Kotlin or Dart coverage evidence with --fail-uncovered; or an exception is stale; or the list command of mutation.tests failed"},
			{Code: cli.CodeUsage, Meaning: "a usage or config error: a bad flag or path, a --since ref that is no commit, --since with --changed, --fail-uncovered=lines without --since, or an itos-cc.yaml that cannot be read"},
			{Code: cli.CodeEnvironment, Meaning: "--changed or --since outside a git repository"},
		},
		Examples: []string{
			"itos-cc mutation check --since origin/main --fail-uncovered  # a commit hook",
			"itos-cc mutation check --since origin/main --fail-uncovered=lines  # coverage of the changed lines alone",
			"itos-cc mutation check --json src/billing",
		},
	}
}
