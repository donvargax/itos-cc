# Go design and quality

This guide adds what is specific to Go to `docs/engineering.md`, which it does not repeat.
It describes what this project actually holds and checks: the Go foundation and the command
line built on it. The command line's interface contract belongs in its own guide. The
policy is harvested from [itos-template](https://github.com/donvargax/itos-template) (its
decisions on thin layers over a domain, unit tests with fakes, rapid properties and the
OS as a value), adapted to this module with its owner's permission (`CONTRIBUTORS.md`).

## Packages

The code follows decision 29 (`docs/decisions/`): a shared domain behind ports, adapters apart
from it, and thin slices named for their commands.

- `internal/counting` is the domain. It owns counting and its result, and reads input only
  through its port. A function that calls the port takes a `context.Context` first (decision
  30); pure logic, such as counting bytes, takes none.
- `internal/counting/port` declares `Files` and its sealed set of failures: `Missing`,
  `Unreadable` and `Cancelled`. `Files.Read` takes the context first.
- `internal/counting/port/porttest` holds our in-memory fake of `Files` for the tests.
- `internal/selection` is the domain every command chooses its files through: paths given as
  arguments, a path that is not there taken as a fragment of a file's path under the working
  directory, `--changed` adding git's changed files, and no paths meaning the whole working
  directory. `Select(ctx, tree, paths, changed)` reads the tree only through its port.
- `internal/selection/port` declares `Tree` (`Discover` roots into sources and tests,
  `Changed` files, whether a path `Exists`, and `Read`, what a selected file holds, each
  taking the context first) and its sealed set of failures: `NoGit`, `Unmatched`, carrying
  the fragment, `Unwalkable`, `Unreadable` and `Cancelled`. Every command reads the files it
  selects, so it reads them through the tree it chose them from, never by itself.
  `internal/selection/port/porttest` holds the in-memory fake of `Tree`.
- `internal/snapshot` is the domain of the snapshots under `.metrics/` that the commands
  share and projects commit (decision 41): the format `Version` every snapshot records, and
  `Key(root, abs)`, an absolute path named as a snapshot records it, from the project root and
  slash-separated, a path the root cannot reach staying whole. The paths a selection discovers
  are absolute, so naming one needs no working directory. `project.FromRootOf` names paths
  through it, so the arithmetic has one copy.
- `internal/snapshot/port` declares `Store`: the project `Root`, and `Read` and `Write` of a
  snapshot by name, each taking the context first, and its sealed set of failures:
  `Unreadable`, `Unwritable`, each carrying the snapshot's name and the library's error text
  whole as its `Reason`, and `Cancelled`. `internal/snapshot/port/porttest` holds the
  in-memory fake of `Store`.
- `internal/disk` and `internal/stdin` are the adapters. `disk` reads real files and translates
  OS errors into the port's failures; `stdin` reads standard input for `-`, stops with
  `Cancelled` once the context is done, and hands every other path to a fallback it is given.
  Each imports nothing of ours but the port.
- `internal/workdir` is the selection port's adapter: the working directory as the disk and git
  hold it. It wraps the top-level `project` library's discovery and git query, stats paths and
  reads files, translating their errors into the port's failures. Beside `Tree` it implements
  `internal/lang`'s port as `Surroundings`, answering from `project.Surroundings`, the disk. It
  imports nothing else of ours.
- `internal/metricsdir` is the snapshot port's adapter: the snapshots under `.metrics/` at the
  project root of the working directory. It wraps the top-level `metrics` library, which
  writes them as indented JSON renamed into place, and `project`, which finds the root, so
  every byte and every atomic write stays the library's, translating their errors into the
  port's failures. It imports nothing else of ours.
- `internal/lang` is the measure library every command shares, a domain package (decision 37):
  it parses a source and names its units, their namespaces, complexity, and each language's
  syntax and mutation rules. Every language's rules for a namespace (the markers it walks up
  to, the go.mod module line, the package clause) are its own, but it reads nothing (decisions
  42 and 43). `internal/lang/port` declares `Surroundings`, what it asks about
  what surrounds a file: whether a path `Exists`, what a file holds (`Read`), the
  `WorkingDir` and a path made absolute (`Abs`), each taking the context first and answering
  false where the disk could not. `Parse(ctx, around, …)` and `UnitsOf(ctx, around, …)` return
  the context's error once it is done. `internal/lang/port/porttest` holds the in-memory fake its
  tests parse through. The top-level `project` library is its disk side: `project.Surroundings`,
  making the calls lang once made itself, and `ParseFile` and `FindUp`, which the packages not
  yet refactored
  call, passing `context.TODO()` until stage S4 threads them a context.
