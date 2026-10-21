# Go design and quality

This guide adds what is specific to Go to `docs/engineering.md`, which it does not repeat.
It describes what this project actually holds and checks: the Go foundation and the command
line built on it. The command line's interface contract belongs in its own guide. The
policy is harvested from [itos-template](https://github.com/donvargax/itos-template) (its
decisions on thin layers over a domain, unit tests with fakes, rapid properties and the
OS as a value), adapted to this module with its owner's permission (`CONTRIBUTORS.md`).

## Packages

The code follows decision 29 (`docs/decisions/`): shared domains behind ports, adapters apart
from them, and thin slices named for their commands. A domain function that calls a port takes
a `context.Context` first (decision 30); pure logic takes none.

- `internal/selection` is the domain every command chooses its files through: paths given as
  arguments, a path that is not there taken as a fragment of a file's path under the working
  directory, `--changed` adding git's changed files, and no paths meaning the whole working
  directory. `Select(ctx, tree, paths, changed, exclude)` reads the tree only through its port,
  then leaves out the sources `exclude` names, never a test, counting them in `Files.Excluded`.
  `Parse(ctx, tree, around, path)` is a selected file parsed, the one copy every command that
  measures what it selects uses (T-63): a language `internal/lang` does not know is refused
  before the read, then the tree reads it and lang parses it through its `Surroundings` port.
  `Exclusion` is the port's `Settings`, itos-cc.yaml's `exclude` as a value, which cmd and the
  slices' handlers are given, read at the edge: `Excludes(path)` matches a path's slash form
  from `Root` against the doublestar globs of `Exclude`, by `filepath.Rel` alone, no I/O.
- `internal/selection/port` declares `Tree` (`Discover` roots into sources and tests,
  `Changed` files, whether a path `Exists`, and `Read`, what a selected file holds, each
  taking the context first) and its sealed set of failures: `NoGit`, `Unmatched`, carrying
  the fragment, `Unwalkable`, `Unreadable` and `Cancelled`. Every command reads the files it
  selects, so it reads them through the tree it chose them from, never by itself. It owns the
  selection settings too (T-147): `Settings{Root, Exclude}`, the root itos-cc.yaml was found
  at and its `exclude` globs, and `SettingsReader`, whose `ReadSelection` rereads them at
  each call, failing as `SettingsInvalid` (a reason and the key at fault), `SettingsFailed`
  (a read that failed short of them, such as a missing binding) or `Cancelled`.
  `Discover` gives absolute paths, which `snapshot.Key` names from the root.
  `internal/selection/port/porttest` holds the in-memory fake of `Tree`, at an absolute
  directory it names and never touches, and `TreeOf`, which lays one out from files' text,
  classified by `internal/lang` as `internal/workdir` classifies them, and `TreeContract`, the
  contract it and `internal/workdir`'s `Tree` both pass (Tests, below), and the fake
  `Settings` with `SettingsContract`, which it and `config.Reader` pass.
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
  `Cancelled`. A snapshot read that did not decode is `Unreadable` too, its `Reason` in each
  store's own words, naming no Go type or package; the port knows no JSON.
  `internal/snapshot/port/porttest` holds the in-memory fake of `Store`, writing the bytes the
  `metrics` library writes and giving a plain reason for a snapshot that does not decode, and
  `StoreContract`, the contract it and `internal/metricsdir`'s `Store` both pass (Tests, below).
- `internal/workdir` is the selection port's adapter: the working directory as the disk and git
  hold it. It wraps the top-level `project` library's discovery and git query, stats paths and
  reads files, translating their errors into the port's failures. Beside `Tree` it implements
  `internal/lang`'s port as `Surroundings`, answering from `project.Surroundings`, the disk. It
  also implements mutants' `SourceEdits`: follow-rename absence uses Stat, while
  annotation rereads current bytes, skips identical content before stat/write, and
  preserves the current permissions.
