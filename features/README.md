# Feature files

Every `feat:` and `fix:` commit of this project is driven by scenarios in this folder.
Nothing else belongs here. The harness is harvested from itos's
(github.com/donvargax/itos, `features/`) and itos-template's, without either one's own
generator behaviour.

## What goes in a feature file

Only behaviour a user of the project can observe: what a command does to a folder, its exit
code, and what it prints.

Never here:

- code structure, dead code, dependencies, formatting (`gofmt`, `go vet`, golangci-lint,
  govulncheck);
- refactors and performance budgets (tasks in `tasks/`);
- internal logic (unit tests, next to the code).

## Black-box boundary

The steps (`steps.go`, `units.go`, `dry.go`, `scrap.go`, `crap.go`, `complexity.go`,
`coverage.go`, `selection.go`, `cli.go`, `metrics_snapshots.go`, and `mutate.go` with its
pieces, `mutate_*.go`, Go, run by [godog](https://github.com/cucumber/godog)) treat itos-cc as a black
box. They reach the world only through a `Driver` (`steps.go`): a fresh folder, files written and
read, symbolic links (`Link`), git, runs of itos-cc with a folder, environment, stdin, arguments
and programs left off the PATH (`Run.Without`), what a project's own coverage tools measure
in a folder (`Coverage`), which a Given declares before a run measures with them, what its own
tests do there (`Tests`), which a Given declares before a run runs them, builds of
itos-cc as a release, go install or a person's go build makes them (`Build`, run through
`Run.Binary`), a port held in use (`Occupy`), and whether the system makes counted runs
(`Counted`). They are ordinary files of package
`features`, so `cmd/itos-cc`'s tests can run them in process against the ports' fakes (decision
44); `itos-cc.yaml` excludes `features/**`, so they are neither measured nor mutated.
Two drivers run them, one scenario at a time each: `cmd/itos-cc`'s `TestScenarios` in process,
its driver holding every folder and file in memory with a fake git and calling itos-cc's `run`
against the ports' fakes, and `TestFeatures` through the binary driver (`binary.go`). The binary
driver runs every converted file; the in-process driver only those of the commands moved onto
kong, the ones `InProcess` lists in `steps.go`, a file joining it when its command moves. A
scenario only one driver can run, for good, carries `@binary-only` or `@in-process-only`, never a
copy, and the other driver passes it by; `@binary-only` never means "not moved yet". The
in-process driver refuses a command still hand-parsed (`serve`, `version`), naming
`@binary-only`; a run's variables (`Run.Env`) reach the fakes as the environment facts the real
adapters read. itos.yaml's scenario lines run both, so every scenario of a moved command is held
both ways. `TestFeatures` builds itos-cc once a run
from this tree (`itos-cc.exe` on Windows), stamped with a known version and commit, as a
release's build is stamped with the release's. Each scenario gets a scratch folder in a temporary
directory, runs the binary in it and reads its exit code, its output and the files it leaves. A
step judges the program by its command line alone: it never reads its source or calls its code to
decide a result.

Commands run in a clean environment: no `GIT_*`, `ITOS_*`, `ITOS_CC_*`, `GITHUB_*`,
`GH_*` or `CI` variable of the caller's, no global or system git config, a fixed author, and none
of the caller's claude, itos or its extensions on the PATH. A `GOCOVERDIR` that exists builds the
binary with `-cover` and collects what each run did, for itos-cc to merge with `go test`'s.

A Given that declares a project's coverage tools (`Coverage`) needs them installed for the binary
driver: Go needs `go`; TypeScript `node` and a `node_modules` holding vitest and
`@vitest/coverage-v8`, `ITOS_CC_NODE_MODULES`'s, else `testdata/tools/node`'s, else the
viewer's, linked into the project; Python `python3` with pytest and coverage, linked into the
project as `.venv/bin/python` where it keeps its own virtualenv (`Venv`); Kotlin needs Java and
the fixture's existing Gradle or Maven (`Program`) and declares its measured JaCoCo XML; and a program the
project's coverage runs besides, such as make (`Program`). A tool missing skips the scenario, or fails it under `ITOS_CC_LANGUAGE_TOOLS`, which the CI job that installs the
pinned tools sets; any other CI job skips the TypeScript, Python and Kotlin ones, its runner's tools being
whatever its image has. The in-process driver hands what was declared to the fake measurer a
command measuring coverage is given.

A Given whose project's tests a run of itos-cc runs, mutation run's baseline and trials, declares
what they do (`Tests`): their language and the command itos-cc runs them with, what a failing
baseline prints, the sites whose mutants survive or time out, every other mutant they run being
killed, and how long a run of them takes. The binary driver checks their tools as `Coverage`
does, the language's own (`go`, `node`, `python3`, `java`) and the program the command runs where
the PATH would find it, pytest for a command running it, skipping or failing alike, skips tests
said to take under 100 milliseconds on Windows, where their baseline takes several times that,
and the real tests then decide every outcome; the in-process driver hands it to mutation's fake
`Workers`, which run each mutant as declared, the outcome chosen by the requested folder and
command. A folder with no `Tests` declared has no tests a run may run, and the fake refuses an
undeclared command, so a fixture names the command itos-cc actually runs. A scenario of a counted run, `mutation run --count`, skips where itos-cc makes
none, on any system but Linux and macOS (#29), in either driver (`Counted`). A scenario that only reads which Gradle or
Maven command itos-cc chose runs with `gradle` and `mvn` off the PATH, so neither runs anywhere;
one that needs a symbolic link skips where the system makes none. The in-process driver refuses
both, naming `@binary-only`, and so a build of itos-cc and a port held: the binary driver builds
into the scenario's folder with -trimpath, `go install` from a module proxy it writes there,
the tree's files zipped beside the module cache's downloads, and holds a port of 127.0.0.1,
skipping where the system lets a second listener take it.

Before each run the world declares the import adjacency of the scratch folder (`imports.go`):
every source and test of a language itos-cc reads, outside the folders no selection walks into
(hidden ones, `node_modules`, `vendor`, `venv`, `__pycache__` and `testdata`), each with the files
a fixture said it imports (`world.imports`) and none otherwise, an edge to a file no longer there
dropped. It hands that to `Driver.Tests` as `Tests.Imports`, alone, with no command or outcome.
The binary driver checks only that such a declaration declares nothing else, since the real
resolver reads the same edges from the files' text; the in-process driver gives it to mutation's
`Reach` fake, which computes each language's reach from it: Go's package and one hop, the other
languages' transitive closure, Python's applicable `conftest.py` and Kotlin's references. A
fixture whose files import one another declares those edges, so both drivers see the same reach.

An interrupt is the binary driver's alone (`interrupt_test.go`), sent as each system delivers
Ctrl-C, from one build-tagged file per system:
`os.Interrupt` on Linux and macOS, and on Windows `CTRL_BREAK_EVENT` to the binary started in a
process group of its own, which Go reports as the same signal. The step first writes more to the
binary's open stdin than any pipe holds unread, so the write returns only once the binary is
reading, its interrupt handling already in place; no sleep guesses when. A run the interrupt
killed by the system's default action fails the step: it never handled the interrupt.

## Which files run

The harness runs the feature files that have steps, the ones `Converted` lists in
`steps.go`: `units.feature`, `dry.feature`, `scrap.feature`, `crap.feature`,
`complexity.feature`, `coverage.feature`, `file_selection.feature`, `cli.feature`,
`mutate.feature`, `metrics_snapshots.feature`, and `count.feature`, the
template's sample, all of whose scenarios are `@wip`. `mutate.feature` converted a piece at a
time (T-70 to T-81); the scenarios still without steps are listed in `Unstepped` (`steps.go`), by
their IDs, and run nowhere: today the four `@wip` Windows ownership scenarios, ID-MUT-169 to 172,
deferred outside T-80. A selection naming only them logs them as not run, their steps not written
yet, and passes; a tag other scenarios carry too selects those others alone; and an ID `Unstepped`
lists that no scenario of a converted file carries fails every selection. Every other file
converts to godog over time, before or with the command that moves onto kong (q-52), its
scenarios' text unchanged; until then the Go tests of `cmd/itos-cc` hold its scenarios. A
selection that names a scenario of such a file, as the smoke set does for every file, logs it as
not run here and runs the rest; one that names nothing a converted file holds fails, as one
matching no tag does.

The in-process driver runs those of `InProcess`: all of them but `cli.feature`, whose help,
version and mutation scenarios need commands that move later, which joins it in S9.
`mutate.feature` joined it when the mutation group moved (T-125), and
`metrics_snapshots.feature` once the mutation commands it runs had moved (T-101). In process, a
selection naming a scenario of a converted file not in `InProcess` logs it as binary driver only
until its command moves, and one naming nothing else runs nothing there and passes, the binary
driver running it.

`Deferred` (`deferred.go`) names, by ID tag, the scenarios of `InProcess` files that the
in-process driver leaves to the binary driver for now, each with why, naming the work that owns
it. Unlike `@binary-only`, which a scenario only the binary can run carries for good, an entry
goes once its owner's work lets the scenario run in process. Today it names ID-MUT-134, 153, 154,
163 and 207, which observe `serve` (`Driver.Graph`) as well as mutation: `serve` stays on its
legacy handler, its moves (T-102 to T-108, T-112 and T-113) deferred while it leaves for its own
web app. `Defer` takes them out of the in-process selection, which logs each one it matches with
its reason; a tag `Deferred` lists that no scenario of a feature file carries fails the run, so an
entry cannot outlive its scenario. The binary driver runs every one of them.

A step a driver cannot do for a reason outside the program, such as a missing tool, a system that
makes no symbolic link, or a capability the memory does not hold (`ErrPhysicalUnsupported`),
skips its scenario with that reason, wrapped around `godog.ErrSkip`, and never passes in its
place. godog's summary counts a skipped scenario as passed, so the in-process run logs each one
with its reason; the binary driver's native CI runs keep godog's cucumber JSON, which tells an
omission from an execution.

`ITOS_CC_SKIP_FEATURES` leaves out a comma-separated list of feature filenames in both drivers;
unset or empty leaves out none. Both drivers log the omitted filenames and run only the other
files, including when a selected tag is shared with an omitted file. A selection naming only
omitted scenarios passes without running them; unknown tags and invalid `Unstepped` IDs still
fail. No CI job sets it: push CI left `mutate.feature` out with it from T-83 until T-125 moved
the mutation scenarios in process, and T-101 confirmed the setting gone.

A `units.feature` scenario runs in a sample project of TypeScript and Python, production code and
tests, unless it writes files of its own and lists those alone. A `dry.feature` scenario that only
says When I run runs in a sample project of TypeScript in a git repository, one file changed since
its commit, its functions paired at and just past every bound those scenarios name; the others
write their own files. A `scrap.feature` scenario that only says When I run runs in a sample of
test files in Python, TypeScript and Go, of four pressures. A `crap.feature` scenario that only
says When I run runs in a sample Go module, `board`, of 24 functions, `f01` to `f24`, each of
complexity its number, the first four wholly covered by its test and the rest never run, so no two
score the same, beside a Kotlin module in `kt/` built with Maven and no JaCoCo, which coverage
cannot measure and no tool needs to be installed to show. A `coverage.feature` scenario that only
says When I run, or that coverage is run, runs in a sample Go module, `example.com/m`, whose
`a/a.go` its test covers and `b/b.go` no test runs, beside `a/lcov.info` and `b/coverage.out`, a
person's reports of each, and a Makefile whose cover target writes `cover.out`, with a second
module, `example.com/other`, in `other/`; each test writes `test-ran` in its folder when it runs.
`file_selection.feature`'s Background project holds a function in each file: a Go module at the
scratch folder (`board.go` and its test), TypeScript and Python sources and tests in
`src/billing/` and `src/cart/`, a Kotlin module in `kt/` built with Maven and no JaCoCo, and
`testdata/board.go`, a fixture of its Go tests, not a source.
Step patterns are global across the files, so each file's own setup is a
Before hook keyed on the scenario's file.

## Running them

```sh
go test ./features -count=1                          # every live scenario, through the binary
go test ./features -count=1 -scenarios='^@ID-CLI-'   # the live scenarios with a matching tag
GOCOVERDIR=$d go test ./features -count=1            # built with -cover, its coverage in $d
go test ./cmd/itos-cc -count=1 -run '^TestScenarios$' -scenarios='^@ID-DRY-'   # in process
itos tests smoke run scenario                        # exactly the smoke set, both ways
```

`-scenarios` takes a regular expression over each tag, `@` included. It exists because godog's
own tag filter takes exact tags joined by commas, while itos merges selections into one regular
expression joined by `|`. One that matches no tag fails the run, so a selection never passes by
running nothing. Each scenario is a subtest of `TestFeatures`, named after the scenario.

## Checking assertion strictness

Use the serial developer tool on Linux or macOS, with the checkout reserved for one writer:

```sh
go run ./tools/bin/step-strictness -plan tools/testdata/step-strictness/smoke.json -print-plan
go run ./tools/bin/step-strictness -plan tools/testdata/step-strictness/smoke.json
```

The reviewed smoke plan selects the already-live MUT156, proves a green control, changes its
killed-count assertion, observes the intended failure, and restores the initial bytes and mode.
This is a local source-editing check, never a CI gate or product mutation run. Later conversions
provide their own reviewed probes; a detected probe proves only that assertion noticed that edit.
Review assertion coverage separately and run the final unedited scenarios. The tool does not
generate expectations, infer them from product internals, or prove all behavior from a sample.

The version 1 JSON manifest follows [schema-v1.json](../tools/testdata/step-strictness/schema-v1.json).
Unknown versions, unknown or duplicate keys, null values and invalid edits are refused before
editing. `-print-plan` validates without writing and prints defaults. Paths use `/`, relative to
the manifest. Fields are:

| Field | Type and requirement | Default or meaning |
| --- | --- | --- |
| `version` | required integer | `1` |
| `root` | required nonempty relative path | checkout containing `features/` |
| `output_dir` | optional relative path | empty or omitted creates a unique OS temporary directory; an explicit directory must be new, outside the checkout, with an existing parent; no commas (Godog formatter separator) |
| `deadline_seconds` | optional integer, 1–600 | `600` per subprocess, including its build; Go's default test timeout stays unchanged |
| `probes` | required nonempty array | ordered independent edits against the initial source |
| `probes[].name` | required unique string | letters, digits, `_`, `-`, beginning with a letter or digit |
| `probes[].scenario_id` | required string | one `ID-AREA-number`, without `@` or a regular expression |
| `probes[].source_path` | required relative path | actual `.go` or `.feature` acceptance source below `features/`; no symlinked components or special permission bits |
| `probes[].original` | required nonempty string | must occur exactly once; include surrounding text to distinguish identical checks |
| `probes[].replacement` | required nonempty string | must differ from `original` |
| `probes[].assertion.uri` | required nonempty string | exact Godog feature URI, such as `mutate.feature` |
| `probes[].assertion.line` | required positive integer | intended step's feature line, stable between control and edit |
| `probes[].assertion.name` | required nonempty string | exact expanded step name after the edit; no keyword prefix |
| `probes[].assertion.error_token` | required nonblank string | reviewed substring of that assertion's error, distinguishing it from a hook/setup failure |

Each invocation owns a new evidence directory, printed on completion, containing the resolved
default manifest, `recovery.json`, exact initial source backups, `report.json`, and distinct
`000-control`/numbered-probe directories with original `test.log` and `cucumber.json`. A retry
uses another directory, preserving earlier attempts. The test-only `-scenario-report=<path>`
flag adds Godog's pinned built-in cucumber JSON formatter beside normal pretty output. Default
feature output and the product's flags are unchanged. The runner consumes structured statuses,
feature URI, element ID/line and step identity, not colored pretty summaries or scenario names.
Repeated scenario names and expanded examples remain distinct. All selected control cases must
actually pass; every expanded example must reach the intended assertion. `And`/`But` inherit the
preceding `Given`/`When`/`Then` phase, so setup failures cannot qualify. Skips after the intended
failure are normal cascades; missing cases, skips before the assertion, compile/undefined/setup
failures, missing/malformed JSON and deadlines are invalid. A passing edited assertion is
undetected. Exit status is 0 only when every probe is detected, 1 for undetected probes, and 2
for invalid evidence or execution/validation/recovery errors. Reports retain actual subprocess
status, case/step records, durations, counts and original/edited/verified-restored SHA-256 hashes.

Restoration preserves ordinary permission bits and initial dirty work. Sources with
setuid/setgid/sticky bits are refused, including newly added bits before restoration. The owned
`go test` process runs in a POSIX group, cancelled as a group and joined before restoration;
output goes directly to a file. This is bounded runner cleanup, not the product's general
process-ownership guarantee. Windows execution is refused before any source or evidence write;
portable validation/restoration tests still run there. Product platform support is unchanged.
No taskkill, job supervisor or native-platform test result is inferred from that refusal.

Handled failures, deadlines and interrupts restore only a known original or known edited state.
An unexpected concurrent change or symlink is preserved and reported with recovery data retained;
stop other writers rather than treating this optimistic check as a checkout lock. SIGKILL or power
loss cannot run cleanup. Before manual recovery, stop and join any remaining owned test processes,
inspect `recovery.json`, compare current SHA-256 and mode with the recorded original/edited states,
then restore the corresponding backup bytes and recorded permission bits only if still owned.
Keep unexpected work and recover by review instead of overwriting it. Evidence is never deleted
automatically. Pytest autoload suppression remains confined to controlled plugin-free fixtures;
this tool changes neither product environments nor dependency caches or nightly policy.

## Tags

| Tag               | Meaning                                                                              |
| ----------------- | ------------------------------------------------------------------------------------ |
| `@phase-<n>`      | The phase a scenario belongs to.                                                     |
| `@<item>`         | The work item that turns it green (`@count-cli`).                                     |
| `@ID-<AREA>-<nn>` | Stable scenario ID. Commits reference these. Never reuse or renumber them.           |
| `@bug-<n>`        | Reproduces a fixed bug. Added by `fix:` commits.                                     |
| `@wip`            | Written, not yet implemented. Excluded from every run, and a `feat` may not name it. |

**IDs.** `<AREA>` is one word in capitals for the area of behaviour, and one feature file holds
one area: `COUNT` is `count.feature`, `CLI` is `cli.feature`, `COMPL` is `completion.feature`.
`<nn>` counts up within the area from `01`; a removed scenario's number is not given again. A new
area gets a new word and a new file.

## Commit rules

- `feat:` must add or change scenarios, or reference `@wip` ones it turns green. Footer:
  `Scenarios: ID-COUNT-01` (`itos commit --scenarios`).
- `fix:` must add a `@bug-<n>` scenario that failed before the fix, or reference an existing
  scenario that was failing. Same footer.
- The commit-msg hook checks that the referenced IDs exist and are live at the commit.
- Red first: a slice's steps go alone in a `test` commit, its scenarios still `@wip`
  (`itos commit --item count-cli`).

## The smoke set

The smoke set is the list in `smoke.yaml`: the scenarios every push runs, beside the ones its
commits name. The rule, which `itos tests smoke check scenario` checks:

- **Every feature file with a live scenario has at least one smoke scenario**, listed under the
  file with the reason it was chosen. A file has more only when the list says why (`more`).
- Every ID in the list is a live scenario of the file it is listed under.
- A smoke scenario is fast and central to its file.

So a `feat` that adds a feature file, or makes a `@wip` one live, picks its smoke scenario in the
same commit.

## Writing a scenario

- **Give a check its own wording.** godog matches a step's text whatever its keyword, so a `Then`
  phrased like an existing `Given` runs the setter and cannot fail.
- **A step is one line**, however long.
- **Assert what a user reads**: the exit code, a sentence of the output. Never more of the output
  than the behaviour is about.
- **Never start a description line with `@`.** Gherkin reads it as tags, and godog then refuses
  the file, failing every run of the features.
- **Say why beside the scenario** when a setup would make a reader ask: a comment above the tag
  line.

## The steps this project's scenarios use

These are the ones the harness must define. Each is a regular expression over the step's text,
with the program named as the built binary is.

| Step                                                                                  | What it asserts                                                  |
| ------------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| `itos-cc runs with "…"`                                                       | Runs the binary in the scratch folder with those words.          |
| `itos-cc runs with "…" and the environment "NAME=value"`                     | The same, with that variable added to the clean environment.     |
| `itos-cc runs with "…" and its standard input left open`                     | Starts the binary, its stdin a pipe held open, and does not wait. |
| `it is interrupted while it waits`                                                    | Interrupts that run once it reads its stdin; waits for it to stop. |
| `it exits with code <n>`                                                              | The binary's exit status.                                         |
| `its standard output says "…"` / `does not say "…"` / `lists "…"` / `does not list "…"` | Containment, or one whole line equal to the text.                |
| `its standard output is empty`                                                        | Nothing at all on stdout.                                        |
| `its error output says "…"` / `does not say "…"`                                      | The same, on stderr.                                              |
| `the first line of its standard output is "…" and the stamped version`                | The stamped version the harness built with.                      |
| `the second line of its standard output is "…" and the stamped commit`                | The stamped commit.                                               |
| `the last line of its standard output is "…"`                                          | The completion protocol's instruction line.                       |
| `its JSON output names the problem "…"`                                               | That the object lists a problem with that rule.                  |
| `its JSON output gives file "…", <n> lines and <n> words`                             | The success object's file, lines and words.                      |
| `its JSON output gives each problem a rule, a message and a fix`                      | That no problem is missing one of the three.                     |
| `its JSON output gives the stamped version and commit`                                | version's object: schema 1, ok, the stamped version and commit.  |
| `its standard output is one line`                                                     | Exactly one line, as `--json` prints its object.                 |
| `the file "…" contains "…", "…"`                                                      | Each text is in the file.                                         |
| `the file "…" holds a line of <n> characters`                                         | A file written for the scenario, long enough to beat a token limit. |
| `standard input holds "…"`                                                            | The text the run reads from its standard input.                   |
| `an empty folder "…"`                                                                 | A folder in the scratch directory.                                |
| `I run "itos-cc …"`, also `… without options`                                         | Runs the binary in the scratch folder with those words.          |
| `stdout is one object with "schema": 1, "ok", "…", and "…"`                           | One JSON value, an object with those keys, none of them null.    |
| `stdout shows:` with its text                                                         | stdout is the text exactly, a path's separators read as slashes. |
| `stderr says "…"`                                                                     | Containment, on stderr, in each run the When made; the text is everything between the first and last quote, `\"` a quote; `<dir>` in it the folder of the Given's project as itos-cc names it, `...` or `…` the rest of what it says there. |
| `the problem is "…"`, `…, with flag "…"`, `with argument "…"`, `with key "…"`, `with file "…"` | A problem of that rule, with that subject, in the run made again with `--json` before its words, which exits as the plain one; the plain run's stderr prints its message. |
| `the exit code is <n>`                                                                | The binary's exit status, of each run the When made.              |
| `"…" is an empty list` / `"…" and "…" are empty lists, not null`                      | `[]` in the JSON object, never `null`.                            |

`units.feature`'s steps, in `units.go`:

| Step                                                                                  | What it asserts                                                  |
| ------------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| `I run "itos-cc …" from that directory`                                               | The same, in the folder a Given made outside every project.      |
| `stdout lists each unit as "file:start-end  namespace#name  kind"`                    | Units are listed, each line of that form.                        |
| `each unit has …`                                                                     | Each listed unit has every key named.                             |
| `only units from test files are listed`                                               | Units are listed, each in a test file of the sample project.     |
| `the selection holds no supported files`                                              | The scratch folder holds only files of no supported language.    |
| `the file "…":` with its text, `… with a component that returns JSX`, `… defining "…"` | Writes the file a scenario lists.                                 |
| `its units are listed` / `the units of … are listed`                                  | Runs `units --json` over that file.                               |
| `the units are:` with a table                                                         | Exactly the table's units, in order: kind, namespace, name, lines, private. |
| `the unit is "…"` / `the component "…" is a unit`                                     | One unit, that one; or that one among them.                       |
| `nested arrow functions, callbacks, and abstract methods are not units`               | No unit starts on a line of the file declaring one the listing leaves out. |
| `… on any object are routes`                                                          | Each method, called on an object of its own with a path, is a route unit. |
| `a route's complexity, mutation sites, duplication, coverage, and hash are its own, not mount's` | A branch added to a mounted route changes its crap complexity, mutation list sites, dry nodes and snapshot hash, and none of mount's; mount is wholly covered though its routes' bodies never ran. |
| `the line a route starts on is mount's, since mounting runs it`                       | That line run leaves the route uncovered; not run, mount is not wholly covered. |
| `test files have no route units`                                                      | The same file as a test file lists units, none a route.          |
| `a Kotlin class annotated with both @Configuration and @EnableWebSecurity` / `its method is a unit of that class` | The class's method is a method unit of the class.     |
| `the Go module "…" with package "…"`                                                  | Writes its go.mod and the package's board.go.                    |
| `x.ts declares function place, in a directory with no package.json, go.mod or .git in it or above it` | A folder outside every project; skipped where a marker is above the temporary directory. |
| `it is "…" from a copy of that directory at another path`                             | The same command in a copy lists that unit alone.                 |

`dry.feature`'s steps, in `dry.go`. "Every pair" is what `dry --json --threshold 0
--min-lines 1 --min-nodes 1` reports, and "the pairs reported" are those of the scenario's run, from
its object, or from the `.metrics/dry.json` it wrote, which its text output must list. A step about
every pair first finds in its fixture the pairs that make the sentence worth checking, and fails
when there are none.

| Step                                                                                  | What it asserts                                                  |
| ------------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| `the <language> functions:` with their text, `the TypeScript function "…"`            | Writes them to one file of that language.                        |
| `a copy of it with <change>`                                                          | Adds a copy, a called function or the returned value changed as the table words it. |
| `the two functions are compared directly`                                             | Every pair of the file, which must be one.                       |
| `their similarity is <n>` / `is above <a> and below <b>` / `is at most <n>`           | That pair's score, to two decimals where written so.             |
| `pairs scoring at least <t> are reported`                                             | Exactly those of the pairs the run's sizes admit (`--threshold 0`). |
| `functions shorter than <l> lines or with fewer than <n> normalized nodes are not candidates` | The functions admitted are exactly those of l lines and n nodes or more. |
| `only pairs scoring at least <t> among functions of <l>+ lines and <n>+ nodes are reported` | Exactly those of every pair.                                 |
| The Givens naming files, as `invoice.py renders an invoice's lines and total` or `two functions that score 0.95 in TypeScript` | Write the files the sentence describes; the ones a score or range is printed of are tuned to it. |
| `exactly one pair is reported: <file>#<name> and <file>#<name>` / `its score is at least <a> but below <b>` | The one pair and its score.             |
| `they are not compared`                                                               | Each file has units, and every pair is none.                     |
| `they are not reported`                                                               | No pair of the two is reported; they score 1.                    |
| `the pair from <a> and <b> is reported` / `is not reported`                           | A pair of the two files; or none, though the whole project's run reports one. |
| `only pairs with at least one function in a changed file are reported`                | Each pair reported touches the sample's changed file.            |
| `those functions are compared against every source in the project`                    | The whole project's pairs touching it, one with an unchanged file. |
| `pairs are listed best first` / `groups are listed best first, then largest first`    | The same command over groups of their own: the order of entries and pairs. |
| `stdout shows one entry, not <n> pairs:` with its text                                | stdout is the text; the run reported that many pairs.            |
| `a group holds every function linked to another member by a pair at or above the threshold` | The same command over functions of their own: each entry is a set of functions joined by pairs at or above the threshold. |
| `.metrics/dry.json holds the version, the threshold, the candidates, and the groups`   | A version, and stdout's threshold, candidates and groups.        |
| `each candidate has …, and left and right sides with …` / `each group has …, and its members` | Each has every key named, `lines` the start and end lines. |

`scrap.feature`'s steps, in `scrap.go`. A measurement is what `scrap --json --verbose` reports
of the scenario's files, and "the file's report" is that of the scenario's run, from its object, or
from the `.metrics/scrap.json` it wrote, of which its text output must print each file's action,
pressure, examples and scores. Where a Given states what its fixture holds (counts, scores, a
similarity, a pressure recorded), the fixture is checked against what scrap measures.

| Step                                                                                  | What it asserts                                                  |
| ------------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| `it is measured` / `it is scored`                                                     | Measures the scenario's files; scored, the example must have the Given's counts. |
| `a <language> test file using <frameworks>` / `each test case is an example, grouped by its enclosing …` | Files of each framework; their examples exactly, each grouped by the constructs of the kinds named, joined by ` › `, and by no other. |
| `the <language> test file "…":` with its text                                         | Writes the file.                                                  |
| `the examples are:` with a table                                                      | Exactly the table's examples, in order: assertions, decisions, mocks, table. |
| `a <language> example containing <code>` / `it counts as an assertion`; `the call counts` or `does not count` | The one example's assertions are 1, or 0.        |
| `an example with <n> code lines, …` / `its score is <n>`                              | An example of exactly those counts, and its score.               |
| `an example with <measure>` / `it has the smell "…"`                                  | The example with the measure has the smell; one just short of it does not. |
| `its action is …`, `its pressure is …`, `it has no recommendations`                   | The file's report.                                                |
| `it has one "…" cluster of <n> examples`, `the first recommendation says to fold them into "…"` | The clusters of that kind; the first recommendation folds the table's examples, naming each. |
| `its first recommendation is <confidence> confidence: …`, `a <confidence> recommendation says to …`, `a recommendation says to …` | A recommendation holding the sentence's words, in order. |
| `HIGH recommendations come before MEDIUM ones, and MEDIUM before LOW` / `at most <n> recommendations are listed` / `each names the example and its line range` | The order; the cap, reached since the file calls for more; each quotes an example with its lines. |
| `each file is printed as "…"` / `files are listed from the highest pressure down` / `recommendations follow, indented, with their confidence` | The text output's lines. |
| `the hint is not printed` / `it is printed with --verbose`                            | A LEAVE_ALONE file's LOW hint, hidden, and printed with the flag. |
| `every example is printed with its …`                                                 | Each example's verbose line holds the measures named.            |
| `each file's report has …`                                                            | Each file of the object has the keys named.                      |
| `.metrics/scrap.json recorded a pressure of <p> for a file` / `I run "…" and its pressure is now <p>` / `its verdict is …` | Fixtures of those pressures, the first recorded by a run of scrap; the verdict in the line and in `--json`'s compare. |
| `its line ends with "…" and the previous pressure` / `a warning says to … before keeping the change` | The worse file's line and the warning under it.          |
| `.metrics/scrap.json holds reports for … and …` / `still holds …'s report`             | Both recorded, the second then changed on disk; its old report kept. |
| `.metrics/scrap.json is not valid JSON`, `stderr says it is ignoring the unreadable snapshot`, `the run completes` | The warning, and a report written. |
| `the selection holds no test files`                                                   | Only a production file and a README.                              |

`crap.feature`'s steps, in `crap.go`. "The functions the run lists" are those of its `--json`
object or its table, and a problem is read from the run made again with `--json` before its words,
as a script would, which must exit as the plain one did. A Given that runs `itos-cc crap` as
written over a project of its own writes one its tools can measure and declares them
(`Coverage`): Go modules, named `board`, so a function at the root is `board#name`; a Python and a
TypeScript project where a scenario names one. Where a Given states what its fixture scores, the
fixture is checked against what crap scores.

| Step                                                                                  | What it asserts                                                  |
| ------------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| `a function with complexity <cc> and <p>% coverage` / `it is scored` / `its CRAP score is <n>` | board.go's place, of cc-1 decisions, and a report run on p% of 20 of its lines; crap scores it through `--coverage-report`, at that complexity and coverage, as n to one decimal. |
| `a project with tests` / `each language's coverage command runs, with its output on stderr` | A Go module of three functions scoring 12, 2 and 1; stderr shows go test's command and its `ok` line, and stdout does not. |
| `stdout is a table with the columns …` / `each row names the function as "…"` / `rows are sorted from the highest CRAP score down` | The header's columns; each row's function, the format's words standing for what `.metrics/crap.json` records; scores, two or more different, falling, N/A rows last. |
| `a function that no coverage report applies to` / `its CRAP and COV% are shown as "…"` / `it is listed after every scored function, by complexity` | A Go module beside the Kotlin module coverage cannot measure; that module's rows show the text, come after every scored row, and the N/A rows fall in complexity. |
| `coverage measured other Python files in the project` / `the report never mentions …` / `the functions of … have <n>% coverage` | A Python project whose test imports src/board.py; the declared report and the tests never name the file; each of its listed functions at n%. |
| `a TypeScript project whose tests import … but never …, whose functions are simple` / `no "…" problem is reported` | A Vitest project, every function of complexity 1 as crap measures it; no problem of that rule. |
| `the coverage report mentions no <language> file` / `the <language> functions have no CRAP score` | A Go module beside the Kotlin module; `.metrics/crap.json` records functions of the language, none with coverage or a score. |
| `only the <n> highest-scoring functions are printed` / `.metrics/crap.json still records every function` | The rows are the n first of the snapshot's functions in crap's order, more than n recorded; the snapshot holds exactly the sample's functions. |
| `each entry has …` / `each unmeasured build root has …`                               | Each entry has every key named; each unmeasured root, one at least, gives each. |
| `.metrics/crap.json holds every scored function`                                       | Each row with a score, one at least, recorded with it.            |
| `a function scores <n>` / `it says so for every function above the threshold, each a "…" problem with …` | place, never run, of the complexity scoring n, beside a function above it; a problem of the rule, with the keys, for each function above `--threshold`, two at least, its message on stderr. |
| `every function scores <n> or less`                                                   | Go functions scoring n at most, one exactly n.                    |
| `a project whose …` / `a Go project whose tests do not compile` / `its functions show "…"` / `the problem is "…", with …` | A project in board/ or web/ as the row says, with `--use-existing-coverage` or `--coverage-report` added to the run where it names them; every function listed shows the text; a problem of the rule giving each key, its dir the project's. |
| `the selection holds no production source files`                                      | A Python test file and a README alone.                           |

`complexity.feature`'s steps, in `complexity.go`. A function's complexity is what
`crap --no-coverage --json` reports of the one file the scenario wrote, no test run.

| Step                                                                                  | What it asserts                                                  |
| ------------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| `a <language> function with no branches` / `the <language> function:` with its text  | Writes it to a file of the language; a Go function in package board. |
| `its complexity is measured` / `it is <n>`                                            | Its one function's complexity is n.                               |
| `it is <n> in a <.ext> file too`                                                      | The same text in a file of that extension, n too.                |
| `a Kotlin safe call a?.b counts like the elvis ?:`                                    | A function returning a?.length and one returning a ?: "x" each one more than one returning a. |

`coverage.feature`'s steps, in `coverage.go`. A coverage command is one itos-cc announces on
stderr, `itos-cc: coverage <dir>$ <command>`, its folder read from the scratch folder by its name,
as the system may name it through another path. A report a Given writes by hand stands for what a
tool writes, and what it gives a function is worked out from the pieces the step wrote: lines, Go
blocks of their statements or JaCoCo lines of their instructions, and the decisions it records.
Paths written from the root, `/p/…` or `/work/…`, are under the scratch folder; a run over sources
named so goes from their first folder. A placeholder stands for an install no pinned tool set has
(Jest, Vitest 4, Vitest without its provider, gradlew, a `.venv` python that cannot run), a file
itos-cc finds and cannot run.

| Step                                                                                  | What it asserts                                                  |
| ------------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| `coverage runs "…"` / `…, without <flag>`                                             | A coverage command is the text, `...` any words or any text in a word; with no word beginning with the flag. |
| `its coverage is the same as with the whole module's tests`                           | `crap --all-tests --json a/a.go`, over `./...`, gives a/a.go's functions what the run gave them. |
| `I run "itos-cc …" in a Vitest or Jest project` / `coverage runs <tool> <words>, or <tool> <words>` | A Vitest project measured by the pinned tools; a command whose program is either tool, the words following it. |
| `coverage runs the whole suite of each build root`                                    | One command in each build root of the sources named, every one go test over `./...`. |
| `a <language> project whose build root holds <marker>` and its setups / `coverage is run (for it)` / `the command is …` | The project at the scratch folder; `itos-cc crap`; its commands, joined by ", then ", each holding the words, `<pm>` the package manager, "the installed <tool>" one from a node_modules/.bin, "writing LCOV" an LCOV reporter, "read from <path>" that report there, giving what the script wrote. |
| `a TypeScript project in a workspace whose root declares "packageManager": "…"` / `<pm> is …` | packages/app with a coverage script, the manager off the PATH; the command running the script is the manager's. |
| `… with its tools installed at the workspace root` / `vitest, jest, or c8 runs from the nearest node_modules/.bin` / `npx is never used, …` | The pinned node_modules at the root alone; the tool's folder is the nearest .bin holding it; no command runs npx. |
| `a TypeScript project using …`, `a Python project whose python cannot import coverage`, `a Kotlin project built by Maven, …` / `nothing runs for it` / `stderr says what to install, or to use --coverage-command` / `only complexity is reported (for it)` | No command runs in the project; a line names its folder and the missing tool, saying to install or add it or to use --coverage-command; every function listed has a complexity and no coverage or score. |
| `a Python project with a .venv directory` / `the .venv's python runs the tests`       | python3 linked as .venv/bin/python; the command starting with it runs pytest or unittest, measuring place. |
| `a Kotlin Gradle module under a build root that holds gradlew` / `the gradlew wrapper is used instead of gradle` | The module's command is the root's gradlew; none is gradle. |
| `a repository with a Go module and a TypeScript package` / `each build root runs its own coverage command` / `the reports are merged into one` | api/ and web/; each runs its language's command; the one run gives a function of each coverage. |
| `a project whose tests fail but still write a coverage report` / `the failure is logged to stderr` / `the report is still used` | go test's FAIL and `itos-cc: coverage: go: exit status N`; Place has what the failing test ran. |
| `reports are written under .metrics/coverage/` / `.metrics/coverage/.gitignore ignores everything in it` | Each command's report is in .metrics/coverage/; git ignores what is in it, and not .metrics/crap.json. |
| `a Go project with a coverage.out from an earlier run` / `no tests are run` / `coverage is read from the first existing report for each build root` | Two modules, coverage.out before cover.out; no command and no test's mark; each function has its first report's coverage. |
| `both reports are read, relative to the working directory` / `"…" runs through the platform shell` / `cover.out is read afterwards` | Each named report there gives its function its coverage; the command announced as a shell line and make's recipe echoed; cover.out, not in the sample, written and read. |
| `a report file "…"` / `it is loaded` / `it is read as …` / `it has one entry for "…"` | A report of the format its name is usual for; crap --json over it; written in that format, its function given what it means; its one entry names the source, the one file scored. |
| `a <format> report` / `the covered share of … is computed` / `each measured piece is weighted by …` / `a decision is …` | A function's pieces of different weights; crap --json over the reports written; the share by that weight; probe reports showing what decides. |
| `a coverage.py LCOV report for:` / `the tests call pick(x) once` / `it is <n>%, <n> of its <n> branches` | pick.py and the LCOV coverage.py writes for that call; the decision's branches and the share. |
| `a function whose lines hold no decision in the report` / `its coverage is the share of its lines or statements that ran` | LCOV and a Go profile with no decision; each function's share of its lines or statements. |
| `an LCOV report from c8, …` / `it lists the function body as a block and leaves out the arm that ran` / `those one-branch blocks are ignored and its lines decide` | One-branch BRDA blocks; the share of the lines. |
| The matching Givens / `it is matched to sources` / `it matches that source …`, `the paths still match` / `it matches neither source` | Sources and entries as written, a run of crap --json over a report of the entries for each case; each source has its entry's coverage, or none. |
| `a report entry that matches no selected source` / `the report is built` / `that entry is ignored` | A test file's and a dependency's entries beside board.go's; the run lists board.go alone, with its coverage, and says nothing of them. |
| `a module with packages …` / `a profile from go test -coverpkg=./..., …` / `it is <n>%, as go tool cover -func says` | Every block once per test binary; the share go tool cover's rule gives, and crap's. |
| `two reports given with --coverage-report, …` / `each covers a line of a file the other does not` / `a line or block is covered when any report covered it` / `a decision counts the branches of the report that took the most` | F has the union of the lines, more than either; G the branches of the report that took most, fewer than both. |
| `its coverage starts at line <n>, not at its first line <n>`                          | The function `the <language> function:` wrote starts on that line; a report running that line alone of all of its lines gives it one line's share of those from there. |
| `stderr has a line "…" naming the command coverage runs` / `no line of stderr begins "…"` / `stderr has a line beginning "…" that gives the command's exit status` | The line, `<dir>` the build root of the sources named, its command's output after it; none so; one so ending in `exit status N`. |

`file_selection.feature`'s steps, in `selection.go`. What a command selects is what units lists,
its production code and with `--tests` its test code, for a command run with no paths or with
`--changed` and for "the project's files are discovered", or the production files a run of crap
as written lists. A file a Given names holds one function of its language; files are named from
the folder the run ran in, as itos-cc names them.

| Step                                                                                  | What it asserts                                                  |
| ------------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| `a project with TypeScript, Python, Kotlin, and Go sources`                           | Writes the Background project; go test measures its module.       |
| `I run a command with no paths` / `with --changed` / `every supported file under the working directory is selected` | units over the scratch folder; each source and test of the project listed as its kind. |
| `I run "itos-cc …" in <folder>` / `I run "itos-cc …", "itos-cc …" or "itos-cc …"`    | The run in that folder; or each run, each held to the exit code and stderr steps. |
| `<files> are selected` / `<file> is not selected`                                     | Exactly the files listed, the production files under a folder and the one named, or every file staged; the file not selected, named from the run's folder or the scratch folder. |
| `there is no file or directory called "…"` / `every source whose path contains "…" is selected, such as …` | Nothing called so; exactly the project's production files with the text in their path, the example among them. |
| `no file's path under the working directory contains "…"`                              | No file the project holds has it in its path.                    |
| `git reports … as modified, … as staged, and … as untracked`, `git reports … as modified`, `… as untracked`, `… was deleted`, `git reports no changes` | A repository whose one commit holds the project, each file at its first text, then changed as said. |
| `a git repository with staged files and no commits`                                   | Every source and test of the project staged, nothing committed.  |
| `the working directory is not a git repository`                                       | No .git in the scratch folder or above; skipped where one is.    |
| `the project contains these directories, …:` / `nothing inside those directories is selected` | A TypeScript and a Python source and a TypeScript test in each; units lists files, none in them. |
| `a directory "…" holding source files` / `its files are selected` / `not selected`    | TypeScript, Python and Go sources there, beside each manifest the row names in turn, a .gitignore git reads as ignoring it, or the Background's go.mod alone; units lists them all, or none, each time. |
| `… is analyzed`                                                                        | The run lists a unit of the file.                                 |
| `the files …` / `each is selected as TypeScript and parsed with the JavaScript grammar` / `neither is selected` | Each a function holding JSX and a decision; each listed as TypeScript production code, and crap gives its function complexity 2, as the JavaScript grammar parses it (TypeScript's loses the decision); or neither listed, though units lists other files. |
| `the file "…" is discovered` / `it is classified as …`                                | units over that file alone lists it as that kind and not the other. |
| `a command reports on a file inside the working directory` / `the file is named by its path relative to the working directory` / `a file outside the working directory keeps its absolute path` | units --json in src over cart/total.ts and ../kt/src/main/kotlin/Shelf.kt; the first named so, the second by an absolute path, the scratch folder found in it by its name. |
| `itos-cc.yaml holds '…'` / `the sources … and …` / `… is scored, and … is not`        | That itos-cc.yaml; the two files; crap lists the first and not the second. |

`cli.feature`'s steps, in `cli.go`. A scenario that only says When I run runs in an empty folder,
or in a project of its own: a Go module, `src/` of eight functions and `lib/billing/` of one
(ID-CLI-05); a folder called `--tests` (06); a Go module go test measures (09); a Vitest project
whose `src/board.ts` has 17 mutation sites, run once with mutation run and then changed by a
comment in two functions (13, which needs Node); `x.ts`, of two sites, in a project whose test
script would leave `test-ran` (14).

| Step                                                                                  | What it asserts                                                  |
| ------------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| `the usage text is printed to stdout` / `it lists the commands …`                     | The top-level help; each command named begins a line of its Commands section. |
| `the command's usage, options, --json shape, problem rules, and exit codes are printed to stdout` / `it ends with examples and the address for issue reports` | The usage names the command the words name, then those sections in order, exit 0; examples after the exit codes, and a last line giving the address. |
| `the binary was built …` / `the first line of stdout is "…"`                          | That build (`Build`), which every run then runs; the line, a checkout's `<next>` the release after its tag, `<time>` fourteen digits and `<commit>` its commit's first twelve characters. |
| `it is the same as "itos-cc …"`                                                       | The run, no usage error, and that command give the same exit code and stdout, and stderr but for its numbers. |
| `"…" is a path, not the flag`                                                         | Every unit listed is under that folder.                          |
| `<situation>` / `the command ends`                                                    | A command for each thing the situation names, each in a folder of its own, each held to the exit code step. |
| `stdout is one object with "schema": 1, "ok": true, and the command's own keys` / `progress and test output go to stderr` | One object, schema 1 and ok, holding each key the command's help gives its --json object; the coverage command and go test's lines on stderr alone. |
| `stdout is:` with its text                                                            | stdout is the text exactly.                                      |
| `a run has a problem` / `stderr says "itos-cc: <message>. <fix>"`                     | crap over a path that is not there; stderr says so of each problem of the run made with --json. |
| `stdout names the subcommands … and …` / `stderr names the subcommands … and …`       | Each on a line of its own; each in a line of stderr naming the subcommands. |
| `stdout says "…"` / `each survivor is a "…" problem`                                  | Containment; as many problems of the rule, made with --json, as the line says survived. |
| `stdout lists each mutation site of … with its line, column, function, original, and replacement` / `no test command runs` | Exactly the file's sites, each line in that form; no `test-ran`, no `.metrics` and no baseline. |

`mutate.feature`'s steps, in `mutate.go`, for the scenarios not in `Unstepped`. A site is read as
`mutation list` prints it, `file:line:column` and its texts, or from `--json`; what a run judged,
from its summary on stdout and the line on stderr for each mutant it ran. The Go modules are
`example.com/m`; ID-MUT-06 runs in a Python project whose test leaves `test-ran` if anything runs
it. A Given whose run runs a project's tests declares them (`Tests`) and, for Go, what they cover
(`Coverage`); one about a counted run skips where itos-cc makes none (`Counted`).

| Step                                                                                  | What it asserts                                                  |
| ------------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| `the sites are:` with a table                                                          | The run, a mutation list of one file, listed exactly the table's sites in order: line, original, replacement, `(deleted)` for nothing. |
| `code outside functions, comments, strings, and type arguments hold no sites`          | The file holds each of the four with an operator, 0, 1 or boolean in it, and no site listed is in one. |
| `a?.() becomes a(), as a?.[0] becomes a[0]`                                           | chains.ts's two optional chains each have one `?.` site, which leaves `a()` and `a[0]`. |
| `.js, .jsx, .mjs, and .cjs files have the same sites`                                 | A copy under each extension named lists the same sites, places and functions. |
| `each site is printed as "…"` / `no tests are run`                                    | Every line is a site of src/board.py in the format, its words standing for the site's, all four in order; no `test-ran`, no `.metrics` and no baseline. |
| `src/app.ts has a function with a site before, inside and after an inline callback` / `its sites are listed in line and column order` / `in the order "itos-cc …" lists the file's "…"` | routes' sites on lines 2 and 7, the callback's on line 4, its tests passing every mutant; the run listed all of them by line and column; that command's object lists the file's mutants the same. |
| `src/setup.go declares two init functions …` / `a previous run recorded both …`         | Two init functions of different bodies, each with a site; mutation run killed every mutant. |
| `I run "itos-cc …" with nothing changed` / `the mutants of both init functions are reused, and none runs` | src/setup.go as the previous run left it; ran 0, reused every site of both, no mutant's line. |
| `the second init function has changed since` / `the two have swapped places since, unchanged` | Its body changed; or the two swapped. |
| `the only problem is "…", for the second init function` / `"itos-cc …" runs only the second init function's mutants` / `"itos-cc …" reuses every mutant` | One problem, of the rule, naming the function and its line; every mutant of the second ran and none other; ran 0 and every one reused. |
| `itos-cc.yaml excludes "…"` / `the only commit since main changes …, whose mutants no test reaches` | That itos-cc.yaml; the module, its results recorded on main, then a branch whose one commit changes the fake. |
| `stderr says "…", and the exit code is <n>` / `"itos-cc …" exits <n>`                  | Both of the run; that run's exit code. |
| `neither "itos-cc …" nor "itos-cc …" names a site in …`                               | Each names a site, as `file:line` before its text, and never the file. |
| `a snapshot under .metrics/mutate for …, and fresh results for every other source` / `itos-cc.yaml then excludes "…"` | A run in a git repository recorded every source, the fake's among them, and check passes; then that itos-cc.yaml. |
| `its "…" list … with "…" "…", and the exit code is <n>` / `the snapshot is removed, and its "…" list it with "…" "…"` | The object's list names the file with that value; the Given's snapshot is gone, and the list names its source. |
| `itos-cc.yaml excepts a survivor in … and excludes "…"` / `it reports "…" with "…" "…"` | An exception of the fake's line 7, column 9, beside the exclude; a problem of the rule with that value. |
| `the itos-cc.yaml HEAD commits excludes "…"` / `no selected site is in …`             | Committed so, the working tree's excluding nothing, where mutation list lists the fake; the run selected sites, none in the file. |

The listed-test mutation scenarios (ID-MUT-121 through 137) use a real Go module with two
listed IDs. Its harness builds a covered binary, records selections and the source hash,
and either splits coverage by ID or allows the public fallback to run each ID separately.
These fixtures pass `--no-annotate` on mutation runs to keep the recorded source hashes stable
and use two workers unless the scenario supplies its own worker flag.
`Tests.Listed` declares the inventory, line coverage, selection durations and failing,
surviving or timing-out outcomes for the future memory driver. Assertions read the snapshot,
command progress and harness records. ID-MUT-134 uses `Driver.Graph`: the binary driver starts
`serve`, waits for its readiness message, requests `GET /api/graph`, and stops its child within
a bounded cleanup. It never calls graph internals; the memory implementation belongs with
S8's serving adapter.

T-101 removed the old holders in `cmd/itos-cc/mutate_listed_test.go` (121–129),
`mutate_listed_fresh_test.go` (130–134) and `mutate_listed_baseline_test.go` (135–137) once the
converted scenarios ran green through both drivers (134 through the binary alone, deferred with
serve): each assertion is its scenario's. Their harness evidence of a run selecting ID-A-02 is
ID-MUT-136's record of that selection's own run, and a functions entry's stale state goes with
its `mutation.stale` problem (ID-MUT-64). The three files keep the module and helpers the counted
tests share; the helpers only the removed tests used went with them.


The differential/range scenarios (ID-MUT-23–27, 37–46, 102–104) use Go for generic
mutation behavior and renames, and real Vitest projects for sentences naming `.ts` files.
The rename fixture declares `Board#Place` with that exact spelling. Each fixture declares
its coverage and test outcomes. The pinned Node CI selection runs 37–41, 46 and 102.

`Driver.ObserveWrites` observes actual writes separately from bytes: the disk driver dates
existing files in the past and checks their modification times, and the memory driver checks
write revisions. Their shared contract includes rewriting identical bytes. Memory snapshots
increment revisions through the actual fake Store.Write, before writeback; the source annotation
writers move in S7 and must likewise record their writes through the memory driver's Write or
an instrumented source port in T-125. A byte-only comparison cannot stand in for those events.
ID-MUT-102 checks both the observed writes and the bytes of source and snapshot.

T-101 removed T-73's thirteen annotated tests in `cmd/itos-cc/mutate_since_test.go`, for
37–46 and 102–104, once the scenarios ran green through both drivers. Each assertion is its
scenario's, with these held elsewhere: the snapshot recording no entry for a function never
judged (37's comment) is `internal/mutants`' `TestFunctionsNotJudgedKeepTheirRecord`; a
`--since` run recording its judged function's kill is 104's; an empty `judged` under `--json`
is 103's; and the first runs' survivor exit 1 is ID-MUT-29's. The file keeps the shared
module helpers and the untagged renamed-file/new-path guard, which no scenario reaches; the
helpers only the removed tests used went with them. The differential guards in
`internal/mutants/snapshot_test.go` stay as domain tests.

The cached-result scenarios (ID-MUT-57–70, 106–108) use real Vitest fixtures for every
literal TypeScript path, and a Go module with an importing package for 70. Their
`Driver.Tests` and `Driver.Coverage` declarations name the fixture's actual outcomes;
the Node CI selection runs 57–69 and 106–108. The deleted-test fixture keeps a second
importing test so coverage still measures the source after `board.test.ts` is removed.
The survivor in 60 is in `Board#place` as written. Missing-site fixtures have several
place sites and remove one recorded mutant from raw snapshot JSON. Test hashes are
checked against the SHA-256 of the actual test bytes, without product internals.

The quiet-check assertion observes public coverage/baseline/trial progress, the real
fixture's marker absence, unchanged snapshot bytes and no new coverage artifacts.
It does not declare that memory Workers write that marker. Keep 57's stronger legacy
marker positive control until T-101 reviews its replacement; it is not established
by an in-process scenario yet. The scenarios also retain the moved-function,
moved-survivor and no-snapshot guards. The uncovered-mutant assertion in 61 permits
the additional current TypeScript coverage-inventory problems while requiring the
uncovered mutant's complete subject and exit 1.

T-101 reviewed T-74's holders once the scenarios ran green through both drivers, and removed
each whose every assertion a scenario or domain test holds. Three stay in
`mutation_check_test.go`, each for an assertion no scenario makes: 57's marker, first shown to
be written when anything runs the tests, so its absence proves check ran none (the scenario's
marker has no positive control, and the memory's Workers write none); 59's missing problem's
line; and 60's plain listing of a survivor by check. 58's stale problem's line is ID-MUT-115's,
and the fixtures' survivor exit 1 ID-MUT-29's.

| ID | Holder | T-101 |
| --- | --- | --- |
| 57 | `mutation_check_test.go`: `TestFreshResultsWithEveryMutantKilledPass` | kept |
| 58 | `mutation_check_test.go`: `TestAFunctionChangedSinceItsResultsIsStale` | removed |
| 59 | `mutation_check_test.go`: `TestAFunctionWithNoResultsIsMissing` | kept |
| 60 | `mutation_check_test.go`: `TestARecordedSurvivorFailsTheCheck` | kept |
| 61 | `mutation_check_test.go`: `TestARecordedUncoveredMutantFailsOnlyWithFailUncovered` | removed |
| 62 | `mutation_check_test.go`: `TestWithSinceOnlyTheFunctionsTheRangeChangedAreChecked` | removed |
| 63 | `mutation_check_test.go`: `TestAFunctionWithNoMutationSiteNeedsNoResults` | removed |
| 64 | `mutation_check_test.go`: `TestEachFunctionsStateAsJSON` | removed |
| 65 | `mutation_test_hash_test.go`: `TestTheSnapshotRecordsTheTestsThatImportTheFile` | removed |
| 66 | `mutation_test_hash_test.go`: `TestChangingATestThatImportsTheFileRerunsItsKills` | removed |
| 67 | `mutation_test_hash_test.go`: `TestDeletingATestThatImportsTheFileRerunsItsKills` | removed |
| 68 | `mutation_test_hash_test.go`: `TestMutationCheckCallsResultsStaleWhenTheirTestsChanged` | removed |
| 69 | `mutation_test_hash_test.go`: `TestASnapshotThatRecordsNoTestsIsStale` | removed |
| 70 | `mutation_test_hash_test.go`: `TestAGoFilesTestsAreItsPackagesAndThoseOfPackagesThatImportIt` | removed |
| 106 | `mutation_unrecorded_test.go`: `TestASiteTheEntryNeverRecordedMakesTheFunctionStale` | removed |
| 107 | `mutation_unrecorded_test.go`: `TestMutationRunRunsOnlyTheSitesTheEntryNeverRecorded` | removed |
| 108 | `mutation_since_stale_test.go`: `TestSinceKeepsTheEntriesItDoesNotJudgeMarkedStaleWhenTheirTestsChanged` | removed |


The sampling/scoped scenarios (ID-MUT-71–84, 109–113, 147–148) execute 23 cases
and 100 steps. Literal TypeScript paths use real Vitest tests and the literal
`make test` uses a fixture Makefile. Generic fifty-mutant cases use real Go sources
with a declared always-passing `go version` command. The pinned Node selection includes
71–75, 83–84, 109–113 and 147–148 (16 expanded cases); the other seven cases use Go
or require no test tool.

`Driver.Tests` upserts by directory and command; another declaration of the same
command replaces its fixture outcomes. Relative and absolute executable forms used
by the public run/sample commands are declared explicitly. Fixed external control
paths are embedded before fingerprints are recorded, so changing the control changes
real test behavior without making those test bytes stale or widening memory environment
support. The end-to-end fixture first demonstrates own tests miss the boundary mutant,
then the actual whole suite kills it. Scope comparisons force new trials with
`--mutate-all`, preserving the separate reuse-keeps-scope claim. MUT112 observes
trace events emitted by the real Makefile before Vitest, checking the original sample
before a diagnostic JSON rerun. MUT113 first proves an unrelated test is discovered
and breaks an explicit whole-suite sample, then samples the unchanged snapshot using
its missing-scope own-test fallback. Only an external control changes between these runs.

`Tests.Trace` optionally identifies an external JSONL file and source. The real fixture
records canonical `cwd` and `original_root` with `realpathSync`, plus
`source_sha256`. MUT71 checks the unmutated baseline first, every independently
computed sampled-site digest afterward, and every execution outside the canonical
original project. It also observes source/snapshot bytes and actual write events.
Public progress alone is not physical-copy evidence. Memory currently stores these
declarations only; T-88/T-125 must emit observations from actual bound fake Workers
state, select outcomes by its request command and retain a binding regression.
No scripted trace or in-process execution pass is claimed before that wiring.

`Driver.Head(ctx, dir)` observes the actual Git HEAD independently of sample output.
The binary implementation uses Git; the current memory Git has only a commit counter
and explicitly reports unmoved history until T-91 supplies its actual history ID.
T-125 must wire that value when mutation scenarios join `InProcess`.
MUT78 explicitly records seed `4813e48` before moving HEAD.

T-101 reviewed T-75's holders once the scenarios ran green through both drivers. Four stay,
each for an assertion no scenario makes: 77's `--json` seed being the HEAD commit's id (the
scenario reads the plain output's), 84's exact set of a sampled mutant's eight keys (the scenario
asserts each is there), 109's own-tests outcome leaving `scope` out of the snapshot (the
scenario reads a missing scope and `own` alike) and 110's reused scope in `mutation run --json`
(the scenario reads the snapshot's). Elsewhere: a new HEAD drawing another sample (78's
precondition) is the slice's `TestSampleSeedsFromTheBoundHistoryOnlyWhenOmitted` with the
domain's `TestTheDrawDependsOnTheSeedAndTheMutantAlone`; a mismatch's file, function and exit 1
(147, 148) are ID-MUT-72's and 73's; a sample with no mismatch exiting 0 (111) is 71's and 74's;
and the fixtures' survivor exit 1 is ID-MUT-29's. `TestAMismatchsFixRerunsInItsScope` lives in
`internal/mutation/sample_test.go`.

| ID | Holder | T-101 |
| --- | --- | --- |
| 71 | `mutation_sample_test.go`: `TestASampleWhoseOutcomesHoldPassesAndWritesNothing` | removed |
| 72 | `mutation_sample_test.go`: `TestARecordedKillThatNowSurvivesIsAMismatch` | removed |
| 73 | `mutation_sample_test.go`: `TestARecordedSurvivorThatIsNowKilledIsAMismatchToo` | removed |
| 74 | `mutation_sample_test.go`: `TestKilledAndTimeoutAgree` | removed |
| 75 | `mutation_sample_test.go`: `TestOnlyFreshMutantsThatRanAreSampled` | removed |
| 76 | `mutation_sample_test.go`: `TestCountSaysHowMany20ByDefault` | removed |
| 77 | `mutation_sample_test.go`: `TestARerunOfOneCommitSamplesTheSameMutants` | kept |
| 78 | `mutation_sample_test.go`: `TestSeedReproducesARun` | removed |
| 79 | `mutation_sample_test.go`: `TestOutsideAGitRepositoryTheSeedMustBeGiven` | removed |
| 80 | `mutation_sample_test.go`: `TestACountBelow1IsAUsageError` | removed |
| 81 | `mutation_sample_test.go`: `TestWithSinceOnlyTheFunctionsTheRangeChangedAreSampled` | removed |
| 82 | `mutation_sample_test.go`: `TestNothingToSample` | removed |
| 83 | `mutation_sample_test.go`: `TestAFailingBaselineInASample` | removed |
| 84 | `mutation_sample_test.go`: `TestTheSampleAsJSON` | kept |
| 109 | `mutation_scope_test.go`: `TestEachRecordedOutcomeKeepsTheScopeOfTheTestsThatDecidedIt` | kept |
| 110 | `mutation_scope_test.go`: `TestAReusedOutcomeKeepsItsScope` | kept |
| 111 | `mutation_scope_test.go`: `TestMutationSampleRerunsEachMutantWithItsRecordedScope` | removed |
| 112 | `mutation_scope_test.go`: `TestAScopeGivenToMutationSampleOverridesTheRecordedOne` | removed |
| 113 | `mutation_scope_test.go`: `TestAnOutcomeWithNoRecordedScopeWasDecidedByTheFilesOwnTests` | removed |
| 147 | `mutation_mismatch_fix_test.go`: `TestAMismatchsFixRerunsTheMutantInTheScopeItWasSampledWith` | removed |
| 148 | `mutation_mismatch_fix_test.go`: `TestAScopeGivenToSampleIsTheOneTheFixNames` | removed |


T-76's result/annotation/cleanup scenarios (ID-MUT-28–36, 52–56, 145 and
231–241) execute 28 cases and 107 steps through the public commands. TypeScript
sentences use actual Vitest
projects; annotation sentences use the exact Python source and real `assert a(1)`
tests, whose comparison survives and numeric-literal mutation is killed. The summary
fixture has seventeen real sites, eight cached kills, seven executions and two
uncovered sites. Its exact `board#place` survivor remains at line 5, column 9.
The original MUT28 `ran 9` was corrected to `ran 7` by the coordinator's separate
specification commit after the public fixture demonstrated the discrepancy; no
product counting behavior changed.

The unaffected pinned language-tools CI selection includes the twenty-five T-76
cases needing Node or Python. MUT233 is permanently binary-only because it observes
the actual parallel coverage collector and raw-data cleanup. Its cleanup check permits
only `.metrics/coverage/<plan>/.coverage` retained beside the durable report; raw
parallel `.coverage.*` files and data elsewhere must be absent. MUT234 is permanently
binary-only because it observes a real OS lock in an isolated temporary directory;
the marker is written before locking and its owned lock is released before cleanup,
even after failure. Neither tag stands for unmoved code. Counted listed outcomes in
MUT232 retain normal `Driver.Counted` platform eligibility and use one controlled
public site and a fixed seed, without domain candidate-selection APIs.

`Driver.Files(ctx, dir)` inventories actual regular files as sorted relative slash
paths, without following directory links. Its shared contract runs for disk and
memory; a real symlink boundary guard remains at the binary driver. `Driver.Lock`
uses the already-pinned x/sys calls in platform leaves and owns only its descriptor,
without retaining context. A direct binary ownership check holds exclusion,
release/reacquisition, cancellation and unchanged marker bytes independently
of scenario selection. Memory explicitly refuses real locking. File observations
check changed versus unchanged annotation writes and `--no-annotate`, not only equal
final bytes. The outside-source fixture preflights its exact snapshot name after a
normal owned sibling succeeds. An embedded drive-colon name that the real filesystem
cannot create is a precise capability skip; other setup and command errors remain
failures. Memory uses its actual map path and executes that logical scenario.

T-101 reviewed T-76's command holders once the scenarios ran green through both drivers. Kept,
each for what no scenario asserts: 52's exact keys of a mutant; 231's one run over a reached and
an unreached file; 232's skipped preparation baseline and listed trial stages; 233's
coverage.py-before-7.14, `patch = subprocess` and counted cases; 235's `--json` listing without
the gone mutant; and 236's uncovered orphan kept, emptied folder removed and removed orphans
named. The extra guards stay: abandoned-worker unmarked/young/foreign/trial-copy, the shared
package baseline, and the removed-site exception. The run exit codes the scenarios leave out are
ID-MUT-29's, and a timed-out mutant counted killed (53) exits 0 by the same rule.

| ID | Holder | T-101 |
| --- | --- | --- |
| 52 | `cmd/itos-cc/mutate_mutants_test.go`: `TestEveryMutantAsJSON` | kept |
| 53 | `cmd/itos-cc/mutate_mutants_test.go`: `TestATimedOutMutantsOutcomeIsItsOwn` | removed |
| 54 | `cmd/itos-cc/mutate_mutants_test.go`: `TestMutantsTakenFromTheSnapshotSaySo` | removed |
| 55 | `cmd/itos-cc/mutate_mutants_test.go`: `TestWithSinceOnlyTheJudgedFunctionsMutantsAreListed` | removed |
| 56 | `cmd/itos-cc/mutate_mutants_test.go`: `TestAFailingBaselineListsNoMutant` | removed |
| 145 | `internal/workdir/annotation_legacy_test.go`: `TestASummaryCommentWithTheOldMarkersIsReplacedByOneWithTheNew` | adapter, kept |
| 231 | `cmd/itos-cc/mutate_unreached_baseline_test.go`: `TestAFileNoTestReachesReportsItsBaselineAsNotRun` | kept |
| 232 | `cmd/itos-cc/mutate_counted_unreached_listed_test.go`: `TestACountedRunJudgesAnUnreachedFileByTheListedTestsThatReachIt` | kept |
| 233 | `cmd/itos-cc/mutate_python_parallel_data_test.go`: `TestAPythonProjectThatWritesParallelCoverageDataIsMeasured` | kept |
| 234 | `cmd/itos-cc/mutate_workers_abandoned_test.go`: `TestALaterRunRemovesTheWorkerCopiesAKilledRunLeftBehind` | removed |
| 235 | `cmd/itos-cc/mutation_removed_sites_test.go`: `TestARecordedMutantWhoseSiteIsGoneIsLeftOut` | kept |
| 236 | `cmd/itos-cc/mutation_orphans_test.go`: `TestASnapshotWhoseSourceIsGoneIsRemovedByTheNextRunOverItsDirectory` | kept |
| 237 | `cmd/itos-cc/mutation_orphans_test.go`: `TestASnapshotOfAFileOutsideTheProjectRootIsKeptWhileItsFileExists` | removed |
| 238 | `cmd/itos-cc/mutation_orphans_test.go`: `TestASinceRunWhosePathsMissACommittedRenamesNewPathKeepsTheOldSnapshot` | removed |
| 239 | `cmd/itos-cc/mutation_orphans_test.go`: `TestARunBeforeARenameIsCommittedKeepsTheOldSnapshot` | removed |
| 240 | `cmd/itos-cc/mutate_reused_baseline_test.go`: `TestAFileWhoseMutantsAreAllReusedReportsItsBaselineAsNotRun` | removed |
| 241 | `cmd/itos-cc/mutation_removed_sites_test.go`: `TestCheckSaysHowManyRecordedMutantsItLeftOut` | removed |

For 32–33, retain `internal/workdir/annotation_legacy_test.go`'s `TestAnnotateReplacesItsOwnBlock`;
for 34, retain `internal/mutants/annotate_test.go`'s `TestStripLeavesABlockFollowedByCode`. These domain/adapter guards
remain alongside the new public observations.


T-77 converts the exception and renewal scenarios (ID-MUT-85 through 99, 138 through
144, 146, 227 and 242). Its local selection runs 25 cases and 121 steps without
skips; 58 restored strictness edits fail at their intended checks, and the
two existing shared problem consumers pass 12 steps without skips. The TypeScript project has fifteen real sites: fourteen
killed by its tests and the count survivor at 7:19, beside place at 5:9. The
separate refusal fixture records killed and uncovered sites at exactly 5:9 and
9:3. Tests and coverage declarations match these actual commands and outcomes;
updating a declaration replaces only its fixture-owned node_modules link.

The reviewed exception comes from the public except command. MUT89 then removes
only its source snapshot so the literal run executes fifteen mutants, without
injecting --mutate-all. MUT90 keeps its actual cache and separately runs its
explicit --mutate-all command. Rename fixtures use actual glob imports from the
beginning, preserving test bytes while executing the renamed source; both the
moved-entry metadata and the excepted outcome are asserted. An unchanged
--since rename is unjudged: its original output is checked before a separately
captured whole-project run proves the reviewed grid.Board#count survivor is
excepted. That probe preserves the original When and makes no fresh-check claim
about the transferred namespace cache. The scoped fixture
reviews count and changes it before base, then commits only place after base.
The Go renewal fixture commits the changed string literal, refreshes the current
whole-project outcomes before renewal, and checks the literal whole-project
check. Its later killed, changed-line and no-fresh-results controls preserve the
entries and every file other than itos-cc.yaml. No new driver capability,
mutation operator, product behavior or binary-only exclusion is added.

The unaffected language-tools CI selection includes the 24 TypeScript fixture
cases; the Go renewal case runs locally until T125 restores mutation scenarios
in the initial CI selection. Local execution counts are distinct from any
selection that T83 omits. T101 owns legacy cleanup after replacement CI. Keep
all the following holders and their helpers, plus the unjudged --since
annotation guard, renamed-and-changed/path-only guards, legacy renewal and
renewal-argument controls. The prior unsupported < to != examples were corrected
to the publicly listed < to <= by the coordinator before this conversion; that
was a specification correction, not evidence of missing product behavior.

T-101 reviewed these holders once the scenarios ran green through both drivers; each kept one
holds what no scenario asserts. An except's exit 0 and recorded fields are ID-MUT-85's, a run with
an excepted survivor exiting 0 is 89's, a check failing a changed entry before renewal is 96's,
and the summary comment's blank line before it is `internal/workdir`'s `TestAnnotateReplacesItsOwnBlock`.

| MUT | Holder | Exact test | T-101 |
| --- | --- | --- | --- |
| 85 | cmd/itos-cc/mutation_except_test.go | TestExceptingASurvivorRecordsItWithItsReason | removed |
| 86 | cmd/itos-cc/mutation_except_test.go | TestOnlyARecordedSurvivorCanBeExcepted | kept: a site where no mutant is |
| 87 | cmd/itos-cc/mutation_except_test.go | TestAReasonIsRequired | removed |
| 88 | cmd/itos-cc/mutation_except_test.go | TestTheRestOfItosCcYamlIsKept | removed |
| 89 | cmd/itos-cc/mutation_except_test.go | TestAnExceptedSurvivorFailsNothing | kept: no excepted survivor listed as survived |
| 90 | cmd/itos-cc/mutation_except_test.go | TestAnExceptedSurvivorIsReusedWhileNothingChanged | kept: the reason on the mutant --mutate-all reruns |
| 91 | cmd/itos-cc/mutation_except_test.go | TestAfterItsTestsChangeAnExceptedMutantThatStillSurvivesStaysExcepted | kept: the rerun mutant's outcome and reason |
| 92 | cmd/itos-cc/mutation_except_test.go | TestAnExceptedMutantThatIsNowKilledMakesItsEntryStale | removed |
| 93 | cmd/itos-cc/mutation_except_test.go | TestAnEntryWhoseFunctionChangedIsStale | kept: a moved function's entry holds |
| 94 | cmd/itos-cc/mutation_except_test.go | TestAnEntryWhoseSiteIsGoneIsStale | kept: a site gone from a function still there |
| 95 | cmd/itos-cc/mutation_except_test.go | TestAnEntryDoesNotExcuseAnUncoveredMutant | kept: the counts, uncovered and excepted 0 |
| 96 | cmd/itos-cc/mutation_except_test.go | TestMutationCheckPassesAnExceptedSurvivorAndFailsAStaleEntry | removed |
| 97 | cmd/itos-cc/mutation_except_test.go | TestWithSinceOnlyTheJudgedFunctionsEntriesAreChecked | kept: check --since |
| 98 | cmd/itos-cc/mutation_except_test.go | TestAnInvalidItosCcYamlIsAConfigError | kept: no mutant runs; check of a file that is no YAML |
| 99 | cmd/itos-cc/mutation_except_test.go | TestExceptedMutantsAsJSON | kept: only an excepted mutant has "excepted" |
| 138 | cmd/itos-cc/mutation_annotate_test.go | TestTheSummaryCommentCountsAnExceptedSurvivorApartWithItsReason | removed |
| 139 | cmd/itos-cc/mutation_annotate_test.go | TestAStaleEntrysSurvivorIsListedAsSurvived | removed |
| 140 | cmd/itos-cc/mutation_annotate_test.go | TestAReasonOverSeveralLinesIsWrittenOnOneCommentLine | removed |
| 141 | cmd/itos-cc/mutation_except_elsewhere_test.go | TestARunOfTheWholeProjectFailsAnEntryWhoseFileIsGone | removed |
| 142 | cmd/itos-cc/mutation_except_elsewhere_test.go | TestMutationCheckOfTheWholeProjectFailsItToo | removed |
| 143 | cmd/itos-cc/mutation_except_elsewhere_test.go | TestWithSinceAnEntryWhoseFileTheRangeDeletedIsGone | removed |
| 144 | cmd/itos-cc/mutation_except_elsewhere_test.go | TestAnEntryWhoseFileWasRenamedHasMovedAndStillExceptsItsMutant | removed |
| 146 | cmd/itos-cc/mutation_except_elsewhere_test.go | TestARunOfTheWholeProjectFindsARenamedFilesEntryMoved | removed |
| 227 | cmd/itos-cc/except_renew_test.go | TestARenewedExceptionFollowsItsUnchangedMutantAcrossARename | removed |
| 242 | cmd/itos-cc/except_renew_test.go | TestAStaleExceptionWhoseFunctionChangedNamesTheRenewalAsItsFix | kept: the fix names the site and itos-cc.yaml |

Additional retained guards are outside the replacement mapping:

- cmd/itos-cc/mutation_except_test.go: `TestMutationExceptTakesOneSite`; `internal/mutation/except_test.go`: `TestASiteIsFileLineAndColumn`.
- cmd/itos-cc/mutation_annotate_test.go: `TestUnderSinceTheSummaryCommentExceptsTheSurvivorsOfFunctionsNotJudged`.
- cmd/itos-cc/mutation_except_elsewhere_test.go: `TestAnEntryWhoseFileWasRenamedAndWhoseFunctionChangedIsGone`, `TestWithPathsAnEntryWhoseFileIsGoneIsNotJudged`.
- cmd/itos-cc/except_renew_test.go: `TestRenewingTakesNoSiteAndNoReason`, `TestALegacyExceptionRenewsByItsPlaceAcrossARenamedModule`.

MUT247 independently checks bug-2's preservation of an unjudged outcome's
original whole-suite evidence in Go, TypeScript, Python and Kotlin source
layouts. Each fixture actually executes the custom `go version` command with
coverage disabled and declares that exact Go command and its surviving sites;
these cases do not claim to run the other languages' runtimes or collectors.
They require nonempty original test and support hashes, preserve the unjudged
judgment and source hash, and check current evidence for the judged function
plus stale reporting and sample exclusion. A separate bounded invocation in
the unaffected Ubuntu language-tools step executes all four examples while
T83 omits mutation scenarios from the initial matrix. This independent
regression does not complete the shelved T78 conversion or replace any legacy
holder; all retained partial-run, fail-fast and coverage guards remain.

T78 converts MUT150..155, 203..210 and 213..220: 22 IDs and 41 expanded
examples, including both Python runners and both Kotlin build systems.
Real pytest, unittest, Vitest, Gradle and Maven fixtures prove execution and
coverage where the sentences require it. Logical historical scope and reach
fixtures use valid recorded artifacts and explicitly declared real custom
commands. A logical fixture executing `go version` declares Tests.Language as
Go even when its source layout is another language; source coverage declarations
retain their actual languages and reports. MUT218 records its Node built-in test script with `--no-coverage`:
it has neither Vitest nor Jest, invokes no collector, and needs no Coverage
declaration. Kotlin Coverage uses the existing Report field for the fixture's
actual JaCoCo facts, with `Program` naming Gradle or Maven.
The same Report contract carries actual LCOV and Go coverprofile data in
other fixtures; the bound memory Measurer must parse the declared format.

`WriteExecutable` writes owned fixture bytes and ensures executable permission,
including after an ordinary-file overwrite; its shared byte contract also
holds the memory driver. MUT205's Python example executes the literal
`./run.sh`. Windows cmd cannot execute that literal, as the retained holder
already records. MUT216's real unittest setup uses `.venv/bin/python`; Windows
virtualenvs use `Scripts` instead. These local Linux and Ubuntu installed-tool
results do not establish Windows scenario execution. Logical freshness and
own-test selection remain eligible for the later bound memory driver.

Own-test traces retain actual canonical execution roots and mutated source
hashes, plus names emitted by the executing Python test or Kotlin test class.
The optional `TestTrace.Tests` inventory includes every runnable selector,
including unrelated tests. T88/T90/T125 must derive events from the actual
bound Workers and Measurer requests, preserve nil-inventory behavior and fail
unsupported selectors; an empty or scripted trace is not an execution result.

Only MUT209, 210 and 214 gain permanent binary-only reasons: actual frozen
exports with installed offline dependencies, interpreter bytecode, and pytest
cache lifecycles. MUT209 compares canonical dependency provenance from setup,
baseline and mutant execution, committed source hashes against dirty live
source, and the original tree's bytes. MUT210 starts with genuine original
bytecode generated through public coverage/compileall setup, then observes
equal-size baseline and mutant contents in one pinned second without worker
bytecode. MUT214 observes default commands without pytest cache and the
literal custom command creating it. Original source and cache bytes are
inventoried through Driver. These physical observations are not memory fakes.

MUT153, 154 and 207 read actual public serve graphs through Driver.Graph.
MUT207 saves initial and post-scoped check/graph/sample populations before the
full refresh, includes fresh new helpers, and compares the final per-function
reuse against that saved population. Its unjudged comparisons require original
nonempty evidence and judgments while allowing stale markers and current
source positions. Bug-2 holds the evidence-preservation fix independently in
MUT247. The staged graph deferral for MUT134/153/154/163/207 remains explicit in
T125/T101 until T112 proves both drivers; it is no permanent exclusion.

CI keeps all prior installed-tool selections and MUT247, then adds three
bounded invocations for T78 policy/reach, suite/coverage-scope, and
installed/cache/own-test families, with the default timeout unchanged. T83's
initial-matrix omission is still distinct from actual installed-tool execution.
Keep all mapped holders, every helper and extra assertion until restored
replacement CI and T101 cleanup. In particular,
`TestStrictMixedRunStopsWhenAnotherLanguageMeasuredNothing` in
`uncovered_fails_closed_test.go` remains outside the replacement mapping;
embedded fixture test functions are not additional repository tests to delete.

| MUT | Retained holder | Exact test |
| --- | --- | --- |
| 150 | cmd/itos-cc/mutate_all_tests_fresh_test.go | TestBinaryOnlyEndToEndTestChangeMakesBroadScopeKillStale |
| 151 | cmd/itos-cc/mutate_all_tests_fresh_test.go | TestBroadScopeSupportFilesAreHashed |
| 152 | cmd/itos-cc/mutate_all_tests_fresh_test.go | TestPlainRunReusesBroadScopeKillsAndKeepsTheirEvidence |
| 153 | cmd/itos-cc/mutate_all_tests_fresh_test.go | TestMixedRecordedScopesKeepTheirOwnFreshnessAndMatchTheGraph |
| 154 | cmd/itos-cc/mutate_all_tests_fresh_test.go | TestPartialRunKeepsUnjudgedBroadOutcomesStale |
| 155 | cmd/itos-cc/mutate_all_tests_fresh_test.go | TestGoBroadScopeIncludesBuildTaggedTestsButExcludesNestedModules |
| 203 | cmd/itos-cc/strict_mixed_languages_test.go | TestStrictGoCoverageIgnoresTheOtherLanguagesOfAMixedSelection |
| 204 | cmd/itos-cc/uncovered_fails_closed_test.go | TestStrictRunsFailClosedWhenAnotherLanguageMeasuredNothing |
| 205 | cmd/itos-cc/whole_suite_freshness_all_test.go | TestANonImportingTestChangeMakesAnotherLanguagesWholeSuiteOutcomeStale |
| 206 | cmd/itos-cc/whole_suite_freshness_all_test.go | TestBuildRootsSupportFilesAndLegacyEvidenceAreExplicitForEveryLanguage |
| 207 | cmd/itos-cc/whole_suite_freshness_all_test.go | TestMixedScopesAndPartialRunsKeepTheirOwnDependenciesInEveryLanguage |
| 208 | cmd/itos-cc/mutate_ts_coverage_scope_test.go | TestTypeScriptCoverageComesFromTheRelatedTestsThatJudgeTheMutants |
| 209 | cmd/itos-cc/mutate_counted_languages_test.go | TestACountedRunJudgesAProjectOfEachLanguageThroughItsInstalledToolsOffline |
| 210 | cmd/itos-cc/mutate_python_bytecode_test.go | TestASameSizePythonMutantWrittenWithinTheBaselinesSecondIsStillKilled |
| 213 | cmd/itos-cc/mutate_counted_unjudged_test.go | TestAStrictCountedRunNeedsNoMeasurementOfALanguageWithNothingToJudge |
| 214 | cmd/itos-cc/mutate_python_pytest_cache_test.go | TestMeasuringAndJudgingAPythonProjectLeavesNoPytestCacheInItsTree |
| 215 | cmd/itos-cc/transitive_reach_test.go | TestATestThatReachesAFileThroughAnotherModuleIsAmongItsTests |
| 216 | cmd/itos-cc/mutate_python_own_tests_test.go | TestAPythonFilesOwnTestsAreTheTestsThatReachIt |
| 217 | cmd/itos-cc/mutate_kotlin_own_tests_test.go | TestAKotlinFilesOwnTestsAreTheTestClassesThatReachIt |
| 218 | cmd/itos-cc/mutate_ts_fallback_test.go | TestATypeScriptTestScriptWithoutVitestOrJestRecordsWholeSuiteOutcomes |
| 219 | cmd/itos-cc/test_support_reach_test.go | TestATestReachesAFileThroughTheTestSupportFilesItUses |
| 220 | cmd/itos-cc/test_support_reach_test.go | TestAPythonMutantReachedOnlyThroughConftestIsJudgedByTheTestThatUsesTheFixture |

Strict coverage fixtures (T-79) may declare `Coverage.IntegrationReport` separately
from their in-process `Report`, using the same existing report formats. An empty
integration report means no subprocess evidence. The actual collector runs in the
binary; the later bound memory measurer must parse and match both reports and
preserve their separate provenance, including actual listed-test IDs.
`Coverage.WithoutC8` is a TypeScript-only fixture with no integration report: its
owned `node_modules` facade links the actual pinned packages and tools except c8
and its platform launchers. It never changes the pinned installation. Only typed
permission/unsupported link failures may omit an optional fixture; all other
link errors and every required-tools failure remain errors. Missing c8
becomes a collector fact only when the command requests that built-in collector;
raw-report and no-coverage commands keep their normal behavior. These logical
coverage outcomes remain eligible for the later in-process driver.


T79 converts MUT156..164, 221..226 and 228..230: 18 IDs, 30 expanded
cases and 187 written steps. Every source layout declares its actual test
command and measured Go coverprofile, LCOV or JaCoCo facts. These fixtures
execute real Go, pytest, Vitest, Gradle and Maven commands. The integration
cases observe real Python subprocesses, child Node processes and JVMs, then
run separate listed-test fixtures to verify the exact IDs reaching each line.
Their whole-suite assertions require integration provenance and actual mutant
trials; listed output proves scope and per-line test IDs. Kotlin includes the
main argument-index mutant as well as the four describe mutants. Missing-c8
and missing-agent controls use fresh roots without mutation caches.

Strict line fixtures save independent cold and warm non-strict observations
without restoring or rewriting caches. Strict findings retain the cold mutant
totals; the later non-strict comparison uses full warm problem and mutant
records, including attribution and coverage. Only mutant reuse metadata is
excluded from identity comparison; actual ran/reused and baseline values are
checked separately. The Go report declarations use the public collector’s observed positive-weight
block positions and counts, including the unloaded package. Moved source gets a
new measurement declaration before the partial run; original stored evidence is
not rewritten. Zero-weight rows add no executable obligation.
The repeated-init fixture pairs saved original inventories
by name and hash through the partial run and full measurement, checking current
positions, producer and current inputs without exchanging the inventories.

`ObserveReads` accepts nonempty actual file/directory targets. Each target must
first demonstrate that a real read or directory listing updates its access
time. It then resets only that access time, preserving modification time, and
reports sorted actual observations. A later missing target or ordinary I/O
failure is an error. Missing access metadata or a filesystem that fails its
positive control is an explicit capability omission, never an empty passing
read log. The current memory driver explicitly reports that its read ports
have not moved; T89/T125 must wire observations to real bound selection,
surroundings and toolchain reads with positive controls. No logical scenario
gets a permanent binary-only exclusion for that staged implementation.

MUT164 distinguishes ordinary in-project discovery from following a dependency.
Its public no-replacement control already reads the same excluded in-root
nested module. Both unsupported replacement operations must still refuse it,
execute no test/list/coverage command, preserve the complete old artifact, and
admit neither replacement tree into evidence. A separate genuine external
observer remains marked through both refusals. GOWORK=off is actual Run.Env
input. MUT224 first measures a supported plain layout through its real collector
and proves its complete cached record reusable. It then changes only the scope
manifest and independently updates only an existing changed manifest hash in
that observed record. Opaque producer/environment values and all unchanged
inputs stay intact. This adversarial fresh cache is not a claim that an
unsupported scope was measured. All controls and cache seeding finish before
the outside-read observation starts.

MUT163 reads the actual public serve graph, including complete fresh and stale
populations, and independently checks sampling, other-language strict coverage
and reviewed survivor exceptions. Its temporary in-process graph deferral joins
MUT134/153/154/207 until T112; it is not a permanent exclusion. Local Linux and
Ubuntu installed-tool results do not establish Windows execution of these tool
fixtures. Counted MUT226 follows the existing native counted capability policy;
read-observation and tool omissions are reported explicitly when unavailable.
The unaffected language-tools CI keeps every prior selection, all four MUT247
examples and all three T78 families, then runs three bounded T79 invocations
for Go/changed lines, strict language/scope, and subprocess integration. Default
timeouts stay unchanged. T83's initial-matrix omission remains separate from
actual execution in that installed-tool selection.

Retain every holder below, its helpers and additional assertions until restored
replacement CI and T101 cleanup. This mapping is not permission to remove an
entire tagged test when it also holds an extra assertion.

| MUT | Retained holder | Exact test |
| --- | --- | --- |
| 156 | cmd/itos-cc/strict_go_coverage_test.go | TestStrictGoRunFindsUncoveredExecutableBlockWithoutMutationSite |
| 157 | cmd/itos-cc/strict_go_coverage_test.go | TestStrictGoChecksZeroSiteFunctionsAndExemptsEmptyBodies |
| 158 | cmd/itos-cc/strict_go_coverage_test.go | TestStrictGoKeepsSameLineSpansSeparateAndAttributesClosureBlocks |
| 159 | cmd/itos-cc/strict_go_coverage_test.go | TestStrictGoRunMeasuresMissingEvidenceWhenMutantsAreReusable |
| 160 | cmd/itos-cc/strict_go_coverage_test.go | TestStrictGoCheckRejectsLegacySnapshotWithoutCoverageEvidenceAndRunsNothing |
| 160 | cmd/itos-cc/strict_go_coverage_test.go | TestStrictGoCheckRejectsPartialOrMisattributedEvidence |
| 161 | cmd/itos-cc/strict_go_coverage_test.go | TestStrictGoCoverageFingerprintStalesOnModuleTestChange |
| 161 | cmd/itos-cc/strict_go_coverage_test.go | TestStrictGoCoverageFingerprintTracksModuleAndConfiguredInputs |
| 162 | cmd/itos-cc/strict_go_coverage_test.go | TestStrictGoPartialJudgmentPreservesUnjudgedEvidenceAndRemeasuresMoves |
| 163 | cmd/itos-cc/strict_go_coverage_test.go | TestStrictGoNoCoverageIsAUsageConflictEvenWithNoSelection |
| 164 | cmd/itos-cc/strict_go_scope_test.go | TestStrictGoRunRejectsCachedEvidenceWithActiveWorkspace |
| 221 | cmd/itos-cc/strict_lines_python_test.go | TestAStrictPythonRunProvesEveryExecutableLineOfEveryJudgedFunction |
| 222 | cmd/itos-cc/strict_lines_typescript_test.go | TestAStrictTypeScriptRunProvesEveryExecutableLineOfEveryJudgedFunction |
| 223 | cmd/itos-cc/strict_lines_kotlin_test.go | TestAStrictKotlinRunProvesEveryExecutableLineOfEveryJudgedFunction |
| 224 | cmd/itos-cc/strict_scope_refusals_test.go | TestStrictCoverageRefusesScopesItsFingerprintsCannotProve |
| 225 | cmd/itos-cc/strict_changed_lines_test.go | TestAGoChangeIsJudgedOnTheLinesItChangedNotTheOldCodeAroundThem |
| 225 | cmd/itos-cc/strict_changed_lines_test.go | TestAPythonChangeIsJudgedOnTheLinesItChangedNotTheOldCodeAroundThem |
| 226 | cmd/itos-cc/strict_changed_lines_test.go | TestChangedLineStrictnessNeedsARange |
| 226 | cmd/itos-cc/strict_changed_lines_counted_test.go | TestChangedLineStrictnessHoldsInACountedRun |
| 228 | cmd/itos-cc/mutate_python_integration_test.go | TestLinesAPythonTestReachesThroughASubprocessAreCovered |
| 229 | cmd/itos-cc/mutate_typescript_integration_test.go | TestLinesATypeScriptTestReachesThroughAChildNodeProcessAreCovered |
| 230 | cmd/itos-cc/mutate_kotlin_integration_test.go | TestLinesAKotlinTestReachesThroughAJVMItStartsAreCovered |

Additional retained guards include unloaded-package ordering and coverage-missing
behavior, unattested raw-report rejection, cached workspace rejection,
out-of-inventory replacements, supported scope controls, Maven strict lines,
fail-fast changed lines, pre-7.13 coverage.py behavior, other fingerprint inputs,
partial/misattributed inventories, unsplit integration fallback, and actual raw
collector/run-directory cleanup. In particular, retain
`TestStrictGoReportsExecutableFunctionInUnloadedPackageAsMissing`,
`TestStrictGoRejectsUnattestedRawCoverageFlags`,
`TestStrictGoCheckRejectsCachedEvidenceWithActiveWorkspace`,
`TestStrictGoRunAndCheckRejectOutOfInventoryLocalReplacements`,
`TestStrictGoScopeGuardrailsRemainSupported`,
`TestAStrictKotlinRunBuiltWithMavenProvesTheSameLines`,
`TestAFailFastChangedLineRunStopsOnlyOnTheChangedLines`, and
`TestACoveragePyThatCannotMeasureSubprocessesIsAMissingCollector`.
Pure-domain coverage guards remain for the later domain/adapter extraction.

## Fail-fast and owned execution conversion (T80)

The 20 IDs MUT165..167,187..189,191..202,211,248 expand to 21 cases and
116 written steps. MUT168/190's exact former prose is retained beside its rule
as labeled adapter-contract comments: private startup/wait/complete fault
injections cannot be a public command scenario. Their guards remain intact.
MUT191/199's comparative Windows and repository-documentation clauses remain
comments; their public help checks remain executable. New MUT248 supplies the
two actual native Windows refusals. Product behavior and exit codes are unchanged.

`Start(ctx, Run)` owns only the product under test and rejects `Run.Binary`.
`Output` is a synchronized snapshot of captured bytes; `Wait` returns the final
result after joining. `Interrupt` and `Close` address that owned run. The memory
driver and native Windows report this asynchronous physical capability
unsupported before launch; synchronous `Run` still proves Windows admission.
The existing interrupt fixture and running handle share their native signal
leaves, preserving their provenance. No Windows tree supervisor is introduced.

`FixturePeers` holds a bounded loopback listener on Linux/macOS. Native Windows
refuses this capability before listening because forced helper exit resets its
TCP connection rather than supplying the orderly EOF oracle. Already-canceled
contexts take precedence; reset and cleanup errors are never accepted as EOF.
Three factory-dependent physical capability tests omit only this typed native
Windows refusal; portable cancellation, cleanup and admission guards still run.
Synchronous native MUT199/248 remain required. Genuine Go tests self-reexecute
their compiled test binary for controlled child/grandchild helpers. Each actual
process authenticates a nonce/PID connection and atomically publishes its ready
record, canonical worker/original root and source SHA before any release barrier.
The connection stays open until process exit: a ping/ack proves liveness, and EOF
after public return proves termination of these helpers. It is not an arbitrary
process oracle. Cooperative stop is cleanup only, after public assertions; a
separate unrelated owned counted product remains live through its assertion.
Its real default-Go preparation is held, so cleanup interrupts and joins the
product-owned intermediary tree as well as the observed peers. Its join and
EOF observations precede cooperative drain, avoiding competing stop writes
while that tree exits. Immutable
mode/block controls and separate release markers avoid read/truncate races.
Cancellation wakes pending observations and joins its deadline callback before
another observation can start. Ordinary I/O/protocol/deadline errors fail.
The world's cleanup callback stops new work, drains exact peers and joins runs
before removing any fixture; a failed join retains the fixture for recovery.

MUT167/189/191 characterize actual two-site execution order with a public fixed
seed count-two run and correlate real source traces with actual judgments. They
never infer order from a sorted snapshot or call internal candidate helpers.
Only an external control and its truthful timing-out declaration change before
the sequential interruption. The first completed progress and later live trial
must both be observed. Product join and descendant EOF observations share the
same cancellation-release-plus-five-second deadline. Current
exit75/count.interrupted remains deliberate. The macOS count-one check and its
subsequent interruption use separate owned trace observations; earlier trials
are cleared before the interruption run.
MUT193 measures its conservative five-second bound from survivor barrier release
after both workers are ready, before public stop/cleanup; product join and all
descendant/trigger EOF observations must finish within that same bound. Exact shared-budget
identity/race assertions remain in the adapter guards.

Logical fixtures declare actual Go commands, sites, baselines and traces.
Coverage declarations use the actual public compiler's observed Go block reports;
listed fixtures predeclare their exact inventory before public named-report setup
verifies it. T89/T90/T125 must produce/parse the requested declared report through
actual bound tool/report operations, never a fixture-ID result shortcut.
MUT195 creates its exception with the public command and removes only its
unnecessary fixture snapshot before changing that function, so valid old reuses
cannot be mistaken for new trial admission. MUT211's JSON and text run on
independently prepared twins with actual prior public judgments, keeping exact
counts without restoring caches. Its aggregate failing-baseline controls run first.

Two logical fixture barriers are obligations for T88/T125: B survives only after
A2's actual Workers request has entered, and MUT197's FF_FAIL_AFTER failing listed
baseline waits for the held own request. The future fake must derive these from
actual bound requests/context cancellation, not scheduling delays or scripted
expected cancellation events. No current binary result proves that future wiring.

Permanent physical `@binary-only` IDs are exactly 165..167,187..189,191,193.
Logical 192,194..202,211,248 remain eligible for later bound memory execution.
The separate native CI invocation follows the ordinary suite on all three OSes,
before installed-language setup, without T83's omission. Verbose raw logs and
`T80-native-<OS>` cucumber artifacts distinguish omissions from execution; the
existing pinned upload action is reused. Prior language selections and default
timeouts remain unchanged. Static expected native execution is Linux15/82,
macOS16/87 and Windows3/12; these are planning counts, not CI evidence. Local
Unix assertion probes do not constitute Windows Godog negative execution.
Portable wrong-fact/admission-write guards establish assertion sensitivity
separately from the native Windows positives.

Keep every legacy holder, helper and extra assertion through actual restored
replacement CI and T101 cleanup. No adapter fault/budget guard is a CLI duplicate.

| MUT | Retained holder | Exact test or shared helper |
| --- | --- | --- |
| 165 / 187 | mutate/command_runner_ownership_linux_test.go / command_runner_ownership_darwin_test.go | TestLinuxOwnTimeoutStopsOwnedProcessTree / TestMacOSOwnTimeoutStopsOwnedProcessTree; checkOwnTimeoutStopsOwnedTree |
| 166 / 188 | same ownership holders | TestLinuxNormalReturnStopsOwnedDescendant / TestMacOSNormalReturnStopsOwnedDescendant; checkNormalReturnStopsOwnedDescendant |
| 167 / 189 | same ownership holders | TestLinuxParentCancellationStopsOwnedTreeWithoutTimeoutJudgment / TestMacOSParentCancellationIsNotAMutantDeadline; cancellation/completed-first helpers |
| 168 / 190 | Linux/Darwin/shared Unix ownership holders | TestLinuxOwnershipStartupFailureDoesNotRunCommand; TestLinuxCleanupFailureIsReturnedNotJudged; TestMacOSSupervisionFailuresDoNotPermitUnsafeFallback; TestLinuxProbeCountsAZombieOnlyGroupEmpty |
| 191 / count part of 248 | cmd/itos-cc/mutate_counted_macos_test.go | TestCountedExecutionIsAdmittedOnMacOSAndStillRefusedOnWindows |
| 192 | cmd/itos-cc/mutate_fail_fast_test.go | TestTheFirstActionableSurvivorStopsAdmissionOfLaterMutants |
| 193 | same fail-fast holder | TestInFlightJudgmentsAreCancelledAndTheirOwnedProcessesCleanedUp |
| 194 | same fail-fast holder | TestNonActionableJudgmentsNeverStopTheRun |
| 195 | same fail-fast holder | TestKnownPolicyFailuresStopBeforeAvoidableMutantWork |
| 196 | same fail-fast holder | TestAFailingBaselineStopsTheRunWithoutInventingOutcomes |
| 197 | same fail-fast holder | TestOutputDistinguishesCompletedWorkFromCancelledAndUnattemptedWork |
| 198 | same fail-fast holder | TestAFileTheStopCutShortPublishesNoPartialProof |
| 199 / fail-fast part of 248 | cmd/itos-cc/mutate_fail_fast_admission_test.go | TestFailFastAdmissionBoundaries |
| 200 | cmd/itos-cc/mutate_fail_fast_partial_test.go | TestValidJudgmentsInAStoppedFileArePreservedUnfinishedSitesStayUnjudged |
| 201 | same partial holder | TestACancelledForcedRerunLeavesAFreshPriorCacheUsable |
| 202 | same partial holder | TestALaterRunFinishesOnlyWhatTheStopLeft |
| 211 | cmd/itos-cc/mutate_fail_fast_counts_test.go | TestAFileBlockedByAFailingListedSelectionCountsWhatItRanAndReused |

The current partial fixture demonstrates reused/completed proof, cancellation and
unattempted sites; the old blocked-selection partial-evidence branch remains an
extra guard. MUT196 observes the actual own-baseline failure with canonical
source traces; MUT197/211 cover its listed-selection-baseline alternative. Keep
both failing-baseline variants, all planning strict/uncovered
branches, exact listed/support fingerprints, source/test/support reuse branches,
argv/environment/streams, no-unrestricted-fallback, captured-group/zombie probes,
completed-exit races, no-run-abort-budget on success/own timeout, and shared
cleanup-budget identity/order. Preserve `checkInterruptedCountedRun` and its extra
SIGTERM/no-remove-while-alive controls for T81. T88 moves adapter guards intact;
T101 must not delete them as duplicates of these public observations.

## Counted execution and worktrees (T81)

MUT173..186 and212 are fifteen cases with99 written steps after the precise
MUT173 comparative-platform correction. The Windows clause remains verbatim
beside173 and is exercised by native248. Deferred169..172 remain explicitly
wip; completed counted IDs leave `Unstepped`. Selection guards use isolated
pending fixtures and retain unknown-ID failure with empty or wip-only lists.

The fixtures run real committed Go sources, tests and measured compiler
profiles. Same-name175 uses five public-listed sites and actual zero-hit
reports. Each runnable package's TestMain emits canonical cwd/original root
and an actual source SHA; optional trace `source` is the project-relative
identity already named by that command's `TestTrace.Source`. Later bound
Workers/Measurer requests must derive that identity and hash from the actual
selected copied input, preserving older trace shapes where it is unnecessary.
Per-command deltas distinguish clean baselines from mutations; zero eligible
sites may genuinely execute nothing. No planner/ranking helper supplies seeds.

Frozen174 leaves broken staged, unstaged and untracked bytes in place while
actual coverage, baseline and trial traces identify HEAD's original/mutated
source. Global176 observes one selected trial with four workers, real omitted
subjects and fresh execution despite matching complete cache. Output184
compares exact public identities and independently observes unfinished states.
Proof185 retains a real annotated other-file cache and checks it separately
from the sampled multi-site function's missing complete proof.

Only183 and212 are permanently binary-only: actual SIGINT/process lifetime,
and physical linked Git metadata/private scratch/symlink escape respectively.
All thirteen logical cases remain eligible for future actual memory bindings.
The separate three-OS native invocation follows T80 and precedes installed
tools, preserving prior selections and default timeout. Required original
`T81-native-<OS>` cucumber artifacts distinguish native execution from Godog's
skipped-as-passed summaries. All fifteen currently omit on Windows through
its counted capability; native248 independently proves the actual refusal.
No logical case is permanently tagged for that temporary implementation gap.

T81's listed-coverage fixtures declare optional `ListedTests.CoverageExit`
and `CoverageOutput` for the actual collection command. Zero/empty keep
successful behavior. Optional `CoverageMetadata` contains the exact invalid
bytes that the real harness writes into its owned per-test `covmeta` file;
nil retains ordinary Go-generated metadata. Own coverage and the clean
listed baseline remain independent. Later memory bindings must execute the
actual requested measurement/tool conversion against the declared artifact
and let the real classifier report failure; these facts do not script a
preparation verdict or require a new Go metadata parser.

`ListedTests.ListOutput` optionally declares exact successful list stdout,
including malformed duplicate lines. Empty retains the ordinary inventory
from `Files`; `ListFails` continues to describe an actual command failure.
Only the real requested list operation returns those bytes to the existing
parser, which determines whether the inventory is valid.

MUT184 keeps its primary sampled report and obtains unattempted, blocked and
cancelled observations from separate real controls. Child products join
before fixture removal. It stays eligible in process: its later cancellation
control must cancel the actual run context after a bound Workers request
enters, without requiring FixturePeers, networking, PID signals or scripted
events. MUT183 separately holds physical process lifetime observations.

Keep every legacy holder, helper and extra guard until actual replacement CI
and T101's review. No legacy file is deleted by T81.

| MUT | Retained holder under cmd/itos-cc | Exact test |
| --- | --- | --- |
| 173 | mutate_counted_test.go | TestCountedExecutionHasExplicitAdmissionAndPlatformBoundaries |
| 174 | same | TestCountedExecutionEvaluatesFrozenCommittedInputs |
| 175 | same | TestFreshSelectionIsReproducibleAcrossPathsAndSameNameUnits |
| 176 | same | TestTheGlobalFreshBudgetHoldsEvenWhenEverySelectedMutantIsKilled |
| 177 | same | TestCountedModeLeavesCompleteAndCachedSampleContractsIntact |
| 178 | mutate_counted_listed_test.go | TestApplicableListedIntegrationTestsFinishASingleFreshJudgment |
| 179 | same | TestFreshOutcomesPreserveSurvivorExceptionAndOwnTimeoutMeanings |
| 180 | mutate_counted_test.go | TestMeasuredUncoveredSelectionIsExplicitAndIsNeverRedrawn |
| 181 | mutate_counted_listed_test.go | TestCountedPreparationNeverConvertsFailedMeasurementIntoSuccessfulAssurance |
| 182 | same | TestCleanBaselineFailureIsNotAMutationKill |
| 183 | mutate_counted_interrupt_test.go | TestInterruptionReportsPartialWorkAndCleansUpOwnedCommands; checkInterruptedCountedRun |
| 184 | mutate_counted_test.go | TestMachineAndTextOutputDistinguishSampledWorkFromPopulationCompleteness |
| 185 | same | TestSampledExecutionPublishesNoPersistentCompleteProof |
| 186 | same | TestOnlyAGenuinelySiteFreeRangeIsNotApplicable |
| 212 | mutate_counted_worktree_test.go | TestACountOneRunInALinkedWorktreeJudgesItsCommittedSelection |

Retain the original183 SIGTERM/second-signal, waiter and no-remove-while-alive
sampled watcher extras; it is supplementary observation rather than exact
sequence proof. The166/183/188/193 public cleanup clauses now assert final
terminated peers and absent copies/private inputs after return. Their exact
former join-before-removal prose remains labeled beside each scenario, with
all adapter guards retained for T88/T92 deterministic boundary proof. Focused
166/193 registration evidence belongs to T81; historical T80's89 probes stay
qualified to their earlier source revision. Preserve the untagged scope,
measurement/provider, zero-site, ordering and helper guards as well as the
T79-owned strict_changed_lines_counted_test.go226 holder.

The frozen T81 implementation is `572f06a8ed7fa750d95d24e4156d743ade609a62`.
Its final unedited control executed all fifteen cases and 99 steps, with no
omissions. The reviewed serial plan ran 118 independent expectation edits:
all 118 reached their intended assertion and failed; none were invalid or
undetected. The run took 18m53.076s. The coordinator independently checked
every original cucumber report, case/step identity and diagnostic, then
verified all eleven source files' exact bytes and 0644 modes against that
commit. HEAD stayed unchanged and the checkout was clean after restoration.
These source-restoration results apply to that revision only.

The manifest SHA-256 is
`5b21b388a5a54d9f0b31b7c40b32a6bd1860a8ad0f465055e4e03029252e40a1`;
the final runner report SHA-256 is
`bbd76249d2450747f61e161620ddb18e25add46954895b29ada75a801a399fe5`.
Original native observations are from
[CI run 38800701382](https://github.com/donvargax/itos-cc/actions/runs/38800701382):
Linux and macOS each executed all 15 cases/99 steps; Windows omitted all 15 cases/99 steps
through the explicit counted capability, executing none of them. The same
revision's retained T80 native reports actually exercised shared cleanup scenarios 166
and 193 on Linux and 188 on macOS; Windows cases 199 and 248 still executed.
No omission is counted as an executed passing case.
The complete workflow passed for that exact revision: all three platform
test jobs, all five release builds and publication.

## Snapshot storage scenarios (T-82)

`metrics_snapshots.feature` converts all twelve IDs, seventeen expanded cases
and fifty steps. It joined `Converted`, with `InProcess` deliberately left to
T-101, which added it (below). Local runtime needs only Go and Git. No source from this repository is
mutated: actual Go tests and collectors run in owned fixture projects.
SNAP01 uses each literal command; SNAP03 actually writes all four command
snapshots before checking every format version. Root-relative dry output uses
real clones large enough for the default bounds, without hidden flags. Dry
requests neither tests nor coverage. Other fixtures declare actual Go test
outcomes and measured coverprofile spans, including the two-module profile's
three positive-weight blocks.

SNAP04 preserves the original temporary-file/rename sentence as an adapter
contract comment. Its public steps read a complete stored JSON object and
release two real collector barriers before joining either writer. Each actual
writer reports a different complete population (both functions at 100 or 66.7
percent); the final stored object must consistently match one of them. This
is concurrent product execution, not proof of every possible reader or the
private rename mechanism. SNAP04 and SNAP06 alone carry permanent binary-only
reasons for physical publication/concurrent collector observations. Native
Windows reports the existing typed lifecycle refusal before those cases run;
ordinary listener, protocol, start and filesystem errors remain failures.

SNAP06 runs the literal mutation and crap commands together in committed root
and nested Go modules. Actual tests use per-run environment to reach both
branches for mutation and only the positive branch for crap. Both nested
collectors hold authenticated owned peer connections after completed first
profiles exist in distinct run directories. Each peer records actual canonical
`GOCOVERDIR`, independently pairing its owned root to those observed reports
before hit comparisons. Reports are loaded per plan:
this barrier makes no claim that those first profiles remain unread. Public
mutation outcomes (four kills, no uncovered, for each file), snapshots and
normal annotations are observed while crap remains held. Crap then reports
66.7 percent for both functions. Each measured profile is checked against
its actual spans/hits; staged latest reports change from all three blocks
hit to two, public existing-report reuse preserves complete entries with
zero collector trace delta, and both run directories disappear. Successful
release uses separate owned marker files; peer Stop is cleanup only. Any
error stops further product work, drains exact owned peers and joins products
before fixture deletion; failed cleanup retains the fixture.

SNAP07's context-first `SetModTime` updates an existing owned file/directory
and returns actual stat-observed modification time. Cancellation precedes I/O;
ordinary errors retain their causes. A 48-hour-old run directory is removed,
while a fresh run marker and unrelated kept report remain byte-identical.
The memory driver honestly returns typed unsupported because its tree records
no entry modification time. SNAP07 remains logically eligible: T-101 reports
that capability omission (below) rather than echoing a requested time as an
observation.

The exact public oracle script/argv and original outputs remain in the task
handoff (`python3 /tmp/T82-concurrent-oracle-multiline.py`, raw log and four
JSON artifacts); the earlier one-line characterization remains separate.
Assertions use only Driver operations, public stdout/JSON and actual artifacts.

The native CI invocation selects only SNAP01..12 after T-81 and before optional
language tools, with the existing default timeout and all prior invocations
unchanged. Required original `T82-native.json` artifacts expose actual cases
and per-step omissions; ordinary matrix selection also includes snapshots,
outside T-83's mutation-only omission. Runtime counts are reported from the
original JSON, not Godog's skipped-as-passed summary.

T-101 removed `cmd/itos-cc/snapshots_root_test.go` once these scenarios ran
green through both drivers. Each test's root location, root-relative names and
relative output are its scenario's assertions; its exit-code preconditions are
ID-MUT-29's (a survivor exits 1) and the crap, dry and scrap features', and its
crap `--no-coverage` is ID-COV-19's. The mapping was:

| ID | Removed holder |
| --- | --- |
| SNAP08 | `TestSnapshotsLiveAtTheProjectRootWhereverACommandRuns` |
| SNAP09 | `TestEveryCommandsSnapshotNamesFilesFromTheRoot` |
| SNAP10 | `TestACommandFindsTheSameResultsFromAnyDirectory` |
| SNAP11 | `TestPathsOnTheCommandLineAndInOutputStayRelativeToTheWorkingDirectory` |
| SNAP12 | `TestOutsideAGitRepositoryTheWorkingDirectoryIsTheRoot` |

Its own helpers, `snapshotFiles`, `noMetricsIn` and `inSrc`, went with it,
used nowhere else; the shared module and source helpers stay for their other
users.
At the metrics adapter, keep `TestWritesAtTheSameTimeLeaveOneWholeSnapshot`,
`TestARenameRefusedForAMomentIsRetried`, `TestARenameThatCannotLandFails`,
`TestAReadRefusedForAMomentIsRetried` and
`TestOneProcessNeverRenamesOrReadsASnapshotWhileItIsRenamed`, including temporary
cleanup, modes, Windows permission/refusal and same-process serialization.
The coveragetools old/live/report cleanup and isolated-run adapter guards
also remain; no private injection or exact ordering guard is a CLI duplicate.

For assertion sensitivity, the current reviewed runner supports one exact
assertion name per probe. SNAP01/SNAP09 therefore use reviewed literal-binding
probes that change only the Then artifact expectation to one constant wrong
path. Their original commands, tables and case identities stay unchanged, and
all four/three expansions must fail at that assertion. Separate non-outline
source-check probes cover shared snapshot, version and path predicates.
Expanded intended failures and probe counts are reported separately; feature
and source bytes/modes are restored exactly.

The final unchanged control for implementation
`159641d484608b1d21ec4cd28960501daa2abbd0` executed all seventeen cases and
fifty steps without omissions. Seventy-one individually reviewed assertion
probes produced seventy-six intended expanded-case failures, with zero
invalid or undetected probes. The two outline literal-binding probes failed
all four and all three original examples; the other sixty-nine probes changed
source checks. Independent review of every original cucumber JSON checked
case identity, assertion line/name and error, passed preceding steps, and
normal skipped steps following the intended failure. All eleven owned files
were restored to their exact committed bytes and modes. Probes took nine
minutes and two seconds, entirely alongside the implementation's CI run.
The version-one manifest SHA-256 is
`c3a4b18e8ecd90dc68c4546daf1cea9fc859cf51dc203f03671a47a43a9ffd3a`;
the final report SHA-256 is
`4e45a931306ab11b29094dae1761d809b261ed5d4948035f9b8a2a318667b405`.

Original native reports from [CI run 38809213695](https://github.com/donvargax/itos-cc/actions/runs/38809213695)
were independently downloaded and checked. Linux and macOS each executed all
seventeen cases and fifty steps successfully. Windows executed fifteen cases
and forty-two steps successfully; SNAP04's three steps and SNAP06's five steps
were explicitly omitted under the existing physical lifecycle limitation.
SNAP07's timestamp cleanup actually ran and passed on all three platforms.
No omitted case is counted as an executed pass.

The complete workflow passed for that exact implementation revision: all
three platform test jobs, all five release builds and publication. Legacy
holders and adapter-only guarantees remain in place for T-101's mapped
cleanup; the new scenarios are not a reason to remove unmatched checks.

T-101 adds `metrics_snapshots.feature` to `InProcess`. The source, root and
format scenarios, SNAP01 to 03 and 08 to 12, run through both drivers, the
memory's mutation run handed the same declared Go tests, whose root-package
command the fixture names as itos-cc runs it (`./.`). SNAP04, 05 and 06 are
`@binary-only` for good: atomic publication under concurrent writers, the raw
reports and the folder the real collectors write under `.metrics/coverage/`,
and concurrent collectors' run directories, none of which a fake writes. SNAP07
stays eligible and skips in process with the memory's typed reason, logged by
the run, until the memory tree records modification times and a fake of the
coverage runner's cleanup reads them. In process the file runs fourteen cases
and forty steps, thirty-seven passing and SNAP07's three skipped.

Worker boundary (T-88): `mutants.Workers` and its model-bearing contracts live
beside mutation values; `worktrees.Workers` owns actual copies/processes and
`mutants/porttest.Workers` holds the in-memory implementation. `Driver.Tests`
selects outcomes by the actual requested directory and command. The fake applies
the requested domain Site to actual virtual source bytes, restores them, and
hashes the resulting copy for observations. Optional runnable test inventories
include unrelated tests: narrow commands select actual files/classes, broad
commands emit the full inventory, and unsupported selector shapes fail. Nested
source traces carry their actual project-relative source identity. No scenario
ID or prewritten event list chooses executions. Physical dependency links and
interpreter behavior remain real-adapter observations.

The memory driver constructed this binding then, but refused a validated
mutation run until T-125 admitted it. Its focused handler-binding guard uses disposable
real source discovery with those same declared fake Workers; it is not evidence
that every mutation scenario already runs entirely in memory. Missing and wrong
handler bindings are exercised independently. Direct legacy library fixture
callers temporarily keep a real-adapter fallback until T-92 (fresh orchestration
until T-95); cmd requires injection and cannot silently choose that fallback.

The old command runner, context, Linux/macOS ownership and scratch tests moved
recognizably to `internal/worktrees`: argv/cwd/environment/streams/elapsed results,
startup/cleanup failure, completed-exit precedence, unrelated processes, zombie
handling and lock-based abandonment guards remain. All existing cmd holders stay.
The public cleanup clauses in MUT166/183/188/193 establish final state; their
private ordering contract is held by `TestPoolJoinsEnteredWaiterBeforeRemovingInputs`
and its identical release-edge oracle's premature-release counterexample.
`TestBoundWorkersShareAbortDeadline` observes deadlines inside actual bound
supervisors: ordinary aggregate cancellation retains local command budgets,
fail-fast mode shares even before an actionable stop (including external parent
cancellation), explicit Abort starts its deadline before cancellation, and frozen trials
share preparation's owner. Existing success/local-timeout guards remain.
`Close` joins entered command cleanup and source restoration even on cancellation;
historical scratch lock/removal failures remain best effort. Native Windows
process-tree ownership remains deferred; normal Windows command behavior is
preserved, not simulated by Linux execution.

Mutation observations (T-89): mutants owns `Toolchain`, `Fingerprints` and
`Reach`, with concrete testtools/testinputs/importgraph adapters. Tool choices
consume independent current facts. `Driver.Tools` verifies installed NodeBin
entries using the existing Stat-only lookup and optionally observes whether the
actual interpreter can import pytest from the fixture caller directory with its
environment. Omitted probe facts differ from explicit failed imports. Memory
creates the declared `.bin` entries and keys probe facts by interpreter, caller
cwd and Python environment; it never infers availability from `Tests.Command`
or verdicts. Shell override avoids a product probe even if fixture setup observed
one. The real self-reexec probe guard proves executable/cwd/environment wiring;
it does not claim a Python runtime was installed for that guard.

Optional `Tests.Imports` holds resolved file-to-file import/reference adjacency
from actual fixture bytes, including explicit no-edge files. These are not
source-to-expected-test lists. Imports-only setup declares neither command nor
outcomes. Root facts survive omitted upserts and conflicting or unknown paths
fail. The memory Reach computes Go's package/one-hop rule and other languages'
transitive closure, Python's applicable conftest files and Kotlin references.
Shared real/fake contracts hold all four languages, external relative support
paths, actual discovery exclusions and changed environment fingerprints.

Memory composes these ports from actual files and `Run.Env`/`Run.Without`; no
host environment supplies fake facts. Actual entered Workers receive those
values and prepend their own copied Python directories before inherited editable
PYTHONPATH. Source and directory read observations derive from actual bound
selection/surroundings/input operations; fixture assertion reads are excluded,
positive controls use those same ports, and removed targets fail observation.
The driver refused a validated mutation run until T-125 admitted it. The focused actual
handler guards use disposable real discovery/config alongside the bound
memory ports; they do not prove full in-memory mutation execution yet.

PATH omissions affect only bare executable lookups; explicit installed paths
remain available. The fake holds simple POSIX missing-command shell exit127
separately from a direct-start failure. Compound shell hiding and uncharacterized
Windows missing-command shell outcomes fail explicitly as unsupported fake
requests. T-125 must settle any needed Windows shell binding before admitting
such execution; there is no silent verdict or permanent logical binary-only tag.
Only partial-result listed-file hash finalization derives caller WithoutCancel
for its old best-effort actual reads. Current freshness/admission/probes and
snapshot publication retain their original caller context.

T90 measurement declarations keep static inventory independent from test coverage.
`Coverage.InventoryReport`, `InventoryExit` and `InventoryOutput` describe actual
Python/TypeScript collector observations and apply only to an entered inventory
request for unloaded files. An explicit empty-file report differs from no report;
listed lines remain unexecuted. Go/Kotlin do not acquire a new static collector.
`ListedTests.ConversionExit` and `ConversionOutput` describe the actual Go
covdata conversion of the owned per-ID `CoverageMetadata` artifact, after it is
written and read. Defaults preserve success; failure removes only that collector's
evidence, retaining valid sibling collectors and IDs. They never script the final
preparation verdict or replace the real binary's converter.

T90 binds mutation coverage through the invocation's separate `MutationCoverage`
capability; aggregate `Coverage` remains the existing port. The real adapter
wraps the unchanged collectors, per-ID split/fallback run, and best-effort
Python/TypeScript inventory. `Test`/`PerTest` now belong to the coverage domain;
legacy aliases keep fresh/supervised preparation unchanged until T95.
The memory binding parses declared Go/LCOV/JaCoCo bytes and independent static
reports, filters subprocess data through the in-process inventory and preserves
per-ID sibling evidence. It observes actual request scope/reach and current
source bytes using the full runnable trace inventory, including unrelated tests.
Unknown fixture sources/selectors fail admission; unexpected request-dependent
observation errors return from `Driver.Run` without changing product failures.
The memory driver refused a validated mutation run until T-125 admitted it;
focused consumer controls use disposable real discovery only, with actual
measurement supplied by the fake. No feature is promoted to `InProcess` here.
Shared named-report/cancellation guards are distinct from successful shared
Go split/fallback and Python static-inventory contracts. The real Go fixture
builds a covered executable in its native temporary root. The Python contract
reports an actual missing-tool omission unless installed-tools mode requires it.
All existing listed/integration/strict inventory and supervised/fresh guards
remain, including declaration retention and contradiction checks; supervised
conversion policy is still owned by T95. Original snapshot/coverage artifacts
and historical mutation records retain their recorded identities.

Listed measurement setup belongs to the fixture root. A later explicit
`Tests.Listed` declaration replaces that root setup deterministically, while
a command upsert omitting it leaves it intact. Retained nested maps, ID slices
and metadata bytes are owned copies; command-keyed Workers outcomes remain
independent. A failed static collector still logs its diagnostic when no
report was written and supplies no inventory proof.

T-91 binds mutation history through `mutants.History`. The memory Driver's
Git setup captures independent staged bytes, immutable commit trees, refs,
parents and tracked working renames; `Head` observes that same state. It does
not take an expected sample seed. Shared real/fake contracts cover divergent
three-dot ranges, caller-directory namespaces, committed bytes despite later
edits, both rename directions, pure deletions and same-name hash pairing.
The bounded fixture setup supports the Givens' add/commit/branch/checkout/tag/mv
operations and errors on unsupported shapes; it runs no Git executable.
The memory driver refused a validated mutation run until T-125 admitted it.
Frozen export/materialization, modes and symlink/gitlink admission remain at
the supervised counted boundary until T-95. All original project history
holders and counted export guards remain; they are not deleted as duplicates.
