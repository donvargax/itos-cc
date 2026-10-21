# What only the binary driver reaches

Decision 52 holds the scenarios in process against the fakes locally and leaves the built binary
to CI and CD, so code only `TestFeatures` reaches is checked nowhere a person runs. T-154 measured
it once, on one tree, and names the local tests that would reach it. Nothing here changes the
product; the ideas at the end are the follow-up.

## The measurement

One tree, commit 7e83825 (with only the registry change taking T-154 on top), on Linux 7.2 with
8 CPUs, Go 1.27.3. Two module-wide runs, one after the other, nothing else heavy meanwhile, after
one compile-only run (`go test -run '^T154NoTest$' -covermode=set -coverpkg=./... ./...`, 35
seconds) so neither tier's wall time holds the cold build.

The environment both tiers share, as `cd.yml`'s languages jobs set it:

| What | Here | CI's languages job |
|---|---|---|
| `ITOS_CC_LANGUAGE_TOOLS` | `1` | `1` |
| `ITOS_CC_NODE_MODULES` | `testdata/tools/node/node_modules`, from `npm ci` there | the same |
| Python | 3.14.8, a virtualenv outside the tree first on the `PATH`, `testdata/tools/python/requirements.txt` installed in it (pytest 9.1.1, coverage 7.16.2) | 3.14, the same pins |
| Node | 24.22.0 | 24 |
| Java, Gradle, Maven | OpenJDK 17.0.20.1, Gradle 9.8.1, Maven 3.10.0 | Temurin 25, Gradle 9.8.1, Maven 3.9.16 |
| `CI`, `ITOS_CC_FEATURES_BINARY` | unset | `CI` set |

Nothing needed a system package: Node's and Python's tools came from the tree's pins, and the
JDK, Gradle and Maven were already installed, older or newer than CI's.

The local tier:

    go test -count=1 -timeout 90m -covermode=set -coverpkg=./... -coverprofile=local.out -json ./...

with `ITOS_CC_FEATURES_BINARY_DRIVER` and `GOCOVERDIR` unset. `TestFeatures` logged that the
binary driver ran nothing and passed.

The full tier: the same command with `ITOS_CC_FEATURES_BINARY_DRIVER=1`, `GOCOVERDIR` naming an
empty folder, and `-exec` naming this wrapper:

    case "$1" in
      */features.test) export GOCOVERDIR="$T154_GOCOVERDIR" ;;
    esac
    exec "$@"

The wrapper is needed because `go test -cover` sets every test binary's `GOCOVERDIR` to a folder
of its own, whatever the environment says, and reads only the test binary's own data from it
(`coverage/gocoverdir.go` says the same, which is why `crap` runs Go tests through itos-cc's own
`-exec`). Without it, `TestFeatures` still builds itos-cc with `-cover`, but what the scenarios
run lands in go test's folder and is thrown away. The wrapper gives the features test binary
alone the empty folder, so the other packages' tests run as they do in the local tier.

The folder then held 15 metadata files: itos-cc's (atomic mode) and 14 of fixture projects
(set mode), which the scenarios' own builds wrote there because the binary driver passes
`GOCOVERDIR` on to every command. `go tool covdata textfmt` refuses a folder mixing modes, so each
metadata file's counters were converted on their own and itos-cc's profile kept:

    go tool covdata textfmt -i=<the folder of itos-cc's metadata and counters> -o=binary.out

The full profile is `full.out` and `binary.out` together, a block counted covered when either
covers it. Both tiers leave out what `itos-cc.yaml` excludes from measurement (`features/**`,
`**/porttest/**`, `cmd/itos-cc/commands_gen.go`). Functions are those of the Go declarations
holding each block; the raw profiles stayed outside the tree.

## Totals

| Tier | Statements covered | Of 13,747 | Wall time |
|---|---|---|---|
| Local | 12,138 | 88.30% | 198 seconds |
| Full | 12,238 | 89.02% | 613 seconds |