- `internal/metricsdir` is the snapshot port's adapter: the snapshots under `.metrics/` at the
  project root of the working directory, or at the root its `RootDir` names, so one `Store` for
  each root serves several projects (graph's, in S8) with no change to the port. It wraps the
  top-level `metrics` library, which writes them as indented JSON renamed into place, removes
  and lists them, and `project`, which finds the root, so
  every byte and every atomic write stays the library's, translating their errors into the
  port's failures. It alone decodes snapshot files, so a snapshot that did not decode is
  worded by its `Undecodable`, in the snapshot's own terms rather than the Go types a reader
  decodes into (bug-3): a value of the wrong kind names its key path and the JSON kinds,
  `files[0].file should be a string, not a number`, and JSON that does not parse keeps the
  parser's text. The in-process scenario driver words through it too, standing in for this
  adapter over the fake. Its model-bearing `Mutations` adapter owns the one concrete
  mutation snapshot codec; all consumers use its concrete read/write boundaries without a second schema.
  The codec keeps mirrors only of the snapshot, unit and mutant (`snapshotWire`,
  `unitResultWire`, `mutantWire`), whose methods write coverage under `go_coverage` or
  `line_coverage` by language and read legacy `go_evidence`; it encodes mutants'
  `CoverageEvidence`, `CoverageBlock` and `SuiteEvidence` through their own tags (T-136).
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
  yet refactored call with the context their caller gives them. It also compares a unit's
  structure, the engine `dry`'s and `scrap`'s domains share (T-148): `Fingerprint(f, n,
  skip...)` is the fingerprint set of the tree under a node of a parsed file, local names,
  field names and literals dropped, called names, operators and shape kept, each node hashed one
  and two levels deep, and `Jaccard(a, b)` the similarity of two such sorted sets. It imports
  no mutation package (T-156): `ChangedSince` and `ChangedSinceAt` hand each file a range
  changed, as HEAD and the merge base hold it, and its changed `LineRange`s, project's own, to
  the caller's function, generic in what it returns, so which functions a change touches is
  mutation's judgment, made in `internal/githistory`.
- `internal/coverage` is the coverage domain (decisions 37 and 46): the `Report` that answers
  which lines a run covered, a function's fraction and which listed tests reach a line, its
  queries, and the parsers of LCOV, Go cover profiles and JaCoCo XML (`Parse` choosing by the
  content). It reads nothing. The top-level `coverage` runner, which runs each language's
  tools, imports no mutation package, keeping its own copy of the Maven and Gradle test
  filters as graph keeps crap's formula (ADR-0051, T-156), and builds every `Report` through
  the domain's API, never its fields (decision 47):
  `Build(ctx, files, …)` and `Merge`, `FromLines` and `FromGoBlocks`, the setters
  `SetMissing`, `SetIntegrationMissing`, `MarkMeasured` and `Prove`, `List` for a static
  inventory's lines, `Integrated` for the data of the processes the tests started, and
  `TestCoverage.Add` with `SetTests`; the zero `Report` is empty. It declares `Measurer`, which
  the commands measure coverage through (decision 46): `Measure(ctx, port.Request)` gives a
  `*Report`, each build root or report it could not measure in its `Missing`, or the port's
  sealed `Failure` of the whole measurement: `Cancelled`, the gap, once the context is done,
  and `Unwritable`, a run folder that could not be made. It sits beside the `Report` it
  returns, so it names that type with no type argument, while the `Request` it is given and
  the `Failure` stay in the port the domain imports for `Files` and `Gap`, and no cycle
  forms (T-135). `MutationMeasurer` embeds it, adding `MeasureListed` and `Inventory`. The
  runner keeps `Load`,
  which opens a report, `Disk`, the `Files` port over the disk, and `Inventory`, which runs a
  static inventory command. An `Unmeasured` is an error, its `String`, so a command hands it
  to the UI, whose classifier makes it its gap's problem. Its tests are package
  `coverage_test`, since its `porttest` imports it.
- `internal/coverage/port` declares `Files`, what the report-path matcher asks of the disk:
  `SameFile(ctx, path, among)`, whether a path names a regular file and which of the sources
  of its name it is, reached through a symlinked directory too. It declares the sealed set of
  `Gap`s an `Unmeasured` build root or report has as its `Cause`: `ToolMissing`,
  `MeasuredNothing`, `NoReport`, `Unreadable` and `Cancelled`, each written as its name. It
  declares what the domain's `Measurer` is given and fails with: the `Request`, and the
  sealed `Failure` of a whole measurement, `Cancelled` or `Unwritable`. `Request` carries
  what crap's coverage flags say: the
  `Sources`, `--coverage-report`'s `Reports`, `--coverage-command`'s `Command`,
  `--use-existing-coverage`'s `Existing`, `--all-tests`' `AllTests`, the `Scope` of tests
  that run without it and the `Reach` of the test files that reach each source.
  `internal/coverage/port/porttest` holds the in-memory fakes: `Files`, with links standing
  for symlinks, and `Measurer`, the domain's `Measurer` in memory, which reads the reports a
  request names from the files it holds and gives a request that names none the canned
  reports of its build roots; and `FilesContract` and `MeasurerContract`, the contracts each
  and its adapter pass.
- `internal/coveragetools` is the adapter of the coverage domain's `Measurer` and its
  port (decision 46). Its `Measurer` wraps the top-level `coverage` runner, its process groups and the run folders under
  `.metrics/coverage` the tools write their reports in, one for each run, the latest kept for
  `--use-existing-coverage`, and runs `--coverage-command` through the runner's `Shell`. It
  returns the domain's `Report` as the runner built it, with no mapper (decision 47), and
  writes what it runs and the tools' output only to the `Log` main gives it. Its
  `MeasureListed` implements the root `MutationMeasurer` capability, retaining
  each listed test's coverage in the same run folder. `Inventory` wraps the
  existing static collectors, logging best-effort failures without inventing proof.
- `internal/slices/mutation/domain` is the mutation slice's own domain (T-87, ADR-0051), the
  subpackage that keeps it apart from the slice's CLI and application code; it was
  `internal/mutants` (T-158), and its importers still name it `mutants` (cmd's adapters
  `mutation`), its fakes' `mutantstest`. It holds mutation sites and unit hashes,
  stored-result values, pairing and freshness rules, exception judgment, executable-coverage
  evidence and its judgment, annotation text, sampling ranks, committed-candidate values and
  diff hunks. It reads nothing and holds no encoding or decoding logic; like every domain it may carry struct
  tags used only for serialization (decision 50). Tests
  parse supplied source through lang's surroundings fake. Ordinary and counted scheduling,
  check/list/sample/exception policy and fresh admission live here over model-bearing ports.
  `metricsdir` owns the concrete snapshot codec, including legacy evidence keys and line
  precision; config owns the exception YAML codec. `CoverageEvidence`, `CoverageBlock` and
  `SuiteEvidence` carry the snapshot's json keys, and `Exception` itos-cc.yaml's as json and
  yaml tags, so metricsdir, the mutation slice's `--json` and config's writes encode them with
  no mirror (T-136). It has no `port` subpackage: its model-bearing ports are declared beside
  their values (below), and `internal/slices/mutation/domain/porttest` holds their fakes and
  contract suites. The `domain`, `domain-io`, `mutant-fakes` and `coverage-fakes` rules hold it,
  and `tools/bin/domain-coverage` measures it. Unlike dry's, scrap's and crap's domains it is on
  the `domain` rule's allow list, since its external tests and porttest import it, and the
  slice's adapters, the adapters not yet moved, `config` and `graph` import it too; the shared
  domains may not (`shared-domains`), nor another slice (the sibling rule). The legacy
  `mutate` package is retired.
- `internal/slices/mutation/adapters` holds the mutation slice's own adapters (ADR-0051), each
  in a subpackage of its name with a depguard rule of its own, as every adapter has:
  `testtools`, mutation's `Toolchain` and `TestLister`, and `testinputs`, its `Fingerprints`
  and the coverage-input inventories (T-159, below), were `internal/testtools` and
  `internal/testinputs`. They import the slice's domain, whose model-bearing ports they
  implement, and the libraries they wrap; cmd wires them in through the slice's `Ports`, and
  no slice imports them, the mutation slice's code neither (`slice-adapters`).
- `internal/processes` is the process-ownership engine (ADR-0021, T-151), an adapter-side
  engine rather than a domain: it starts and kills processes and imports nothing of ours.
  `OwnedExecutor` runs an already configured command in a process group it owns on Linux and
  macOS, and when the command ends or its context is done kills the group and joins its
  members and the readers of its output within one five-second cleanup deadline; a group
  whose cleanup did not finish is a `CleanupFailed` naming the group and the command. Its
  `Hold`, which cmd binds to the run's `Cleaning`, marks a group being killed and joined, so a
  second interrupt waits for it (decision 38). `Run` words a command's end as an error and
  `Command` returns its `Result`, which a caller converts into its own terms, as cmd does into
  mutation's `CommandResult` for freshenv; `Budget` is the shared run-abort `CleanupBudget` a
  worker pool it owns shares from its creation. `Runner` is the command runner mutation's
  workers run through, `ErrCommandTimeout` the cause of a command's own timeout, judged
  `TimedOut` rather than cancelled, and `PlatformAtBoundary` the host only for a zero
  platform: the OS is a value, and the build tags of `proc_unix.go` and `proc_windows.go` hold
  only the group calls Windows lacks. Crap's and mutation's coverage commands (cmd's
  `coverageCommand`), counted preparation and mutation's workers run through it; cmd binds
  it, and no domain or slice imports it, so crap reaches no mutation adapter.
- `internal/freshenv` implements mutation's coarse `Fresh` Plan/Prepare/Release contract.
  It owns immutable Git export, native mode and symlink admission, scratch materialization,
  installed-dependency links and ordered supervised preparation. Cmd injects its owned
  executor and existing observation adapters. The domain shares path selection, source
  admission, baseline deduplication and trial scheduling with the memory implementation.
  Pure planning uses committed settings; trials reread actual frozen bytes after preparation.
  Release retains historical best-effort scratch cleanup; checked worker Close retains
  lifetime failures and partial trial results. Its depguard rule permits only the served
  domain and the actual config/project/coverage/language/selection libraries it wraps.
- `internal/slices/mutation` declares the whole mutation group and its five child command
  structs and help contracts (T-96), the sixth slice under `internal/slices` and the first that
  is a group (T-157); it was `internal/mutation`. `Command` declares the group, its `New` a fresh
  `*CLI` whose fields kong reads the five children from, and `Ports` names what
  `NewHandler(ctx, p)` builds the handler over: `SelectionTree`, `LangSurroundings` and
  `SnapshotStore`, as the other slices declare them, a method for each mutation port the handler
  holds, named for the domain's port (`MutationSettings`, `MutationSnapshotReader`,
  `MutationCatalog`, `MutationSourcePresence`, `MutationSourceStatus`, `MutationHistory`,
  `MutationToolchain`, `MutationFingerprints`, `MutationReach`, `MutationWorkers`,
  `MutationSnapshots`, `MutationSourceEdits`, `MutationTestLister`, `MutationFresh`,
  `MutationFreshWorkers`), `CoverageMutationMeasurer`, `MutationPlatform` and `WorkingDir`.
  `MutationExceptions`, `MutationRepository` and `MutationPhysicalDir` give the functions
  `adapters`' fields hold as they are, never a method value, so a binding left nil reaches the
  handler nil and says it is missing in its own words. `NewHandler` copies them and reads
  nothing, so settings are read only at each command's admission point, never for help or a
  line kong refuses, and it never fails. Cmd supplies the worker default (`mutation_workers`)
  and creates one fresh model/parser per run. Kong validates each child once; `cli.Provided`
  retains the actual validated path's explicit flag facts and original value spellings, which
  cmd binds once for every command as `cli.Written` and run and sample convert to the slice's
  own `Written`. Every child runs in the slice: list and
  check (T-97), sample (T-98), except (T-99) and run (T-100): application queries carry
  selection, coverage, sampling and exception values once, and their handler coordinates the
  existing mutants, selection and language capabilities. List presents its ordered prefix on later read
  failure; check formats no per-file findings until ordinary and strict admission both
  succeed. Sample observes in the old cmd order: its selection, then the seed (an
  explicit one, even empty, never reads `History.Head`), the no-source return, settings,
  reach, the listed tests' support files and the bindings, then the domain's draw; the
  sources exclude left out reach stderr through its query's `SelectionProgress` before any
  run progress, and the missing `Workers` binding alone emits no result object. Its
  `Run` refuses a `--count` below 1 before anything is observed, quoting the value as
  written from the `Written` flags cmd binds (`cli.Provided`, as `cli.Written`). Except's `Run` takes
  `--renew`'s branch before it looks at the reason, refuses an absent `--reason` (nil) apart
  from an empty one, then the site's count and form, read from the right so a drive letter's
  colon is no separator, before assembling its `ExceptQuery`. Its handler reads settings, then
  the site's exclusion, then the bindings, the listed tests' support files and reach, then the
  domain's survivor, and writes the entry through the `Exceptions` writer cmd selects for the
  settings' root; `Renew` returns before any binding when the settings hold no entry, then
  discovers every source through the `Tree` and writes only the renewals. Run's `Run`
  refuses what its flags alone refuse (`--fail-uncovered=lines` without `--since`, then a
  counted run's flags and platform, or `--no-coverage` beside `--fail-uncovered` and
  `--fail-fast`'s platform, the `Platform` value cmd detects) before anything is observed,
  reading which flags were given from `Written`, and assembles one `RunQuery` or
  `CountedQuery`. Its handler reads settings at run's own four points: its admission, the
  selection's exclude, the exceptions' exclude and the orphans' exclude; then the
  exceptions of files it does not select, strict coverage's flag refusal, reach, the
  listed tests' support files and the bindings, strict coverage's checks and measurement
  through `Coverage`, the run through the `Snapshots` and `SourceEdits` writers and the
  orphans it removes from `Metrics`. `Counted` reads no settings: it asks cmd's `Repository`
  and `PhysicalDir`, then, under the command's context, which an interrupt cancels (decision
  38), plans, prepares and judges through `Fresh` and `FreshWorkers`, handing its report to
  its query's `CountedProgress` while it still owns the frozen inputs. The
  handler builds no problem and picks no exit code: it returns the slice's sealed
  `Failure` (`LinesWithoutSince`, `SinceWithChanged`, `SinceUnavailable` with its ref,
  `SeedUnavailable`, `SiteExcluded`, `NoSurvivor`, `StrictCoverageFlag`, `CountedNoGit`,
  `OutsideRepository`, `CountedPlanFailed`) at the cmd handler's admission point, and `Run`'s classifier words
  it, beside the settings, history, snapshot-read and run failures (`RunError`) and the
  `Exceptions` writer's, in the bytes and codes cmd gave them; a run failure is worded as
  one (`RunError`) only where cmd's handler worded it so. Coverage gaps reach the coverage
  domain's classifier the UI holds (`GapProblems`). Shared mutation presentation, the progress lines
  (`Progress`) among it, has one owner here. An explicit zero/empty flag remains
  distinct from omission. The literal `no-coverage` and `no-annotate` names have no positive
  aliases. Group admission preserves the old before-child help/error precedence; group
  help and the flattened top-level rows come from the same model. Leading group `--json`
  affects group errors but is stripped before a known child's dispatch, as before;
  top-level and child-level JSON still reach that child.
  `mutants.Config` is the existing semantic settings value; `config.Config` aliases it.
  It carries the selection engine's `Settings`, root and `exclude`, beside mutation's
  exceptions and tests, so each admission point reads all four from one observation.
  The model-bearing `SettingsReader` rereads actual current settings at the preserved
  admission points. `config.Reader` uses the one existing codec over cmd-selected disk
  or memory bytes. A file the bytes' source cannot read is invalid settings, a reason
  with no key, as a file that does not parse is (bug-4); only a cancelled read, or a
  missing binding, stays apart. `crap`, `dry` and `units` read only the selection
  settings, through selection's `SettingsReader`, which `config.Reader` implements over
  the same read and codec (`ReadSelection`), so every command classifies the file alike. The
  unit fake owns only semantic values. The memory driver runs all five children over its
  fakes (T-125), binding a counted run as the disk adapters bind it: porttest's `Fresh`
  over the fake repository, the same `Workers` for its trials, the fake repository's
  root, a context of the run's own that installs no signal handling, and the configured
  platform value. Its tests hold that nothing reaches the host with a physical decoy: an
  invalid itos-cc.yaml in the process's working directory that a fall-through would read,
  beside which run and except read the memory's.
