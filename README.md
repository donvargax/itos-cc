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

Run from a project root. Paths are files, directories, or fragments of a
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

A build root whose coverage could not be measured (tests that do not
compile, a missing tool, no report) shows `N/A`, not 0%, and is named on
stderr. With `--threshold`, a function above it exits 1, and so does a
coverage run that measured nothing, such as tests that do not compile; a
missing tool or report exits 3. A broken test setup never passes the gate.

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
  `vitest related` / `jest --findRelatedTests`, `pytest -x`. Coverage comes
  from the same tests, so a line only integration or end-to-end tests reach
  is uncovered, not a survivor.
- **Parallel workers** in private copies of the project, so the real tree is
  never modified while tests run. The baseline runs inside a worker, which
  proves the copy works before any mutant does.

`--all-tests` runs the whole suite for coverage and for every mutant, so
integration and end-to-end tests anywhere in the build root can kill
mutants. Tests that only run the built binary do not show up in coverage, so
add `--no-coverage` to let them reach code nothing else covers. It is slow;
run it nightly rather than on every change:

```bash
itos-cc mutation run --changed                               # while working: own tests, fast
itos-cc mutation run --since origin/main --fail-uncovered    # a gate: the functions the branch's commits changed
itos-cc mutation run --all-tests                             # nightly, e.g. a scheduled CI job
itos-cc mutation run --all-tests --no-coverage               # nightly, with end-to-end tests that run the binary
itos-cc mutation list src/billing                            # the mutation sites, without running tests
itos-cc mutation check --since origin/main --fail-uncovered  # a commit hook: cached results, nothing run
itos-cc mutation sample                                      # in CI: do 20 cached results still hold?
itos-cc mutation except src/board.ts:3:13 --reason '…'       # an equivalent mutant: no test can kill it
```

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
uncovered.

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

`mutation check` trusts the snapshots, so a cache written by hand, or
against other code, passes it. `mutation sample` runs a few cached mutants
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
it overrides the scope of every mutant.

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
site is `gone`. With `--since`, only the judged functions' entries count. An
entry never excuses an uncovered mutant: that needs a test, not a reason. An
`itos-cc.yaml` that cannot be read, or an entry with no reason, is
`config.invalid`, exit 2.

Killed mutants are kept per function in `.metrics/mutate/`, so the day's
runs reuse the night's kills for code that has not changed. Each file's
snapshot also records, under `tests`, the SHA-256 of every test file that
imports the file directly, by its path from the project root: in Go, the
test files of its package and of the packages that import it. That set is
the same whichever command ran the mutants (its own tests, `--all-tests`, or
`--test-command`). When a test file is added to it, changed, or removed, the
kills it may have made no longer hold: the file's mutants all run again, and
`mutation check` calls its functions stale and names the test files. A
snapshot written before snapshots recorded tests is stale too. A test that
reaches the file only through another module, or that runs the built binary,
is not in the set, so changing it leaves the results as they were. Commit that
directory, and have the nightly job commit it back: CI starts from a fresh
checkout, and without it every night is a first run. This repository's own
nightly job, [`.github/workflows/nightly.yml`](.github/workflows/nightly.yml),
runs `itos-cc mutation run --all-tests --no-annotate` at 09:00 UTC and commits
`.metrics/mutate/` back even on nights when a mutant survives and the job
fails; copy it as a starting point. Raw coverage under
`.metrics/coverage/` ignores itself and is never committed.

Results go to `.metrics/mutate/<file>.json`, and a summary comment is kept at
the end of each source file (`--no-annotate` turns it off). A surviving
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
  results are marked stale as `mutation check` calls them: when the function
  was edited, or a test file that imports its file changed, since its last
  mutation run.
- Click a box for its functions, a function for its source, an arrow for the
  imports behind it. Double-click or Enter opens a box; Esc goes up.

API: `GET /api/graph` (JSON), `GET /api/events` (server-sent events carrying
the graph version), `GET /api/source?repo=&file=` (only files in the graph).
The server listens on localhost only.

## .metrics

Every command writes a JSON snapshot under `.metrics/`: `crap.json`,
`dry.json`, `scrap.json`, and `mutate/`. Commit them: a clone then has the
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
