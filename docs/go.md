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
- `internal/counting/port/porttest` holds our in-memory fake of `Files` for the tests, at an
  absolute `Dir` a relative path is from, as the other ports' fakes are, and `FilesContract`,
  the contract it and `internal/disk`'s `Files` both pass (Tests, below).
- `internal/selection` is the domain every command chooses its files through: paths given as
  arguments, a path that is not there taken as a fragment of a file's path under the working
  directory, `--changed` adding git's changed files, and no paths meaning the whole working
  directory. `Select(ctx, tree, paths, changed, exclude)` reads the tree only through its port,
  then leaves out the sources `exclude` names, never a test, counting them in `Files.Excluded`.
  `Parse(ctx, tree, around, path)` is a selected file parsed, the one copy every command that
  measures what it selects uses (T-63): a language `internal/lang` does not know is refused
  before the read, then the tree reads it and lang parses it through its `Surroundings` port.
  `Exclusion{Root, Globs}` is itos-cc.yaml's `exclude` as a value, which cmd and the slices'
  handlers are given, read at the edge: `Excludes(path)` matches a path's slash form from
  `Root` against the doublestar globs, by `filepath.Rel` alone, no I/O.
- `internal/selection/port` declares `Tree` (`Discover` roots into sources and tests,
  `Changed` files, whether a path `Exists`, and `Read`, what a selected file holds, each
  taking the context first) and its sealed set of failures: `NoGit`, `Unmatched`, carrying
  the fragment, `Unwalkable`, `Unreadable` and `Cancelled`. Every command reads the files it
  selects, so it reads them through the tree it chose them from, never by itself.
  `Discover` gives absolute paths, which `snapshot.Key` names from the root.
  `internal/selection/port/porttest` holds the in-memory fake of `Tree`, at an absolute
  directory it names and never touches, and `TreeOf`, which lays one out from files' text,
  classified by `internal/lang` as the disk adapter classifies them, and `TreeContract`, the
  contract it and `internal/workdir`'s `Tree` both pass (Tests, below).
- `internal/snapshot` is the domain of the snapshots under `.metrics/` that the commands
  share and projects commit (decision 41): the `Folder` they live in, the format `Version`
  every snapshot records, and `Key(root, abs)`, an absolute path named as a snapshot records
  it, from the project root and slash-separated, a path the root cannot reach staying whole.
  The paths a selection discovers are absolute, so naming one needs no working directory.
  `project.FromRootOf` names paths through it, and `metrics.Name` and `metrics.Version` are
  its, so each has one copy.