- `internal/duplication` is the domain that finds functions similar enough to be duplicates
  (decision 37), named for what it is while the command keeps its name, `dry`. It takes files
  `internal/lang` parsed, `Forms(f, name, opt)` a file's functions large enough to compare,
  with the name its caller gives the file, since a side's file is the key its pairs and groups
  are sorted and paired by. It reads nothing and imports neither `project` nor any disk code,
  so it has no port; its tests parse in memory through lang's fake. `internal/teststructure`
  measures test code with its `Fingerprint` and `Jaccard`.
- `internal/teststructure` is the domain that measures the structure of test code (decision
  37), named for what it is while the command keeps its name, `scrap`. `Analyze(f, name)`
  takes a file `internal/lang` parsed and the name its caller gives it, and `History` merges a
  run's reports into what the last snapshot recorded, keeping the files the run did not
  measure. It reads nothing and has no port; its tests parse in memory through lang's fake.
- `internal/count` is the `count` command's slice, named for the command. Its kong struct and
  `Run` are the UI; its `Query`, carrying domain values, and `Handler.Handle(ctx, q)`, which
  only calls the domain through the port, are the application. `Run` assembling the `Query`
  from kong's flags is the one copy the layout accepts. Its `Classify` is the counting
  domain's classifier, turning the port's sealed failures into problems.
- `internal/units` is the `units` command's slice, the first of itos-cc's own commands moved
  onto kong (T-32), laid out as `count`'s: its kong struct `CLI` with its `Contract` for the
  help, its `Query`, and `Handler.Handle(ctx, q)`, which selects the files through the
  selection domain, reads each through the same `Tree` port and names its units with
  `internal/lang`, through the `Surroundings` port its handler is given. `Run` prints them,
  their paths from the working directory (`UI.Rel`). It has no classifier: its failures are the
  selection domain's, whose classifier is cmd's, or `internal/lang`'s, internal ones.
- `internal/dry` is the `dry` command's slice (T-40), laid out as `units`'s, the first moved
  command with number flags. Its `Handler` holds the selection `Tree`, lang's `Surroundings`,
  the snapshot `Store` and `Name`, the name a person reads a file by, which cmd binds to
  `UI.Rel`. `Handle` selects the sources, and with paths or `--changed` checks them against
  every source the selection finds under the working directory; it reads each through the
  `Tree`, parses it with `internal/lang`, hands it to `internal/duplication` under its `Name`,
  and records `dry.json` through the `Store`, each file named from the root
  (`snapshot.Key`). It has no classifier: its failures are the selection and snapshot
  domains', whose classifiers are cmd's, or `internal/lang`'s, internal ones.
- `internal/scrap` is the `scrap` command's slice (T-43), laid out as `dry`'s, its `Handler`
  holding the same four. `Handle` selects the test files, reads the last `scrap.json` through
  the `Store`, going on without it when it is `Unreadable`, reads and parses each test file as
  `dry` does, measures it with `internal/teststructure` under its `Name`, merges the run into
  the last snapshot with the domain's `History` and records it, each file keyed from the root.
  `Run` prints each file's report, and says on stderr, with `--json` too, when the last
  snapshot was ignored or there was no test file to measure. It has no classifier.
