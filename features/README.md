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
`coverage.go`, `selection.go`, `cli.go` and `mutate.go`, Go, run by [godog](https://github.com/cucumber/godog)) treat itos-cc as a black
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
in-process driver refuses a command still hand-parsed and an environment variable, naming
`@binary-only`. itos.yaml's scenario lines run both, so every scenario of a moved command is held
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
project as `.venv/bin/python` where it keeps its own virtualenv (`Venv`); and a program the
project's coverage runs besides, such as make (`Program`). A tool missing skips the scenario, or fails it under `ITOS_CC_LANGUAGE_TOOLS`, which the CI job that installs the
pinned tools sets; any other CI job skips the TypeScript and Python ones, its runner's tools being
whatever its image has. The in-process driver records what was declared, for the fake measurer a
command measuring coverage is handed once it moves.

A Given whose project's tests a run of itos-cc runs, mutation run's baseline and trials, declares
what they do (`Tests`): their language and the command itos-cc runs them with, what a failing
baseline prints, the sites whose mutants survive or time out, every other mutant they run being
killed, and how long a run of them takes. The binary driver checks their tools as `Coverage`
does, the language's own (`go`, `node`, `python3`, `java`) and the program the command runs where
the PATH would find it, pytest for a command running it, skipping or failing alike, skips tests
said to take under 100 milliseconds on Windows, where their baseline takes several times that,
and the real tests then decide every outcome; the in-process driver records it, for the fake that runs a
project's tests once the mutation group moves (S7). A folder with no `Tests` declared has no tests
a run may run. A scenario of a counted run, `mutation run --count`, skips where itos-cc makes
none, on any system but Linux and macOS (#29), in either driver (`Counted`). A scenario that only reads which Gradle or
Maven command itos-cc chose runs with `gradle` and `mvn` off the PATH, so neither runs anywhere;
one that needs a symbolic link skips where the system makes none. The in-process driver refuses
both, naming `@binary-only`, and so a build of itos-cc and a port held: the binary driver builds
into the scenario's folder with -trimpath, `go install` from a module proxy it writes there,
the tree's files zipped beside the module cache's downloads, and holds a port of 127.0.0.1,
skipping where the system lets a second listener take it.

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
`mutate.feature`, and `count.feature`, the
template's sample, all of whose scenarios are `@wip`. `mutate.feature` converts a piece at a
time (T-70 to T-81): the scenarios of the pieces whose steps are not written yet are listed in
`Unstepped` (`steps.go`), by their IDs, an entry for each piece, and run nowhere. A selection
naming only them logs them as not run, their steps not written yet, and passes; a tag other
scenarios carry too selects those others alone; and an ID `Unstepped` lists that no scenario of a
converted file carries fails every selection. The Go tests of `cmd/itos-cc` hold those scenarios
until their piece. The in-process driver runs those of
`InProcess`: all of them but `cli.feature`, whose help, version and mutation scenarios need
commands that move later, which joins it in S9, and `mutate.feature`, which joins it when the
mutation group moves (S7). Every other file converts to godog over time,
before or with the command that moves onto kong (q-52), its scenarios' text unchanged; until then
the Go tests of `cmd/itos-cc` hold its scenarios. A selection that names a scenario of such a
file, as the smoke set does for every file, logs it as not run here and runs the rest; one that
names nothing a converted file holds fails, as one matching no tag does. In process, a selection
naming a scenario of a converted file not in `InProcess` logs it as binary driver only until its
command moves, and one naming nothing else runs nothing there and passes, the binary driver
running it.

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