The full tier covers 100 statements more, in 69 blocks of 39 functions in 13 packages. No block
the local tier covers is missing from the full tier. Between the two runs' go test profiles alone
the only differences were 3 blocks of `internal/processes` the local run reached and the full
run did not, timing in the process-group tests, so the noise is that small.

`cmd/itos-cc` took 196 seconds in the local tier and 438 in the full tier, sharing the CPUs with
the binary driver; the features package took 611 seconds with the driver against 37 without.

## Skipped and failed scenarios

The binary driver ran 642 scenarios: none skipped, every tool a scenario declared being
installed, so nothing it alone reaches is unmeasured for a missing tool. Under
`ITOS_CC_LANGUAGE_TOOLS` a missing tool would have failed its scenario, and none did.

Seven failed, all under the coverage build only: ID-MUT-123, 178, 179, 182, 184, 197 and 211.
Run again with the binary driver and no `GOCOVERDIR`, all seven pass. Two causes, both the
coverage build's `GOCOVERDIR` reaching the commands itos-cc runs for the project:

- ID-MUT-123: a `-cover` itos-cc wrote atomic-mode counters into the per-test folder itos-cc gave
  a listed test's `GOCOVERDIR`, and `go tool covdata textfmt` refused that folder ("counter mode
  clash"), so the listed tests measured nothing.
- The other six: their fixtures' tests record whether `GOCOVERDIR` was set, which is how they
  report a coverage run, and the harness's inherited `GOCOVERDIR` made trials itos-cc runs
  without coverage read as coverage runs.

Those scenarios ran to the failing step, so most of what they reach is measured. The blocks only
they reached in the full tier are marked in the table. The idea
`features-coverage-gocoverdir-leak` records the harness gap.

## Not measured by either tier

A process ended by a signal writes no coverage counters. `main`'s interrupt branch and
`endInterrupted`, and `cmd/itos-cc/raise_unix.go` (5 statements), are covered by neither tier,
although the binary driver's interrupt scenarios (ID-MUT-251 and the rest of its rule, and
`features/interrupt_test.go`) run them in CI and CD: itos-cc re-raises the signal by design, so
the run dies before it writes. `internal/cli/interrupt.go`, the signal handling's logic, is
covered entirely by the local tier's unit tests. Signal-to-exit paths therefore do not appear in
the difference below, and stay CI's and CD's.

Build stamping does not appear either: `versionString` and the stamped version are covered
locally.

## What only the full tier reaches

By group, with the functions' statements only the full tier covers:

| Group | Functions | Statements | Local test that would reach it |
|---|---|---|---|
| main's wiring | 1 | 8 | none: stays CI's and CD's |
| real-tool adapter: coverage plans, toolchain probe, real reach | 9 | 19 | `language-adapter-cases` |
| real-tool adapter: runner failures | 4 | 9 | `coverage-runner-failure-cases` |
| worker copies and real processes: cleanup that fails | 6 | 6 | `cleanup-failed-local-reach` |
| git: fresh plans, counted root, changed functions | 12 | 25 | `fresh-git-adapter-cases` |
| other: serve | 3 | 29 | `serve-local-tests` |
| other: domain code only `@binary-only` scenarios reach | 4 | 4 | `binary-only-scenarios-in-process` |

By package: `cmd/itos-cc` 4 functions (22 statements), `coverage` 7 (21), `internal/freshenv` 9
(22), `server` 2 (18), `internal/mutation` 6 (6), `internal/mutants` 3 (3), `graph` 2 (2), and
one function each in `internal/coveragetools`, `internal/processes`, `internal/testinputs`,
`internal/testtools`, `internal/worktrees` and `project`.

Every function, its statements only the full tier covers of its total, its group and the idea
that would reach it locally. A † marks blocks reached in the full tier by the seven scenarios
that failed under the coverage build; they may be those failures' own paths.