- `internal/cli` is the UI every slice shares, and imports nothing of ours. It alone turns a
  run's problems into its exit code, by the precedence `docs/CLI.md` gives, and prints the
  plain or JSON answer, and it stops a command on Ctrl-C (decision 38). A domain's failures
  reach it through that domain's classifier, which cmd hands it. For the commands kong parses
  it reads the command line (`Read`, `Parse`), turns kong's failures into problems by their
  types (`ParseProblem`) and prints help (`Help`, a command's `Page`), as "Moving a command"
  below says.
- `cmd/itos-cc` assembles everything: it makes the context, feeds the interrupts to
  `internal/cli`, hands it each domain's classifier, gives `stdin` the disk as its fallback,
  and hands the context and the handler, with `workdir`'s `Tree` and `Surroundings`, to `Run`
  through kong's bindings. It is the only place
  that knows every package. It routes a command moved onto kong (`moved.go`) through kong and
  every other through its own parser (`cli.go`) until it moves. Until every command is a slice,
  it also holds the selection domain's classifier, `classifySelection`, since every command
  selects its files, and `files`, which calls the domain through `workdir` for the hand-parsed
  ones. It holds the snapshot domain's too, `classifySnapshot`, since several commands share
  it, and `snapshots`, the `Store` `internal/metricsdir` gives `dry`'s and `scrap`'s handlers:
  a snapshot that cannot be written is the internal 70 it always was, and `scrap` warns about
  an unreadable last one and goes on. `crap`, `mutation` and `serve` still call the `metrics`
  library until each moves (decision 41).
- `internal/version` says what the build is, and `internal/release` is the library of the
  release tools that `tools/bin/release-version` and `tools/bin/release-notes` run. Neither is
  a command.

The domain is shared, not part of any command, so a second command reuses counting through its
port without reaching into another command's package. A context carries cancellation and
deadlines only, always as the first parameter and never in a struct or as a value.

`.golangci.yml` holds these arrows with depguard:

- `domain`: the domains, their ports and their tests import nothing of ours but the domains and
  their port subpackages, and never kong.
- `domain-io`: the domain's own code does no I/O, neither `os` nor `net`. Its tests may read
  fixtures.
- `slices`: a slice imports nothing of ours but the domains, `internal/cli` and
  `internal/lang`, the domain library naming a source's units that every command measures by,
  with its port and the port's fake, so never another slice, an adapter, the `project` library
  an adapter wraps and lang's disk side, or `cmd`. Its files are every package under `internal/` except the ones the
  rule lists as something else, so a new command's slice is held with no edit. A slice's tests
  are in its own package, since the rule cannot tell a package's import of itself from another's.
- `ui`: `internal/cli` and its tests import nothing of ours, so adding or removing a domain
  touches no file of the UI (q-54).
- `infra`, `workdir` and `metricsdir`: an adapter imports nothing of ours but the port it
  implements, `internal/workdir` the two it implements, selection's and `internal/lang`'s, and
  the `project` library it wraps, and `internal/metricsdir` the snapshot port and the `metrics`
  and `project` libraries it wraps; only its tests may use `porttest`. Each port's
  adapters have a rule of their own, so none is allowed another's port or library.
- `mocks`: no mocking library anywhere. Fakes live in `porttest`.
- `rapid`: the property library only in the domains' own tests.
- `kong`: kong only in `cmd` and `internal/cli`. A slice's struct tags need no import.

### Adding a package

Lint refusing an import after you add a package means a rule has not been told what the package
is. Extend the rule as below; never loosen it, since every arrow above is what keeps the next
command from reaching into this one.

- A command: add its slice as `internal/<command>`. The `slices` rule holds it already. Add it
  to `cmd`'s bindings and give it scenarios.
- A domain package: add its folder as a `**/internal/<name>/**` glob to `domain` and
  `domain-io`, as a `!**/internal/<name>/**` exclusion to `slices`, and its import path to the
  `allow` lists of `domain` and of `slices` if commands use it. Give it a `port` subpackage for
  what it reaches outside, and a classifier for its failures beside the slice that uses it,
  which `cmd` adds to the UI's classifiers. `tools/bin/domain-coverage` reads its list from `domain`'s `files`,
  so the package is held to 80% from then on. If its tests use rapid, add a
  `!**/internal/<name>/*_test.go` exclusion to `rapid`.
- An adapter: add its folder to the `files` of the rule for its port's adapters (`infra` for
  counting's, `workdir` for selection's, `metricsdir` for the snapshot port's), or give a new
  port's adapters a rule of their own allowing that port and any library they wrap; add a
  `!**/internal/<name>/**` exclusion to `slices`. Wire it in `cmd`.
- Any other package that is no command, such as `internal/version`: add a `slices` exclusion
  saying what it is.
- Then add compile-valid fixtures to `tools/testdata/architecture` that each rule you touched
  must accept and refuse. `tools/bin/architecture-check` runs the pinned linter over them, so a
  rule that stops matching fails there instead of passing silently.

A new sealed failure set, an interface with an unexported marker method like
`port.ReadFailure`, is annotated `//sumtype:decl` and given its exit codes in one switch that
names every kind, in its domain's classifier, as `internal/count`'s `Classify` gives the
counting port's. gochecksumtype then refuses the switch when a kind is
added and left out; without the annotation nothing checks it.

Code that differs by OS takes the OS as a value. Logic never branches on `runtime.GOOS`. A
build-tagged file holds only a call that one OS lacks, with no mutation site: the scenarios'
interrupt step is the example, `os.Interrupt` on Unix and `CTRL_BREAK_EVENT` on Windows.

## Moving a command

A command parsed by hand moves onto kong as a slice of its own, printing the same bytes and
exiting with the same codes as before: a refactor, no behaviour change. `units` moved first
(T-32), and every step below points at what it did. A project made from go-template-itos
follows the same steps for a command it wrote before it had the template's design.

1. **The slice.** Make `internal/<command>/<command>.go`, as `internal/units` is laid out:
   - `CLI`, the kong struct: one field per flag, in the order help lists them, each with its
     `help` tag; `JSON bool` named `json`; the arguments last
     (`arg:"" optional:"" name:"path"`). Its tags need no import of kong.
   - `Contract() cli.Contract`: what the help says beyond the flags, its `About`, the keys
     `--json` adds, its problem rules, its exit codes and examples, copied from the old
     `command` value word for word.
   - `Query`, the command line's values as the domain takes them, and `Handler`, holding the
     ports cmd gives it. `Handle(ctx, q)` only calls the domain through them.
   - `Run(ctx, ui, h) int`: builds the `Query`, calls `Handle`, prints through `ui`, and returns
     `ui.Emit`'s exit code, with the data printed so far beside a failure's problems, as the
     old command returned it.
   - Tests in the same package, with the domain's `porttest` fake, for what the scenarios
     cannot reach, such as a file that cannot be read.
2. **Its I/O goes through a port.** A slice reads, walks and runs nothing itself (decision
   29). When it needs something of the outside that every command of the domain needs too,
   the domain's port grows it, as the selection port's `Tree` grew `Read` for the sources
   `units` parses: one port the adapter already implements, never a port of the slice's own,
   which an adapter could not implement without importing a slice. A path a person reads is
   named from the working directory by `ui.Rel`, not by a library an adapter wraps.
3. **Its classifier.** A domain's failures become problems in one classifier, held beside the
   one slice that uses the domain (`count.Classify`). A domain several commands share keeps
   its classifier in cmd (`classifySelection`), since no slice may import another; the
   moved slice then needs none. internal/cli never holds one.
4. **Routing in cmd.** In `cmd/itos-cc/moved.go`, add the slice's struct to `commandLine`,
   its `help` tag the line the top-level help lists, its handler to `parserOf`'s bindings,
   and a case to `runMoved` that calls its `Run` through `callSafely`. Remove the old
   `command` from `main.go`'s list and delete its file. `run` sends a moved command to kong
   and every other to the hand parser; the top-level help, `help <command>` and the guess
   for an unknown command know both. A group, such as `mutation`, moves whole, and
   `ParseProblem`'s `command.unknown` then takes the group's wording, which names its
   subcommands.
5. **The command line.** `cli.Read` finds `--json` and help (`-h` or `--help` anywhere
   before `--`) before anything is judged, so help wins over any mistake and a usage error
   prints the object `--json` asks for. `cli.Parse` holds kong to what it does not hold by
   itself: a word of one dash, other than `-` and `-h`, is a long flag mistyped, found
   against kong's model unless what precedes it fails first (q-55); arguments may stand
   between flags, so they move behind them before kong reads them. `cli.Flags` refuses a
   switch given a value, a flag given no value and a flag given twice, the second time
   before its value is judged. It reads switches, string values and numbers: a float64 flag
   by `strconv.ParseFloat` and an int flag by `strconv.Atoi`, as the hand parser read them,
   refusing what is none as a `cli.NumberError` in its words (`--threshold needs a number,
   and was given "abc"`; `a whole number` for an int), added when `dry` moved (T-40). A
   command whose flags take durations or a switch's choices (`crap`, `mutation run`) first
   gives `Flags` a mapper for that kind, with a typed refusal in the words the hand parser
   used.
6. **Failures by type, never by text.** `cli.ParseProblem` turns each failure into the
   problem `docs/CLI.md` names, by the kong fork's error types and `Flags`' own:

   | Failure                                             | Problem                 |
   | --------------------------------------------------- | ----------------------- |
   | `kong.UnknownFlagError`, `cli.MistypedFlagError`    | `flags.unknown`, the flag as written and the one meant |
   | `cli.SwitchValueError`                              | `flags.switch-value`    |
   | `cli.ValueMissingError`, `kong.MissingValueError`   | `flags.value-missing`   |
   | `cli.RepeatedFlagError`                             | `flags.repeated`        |
   | `cli.NumberError`                                   | `flags.value-invalid`, in the hand parser's words |
   | `kong.InvalidValueError`, `kong.EnumError`          | `flags.value-invalid`, in kong's words until a mapper of ours words it |
   | `kong.UnexpectedArgumentError`, a command expected  | `command.unknown`       |
   | `kong.UnexpectedArgumentError`, no argument taken   | `args.unexpected`       |
   | `kong.MissingCommandError`                          | `command.missing`       |

   Any other failure of the command line is a usage problem in kong's words, and an error
   that is no parse failure is internal. A moved command that meets a kind with no wording
   yet gives it one here, from `features/cli.feature` and `docs/CLI.md`, never by matching
   kong's text.
7. **Help.** kong's printer is replaced: `kong.Help(cli.Help)` prints the selected command's
   `cli.Page`, from kong's model (its flags, the command's own and then the program's, and
   its arguments for the usage line) and its `Contract`, in the layout the hand-parsed
   commands print through the same `Page`. The help flag's text is set in `parserOf`. Help
   for `<command> --help` and `help <command>` both go through `movedHelp`.
