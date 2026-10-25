# CLI design

itos-cc follows the CLI rules of itos, its host, so a person or an agent meets
the same rules in both: [itos's docs/CLI.md](https://github.com/donvargax/itos/blob/main/docs/CLI.md).
This page names those rules by their numbers there, says whether itos-cc
follows each, and lists where it does not yet. Change this page in the same
commit as the command line.

## Why the codes and problems look as they do

The exit codes are [square/exit](https://github.com/square/exit)'s, Square's semantic exit
codes, which take their model from HTTP's status codes (decision 36). Its two ranges play the
part of HTTP's classes: 80 to 99 are the caller's errors, as 4xx are the client's (80 a usage
error, 81 an unknown command, 82 a requirement not met), and 100 to 119 the program's, as 5xx
are the server's (100 an internal error). Its 0 and 1 are success and a check that said no.
Two codes are kept from outside square/exit: 75 for a failure that may pass when run again
unchanged, and 130 for an interrupt, what a shell reports for a command SIGINT ended.

Each problem carries a stable `rule`, a dotted lower-case id such as `mutation.survived`, and its
own `exit` code, as [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) gives each problem
detail its own `status`. Its subject keys sit beside them as flat members, as RFC 9457's
extension members do; `rule`, `message`, `fix` and `exit` are reserved (decision 34). A script
reads the rule and the code, never the words.

A run with several problems exits with the first code in the precedence order that one of them
has, not the highest: 130, 80 and 81, 100, 82, 75, 1. The first says the most about why nothing
after it could be trusted: an interrupted run proves nothing, and a misused one checked nothing.
`internal/cli`'s `Precedence` holds the order and `ExitCode` applies it; a code missing from it
is a defect of ours, exit 100. Only `internal/cli` turns problems into an exit code: a domain's
failures carry no code, rule or wording.

`docs/architecture.md` is this page's partner for the code: how the commands are laid out, where
each domain's failures become problems, and the gates that hold both.

## The contract with scripts

Scripts can rely on three things only, as with itos (its decision 35):

- The exit code.
- The `--json` output, less every key named `message` or `fix`. These keys
  hold the same sentences as the plain output.
- The files that itos-cc writes: those under `.metrics/`, and the keys of
  `itos-cc.yaml`, the project's settings, which `mutation except` writes
  and a person may write too. A release only adds keys to them, as `tests`
  was added to the snapshots under `.metrics/mutate/` without changing
  their `version`.

All plain output is for people and can change in any release. A change to the
contract is a breaking change, made in a major release with the others that
are ready (rules 42 and 43). Before 1.0 a breaking change is made in a minor
release instead: itos-cc is at 0.x.

A plain line of `mutation run`, `check` or `sample` gives its counts or verdict first and the
file after them, and names a site as `<file> line <line> column <column>`: no such line starts
with a path or holds `path:line:column`, which CI problem matchers, setup-go's among them, read
as a compiler error on a passing run (slice-51, issue #21). A problem's `message`, which `--json`
holds unchanged, keeps its `<file>:<line>:<column>`: a problem is printed only when a run fails.

### Exit codes

| Code | Meaning                                                                                |
| ---- | -------------------------------------------------------------------------------------- |
| 0    | Success.                                                                               |
| 1    | A check said no: a mutant survived, an uncovered mutant, executable Go coverage block or executable Python, TypeScript, Kotlin or Dart line with `--fail-uncovered`, missing or stale mutation/coverage evidence, an exception in `itos-cc.yaml` that no longer holds, a sampled mutant whose outcome differs from its cached one, or that its tests decide differently run to run, a function is over `--threshold`, tests that measure nothing, a list command of `mutation.tests` that fails, the listed tests' coverage run that fails. |
| 75   | A temporary failure: the same command can pass when run again unchanged.               |
| 80   | A usage or config error: a bad flag, path, argument, or report, an `itos-cc.yaml` that cannot be read, a site to except that is no recorded survivor or is in an excluded source. |
| 81   | An unknown command or subcommand.                                                      |
| 82   | A requirement not met: the environment lacks something, a tool, a report, a git repository. |
| 100  | An internal error that itos-cc could not classify, a panic included. Report it.        |
| 130  | Interrupted: SIGINT (Ctrl-C) or SIGTERM stopped the command.                           |

Before decision 36 a usage error and an unknown command were 2, a
requirement not met 3, and an internal error 70. Why the codes and their
order are what they are is the section above.

### Interrupts

Every command, `serve` and `mutation run --count` included, stops on SIGINT
and on SIGTERM (decision 38): its context is cancelled, it says once that it
was interrupted, `itos-cc: interrupted …` on stderr or the rule
`interrupted`, exit 130, in `--json`, and exits 130. On Linux and macOS it
then raises the signal that came again, with its default action, so a shell
reads 130 for SIGINT and 143 for SIGTERM and stops a loop over it; Windows,
where Ctrl-C and CTRL_BREAK arrive as SIGINT, exits 130. `serve` reports the
address it served beside the problem. A second interrupt ends the command at
once, unless the process groups it owns are being killed and joined then
(ADR-0021), which it waits for, up to their shared five-second deadline; the
command then ends on its own. A cancellation that is no interrupt, such as a
`--fail-fast` stop or a mutant's timeout, keeps its own classification. An
interrupted `mutation run` keeps what it finished, as a `--fail-fast` stop
does: each file has its `"state"` and `"work"`, a file it cut short gets no
summary comment, and its `--json` gives no `"stop"`; a counted run's report
says completion `interrupted`. An owned process group whose cleanup does not
finish by its deadline is `mutation.cleanup-failed`, or for a coverage
command `coverage.cleanup-failed`, exit 100, one problem for each group,
beside the run's other problems.

### `--json`

Every command takes `--json` and then prints one object on stdout:

```json
{"schema": 1, "ok": true, "...": "the command's own keys"}
```

A later release only adds keys. A run that did not succeed, a usage error
included, prints the object too, with `"ok": false` and its problems:

```json
{"schema": 1, "ok": false, "problems": [
  {"rule": "mutation.survived", "exit": 1, "message": "…", "fix": "…",
   "file": "src/a.go", "line": 12, "column": 9, "function": "example.com/m/a#Pos",
   "original": ">", "replacement": ">="}
]}
```

`rule` is a stable id, and `exit` the problem's own exit code, as RFC
9457's `status` gives each problem its own (decision 34); the run exits with the
first of its problems' codes in the order above. The other keys besides
`message` and `fix` are the problem's subject. A subject key is never
named `rule`, `message`, `fix` or `exit`: these are reserved, and
`internal/cli` refuses a subject key of one of them. Progress and test output go to stderr, never stdout:
each line itos-cc writes there itself begins `itos-cc:`, such as
`itos-cc: coverage <dir>$ <command>` before a coverage command runs, and the
output of the commands it runs passes as it is.

### Problem rules

| Rule                         | Exit | Command        | Subject keys                                                |
| ---------------------------- | ---- | -------------- | ----------------------------------------------------------- |
| `flags.unknown`              | 80   | every          | `flag`                                                      |
| `flags.value-missing`        | 80   | every          | `flag`                                                      |
| `flags.value-invalid`        | 80   | every          | `flag`, `value`                                             |
| `flags.switch-value`         | 80   | every          | `flag`                                                      |
| `flags.repeated`             | 80   | every          | `flag`                                                      |
| `flags.conflict`             | 80   | mutation run, mutation check, mutation sample, mutation except | `flag`                                        |
| `command.unknown`            | 81   | itos-cc, mutation | `command`                                                |
| `command.missing`            | 80   | itos-cc --json, mutation --json | none                                       |
| `args.unexpected`            | 80   | version, help, mutation except | `argument`                                  |
| `args.missing`               | 80   | mutation except | none                                                       |
| `args.invalid`               | 80   | mutation except | `argument`                                                 |
| `config.invalid`             | 80   | crap, dry, units, mutation run, mutation list, mutation check, mutation sample, mutation except | `file`, and `key` for an `exclude` or `mutation.copy` glob (`exclude[1]`, `mutation.copy[0]`) |
| `exception.no-survivor`      | 80   | mutation except | `file`, `line`, `column`                                   |
| `exception.excluded`         | 80   | mutation except | `file`                                                     |
| `exception.identity-stale`   | 80   | mutation except | `file`, `function`, `identity`, `why` (`changed`, with `hash` the function's hash now, or `gone`) |
| `paths.unmatched`            | 80   | crap, dry, mutation run, mutation list, mutation check, mutation sample, scrap, units | `argument` |
| `changed.no-git`             | 82   | crap, dry, mutation run, mutation list, mutation check, mutation sample, scrap, units | none     |
| `since.bad-ref`              | 80   | mutation run, mutation check, mutation sample | `ref`                                         |
| `since.no-git`               | 82   | mutation run, mutation check, mutation sample | none                                          |
| `sample.no-git`              | 82   | mutation sample, without `--seed` | none                                     |
| `count.platform`             | 82   | mutation run --count | `platform`                                            |
| `count.no-git`               | 82   | mutation run --count | none                                                  |
| `count.unsupported-scope`    | 80   | mutation run --count | none                                                  |
| `count.preparation-failed`   | 1, 82 (a tool missing) | mutation run --count | `stage`                             |
| `count.trial-failed`         | 1    | mutation run --count | `file`, `line`, `column`, `function`, `original`, `replacement`, `identity` |
| `fail-fast.platform`         | 82   | mutation run --fail-fast | `platform`                                        |
| `coverage.command-needs-report` | 80 | crap, mutation run | none                                                    |
| `coverage.measured-nothing`  | 1    | crap --threshold, mutation run --fail-uncovered (a coverage command that failed, even after writing its report, or wrote none, where its language has mutants to judge) | `dir`, `language`, or `report`                            |
| `coverage.tool-missing`      | 82   | crap --threshold, mutation run --fail-uncovered (a coverage tool, or the collector of the processes tests start, missing where its language has mutants to judge) | `dir`, `language`                                         |
| `coverage.no-report`         | 82   | crap --threshold | `dir`, `language`                                         |
| `coverage.report-unreadable` | 80   | crap --threshold | `report`                                                  |
| `crap.threshold`             | 1    | crap           | `file`, `line`, `function`, `crap`, `threshold`             |
| `mutation.survived`          | 1    | mutation run, mutation check | `file`, `line`, `column`, `function`, `original`, `replacement`; with `--count` `identity`, which its message names and its fix's `mutation except` takes |
| `mutation.uncovered`         | 1    | mutation run --fail-uncovered, mutation check --fail-uncovered | `file`, `line`, `column`, `function`, `original`, `replacement` |
| `mutation.uncovered-statement` | 1 | mutation run --fail-uncovered, mutation check --fail-uncovered | `file`, `function`, `line` |
| `mutation.coverage-missing`  | 1    | mutation run --fail-uncovered, mutation check --fail-uncovered | `file`, `function`, `line` |
| `mutation.coverage-stale`    | 1    | mutation check --fail-uncovered | `file`, `function`, `line` |
| `mutation.coverage-unsupported` | 1 | mutation run --fail-uncovered, mutation check --fail-uncovered | `file`, `function`, `line` |
| `mutation.exception-stale`   | 1    | mutation run, mutation check, mutation except --renew | `file`, `function`, `line` (none when the function or its file is gone, or with `--renew` when its site is not found), `column`, `original`, `replacement`, `why` (`killed`, `changed`, `gone`, `moved`, `excluded`; with `--renew` also `ambiguous`, `no-results`, `uncovered`), with `moved` `new_file` |
| `mutation.missing`           | 1    | mutation check | `file`, `line`, `function`                                  |
| `mutation.stale`             | 1    | mutation check | `file`, `line`, `function`                                  |
| `mutation.mismatch`          | 1    | mutation sample | `file`, `line`, `column`, `function`, `original`, `replacement`, `recorded`, `outcome`, `scope` |
| `mutation.flaky`             | 1    | mutation sample | a mismatch's keys and `outcomes`, every outcome its runs gave in run order |
| `mutation.baseline-failed`   | 1    | mutation run, mutation sample | `file`, `output` (the first 20 and last 20 lines of what the tests printed, whole up to 40) |
| `tests.list-failed`          | 1    | mutation run; mutation check, sample and except when they date a kill by a fingerprint | `command`, `exit_code` |
| `tests.coverage-failed`      | 1    | mutation run   | `command`, `exit_code`                                      |
| `tests.selection-failed`     | 1    | mutation run, mutation sample | `ids`, `command`, `exit_code`                 |
| `serve.repo-unreadable`      | 80   | serve          | none                                                        |
| `serve.port-in-use`          | 75   | serve          | `port`                                                      |
| `mutation.cleanup-failed`    | 100  | mutation run   | `process_group`, `command`                                  |
| `coverage.cleanup-failed`    | 100  | crap, mutation run | `process_group`, `command`, `dir`, `language`           |
| `interrupted`                | 130  | every          | none                                                        |
| `internal`                   | 100  | every          | none                                                        |

## The rules

Rules 1 to 43 of itos's docs/CLI.md, as they apply to itos-cc.

A snapshot's `tests` are the test files that reach its file. In TypeScript,
Python, Kotlin and Dart those are the test files whose imports reach its module
transitively (Kotlin's same-package references included, and Dart's
`package:` and relative imports, exports, parts, every target of a
conditional import, and a pub workspace's sibling members), as `vitest
related` and `jest --findRelatedTests` select them; in Go, the test files of
its package and of the packages that import it, one hop. A test reaches
through the test-support files it imports too, files the language counts as
test code that are not runnable tests (a Python helper under `tests/`, a
TypeScript helper under `__tests__/`, a Kotlin helper in `src/test`, a Dart
helper under `test/`): it
reaches what they reach, transitively, and they are among the file's tests
themselves. A Python test pytest runs, and a `conftest.py`, also reaches
what every `conftest.py` in its directory and those above it, up to its
build root, reaches, and a Dart test file what the nearest
`flutter_test_config.dart` in its folder or above it, up to its package,
reaches. A Dart file's `tests` are only those its own tests run: the test
files under its package's `test/` and the support files on their way, never
those under `integration_test/` or `test_driver/`, so a change there makes
nothing stale. A snapshot recorded when a TypeScript, Python or Kotlin
file's tests were only those importing it directly, or before tests reached
through support files, reads stale once if a test reaches the file through
another module or a support file, and is re-run as any stale result is.

`mutation run --json` gives each file's `"baseline"`: whether this run ran
the file's own tests without any mutant. `"passed"` means they ran in this
run and passed, `"failed"` that they failed. Files sharing a command, as a
Go package's files do, share its baseline: one that ran for any of them is
`"passed"` for all. `"not-run"` means none ran for the file in this run: a
file whose mutants were all reused from its snapshot, or otherwise decided
before any ran, while no other file of the run ran its command, or a file
no test reaches. A file whose mutants were all reused said `"passed"`
before; a script that checked for `"passed"` should accept `"not-run"`, or
check for `"failed"` instead. Snapshots record no baseline, so none reads
stale for it.

A Python file's own tests, the scope `own`, are those of its `tests` that
pytest collects by name (`test_*.py`, `*_test.py`): `mutation run` runs them
as `python -m pytest -q -x -p no:cacheprovider <files>`, or `python -m
unittest -f <modules>` without pytest, and measures the coverage that
decides which of the file's mutants run from them too, one coverage run per
distinct set of those files. A Python file none of whose tests is such a
file runs no test: its mutants are `uncovered`, and its `--json`
`"baseline"` is `"not-run"`, as no baseline ran. A test that executes code
only through a subprocess or the CLI reaches none of it, so those kills
need `--all-tests` or
`--test-command`, which keep the whole suite and the given command. Under
`--all-tests` the coverage run measures the Python processes such a test
starts too, through an rcfile of the run's own that `COVERAGE_PROCESS_START`
names (coverage.py 7.13 and later), so their lines are covered, as
`"integration"`, and their mutants run; listed tests (`mutation.tests`)
reach them the same way, one test at a time or as a harness splits them
with `COVERAGE_FILE=<ITOS_CC_TEST_COVERDIR>/<test ID>/.coverage`. A
project whose coverage.py configuration sets `[run] parallel = true`, or
`patch = subprocess`, which forces it, is measured too: itos-cc combines
the parallel data files coverage run writes into the run's own data file,
with the project's `[paths]`, before the report, leaving none behind. Under
the project's own `patch = subprocess` the processes a test starts write
beside the in-process data, so their lines count as `"in-process"`. A
Python outcome of scope `own` recorded before, which the whole suite
decided, still reads fresh while its tests are unchanged; `mutation sample`
re-runs it with the narrowed tests, so a kill only another test made is a
`mutation.mismatch`, and `mutation run --mutate-all` judges it again.

A Kotlin file's own tests, the scope `own`, are the runnable test classes
its `tests` declare, by fully qualified name: each top-level class, neither
abstract nor an interface, with a function annotated `@Test` (or
`@ParameterizedTest`, `@RepeatedTest`, `@TestFactory`, `@TestTemplate`, any
annotation whose name ends in `Test`), or in a class nested in it, a
Kotest spec (a class whose supertype's name ends in `Spec`), or a class
named `*Test` or `*Tests` that extends another, inheriting its tests. A
test-support file, such as a helper in `src/test` that declares no test, is
among the file's `tests` but runs as no class. `mutation run` runs them as
`gradle -p <module> test --fail-fast --tests <class>…` when they are one
Gradle module's, as `gradle :<module>:test --fail-fast --tests <class>… …`
from the build root, each module's test task by the project path its
directory names, when they are several modules', and as `mvn -q test
-Dtest=<classes> -Dsurefire.failIfNoSpecifiedTests=false` in the file's
Maven module, or from the top of its reactor with `-pl <modules> -am` when
other modules hold some. The coverage that decides which of its mutants run
is measured from the classes of its own module (`gradle -p <module> test
--tests <class>… jacocoTestReport` or `koverXmlReport`, `mvn -q
jacoco:prepare-agent test jacoco:report -Dtest=<classes>
-Dsurefire.failIfNoSpecifiedTests=false`), one coverage run per distinct set
of them: a module's report measures its own tests alone, so a class in
another module can kill a mutant but measures none of it. A Kotlin file
none of whose tests declares a class of its build runs no test: its mutants
are `uncovered`, and its `"baseline"` is `"not-run"`. A test that executes code only through a subprocess, the
CLI or reflection reaches none of it, so those kills need `--all-tests` or
`--test-command`, which keep the whole suite and the given command. As in
Python, a Kotlin outcome of scope `own` recorded before still reads fresh
while its tests are unchanged, and `mutation sample` re-runs it with the
narrowed tests.

A Dart file's own tests, the scope `own`, are those of its `tests` under its
package's `test/` named as `flutter test` finds test files, `*_test.dart`.
`mutation run` runs them as `flutter test --no-pub <files>` in the package's
directory, which has no fail-fast flag, and measures the coverage that
decides which of the file's mutants run from them too, one coverage run per
distinct set of them. The tests under `integration_test/` and
`test_driver/` never run: they need a device. A Dart file none of whose
`tests` is such a file runs no test: its mutants are `uncovered`, and its
`"baseline"` is `"not-run"`. A worker resolves its copy of the package once
with `flutter pub get --offline` before its first run. A Dart mutant that
does not compile fails its tests and is `killed`, as a Go or Kotlin one is.

A worker copies a TypeScript package's own folder, or, when the package
belongs to a workspace, the workspace's whole root: a `pnpm-workspace.yaml`
with a `packages` key, or a `package.json` with a `workspaces` field, as npm
and yarn read it, in a folder above the package and within the project
root, whose globs match the package's folder. The copy leaves out what every
copy leaves out, links the dependency folders, and runs the package in its
place there, so what the package reads from the workspace's root, such as a
`tsconfig.base.json` its `tsconfig.json` extends, is there. Members of one
workspace share one copy per worker. A workspace root above the project root
is not copied, and nothing outside a workspace is followed.

When a file's baseline fails in a worker's copy, `mutation run` and
`mutation sample` run the same command once more, unmutated, in the project's
own folder, as coverage runs there. When it passes there, the problem is
still `mutation.baseline-failed`, exit 1, its message saying the tests pass
in the project but fail in the worker's copy of the tree it held, and its
fix that the copy holds that tree alone and its tests read something outside
it. When they fail there too, the message is the usual one.

A TypeScript file whose project is run by neither Vitest nor Jest runs its
project's test script. Vitest runs a project whose `package.json` declares
vitest, or one where a vitest resolves from its `node_modules/.bin` and its
test script names `vitest`, `vp` or `vite`, or a `vite.config.*` or
`vitest.config.*` sits beside its `package.json`, so Vitest that comes in
through a toolchain package counts and a bin hoisted from another
dependency does not; Jest runs one that declares jest. Nothing can narrow
that script, `<pm> run test`, to the tests that reach the file: its own
tests are the whole suite. `mutation run`
records their outcomes with scope `all-tests` and the whole suite's evidence
(`suite_evidence`), as a `--test-command` outcome is recorded, so a change to
any test beneath its `package.json` makes them stale in `mutation check`,
`run`, `sample` and the graph, and `mutation sample` re-runs them with that
script. Coverage is measured from the same script, by the project's
`coverage` script or `c8`. An outcome of such a file recorded before as
`own`, with no whole-suite evidence, reads stale once and is re-run. Projects
with Vitest or Jest keep their related tests and scope `own`. Their coverage
also measures the Node processes those tests start, through
`NODE_V8_COVERAGE` and the project's c8 (`c8 report`), as `"integration"`,
leaving out what the runner's own processes, which loaded Vitest's or
Jest's modules, wrote; listed tests (`mutation.tests`) reach them the same
way, one test at a time or as a harness splits them with
`NODE_V8_COVERAGE=<ITOS_CC_TEST_COVERDIR>/<test ID>`. Without c8 the run
logs it and goes on without them, and `--fail-uncovered` reports
`coverage.tool-missing` where TypeScript has mutants to judge.

Kotlin coverage measures the JVMs a test starts when its harness opts in.
While the coverage command runs, `ITOS_CC_JACOCO_AGENT` names the JaCoCo
runtime agent jar (`org.jacoco.agent-<version>-runtime.jar`) in the Gradle
or Maven cache, at the version the build files name where they name one,
else the newest, and `ITOS_CC_JACOCO_DESTDIR` a directory of the run's own;
the harness starts each JVM with
`-javaagent:$ITOS_CC_JACOCO_AGENT=destfile=$ITOS_CC_JACOCO_DESTDIR/<name>.exec`,
a `<name>` of the JVM's own, or `append=true` added where JVMs share a file.
The `.exec` files written are merged and reported as XML by jacococli
(`org.jacoco.cli-<version>-nodeps.jar`, from the same caches, run with
`JAVA_HOME`'s `java`) against the module's class files
(`build/classes/kotlin/main` and `build/classes/java/main`, or
`target/classes`) and sources, as `"integration"` beside the JaCoCo or
Kover report; while the agent is named, Gradle's test task runs with
`--rerun`. Listed tests reach them the same way, one test at a time with an
`ITOS_CC_JACOCO_DESTDIR` of its own, or as a harness splits them by
writing each test's `.exec` files under `<ITOS_CC_TEST_COVERDIR>/<test ID>/`.
Neither jar is downloaded. Without the agent `ITOS_CC_JACOCO_AGENT` is unset
and the run logs it; `.exec` files with no jacococli to report them are a
log line, and `coverage.tool-missing` under `--fail-uncovered` where Kotlin
has mutants to judge. Kover's own agent is not named. Kotlin evidence
recorded before this (its producer without
`subprocesses=ITOS_CC_JACOCO_AGENT, jacococli report`) reads stale once.

Dart coverage is `flutter --suppress-analytics --no-version-check test
--no-pub --no-dds --coverage --coverage-path=<report>`, over the own tests'
files, or with no path for `--all-tests` and for `crap`, whose Dart run is
always the whole suite of `test/`. Its `flutter` is the SDK FVM links at
`.fvm/flutter_sdk` beside the nearest `.fvmrc`, else the one on `PATH`. It
measures only a Flutter app or package, whose `pubspec.yaml` depends on the
flutter SDK, once pub has resolved it (`.dart_tool/package_config.json`, or
its workspace root's for a pub workspace's member); a pure Dart package, one
not resolved, and a missing `flutter` are left unmeasured, the message
naming `--coverage-command`. Each pub workspace member is measured in its
own directory. Coverage is by line, a library marked
`// coverage:ignore-file` has nothing to cover, and the processes a test
starts are not measured: Dart has no `"integration"` coverage, and listed
tests (`mutation.tests`) reach no Dart line, as no collector measures it.

For Go outcomes recorded with `--all-tests` or `--test-command`, freshness
also depends on every `_test.go` file beneath the source's nearest `go.mod`
(including build-tagged tests and excluding nested modules) and on the files
matched by project-root `mutation.tests.support` globs. Feature files and
other custom-command inputs must be named by those globs. Check, sample, run,
and graph use the recorded scope's evidence; freshness checks run no tests or
list command. Older Go broad-scope outcomes without evidence are stale.
A Dart outcome recorded with `--all-tests` depends on the test files under
its package's `test/` alone, those `flutter test` runs with no path, and
one recorded with `--test-command` on every Dart test file beneath its
nearest `pubspec.yaml`.

A kill by listed tests (`mutation.tests`) is dated by the files its tests are
defined in and the `support` files, with no command run, and not by the tests
that import its file: a change among those stales the function only when it
also holds an own-scope outcome, a survivor or an uncovered mutant, and
`mutation run` then judges only those again, reusing the listed kills (a
timeout or an over-limit counts as a kill). A listed kill whose test has no
recorded file hash and no fingerprint keeps resting on the importing tests
too, so the `support` files are never its only date. The list command may
print a third tab-separated column, `<ID>\t<file>\t<fingerprint>`, opaque text
the lister computes from the test's own text; itos-cc never reads a test's
format. A kill by a test listed with one is dated by it instead: it holds while
the list prints the test with that fingerprint, whatever else its file holds and
whichever file names it, and is `mutation.stale`, naming the test, once the
fingerprint changed or the test is no longer listed. Check, run, sample and
except run the list command only when the file of such a test changed, at most
once a command; one that fails is `tests.list-failed`, exit 1. A test listed
without a fingerprint, and an outcome recorded before itos-cc read them, is
dated by its file; `support` files stale every listed kill either way.

Strict Go, Python, TypeScript, Kotlin and Dart coverage is independent of mutant
outcomes. `--fail-uncovered` requires a complete per-function executable
inventory from a successful built-in or listed measurement, including
functions with no mutation sites: Go's positive-weight cover-profile blocks,
the lines a Python, TypeScript or Dart LCOV report names executable (DA lines),
and the lines of a Kotlin JaCoCo or Kover XML report with instructions, a
line uncovered only when none of them ran (a partly executed line, such as
one a test took one branch of, is covered; branches are out of scope), each
from a function's body's first statement to its end, a Python `def` line or
a TypeScript arrow function's declaration running at import, and a Kotlin
declaration's line holding its default-argument bridge. A Dart function's
lines run from its signature line, which the Dart VM counts at each call,
and a Dart line any of whose code ran is covered. A one-line Kotlin
expression body shares that line. Python's report is coverage.py's;
TypeScript's is Vitest's v8 or Jest's over the related tests, or with
`--all-tests` the whole suite, or c8's over the test script of a package
with neither; Kotlin's is Gradle's `jacocoTestReport` or `koverXmlReport`,
or Maven's `jacoco:report`, over the module's test classes that reach the
file, or with `--all-tests` or `--test-command` the module's whole suite;
Dart's is `flutter test --coverage`'s over the test files under the
package's `test/` that reach the file, or with `--all-tests` or
`--test-command` the whole suite of `test/`. A
Python file no test reaches has its executable lines listed by coverage.py's
own analysis (`Coverage.analysis2`), a TypeScript file no test loads by
Vitest's v8 provider run with no test (`vitest run <no test>
--passWithNoTests --coverage.include=<file>`), and a Kotlin file no test
reaches by the report of another file of its module, which lists every
class the module compiled, all of them uncovered; under Jest or c8 such a
file has no evidence, nor does a Kotlin file whose module measured no other
file (no report task writes a report without running a test), nor a Dart
library no test loads, which `flutter test`'s report never names, nor a
file a project's own `coverage` script measured, and is
`mutation.coverage-missing`. Empty or comment-only Go and Kotlin bodies,
and Python or TypeScript functions with no executable body line, have no
executable obligation. Cached checks report missing or stale coverage
evidence without running tests, coverage or list commands. A strict run
measures again only the languages whose evidence is not fresh or whose
mutants need coverage. Strict runs reject `--coverage-report`,
`--use-existing-coverage`, and `--coverage-command` in every language;
non-strict behavior is unchanged, and a counted (`--count`) run still
proves only Go's lines strictly, not Python, TypeScript, Kotlin or Dart
lines. Kotlin evidence is bound to its whole build root, every module
of a multi-module build included. Dart evidence is bound to its package,
the nearest `pubspec.yaml`, or for a pub workspace's member to the whole
workspace: every Dart file there, generated Dart included, each
`pubspec.yaml`, `pubspec_overrides.yaml` and `dart_test.yaml`, the root's
`pubspec.lock`, the nearest `.fvmrc`, the `flutter` that runs,
`FLUTTER_ROOT` and `PUB_CACHE`, leaving out hidden folders, `build/`,
`coverage/`, `integration_test/` and `test_driver/` beside a
`pubspec.yaml`, a Flutter app's platform folders and a nested package the
workspace does not list. Python, TypeScript, Kotlin and Dart evidence is
written under a
unit's `line_coverage` key, with its `"language"`; Go evidence stays under
`go_coverage`. Go evidence is bound to its nearest module's `go.mod` and
`go.sum` and the Go files git tracks beneath it that a build can include,
read from the working tree: never under `testdata`, in a directory or file
whose name starts with `_` or `.`, nor in a nested module; outside a git
repository, the Go files those rules take from the disk. It is bound to the
Go environment as `go env` resolves it in the module, `GOTOOLCHAIN` aside,
which only says how the version was chosen, and to the project toolchain's
`GOVERSION`, so evidence recorded in one checkout stays fresh in another,
CI's included. Strict run and check refuse active Go
workspaces and local replacements outside the inventoried nearest module,
including excluded nested modules; `GOWORK=off` remains supported. They
refuse, likewise, npm and yarn workspaces, `workspace:` dependencies,
`file:` or `link:` dependencies and pnpm overrides to one outside the package
root, a pnpm workspace's own root package and a pnpm member that depends on
another member by its name (a plain or `catalog:` version); a
`pnpm-workspace.yaml` with no `packages`, or null or empty ones, is
settings, and a pnpm member with no local dependency is admitted, its
evidence bound too to its workspace root's config files, `.npmrc`,
`.pnpmfile.cjs` and `patchedDependencies` patch files; Python path
or editable dependencies outside the build root (`pyproject.toml` path
keys and `file:` URLs, `requirements*.txt` editable or path lines, `.pth`
paths and `__editable__` finder mappings in the root's `.venv` or `venv`);
Gradle `includeBuild`, Maven `<module>`, `<subproject>` and parent
`<relativePath>` outside the build root; and pub path dependencies and
overrides (`dependencies`, `dev_dependencies` and `dependency_overrides`
in a `pubspec.yaml` or a `pubspec_overrides.yaml` beside one) outside the
Dart root, the package or a member's whole workspace. Git and hosted pub
dependencies are pinned by `pubspec.lock` and admitted. A target inside the root counts
only through no symbolic link and no nested package or build root. They
are read lexically from files beneath the project root; nothing outside it
is read, listed or followed. The refusal
is reported as `mutation.coverage-unsupported`, and cached evidence cannot
bypass it. Each cached inventory is bound to its file, function identity, and
function hash; missing, incomplete, legacy, or misattributed inventories do
not prove coverage.

`mutation except --renew`, run after `mutation run`, renews each entry of
`itos-cc.yaml` that no longer holds when its mutant is unchanged: its
function, found by `namespace#name` in its file, else by its name alone
there when exactly one function has it, or, its file gone, by name and hash
in exactly one other source, has fresh results holding exactly one site of
the entry's `original` and `replacement` on a line whose text, less all
space, is its `line_text`, or of several such the one at its place; an entry
without `line_text` matches the site at its `line_in_function` and
`column`, else the function's only site of that change. That mutant must be
recorded survived. The renewed entry takes the function's current `hash`,
`function`, `file`, `line_in_function`, `column` and `line_text`, and keeps
its `reason` and comments; only the renewed entries' lines change. `--json` lists
`renewed`, each with `old_hash`, `new_hash`, the site and `match` (`line`,
`place` or `only`), and `not_renewed`, each entry left stale with `why`,
also reported as `mutation.exception-stale`, exit 1. `--renew` with a site
is `args.unexpected`, and with `--reason` `flags.conflict`, exit 80.
`mutation run` and `mutation check` call an entry `changed` when its
function, by `namespace#name`, still has a site of its change, and `gone`
when the function or every such site is gone, as `--renew` does.

`mutation except <file>:<line>:<column>` re-points the one stale entry that
names the site's file, its function by `namespace#name`, and its `original`
and `replacement`, when no entry is for the site itself: the entry is
replaced, `"replaced": true`, and keeps its `reason` unless `--reason`
gives another. Without `--reason` and nothing to re-point, it is
`flags.value-missing`, exit 80. An entry that holds, one stale as `killed`,
and one whose file moved are never re-pointed; with several, the site is
excepted as a new entry.

`mutation except <identity>` takes the identity a counted run reports for a
survivor, `<file>:<namespace#name>@<start>:<end>:<hash>:<offset>:<original>><replacement>`,
told apart from a site by its `@<start>:<end>:<hash>:<offset>:` part, and
excepts it with no mutation run and no snapshot. Its file is read from the
project root, whatever the working directory; its function is the one of its
name, and namespace in Go, Kotlin and Dart, whose hash is the identity's,
its lines choosing among several, so a function that only moved is still
found; and its site is the one whose whole key, the offset and the change, is
the identity's, never split at `>`. The entry written, its output, `--json`
and exits are the site's except's, field for field. A missing or stale
snapshot of the function is ignored; a fresh one that records the mutant
killed, timed out, over the limit or uncovered refuses it as
`exception.no-survivor`, at its site. A function of that name with another
hash is `exception.identity-stale` with why `changed` and the `hash` now, and
a file, function or site that is gone is why `gone`, exit 80, the fix asking
for a report of the code as it is now. One identity is excepted at a time,
with its own `--reason`, as a site is. A counted run's `mutation.survived`
problem names the survivor's identity in its message, which a CI log prints,
and its fix spells out `itos-cc mutation except '<identity>' --reason …`.

`mutation except --prune`, run after `mutation run`, removes each entry of
`itos-cc.yaml` that nothing can mend, judging every entry as `mutation
check` of the whole project does: one stale as `gone`, as `killed` (its
function's fresh results record its mutant killed) or as `excluded`. An
entry stale as `changed` or `moved` is kept, for `--renew` or an edit to
mend. Only the removed entries' lines change, with the comment lines
directly above each. `--json` lists `pruned`, each entry with `why` and,
when its site is found, `line`; it exits 0 once it has written. `--prune`
with a site is `args.unexpected`, and with `--reason` or `--renew`
`flags.conflict`, exit 80. The fix `mutation run` and `mutation check` give
an entry stale as `gone`, `killed` or `excluded` names `mutation except
--prune`.

`mutation except` refuses a site whose mutant is recorded uncovered as
`exception.no-survivor`, on purpose: an exception covers survivors only.
Its message says so, and its fix names a test that runs the site's line, or
routing the branch through code a test already reaches, such as a helper the
tests call, then `mutation run` again. A killed site, or a function with
stale or missing results, keeps the fix that asks for a site `mutation run`
lists as survived.

A function's results are keyed by its hash, which leaves its comments out,
in every language, with the space and line breaks only a comment needed, and
its sites' offsets are taken in that comment-free text. A change to a
function's comments alone keeps its results fresh, and `--since` judges a
function the range touched only when its hash changed too: one the range
touched only in its comments is not judged, by `mutation run`, `check` or
`sample`, and none of its lines counts for `--fail-uncovered=lines`. A Python
docstring is code, not a comment.

`--fail-uncovered` takes an optional value: alone it is
`--fail-uncovered=functions`, the whole judged functions above.
`--fail-uncovered=lines`, which needs `--since`, judges only where the range
changed, in `mutation run`, counted runs, `--fail-fast` runs and
`mutation check` alike: a Go block or a Python, TypeScript, Kotlin or Dart
line counts when it overlaps a line the commits since the ref added or changed,
the new side of a hunk of `git diff --unified=0 <ref>...HEAD`, a renamed
file's under its new path; a pure deletion changes no line. An uncovered
mutant fails (`mutation.uncovered`) only on such a line. A judged function
with such a line still needs complete fresh evidence
(`mutation.coverage-missing`, `mutation.coverage-stale`,
`mutation.coverage-unsupported`), its changed lines being unjudgeable
without it; a function with none, such as one the range only deleted lines
from, is judged for survivors and exceptions alone. Survivors and
exceptions are judged as before. Evidence is measured and recorded for whole
functions either way, so a later `--fail-uncovered` check of the same
snapshot judges them whole. `=lines` without `--since` is `flags.conflict`,
exit 80, and any other value is `flags.value-invalid`.

`mutation run --count` runs on Linux and macOS, where every command it starts
runs in a process group it owns, stopped and joined before the run returns.
A command that detaches from its process group or session is not followed.
On Windows it fails with `count.platform`, exit 82, before launching any
command; native Windows support is #29.

`mutation run --count` judges the files of the `HEAD` commit, never the
working tree, so a change not yet committed goes unjudged. When one does,
it warns on stderr, a line `itos-cc: warning: this run judges commit <id>, …`
and then a line `itos-cc: unjudged uncommitted change: <path>` for each
source, test or `itos-cc.yaml` whose working tree differs from that commit:
staged, unstaged, or untracked and not ignored. A change to any other file,
a README say, is no input of the run and is not named. Its `--json`
sampling object lists those paths as `"uncommitted"`, slash paths from the
repository root, sorted, and an empty list when there are none. The
warning changes neither the judgment nor the exit code (#41).

`mutation run --fail-fast` stops a complete run at the first actionable final
judgment it observes, and reports it under the rule and exit code a run
without it would. Its `--json` object stays one object and adds keys only:
`"stop": {"stopped", "rule", "subject"}`, where `subject` holds the
failure's problem subject keys; `"work": {"completed", "cancelled",
"unattempted", "blocked"}`, disjoint counts of the selected mutants; and in
each file `"state"` (`completed`, `stopped`, `unattempted` or `blocked`),
`"work"`, `"ran"` and `"reused"` counting what the run ran and reused of
the file in every state, blocked included (its completed work less any
measured uncovered), `"cache"` (`complete` when its snapshot, as the run leaves it,
holds a valid result for every mutant of its judged functions, else
`incomplete`, apart from the run's own work in `"state"`), `"baseline"` as
it ran, `not-run` when it never did, and every mutant of its judged
functions with its `"state"` (`completed`, `cancelled`, `unattempted` or
`blocked`), `"outcome"` and `"scope"` only when completed. A file the stop
cut short gets no summary comment; its snapshot keeps the judgments
completed or reused, with their original scope and freshness evidence, and
no outcome for an undecided mutant (ADR-0022). Like `--count`, it runs on Linux and macOS, where each command it
starts runs in an owned process group, and every one is stopped and joined
within one shared five-second deadline from the stop; on Windows it fails
with `fail-fast.platform`, exit 82, before launching any command (#29).
`--fail-fast` with `--count` is `flags.conflict`.

A snapshot under `.metrics/mutate/` whose source file is gone, nothing at
its path in the working tree, is an orphan. Its source path is its own path
under `.metrics/mutate/` less `.json`, taken as the file system names it,
so a name with a space or, off Windows, a backslash maps back to itself,
unless the snapshot is a file's outside the project root: its `"file"` is
then that file's absolute path, in the platform's form (a drive letter or a
share on Windows), the snapshot's name being that path under
`.metrics/mutate/`, and the snapshot is judged by that file, so it is no
orphan while the file exists, and one gone is an orphan at that path,
outside the root, which only paths that cover it reach;
only regular files ending in `.json` there are snapshots, and nothing
outside `.metrics/mutate/` is ever removed. A snapshot whose file git shows
renamed to a path where something is, by `git diff -M REF...HEAD` with
`--since REF` and by `git diff -M HEAD` always, is no orphan, so the
`--since` run over the new path still moves it. The working tree's diff
sees a rename only once the new file is tracked: `git mv` stages it, and a
plain `mv` with nothing staged is a deletion and an untracked file, whose
old snapshot is an orphan; stage a move (`git mv`, or `git add`) before a
run. A file deleted, renamed to a path where nothing is, or never tracked
is an orphan. Outside a git repository, in one with no commit, or when git
fails, no orphan can be judged: none is named or removed. The snapshot of
a source that `itos-cc.yaml`'s `exclude` leaves out is an orphan too,
though its file is there. An orphan is not a problem, as
problems mean `"ok": false`: `mutation check` names the orphans under its
paths in a key of its own, `"orphans": [{"file", "snapshot", "why"}]`, there only
when it found one, so output with none keeps the keys it had, `file` from
the working directory as in `"files"`, `snapshot` from the project root,
slash-separated, and `why` `gone` or `excluded`, a key added to each entry,
and its exit code stays what its functions make it.
`mutation run` removes the orphans under its paths (with none, the working
directory) once it has written what it judged, and lists them under the
same key; a `--count` run writes nothing and removes nothing.

A Go file only another system builds, its name's `_GOOS` or `_GOARCH`
suffix or its `//go:build` line ruling out the `GOOS` and `GOARCH` that `go
env` reports whatever its other tags, is no problem either: `mutation run`,
`check` and `sample` leave it out of `"files"`, list it in a key of its own,
`"not_built": [{"file", "platform"}]`, there only when there is one, with
`file` as in `"files"` and `platform` as `go env` names it, `linux/amd64`,
say `itos-cc: not built on <platform>, so <file> is not judged here` on
stderr, and fail nothing for it, `--fail-uncovered` included; a `--count`
run lists it the same way, `file` from the repository's top as its
`"selected"` name files. Its snapshot is no orphan and is neither rewritten
nor judged here. A file only a tag of the project's own leaves out is
judged as before.

A mutant a fresh entry records at a site its unchanged function no longer
holds, as when a newer itos-cc removes a mutation operator, is left out of
`mutation check`'s verdict and counts, and the next `mutation run` drops it
from the entry. Leaving it out is a note, not a problem: `mutation check`
prints `left out <file>:<line> in <namespace#name>: <n> recorded mutants at
sites gone, which the next mutation run drops` under the function's file,
and in `--json` gives that function's `"functions"` entry `"left_out": <n>`,
there only when n is above zero, so output without one keeps its keys. No
verdict and no exit code changes for it.

| Rule | Topic | itos-cc |
| ---- | ----- | ------- |
| 1 | Short lowercase program name | Follows. |
| 2 | Lowercase subcommands with dashes | Follows. |
| 3, 4 | Groups are singular nouns, actions imperative verbs | Follows. One group, `mutation`, a singular noun whose actions are verbs: `mutation run`, `mutation list`, `mutation check`, `mutation sample`, `mutation except`. Every other command is named for what it measures (`crap`, `dry`, `scrap`, `units`) or does (`serve`). |
| 5 | No two commands with similar names | Does not follow: `crap` and `scrap`. |
| 6 | No everyday verb that points at another command | Follows. |
| 7, 8 | No implicit default subcommand, no abbreviations | Follows: `itos-cc` alone and `itos-cc mutation` alone print help; an unknown command names the one meant. |
| 9 | Help everywhere, on stdout, exit 0 | Follows: `itos-cc`, `--help`, `help <command>`, `<command> --help`, `-h` in any position; for the group, `mutation`, `mutation -h`, `help mutation run`, `mutation run --help`. |
| 10 | Help gives the `--json` shape and exit codes | Follows, with each command's problem rules. |
| 11 | `--version` and `version` print `itos-cc <version>` first | Follows: the version has no leading v, so a release's and a `go install`'s of one version say it alike (#19). |
| 12 | Unknown command exits 2 and names a guess | Follows with decision 36's 81, for a group's subcommands too: `mutation nosuch` exits 81 and names `run`, `list`, `check`, `sample`, and `except`. |
| 13 | A group with no subcommand names them | Follows: `itos-cc mutation` prints the group's help, naming `run`, `list`, `check`, `sample`, and `except`, on stdout with exit 0, as `itos-cc` alone does; with `--json` it is `command.missing`, exit 80. |
| 14 | Help ends with examples and the issues address | Follows. |
| 15, 16 | Long flags, `-h` the only short one; standard names | Follows. |
| 17 | A flag means the same in every command | Does not follow: `--threshold`. |
| 18 | A flag changes an action, never selects another | Follows. |
| 19–22 | `--flag=value` and `--flag value`; bad, missing, repeated, or switch values exit 2; a value is never a flag; no optional values | Follows, exiting decision 36's 80: `units`, `dry`, `scrap`, `crap`, `version` and the `mutation` group read their flags through kong, held to these rules by `internal/cli` (its `Flags` and `Parse`), and `serve`, on its legacy handler, from its declared spec (`cmd/itos-cc/cli.go`), in the same words. |
| 23 | `--` ends the options; `-` is stdin or stdout | Follows for `--`; no command reads stdin, and the one file a command writes outside `.metrics/` is `itos-cc.yaml`, which `mutation except` edits in place. |
| 24 | Flags in any position | Follows. |
| 25 | Each argument checked; a bad one exits 2 | Follows with decision 36's 80: a path that is no file and no fragment of one exits 80. |
| 26 | Main output on stdout, the rest on stderr | Follows. |
| 27 | JSON only with `--json`, `"schema": 1`, keys only added | Follows. `serve`'s HTTP API is JSON by design and not stdout. |
| 28 | A stderr hint when plain output looks like data | No plain output looks like data. |
| 29 | `--json` prints the object for every failure | Follows, usage errors and panics included. |
| 30 | No colour; `NO_COLOR` passed on | Follows: itos-cc prints no colour, and the tests it runs inherit the environment. |
| 31 | The exit code comes from the kind; unclassified is 70 | Follows with decision 36's codes: unclassified is 100. |
| 32 | Error lines start `itos-cc:` and say what to do | Partly: an internal error prints Go's error text. |
| 33 | Help and code agree on exit codes | Follows; tests check every command's help. |
| 34, 35 | `ITOS_CC_` variables; flag, then environment, then config | itos-cc reads no variables of its own; it sets `ITOS_CC_TEST_COVERDIR` for a project's listed tests, coverage.py's `COVERAGE_PROCESS_START` for the Python processes tests start, Node's `NODE_V8_COVERAGE` for the Node processes Vitest and Jest tests start, and `ITOS_CC_JACOCO_AGENT` and `ITOS_CC_JACOCO_DESTDIR` for the JVMs a Kotlin harness starts with the JaCoCo agent. Its project settings, `mutation.exceptions` and `mutation.tests` in `itos-cc.yaml`, have no flag or variable to set them instead: an exception belongs with the code it excuses, and the tests' commands with the project's harness, so they are read only from the file. |
| 36 | No network check in CI | Follows: itos-cc never touches the network, nor downloads a tool. Every `flutter` command it starts carries `--suppress-analytics --no-version-check`, `flutter test` runs with `--no-pub`, and a worker's copy or a counted run's export of a Dart package is resolved with `flutter pub get --offline`. |
| 37 | Questions only on a terminal, with a flag each | itos-cc asks nothing. |
| 38 | Project settings in a file under version control | Follows with `itos-cc.yaml` at the project root, the git top level of the working directory, or the working directory outside a git repository. Its settings are `exclude`, doublestar globs, slash-separated and from the project root, of the sources (never tests) that `crap`, `dry`, `units` and `mutation run`, `list`, `check`, `sample` and `run --count` leave out however they were chosen, which a person writes; `mutation.exceptions`, the equivalent mutants excepted, each with its file, function, the function's hash, the site's `line_in_function` and `column`, `original`, `replacement`, `reason`, and the optional `line_text` (the site's line less its surrounding space, which entries written before it lack), which `mutation except` writes, from a site or the identity a counted run reports (its file read from the project root), `mutation except --renew` renews and `mutation except --prune` removes, changing only the lines of the entries they write and keeping every other byte of the file; `mutation.tests`, the commands that list the project's tests and run a selection of them (`list`, `run`, `ids_pattern`, `join`, `whole`, and `support`); and `mutation.copy`, doublestar globs from the project root of the paths a worker's copy leaves out, ignored by git, build output or an untracked nested checkout, that `mutation run` and `mutation sample` copy into it, a folder whole, never `.git`, `.dart_tool` or a linked folder. A file that cannot be read is `config.invalid`, exit 80, and so is an `exclude` or `mutation.copy` glob that is empty, absolute, holds `..` or a backslash, or is no pattern, its `key` naming it (`exclude[1]`, `mutation.copy[0]`), and a file holding an anchor, an alias or a merge key (`&x`, `*x`, `<<`) anywhere, for every command, its `key` naming the key whose value carries the first anchor or the merge key itself, so that `mutation except`, `--renew` and `--prune` never write through one, and, in document order with those, a tag written other than `!` and JSON's (`!!str`, `!!int`, `!!float`, `!!bool`, `!!null`, `!!seq`, `!!map`), its `key` naming the key whose value carries it, a list or a mapping as a key, its `key` naming the mapping holding it, none at the top level, and a second document after `---`, naming the file alone; after those and before any value is checked, so is a key the file does not list, at any depth, its `key` the path from the top (`mutation.exceptions[0].line_txt`), its fix "Did you mean `line_text`?" when a listed key at that place is within two edits, else the keys listed there (`exclude, mutation` at the top level); a key starting with exactly `x-`, an extension key as Docker Compose and OpenAPI allow, is accepted and ignored at any depth (`x-owner`, `mutation.x-note`), its value still held to the refusals of anchors, aliases, merge keys and tags, and kept byte for byte by `mutation except`, `--renew` and `--prune`, while `X-owner` and `x_owner` are refused as any key the file does not list. `crap`, `dry`, `units` and `mutation list` read the file too, from the version that has `exclude`; `units` and `dry` read it once their command line is parsed, so a file that cannot be read never breaks help or `version`, and `scrap`, which reads only tests, never reads it. `mutation run --count` reads the file the commit at `HEAD` holds. |
| 39–41 | Entry points for other programs | None. |
| 42, 43 | Breaking changes together in a major release, no compatibility code | Follows; before 1.0, in a minor release: `mutate` became `mutation run` in one with no alias. |

## Where itos-cc does not follow these rules yet

| Rule | What itos-cc does now | Example |
| ---- | --------------------- | ------- |
| 5 | Two commands with names a letter apart. | `crap`, `scrap` |
| 17 | `--threshold` is a CRAP score in `crap` and a similarity in `dry`. | `crap --threshold 30`, `dry --threshold 0.9` |
| 32 | An internal error's message is Go's error text. | a source file that cannot be read |
| Shell completion, go-template-itos's rule 41 | No `completion <shell>` command and no hidden `__complete`, so no shell completes itos-cc's commands and flags. | `itos-cc completion bash` exits 81 as an unknown command |
