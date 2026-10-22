# itos-cc

Code-quality measurements for TypeScript (and JavaScript), Python, Kotlin,
Dart (Flutter apps and packages) and Go, in one binary, with output meant
for both people and coding agents.

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

Beside `LICENSE`, each archive carries `THIRD_PARTY_NOTICES`: the license and notice texts of
every Go module the binary compiles in, which `go run ./tools/bin/notices` writes.

Or build it with Go, which needs cgo and a C compiler (tree-sitter is C):

```bash
go install github.com/donvargax/itos-cc/cmd/itos-cc@latest
```

## Build

Parsing uses tree-sitter, which needs cgo and a C compiler:

```bash
go build -o itos-cc ./cmd/itos-cc
go test ./...                                     # scenarios in process alone
ITOS_CC_FEATURES_BINARY_DRIVER=1 go test ./...    # and through the built binary too
```

Locally the scenarios run in process against the fakes; the binary driver runs only with
`ITOS_CC_FEATURES_BINARY_DRIVER=1`, which CI and CD set (decision 52, see [CI](#ci)).

Releases are cut by CD (see [CI](#ci)): on a push to `main` whose tests pass on
Linux, macOS, and Windows in `.github/workflows/cd.yml`,
`.github/workflows/release.yml` computes the version from the commits since
the last `vX.Y.Z` tag and builds each platform on its own runner. To cross-compile Linux and Windows binaries locally instead, use
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

Source files that are test support, such as a package of fakes several
packages' tests share, are left out of what is measured and mutated by
`exclude` in `itos-cc.yaml`: a list of globs, slash-separated and from the
project root, in [doublestar](https://github.com/bmatcuk/doublestar)'s
syntax (`**`, `*`, `?`, `[…]`, `{a,b}`).

```yaml
exclude: ["**/porttest/**", "features/**"]
```

`crap`, `dry` (the whole project a change is checked against included),
`units` and `mutation run`, `list`, `check`, `sample` and `run --count` leave
those sources out however they were chosen: a path, a fragment or
`--changed` naming one selects nothing, so a gate's command stays the same.
A test is never excluded, and `scrap` and the graph see every file. Each
says on stderr how many sources it left out. The snapshot of a source newly
excluded is an orphan, which the next `mutation run` removes; an exception
naming one is stale, as `excluded`, and `mutation except` refuses its sites
as `exception.excluded`. `mutation run --count` reads the `itos-cc.yaml`
`HEAD` commits. A glob that is no pattern, empty, absolute, holding `..` or
a backslash is `config.invalid`, exit 80, its `key` naming it
(`exclude[1]`). An itos-cc release older than this one ignores `exclude`.

Some Dart is left out without `exclude`, in every command, tests included.
A Flutter app's platform folders (`android`, `ios`, `macos`, `linux`,
`windows` and `web` beside a `pubspec.yaml` that depends on the flutter SDK)
are never walked into, and generated Dart is never taken from a folder that
is walked: build_runner's and protoc's files (`.g.dart`, `.freezed.dart`,
`.mocks.dart`, `.gr.dart`, `.config.dart`, `.pb.dart` and the other `.pb*`
files), the plugin registrant, a file whose leading comments say `GENERATED
CODE - DO NOT MODIFY BY HAND`, and what `flutter gen-l10n` writes. `build/`
and `coverage/` beside a `pubspec.yaml` are build output. No setting brings
platform folders or generated Dart back yet: how one would is an open
question (q-80).

Scripts rely on three things only, as with itos ([docs/CLI.md](docs/CLI.md)):
the exit code, the `--json` object less its `message` and `fix` keys, and the
files itos-cc writes: those under `.metrics/`, and `itos-cc.yaml`. Plain
output is for people.

| Code | Meaning |
| --- | --- |
| 0 | Success. |
| 1 | A check said no: a mutant survived, a function is over `--threshold`. |
| 75 | A temporary failure: the same command may pass when run again. |
| 80 | A usage or config error: a bad flag, path, or report, or an `itos-cc.yaml` that cannot be read. |
| 81 | An unknown command or subcommand. |
| 82 | A requirement not met: a tool, a report, a git repository. |
| 100 | An internal error itos-cc could not classify; please report it. |
| 130 | Interrupted: SIGINT (Ctrl-C) or SIGTERM stopped it. On Linux and macOS it then ends by that signal, so a shell reads 130, or 143 for SIGTERM, and stops a loop over it. |

A run with problems of several kinds exits with the first of 130, 80 and 81,
100, 82, 75 and 1 that it has. Every command, `serve` and `mutation run`
included, stops on SIGINT or SIGTERM, says once that it was interrupted, and
reports the rule `interrupted`; a second interrupt ends it at once, unless
the process groups it owns are being killed, which it waits for. A process
group whose cleanup does not finish is `mutation.cleanup-failed` or
`coverage.cleanup-failed`, exit 100. Before the semantic exit codes (decision 36), a
usage error and an unknown command were 2, a requirement not met 3 and an
internal error 70.

`--json` prints one object, `{"schema": 1, "ok": true|false, …}`, and for a
failure, usage errors included, `"problems": [{"rule", "exit", "message", "fix", …}]`
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
(LCOV), coverage.py (LCOV), JaCoCo or Kover XML for Kotlin, and
`flutter test --no-pub --coverage` (LCOV) for Dart. Tools are
the project's own: Node tools from its `node_modules`, run with the package
manager it declares, coverage.py from its virtualenv, the JaCoCo its
`pom.xml` declares, and the Flutter SDK FVM links at `.fvm/flutter_sdk`
beside the nearest `.fvmrc`, else the `flutter` on `PATH`. One that is
missing is reported, with what to install, and never downloaded.

A TypeScript package's tests are run by Vitest when its `package.json`
declares vitest, or when a vitest resolves from its `node_modules/.bin` and
its test script names `vitest`, `vp` or `vite`, or a `vite.config.*` or
`vitest.config.*` sits beside its `package.json`: Vitest that comes in
through another package, such as a toolchain's, is never named there, and
npm and yarn put every transitive dependency's bin in `node_modules/.bin`,
so the bin alone does not count. A package that declares jest is run by
Jest; one run by neither is measured by c8 over its test script.

Dart is measured in a Flutter app or package, one whose `pubspec.yaml`
depends on the flutter SDK, once pub has resolved it
(`.dart_tool/package_config.json`, or its workspace root's for a pub
workspace's member): `--no-pub` keeps `flutter test` from resolving or
fetching anything. A pure Dart package, or one not yet resolved, is left
unmeasured and shows `N/A`, its message naming `--coverage-command`. Each
member of a pub workspace is measured in its own directory. Coverage is by
line only, and a library marked `// coverage:ignore-file` has nothing to
cover. The Vitest, Jest and coverage.py commands itos-cc
composes set the project's coverage floor to nothing (Vitest's
`test.coverage.thresholds`, its `autoUpdate` off, Jest's
`coverageThreshold`, coverage.py's `fail_under`), which a run of some
files' tests would miss: itos-cc measures, and the project's own test
script still enforces its floor. Bring your own with `--coverage-command` and
`--coverage-report`; several reports are combined, and code that more than
one report or Go test binary lists counts once, covered if any of them ran
it. With paths or `--changed`, Go and TypeScript coverage runs only the
tests that load those files, which measures them the same as the whole
suite does; `--all-tests` runs the whole suite anyway. Python, Kotlin and
Dart run their whole suite: Dart's is `flutter test` with no path, the
tests under the package's `test/`.

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

Python code that tests reach by starting another Python process (a
`subprocess.run([sys.executable, "cli.py"])`, the installed CLI) counts too.
coverage.py 7.13 and later install a `.pth` file that starts coverage.py in
every Python process whose environment names an rcfile in
`COVERAGE_PROCESS_START`. Before the coverage run, itos-cc writes one in a
directory of the run's own under `.metrics/coverage/`: the project's own
`[run]` settings and plugin options, read with coverage.py's reader from
whichever file it reads (`.coveragerc`, `setup.cfg`, `tox.ini`,
`pyproject.toml`, `COVERAGE_RCFILE`), with itos-cc's on top (branch
coverage, the build root as source, parallel data files in that
directory). The coverage command itself runs as before, with the project's
configuration untouched, so its data stays apart from the subprocesses':
afterwards those are combined (with the project's `[paths]`) into an LCOV
report read beside the in-process one, and the directory is removed. A
mutant whose line only a subprocess ran says `"coverage": ["integration"]`.
A project whose coverage.py configuration sets `[run] parallel = true`, or
`patch = subprocess`, which forces it, has coverage run write parallel data
files beside the data file itos-cc names instead of that file: itos-cc
combines them into it (with the project's `[paths]`) before the report, as
coverage.py 7.14 and later do when they report, so such a project is
measured whichever coverage.py it has, and none of them is left behind.
Under the project's own `patch = subprocess`, coverage.py hands the Python
processes a test starts coverage run's own configuration, which they read
before `COVERAGE_PROCESS_START`, so they write beside the in-process data,
where nothing tells their files apart: their lines count as
`"in-process"`, covered all the same.
A coverage.py that does not start in new processes (before 7.13, or
without its `.pth` file) is a log line, the run going on without
integration coverage, and `coverage.tool-missing` under `mutation run
--fail-uncovered` where Python has mutants to judge. A test that starts its
process with an environment of its own, without `COVERAGE_PROCESS_START`,
measures nothing there.

TypeScript code that Vitest or Jest tests reach by starting another Node
process (`execFileSync(process.execPath, ["src/cli.ts"])`, a CLI) counts
too. While the coverage command runs, `NODE_V8_COVERAGE` names a directory
of the run's own under `.metrics/coverage/`, where every Node process the
tests start writes the V8 coverage of what it ran; afterwards the
project's own c8 (`node_modules/.bin/c8`, never fetched) reports it as LCOV
(`c8 report`, with the project's c8 configuration and the source maps Node
records), read beside the runner's own report, and the directory is
removed. The runner's own processes write there too, as they inherit the
variable: Vitest's main process does, and with `pool: threads` its worker
threads, which ran the in-process tests. A file written by a process that
loaded Vitest's or Jest's own modules is left out, so `"integration"` is
only what child processes ran. In a file the runner's report names,
integration data covers its lines and adds none: c8 names other lines,
such as closing braces, than Vitest's v8 provider does. A project without
c8 logs that integration coverage needs it and goes on, and is
`coverage.tool-missing` under `mutation run --fail-uncovered` where
TypeScript has mutants to judge. A project measured by c8 over its test
script, run by neither Vitest nor Jest, already measures the processes its
tests start, as its own.

Kotlin code that tests reach by starting another JVM (`ProcessBuilder`
running `java -cp … board.MainKt`, a server) counts when the harness opts
in, as a Go one builds its binary with `-cover`. While the coverage command
runs, `ITOS_CC_JACOCO_AGENT` names the JaCoCo runtime agent jar
(`org.jacoco.agent-<version>-runtime.jar`) in the Gradle cache
(`GRADLE_USER_HOME` or `~/.gradle`) or Maven's (`~/.m2/repository`), the
build tool's own first, at the version the build files name (the jacoco
`toolVersion`, the `jacoco-maven-plugin` version) where they name one, else
the newest; and `ITOS_CC_JACOCO_DESTDIR` names a directory of the run's own
under `.metrics/coverage/`. The harness starts the JVMs its tests run with

```
-javaagent:$ITOS_CC_JACOCO_AGENT=destfile=$ITOS_CC_JACOCO_DESTDIR/<name>.exec
```

where `<name>` is the JVM's own (a test name and a random suffix), or adds
`,append=true` when JVMs share a file. In Kotlin:

```kotlin
val agent = System.getenv("ITOS_CC_JACOCO_AGENT")
val dest = System.getenv("ITOS_CC_JACOCO_DESTDIR")
if (!agent.isNullOrEmpty() && !dest.isNullOrEmpty()) {
    command += "-javaagent:$agent=destfile=$dest/$name-${UUID.randomUUID()}.exec"
}
```

Afterwards, if any `.exec` file was written there, jacococli
(`org.jacoco.cli-<version>-nodeps.jar`, from the same caches, the agent's
version first, run with `JAVA_HOME`'s `java`, else the one on `PATH`)
merges them into an XML report against the module's class files
(`build/classes/kotlin/main` and `build/classes/java/main`, or Maven's
`target/classes`) and sources, read beside the JaCoCo or Kover report, and
the directory is removed. While the agent is named, Gradle's test task runs
with `--rerun` (Gradle 7.6 and later), since a task Gradle deems up to date
would start no JVM. Nothing is downloaded, and Gradle's jacoco plugin
fetches neither jar (Maven's jacoco-maven-plugin fetches the agent), so a
build resolves them once, for instance with a configuration of its own:

```kotlin
val itosJacoco by configurations.creating { isTransitive = false }
dependencies {
    itosJacoco("org.jacoco:org.jacoco.agent:0.8.15:runtime")
    itosJacoco("org.jacoco:org.jacoco.cli:0.8.15:nodeps")
}
tasks.register("jacocoJars") {
    val jars: FileCollection = itosJacoco
    inputs.files(jars)
    doLast { jars.files.forEach { println(it) } }
}
```

and `gradle jacocoJars` before itos-cc runs. Without the agent,
`ITOS_CC_JACOCO_AGENT` is unset, which a harness can tell, its JVMs go
unmeasured as before, and the run logs it. `.exec` files with no jacococli
to report them are a log line, the run going on without integration
coverage, and `coverage.tool-missing` under `mutation run
--fail-uncovered` where Kotlin has mutants to judge. A JVM started without
the agent measures nothing. Kover measures with an agent of its own, which
itos-cc does not name: a Kover project's harness uses JaCoCo's agent the
same way, its report read beside Kover's.

Dart has no integration coverage: what the processes a Flutter test starts
run counts for nothing, and the tests under `integration_test/` and
`test_driver/`, which need a device, never run.

A build root whose coverage could not be measured (tests that do not
compile, a missing tool, no report) shows `N/A`, not 0%, and is named on
stderr. With `--threshold`, a function above it exits 1, and so does a
coverage run that measured nothing, such as tests that do not compile; a
missing tool or report exits 82. A broken test setup never passes the gate.
A file no test loads is untested, not unmeasured: when the coverage run for
its language succeeded and wrote its report, its functions show 0%, even
when it is scored alone and the run found no test for it.

A function with branches in the report is scored by the share of branches it
took (LCOV `BRDA` blocks with two or more arms, JaCoCo branch counters,
coverage.py with `--branch`); one without is scored by its lines. Go
profiles have no branches, so Go is scored by statements. c8's LCOV lists V8
blocks rather than branches, so a project run by neither Vitest nor Jest falls
back to lines. Dart is scored by its lines: `flutter test` runs without
`--branch-coverage`, whose blocks are not decisions. Only the current Vitest major (5) is supported; older ones
are reported and left unmeasured. Python and TypeScript coverage is measured from the first line
of the body, because loading a module executes every `def` and
`export const f = …` line. A Dart function's is measured from its
signature line, which the Dart VM counts at each call.

### dry

Local names, field names, and literals are normalized away; called function
names, operators, and the tree's shape stay. Functions are compared by the
Jaccard similarity of their subtree fingerprints. The default threshold is
0.82.

### mutation

`mutation run` swaps operators (`<`/`<=`, `==`/`!=`, `&&`/`||`, `+`/`-`, …),
deletes `!` and unary `-`, and flips `true`/`false` and `0`/`1`, one at a
time, inside functions only, and runs the tests against each change.
In Dart it makes only the changes that compile whatever the operands'
types: the relational and equality swaps, `+`/`-`, `&&`/`||`, deleting `!`
and unary `-`, `true`/`false`, `0`/`1`, and `a ?? b` to `b`; not `*`/`/`, as
`int / int` is a `double`, nor `?.` to `.`. A Dart mutant that does not
compile fails its tests and counts as killed, as a Go or Kotlin one does.
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
  `mvn -q test -Dtest=<classes> -Dsurefire.failIfNoSpecifiedTests=false`,
  and in Dart the test files under the file's package's `test/` that reach
  it (named `*_test.dart`), as `flutter test --no-pub <files>`, which has no
  fail-fast flag. A Dart test under `integration_test/` or `test_driver/`
  never runs: they need a device.
  Coverage comes from the same tests, so a line only integration or
  end-to-end tests reach is uncovered, not a survivor; a Vitest or Jest
  project's `coverage` script measures mutation run only with `--all-tests`.
  A TypeScript project run by neither Vitest nor Jest (as [crap](#crap)
  decides) runs its test script, which cannot be narrowed: its outcomes are the whole suite's,
  recorded with scope `all-tests` and the whole suite's evidence, so any
  test change makes them stale, as a `--test-command` outcome. A Python,
  Kotlin or Dart file no such test reaches runs no test, and its
  mutants are uncovered (its `--json` `"baseline"` is `"not-run"`). A test reaches code through the helpers it imports
  and, in Python, the `conftest.py` files above it, and in Dart the nearest
  `flutter_test_config.dart` in its folder or above it, but a Python or Kotlin
  test that runs code only through a subprocess, the CLI or reflection
  reaches none of it, so that code's mutants are uncovered too: judge them
  with `--all-tests` or `--test-command`. Every command itos-cc
  composes runs with `PYTHONDONTWRITEBYTECODE=1`, and every pytest one with
  `-p no:cacheprovider`, so measuring and judging write no bytecode and no
  `.pytest_cache` into the project; a `--test-command` or
  `--coverage-command` runs exactly as given.
- **Parallel workers** in private copies of the project, so the real tree is
  never modified while tests run. The baseline runs inside a worker, which
  proves the copy works before any mutant does. Every run in a worker, the
  baseline's and each mutant's, starts from the copy as it was made: what a
  test run wrote, rewrote or removed there is put back before the next, so
  one run's leftovers never fail a later mutant. Build output, caches,
  `.dart_tool` and the linked dependencies stay as the runs leave them, and
  putting the copy back is not part of a run's time. A copy of a Dart
  package leaves out `build/` and `coverage/` beside its `pubspec.yaml` and
  a Flutter app's platform folders, keeps the generated Dart its tests
  compile against, and holds a pub workspace member's workspace root and
  members. It takes no `.dart_tool` from the project, whose resolution
  points into the project's tree: `flutter pub get --offline` makes the
  copy's own once, before its first run. A mutant times out after
  the baseline's time times `--timeout-factor` (10 by default), plus 5
  seconds for building the mutated code, and a timeout counts as killed.
  The copies live in an `itos-cc-mutate-` directory of the temporary
  directory (`itos-cc-fresh-` for a counted run's trials), removed when the
  run ends. A run killed before then leaves it, and the next run removes
  it: a run holds a lock on `owner.lock` in its directory while it runs,
  which the OS releases when the process dies, so a run never removes the
  copies of one still going, such as an agent's while a commit hook runs.
  A directory with no `owner.lock`, from an older itos-cc, is removed once
  nothing in it changed for three days.

`--all-tests` runs the whole suite for coverage and for every mutant, so
integration and end-to-end tests anywhere in the build root can kill
mutants. In Go, tests that run the built binary show up in coverage when
the binary is built with `go build -cover` under `GOCOVERDIR` (see
[crap](#crap)), and each mutant's run rebuilds it from the mutated copy, so
their kills count. `--json` says, for each mutant whose line is covered,
which coverage covered it: `"in-process"`, `"integration"`, or both. In
Python, the Python processes tests start show up in coverage too (see
[crap](#crap)), and in TypeScript, under Vitest or Jest with c8 installed,
the Node processes they start, and in Kotlin the JVMs a harness starts
with the JaCoCo agent itos-cc names. In Dart, `--all-tests` runs
`flutter test --no-pub` with no path, the tests under the package's `test/`
alone, and the processes a test starts show up in no coverage. Where such
tests do not show up in coverage, add `--no-coverage` to let them reach code nothing else
covers. It is slow; run
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
    list: go run ./features/list   # a test a line: its ID, a tab and its file, a tab and its fingerprint; each optional
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
test binaries, as it does not `GOCOVERDIR`); so does one that gives the
Python processes each test starts
`COVERAGE_FILE=<that directory>/<test ID>/.coverage`, which coverage.py
starts in from the rcfile itos-cc names in `COVERAGE_PROCESS_START`, as
above, and one that gives the Node processes each test starts
`NODE_V8_COVERAGE=<that directory>/<test ID>`, which the project's c8
reports, and one that starts the JVMs each test runs with the JaCoCo agent
writing under `<that directory>/<test ID>/`
(`destfile=$ITOS_CC_TEST_COVERDIR/<test ID>/<name>.exec`), which jacococli
reports. Dart has no such collector: what a listed test runs of Dart shows
up in no coverage, so no listed test reaches a Dart line. Otherwise each test runs alone, every process it starts writing
to a `GOCOVERDIR`, a coverage.py data file, a `NODE_V8_COVERAGE` and an
`ITOS_CC_JACOCO_DESTDIR` of its own, the test runner's own processes
included. A Gradle harness's run command passes `--rerun` to its test
task, so it runs the tests though Gradle deems the task up to date. A mutant runs its file's own tests first
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
runs it again the same way, after its selection's own baseline. A coverage run of the listed tests that fails
without any mutant, the run of every test or, when each runs alone, one
test's run, is `tests.coverage-failed`, exit 1, with its `command` and
`exit_code`: nothing is judged and no snapshot is written. The run of
every test sets `ITOS_CC_TEST_COVERDIR` without `GOCOVERDIR`, so a harness
builds its binary with `-cover` only when it gives the processes
`GOCOVERDIR` itself. A line a listed test reaches is never uncovered. Such a kill
rests on files no import names: the snapshot records the hash of the file
each of its tests is defined in, and of every file the `support` globs
match, such as the step code the tests run, and the kill holds while they
are unchanged. `mutation check`, `run`, `sample` and the graph call a
function stale when one changed, naming it; a run then reruns its listed
mutants, own tests first. No list command runs to judge it: deleting a
test changes its file. A list command may print a third column, a
fingerprint of the test's own text that it computes (for a Gherkin
scenario, its steps, tables, `Examples`, `Background` and tags, not its
comments or the scenarios beside it). A kill by a test listed with one is
dated by it, not its file: it holds while the list prints the test with
that fingerprint, wherever the test moved, and is stale, naming the test,
once it changed or the test left the list. Check, run, sample and except
then run the list command only when such a test's file changed, once a
command, and one that fails is `tests.list-failed`. Kills recorded before
itos-cc read fingerprints keep their files' dates until a run records
them again, and the graph dates every listed kill by its file. Only Go programs
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

A snapshot whose source file is gone, as when a run without `--since`, or
one whose paths do not select the new path, follows no rename, is an
orphan: nothing is at its path in the working tree, so a file deleted but
still committed is gone, while a new file at the old path is that path's
file again, whose functions go stale by hash. A file git shows renamed to a
file that exists, in the `--since` range or in the working tree against
`HEAD` (staged, as `git mv` leaves it, or added), is no orphan: its snapshot
stays for the `--since` run over the new path, which moves it. A plain `mv`
with nothing staged is a deletion and an untracked file to git, so its
snapshot is an orphan: stage a move (`git mv`, or `git add` the new file)
before a run. Outside a git repository, or when git fails, no snapshot can
be judged, and none is removed or named. A file outside the project
root has its snapshot named by its absolute path; such a snapshot is judged
by the file it records, not the path that name maps to under the root, so
it is no orphan while the file exists. Each run removes the orphans
under its paths, the working directory when it is given none, keeping
every snapshot whose file exists, and names each one it removed, in
`--json` under `orphans` as `{"file", "snapshot", "why"}`, a key there only
when there is one. The snapshot of a source `itos-cc.yaml` excludes is an
orphan too, though its file is there: `why` is `excluded` for it and `gone`
for the others. `mutation check` names
the orphans under its paths the same way, without judging or failing
anything for them. A `--count` run writes nothing, so it removes nothing.

Uncovered mutants never run, so a changed function no test executes passes.
`--fail-uncovered` makes each one a failure: listed like a survivor, a
`mutation.uncovered` problem in `--json`, and exit 1. With `--since`, only the
judged functions' uncovered mutants count. With `--no-coverage`, or where
coverage measured nothing for the language, every mutant runs and none is
uncovered, without `--fail-uncovered`. With it, a TypeScript, Python,
Kotlin or Dart build root with mutants to judge whose coverage tool is missing, or
whose coverage command fails or writes no report, fails closed as Go does:
`coverage.tool-missing` (exit 82) or `coverage.measured-nothing` (exit 1),
naming its language and directory, and the run stops before any mutant
runs; so does a Python build root whose coverage.py does not start in the
processes its tests start, a TypeScript package measured by Vitest or
Jest without c8, or a Kotlin build root whose JVMs wrote `.exec` files
with no jacococli to report them (`coverage.tool-missing`). `--fail-uncovered` is strict in every language: it also requires
fresh measured evidence for every judged Go, Python, TypeScript, Kotlin or
Dart function, including functions with no mutation sites, and fails each
uncovered Go executable block, each line a Python, TypeScript or Dart LCOV
report names executable that no test executed, and each line of a Kotlin JaCoCo or
Kover XML report none of whose instructions ran, as
`mutation.uncovered-statement` without changing mutant counts. A Kotlin
line some of whose instructions ran, such as one a test took one branch of,
is covered: branches are out of scope, as Go has none. So is a Dart line
any of whose code ran. Python's report is
coverage.py's over the tests that reach the file; TypeScript's is Vitest's
v8 or Jest's over its related tests (the whole suite with `--all-tests`), or
c8's over the test script of a package with neither; Kotlin's is Gradle's
`jacocoTestReport` or `koverXmlReport`, or Maven's `jacoco:report`, over
the test classes of the file's module that reach it (the module's whole
suite with `--all-tests` or `--test-command`); Dart's is `flutter test
--coverage`'s over the test files under the package's `test/` that reach
it (the whole suite with `--all-tests` or `--test-command`). A function's lines run from
its body's first statement to its end: a Python `def` line, or a TypeScript
arrow function's declaration, runs when the module is imported, not when
the function is called, and a Kotlin declaration's line holds the bridge
that fills in default arguments, which runs only for a call that omits one.
A Dart function's lines run from its signature line, which the Dart VM
counts at each call. A one-line Kotlin expression body (`fun f(x: Int) = x * 2`) shares that
line, which is covered when any of its instructions ran. A Python file no
test reaches has its executable lines listed by coverage.py's own analysis,
a TypeScript file no test loads by Vitest's v8 provider run with no test,
and a Kotlin file no test reaches by the report of another file of its
module, since JaCoCo and Kover list every class the module compiled, and
all of them are uncovered. Under Jest or c8 such a TypeScript file has no
evidence, nor does a Kotlin file whose module measured no other file in
the run (no report task writes a report without running a test), nor a
Dart library no test loads, which `flutter test`'s report never names, nor
any file a project's own `coverage` script measured (with `--all-tests`,
where it has one): `mutation.coverage-missing`. Empty and comment-only Go and
Kotlin bodies, and Python or TypeScript functions with no executable body
line, have no executable obligation. Strict coverage needs a successful
built-in or listed measurement; `--coverage-report`,
`--use-existing-coverage`, and `--coverage-command` are rejected in every
language. Kotlin evidence rests on its whole build root, every module of a
multi-module build included: the Gradle root its settings mark, or the top
of its Maven reactor. Dart evidence rests on its package, the nearest
`pubspec.yaml`, or for a pub workspace's member on the whole workspace. An active Go workspace or local
replacement outside the inventoried module is refused; set `GOWORK=off` to
disable workspace use. Refused likewise, as `mutation.coverage-unsupported`
for every judged function of its root even when older evidence looks
fresh, are an npm or yarn workspace (a `workspaces` field in a `package.json` from
the package root up to the project root), a `workspace:` dependency, and a
`file:` or `link:` dependency outside the package root, or a pnpm override
(`overrides` in a `pnpm-workspace.yaml` there) whose value is one; under a
`pnpm-workspace.yaml` whose `packages` list members, the workspace's own
root package, and a member that depends on another member by its package
name with a plain or `catalog:` version, which pnpm links as it links a
`workspace:` one. A `pnpm-workspace.yaml` with no `packages` key, or a null
or empty one, holds settings and is no workspace, and a pnpm workspace
member with no such dependency is proven as a package of its own: its
evidence holds what pnpm reads of its workspace root beside it, the root's
config files as a package root's are named, its `.npmrc` and
`.pnpmfile.cjs` and the patch files `patchedDependencies` names, and not
the other members. Refused too are a Python path or editable dependency
outside the build root, as `pyproject.toml` (a `path = "…"` key, as `[tool.uv.sources]` and Poetry
write one, or a `file:` URL), a `requirements*.txt` there (`-e` or
`--editable`, a path, a `file:` URL), or the root's `.venv` or `venv`
editable installs (`.pth` paths, setuptools' `__editable__` finders) name
one; a Gradle `includeBuild` outside the build root, or a Maven
`<module>`, `<subproject>` or parent `<relativePath>` outside the reactor;
and a pub path dependency (under `dependencies`, `dev_dependencies` or
`dependency_overrides`, in a `pubspec.yaml` or a `pubspec_overrides.yaml`
beside one) outside its root, the package or a member's whole workspace. Git and
hosted pub dependencies are pinned by `pubspec.lock` and admitted.
A dependency inside the root is admitted, unless it is reached through a
symbolic link or lies in a nested package or build root, which the
fingerprints leave out. The scopes are read from the root's own files,
lexically: detecting them, itos-cc never reads, lists or follows a path
outside the project root.

With `--since`, `--fail-uncovered=lines` judges coverage only where the
range changed, so old uncovered code beside a change does not fail it:
a Go block or a Python, TypeScript, Kotlin or Dart line counts when it
overlaps a line the commits added or changed (the new side of each hunk of
`git diff --unified=0 <base>...HEAD`, a renamed file's under its new path),
and an uncovered mutant fails only on such a line. A judged function with a
changed line still needs fresh evidence, as missing or stale evidence
leaves its changed lines unjudged; a function the range only deleted lines
from has none to judge. Survivors and exceptions are judged as before, and
evidence is still measured and recorded for whole functions, so a later
plain `--fail-uncovered` check of the same snapshot judges them whole. It
holds in `mutation run`, counted runs and `mutation check` alike.
`--fail-uncovered` alone is `--fail-uncovered=functions`, whole functions;
`=lines` without `--since` is a usage error, exit 80.

```sh
itos-cc mutation run --since origin/main --fail-uncovered=lines    # a gate on the lines a branch changed
itos-cc mutation check --since origin/main --fail-uncovered=lines  # the commit hook to match
```
A Maven parent with no `<relativePath>` is taken from a repository, not
refused. Strict coverage judges each language's files by its
own evidence alone: a Python, TypeScript, Kotlin or Dart file is never
judged for Go evidence, whether or not a `go.mod` sits above it. A counted
run (`--count`) still proves only Go's lines strictly: it does not yet prove
Python, TypeScript, Kotlin or Dart lines.

`--count N` judges at most N mutation sites freshly, without the cache, as
a bounded check of committed work. It resolves the repository and HEAD once
and judges that commit's tracked files in a private frozen copy under the
checkout's git directory (`git rev-parse --git-path itos`; a linked
worktree's own works too), so uncommitted changes play no part; tools and dependencies are used as
installed, from the live project, and nothing is installed or downloaded:
TypeScript's `node_modules` and Python's `.venv` or `venv` (else the
virtualenv `VIRTUAL_ENV` names) at its build roots, the `.fvm/flutter_sdk`
FVM links beside a committed `.fvmrc`, and the Go module, Gradle, Maven and
pub caches, with Go commands run with `GOPROXY=off`, Gradle with
`--offline` and Maven with `-o`. Each Dart package is readied once with
`flutter pub get --offline --enforce-lockfile`, which resolves the commit's
`pubspec.lock` from the pub cache, and its tests run with `--no-pub`. A dependency missing offline fails the
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
baseline, as a complete run does: both stages are one trial. A site of a
Python, Kotlin or Dart file no own test reaches has no own stage and needs no
clean baseline of its own tests: the listed tests that reach its line judge
it alone, as in a complete run. A selection
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
is reported not applicable. SIGINT or SIGTERM interrupts it, as it does
every command: the active judgment is cancelled with no outcome, every
process it started is stopped within five seconds, and the partial report
says `interrupted`, exit 130 (it exited 75 with `count.interrupted` before
decision 38). It runs on Linux and macOS, where every command it
starts runs in a process group it owns; a command that detaches from its
process group or session is not followed. On Windows it fails with
`count.platform` (#29).

`--fail-fast` stops a complete run at the first actionable failure it
observes, in the order judgments finish, for a quick fix-and-retry loop: an
unexcepted survivor, once the listed tests that reach it failed to kill it
too; with `--fail-uncovered`, an uncovered mutant or a strict Go, Python, TypeScript, Kotlin or
Dart coverage finding; a stale exception; a file whose tests fail before any mutant; or a
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
on Windows it fails with `fail-fast.platform`, exit 82, before any command
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
exits 1. A mutant an entry records at a site its unchanged function no
longer holds, as when a newer itos-cc removes a mutation operator, is not in
the code: `mutation check` leaves it out of the verdict and the counts, and
the next `mutation run` drops it from the entry, reusing the rest; `mutation
check` notes how many it left out, a `left out` line naming the function,
and `left_out` in its `--json` entry, failing nothing. An
exception of such a mutant is stale, as `gone`. A snapshot whose source
file is gone is an orphan, judged by the file it records, so the snapshot
of a file outside the root is none while that file exists, nor is one
whose file git shows renamed to a file that exists: `mutation check`
names an orphan, in `--json` under
`orphans`, and fails nothing for it, and the next `mutation run` over its
directory removes it. It takes `mutation run`'s paths, `--changed`, and `--since`, and
`--json` gives each file's `functions` with their `state`: `fresh`, `stale`,
or `missing`.

For strict Go, Python, TypeScript, Kotlin and Dart coverage, `mutation check` additionally rejects
missing or stale per-function block or line evidence without running tests,
coverage, or list commands. Go evidence fingerprints module Go sources and
tests, module configuration, project config, configured support files,
producer options, and relevant Go build settings. Python evidence
fingerprints every Python file of the build root (the file's reaching tests,
their helpers and `conftest.py` files, and the code they import included),
its `pyproject.toml`, `setup.cfg`, `tox.ini`, `pytest.ini` and `.coveragerc`,
project config, configured support files, the producer and the interpreter
with its environment; a virtualenv, hidden directories and nested build roots
are left out. TypeScript evidence fingerprints every TypeScript and
JavaScript file of the package root (the nearest `package.json`), sources,
tests and Vitest, Jest or Vite configuration alike, its `package.json`,
lockfile, `tsconfig*.json` and JSON Babel, SWC, Vitest and Jest
configuration, project config, configured support files, the producer and
the Node environment; `node_modules`, hidden directories, nested packages
and the root's `dist`, `build`, `out` and `coverage` are left out. Under a
`pnpm-workspace.yaml`, the nearest from the package root up within the
project root, it fingerprints too the config files of that file's folder as
a package root's are named, its `.npmrc` and `.pnpmfile.cjs`, and the patch
files its `patchedDependencies` name. Kotlin
evidence fingerprints its whole build root, the nearest directory with a
`settings.gradle(.kts)`, else the module's, or the top of its Maven reactor:
every module's `.kt`, `.kts` and `.java` sources and tests, Gradle scripts
(`build.gradle(.kts)`, `settings.gradle(.kts)` and the `.gradle` scripts
they apply), `gradle.properties`, version catalogs, the wrapper's
`gradle-wrapper.properties`, every `pom.xml` and `.mvn`'s configuration,
project config, configured support files, the producer, whether Gradle runs
through its wrapper, and `JAVA_HOME`, `GRADLE_OPTS`, `MAVEN_OPTS` and
`JAVA_TOOL_OPTIONS`; each module's `build` and `target` outputs, other
hidden directories and nested Gradle builds (a directory with settings of
its own, other than `buildSrc` or a build the root's settings name) are left
out. Dart evidence fingerprints every Dart file of its package, or of a
member's whole pub workspace, generated Dart included, each `pubspec.yaml`,
`pubspec_overrides.yaml` and `dart_test.yaml` there, the root's
`pubspec.lock`, the nearest `.fvmrc`, project config, configured support
files, the producer, the `flutter` that runs, `FLUTTER_ROOT` and
`PUB_CACHE`; hidden folders, `.dart_tool` among them, `build/`, `coverage/`,
`integration_test/` and `test_driver/` beside a `pubspec.yaml`, a Flutter
app's platform folders and a nested package the workspace does not list are
left out. Raw profiles and reports cannot be stamped with current
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
`pyproject.toml`, `setup.py` or `setup.cfg`; Kotlin's Gradle build root,
which holds `settings.gradle` or `settings.gradle.kts`, or else its Maven
module's `pom.xml`; and Dart's nearest `pubspec.yaml`. A Dart outcome
recorded with `--all-tests` depends only on the test files under its
package's `test/`, those `flutter test` runs with no path; one recorded with
`--test-command` depends on every Dart test file beneath its nearest
`pubspec.yaml`. Test files are those project discovery calls tests, so
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
the `replacement`, the `reason`, and the site's `line_text`, its line less
the space around it; excepting a site again replaces it. Only the lines of
the entry written change, in block style, after the list's last entry or in
the place of the one it replaces, keeping the comment above it; every other
byte of the file, its other entries included, is kept, so the commit shows
only the exception:

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
      line_text: let c = 0;
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
`gone`. An entry those judge whose file `exclude` leaves out is `excluded`.
`mutation except --prune`, run after `mutation run`, removes every entry
stale as `gone`, `killed` or `excluded`, which nothing can mend, judging
each entry as a check of the whole project does, and reports each removal
(`--json` lists them under `pruned`, each with its `why`). It keeps an entry
that `changed` or `moved`, for `--renew` or an edit to mend, and changes
only the removed entries' lines, with the comment lines directly above
each. The fix of an entry stale as `gone`, `killed` or `excluded` names it.
Only `mutation except` writes `itos-cc.yaml`. An
entry never excuses an uncovered mutant: that needs a test, not a reason. An
`itos-cc.yaml` that cannot be read, or an entry with no reason, is
`config.invalid`, exit 80.

A change that leaves an excepted mutant the same, such as a rename, a
literal or comment edit, reformatting or a line added elsewhere in its
function, still makes the entry stale. `mutation except --renew`, run after
`mutation run`, carries such entries across: an entry is renewed when its
function, by name in its file (or, its file gone, by name and hash in one
other file), has fresh results holding exactly one site of its `original`
and `replacement` on a line whose text, spaces aside, is its `line_text`
(an entry without one, from before entries had it, matches the site at its
`line_in_function` and `column`, else the function's only site of that
change), and that mutant survived. The entry then names the function's
current hash, name, place and line text, and keeps its reason and comments;
`--renew` changes only the lines of the entries it renews. Each renewal is reported with its old and
new hash and the site, and each entry left stale stays
`mutation.exception-stale` with why: `killed`, `changed` (its line's text
changed), `gone`, `ambiguous`, `no-results` (no fresh results for its
function) or `uncovered`, exit 1. A setup step can so re-prove a project a
template rendered: `mutation run`, then `mutation except --renew`. The fix
`mutation run` and `mutation check` give an entry stale because its function
`changed` names `mutation except --renew` first, before excepting the mutant
again by hand.

Killed mutants are kept per function in `.metrics/mutate/`, so the day's
runs reuse the night's kills for code that has not changed. Each file's
snapshot also records, under `tests`, the SHA-256 of every test file that
reaches the file, by its path from the project root. In TypeScript, Python,
Kotlin and Dart those are the test files whose imports reach its module, directly
or through other modules of the project (Kotlin's same-package references
included, and in Dart `package:` and relative imports, exports, parts,
every target of a conditional import, and a pub workspace's sibling
members), as `vitest related` and `jest --findRelatedTests` select them.
A test also reaches what the test-support files it imports reach, such as a
helper under `tests/` or `__tests__/`, a Kotlin helper in `src/test` or a
Dart one under `test/`, which are in the set themselves, a Python test what
every `conftest.py` in its directory and those above it, up to its build
root, reaches, and a Dart test what the nearest `flutter_test_config.dart`
in its folder or above it, up to its package, reaches. In Dart the set holds
only what its own tests run: the test files under its package's `test/` and
the support files on their way, so a change under `integration_test/` or
`test_driver/` makes nothing stale. In
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
is written to run `itos-cc mutation run --all-tests --no-annotate` at 09:00 UTC
and commit `.metrics/mutate/` back even on nights when a mutant survives and
the job fails; copy it as a starting point. It is disabled here until T-46
makes a whole run of this repository finish within a job's time limit. Raw coverage under
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
baseline failed. Each file's `baseline` says whether this run ran its own
tests without any mutant: `passed` means they ran and passed in this run,
for it or for another file sharing its command (a Go package's files share
one), `failed` that they failed, and `not-run` that none ran, as for a file
whose mutants were all reused from its snapshot, which said `passed` before,
or one no test reaches. A script that checked for `passed` should accept
`not-run`, or check for `failed` instead.

### scrap

Finds the test cases of Vitest/Jest, pytest/unittest, Go `testing`
(including `t.Run` and table loops), JUnit/kotest, and flutter_test and
package:test (`test` and `testWidgets` calls, named under their `group`s,
and a test called in a loop as a table), and scores each one on
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

- A module is a file in TypeScript, Python, Kotlin and Dart, and a package
  in Go.
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

The Go code is laid out as [decision 29](docs/decisions/) says: shared domains behind ports,
adapters apart from them, and thin slices named for their commands. [docs/go.md](docs/go.md)
describes each package and the import rules `.golangci.yml` holds them to.

| Package | Role |
| --- | --- |
| `cmd/itos-cc` | The command line: `main`, the one kong model of every command, and the wiring of each slice to the adapters, which `tools/bin/assemble` writes into `commands_gen.go` from each slice's declaration |
| `internal/cli` | The UI every command shares: reading the command line, help, problems and exit codes |
| `internal/slices/units`, `internal/slices/dry`, `internal/slices/scrap`, `internal/slices/crap`, `internal/slices/version`, `internal/slices/mutation` | The commands' slices, each under `internal/slices` (T-86), a slice's own domain in its `domain` subpackage (`internal/slices/dry/domain`, which finds duplicates, `internal/slices/scrap/domain`, which measures test code's structure, and `internal/slices/crap/domain`, which scores functions by CRAP, and `internal/slices/mutation/domain`, which owns mutation sites, results and their judgment); `serve` alone stays on its legacy handler in `cmd/itos-cc` |
| `internal/selection`, `internal/lang`, `internal/snapshot`, `internal/coverage` | The shared domains, each reaching outside only through its ports |
| `internal/workdir`, `internal/metricsdir`, `internal/coveragetools` | The adapters implementing those ports |
| `internal/slices/mutation/adapters/testtools`, `internal/slices/mutation/adapters/testinputs`, `internal/slices/mutation/adapters/worktrees`, `internal/slices/mutation/adapters/githistory`, `internal/slices/mutation/adapters/importgraph`, `internal/slices/mutation/adapters/freshenv`, `internal/slices/mutation/adapters/snapshots`, `internal/slices/mutation/adapters/exceptions`, `internal/slices/mutation/adapters/sources` | A slice's own adapters, each under `internal/slices/<command>/adapters/<name>` with a rule of its own: the mutation slice's tools, its inputs' fingerprints, its workers, its committed history, its import reach over `internal/imports`, its counted fresh environments, its snapshot codec, its itos-cc.yaml exceptions writer and its source edits |
| `internal/processes` | The process-ownership engine: the commands mutation and crap run, each in a process group it owns, killed and joined when it ends (ADR-0021) |
| `internal/imports` | The import-resolution engine: each language's imports resolved into dependencies between modules, and the tests that reach each source, for the graph and the mutation slice's import reach |
| `project` | Finding source and test files, and the disk lang's port asks: reading and parsing a file (`ParseFile`) and finding a project marker above it (`FindUp`) |
| `coverage` | Running coverage and reading LCOV, Go profiles, and JaCoCo |
| `config` | Reading `itos-cc.yaml`, which the mutation slice's exceptions adapter writes |
| `metrics` | Snapshot files |
| `graph`, `server` | The architecture graph, its dependencies resolved by `internal/imports`, and its live HTTP API |
| `internal/version`, `internal/release` | The build's version, the one source the `version` command prints through cmd, and the library of the release tools in `tools/bin` |
| `features` | The scenarios and their steps, each file of steps declaring its feature file, listed in `steps_gen.go` by `tools/bin/assemble` ([features/README.md](features/README.md)) |
| `viewer/` | The web viewer, a separate npm project |

Adding a language means one file in `internal/lang/` (grammar, units, decisions,
syntax, mutation rules), test-framework rules in `internal/slices/scrap/domain/`, a
coverage plan in `coverage/`, an import resolver in `internal/imports/`, and the mutation
slice's commands, workers and evidence. [docs/architecture.md](docs/architecture.md)'s "Adding
a language" lists each place, with Dart as the worked example.

## Working here

The repository is adopted from the template
[go-template-itos](https://github.com/donvargax/go-template-itos) (stack go, feature cli;
`.itos-template.yaml` records the release), so `itos-template update` can bring its later
releases. Read [AGENTS.md](AGENTS.md) for the project's rules. [docs/CLI.md](docs/CLI.md) holds
the command line's contract and rules, [docs/engineering.md](docs/engineering.md) the shared
engineering rules, [docs/go.md](docs/go.md) Go's, [docs/architecture.md](docs/architecture.md)
the layer boundaries, [docs/security.md](docs/security.md) the trust boundaries,
[docs/upgrading.md](docs/upgrading.md) how to move dependency pins,
[docs/CONFIG.md](docs/CONFIG.md) the rules for a configuration file the project owns,
[features/README.md](features/README.md) the scenarios, and
[docs/decisions/](docs/decisions/) its decision records. The code is laid out as decision 51
says, each command a slice under `internal/slices` over shared engines, as docs/go.md and
docs/architecture.md describe; `serve` alone keeps its legacy handler while it leaves for a web
app of its own. [docs/ownership.md](docs/ownership.md) says who owns each path, slice by slice,
and which paths are shared. The template's sample count app, kept as a reference while porting
(q-48), is gone (T-116).
[CONTRIBUTORS.md](CONTRIBUTORS.md) records where the harvested infrastructure came from. `itos go`
coordinates the work; an implementing agent uses `itos guide work`.

### Hooks

A clone runs four hooks, each declared in its own `.git/config`, so a fresh clone has none until
it declares them, and every worktree of the clone runs what the clone declares. `itos hook
install` declares itos's own, `commit-msg` and `pre-push`. The project's `pre-commit` hook,
[`tools/hooks/pre-commit`](tools/hooks/pre-commit), runs gofmt over the staged version of each Go
file a commit adds, copies, modifies or renames, and names each one gofmt would change. It reads
the index, not the working tree, so it refuses an unformatted staged file whose working copy was
fixed later and accepts a formatted one with unstaged edits beside it. It formats nothing, stages
nothing and skips deleted files; a commit with no staged Go file, prose alone for one, runs no
gofmt. It needs `gofmt` on `PATH`, the Go toolchain's.

The project's `pre-push` hook, [`tools/hooks/pre-push`](tools/hooks/pre-push), runs after
itos's and refuses the push, with vet's output, when `go vet ./...` fails on the working tree:
vet compiles every package and its tests. `itos push` rebases onto the upstream and refuses
tracked changes, so the working tree is the rebased HEAD about to land, and the hook catches two
pushes that each compiled alone but clash together, two files of one package declaring the same
name, before main stops compiling (T-177). It needs `go` on `PATH`, and costs a fraction of a
second with the build cache warm, some seconds after a change to the module. golangci-lint and
the tests stay in CI, so the hook stays that quick. Declare both hooks once in each clone, from
the top of the working tree:

```sh
git config hook.itos-cc-pre-commit.event pre-commit
git config hook.itos-cc-pre-commit.command tools/hooks/pre-commit
git config hook.itos-cc-pre-push.event pre-push
git config hook.itos-cc-pre-push.command tools/hooks/pre-push
```

Check what the clone declares with `git config --get-regexp '^hook\.'`, which lists the four
lines above beside itos's four, `git hook list pre-commit`, which names `tools/hooks/pre-commit`,
and `git hook list pre-push`, which names itos's hook and then `tools/hooks/pre-push`.
`tools/bin/pre-commit-check` proves the first hook itself: it declares it the same way in a
temporary repository of its own and makes real commits there, accepted and refused (T-120).
`go run ./tools/bin/pre-push-check` proves the second: two clones of a local bare remote declare
it, one pushes a helper, the other's helper of the same name compiles alone and is refused once
rebased, with vet's message, and accepted once renamed. Neither proves another clone has
declared them; that clone's `git config` does.

The template, go-template-itos v0.2.0, brought `tools/hooks/pre-commit` running the pinned
golangci-lint and the networked govulncheck over the whole module on every commit that staged
code. This project's version is the staged gofmt check above instead: a commit pays only for the
files it stages, and needs no network. The template's two checks, and gofmt over every tracked Go
file, are steps of `itos.yaml`'s CI plan, which `ci.yml` and `cd.yml` run on every push that
touches code ([docs/go.md](docs/go.md), Quality checks); none was moved or turned off.

## CI

Every push to `main` and every pull request starts two workflows side by side, as two separate
runs ([decision 48](docs/decisions/0048-ci-is-a-fast-linux-push-gate-and-cd-runs-the-full-suite-on-every-system-and-cuts-the-release.md)):

- [`ci.yml`](.github/workflows/ci.yml) is the push gate, the run `itos push`, `itos ci watch` and
  `itos work done` wait on. It runs on Linux alone and ends in a few minutes: the commit rules
  and `itos.yaml`'s plan (`itos ci run`, whose one scenario run takes the smoke set and the
  scenarios the push's range names through both drivers), and the Go tests of every package,
  the in-process driver's whole suite among them. It cuts no release.
- [`cd.yml`](.github/workflows/cd.yml) runs everything: the same commit rules and plan, every
  Go test on Linux, macOS and Windows, every scenario through both drivers, the native mutation
  and snapshot scenario runs, the tests that need the real TypeScript, Python and Kotlin
  tools, and, in its `dart` job on Linux alone, the Dart scenarios with the Flutter the root
  `.fvmrc` pins. On a push to `main`, once all of them pass, it cuts the release
  ([`release.yml`](.github/workflows/release.yml)). It is the only workflow that does, so a
  green `ci.yml` alone never publishes.

The scenarios run through two drivers: in process against the ports' fakes, and against the built
binary. The binary driver runs only where it is opted into
([decision 52](docs/decisions/0052-local-runs-hold-the-scenarios-in-process-against-the-fakes-and-the-built-binary-runs-them-in-ci-and-cd.md)):
every workflow sets `ITOS_CC_FEATURES_BINARY_DRIVER=1`, so the binary runs every scenario in CI
and CD as above. A local `go test ./...` or `itos tests smoke run scenario` runs the scenarios in
process alone, as in-memory end-to-end tests, and `TestFeatures` logs that the binary driver ran
nothing; set `ITOS_CC_FEATURES_BINARY_DRIVER=1` to run it, as an agent working on a
`@binary-only` scenario does ([features/README.md](features/README.md)).

A push whose range touches only prose runs no tests in either workflow and cuts nothing; with no
green run to start from, both run everything. A green `ci.yml` no longer means every test
passed: itos watches `ci.yml` alone until [itos#84](https://github.com/donvargax/itos/issues/84)
lets it watch a second workflow, so the coordinator watches each `cd.yml` run with
`gh run watch <id>` in the background and hands a red one to a fresh agent as repair work, ahead
of new items.

Renovate lands each update on `main` through a pull request of its own
([`.github/renovate.json5`](.github/renovate.json5)). Neither workflow runs on a push to a
`renovate/` branch, so Renovate opens the pull request as soon as it makes the branch, once the
release's 7-day wait is over, and both workflows run on it. When `main` moves on, Renovate rebases
the branch and the checks run again. Renovate merges the pull request once every check on its head
is green, `cd.yml`'s Windows and macOS jobs included, with GitHub's rebase merge, so the commit
lands on `main` with the message Renovate wrote and its `Task:` footer alone in the last
paragraph. The repository does not allow GitHub's auto-merge, so Renovate's own run does the merge:
an update lands at the first run of the Renovate workflow in donvargax/github-admin after its
checks pass, hourly on Mondays and every four hours the rest of the week.

No workflow runs mutation proof yet. `itos.yaml`'s plan has no `proof.code` step and no mutation
sample, and [`nightly.yml`](.github/workflows/nightly.yml), the nightly mutation run, is disabled
on GitHub until T-46; both now wait on T-46 alone, the refactor's final comparison (T-132)
being done.
CI's golangci-lint covers the whole module, `./...`.
[docs/go.md](docs/go.md) lists every configured check.

## License

[GNU Affero General Public License v3.0](LICENSE).