8. **Its scenarios run through godog.** Convert the command's feature file: its steps in
   `features/<file>_test.go`, registered from `initializeScenario`, reading only what the
   binary prints and writes, and the file added to `converted` in `features_test.go`. A live
   scenario's text never changes outside a `feat` or a `fix`; write the step for the
   sentence as it stands, or stop and say which scenario cannot be run as written. Check
   each step can fail: change an expectation in the file locally, see the step fail, and
   restore it. The file's smoke scenario in `smoke.yaml` then runs in every push's scenario
   step instead of being logged as not run.
9. **The proof comes once, after the whole refactor.** A move does not compare binaries on
   its own: its scenarios and tests hold it. Once every command has moved, one task builds
   the binary from before the refactor and the refactored one, and runs both over many
   command lines of every command: every flag alone, with `=value` and given twice, unknown
   and mistyped flags (`-changed`, `--chnged`), `-h` and `--help` anywhere, after `--` and
   beside mistakes, `--json` before the command, `help <command>`, arguments between flags,
   paths that are there, fragments and nothing, in a git repository, in a plain folder and
   beside a file that cannot be read. It compares stdout, stderr and the exit code of each,
   and every line that differs is either a deliberate change that task lists or a bug. A move
   that finds a case worth comparing adds it to that task's list. `units` (T-32) was compared
   on its own before this: 236 lines agreed but one, `---changed`, which the hand parser read
   as `--changed` and kong refuses as an unknown flag, as rule 15 has it.