- `internal/slices/units` is the `units` command's slice, the first of itos-cc's own commands
  moved onto kong (T-32) and the layout every slice follows, and the first under
  `internal/slices` (ADR-0051, T-145): its kong struct `CLI` and `Run` are the
  UI, with its `Contract` for the help; its `Query`, carrying domain values, and
  `Handler.Handle(ctx, q)`, which only calls the domains through their ports, are the
  application, `Run` assembling the `Query` from kong's flags the one copy the layout accepts.
  It declares itself to cmd: `Command`, a `cli.Declaration`, gives its name, the line the
  top-level help lists it by and `New`, a fresh `*CLI` for each parser; `Ports`, an interface
  it owns, names the ports its handler is built over, the selection `Tree`, lang's
  `Surroundings` and the selection engine's `SettingsReader`, which cmd's `adapters` satisfy
  through methods (`SelectionTree`, `LangSurroundings`, `SelectionExclusion`); and
  `NewHandler(ctx, p Ports) (Handler, error)` builds the handler over them, the `Exclusion` read
  through `SelectionExclusion` under `ctx`, its failure returned unwrapped (T-146).
  Its handler selects the files through the
  selection domain, parses each with the domain's `Parse`, read through the same `Tree` port
  and named by `internal/lang` through the `Surroundings` port its handler is given, its
  `Exclude` the sources the selection leaves out. `Run` prints them, their paths from the
  working directory (`UI.Rel`). It has no classifier: its failures are the
  selection domain's, whose classifier is cmd's, or `internal/lang`'s, internal ones.
- `internal/slices/dry` is the `dry` command's slice (T-40), laid out as `units`'s, the first
  moved command with number flags, and the second under `internal/slices` (T-148), the first
  the generator assembled. Its `Handler` holds the selection `Tree`, lang's `Surroundings`, the
  snapshot `Store`, `Name`, the name a person reads a file by, and `Exclude`, the sources both
  selections leave out. `Command` declares it and `Ports` names what `NewHandler(ctx, p)` builds
  the handler over: `SelectionTree`, `LangSurroundings`, `SelectionExclusion`, as `units`
  declares them, `SnapshotStore`, the snapshot port, and `Rel(path)`, a path named from the
  working directory, which `adapters` answers with `cli.Rel` from its `Dir`, the one copy of
  `UI.Rel`'s logic, from the directory the UI is given, so the UI stays out of the slice's
  ports. `Handle` selects the sources, and with paths or `--changed` checks them against every
  source the selection finds under the working directory; it parses each with the selection
  domain's `Parse`, hands it to its domain under its `Name`, and records `dry.json` through the
  `Store`, each file named from the root (`snapshot.Key`). It has no classifier: its failures
  are the selection and snapshot domains', whose classifiers are cmd's, or `internal/lang`'s,
  internal ones.
- `internal/slices/dry/domain` is the `dry` slice's own domain (decision 37, ADR-0051), the
  subpackage that keeps it apart from the slice's CLI and application code; it was
  `internal/duplication`. It finds functions similar enough to be duplicates in files
  `internal/lang` parsed, `Forms(f, name, opt)` a file's functions large enough to compare,
  with the name its caller gives the file, since a side's file is the key its pairs and groups
  are sorted and paired by, compared by lang's `Fingerprint` and `Jaccard`. It reads nothing
  and imports neither `project` nor any disk code, so it has no port; its tests parse in
  memory through lang's fake. The `domain` and `domain-io` rules hold it as they hold the
  shared domains, and `tools/bin/domain-coverage` measures it.
- `internal/slices/scrap` is the `scrap` command's slice (T-43), laid out as `dry`'s, and the
  third under `internal/slices` (T-150). Its `Handler` holds the selection `Tree`, lang's
  `Surroundings`, the snapshot `Store` and `Name`, with no `Exclude`: it selects only tests,
  which itos-cc.yaml's `exclude` never leaves out. `Command` declares it and `Ports` names what
  `NewHandler(ctx, p)` builds the handler over, `SelectionTree`, `LangSurroundings`,
  `SnapshotStore` and `Rel`, as `dry` declares them, with no `SelectionExclusion`, so its
  provider reads no settings and `scrap`, its `--json` and its help never read the file (CLI
  rule 38). `NewHandler` cannot fail and keeps the `(Handler, error)` shape every slice's
  constructor has. `Handle` selects the test files, reads the last `scrap.json` through the
  `Store`, going on without it when it is `Unreadable`, reads and parses each test file as
  `dry` does, measures it with its domain under its `Name`, merges the run into the last
  snapshot with the domain's `History` and records it, each file keyed from the root. `Run`
  prints each file's report, and says on stderr, with `--json` too, when the last snapshot
  was ignored or there was no test file to measure. It has no classifier.
- `internal/slices/scrap/domain` is the `scrap` slice's own domain (decision 37, ADR-0051),
  measuring the structure of test code; it was `internal/teststructure`. `Analyze(f, name)`
  takes a file `internal/lang` parsed and the name its caller gives it, and `History` merges a
  run's reports into what the last snapshot recorded, keeping the files the run did not
  measure. It compares a test's examples by `internal/lang`'s `Fingerprint` and `Jaccard`. It
  reads nothing and has no port; its tests parse in memory through lang's fake. The `domain`
  and `domain-io` rules hold it as they hold the shared domains, and
  `tools/bin/domain-coverage` measures it.