| Function | File | Only full | Group | Proposal |
|---|---|---|---|---|
| `coverageCommand` | `cmd/itos-cc/coverage.go` | 2 of 10 | real-tool adapter: runner failures | `coverage-runner-failure-cases` |
| `main` † | `cmd/itos-cc/main.go` | 8 of 10 | main's wiring | stays CI and CD |
| `countedRoot` | `cmd/itos-cc/mutate_counted.go` | 1 of 5 | git | `fresh-git-adapter-cases` |
| `runServe` | `cmd/itos-cc/serve.go` | 11 of 21 | other: serve | `serve-local-tests` |
| `Plan.prove` | `coverage/measure.go` | 2 of 4 | real-tool adapter: coverage plans | `language-adapter-cases` |
| `MeasureTests` † | `coverage/pertest.go` | 3 of 55 | real-tool adapter: runner failures | none: the covdata failure the coverage build caused |
| `pythonRunner` | `coverage/python.go` | 1 of 4 | real-tool adapter: coverage plans | `language-adapter-cases` |
| `TypeScriptRunner` | `coverage/run.go` | 2 of 7 | real-tool adapter: coverage plans | `language-adapter-cases` |
| `typescriptPlan` | `coverage/run.go` | 9 of 31 | real-tool adapter: coverage plans | `language-adapter-cases` |
| `kotlinPlan` | `coverage/run.go` | 1 of 32 | real-tool adapter: coverage plans | `language-adapter-cases` |
| `MeasureTestsSupervised` | `coverage/supervised.go` | 3 of 79 | real-tool adapter: runner failures | `coverage-runner-failure-cases` |
| `index.kotlin` | `graph/graph.go` | 1 of 10 | other: real reach | `language-adapter-cases` |
| `importingTests` | `graph/tests.go` | 1 of 52 | other: real reach | `language-adapter-cases` |
| `Measurer.named` | `internal/coveragetools/measurer.go` | 1 of 12 | real-tool adapter: runner failures | `coverage-runner-failure-cases` |
| `orDir` | `internal/freshenv/baseline.go` | 1 of 3 | git | `fresh-git-adapter-cases` |
| `Environment.plan` | `internal/freshenv/plan.go` | 1 of 97 | git | `fresh-git-adapter-cases` |
| `gitCommandError.Error` | `internal/freshenv/plan.go` | 1 of 1 | git | `fresh-git-adapter-cases` |
| `Environment.gitAtEnv` | `internal/freshenv/plan.go` | 1 of 21 | git | `fresh-git-adapter-cases` |
| `ensureInside` | `internal/freshenv/plan.go` | 1 of 4 | git | `fresh-git-adapter-cases` |
| `Environment.privateScratch` | `internal/freshenv/plan.go` | 10 of 23 | git | `fresh-git-adapter-cases` |
| `Environment.prepare` | `internal/freshenv/preparation.go` | 4 of 178 | git | `fresh-git-adapter-cases` |
| `listFreshTests` | `internal/freshenv/preparation.go` | 2 of 23 | git | `fresh-git-adapter-cases` |
| `withGitConfigEnv` † | `internal/freshenv/preparation.go` | 1 of 8 | git | `fresh-git-adapter-cases` |
| `execute` | `internal/mutants/run_execute.go` | 1 of 112 | worker copies and real processes: cleanup | `cleanup-failed-local-reach` |
| `runMutant` | `internal/mutants/run_execute.go` | 1 of 36 | worker copies and real processes: cleanup | `cleanup-failed-local-reach` |
| `Execution.preserved` | `internal/mutants/run_fail_fast.go` | 1 of 5 | other: binary-only mutation scenarios | `binary-only-scenarios-in-process` |
| `countedPlanError` | `internal/mutation/errors.go` | 1 of 9 | git | `fresh-git-adapter-cases` |
| `reports` | `internal/mutation/run.go` | 1 of 3 | other: binary-only mutation scenarios | `binary-only-scenarios-in-process` |
| `Run.present` † | `internal/mutation/run_ui.go` | 1 of 76 | other: binary-only mutation scenarios | `binary-only-scenarios-in-process` |
| `Presentation.cleanupFailed` | `internal/mutation/run_ui.go` | 1 of 3 | worker copies and real processes: cleanup | `cleanup-failed-local-reach` |
| `CleanupProblem` | `internal/mutation/run_ui.go` | 1 of 1 | worker copies and real processes: cleanup | `cleanup-failed-local-reach` |
| `describeStop` † | `internal/mutation/run_ui.go` | 1 of 7 | other: binary-only mutation scenarios | `binary-only-scenarios-in-process` |
| `CleanupFailed.Error` | `internal/processes/proc_owned.go` | 1 of 1 | worker copies and real processes: cleanup | `cleanup-failed-local-reach` |
| `addProjectCoverageInputs` | `internal/testinputs/line_coverage_inputs.go` | 1 of 8 | real-tool adapter: coverage plans | `language-adapter-cases` |
| `Toolchain.Observe` | `internal/testtools/toolchain.go` | 1 of 49 | real-tool adapter: coverage plans | `language-adapter-cases` |
| `boundWorker.Run` | `internal/worktrees/workers.go` | 1 of 23 | worker copies and real processes: cleanup | `cleanup-failed-local-reach` |
| `ChangedSinceAt` | `project/since.go` | 1 of 61 | git | `fresh-git-adapter-cases` |
| `New` | `server/server.go` | 5 of 7 | other: serve | `serve-local-tests` |
| `Server.rebuild` | `server/server.go` | 13 of 17 | other: serve | `serve-local-tests` |