10. **Boundaries.** The `slices` rule holds the new package with no edit. A top-level library
    it imports that the rule refuses is either a domain library every command measures by,
    allowed in `slices` with a comment saying why (`lang` was, until T-36 made it the domain
    `internal/lang`), or one an adapter wraps, which stays refused (`project`). Add the slice to the fixture lists of
    `tools/testdata/architecture/check_test.go`, with every library you allowed or refused,
    and run `tools/bin/architecture-check` and `tools/bin/pinned golangci-lint run
    ./internal/...`.
11. **Then the old tests.** Once CI has run the converted scenarios green, remove only the Go
    tests of `cmd/itos-cc` that are the same check as a scenario; keep any that holds what no
    scenario reaches.

## Tests

Each domain's unit tests use its `porttest` fake and assert results and typed failures.
The disk and workdir adapters' tests use real temporary files, and workdir's a real git
repository, since translating OS and git errors is what an adapter does. The `count` slice's tests use the fake too, standard input being its `-`
key; reading standard input, and stopping when the context is done, is `internal/stdin`'s,
held by its own tests. The scenarios in `features/` run the built binary through godog,
interrupting it once it waits on an open standard input to show the 130 exit. A second Ctrl-C
ending the process at once is held by `internal/cli`'s unit tests, which feed it fake
interrupts on every system: `count` has no cleanup for a scenario to see skipped.

