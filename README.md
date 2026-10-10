# itos-cc

Code-quality measurements for TypeScript (and JavaScript), Python, Kotlin,
and Go, in one binary, with output meant for both people and coding agents.

> **Experimental.** This is a proof of concept. Its contract with scripts
> (exit codes, `--json`, and `.metrics/`) changes only in a major release;
> plain output may change in any release.

The aim is for coding agents to use it through
[itos](https://github.com/donvargax/itos), to find out where code is risky
before and after they change it. [IDEAS.md](IDEAS.md) lists what is planned
and what has not been verified.

| Command | Question it answers |
| --- | --- |
| `itos-cc crap` | Which functions are complex *and* under-tested? |
| `itos-cc dry` | Which functions are the same code with the names changed? |
| `itos-cc mutation run` | Would the tests notice if this code were wrong? |
| `itos-cc scrap` | Which test files should an agent leave alone, table-drive, refactor, or split? |
| `itos-cc units` | What functions and methods do the tools see? |
| `itos-cc serve` | Live architecture graph for the viewer: what depends on what, and where is it risky? |

The ideas come from Robert C. Martin's
[crapper](https://github.com/unclebob/crapper),
[mutator](https://github.com/unclebob/mutator),
[dryer](https://github.com/unclebob/dryer), and
[scrap](https://github.com/unclebob/scrap). This is a separate
implementation in Go around one shared core, so every tool understands every
language the same way.

## Install

Download your platform's archive from the
[latest release](https://github.com/donvargax/itos-cc/releases/latest):
Linux (static) and macOS on amd64 and arm64, and Windows on amd64. Check it
against `checksums.txt` and put `itos-cc` on your `PATH`:

```bash
sha256sum --ignore-missing -c checksums.txt
tar -xzf itos-cc-<version>-linux-amd64.tar.gz itos-cc
itos-cc version
```

Or build it with Go, which needs cgo and a C compiler (tree-sitter is C):

```bash
go install github.com/donvargax/itos-cc/cmd/itos-cc@latest
```

## Build

Parsing uses tree-sitter, which needs cgo and a C compiler:

```bash
go build -o itos-cc ./cmd/itos-cc
go test ./...
```

Releases are built by CI (`.github/workflows/ci.yml`) on each platform's own
runner when a `vX.Y.Z` tag is pushed, after the tests pass on Linux, macOS,
and Windows. To cross-compile Linux and Windows binaries locally instead, use
[zig](https://ziglang.org) as the C compiler:

```bash
CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC="zig cc -target aarch64-linux-musl" \
  go build -ldflags='-linkmode=external -extldflags=-static' -o itos-cc ./cmd/itos-cc
CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC="zig cc -target x86_64-windows-gnu" \
  go build -o itos-cc.exe ./cmd/itos-cc
```

macOS binaries need a macOS machine or CI runner: Go's macOS link step asks
for a system library zig does not ship.

## Use

Run anywhere in a project: `.metrics/` and `itos-cc.yaml` are at its root
(see [.metrics](#metrics)). Paths are files, directories, or fragments of a
path (`itos-cc crap billing`); `--changed` selects what git reports as added
or modified. Every command takes `-h` and `--json`.

Scripts rely on three things only, as with itos ([docs/CLI.md](docs/CLI.md)):
the exit code, the `--json` object less its `message` and `fix` keys, and the
files itos-cc writes: those under `.metrics/`, and `itos-cc.yaml`. Plain
output is for people.

| Code | Meaning |
| --- | --- |
| 0 | Success. |
| 1 | A check said no: a mutant survived, a function is over `--threshold`. |
| 2 | A usage or config error: a bad flag, path, or report, or an `itos-cc.yaml` that cannot be read. |
| 3 | The environment lacks something: a tool, a report, a git repository. |
| 70 | An internal error itos-cc could not classify; please report it. |
| 75 | A temporary failure: the same command may pass when run again. |

`--json` prints one object, `{"schema": 1, "ok": true|false, …}`, and for a
failure, usage errors included, `"problems": [{"rule", "message", "fix", …}]`
with a stable rule id and the problem's subject (file, line, function, …)
as keys. Each command's `--help` gives its keys, rules, and exit codes.

Every tool works on the same units: functions and methods, plus inline
Express-style route callbacks (`app.get("/users", (req, res) => …)` is the
unit `GET /users`), which are measured apart from the function that mounts
them.

```bash
itos-cc crap --top 20             # runs the tests with coverage first
itos-cc crap --use-existing-coverage
itos-cc dry --changed             # changed files against the whole project
itos-cc mutation run src/billing/invoice.ts
itos-cc scrap --verbose
```

### crap

`CRAP = CC² × (1 − coverage)³ + CC`. Coverage comes from each language's own
tools, run per build root: `go test -coverprofile`, Vitest, Jest, or c8
(LCOV), coverage.py (LCOV), and JaCoCo or Kover XML for Kotlin. Tools are
the project's own: Node tools from its `node_modules`, run with the package
manager it declares, coverage.py from its virtualenv, and the JaCoCo its
`pom.xml` declares. One that is missing is reported, with what to install,
and never downloaded. Bring your own with `--coverage-command` and
`--coverage-report`; several reports are combined, and code that more than
one report or Go test binary lists counts once, covered if any of them ran
it. With paths or `--changed`, Go and TypeScript coverage runs only the
tests that load those files, which measures them the same as the whole
suite does; `--all-tests` runs the whole suite anyway.

Go code that tests reach by running the built binary counts too, through
Go's integration coverage: while `go test` runs, itos-cc points `GOCOVERDIR`
at a directory of the run's own under `.metrics/coverage/`, then turns
whatever was written there into a profile (`go tool covdata textfmt`) and
merges it with `go test`'s, a line covered if either covers it. A project
opts in by having its test harness build the binary with `go build -cover`
when `GOCOVERDIR` is set; one that does not measures as before. `go test`
replaces `GOCOVERDIR` in each test binary's environment with a directory of
its own, so itos-cc runs each test binary itself (`go test -exec`), with
`GOCOVERDIR` set to the run's directory.

A build root whose coverage could not be measured (tests that do not
compile, a missing tool, no report) shows `N/A`, not 0%, and is named on
stderr. With `--threshold`, a function above it exits 1, and so does a
coverage run that measured nothing, such as tests that do not compile; a
missing tool or report exits 3. A broken test setup never passes the gate.
A file no test loads is untested, not unmeasured: when the coverage run for
its language succeeded and wrote its report, its functions show 0%, even
when it is scored alone and the run found no test for it.

A function with branches in the report is scored by the share of branches it
took (LCOV `BRDA` blocks with two or more arms, JaCoCo branch counters,
coverage.py with `--branch`); one without is scored by its lines. Go
profiles have no branches, so Go is scored by statements. c8's LCOV lists V8
blocks rather than branches, so a project with neither Vitest nor Jest falls
back to lines. Only the current Vitest major (5) is supported; older ones
are reported and left unmeasured. Python and TypeScript coverage is measured from the first line
of the body, because loading a module executes every `def` and
`export const f = …` line.

### dry

Local names, field names, and literals are normalized away; called function
names, operators, and the tree's shape stay. Functions are compared by the
Jaccard similarity of their subtree fingerprints. The default threshold is
0.82.

### mutation

`mutation run` swaps operators (`<`/`<=`, `==`/`!=`, `&&`/`||`, `+`/`-`, …),
deletes `!` and unary `-`, and flips `true`/`false` and `0`/`1`, one at a
time, inside functions only, and runs the tests against each change.
`mutation list` lists those changes, the mutation sites, without running any
test. What keeps `mutation run` fast:

- **Differential runs.** Each function's source is hashed, and so is each
  test file that imports the function's file. Killed mutants of unchanged
  functions stay killed while those tests are unchanged too, as do the
  survivors `itos-cc.yaml` excepts; other survivors, changed functions, and
  every function of a file whose tests changed rerun.
- **Coverage first.** Mutants on lines no test executes are reported as
  uncovered and never run.
- **Narrow, fail-fast test runs.** The file's own Go package (`-failfast`),
  `vitest related` / `jest --findRelatedTests`, in Python the test files
  that reach the file (named `test_*.py` or `*_test.py`), as
  `pytest -x <files>`, or `python -m unittest <modules>` without pytest,
  and in Kotlin the test classes the test files that reach the file
  declare, as `gradle -p <module> test --fail-fast --tests <class>…`, or
  `mvn -q test -Dtest=<classes> -Dsurefire.failIfNoSpecifiedTests=false`.
  Coverage comes from the same tests, so a line only integration or
  end-to-end tests reach is uncovered, not a survivor; a Vitest or Jest
  project's `coverage` script measures mutation run only with `--all-tests`.
  A TypeScript project with neither Vitest nor Jest installed runs its test
  script, which cannot be narrowed: its outcomes are the whole suite's,
  recorded with scope `all-tests` and the whole suite's evidence, so any
  test change makes them stale, as a `--test-command` outcome. A Python or
  Kotlin file no such test reaches runs no test, and its
  mutants are uncovered. A test reaches code through the helpers it imports
  and, in Python, the `conftest.py` files above it, but a Python or Kotlin
  test that runs code only through a subprocess, the CLI or reflection
  reaches none of it, so that code's mutants are uncovered too: judge them
  with `--all-tests` or `--test-command`. Every command itos-cc
  composes runs with `PYTHONDONTWRITEBYTECODE=1`, and every pytest one with
  `-p no:cacheprovider`, so measuring and judging write no bytecode and no
  `.pytest_cache` into the project; a `--test-command` or
  `--coverage-command` runs exactly as given.
- **Parallel workers** in private copies of the project, so the real tree is
  never modified while tests run. The baseline runs inside a worker, which
  proves the copy works before any mutant does. A mutant times out after
  the baseline's time times `--timeout-factor` (10 by default), plus 5
  seconds for building the mutated code, and a timeout counts as killed.

`--all-tests` runs the whole suite for coverage and for every mutant, so
integration and end-to-end tests anywhere in the build root can kill
mutants. In Go, tests that run the built binary show up in coverage when
the binary is built with `go build -cover` under `GOCOVERDIR` (see
[crap](#crap)), and each mutant's run rebuilds it from the mutated copy, so
their kills count. `--json` says, for each mutant whose line is covered,
which coverage covered it: `"in-process"`, `"integration"`, or both. In
other languages such tests do not show up in coverage, so add
`--no-coverage` to let them reach code nothing else covers. It is slow; run
it nightly rather than on every change:

```bash
itos-cc mutation run --changed                               # while working: own tests, fast
itos-cc mutation run --changed --fail-fast                   # while fixing: stop at the first survivor (Linux and macOS)
itos-cc mutation run --since origin/main --fail-uncovered    # a gate: the functions the branch's commits changed
itos-cc mutation run --all-tests                             # nightly, e.g. a scheduled CI job
itos-cc mutation run --all-tests --no-coverage               # nightly, with end-to-end tests that run a binary without coverage
itos-cc mutation list src/billing                            # the mutation sites, without running tests
itos-cc mutation check --since origin/main --fail-uncovered  # a commit hook: cached results, nothing run
itos-cc mutation sample                                      # in CI: do 20 cached results still hold?
itos-cc mutation run --count 20 --since origin/main          # bounded and fresh: 20 committed sites, no cache (Linux and macOS)
itos-cc mutation except src/board.ts:3:13 --reason '…'       # an equivalent mutant: no test can kill it
```

A suite that runs the built program, such as end-to-end scenarios that one
test function runs, can be listed in `itos-cc.yaml`, so each mutant runs
only the tests that reach its line rather than the whole suite:

```yaml
mutation:
  tests:
    list: go run ./features/list   # a test a line: its ID, then a tab and its file, or not
    run: go test ./features -args -tests={pattern}
    ids_pattern: "{ids}"
    join: {each: "{id}", sep: ","}
    whole: go test ./features      # optional: every test; else run with every ID
    support: ["features/*_test.go"] # optional: files every test depends on
```

The commands run through the platform shell at the project root. When a
mutant has to run, the list command runs once (one that fails is
`tests.list-failed`, exit 1, and nothing is judged), then every listed test
for coverage, with `ITOS_CC_TEST_COVERDIR` set to a directory of the run's
own. A harness that builds the binary with `go build -cover` and gives the
processes each test starts `GOCOVERDIR=<that directory>/<test ID>` splits
the coverage by test in one run (`go test` passes that variable on to its
test binaries, as it does not `GOCOVERDIR`); otherwise each test runs alone,
with a `GOCOVERDIR` of its own. A mutant runs its file's own tests first
and, only if it survives them, the listed tests that reach its line, in one
run. The first time a mutant needs a selection of listed tests, that
selection runs once without any mutant in the mutant's worker's copy: its
time, times `--timeout-factor`, plus 5 seconds, is the timeout of
every mutant run of it, so a mutant that hangs one quick scenario waits for
that scenario's time, not the whole suite's. A selection that fails without
a mutant is `tests.selection-failed`, exit 1, with its `ids`: no mutant that
would run it is judged, and the files holding them keep their snapshots, as
with a failing baseline. That outcome has scope `"listed"` and records their
IDs under `tests`, in the snapshot and in `--json`, and `mutation sample`
runs it again the same way, after its selection's own baseline. A line a listed test reaches is never uncovered. Such a kill
rests on files no import names: the snapshot records the hash of the file
each of its tests is defined in, and of every file the `support` globs
match, such as the step code the tests run, and the kill holds while they
are unchanged. `mutation check`, `run`, `sample` and the graph call a
function stale when one changed, naming it; a run then reruns its listed
mutants, own tests first. No list command runs to judge it: deleting a
test changes its file. Only Go programs
report coverage per test so far, and listed tests are not run with
`--no-coverage`, `--all-tests`, `--test-command`, or coverage read from
reports.

`--since <ref>` judges only the functions the commits since `<ref>`
changed, `git diff <ref>...HEAD`: a branch's own commits, never uncommitted
work, so a gate judges a task by what it committed. A deleted line counts as
a change of the function around it, and paths narrow the range. The other
functions neither run nor change in the snapshot, and only those judged count
in the summary, the problems, and the exit code; `--json` names them in each
file's `judged`. The exception is a file whose tests changed since its
snapshot: its results no longer hold, so the functions not judged lose their
entries, and a later run or check finds them missing. A file the range
changed only outside its functions, such as its imports, has none judged and
is left as it was: neither its snapshot nor its summary comment is written.
`--since` follows renames: a move is no change, so a renamed file judges only
the functions the range edited, and its snapshot moves to the new path, even
when none is judged. Paths narrow the range by the path a file has now.

Uncovered mutants never run, so a changed function no test executes passes.
`--fail-uncovered` makes each one a failure: listed like a survivor, a
`mutation.uncovered` problem in `--json`, and exit 1. With `--since`, only the
judged functions' uncovered mutants count. With `--no-coverage`, or where
coverage measured nothing for the language, every mutant runs and none is
uncovered, without `--fail-uncovered`. With it, a TypeScript, Python or
Kotlin build root with mutants to judge whose coverage tool is missing, or
whose coverage command fails or writes no report, fails closed as Go does:
`coverage.tool-missing` (exit 3) or `coverage.measured-nothing` (exit 1),
naming its language and directory, and the run stops before any mutant
runs. Reports read with `--coverage-report`, `--use-existing-coverage` or
`--coverage-command` keep the fallback. Go and Python `--fail-uncovered` is
strict: it also requires fresh measured evidence for every judged Go or
Python function, including functions with no mutation sites, and fails each
uncovered Go executable block, and each Python line coverage.py's LCOV report
names executable that no reaching test executed, as
`mutation.uncovered-statement` without changing mutant counts. A Python
function's lines run from its body to its end: its `def` line runs when the
module is imported, not when the function is called. A Python file no test
reaches has its executable lines listed by coverage.py's own analysis,
without running a test, and all of them are uncovered. Empty and
comment-only Go bodies, and Python functions with no executable body line,
have no executable obligation. Strict coverage needs a successful built-in or
listed measurement; `--coverage-report`, `--use-existing-coverage`, and
`--coverage-command` are rejected. An active Go workspace or local
replacement outside the inventoried module is refused; set `GOWORK=off` to
disable workspace use. Strict Go coverage judges Go files alone and strict
Python coverage Python files alone: a TypeScript or Kotlin file of the same
selection is outside their evidence, whether or not a `go.mod` sits above
it, and keeps the uncovered-mutant rule of its language. A counted run
(`--count`) does not yet prove Python lines.

`--count N` judges at most N mutation sites freshly, without the cache, as
a bounded check of committed work. It resolves the repository and HEAD once
and judges that commit's tracked files in a private frozen copy under the
checkout's git directory (`git rev-parse --git-path itos`; a linked
worktree's own works too), so uncommitted changes play no part; tools and dependencies are used as
installed, from the live project, and nothing is installed or downloaded:
TypeScript's `node_modules` and Python's `.venv` or `venv` (else the
virtualenv `VIRTUAL_ENV` names) at its build roots, and the Go module,
Gradle and Maven caches, with Go commands run with `GOPROXY=off`, Gradle
with `--offline` and Maven with `-o`. A dependency missing offline fails the
run as `count.preparation-failed`, naming the stage, and so does a scratch
directory replaced by a symlink or a file (stage `scratch`). Only a
language with something to judge is measured, one with an eligible site in
the selection and Go, whose admitted functions strict Go coverage judges,
so a file of another language with no site needs no tool. Sites of the selection (paths, and `--since`) are ranked by
SHA-256 over `--seed TEXT`, the HEAD commit's id by default, and the first N
across every file and function are selected. Coverage, listed reach and
each selected file's clean baseline are measured on the frozen copy, and any
failing command fails the run before a mutant runs. A selected site no test
reaches is uncovered and never redrawn; every other selected site runs its
own tests once, even when the cache holds a kill, and one that survives them
runs the listed tests that reach its line, after their selection's clean
baseline, as a complete run does: both stages are one trial. A selection
whose baseline fails is `tests.selection-failed`, and a site that needs it
is blocked, with no outcome. Exceptions of the functions with a selected
site apply as in a complete run: a valid one excepts its survivor, which is
still drawn and run, and a stale one fails. `--json` gives `sampling`
(budget, eligible, selected, executed and omitted counts, seed, algorithm
and commit), the `selected` sites with their `state`, outcome and the
stages their trial ran, the judged and omitted `subjects`, and the `stages`
that ran. A counted run writes no snapshot, comment or coverage
cache, and its pass proves only its sampled judgments; `--count` bounds
mutant trials, not discovery, baselines or total time. A range with no site
is reported not applicable. SIGINT or SIGTERM interrupts it: the active
judgment is cancelled with no outcome, every process it started is stopped
within five seconds, and the partial report exits 75 with
`count.interrupted`. It runs on Linux and macOS, where every command it
starts runs in a process group it owns; a command that detaches from its
process group or session is not followed. On Windows it fails with
`count.platform` (#29).

`--fail-fast` stops a complete run at the first actionable failure it
observes, in the order judgments finish, for a quick fix-and-retry loop: an
unexcepted survivor, once the listed tests that reach it failed to kill it
too; with `--fail-uncovered`, an uncovered mutant or a strict Go or Python
coverage finding; a stale exception; a file whose tests fail before any mutant; or a
selection of listed tests that fails without any mutant. It reports that
failure under the rule a run without it reports, and exits as that rule
says. Killed, timed-out, validly excepted and listed-killed mutants never
stop it. A failure planning or coverage already shows stops it before any
baseline or mutant runs; listing and coverage keep their scope, so it bounds
no total time. At the stop no further command starts, each judgment still
running is cancelled with no outcome, never killed or timed out, and every
process the run started is stopped and joined within one shared five-second
deadline from the stop, before its worker copies are removed. A file whose
every selected mutant was decided is written as usual. A file the stop cut
short, or whose mutant a failing selection of listed tests blocked, gets no
comment and keeps its source bytes, but its snapshot keeps every judgment
the run completed or reused, with the scope and freshness evidence that
decided it. A cancelled, blocked or unattempted mutant gets no outcome; it
keeps what the snapshot recorded for it that still holds, if anything, so a
stopped `--mutate-all` rerun never drops a valid result. `mutation check`
still fails a function with a mutant no valid result records, and the next
run reuses what was kept while its inputs hold and runs only the rest.
`--json` adds `stop` (`stopped`, and the failure's `rule` and `subject`) and
`work`, the selected mutants `completed` (run, reused or measured
uncovered), `cancelled`, `unattempted` and `blocked`, disjointly; each file
gets its `state` and `work`, the run's own work on it, `ran` and `reused`
counting what it ran and reused of the file in every state, blocked
included (its completed work less any measured uncovered), its `cache`
(`complete` when its snapshot, as the run leaves it, holds a valid result
for every mutant of its judged functions, else `incomplete`) and its
`baseline` as it ran (`not-run` when it never did), and each mutant its
`state`, with an outcome only when completed. Plain output says the run stopped early and why. It runs on Linux
and macOS, where every command it starts runs in a process group it owns;
on Windows it fails with `fail-fast.platform`, exit 3, before any command
runs (#29). It refuses `--count`, which stays aggregate. Without it, runs
are exactly as before.

`mutation check` gives the verdict a run would give from the cached results
alone, running no test and no coverage command and writing nothing. Run
`mutation run` while working, and check in a commit hook,
`itos-cc mutation check --since <base> --fail-uncovered`: each function the
commits changed needs results for its code as committed. A function with no
entry in `.metrics/mutate/` is `mutation.missing`, one changed since its
entry is `mutation.stale` (a move is not a change), as is every function of
a file whose tests changed (below), and a fresh entry fails
on a recorded survivor, or an uncovered mutant with `--fail-uncovered`; each
exits 1. It takes `mutation run`'s paths, `--changed`, and `--since`, and
`--json` gives each file's `functions` with their `state`: `fresh`, `stale`,
or `missing`.

For strict Go and Python coverage, `mutation check` additionally rejects
missing or stale per-function block or line evidence without running tests,
coverage, or list commands. Go evidence fingerprints module Go sources and
tests, module configuration, project config, configured support files,
producer options, and relevant Go build settings. Python evidence
fingerprints every Python file of the build root (the file's reaching tests,
their helpers and `conftest.py` files, and the code they import included),
its `pyproject.toml`, `setup.cfg`, `tox.ini`, `pytest.ini` and `.coveragerc`,
project config, configured support files, the producer and the interpreter
with its environment; a virtualenv, hidden directories and nested build roots
are left out. Raw profiles and reports cannot be stamped with current
hashes. `mutation sample` runs a few cached mutants
again, in CI say, and fails with `mutation.mismatch` when an outcome differs
from the one recorded, whichever way: a kill that now survives, or a
survivor now killed. Killed and timeout agree. It draws `--count` mutants (20
by default) from the fresh entries of its selection, only those that ran
(killed, timeout, or survived), so no coverage runs; stale and missing
results are `mutation check`'s. The draw is seeded with the HEAD commit's
id, so a rerun of one commit samples the same mutants; `--seed` reproduces
another run's. It writes nothing, and takes `mutation check`'s paths,
`--changed`, and `--since`. Each outcome in a snapshot records the scope of
the tests that decided it, the file's own tests, `--all-tests`, or the
`--test-command` line, and keeps it when a later run reuses it, so `mutation
sample` re-runs each mutant in its own scope and a kill only the whole suite
makes is not read as a survivor. `--all-tests` or `--test-command` given to
it overrides the scope of every mutant. A mismatch carries the `scope` its
mutant ran with, and its fix re-runs the file in that scope: `itos-cc
mutation run --mutate-all` with `--all-tests` or `--test-command '<line>'`
where the scope was one of those.

For outcomes recorded with `--all-tests` or `--test-command`, in every
language, cached freshness also depends on every test file beneath the
source's build root and on the files matched by `mutation.tests.support`. The
build root is Go's nearest `go.mod` (every `_test.go` file, build-tagged ones
included); TypeScript's nearest `package.json`; Python's nearest
`pyproject.toml`, `setup.py` or `setup.cfg`; and Kotlin's Gradle build root,
which holds `settings.gradle` or `settings.gradle.kts`, or else its Maven
module's `pom.xml`. Test files are those project discovery calls tests, so
`node_modules` and `.venv` are never read and `conftest.py` counts, and a
nested build root of the same language keeps its tests to itself. The
evidence is saved under `suite_evidence`; Go's older `go_evidence` reads the
same. Add feature files and other custom command inputs to those support
globs; arbitrary inputs are not inferred. `mutation check` and the graph
compare saved hashes only and never run tests or the list command. A plain
run retains the recorded scope and evidence when it reuses a broad-scope
outcome; a partial run keeps stale evidence for functions it does not judge.
Older broad-scope outcomes without this evidence, Go's or another
language's, rerun once.

An equivalent mutant changes no behaviour (a `0` set again before it is
ever read, say), so no test can kill it, and it would fail every run. `mutation except <file>:<line>:<column> --reason '…'` excepts it in
`itos-cc.yaml` at the project root, itos-cc's project settings, to commit
and review like the code. The site must be a survivor its fresh snapshot
records. The entry, under `mutation.exceptions`, holds the file, the
function, the function's hash, the site's `line_in_function` (counted from
the function's first line, so a move keeps it) and `column`, the `original`,
the `replacement`, and the `reason`; excepting a site again replaces it, and
the file's other keys and comments are kept:

```yaml
mutation:
  exceptions:
    - file: src/board.ts
      function: board.Board#count
      hash: 9c1f0e2a7b3d4c5e
      line_in_function: 2
      column: 13
      original: "0"
      replacement: "1"
      reason: c is set again before it is read
```

An excepted survivor fails nothing in `mutation run` and `mutation check`: it
is counted `excepted`, not `survived` (the summary shows `N excepted` only
where there is one; `--json` gives each file `excepted` and the mutant its
reason, its `outcome` still `survived`). It is reused without running, as a
kill is, while its function and the tests that import its file are
unchanged; `--mutate-all` runs it too. The entry no longer holds, and fails
as `mutation.exception-stale` (exit 1) with its `why`, when the mutant, run
again after its tests changed, is now `killed`; when its function `changed`,
and the mutant is judged as if it had no entry; or when its function or its
site is `gone`. With `--since`, only the judged functions' entries count. A
run or check of the whole project (no paths, no `--changed`, no `--since`),
and a `--since` run whose range deleted or renamed an entry's file, judge
the entries of files they do not select: when exactly one source they
select holds a function of the entry's name and hash, the entry has
`moved`, still excepts the mutant at its site there, and fails with
`new_file` naming the file to put in the entry; otherwise its file is
`gone`. Only `mutation except` writes `itos-cc.yaml`. An
entry never excuses an uncovered mutant: that needs a test, not a reason. An
`itos-cc.yaml` that cannot be read, or an entry with no reason, is
`config.invalid`, exit 2.

Killed mutants are kept per function in `.metrics/mutate/`, so the day's
runs reuse the night's kills for code that has not changed. Each file's
snapshot also records, under `tests`, the SHA-256 of every test file that
reaches the file, by its path from the project root. In TypeScript, Python
and Kotlin those are the test files whose imports reach its module, directly
or through other modules of the project (Kotlin's same-package references
included), as `vitest related` and `jest --findRelatedTests` select them.
A test also reaches what the test-support files it imports reach, such as a
helper under `tests/` or `__tests__/` or a Kotlin helper in `src/test`,
which are in the set themselves, and a Python test what every `conftest.py`
in its directory and those above it, up to its build root, reaches. In
Go they are the test files of its package and of the packages that import
it, one hop: a Go test that reaches the package only through another package
is not in the set. That set is the same whichever command ran the mutants
(its own tests, `--all-tests`, or `--test-command`). When a test file is
added to it, changed, or removed, the kills it may have made no longer hold:
the file's mutants all run again, and `mutation check` calls its functions
stale and names the test files. A snapshot written before snapshots recorded
tests is stale too, and so, once, is one written when a TypeScript, Python or
Kotlin file's set held only the tests that import it directly, if a test
reaches it through another module. A test that runs the built binary is not
in the set, so changing it leaves the results as they were. Commit that
directory, and have the nightly job commit it back: CI starts from a fresh
checkout, and without it every night is a first run. This repository's own
nightly job, [`.github/workflows/nightly.yml`](.github/workflows/nightly.yml),
runs `itos-cc mutation run --all-tests --no-annotate` at 09:00 UTC and commits
`.metrics/mutate/` back even on nights when a mutant survives and the job
fails; copy it as a starting point. Raw coverage under
`.metrics/coverage/` ignores itself and is never committed.

Results go to `.metrics/mutate/<file>.json`, and a summary comment is kept at
the end of each source file (`--no-annotate` turns it off): its counts, each
survivor, and each survivor `itos-cc.yaml` excepts, counted and listed apart
with its reason, whichever functions the run judged. A surviving
mutant, an uncovered one with `--fail-uncovered`, or tests that fail before
any mutant, exits 1. `--json` lists each file's `mutants` in site order,
each with the keys `mutation list` gives a site, its `outcome` (`killed`,
`survived`, `timeout`, which counts as killed, or `uncovered`), and `reused`,
true when the outcome came from the snapshot without running; with
`--since`, only the judged functions' mutants, and none for a file whose
baseline failed.

### scrap

Finds the test cases of Vitest/Jest, pytest/unittest, Go `testing`
(including `t.Run` and table loops), and JUnit/kotest, and scores each one on
size (fixture text and case tables excluded), logic, mocking, and assertions (helpers that
assert count as assertions). Similar examples are clustered with dry's
fingerprints. Each file gets one action: `LEAVE_ALONE`, `AUTO_TABLE_DRIVE`,
`AUTO_REFACTOR`, `MANUAL_SPLIT`, or `REVIEW_FIRST`, plus ranked
recommendations with line ranges. Every run is compared with the previous
snapshot, so rerunning after a refactor says whether it helped.

## The architecture viewer

`itos-cc serve` watches one or more repositories and publishes their
architecture over HTTP; `viewer/` is a separate web app (TypeScript, React,
React Flow, ELK) that draws it as a canvas you drill into: repositories →
directories → modules → functions.

```bash
itos-cc serve . ../other-repo          # API on http://127.0.0.1:7070
cd viewer && npm install && npm run dev # viewer on http://localhost:5173

# or serve a built viewer from the same port
(cd viewer && npm run build) && itos-cc serve --ui viewer/dist .
```

- A module is a file in TypeScript, Python, and Kotlin, and a package in Go.
  Directories with a single child collapse into one box, so a deep
  `src/main/kotlin/com/acme` tree is one click, not five.
- Solid arrows are imports, rolled up to the boxes on the current level;
  boxes sit above what they depend on. Arrows in a dependency cycle are red at
  every level. Kotlin dependencies within a package count even without an
  import, and Python's `importlib.import_module("x")` counts as an import.
- Dashed arrows are HTTP: a request (`fetch`, `EventSource`, axios,
  `requests`, `httpx`, `http.Get`, RestTemplate, WebClient, Ktor client)
  whose path matches a route another module serves (`net/http`, chi, gin,
  echo, Express, Fastify, Hono, Flask, FastAPI, Spring, Ktor). Matching runs
  across every repository served together, so a frontend in one repository
  points at the service it calls in another. Requests whose path is all
  parameters, such as `${base}/${path}`, are too vague to place and are
  skipped.
- Box color runs from red to green and combines CRAP with the mutation
  score; grey means not measured yet. A module is as risky as its worst
  function. A directory or repository is graded by the share of its functions
  that are risky, and by its overall kill rate, so one bad file does not
  paint a whole system red. The bar under each box splits its functions into
  risky, worth a look, low risk, and unmeasured. `mut 100% · 6/53` means every
  mutant run was killed, in the 6 of 53 functions mutation-tested so far.
- TypeScript imports through `tsconfig.json` aliases (`paths`, `baseUrl`,
  `extends`, and Vite-style `references`) resolve to project files.
- Saving a file updates complexity, dependencies, and CRAP (live complexity
  with the last measured coverage) within a second. Rerunning `crap`,
  `mutation run`, or `dry` updates their numbers. A function's mutation
  results are marked stale exactly when `mutation check` calls them stale,
  by check's own rule: the function was edited, or a test file that imports
  its file changed, since its last mutation run; they lack one of its
  sites; or a `mutation run --since` kept them marked stale.
- Click a box for its functions, a function for its source, an arrow for the
  imports behind it. Double-click or Enter opens a box; Esc goes up.

API: `GET /api/graph` (JSON), `GET /api/events` (server-sent events carrying
the graph version), `GET /api/source?repo=&file=` (only files in the graph).
The server listens on localhost only.

## .metrics

Every command writes a JSON snapshot under `.metrics/`: `crap.json`,
`dry.json`, `scrap.json`, and `mutate/`. `.metrics/` sits at the project
root, the git top level of the working directory (outside a git repository,
the working directory), wherever a command runs, and every path a snapshot
records is relative to that root and slash-separated; paths on the command
line and in output stay relative to the working directory. Commit them: a clone then has the
numbers without rerunning, and mutation results are shared, so nobody reruns
mutants that are already killed. Snapshots carry no timestamps and are
sorted, so an unchanged result is an unchanged file. Raw coverage reports go
to `.metrics/coverage/`, which ignores itself.

## Layout

| Package | Role |
| --- | --- |
| `lang` | Parsing, units (functions and methods), namespaces, complexity, and each language's syntax and mutation rules |
| `project` | Finding source and test files |
| `coverage` | Running coverage and reading LCOV, Go profiles, and JaCoCo |
| `crap`, `dry`, `mutate`, `scrap` | The tools |
| `metrics` | Snapshot files |
| `graph`, `server` | The architecture graph and its live HTTP API |
| `cmd/itos-cc` | The command line |
| `viewer/` | The web viewer, a separate npm project |

Adding a language means one file in `lang/` (grammar, units, decisions,
syntax, mutation rules), test-framework rules in `scrap/`, and a coverage
plan in `coverage/`.

## License

[GNU Affero General Public License v3.0](LICENSE).