## The groups and their proposals

Each proposal is an adapter or contract test (decisions 44 and 49) that runs in seconds with no
Python, Node, Gradle or Maven run; none puts a fake in place of the real tool a scenario checks.

**main's wiring** (`main`). The process's signal registration, the call to `run` with the disk
adapters and `os.Exit`. The subprocess helper runs `run` with the same adapters, so all main adds
is the wiring, which only a built binary proves. Stays CI's smoke set and CD's.

**Coverage plans, the toolchain probe and the real reach** (`language-adapter-cases`). The
blocks are choices made from a project's files before any tool runs: a TypeScript package with
its own `coverage` script (run as `<pm> run coverage`, its report Unattested, so `Plan.prove`
proves no file), one using Jest, installed or not (`TypeScriptRunner`, `typescriptPlan`,
`Toolchain.Observe`), a Kotlin module under a build root holding `gradlew` (`kotlinPlan`),
Python without pytest running `unittest` over named modules (`pythonRunner`), a strict project
that has an `itos-cc.yaml` (`addProjectCoverageInputs`), and the graph's Kotlin import of an
unknown package and a Go test's resolved imports (`index.kotlin`, `importingTests`). Cases in
`coverage/run_test.go`, `internal/testtools`, `internal/testinputs` and `graph/graph_test.go` /
`graph/tests_test.go` over fixture files reach them all; only the scenarios that then run the
tools stay CD's.

**Runner failures** (`coverage-runner-failure-cases`). The error paths of running a coverage
command: an owned command that cannot start or exits non-zero (`coverageCommand`), a
`--coverage-command` that fails (`Measurer.named`), and a listed test's command, the whole
suite's command or a listed test's conversion failing (`MeasureTestsSupervised`). Adapter tests
reach each with `sh -c 'exit 3'`, a missing executable or a Go fixture whose test fails:
`coverageCommand` through the subprocess helper (`crap --coverage-command`) or a test of
`cmd/itos-cc/coverage.go` with a real `processes.OwnedExecutor`, the others in
`internal/coveragetools` and `coverage`. `MeasureTests`'s three statements were the covdata
failure the coverage build caused, not a path a passing run takes.

