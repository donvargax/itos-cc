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

The steps (`steps.go`, `units.go`, `dry.go`, `scrap.go`, `crap.go` and `complexity.go`, Go, run by
[godog](https://github.com/cucumber/godog)) treat itos-cc as a black box. They reach the world
only through a `Driver` (`steps.go`): a fresh folder, files written and read, git, runs of
itos-cc with a folder, environment, stdin and arguments, and what a project's own coverage tools
measure in a folder (`Coverage`), which a Given declares before a run measures with them. They are ordinary files of package
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
viewer's, linked into the project; Python `python3` with pytest and coverage. A tool missing skips
the scenario, or fails it under `ITOS_CC_LANGUAGE_TOOLS`, which the CI job that installs the
pinned tools sets; any other CI job skips the TypeScript and Python ones, its runner's tools being
whatever its image has. The in-process driver records what was declared, for the fake measurer a
command measuring coverage is handed once it moves.

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
`complexity.feature`, and `count.feature`, the template's sample, all of whose scenarios are
`@wip`. The in-process driver runs those of `InProcess`: all of them but `crap.feature` and
`complexity.feature`, which join it when crap moves. Every other file converts to godog over time,
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
cannot measure and no tool needs to be installed to show. Step patterns are global across the files, so each file's own setup is a
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
| `stderr says "…"`                                                                     | Containment, on stderr; `<dir>` in it the folder of the Given's project as itos-cc names it, `...` the rest of what it says there. |
| `the exit code is <n>`                                                                | The binary's exit status.                                         |
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