- `internal/snapshot/port` declares `Store`: the project `Root`, `Read`, `Write` and `Remove`
  of a snapshot by name, its path from `.metrics/`, a removal taking the folders it empties
  below the top one (`mutate/` stays), and `List`, the names of the regular `*.json` files
  under a folder, each taking the context first, and its sealed set of failures:
  `Unreadable`, `Unwritable` and `Unremovable`, each carrying the snapshot's name, and
  `Unlistable`, the folder's, with the library's error text whole as its `Reason`, and
  `Cancelled`. `internal/snapshot/port/porttest` holds the in-memory fake of `Store`, writing
  the bytes the `metrics` library writes, and `StoreContract`, the contract it and
  `internal/metricsdir`'s `Store` both pass (Tests, below).
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
  project root of the working directory, or at the root its `RootDir` names, so one `Store` for
  each root serves several projects (graph's, in S8) with no change to the port. It wraps the
  top-level `metrics` library, which writes them as indented JSON renamed into place, removes
  and lists them, and `project`, which finds the root, so
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
  tests parse through, at an absolute `Dir` a relative path is from, a path rooted with no
  volume such as `/work` on `Dir`'s volume so a test reads the same on every system, and
  `SurroundingsContract`, the contract it and both adapters, `project`'s and
  `internal/workdir`'s, pass (Tests, below). The top-level `project` library is its disk side: `project.Surroundings`,
  making the calls lang once made itself, and `ParseFile` and `FindUp`, which the packages not
  yet refactored call with the context their caller gives them.
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
- `internal/changerisk` is the domain that scores functions by CRAP (decision 37), named for
  what it is while the command keeps its name, `crap`. `Analyze(f, name, report, measured)`
  takes a file `internal/lang` parsed, the name its caller gives it and a coverage `Report`;
  `Measured(report, files)` is the languages the report speaks for among every file a run
  scores, so a file it never names is untested rather than unknown. It reads nothing and has
  no port; its tests parse in memory through lang's fake. The `crap` slice reads and
  parses each source through the selection domain's `Parse`.
- `internal/coverage` is the coverage domain (decisions 37 and 46): the `Report` that answers
  which lines a run covered, a function's fraction and which listed tests reach a line, its
  queries, and the parsers of LCOV, Go cover profiles and JaCoCo XML (`Parse` choosing by the
  content). It reads nothing. The top-level `coverage` runner, which runs each language's
  tools, builds every `Report` through the domain's API, never its fields (decision 47):
  `Build(ctx, files, …)` and `Merge`, `FromLines` and `FromGoBlocks`, the setters
  `SetMissing`, `SetIntegrationMissing`, `MarkMeasured` and `Prove`, `List` for a static
  inventory's lines, `Integrated` for the data of the processes the tests started, and
  `TestCoverage.Add` with `SetTests`; the zero `Report` is empty. The runner keeps `Load`,
  which opens a report, `Disk`, the `Files` port over the disk, and `Inventory`, which runs a
  static inventory command. An `Unmeasured` is an error, its `String`, so a command hands it
  to the UI, whose classifier makes it its gap's problem. Its tests are package
  `coverage_test`, since its `porttest` imports it.
- `internal/coverage/port` declares `Files`, what the report-path matcher asks of the disk:
  `SameFile(ctx, path, among)`, whether a path names a regular file and which of the sources
  of its name it is, reached through a symlinked directory too. It declares the sealed set of
  `Gap`s an `Unmeasured` build root or report has as its `Cause`: `ToolMissing`,
  `MeasuredNothing`, `NoReport`, `Unreadable` and `Cancelled`, each written as its name. It
  declares `Measurer[R]`, which the commands measure coverage through (decision 46):
  `Measure(ctx, Request)` gives the domain's `*coverage.Report`, each build root or report it
  could not measure in its `Missing`, or a sealed `Failure` of the whole measurement:
  `Cancelled`, the gap, once the context is done, and `Unwritable`, a run folder that could
  not be made. `R` is always `*coverage.Report`: the port names the domain's type as a type
  argument, since the domain imports the port for `Files` and `Gap` and Go refuses the cycle
  an import back would make. `Request` carries what crap's coverage flags say: the
  `Sources`, `--coverage-report`'s `Reports`, `--coverage-command`'s `Command`,
  `--use-existing-coverage`'s `Existing`, `--all-tests`' `AllTests`, the `Scope` of tests
  that run without it and the `Reach` of the test files that reach each source.
  `internal/coverage/port/porttest` holds the in-memory fakes: `Files`, with links standing
  for symlinks, and `Measurer`, which reads the reports a request names from the files it
  holds and gives a request that names none the canned reports of its build roots; and
  `FilesContract` and `MeasurerContract`, the contracts each and its adapter pass.
- `internal/coveragetools` is the coverage port's adapter (decision 46). Its `Measurer`
  wraps the top-level `coverage` runner, its process groups and the run folders under
  `.metrics/coverage` the tools write their reports in, one for each run, the latest kept for
  `--use-existing-coverage`, and runs `--coverage-command` through the runner's `Shell`. It
  returns the domain's `Report` as the runner built it, with no mapper (decision 47), and
  writes what it runs and the tools' output only to the `Log` main gives it. Its
  `MeasureTests` measures each listed test's coverage too, mutation's until it moves.
- `internal/count` is the `count` command's slice, named for the command. Its kong struct and
  `Run` are the UI; its `Query`, carrying domain values, and `Handler.Handle(ctx, q)`, which
  only calls the domain through the port, are the application. `Run` assembling the `Query`
  from kong's flags is the one copy the layout accepts. Its `Classify` is the counting
  domain's classifier, turning the port's sealed failures into problems.
- `internal/units` is the `units` command's slice, the first of itos-cc's own commands moved
  onto kong (T-32), laid out as `count`'s: its kong struct `CLI` with its `Contract` for the
  help, its `Query`, and `Handler.Handle(ctx, q)`, which selects the files through the
  selection domain, parses each with the domain's `Parse`, read through the same `Tree` port
  and named by `internal/lang` through the `Surroundings` port its handler is given, its
  `Exclude` the sources the selection leaves out. `Run` prints them, their paths from the
  working directory (`UI.Rel`). It has no classifier: its failures are the
  selection domain's, whose classifier is cmd's, or `internal/lang`'s, internal ones.
- `internal/dry` is the `dry` command's slice (T-40), laid out as `units`'s, the first moved
  command with number flags. Its `Handler` holds the selection `Tree`, lang's `Surroundings`,
  the snapshot `Store` and `Name`, the name a person reads a file by, which cmd binds to
  `UI.Rel`, and `Exclude`, the sources both selections leave out. `Handle` selects the
  sources, and with paths or `--changed` checks them against every source the selection finds
  under the working directory; it parses each with the selection domain's `Parse`, hands it
  to `internal/duplication` under its `Name`, and records `dry.json` through the `Store`, each
  file named from the root (`snapshot.Key`). It has no classifier: its failures are the selection and snapshot
  domains', whose classifiers are cmd's, or `internal/lang`'s, internal ones.
- `internal/scrap` is the `scrap` command's slice (T-43), laid out as `dry`'s, its `Handler`
  holding the same four. `Handle` selects the test files, reads the last `scrap.json` through
  the `Store`, going on without it when it is `Unreadable`, reads and parses each test file as
  `dry` does, measures it with `internal/teststructure` under its `Name`, merges the run into
  the last snapshot with the domain's `History` and records it, each file keyed from the root.
  `Run` prints each file's report, and says on stderr, with `--json` too, when the last
  snapshot was ignored or there was no test file to measure. It has no classifier.
- `internal/crap` is the `crap` command's slice (T-67), laid out as `dry`'s, its `Handler`
  holding `dry`'s and the coverage port's `Measurer`, `Coverage`. Its `CLI` takes the
  coverage flags crap always took: `--coverage-report` is the first cumulative flag, each a
  report, a comma no separator (`sep:"none"`), and `--threshold` is a pointer, nil when it is
  not given, so 0 is a threshold. `Select` chooses the sources, so `Run` says how many
  itos-cc.yaml left out, and that there is none to score, before anything is measured, and
  refuses `--coverage-command` with no `--coverage-report` only then; `Handle` measures them
  through `Coverage`, whose progress goes to the writer cmd gave its adapter, parses each as
  `dry` does, scores it with `internal/changerisk` under its `Name` and records `crap.json`
  through the `Store`, each file keyed from the root. `Run` prints the worst first, warns of
  each build root or report coverage did not measure, named from the working directory
  (`UI.Rel`), and with `--threshold` reports each as its gap's problem through the UI's
  classifiers, beside each function above it. It has no classifier: the coverage domain's
  is cmd's.
- `internal/cli` is the UI every slice shares, and imports nothing of ours. Its `Selection`,
  `--changed` and the path arguments, is the command line every command selects its files by,
  embedded first in each slice's kong struct (T-63). It alone turns a
  run's problems into its exit code, by the precedence `docs/CLI.md` gives, and prints the
  plain or JSON answer, and it stops a command on Ctrl-C (decision 38). A domain's failures
  reach it through that domain's classifier, which cmd hands it. For the commands kong parses
  it reads the command line (`Read`, `Parse`), turns kong's failures into problems by their
  types (`ParseProblem`) and prints help (`Help`, a command's `Page`), as "Moving a command"
  below says.
- `cmd/itos-cc` assembles everything: it makes the context, feeds the interrupts to
  `internal/cli`, hands it each domain's classifier, gives `stdin` the disk as its fallback,
  and hands the context and the handler, over the ports in `adapters`, to `Run` through
  kong's bindings. `main` makes the program's one `context.Background()` and passes it to
  `run(ctx, args, stdin, stdout, stderr, a)` with the process's streams and
  `diskAdapters(os.Stderr)`: the working directory, `workdir`'s `Tree` and `Surroundings`,
  `metricsdir`'s `Store`, `coveragetools`' `Measurer`, saying what it runs on standard
  error, and itos-cc.yaml on the disk. The top-level help, its problems and
  the moved commands use the streams and adapters given, so a test runs them in process with
  buffers and porttest's fakes (`TestRunWithFakes`, decision 44); the hand-parsed commands
  keep the process's streams and disk until they move, and only `main` exits. Every command
  runs under the context: a hand-parsed one through
  `execute` and `runSafely` to its `run(ctx, in)`, which passes it on first to what it
  calls; a moved one through the parser `run` builds once with `parserOf(ctx, …, a)`, whose
  bindings hand it to `Run`. The handlers of `crap`, `units` and `dry` hold itos-cc.yaml's
  `exclude`, so `runMoved` reads the file through `a.Config` once the command line is parsed
  and binds them then: a file that cannot be read fails that command as `config.invalid`,
  never help or `version`. A command that stops on a signal derives its own from it
  (`serve`'s `signal.NotifyContext`, `mutation run --count`'s `countedSignalContext`), and
  cleanup that must outlive a stop runs under `context.WithoutCancel(ctx)`. Nothing below
  `main` makes a context of its own, and none is held in a value or captured by a closure
  that does I/O: a callback that does I/O takes the context first, as `project.WalkDir`'s
  does. A done context starts no command. It is the only place
  that knows every package. It routes a command moved onto kong (`moved.go`) through kong and
  every other through its own parser (`cli.go`) until it moves. Until every command is a slice,
  it also holds the selection domain's classifier, `classifySelection`, since every command
  selects its files, and `files`, which calls the domain through `workdir` for the hand-parsed
  ones with the `Exclusion` it reads from itos-cc.yaml (`exclusion`). It holds the snapshot domain's too, `classifySnapshot`, since several commands share
  it, and `snapshots`, the `Store` `internal/metricsdir` gives `dry`'s and `scrap`'s handlers:
  a snapshot that cannot be written is the internal 70 it always was, its message whole with
  whatever the command wrapped it in, and `scrap` warns about an unreadable last one and goes
  on. The hand-parsed `mutation` commands read, write, remove and list their snapshots only
  through the `Store` their invocation's adapters hold, which `mutate.Options` carries and
  `mutate.LoadSnapshotOf` takes; `serve`'s graph still walks `.metrics/` with the `metrics`
  library until it moves, reading each mutation snapshot through a `metricsdir.Store` of its
  root (decision 41). It holds the coverage domain's classifier,
  `classifyCoverage`, one case for each gap and each failure of the port, since `crap` and
  `mutation` share the domain. `crap`'s handler measures coverage through the port,
  `adapters`' `Coverage`; `mutation` measures through `loadCoverage`, which builds the
  request from its flags with `coverageRequest` and calls `coveragetools`' `MeasureTests`
  directly until it moves. Both refuse `--coverage-command` with no `--coverage-report` as
  `cli.CommandNeedsReport`.
- `internal/version` says what the build is, and `internal/release` is the library of the
  release tools that `tools/bin/release-version` and `tools/bin/release-notes` run. Neither is
  a command.

The domain is shared, not part of any command, so a second command reuses counting through its
port without reaching into another command's package. A context carries cancellation and
deadlines only, always as the first parameter and never in a struct or as a value (decision 30).
golangci-lint holds the shape: containedctx refuses a struct field holding one, and revive's
context-as-argument a context after any other parameter, tests included.

`.golangci.yml` holds these arrows with depguard:

- `domain`: the domains, their ports and their tests import nothing of ours but the domains and
  their port subpackages, and never kong.
- `domain-io`: the domain's own code does no I/O, neither `os` nor `net`. Its tests may read
  fixtures.
- `slices`: a slice imports nothing of ours but the domains, `internal/cli` and
  `internal/lang`, the domain library naming a source's units that every command measures by,
  with its port and, in its tests, the port's fake, so never another slice, an adapter, the `project` library
  an adapter wraps and lang's disk side, or `cmd`. Its files are every package under `internal/` except the ones the
  rule lists as something else, so a new command's slice is held with no edit. A slice's tests
  are in its own package, since the rule cannot tell a package's import of itself from another's.
- `slice-fakes`: a slice's own code imports no `porttest`, which only its tests may use (T-56).
  It holds the same files as `slices`, tests left out, and names each port's `porttest`, since
  depguard matches a prefix and `slices` allows the ports' folders. `tools/bin/architecture-check`
  also refuses `itos-cc`'s binary linking any `porttest` (`go list -deps ./cmd/itos-cc`), since
  golangci-lint does not lint `cmd`: a fake enters `cmd` only through its `_test.go` files.
- `ui`: `internal/cli` and its tests import nothing of ours, so adding or removing a domain
  touches no file of the UI (q-54).
- `infra`, `workdir`, `metricsdir` and `coveragetools`: an adapter imports nothing of ours but
  the port it implements, `internal/workdir` the two it implements, selection's and
  `internal/lang`'s, and the `project` library it wraps, and `internal/metricsdir` the
  snapshot port and the `metrics` and `project` libraries it wraps; only its tests may use
  `porttest`. Each port's adapters have a rule of their own, so none is allowed another's port
  or library. An adapter may also import the domain it serves, and a port the domain it
  belongs to, to build or return the domain's own types with no mapper between them (decision
  47, amending 29's adapter rule): its rule allows that domain once the adapter needs it.
  `internal/coveragetools`' allows the coverage domain, whose `Report` it returns, beside its
  port and the `coverage` runner and `metrics` it wraps. Nothing leaks the other way: the
  `domain` and `domain-io` rules still refuse every adapter and I/O package in a domain.
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
  `domain-io`, as a `!**/internal/<name>/**` exclusion to `slices` and `slice-fakes`, and its
  import path to the `allow` lists of `domain` and of `slices` if commands use it. Give it a
  `port` subpackage for what it reaches outside, its `porttest` named in `slice-fakes`' `deny`,
  and a classifier for its failures beside the slice that uses it, which `cmd` adds to the UI's classifiers. `tools/bin/domain-coverage` reads its list from `domain`'s `files`,
  so the package is held to 80% from then on. If its tests use rapid, add a
  `!**/internal/<name>/*_test.go` exclusion to `rapid`.
- An adapter: add its folder to the `files` of the rule for its port's adapters (`infra` for
  counting's, `workdir` for selection's, `metricsdir` for the snapshot port's,
  `coveragetools` for coverage's), or give a new port's adapters a rule of their own allowing
  that port, any library they wrap and, when it returns the domain's types, the domain it
  serves (decision 47); add a
  `!**/internal/<name>/**` exclusion to `slices` and to `slice-fakes`. Wire it in `cmd`.
- Any other package that is no command, such as `internal/version`: add a `slices` exclusion
  saying what it is, and the same to `slice-fakes`.
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
   - `CLI`, the kong struct: `cli.Selection` embedded first when the command selects files,
     giving it `--changed` and the path arguments, then one field per flag of its own, in the
     order help lists them, each with its `help` tag; `JSON bool` named `json`; any other
     arguments last (`arg:""`). Its tags need no import of kong.
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
   its `help` tag the line the top-level help lists, its handler to `parserOf`'s bindings
   (or, when it holds what itos-cc.yaml says, as `units`' and `dry`'s do, bound in `runMoved`
   once the line is parsed), built over the ports in `adapters` and never over an adapter
   itself, a port it needs added there and to `diskAdapters`,
   and a case to `runMoved` that calls its `Run` through `callSafely`. Remove the old
   `command` from `main.go`'s list and delete its file. `run` sends a moved command to kong
   and every other to the hand parser; the top-level help, `help <command>` and the guess
   for an unknown command know both. A group, such as `mutation`, moves whole, and
   `ParseProblem`'s `command.unknown` then takes the group's wording, which names its
   subcommands.
5. **The command line.** `cli.Read` finds `--json` and help (`-h` or `--help` anywhere
   before `--`) before anything is judged, so help wins over any mistake and a usage error
   prints the object `--json` asks for. `cli.Parse` parses with a parser that has parsed
   nothing before, since `cli.Flags` holds the state of one parse, so `run` parses once
   with the parser it built. It holds kong to what it does not hold by
   itself: a word of one dash, other than `-` and `-h`, is a long flag mistyped, found
   against kong's model unless what precedes it fails first (q-55); arguments may stand
   between flags, so they move behind them before kong reads them. `cli.Flags` refuses a
   switch given a value, a flag given no value and a flag given twice, the second time
   before its value is judged. It reads switches, string values and numbers: a float64 flag
   by `strconv.ParseFloat` and an int flag by `strconv.Atoi`, as the hand parser read them,
   refusing what is none as a `cli.NumberError` in its words (`--threshold needs a number,
   and was given "abc"`; `a whole number` for an int), added when `dry` moved (T-40). A
   cumulative flag, a slice, may be given again, each time with a value its mapper judges
   given before kong's slice decoder reads it (`crap`'s `--coverage-report`, T-67); an
   optional number is a pointer, nil when not given (`crap`'s `--threshold`). A switch that
   may take one of its choices after `=` is a `*cli.Choice` whose `enum` tag names them
   (`mutation run`'s `--fail-uncovered[=functions|lines]`, T-68): nil when not given, its
   first choice when given alone, never the next word as its value, and any other value
   refused as a `cli.ChoiceError` in the hand parser's words; help and completion take the
   choices from kong's model. A command whose flags take durations (`serve`) first gives
   `Flags` a mapper for that kind, with a typed refusal in the words the hand parser used.
6. **Failures by type, never by text.** `cli.ParseProblem` turns each failure into the
   problem `docs/CLI.md` names, by the kong fork's error types and `Flags`' own:

   | Failure                                             | Problem                 |
   | --------------------------------------------------- | ----------------------- |
   | `kong.UnknownFlagError`, `cli.MistypedFlagError`    | `flags.unknown`, the flag as written and the one meant |
   | `cli.SwitchValueError`                              | `flags.switch-value`    |
   | `cli.ValueMissingError`, `kong.MissingValueError`   | `flags.value-missing`   |
   | `cli.RepeatedFlagError`                             | `flags.repeated`        |
   | `cli.NumberError`, `cli.ChoiceError`                | `flags.value-invalid`, in the hand parser's words |
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
8. **Its scenarios run through godog.** Convert the command's feature file before the move,
   so its scenarios hold it: its steps in `features/<file>.go`, registered from `Initialize`
   in `steps.go`, reaching the world only through its `Driver` and reading only what itos-cc
   prints and writes, and the file added to `Converted` in `steps.go`. The binary driver then
   runs its scenarios (Tests, below); a Given whose run measures coverage with the project's
   own tools declares them (`Driver.Coverage`), so the binary driver skips the scenario where
   they are not installed. The move adds the file to `InProcess`, and its scenarios run
   through the in-process driver too, the moved command needing nothing more of it than the
   ports in `adapters`. Never tag a scenario `@binary-only` because its command has not
   moved: the tag says the binary's for good, as for a scenario about the real tools' commands
   (crap.feature's ID-CRAP-02). A live scenario's text never changes outside a `feat` or a
   `fix`; write the step for the sentence as it stands, or stop and say which scenario cannot
   be run as written. Check each step can fail: change an expectation in the file locally,
   see the step fail, and restore it, and for a sentence with no value to change, change the
   step's check. The file's smoke scenario in `smoke.yaml` then runs in every push's scenario
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
repository, since translating OS and git errors is what an adapter does.

A fake stands in for its adapter only when both pass one contract suite (decision 44). Each
port's `porttest` exports it from `contract.go`, code that imports `testing` and no test file,
so the adapter's tests can run it: `TreeContract(t, open)` for selection's `Tree` (T-52) and
`StoreContract(t, open)` for the snapshot `Store` (T-53), `SurroundingsContract(t, open)`
for lang's `Surroundings` (T-54), `FilesContract(t, open)` for counting's `Files` (T-55),
coverage's `FilesContract(t, open)` for its `Files` (T-64) and `MeasurerContract(t, open)`
for its `Measurer` (T-66), over the reports a request names: formats, matching, merging,
an unreadable report, one a command did not write, and a done context. Running the
project's own tools, and the run folders, are `internal/coveragetools`' own tests'.
`open` lays out a fixture, a `TreeFixture` of files' text, whether git holds them and which
changed, the snapshots' raw text by name, or files' text or bytes, and returns the port over
it and its absolute working directory. Every fake that names paths sits at an absolute `Dir`
a relative path is from, a path rooted with no volume such as `/work` on `Dir`'s volume, so a
suite reads by relative and absolute paths alike. A port whose doc says it does not act on a
done context, or that answers after one do not matter, as `Files`' and `Surroundings`' do,
has no done-context case, and its suite says why. The suite
asserts what every implementation must do: results, the typed failures of fixture states both
sides can hold, and a done context. The fake's opener names a `t.TempDir()` path it never
touches, so its paths are absolute on every system; the adapter's lays the fixture out under
an `EvalSymlinks`'d `t.TempDir()`, git above it out of reach, and makes it the test's working
directory with `t.Chdir`, so no case runs in parallel. A failure the fake is configured with
stays in the fake's own tests, and what only a disk can do (symlinks, permissions, OS error
texts, what git ignores) in the adapter's. A difference the suite finds is fixed in whichever
side is wrong, never by leaving the case out; the fake's lists of skipped directories and
build output are copies of `project`'s, which the suite keeps honest. The `count` slice's tests use the fake too, standard input being its `-`
key; reading standard input, and stopping when the context is done, is `internal/stdin`'s,
held by its own tests.

The scenarios in `features/` run through godog with two drivers (decision 44), the same steps
of package `features` behind its `Driver`. `features`' `TestFeatures` runs every converted
file against the built binary (`binary.go`), which proves main's wiring, the real adapters and
tools, exit codes and signals, interrupting it once it waits on an open standard input to show
the 130 exit. `cmd/itos-cc`'s `TestScenarios` runs the files of the moved commands
(`InProcess`) in process, a selection naming only scenarios of the others running nothing there: its driver, a `_test.go`, holds every
folder and file in memory with a fake git (init, add and commit; `--changed` is what differs
from the last commit, and every file before one), and each run calls `run` with buffers and
porttest's fakes built from that memory: `TreeOf` over the whole scratch folder, files
outside the run's folder named by `../`, lang's `Surroundings`, snapshot's `Store` written
back to `.metrics/`, an itos-cc.yaml held in memory, and coverage's `Measurer`, which reads
the reports a run names from the memory and gives each build root what the scenario
declared its tools measure (`Driver.Coverage`): the LCOV report they write, nothing when
they write none, a missing tool when none was declared, and no report left for
`--use-existing-coverage`. It refuses a hand-parsed
command and an environment variable, naming `@binary-only`, so nothing reaches the real disk.
A scenario only one driver can run carries `@binary-only` or `@in-process-only`, never a
copy, and the other passes it by; a step a driver cannot do fails naming the tag. godog runs
one scenario at a time in both. itos.yaml's scenario lines run both tests, so CI and itos's
scenario step hold every scenario both ways. A second Ctrl-C
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
  linters plus depguard, gosec on non-test code, revive's file-length limit,
  gochecksumtype, which refuses a type switch over a sealed set that leaves a kind out, a
  default arm not counting, and the context rules of decision 30: containedctx, which refuses
  a context held in a struct, and revive's context-as-argument, which refuses one passed
  anywhere but first. revive runs only the rules `.golangci.yml` lists, never its defaults.
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