**Cleanup that fails** (`cleanup-failed-local-reach`). ID-MUT-250 alone reaches it: an owned
process group whose cleanup does not finish within the shared deadline (`processes`'s
`CleanupFailed`), mapped by `boundWorker.Run` to `mutants.WorkerCleanupFailed`, cancelling the
judgment (`runMutant`, `execute`) and reported as `mutation.cleanup-failed`
(`Presentation.cleanupFailed`, `CleanupProblem`). A `processes` adapter test whose child leaves a
grandchild holding the output past a short deadline reaches the first; a `worktrees` adapter
test the mapping; a case in `internal/mutants/porttest`'s Workers contract, run against the fake
and the adapter, holds the fake to reporting a cleanup failure as the adapter does. The domain's
handling is then reachable in process only through a scenario: ID-MUT-250 could run in both
drivers once its Given can declare the failure to the fake, which the follow-up decides; its tag
is unchanged here.

**Git** (`fresh-git-adapter-cases`). Real git in the fresh environment and around it: a checkout
whose `.git/itos` lies outside it, as a linked worktree's does (`privateScratch`'s second branch,
`ensureInside`), an unborn `HEAD` (`Environment.plan`, `gitAtEnv`, `gitCommandError.Error`),
`GIT_CONFIG_*` in the environment (`withGitConfigEnv`), a listed-tests listing that fails
(`listFreshTests`, `prepare`'s failed stages), the interpreter path's folder (`orDir`), a range
changing a file of no language (`ChangedSinceAt`), `mutation run --count` outside any repository
(`countedRoot`) and the scratch failure's problem (`countedPlanError`'s `FreshScratch`). Git is
fast and installed wherever the tests run: cases in `internal/freshenv`'s plan, preparation and
contract tests over temporary repositories, one in `project/since_test.go`, a cmd test through
the subprocess helper for `count.no-git`, and a `FreshPreparationContract` case for a scratch
failure in `internal/mutants/porttest`.

**serve** (`serve-local-tests`). `serve` is the one command still on its legacy handler, so the
in-process driver defers its scenarios (`features.Deferred`) and `server` has no tests of its
own. A `server` test building a `Server` over a temporary tree and rebuilding it (`New`,
`rebuild`: graph, JSON, the files it watches, listeners told) and a cmd test through the
subprocess helper with the port held (`runServe` to `serve.port-in-use`) need no network beyond
127.0.0.1. serve's move onto kong would bring its scenarios in process instead.

**Domain code only `@binary-only` scenarios reach** (`binary-only-scenarios-in-process`). A fail-fast
stop's preserved judgments (`Execution.preserved`, ID-MUT-202), the "stopped early" line and a
stop naming its site (`Run.present`, `describeStop`, reached here by ID-MUT-197 among the seven
that failed) and `mutation run --coverage-report`'s reports (`reports`). Under decision 49
domain lines belong to in-process scenarios, not adapter tests; the follow-up finds which
binary-only scenarios reach them and whether each can run in process, as UNIT-06 below.

## UNIT-06 in process

With its `@binary-only` tag removed in the working tree only, ID-UNIT-06 fails in process. Its
first four steps pass: the units listed, the route table and the route-name rule. It fails at
"a route's complexity, mutation sites, duplication, coverage, and hash are its own, not mount's",
whose `mutation run` runs `exit 0` as the project's tests in `src/api`: the in-process driver's
fake `Workers` refuses a command no `Tests` declared ("no tests declared for … exit 0", exit
100, no snapshot written). So its recorded reason, that crap and mutation run are still
hand-parsed, is stale, but the scenario still needs one change to run in process: the step
declares the tests its `mutation run` runs (`Tests`), as other mutation fixtures do. That is in
`binary-only-scenarios-in-process`; its tag is unchanged.

## Summary

Locally the in-process scenarios and the package tests reach 88.30% of the measured statements;
the binary driver adds 0.72 points, 100 statements in 39 functions. Only `main` (8 statements)
needs the built binary for good, and signal endings are measured by neither tier. The rest is
reachable by cheap local tests: serve (29 statements) once it has tests or moves, git in the
fresh environment (25), coverage plans and the real reach from project files (19), coverage
runner failures (9), the cleanup failure (6), and four domain lines that binary-only scenarios
hold, UNIT-06 among those that could run in process. Measuring with `GOCOVERDIR` broke seven
scenarios, which the harness should prevent before the next measurement.