Properties use `pgregory.net/rapid` and live in the domain's tests. The counting property
joins newline-terminated pieces and checks that their counts add. Arbitrary concatenation
can join two words, so it is not promised to be additive. The selection property checks
that paths and fragments only narrow the working directory: what they select is exactly the
files of `.` under a path that is there or holding one that is not. Every run sets
`RAPID_CHECKS=1`: one new random case per property, as fast as an example test, with the
seed printed on failure. This project has no scheduled high-count run.

Tests run on Linux, macOS and Windows in CI's platform jobs. The import-boundary self-test
runs there as well, because it checks paths and diagnostics that differ by platform.

## Quality checks

`itos.yaml`'s CI plan runs these checks in order, and its generated rules in `AGENTS.md`
list them exactly:

- `gofmt` over `cmd`, `internal` and `features`, `go build`, `go vet`, `go mod verify`
  against `go.sum`, and `go mod tidy -diff` so the module files match the code.
- `itos tests smoke check scenario`, so every feature file has a smoke scenario.
- golangci-lint over `internal/`, widening as packages move there (q-50): the standard
  linters plus depguard, gosec on non-test code, revive's file-length limit and
  gochecksumtype, which refuses a type switch over a sealed set that leaves a kind out, a
  default arm not counting.
- `tools/bin/architecture-check`, the self-test above.
- `go tool govulncheck -test ./...`, covering the dependencies in use, test-only ones and
  the standard library.
- The unit tests of `cmd` and `internal` with `RAPID_CHECKS=1`, then the release helpers'
  tests, over example repositories.
- `tools/bin/domain-coverage`, which requires each domain package to reach 80% statement
  coverage from its own tests.
- The scenarios, run once: the smoke set and those the push's range names.
- `itos-cc mutation sample --count 10`, a sample of recorded mutation results. It is not
  proof of the changed code.

Changed-code proof is judged in CI. Before pushing code, run
`tools/bin/pinned itos-cc mutation run --since <base> --fail-uncovered --all-tests
--no-annotate cmd internal`, where the base is the push's range start, the nearest ancestor
with a green CI run (`itos ci range --head HEAD` prints it, given GITHUB_REPOSITORY and
GITHUB_TOKEN), and commit `.metrics/mutate/`. When a push's range touches `cmd/**/*.go` or
`internal/**/*.go`, `itos ci run` runs `itos.yaml`'s `proof.code` check from that base as a
static step, before the tests, and fails while a mutant survives, is uncovered or has no
result. `itos work done` runs no proof; it needs that green run.

Tools are pinned:

- The Go toolchain is pinned in `go.mod`, and govulncheck is a `tool` dependency there.
- `tools/bin/pinned` fetches golangci-lint, itos-cc, gitleaks, GoReleaser and git-cliff at
  fixed versions and checks each against a pinned SHA-256, copied from its release's own
  checksums or, for git-cliff, taken once the archive matched its release's `.sha512`.
- Workflow actions are pinned by digest.

`docs/upgrading.md` says how to move any of these pins.

## Setup

A new project has no `itos.yaml` or `tasks` yet but has `itos-policy.yaml`: its commit rules,
CI plan and code-proof gate. `itos-template new` prints the setup steps that adopt it.
`itos init --policy itos-policy.yaml` gives the project its own `itos.yaml` and a fresh ledger,
keeping its smoke set, then the policy file is removed. The last step,
`itos init --agent-rules`, rewrites `AGENTS.md`'s rules to describe the project's own config.
From then on the workflow runs the Go steps, the scenarios and the Linux, macOS and Windows
tests on the project's own `main`, and the release job after them, as `docs/CLI.md`
describes.

The project starts with no mutation records: the template's are taken over its own names and
stay behind. Setup makes the render commit the point judging starts from, so the first push
checks no code. A change to a function needs that function's results: run `itos-cc mutation run`
as the code proof asks of any change, and commit `.metrics/mutate/` with the change.
`itos-cc.yaml` comes with the template's reviewed exceptions. Most still hold, their hashes
covering a function's body; one whose function holds the project's name, such as `cmd`'s `run`,
goes stale, and its reviewed reason is re-applied with `itos-cc mutation except` when that
function next changes.
