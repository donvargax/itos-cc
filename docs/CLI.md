# CLI design

itos-cc follows the CLI rules of itos, its host, so a person or an agent meets
the same rules in both: [itos's docs/CLI.md](https://github.com/donvargax/itos/blob/main/docs/CLI.md).
This page names those rules by their numbers there, says whether itos-cc
follows each, and lists where it does not yet. Change this page in the same
commit as the command line.

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

### Exit codes

| Code | Meaning                                                                                |
| ---- | -------------------------------------------------------------------------------------- |
| 0    | Success.                                                                               |
| 1    | A check said no: a mutant survived, an uncovered mutant, executable Go coverage block or executable Python, TypeScript or Kotlin line with `--fail-uncovered`, missing or stale mutation/coverage evidence, an exception in `itos-cc.yaml` that no longer holds, a sampled mutant whose outcome differs from its cached one, a function is over `--threshold`, tests that measure nothing, a list command of `mutation.tests` that fails. |
| 2    | A usage or config error: a bad flag, path, argument, or report, an `itos-cc.yaml` that cannot be read, a site to except that is no recorded survivor. |
| 3    | The environment lacks something: a tool, a report, a git repository.                   |
| 70   | An internal error that itos-cc could not classify, a panic included. Report it.        |
| 75   | A temporary failure: the same command can pass when run again unchanged.               |

A run with problems of several kinds exits with the first of 2, 70, 3, 75,
and 1 that it has.

### `--json`

Every command takes `--json` and then prints one object on stdout:

```json
{"schema": 1, "ok": true, "...": "the command's own keys"}
```

A later release only adds keys. A run that did not succeed, a usage error
included, prints the object too, with `"ok": false` and its problems:

```json
{"schema": 1, "ok": false, "problems": [
  {"rule": "mutation.survived", "message": "…", "fix": "…",
   "file": "src/a.go", "line": 12, "column": 9, "function": "example.com/m/a#Pos",
   "original": ">", "replacement": ">="}
]}
```

`rule` is a stable id; the other keys besides `message` and `fix` are the
problem's subject. Progress and test output go to stderr, never stdout:
each line itos-cc writes there itself begins `itos-cc:`, such as
`itos-cc: coverage <dir>$ <command>` before a coverage command runs, and the
output of the commands it runs passes as it is.

### Problem rules

| Rule                         | Exit | Command        | Subject keys                                                |
| ---------------------------- | ---- | -------------- | ----------------------------------------------------------- |
| `flags.unknown`              | 2    | every          | `flag`                                                      |
| `flags.value-missing`        | 2    | every          | `flag`                                                      |
| `flags.value-invalid`        | 2    | every          | `flag`, `value`                                             |
| `flags.switch-value`         | 2    | every          | `flag`                                                      |
| `flags.repeated`             | 2    | every          | `flag`                                                      |
| `flags.conflict`             | 2    | mutation run, mutation check, mutation sample, mutation except | `flag`                                        |
| `command.unknown`            | 2    | itos-cc, mutation | `command`                                                |
| `command.missing`            | 2    | itos-cc --json, mutation --json | none                                       |
| `args.unexpected`            | 2    | version, help, mutation except | `argument`                                  |
| `args.missing`               | 2    | mutation except | none                                                       |
| `args.invalid`               | 2    | mutation except | `argument`                                                 |
| `config.invalid`             | 2    | mutation run, mutation check, mutation sample, mutation except | `file`        |
| `exception.no-survivor`      | 2    | mutation except | `file`, `line`, `column`                                   |
| `paths.unmatched`            | 2    | crap, dry, mutation run, mutation list, mutation check, mutation sample, scrap, units | `argument` |
| `changed.no-git`             | 3    | crap, dry, mutation run, mutation list, mutation check, mutation sample, scrap, units | none     |
| `since.bad-ref`              | 2    | mutation run, mutation check, mutation sample | `ref`                                         |
| `since.no-git`               | 3    | mutation run, mutation check, mutation sample | none                                          |
| `sample.no-git`              | 3    | mutation sample, without `--seed` | none                                     |
| `count.platform`             | 3    | mutation run --count | `platform`                                            |
| `count.no-git`               | 3    | mutation run --count | none                                                  |
| `count.unsupported-scope`    | 2    | mutation run --count | none                                                  |
| `count.preparation-failed`   | 1, 3 (a tool missing) | mutation run --count | `stage`                             |
| `count.trial-failed`         | 1    | mutation run --count | `file`, `line`, `column`, `function`, `original`, `replacement`, `identity` |
| `count.interrupted`        | 75   | mutation run --count | none                                                  |
| `fail-fast.platform`         | 3    | mutation run --fail-fast | `platform`                                        |
| `coverage.command-needs-report` | 2 | crap, mutation run | none                                                    |
| `coverage.measured-nothing`  | 1    | crap --threshold | `dir`, `language`, or `report`                            |
| `coverage.tool-missing`      | 3    | crap --threshold, mutation run --fail-uncovered (a coverage tool, or the collector of the processes tests start, missing where its language has mutants to judge) | `dir`, `language`                                         |
| `coverage.no-report`         | 3    | crap --threshold | `dir`, `language`                                         |
| `coverage.report-unreadable` | 2    | crap --threshold | `report`                                                  |
| `crap.threshold`             | 1    | crap           | `file`, `line`, `function`, `crap`, `threshold`             |
| `mutation.survived`          | 1    | mutation run, mutation check | `file`, `line`, `column`, `function`, `original`, `replacement` |
| `mutation.uncovered`         | 1    | mutation run --fail-uncovered, mutation check --fail-uncovered | `file`, `line`, `column`, `function`, `original`, `replacement` |
| `mutation.uncovered-statement` | 1 | mutation run --fail-uncovered, mutation check --fail-uncovered | `file`, `function`, `line` |
| `mutation.coverage-missing`  | 1    | mutation run --fail-uncovered, mutation check --fail-uncovered | `file`, `function`, `line` |
| `mutation.coverage-stale`    | 1    | mutation check --fail-uncovered | `file`, `function`, `line` |
| `mutation.coverage-unsupported` | 1 | mutation run --fail-uncovered, mutation check --fail-uncovered | `file`, `function`, `line` |
| `mutation.exception-stale`   | 1    | mutation run, mutation check, mutation except --renew | `file`, `function`, `line` (none when the function or its file is gone, or with `--renew` when its site is not found), `column`, `original`, `replacement`, `why` (`killed`, `changed`, `gone`, `moved`; with `--renew` also `ambiguous`, `no-results`, `uncovered`), with `moved` `new_file` |
| `mutation.missing`           | 1    | mutation check | `file`, `line`, `function`                                  |
| `mutation.stale`             | 1    | mutation check | `file`, `line`, `function`                                  |
| `mutation.mismatch`          | 1    | mutation sample | `file`, `line`, `column`, `function`, `original`, `replacement`, `recorded`, `outcome`, `scope` |
| `mutation.baseline-failed`   | 1    | mutation run, mutation sample | `file`                                       |
| `tests.list-failed`          | 1    | mutation run   | `command`, `exit_code`                                      |
| `tests.selection-failed`     | 1    | mutation run, mutation sample | `ids`, `command`, `exit_code`                 |
| `serve.repo-unreadable`      | 2    | serve          | none                                                        |
| `serve.port-in-use`          | 75   | serve          | `port`                                                      |
| `internal`                   | 70   | every          | none                                                        |

## The rules

Rules 1 to 43 of itos's docs/CLI.md, as they apply to itos-cc.

A snapshot's `tests` are the test files that reach its file. In TypeScript,
Python and Kotlin those are the test files whose imports reach its module
transitively (Kotlin's same-package references included), as `vitest
related` and `jest --findRelatedTests` select them; in Go, the test files of
its package and of the packages that import it, one hop. A test reaches
through the test-support files it imports too, files the language counts as
test code that are not runnable tests (a Python helper under `tests/`, a
TypeScript helper under `__tests__/`, a Kotlin helper in `src/test`): it
reaches what they reach, transitively, and they are among the file's tests
themselves. A Python test pytest runs, and a `conftest.py`, also reaches
what every `conftest.py` in its directory and those above it, up to its
build root, reaches. A snapshot recorded when a TypeScript, Python or Kotlin
file's tests were only those importing it directly, or before tests reached
through support files, reads stale once if a test reaches the file through
another module or a support file, and is re-run as any stale result is.

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

A TypeScript file whose project has neither Vitest nor Jest installed runs
its project's test script, `<pm> run test`, which nothing can narrow to the
tests that reach the file: its own tests are the whole suite. `mutation run`
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

For Go outcomes recorded with `--all-tests` or `--test-command`, freshness
also depends on every `_test.go` file beneath the source's nearest `go.mod`
(including build-tagged tests and excluding nested modules) and on the files
matched by project-root `mutation.tests.support` globs. Feature files and
other custom-command inputs must be named by those globs. Check, sample, run,
and graph use the recorded scope's evidence; freshness checks run no tests or
list command. Older Go broad-scope outcomes without evidence are stale.

Strict Go, Python, TypeScript and Kotlin coverage is independent of mutant
outcomes. `--fail-uncovered` requires a complete per-function executable
inventory from a successful built-in or listed measurement, including
functions with no mutation sites: Go's positive-weight cover-profile blocks,
the lines a Python or TypeScript LCOV report names executable (DA lines),
and the lines of a Kotlin JaCoCo or Kover XML report with instructions, a
line uncovered only when none of them ran (a partly executed line, such as
one a test took one branch of, is covered; branches are out of scope), each
from a function's body's first statement to its end, a Python `def` line or
a TypeScript arrow function's declaration running at import, and a Kotlin
declaration's line holding its default-argument bridge. A one-line Kotlin
expression body shares that line. Python's report is coverage.py's;
TypeScript's is Vitest's v8 or Jest's over the related tests, or with
`--all-tests` the whole suite, or c8's over the test script of a package
with neither; Kotlin's is Gradle's `jacocoTestReport` or `koverXmlReport`,
or Maven's `jacoco:report`, over the module's test classes that reach the
file, or with `--all-tests` or `--test-command` the module's whole suite. A
Python file no test reaches has its executable lines listed by coverage.py's
own analysis (`Coverage.analysis2`), a TypeScript file no test loads by
Vitest's v8 provider run with no test (`vitest run <no test>
--passWithNoTests --coverage.include=<file>`), and a Kotlin file no test
reaches by the report of another file of its module, which lists every
class the module compiled, all of them uncovered; under Jest or c8 such a
file has no evidence, nor does a Kotlin file whose module measured no other
file (no report task writes a report without running a test), nor a file a
project's own `coverage` script measured, and is
`mutation.coverage-missing`. Empty or comment-only Go and Kotlin bodies,
and Python or TypeScript functions with no executable body line, have no
executable obligation. Cached checks report missing or stale coverage
evidence without running tests, coverage or list commands. A strict run
measures again only the languages whose evidence is not fresh or whose
mutants need coverage. Strict runs reject `--coverage-report`,
`--use-existing-coverage`, and `--coverage-command` in every language;
non-strict and counted (`--count`) Python, TypeScript and Kotlin behavior is
unchanged. Kotlin evidence is bound to its whole build root, every module
of a multi-module build included. Python, TypeScript and Kotlin evidence is
written under a
unit's `line_coverage` key, with its `"language"`; Go evidence stays under
`go_coverage`. Strict run and check refuse active Go
workspaces and local replacements outside the inventoried nearest module,
including excluded nested modules; `GOWORK=off` remains supported. They
refuse, likewise, npm, yarn and pnpm workspaces, `workspace:` dependencies
and `file:` or `link:` dependencies outside the package root; Python path
or editable dependencies outside the build root (`pyproject.toml` path
keys and `file:` URLs, `requirements*.txt` editable or path lines, `.pth`
paths and `__editable__` finder mappings in the root's `.venv` or `venv`);
and Gradle `includeBuild`, Maven `<module>`, `<subproject>` and parent
`<relativePath>` outside the build root. A target inside the root counts
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
its `reason` and comments; nothing else is written. `--json` lists
`renewed`, each with `old_hash`, `new_hash`, the site and `match` (`line`,
`place` or `only`), and `not_renewed`, each entry left stale with `why`,
also reported as `mutation.exception-stale`, exit 1. `--renew` with a site
is `args.unexpected`, and with `--reason` `flags.conflict`, exit 2.

`--fail-uncovered` takes an optional value: alone it is
`--fail-uncovered=functions`, the whole judged functions above.
`--fail-uncovered=lines`, which needs `--since`, judges only where the range
changed, in `mutation run`, counted runs, `--fail-fast` runs and
`mutation check` alike: a Go block or a Python, TypeScript or Kotlin line
counts when it overlaps a line the commits since the ref added or changed,
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
exit 2, and any other value is `flags.value-invalid`.

`mutation run --count` runs on Linux and macOS, where every command it starts
runs in a process group it owns, stopped and joined before the run returns.
A command that detaches from its process group or session is not followed.
On Windows it fails with `count.platform`, exit 3, before launching any
command; native Windows support is #29.

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
with `fail-fast.platform`, exit 3, before launching any command (#29).
`--fail-fast` with `--count` is `flags.conflict`.

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
| 11 | `--version` and `version` print `itos-cc <version>` first | Follows. |
| 12 | Unknown command exits 2 and names a guess | Follows, for a group's subcommands too: `mutation nosuch` exits 2 and names `run`, `list`, `check`, `sample`, and `except`. |
| 13 | A group with no subcommand names them | Follows: `itos-cc mutation` prints the group's help, naming `run`, `list`, `check`, `sample`, and `except`, on stdout with exit 0, as `itos-cc` alone does; with `--json` it is `command.missing`, exit 2. |
| 14 | Help ends with examples and the issues address | Follows. |
| 15, 16 | Long flags, `-h` the only short one; standard names | Follows. |
| 17 | A flag means the same in every command | Does not follow: `--threshold`. |
| 18 | A flag changes an action, never selects another | Follows. |
| 19–22 | `--flag=value` and `--flag value`; bad, missing, repeated, or switch values exit 2; a value is never a flag; no optional values | Follows: every command reads its flags from one declared spec (`cmd/itos-cc/cli.go`). |
| 23 | `--` ends the options; `-` is stdin or stdout | Follows for `--`; no command reads stdin, and the one file a command writes outside `.metrics/` is `itos-cc.yaml`, which `mutation except` edits in place. |
| 24 | Flags in any position | Follows. |
| 25 | Each argument checked; a bad one exits 2 | Follows: a path that is no file and no fragment of one exits 2. |
| 26 | Main output on stdout, the rest on stderr | Follows. |
| 27 | JSON only with `--json`, `"schema": 1`, keys only added | Follows. `serve`'s HTTP API is JSON by design and not stdout. |
| 28 | A stderr hint when plain output looks like data | No plain output looks like data. |
| 29 | `--json` prints the object for every failure | Follows, usage errors and panics included. |
| 30 | No colour; `NO_COLOR` passed on | Follows: itos-cc prints no colour, and the tests it runs inherit the environment. |
| 31 | The exit code comes from the kind; unclassified is 70 | Follows. |
| 32 | Error lines start `itos-cc:` and say what to do | Partly: an internal error prints Go's error text. |
| 33 | Help and code agree on exit codes | Follows; tests check every command's help. |
| 34, 35 | `ITOS_CC_` variables; flag, then environment, then config | itos-cc reads no variables of its own; it sets `ITOS_CC_TEST_COVERDIR` for a project's listed tests, coverage.py's `COVERAGE_PROCESS_START` for the Python processes tests start, Node's `NODE_V8_COVERAGE` for the Node processes Vitest and Jest tests start, and `ITOS_CC_JACOCO_AGENT` and `ITOS_CC_JACOCO_DESTDIR` for the JVMs a Kotlin harness starts with the JaCoCo agent. Its project settings, `mutation.exceptions` and `mutation.tests` in `itos-cc.yaml`, have no flag or variable to set them instead: an exception belongs with the code it excuses, and the tests' commands with the project's harness, so they are read only from the file. |
| 36 | No network check in CI | Follows: itos-cc never touches the network, nor downloads a tool. |
| 37 | Questions only on a terminal, with a flag each | itos-cc asks nothing. |
| 38 | Project settings in a file under version control | Follows with `itos-cc.yaml` at the project root, the git top level of the working directory, or the working directory outside a git repository. Its settings are `mutation.exceptions`, the equivalent mutants excepted, each with its file, function, the function's hash, the site's `line_in_function` and `column`, `original`, `replacement`, `reason`, and the optional `line_text` (the site's line less its surrounding space, which entries written before it lack), which `mutation except` writes, and `mutation except --renew` renews, keeping the file's other keys and comments; and `mutation.tests`, the commands that list the project's tests and run a selection of them (`list`, `run`, `ids_pattern`, `join`, `whole`, and `support`). A file that cannot be read is `config.invalid`, exit 2. |
| 39–41 | Entry points for other programs | None. |
| 42, 43 | Breaking changes together in a major release, no compatibility code | Follows; before 1.0, in a minor release: `mutate` became `mutation run` in one with no alias. |

## Where itos-cc does not follow these rules yet

| Rule | What itos-cc does now | Example |
| ---- | --------------------- | ------- |
| 5 | Two commands with names a letter apart. | `crap`, `scrap` |
| 17 | `--threshold` is a CRAP score in `crap` and a similarity in `dry`. | `crap --threshold 30`, `dry --threshold 0.9` |
| 32 | An internal error's message is Go's error text. | a source file that cannot be read |