- `internal/slices/crap` is the `crap` command's slice (T-67), laid out as `dry`'s, and the
  fourth under `internal/slices` (T-152). Its `Handler` holds `dry`'s and the coverage domain's
  `Measurer`, `Coverage`. `Command` declares it and `Ports` names what `NewHandler(ctx, p)`
  builds the handler over: `SelectionTree`, `LangSurroundings`, `SnapshotStore`, `Rel` and
  `SelectionExclusion`, as `dry` declares them, and `CoverageMeasurer`, the coverage domain's
  `Measurer`, which `adapters` answers with `coveragetools`' measurer over the coverage runner,
  its commands run through `internal/processes`' executor (`coverageCommand`), never through a
  mutation adapter. `NewHandler` returns the exclusion's failure as `adapters` worded it, so a
  file that cannot be read is `config.invalid` for `crap` alone, and never for its help. Its
  `CLI` takes the coverage flags crap always took: `--coverage-report` is the first cumulative
  flag, each a report, a comma no separator (`sep:"none"`), and `--threshold` is a pointer, nil
  when it is not given, so 0 is a threshold. `Select` chooses the sources, so `Run` says how many
  itos-cc.yaml left out, and that there is none to score, before anything is measured, and
  refuses `--coverage-command` with no `--coverage-report` only then; `Handle` measures them
  through `Coverage`, whose progress goes to the writer cmd gave its adapter, parses each as
  `dry` does, scores it with its domain under its `Name` and records `crap.json` through the
  `Store`, each file keyed from the root. `Run` prints the worst first, warns of each build
  root or report coverage did not measure, named from the working directory (`UI.Rel`), and
  with `--threshold` reports each as its gap's problem through the UI's classifiers, beside
  each function above it. It has no classifier: the coverage domain's is cmd's.
- `internal/slices/crap/domain` is the `crap` slice's own domain (decision 37, ADR-0051),
  scoring functions by CRAP; it was `internal/changerisk`. `Analyze(f, name, report,
  measured)` takes a file `internal/lang` parsed, the name its caller gives it and a coverage
  `Report`; `Measured(report, files)` is the languages the report speaks for among every file a
  run scores, so a file it never names is untested rather than unknown. It reads nothing and
  has no port; its tests parse in memory through lang's fake. The `domain` and `domain-io`
  rules hold it as they hold the shared domains, and `tools/bin/domain-coverage` measures it.
  `crap.json`'s entries are its `Entry`, which graph does not import: graph, legacy and outside
  the slices, decodes the fields it reads into a type of its own and keeps its own copy of the
  formula, `crapScore` (ADR-0051).
- `internal/slices/version` is the `version` command's slice, the fifth under
  `internal/slices` (T-155), with no domain of its own. `Command` declares it and `Ports` names
  what `NewHandler(ctx, p)` builds the handler over: `BuildVersion`, the version the binary says
  it is, which `adapters` answers with `internal/version`'s `Version()`, since no slice imports
  `internal/version`. Its `Handler` holds that version, so `NewHandler` reads nothing and never
  fails, and its `CLI`'s `Run` prints `itos-cc <version>`, or with `--json` the object of its
  version alone. `--version` before any command is the same command (`cli.Dispatch`).
- `internal/cli` is the UI every slice shares, and imports nothing of ours. Its `Selection`,
  `--changed` and the path arguments, is the command line every command selects its files by,
  embedded first in each slice's kong struct (T-63). It alone turns a
  run's problems into its exit code, by the precedence `docs/CLI.md` gives, and prints the
  plain or JSON answer. It holds `Interrupts`, which stops every command on SIGINT or SIGTERM
  (decision 38), cancelling its context with the cause `ErrInterrupted`, and lets a later
  interrupt end it at once unless the process groups it owns are being killed (`Cleaning`);
  cmd makes one per run and binds `Cleaning` into the adapters that kill those groups. A domain's failures
  reach it through that domain's classifier, which cmd hands it. For the commands kong parses
  it reads the command line (`Read`, `Parse`), turns kong's failures into problems by their
  types (`ParseProblem`), retains validated explicit/raw flag facts (`Provided`) and prints
  leaf/group help (`Help`, `Page` and `GroupHelp`), as "Moving a command"
  below says. Before any command reads its line, `Dispatch` reads the whole of it against
  kong's model of every command: the flags before a command (`--json`, `-h`, `--help`, and
  `--version`, the `version` command), the top-level help, `help <command>` and the guess for
  an unknown command, and `Usage` prints the top-level help from the same model (T-114).
