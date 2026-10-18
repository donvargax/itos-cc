package mutation

import "github.com/donvargax/itos-cc/internal/cli"

// Contract preserves the sample command’s existing help contract.
func (Sample) Contract() cli.Contract {
	return cli.Contract{
		About: `
Runs again a few of the mutants whose results mutation run cached in
.metrics/mutate/, and fails when an outcome differs from the one recorded,
so CI catches a cache written by hand, or against other code, which
mutation check trusts. It writes nothing: no snapshot and no summary
comment.

It draws from the fresh entries of the functions chosen, fresh as mutation
check judges them, and among them only from the mutants that ran: killed,
timeout, or survived, never uncovered, so no coverage command runs. Stale
and missing results are mutation check's to report. Each test command's
baseline runs first, then each sampled mutant, in a worker's copy of the
project, as in mutation run.

--count N draws N mutants from the whole selection, all of them when there
are fewer. The draw is seeded with the HEAD commit's id, so a rerun at one
commit samples the same mutants and each new commit others; --seed TEXT
replaces it, to reproduce a run from the seed it printed. Outside a git
repository, --seed is needed.

An outcome that differs from the recorded one fails, whichever way it went:
a kill that now survives, or a survivor now killed. Killed and timeout
agree, since both mean the tests noticed. A recorded survivor that still
survives is no failure here: failing on survivors is mutation check's.

It chooses as mutation check does: paths, --changed, and --since REF, which
samples only the functions the commits since REF changed (git diff
REF...HEAD).

Each mutant runs with the scope of the tests its outcome was recorded with:
the file's own tests, the whole suite (--all-tests), the --test-command
line, or "listed", the file's own tests then, if the mutant survives them,
the listed tests its outcome records (see mutation run), one baseline per
scope's command and per selection of listed tests, so a kill only the
whole suite makes is not read as a survivor. A selection's baseline sets
the timeout of its runs, and one that fails without a mutant is
tests.selection-failed: none of its file's sampled mutants is judged. An
outcome recorded before scopes were is the file's own tests'.
--all-tests or --test-command given to mutation sample runs every sampled
mutant with it instead.

Plain output is the seed and how many of the cached mutants were sampled,
then a line per file, "<file>: N sampled, N mismatched", then each mismatch:
  mismatch <file>:<line>:<column> ` + "`original` → `replacement`" + ` in <namespace#name>: recorded <outcome>, now <outcome>
With --json, each file's "mutants" holds the mutants sampled, in site order;
it is empty, and "baseline" is "failed", when the file's baseline or a
selection of listed tests failed.`,
		JSON: `"seed", "files": [{"file", "baseline": "passed"|"failed",
   "mutants": [{"line", "column", "function", "original", "replacement",
   "recorded", "outcome": "killed"|"survived"|"timeout",
   "scope": "own"|"all-tests"|"listed"|"<test command>"}]}]`,
		Rules: []string{
			"mutation.mismatch         a sampled mutant's outcome differs from the one recorded: file, line, column, function, original, replacement, recorded, outcome, scope",
			"mutation.baseline-failed  the tests fail before any mutant: file",
			"tests.selection-failed    a selection of listed tests fails without any mutant: ids, command, exit_code",
			"config.invalid            itos-cc.yaml cannot be read: file, and key for an exclude glob",
			"sample.no-git             no --seed outside a git repository",
			"since.bad-ref             --since names no commit: ref",
			"since.no-git              --since outside a git repository",
			"flags.conflict            --since with --changed: flag",
		},
		Exits: []cli.Exit{
			{Code: 0, Meaning: "every sampled mutant's outcome agrees with the one recorded, or there was nothing to sample"},
			{Code: 1, Meaning: "a sampled mutant's outcome differs from the one recorded, a file's tests fail before any mutant, or a selection of listed tests fails without any mutant"},
			{Code: cli.CodeUsage, Meaning: "a usage error: a bad flag or path, a --count below 1, a --since ref that is no commit, --since with --changed, or an itos-cc.yaml that cannot be read"},
			{Code: cli.CodeEnvironment, Meaning: "outside a git repository: with no --seed, or with --changed or --since"},
		},
		Examples: []string{
			"itos-cc mutation sample                        # in CI: 20 mutants, seeded with HEAD",
			"itos-cc mutation sample --since origin/main --count 100",
			"itos-cc mutation sample --seed 4813e48 --json  # reproduce a run",
		},
	}
}