- `cmd/itos-cc` assembles everything: it makes the context, hands `internal/cli` each domain's
  classifier, and hands the context and the handler, over the ports in `adapters`, to `Run`
  through kong's bindings. `main` makes the program's one `context.Background()` and passes it to
  `run(ctx, args, stdin, stdout, stderr, a)` with the process's streams and
  `diskAdapters(os.Stderr)`: the working directory, `workdir`'s `Tree` and `Surroundings`,
  `metricsdir`'s `Store`, `coveragetools`' `Measurer`, saying what it runs on standard
  error, and itos-cc.yaml on the disk. The top-level help, its problems and
  every command but `serve` use the streams and adapters given, so a test runs them in process
  with buffers and porttest's fakes (`TestRunWithFakes`, decision 44). `serve`, its moves
  deferred while it leaves for a web app of its own, alone stays on its legacy handler and hand
  parser (`cli.go`, `serve.go`) and keeps the process's streams and disk, and only `main`
  exits. `run` builds the parser of every command once, with fresh state, with
  `parserOf(ctx, …, a)`, and `internal/cli`'s `Dispatch` reads the line against its model;
  `dispatch` (`dispatch.go`) then hands `serve`'s line, as it was written, to `execute` and
  `runSafely`, which run its `run(ctx, in)`, admits a group's words before its subcommand
  (`cli.GroupLine`, for any command with subcommands), and parses every other's once, then
  calls the `Run` of the command kong selected through one generic call (`runSelected`,
  `Context.Call` on `parsed.Selected()`'s `Run`, `--json` read from the parsed command),
  with no case per command. `parserOf` binds every handler, over the ports in `adapters`,
  through kong providers (`assembled`), which kong calls only when the `Run` it calls asks for
  their value, and binds once for every command the flags its line gave as written
  (`cli.Written`, read by `cli.Provided` from the parsed line) and `mutation_workers`, the
  default of `mutation run`'s and `sample`'s `--workers`, which kong interpolates once the
  declared commands are built. The commands under `internal/slices` are not fields of
  `commandLine`, which holds `serve` alone: `assembled`
  (`commands_gen.go`, which `tools/bin/assemble` writes from each slice's declaration, T-146)
  adds each slice's declaration with kong's `DynamicCommand`, a fresh command line each, in
  their names' order, and the provider of its handler, one shape for every slice,
  `func(ctx) (<slice>.Handler, error) { return <slice>.NewHandler(ctx, a) }`, `adapters` given
  as the slice's `Ports`. kong adds a dynamic command after the struct's fields, so
  `assembled` then puts the model's commands in their names' order (`byName`, in
  `dispatch.go`), the order `TestTheModelNamesEveryCommandInOrder` holds. The handlers of
  `crap`, `units` and `dry` hold itos-cc.yaml's `exclude`, so each reads the file through the
  selection engine's settings reader under the `Run`'s context in its own provider, through
  its `Ports`' `SelectionExclusion`, which `adapters` answers with `settingsError`'s
  wording and `NewHandler` returns as it is, since kong wraps a dependent provider's error as
  `<type>: %w`: a file that cannot be read fails that command alone as `config.invalid`, the
  provider's error kong returns unwrapped, never help or `version`. SIGINT and SIGTERM cancel it in `main`, through `cli.Interrupts`,
  with the cause `cli.ErrInterrupted`, for every command alike (decision 38), and
  cleanup that must outlive a stop runs under `context.WithoutCancel(ctx)`, as an
  interrupted `mutation run` writes what it finished. Nothing below
  `main` makes a context of its own, and none is held in a value or captured by a closure
  that does I/O: a callback that does I/O takes the context first, as `project.WalkDir`'s
  does. A done context starts no command. It is the only place
  that knows every package. Every command is a node of its one kong model, `commandLine` and
  the slices' declarations, in their names' order (`byName`), which the guess weighs a tie by;
  `serve`'s node only names it to the top-level help and the guess. A group is a declaration
  like any other: `mutation`'s `New` makes the group's struct, whose fields are its children, so
  its help, its children's, their top-level rows and the guess for a mistyped child come from
  the same model. It holds the selection domain's classifier,
  `classifySelection`, since every command selects its files; `serve`, on the hand parser, selects none (T-100). It holds the snapshot domain's too, `classifySnapshot`, since several commands share
  it; `dry`'s and `scrap`'s handlers write through `adapters`' `Store`, `internal/metricsdir`'s
  on the disk: a snapshot that cannot be written is the internal 100, its message whole with
  whatever the command wrapped it in, and `scrap` warns about an unreadable last one and goes
  on. The `mutation` slice reads, writes, removes and lists its snapshots only through
  the handler's bound readers, catalog, publication capability and store. `serve`'s
  graph still walks `.metrics/` with the `metrics` library until S8, but receives a root-specific
  snapshot reader and current Toolchain/Fingerprints from cmd. It selects no concrete
  observation adapters, and reads a complete admitted snapshot before TypeScript probes. It holds the coverage domain's classifier,
  `classifyCoverage`, one case for each gap and each failure of the port, since `crap` and
  `mutation` share the domain. `crap`'s handler measures coverage through the port,
  `adapters`' `Coverage`, which its `Ports`' `CoverageMeasurer` gives it; `mutation run` measures through its handler's `loadCoverage`,
  which builds the request from its query and calls the separate `MutationCoverage`
  binding's `MeasureListed`, which its `Ports`' `CoverageMutationMeasurer` gives it. Strict coverage also calls that binding's
  `Inventory`; neither consumer constructs a real fallback. Strict coverage refuses
  `--coverage-command`, as every raw coverage flag, before it measures
  (`StrictCoverageFlag`); an ordinary run given a coverage command and no report logs
  `cli.CommandNeedsReport`'s words after configured listing and goes on without coverage.
- `internal/version` says what the build is, the one source of the version the binary
  reports: a release stamps `-X github.com/donvargax/itos-cc/internal/version.stamp=<v>`
  (`release.yml`, `.goreleaser.yaml`), as the scenario harness does, else Go's module version
  is read, each without a leading v, else `dev`; cmd's `adapters` give it to the `version`
  slice (`BuildVersion`). `internal/release` is the library of the release tools that
  `tools/bin/release-version` and `tools/bin/release-notes` run. Neither is a command.

A domain is shared, not part of any command, so a second command reuses it through its port
without reaching into another command's package. A context carries cancellation and
deadlines only, always as the first parameter and never in a struct or as a value (decision 30).
golangci-lint holds the shape: containedctx refuses a struct field holding one, and revive's
context-as-argument a context after any other parameter, tests included.

`.golangci.yml` holds these arrows with depguard:

- `domain`: the domains, their ports and their tests import nothing of ours but the domains and
  their port subpackages, and never kong. A slice's own domain, `internal/slices/dry/domain`,
  `internal/slices/scrap/domain`, `internal/slices/crap/domain` and
  `internal/slices/mutation/domain`, is held as the others are, and left out of what a domain may
  import, since it is no shared one, but for mutation's and its `porttest`, which its own
  external tests and porttest import. `config` imports `internal/slices/mutation/domain`, whose `Config`
  it aliases and whose `SettingsReader` its `Reader` implements, and `internal/selection/port`,
  whose `SettingsReader` it implements too, never the reverse.
- `shared-domains`: the shared domains, `internal/selection`, `internal/lang`,
  `internal/snapshot` and `internal/coverage`, their ports, fakes and tests, import nothing under
  `internal/slices`, so mutation's domain on `domain`'s allow list is open to no shared domain
  (ADR-0051); the sibling rule keeps it from another slice's domain.
- `domain-io`: the domain's own code does no I/O, neither `os` nor `net`. Its tests may read
  fixtures.
- `slices`: a package under `internal/` that no rule names, held as a slice now that every
  command's slice is under `internal/slices` (T-157), imports nothing of ours but the domains, `internal/cli` and
  `internal/lang`, the domain library naming a source's units that every command measures by,
  with its port and, in its tests, the port's fake, so never another slice, an adapter, the `project` library
  an adapter wraps and lang's disk side, the `config` codec (mutation reads settings
  through mutants' `SettingsReader`, T-97), a slice's domain, or `cmd`. Its files are every package under `internal/`
  outside `internal/slices` except the ones the rule lists as something else, so a package no
  rule names is held as a slice, and it refuses each of them importing a slice under
  `internal/slices`. A slice's tests are in its own package, since the rule cannot tell a
  package's import of itself from another's.
- `slice-packages`: a slice under `internal/slices`, its subpackages with it, imports what
  `slices` allows and its own subpackages, so it can keep its domain, port, porttest and
  adapter apart (ADR-0051), the mutation slice's domain and its `porttest` among them. Its
  domain subpackage is held by `domain` instead, excluded here, and its adapters, under
  `internal/slices/<command>/adapters/<name>`, by their own rules, excluded here too, since
  this rule would refuse the libraries they wrap. depguard matches a prefix, so the rule allows `internal/slices/`
  and the sibling rule below refuses another slice; its allow list repeats `slices`', each
  domain added to both, until the last slice moves.
- `slice-adapters`: a slice under `internal/slices`, its tests too, imports no adapter of a
  slice, its own neither, since cmd wires them in through its `Ports` (decision 29). It holds
  `slice-packages`' files and denies each slice's `adapters` folder, which `slice-packages`'
  allowing `internal/slices/` would let by and the sibling rule lets a slice's own code
  import. It is a rule of its own, not a deny beside `slice-packages`' module-wide one,
  because depguard looks a deny up as the one entry sorting just before the import, so a
  longer deny there would hide the module-wide one from every import sorting after it.
- `slice-fakes`: a slice's own code imports no `porttest`, which only its tests may use (T-56).
  It holds the files of `slices` and `slice-packages`, tests left out, and names each port's `porttest`, since
  depguard matches a prefix and `slices` allows the ports' folders. It leaves out the mutation
  domain's folder, whose `porttest` builds on other ports' fakes; `mutant-fakes` keeps that
  fake out of the domain's own code. It leaves out a slice's adapters too, whose own rules
  refuse every fake in their code. `tools/bin/architecture-check`
  also refuses `itos-cc`'s binary linking any `porttest` (`go list -deps ./cmd/itos-cc`), since
  depguard sees only the imports a file names, not what a binary links through them: a fake
  enters `cmd` only through its `_test.go` files.
- Siblings: no package under `internal/slices/X` imports one under `internal/slices/Y`, from its
  code or its tests (ADR-0051). depguard cannot say "not a sibling", since its deny list is a
  prefix that would refuse a slice's own subpackages too, so `tools/bin/architecture-check`
  holds it over `go list ./...`'s imports and names both packages. A slice may import its own
  subpackages and any engine, and what is outside `internal/slices`, `cmd` above all, may import
  every slice.
- `ui`: `internal/cli` and its tests import nothing of ours, so adding or removing a domain
  touches no file of the UI (q-54).
- `workdir`, `metricsdir`, `coveragetools`, and the mutation adapters' `worktrees`,
  `testtools`, `testinputs`, `githistory`, `freshenv` and `importgraph` (`testtools` and
  `testinputs` the mutation slice's own, under `internal/slices/mutation/adapters`): an adapter imports
  nothing of ours but the port it implements and the libraries it wraps, `internal/workdir`
  the two ports it implements, selection's and `internal/lang`'s, and the `project` library,
  and `internal/metricsdir` the snapshot port and the `metrics` and `project` libraries; only
  its tests may use `porttest`. Each port's adapters have a rule of their own, so none is allowed another's port
  or library. An adapter may also import the domain it serves, and a port the domain it
  belongs to, to build or return the domain's own types with no mapper between them (decision
  47, amending 29's adapter rule): its rule allows that domain once the adapter needs it.
  An adapter may also use the shared pure domains its rule names, `internal/lang`,
  `internal/selection` and `internal/coverage`, which read nothing, as `freshenv`, `testtools`
  and `testinputs` do. The mutation adapters' rules and `workdir`'s and `metricsdir`'s allow
  `internal/slices/mutation/domain`, which the sibling rule lets them import: the slice's own
  adapters being the slice's, the others being outside `internal/slices`. `freshenv` serves mutants, yet its rule allows `internal/coverage` and its
  port too, since it reads Go coverage blocks (`coverage.GoBlock`) through that domain.
  `internal/coveragetools`' allows the coverage domain, whose `Report` it returns, beside its
  port and the `coverage` runner and `metrics` it wraps. Nothing leaks the other way: the
  `domain` and `domain-io` rules still refuse every adapter and I/O package in a domain.
- `processes`: the process-ownership engine, code and tests, imports nothing of ours (T-151).
  It is an adapter-side engine, not a domain: the `domain` and `slices` rules refuse it, so no
  domain or slice imports it, and of the adapters only `worktrees`' rule allows it, since
  mutation's workers run through it; cmd binds it for counted preparation and the coverage
  command. `slices` and `slice-fakes` leave it out, as they leave out the adapters.
- `mocks`: no mocking library anywhere. Fakes live in `porttest`.
- `rapid`: the property library only in the domains' own tests.
- `kong`: kong only in `cmd` and `internal/cli`. A slice's struct tags need no import.

### Adding a package

Lint refusing an import after you add a package means a rule has not been told what the package
is. Extend the rule as below; never loosen it, since every arrow above is what keeps the next
command from reaching into this one.

- A command: add its slice as `internal/slices/<command>`. The `slice-packages` rule holds it
  already. Give it scenarios, and in its own package, as `internal/slices/units` does:
  - `var Command = cli.Declaration{Name: "<command>", Help: "…", New: func() any { return &CLI{} }}`,
    its name a string literal;
  - `type Ports interface`, the ports its handler is built over, each a method named for the
    engine and the port, such as `SelectionTree()`, `LangSurroundings()`,
    `SelectionExclusion(ctx)` and `SnapshotStore()`, shared by every slice that needs it, and
    `Rel(path)`, a path named from the working directory as the UI names it;
  - `NewHandler(ctx context.Context, p Ports) (Handler, error)`, the one shape every slice's
    constructor has. What differs between slices goes behind a `Ports` method, never into the
    signature, and its error is returned as the port gave it, unwrapped.

  Add any `Ports` method no slice declared before to `cmd`'s `adapters` (`adapters.go`), once
  for every slice that declares it, the port it answers from added to `adapters` and
  `diskAdapters` as well. Then run `go run ./tools/bin/assemble` from the repository's root
  and commit `cmd/itos-cc/commands_gen.go`, which it rewrites: the slice's import, its command
  in its name's order and its handler's provider. Never edit that file by hand: CI's static
  step `go run ./tools/bin/assemble -check` fails, naming it, when it differs from what the
  slices declare. The generator refuses, naming the slice, a folder under `internal/slices`
  with no `Command`, a `Command` with no `Ports`, and two slices with one name.
- A slice's own domain: put it in the slice's `domain` subpackage, as
  `internal/slices/dry/domain`, and add its folder as a `**/internal/slices/<command>/domain/**`
  glob to `domain`, `domain-io` and `coverage-fakes`, and as an exclusion to `slice-packages`,
  never to an `allow` list: no other domain or slice imports it. A domain with external tests
  or a `porttest`, as `internal/slices/mutation/domain` has, goes on `domain`'s allow list with
  its `porttest`, `shared-domains` keeping the shared domains from it, and is excluded from
  `slice-fakes`, with a rule like `mutant-fakes` keeping its fake out of its own code. `tools/bin/domain-coverage`
  then holds it to 80%. Anything another slice needs from it moves to an engine first, as dry's
  fingerprints moved to `internal/lang` (T-148).
- A domain package: add its folder as a `**/internal/<name>/**` glob to `domain` and
  `domain-io`, `coverage-fakes` and `shared-domains`, as a `!**/internal/<name>/**` exclusion to `slices` and `slice-fakes`, and its
  import path to the `allow` lists of `domain` and of `slices` and `slice-packages` if commands use it. Give it a
  `port` subpackage for what it reaches outside, its `porttest` named in `slice-fakes`' `deny`,
  and a classifier for its failures beside the slice that uses it, which `cmd` adds to the UI's classifiers. `tools/bin/domain-coverage` reads its list from `domain`'s `files`,
  so the package is held to 80% from then on. If its tests use rapid, add a
  `!**/internal/<name>/*_test.go` exclusion to `rapid`.
- A slice's adapter, one that serves only that slice's domain: put it in
  `internal/slices/<command>/adapters/<name>`, as `internal/slices/mutation/adapters/testtools`,
  never in the slice's own package or under `internal/` beside the shared ones. Give it a
  depguard rule of its own, named for it, whose `files` are
  `**/internal/slices/<command>/adapters/<name>/**` with `!$test`, allowing the slice's domain
  it serves (decision 47), the shared pure domains it uses and the libraries it wraps, and
  denying the rest of the module. The slice's first adapter also adds
  `!**/internal/slices/<command>/adapters/**` to the `files` of `slice-packages`,
  `slice-adapters` and `slice-fakes`, and the folder's import path,
  `github.com/donvargax/itos-cc/internal/slices/<command>/adapters`, to `slice-adapters`'
  `deny`; the slice's later adapters touch neither. The sibling rule counts it as the slice's,
  so it imports the slice's domain and no other slice does. Wire it in `cmd` through the
  slice's `Ports`.
- Any other adapter: add its folder to the `files` of the rule for its port's adapters (`workdir` for
  selection's, `metricsdir` for the snapshot port's, `coveragetools` for coverage's), or give a new port's adapters a rule of their own allowing
  that port, any library they wrap and, when it returns the domain's types, the domain it
  serves (decision 47); add a
  `!**/internal/<name>/**` exclusion to `slices` and to `slice-fakes`. Wire it in `cmd`.
- Any other package that is no command, such as `internal/version`: add a `slices` exclusion
  saying what it is, and the same to `slice-fakes`.
- Then add compile-valid fixtures to `tools/testdata/architecture` that each rule you touched
  must accept and refuse. `tools/bin/architecture-check` runs the pinned linter over them, so a
  rule that stops matching fails there instead of passing silently.

A new sealed failure set, an interface with an unexported marker method like the selection
port's `Failure`, is annotated `//sumtype:decl` and given its exit codes in one switch that
names every kind, in its domain's classifier, as cmd's `selectionProblem` gives the selection
port's and `internal/slices/mutation`'s `failureError` words the slice's own `Failure`. gochecksumtype
then refuses the switch when a kind is added and left out; without the annotation nothing
checks it. CI's lint runs over the whole module, so it checks cmd's classifiers as well as the
switches in `internal/`.

Code that differs by OS takes the OS as a value. Logic never branches on `runtime.GOOS`. A
build-tagged file holds only a call that one OS lacks, with no mutation site: the scenarios'
interrupt step is the example, `os.Interrupt` on Unix and `CTRL_BREAK_EVENT` on Windows.

## Moving a command

A command parsed by hand moves onto kong as a slice of its own, printing the same bytes and
exiting with the same codes as before: a refactor, no behaviour change. `units` moved first
(T-32), and every step below points at what it did. A project made from go-template-itos
follows the same steps for a command it wrote before it had the template's design.

1. **The slice.** Make `internal/slices/<command>/<command>.go`, as `internal/slices/units` is laid out:
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
   named from the working directory by `ui.Rel`, or in a handler by its `Ports`' `Rel`, which
   cmd answers with `cli.Rel` from the same directory, not by a library an adapter wraps.
3. **Its classifier.** A domain's failures become problems in one classifier, held beside the
   one slice that uses the domain, as `internal/slices/mutation` words its own `Failure`s. A domain
   several commands share keeps its classifier in cmd (`classifySelection`), since no slice may
   import another; the moved slice then needs none. internal/cli never holds one.
4. **Routing in cmd.** The slice declares its command, a `cli.Declaration` named `Command`
   with its name, the line the top-level help lists it by and a constructor of a fresh `*CLI`,
   the ports its handler needs, a `Ports` interface it owns, and `NewHandler(ctx, p Ports)
   (Handler, error)`, as `internal/slices/units` does and "Adding a package" above says. A
   port it needs is added to `adapters` as a method and to `diskAdapters`, never handed as an
   adapter itself; a handler that holds what itos-cc.yaml says, as `units`' does, reads it
   through a `Ports` method under the context `NewHandler` is given, which kong calls only for
   the `Run` that asks for it. `go run ./tools/bin/assemble` then writes the slice into
   `cmd/itos-cc/commands_gen.go`, whose `assembled` puts the model's commands in their names'
   order; a group declares itself the same way, its `CLI` the struct of its subcommands, as
   `internal/slices/mutation` does. A Run that quotes a flag back as written asks for
   `cli.Written`, which `parserOf` binds for every command. `dispatch` needs no case: it calls the
   selected command's `Run` through `callSafely`, and admits any group's words before its
   subcommand. A command still on the
   hand parser leaves `legacy` and the in-process driver's refusal with its old file;
   `serve` is the last (T-112). The top-level help, `help <command>` and the guess for an
   unknown command read the one model (`cli.Dispatch`). A group, such as `mutation`, moves whole, and
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
   for `<command> --help` and `help <command>` both go through `modelHelp`.
8. **Its scenarios run through godog.** Convert the command's feature file before the move,
   so its scenarios hold it: its steps in `features/<file>.go`, registered from `Initialize`
   in `steps.go`, reaching the world only through its `Driver` and reading only what itos-cc
   prints and writes, and the file added to `Converted` in `steps.go`. The binary driver then
   runs its scenarios (Tests, below); a Given whose run measures coverage with the project's
   own tools declares them (`Driver.Coverage`), and one whose run runs the project's tests
   declares what they do (`Driver.Tests`): the binary driver skips the scenario where their
   tools are not installed, and the in-process driver records it for its fakes. A file too
   large for one task converts in pieces, the scenarios still without steps listed in
   `features.Unstepped`, which no run selects (mutate.feature, T-70 to T-81, its @wip
   Windows ownership scenarios still listed). The move adds the file to `InProcess`, and its scenarios run
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
   its own: its scenarios and tests hold it. Once the refactor's moves are done (`serve`
   stays on its legacy handler, its moves deferred), one task, refactor-final-proof, builds
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
10. **Boundaries.** The `slice-packages` rule holds the new package with no edit. A top-level library
    it imports that the rule refuses is either a domain library every command measures by,
    allowed in `slices` with a comment saying why (`lang` was, until T-36 made it the domain
    `internal/lang`), or one an adapter wraps, which stays refused (`project`). Add the slice to the fixture lists of
    `tools/testdata/architecture/check_test.go`, with every library you allowed or refused,
    and run `tools/bin/architecture-check` and `tools/bin/pinned golangci-lint run ./...`.
11. **Then the old tests.** Once CI has run the converted scenarios green, remove only the Go
    tests of `cmd/itos-cc` that are the same check as a scenario; keep any that holds what no
    scenario reaches.

Mutation's model-bearing `Workers` contract is declared in `internal/slices/mutation/domain`,
beside the domain values it consumes and returns. This is a narrow exception to
putting ports in a child `port` package: the domain's later orchestration must
use it without an import cycle. `internal/worktrees` implements it, running each
command through `internal/processes`' `Runner`, and
`internal/slices/mutation/domain/porttest` holds our fake and shared contract suite. Both import
the domain directly; the domain imports neither. No model-copy DTO, generic
result, or forwarding-only package is needed (decisions 29 and 47). Worker
requests name the semantic `WorkPurpose`; the adapter owns the exact scratch
prefixes. `Close` joins entered command/cleanup/restoration before removing
inputs, including on a cancelled caller. Preserve existing cleanup budgets: ordinary
aggregate cancellation uses local command deadlines. FailFastJudgments shares
from startup, including external cancellation before any actionable stop; Abort
starts the deadline before stop cancellation. Frozen trials share preparation's owner
from creation. Success/local timeout never activates a shared abort deadline.
Historical scratch removal remains
best effort. The pure Run and actual cmd composition never hide a missing binding. Counted preparation
uses its own shared owner; ordinary mutation workers retain independent budgets.


T-89 keeps the model-bearing `Toolchain`, `Fingerprints` and `Reach` contracts
beside those values too. `internal/slices/mutation/adapters/testtools` observes actual manifests, installed
runner entries, interpreter probes and runnable files/classes; pure mutants policy
constructs `Command`. `internal/slices/mutation/adapters/testinputs` owns current test/support/suite and
independent-coverage inventories, including lexical unsupported-scope admission.
It wraps the existing coverage library for unchanged Kotlin runner/wrapper
fingerprints, as testtools does for runnable-file/class observations.
`internal/importgraph` wraps the existing graph import resolver until S8. Only cmd
composes Reach: graph imports the mutation domain, so the domain cannot import that
adapter without a cycle. Each adapter has its own depguard rule, accepting only
mutants plus the actual libraries it wraps. The shared contract suites live in
`internal/slices/mutation/domain/porttest`; no production binary links them.

Cmd supplies all three actual bindings and rejects missing injections at a work
boundary. Graph receives read-only mutation inputs from composition while retaining its
existing discovery/cache until S8. Fresh/count preparation keeps its separately supervised
root-cwd Python probe; ordinary optional probing keeps the caller cwd. Existing coverage's pure
Gradle/Maven formatting helpers forward to their single mutants implementation
until the T-90 callers move. Python command discovery deliberately chooses the last
existing `.venv`/`venv` interpreter; coverage inputs choose the first. Pytest import
probes use the caller cwd, separately from the command root, and actual runtime
environment inputs are supplied at composition rather than read by the fake.

T-91 adds the model-bearing `History` contract beside these values.
`internal/githistory` wraps the existing project Git queries with an explicit
caller directory and context; `internal/slices/mutation/domain/porttest` holds immutable
commit trees, refs and parents captured from actual fixture setup. Cmd binds
all five existing query consumers: `--since`, default sample seed, deleted-file
exceptions and both committed/working orphan rename queries. A missing binding
fails at the actual consumer; explicit seeds and narrowed paths retain their
unused-query bypass. Caller-relative current-to-prior change maps remain separate
from repository-wide prior-to-current orphan maps. Frozen export, modes and
supervised counted preparation are owned by freshenv's coarse Fresh boundary.
The githistory rule admits only mutants and the project library it wraps.

T-92 moves ordinary Run planning, reuse, scheduling, judgment and publication order
into the mutation domain, now `internal/slices/mutation/domain`. Its `Options` contains actual bound ports and pure map
queries, with caller-relative paths resolved lexically from the supplied caller
folder. `MutationSnapshots` reads complete domain models; `metricsdir.Mutations`
performs concrete decoding before the domain checks the recorded file and only
then normalizes admitted TypeScript command scope. `SourceEdits` publishes the
summary using current source bytes. Snapshot write precedes moved-snapshot removal
and annotation; failed publication stops both. The shared `OrphanReason` policy
keeps follow-missing Stat separate from orphan Lstat observations.

`TestLister` returns stdout rows separately from streamed stderr and actual exit
reason. The testtools adapter owns execution/logging, while our fake consumes
independent root/command facts. Run lists only pending work, before report admission
and measurement, preserving fatal list failure versus ordinary best-effort coverage
failure. Existing MutationMeasurer/Fingerprints bindings replace captured live I/O
callbacks. Progress events are domain facts; the slice's `Progress` alone formats them,
cmd's run delegating to it.
Sample draws/checks in the domain and uses the same Execution scheduler, with
listed IDs admitted only for recorded listed scope.
Counted/frozen preparation uses Fresh. mutate.feature joined the in-process
feature driver at T-125, and metrics_snapshots.feature at T-101.

T-93 moves check, strict evidence inventory, site listing and orphan judgment into
the mutation domain. Check receives `MutationSnapshotReader`, never mutation write
capabilities; `MutationSnapshots` embeds that reader. A separate `MutationCatalog`
returns the store's regular-JSON names unchanged and observes only permissive
recorded-file metadata through the same metricsdir adapter. Full decoding remains
in its single concrete codec. `SourcePresence` is separate from follow-Stat
`Missing`: Lstat directories, dangling links and ordinary errors are present,
while lazy rename traversal requires successful `RenameTarget` evidence.

`CheckParsed` keeps recorded stale-but-judged mutants for Sample, except and renew
in the domain. Check alone hides those stale mutants.
List returns the ordered prefix before a later source failure; cmd renders it
before the error, whereas check admits ordinary and strict results before printing
checked files. Actual cmd and graph consumers use invocation-bound readers, catalog,
presence, Tree/Surroundings and existing Toolchain/Fingerprints. No legacy mutation
entry point remains, and no domain callback performs hidden I/O.
The existing inward depguard/fixture inventories cover these unchanged package
arrows; no production fake or adapter import is admitted to the domain.

T-94 moves Sample, Survivor, Renew and Elsewhere into mutants. Sample receives
read-only CheckOptions and execution bindings, never snapshot writes. It keeps
recorded scopes and seed ranking, reusing the one scheduler and selection-baseline
cache. Renewal and Unrenewed are genuine domain values; config's compatibility aliases
retain its codec callers without a second model or algorithm.

`Exceptions.Except` and `Exceptions.Renew` update the current document. The existing
config package implements them at an explicit RootDir, sharing one format-preserving
YAML transform with memory scenario bytes. Initial Config selects entries; updates
reread current bytes, preserve comments/settings/permissions, and consume complete
Old entries once, validate the whole batch, then write once. Invalid document
reason/key stays distinct from ordinary I/O and cancellation causes. CLI supplies a
pure root-to-Exceptions constructor only after existing root discovery, preserving
lazy help/version startup. Unit fakes own copied exception values and no codec.

`SourceStatus.Kind` observes following-Stat missing/directory/non-directory facts,
independently of conservative orphan Present and successful RenameTarget Lstat.
Survivor admits syntax first and maps ordinary Stat failure to its no-survivor reason;
Renew permits moved search for missing/directory/unsupported sources and propagates
other Stat failures. Elsewhere uses explicit root/caller paths and lexical pure
judge/exclusion queries, requesting parsing only for actual orphan work. Existing
boundary lists already admit these inward arrows and refuse domain-to-config or
workdir dependencies; no new adapter package or rule exception is required.


## Tests

Each domain's unit tests use its `porttest` fake and assert results and typed failures.
The adapters' tests use real temporary files, and workdir's a real git repository, since
translating OS and git errors is what an adapter does.

A fake stands in for its adapter only when both pass one contract suite (decision 44). Each
port's `porttest` exports it from `contract.go`, code that imports `testing` and no test file,
so the adapter's tests can run it: `TreeContract(t, open)` for selection's `Tree` (T-52) and
`StoreContract(t, open)` for the snapshot `Store` (T-53), `SurroundingsContract(t, open)`
for lang's `Surroundings` (T-54), coverage's `FilesContract(t, open)` for its `Files` (T-64) and `MeasurerContract(t, open)`
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
build output are copies of `project`'s, which the suite keeps honest.

The scenarios in `features/` run through godog with two drivers (decision 44), the same steps
of package `features` behind its `Driver`. `features`' `TestFeatures` runs every converted
file against the built binary (`binary.go`), which proves main's wiring, the real adapters and
tools and exit codes. Its interrupt steps, which signal serve, crap or a shell running itos-cc
once its output shows it under way, are the binary's alone (`interrupt_test.go`). `cmd/itos-cc`'s `TestScenarios` runs the files of the moved commands
(`InProcess`), every converted file since `cli.feature` joined it (T-114), in process: its driver, a `_test.go`, holds every
folder and file in memory with independent fake Git staging, immutable commits, branches, tags and tracked
renames (`--changed` is what differs from the last commit, and every file before one), and each run calls `run` with buffers and
porttest's fakes built from that memory: `TreeOf` over the whole scratch folder, files
outside the run's folder named by `../`, lang's `Surroundings`, snapshot's `Store` written
back to `.metrics/`, an itos-cc.yaml held in memory, and coverage's `Measurer`, which reads
the reports a run names from the memory and gives each build root what the scenario
declared its tools measure (`Driver.Coverage`): the LCOV report they write, nothing when
they write none, a missing tool when none was declared, and no report left for
`--use-existing-coverage`, writing what their command prints on success where the collectors
pass it on. A mutation run is handed mutation's fakes
(`internal/slices/mutation/domain/porttest`) over the same memory: `Workers` running each mutant as the
scenario declared the folder's tests do (`Driver.Tests`), the `History` of the fake Git,
`Toolchain`, `Fingerprints` and `Reach` from the files, the run's `Run.Env` and the import
adjacency the world declares before each run (`features/imports.go`), the listed tests'
`Lister`, coverage's `MutationMeasurer`, and a counted run's `Fresh` over the fake
repository. It refuses `serve`, still on its legacy handler, so nothing reaches the real disk
or network. A scenario only one driver can run carries `@binary-only` or `@in-process-only`,
never a copy, and the other passes it by; a step a driver cannot do fails naming the tag, or
skips with a typed reason (`features.ErrPhysicalUnsupported`), which the in-process run logs.
`features.Deferred` names the scenarios of `InProcess` files the in-process driver leaves to
the binary for now, each with why and the work that owns it: the five mutation scenarios
that also observe `serve`, which stays on its legacy handler until its own web app (T-102 to
T-108, T-112, T-113), and, by its Examples block's tag, `cli.feature`'s exit-code row that
runs `serve` on a port in use (T-112). The in-process driver runs one scenario at a time, the binary driver as many at once as the CPUs, each scenario isolated (`features/README.md`, T-128). itos.yaml's scenario lines run both tests, so `cd.yml`
holds every scenario both ways, and itos's scenario step each one it selects. The binary
driver runs only where it is opted into (decision 52, T-153): `ITOS_CC_FEATURES_BINARY_DRIVER=1`,
which every workflow sets in its top-level `env`, or `ITOS_CC_FEATURES_BINARY` naming a binary.
Anywhere else, a local `go test ./...` or itos.yaml's scenario lines among them, `TestFeatures`
logs that the binary driver ran nothing, naming the variable, builds nothing and passes, so a
local run holds the scenarios in process against the fakes alone, and the `@binary-only` and
`Deferred` scenarios run in CI (the smoke set and those the range names) and CD. An agent working
on a `@binary-only` scenario sets the variable itself. A second Ctrl-C
ending the process at once, unless owned process groups are being killed, is held by
`internal/cli`'s unit tests, which feed `Interrupts` fake interrupts on every system.

Properties use `pgregory.net/rapid` and live in the domain's tests; the `rapid` rule admits it
in `internal/selection`'s alone. The selection property checks that paths and fragments only
narrow the working directory: what they select is exactly the files of `.` under a path that
is there or holding one that is not. `tools/bin/domain-coverage` runs the domains' tests with
`RAPID_CHECKS=1`, one new random case per property, as fast as an example test; the Go test
jobs of `ci.yml` and `cd.yml` set nothing, so a property runs rapid's default 100 cases there.
Either prints the seed on failure. This project has no scheduled high-count run.

CI has two tiers (decision 48). `ci.yml`, the push gate itos waits on, runs on Linux alone:
the plan below, with the smoke set and the scenarios the push's range names through both
drivers, and the Go tests of every package, the in-process driver's whole suite among them.
`cd.yml`, on the same push, runs every test on Linux, macOS and Windows in its platform jobs,
every scenario through both drivers, and the release after them. The import-boundary
self-test, `tools/bin/architecture-check`, runs in the plan of both. Both set
`ITOS_CC_FEATURES_BINARY_DRIVER`, as `nightly.yml` and `refactor-proof.yml` do, the binary
driver running nowhere else (decision 52).

## Quality checks

`itos.yaml`'s CI plan (`ci.steps`), which the ci jobs of `ci.yml` and `cd.yml` both run, runs
these checks in cost order, the first failure stopping the run:

- `itos config check` and `tools/bin/doc-caps`, the only steps of a push that touches prose
  alone (`ci.prose`).
- `gofmt` over every tracked Go file, `go build ./...`, `go vet ./...`, `go mod verify` against
  `go.sum`, and `go mod tidy -diff` so the module files match the code.
- `itos tests smoke check scenario`, so every feature file has a smoke scenario.
- The pinned golangci-lint over the whole module, `./...` (q-50, T-119): the standard linters plus
  depguard, gosec on non-test code, revive's file-length limit, gochecksumtype, which refuses
  a type switch over a sealed set that leaves a kind out, a default arm not counting, and the
  context rules of decision 30: containedctx, which refuses a context held in a struct, and
  revive's context-as-argument, which refuses one passed anywhere but first. revive runs only
  the rules `.golangci.yml` lists, never its defaults. `cmd`, `internal`, the top-level
  libraries, the scenario harness in `features` and `tools` are all linted. A finding that is
  by design carries a `//nolint:<linter> // <reason>` at its own site, saying why.
- `tools/bin/architecture-check`, the self-test above.
- `go tool govulncheck -test ./...`, covering the dependencies in use, test-only ones and
  the standard library.
- `tools/bin/domain-coverage`, which requires each domain package to reach 80% statement
  coverage from its own tests.
- The scenarios, run once: the smoke set and those the push's range names, through both
  drivers.

The Go tests of every package are not a plan step: `ci.yml`'s test job runs them on Linux and
`cd.yml`'s platform jobs on Linux, macOS and Windows (decision 48).

Mutation proof is off, and nothing here claims a mutation result for a push. The plan has no
`proof.code` step and no `itos-cc mutation sample` (q-53): whether itos-cc sets itos's
`proof.code` is the idea p1-itos-proof-code, waiting on T-46 alone now that the refactor's
final proof (T-132) is done. The nightly mutation workflow, `nightly.yml`, is disabled on GitHub
(`gh workflow disable Nightly`) until T-46 makes a whole run finish and commit its results.
`.metrics/mutate/` holds the results committed before both were turned off.

Each check runs where its cost fits:

- On every commit, the project's `pre-commit` hook, `tools/hooks/pre-commit`: gofmt over the
  staged version of each Go file the commit stages, and nothing else, so a commit costs a
  fraction of a second and no network. Each clone declares it (README.md, Hooks);
  `tools/bin/pre-commit-check` proves it with real commits in a repository of its own (T-120).
- itos's own hooks (`itos hook install`), `commit-msg` on every commit and `pre-push` on every
  push.
- On every push touching more than prose, the CI plan above: gofmt over every tracked Go file,
  which also catches a commit made in a clone without the hook, and the module-wide
  golangci-lint and the networked govulncheck, which the template's `tools/hooks/pre-commit` ran
  on every commit staging code. They are slower and need the network or a download, so they
  stay here, where they ran before this project declared any pre-commit hook; none was moved or
  turned off.

Tools are pinned:

- The Go toolchain is pinned in `go.mod`, and govulncheck is a `tool` dependency there.
- `tools/bin/pinned` fetches golangci-lint, itos-cc, gitleaks, GoReleaser and git-cliff at
  fixed versions and checks each against a pinned SHA-256, copied from its release's own
  checksums or, for git-cliff, taken once the archive matched its release's `.sha512`.
- Workflow actions are pinned by digest.

`docs/upgrading.md` says how to move any of these pins.

## Setup

itos-cc adopted go-template-itos v0.2.0 with `itos-template adopt --merge` (T-26), so it has its
own `itos.yaml`, ledger and work registry rather than a fresh project's. `itos-policy.yaml` is the
template's policy file, which `itos.yaml`'s comments cite as the source of the plan and the
scenario runner they took. `itos-cc.yaml` holds itos-cc's own settings and two reviewed
exceptions, both of `internal/release`: the template's parked exception for its main.go went
with T-115, and its sample count app, which had none, with T-116.

T-90 added the model-bearing `MutationMeasurer` contract beside `Report` and
`TestCoverage`, composing aggregate measurement with `MeasureListed` and
`Inventory`; T-135 moved that aggregate `Measurer` beside it, a plain interface
over the domain's `Report`, from the port's `Measurer[R]`. `Test` and `PerTest` are domain values; `PerTest.Select` constructs
a command without I/O. The root-owned fake and shared suite belong in
`internal/coverage/porttest` and reuse the child aggregate fake. This is the
same narrow model-bearing recipe exception as mutation's root contracts,
not permission for production domain code to import fakes or adapters.
