<!-- T-86's ownership inventory, written 2026-10-20 at 2f63821 by a read-only survey; line numbers are as of that commit. The decisions it asks for are settled in ADR-0051. -->

> **Historical.** This is the survey T-86's slice tasks were planned from, kept as it was made: its
> paths and line numbers are those of `2f63821`, and every package section 1 lists for a command,
> its domain or mutation's adapters has moved since, as have `graph/tests.go` and `config`'s
> exceptions codec. ADR-0051 settled section 7's questions. Today's
> layout is `docs/go.md`'s; the last section says where each move put each package and which work
> did each of section 6's steps.

# T-86 inventory: feature-owned vertical slices

Read-only survey of `/home/yorch/tmp/llms-safe/code-quality` at `2f63821` (2026-10-20). Sources: T-86's
ledger entry (`tasks/phase-1.yaml:2658`), `docs/engineering.md`, `docs/go.md`, `docs/architecture.md`,
`docs/vertical-slices-proposal.md` (PROPOSED), ADR-0029/0030/0034/0035/0037/0039/0041-0050,
`.golangci.yml`, `tools/bin/architecture-check`, `tools/testdata/architecture/check_test.go`,
`go list` import graphs, and kong fork `github.com/donvargax/kong v1.16.2-itos.1` (module cache).
Sizes are `.go` files and lines including tests. All T-86 dependencies are done (T-141, T-132,
slice-7, slice-8, T-140). The ADR-0047 review T-86 says to fold in is T-140, done 2026-10-18, with
findings A47-1..10 settled. Nothing is left from it except the layout ADR revision.

## 1. Today's layout

### cmd and internal/

| Package | Holds | Used by | Size |
|---|---|---|---|
| `cmd/itos-cc` | main, `run`, the kong model `commandLine`, `parserOf`, `dispatch`, `adapters`/`diskAdapters`, three shared classifiers, legacy hand parser + `serve`, mutation counted roots; tests: in-process driver `TestScenarios` + ~70 mutation integration tests | every command | 11 src (1,173 l) + 95 tests (15,426 l; ~11.2k mutation, ~2.9k scenarios driver) |
| `internal/cli` | shared UI: Problem, exit precedence, `Read/Parse/Dispatch/GroupLine/Usage/Help/Page`, `Flags`/mappers, `Provided`, `Interrupts`, `Selection`, `Version` cmd, `CommandNeedsReport`, unused `Completion` | all slices, cmd | 28 f / 3,892 l |
| `internal/units` | `units` slice (CLI/Contract/Query/Handler/Run) | units | 2 / 299 |
| `internal/dry` | `dry` slice | dry | 2 / 491 |
| `internal/scrap` | `scrap` slice | scrap | 2 / 426 |
| `internal/crap` | `crap` slice, coverage flags | crap | 2 / 485 |
| `internal/mutation` | mutation group + 5 children (CLI, contracts, queries, handler, sealed `Failure` classifier, presentation/progress) | mutation | 31 / 5,244 |
| `internal/selection` (+`port`, `port/porttest`) | file selection domain, `Select`, `Parse`, `Exclusion`; `Tree` port, fake, `TreeContract` | every command, freshenv, mutants | 4/578 + 1/84 + 3/647 |
| `internal/lang` (+`port`, `port/porttest`) | parsing engine: tree-sitter, units, namespaces, complexity, syntax and mutation rules; `Surroundings` port | every command, graph, project, coverage runner, adapters | 9/2,931 + 1/28 + 3/256 |
| `internal/snapshot` (+`port`, `port/porttest`) | `.metrics/` naming/version; `Store` port, fake, `StoreContract` | dry, scrap, crap, mutation, metrics, project | 2/67 + 1/96 + 3/502 |
| `internal/duplication` | dry's domain; also exports `Fingerprint`/`Jaccard` | dry, **teststructure** | 2 / 563 |
| `internal/teststructure` | scrap's domain | scrap | 6 / 1,582 |
| `internal/changerisk` | crap's domain (`Analyze`, `Score`, `Measured`) | crap, **graph** | 2 / 238 |
| `internal/coverage` (+`port`, `port/porttest`, `porttest`) | coverage engine domain: `Report`, parsers, `Measurer`, `MutationMeasurer`; fakes + contracts | crap, mutation, mutants, freshenv, coverage runner | 8/1,643 + 3/175 + 5/584 + 4/965 |
| `internal/mutants` (+`porttest`) | pure mutation domain + model-bearing ports (Workers, History, Toolchain, Fingerprints, Reach, Fresh, SettingsReader, Exceptions...), fakes + contracts | mutation, **config, project, coverage runner, graph**, all mutation adapters, metricsdir, workdir | 79/9,962 + 35/5,710 |
| `internal/workdir` | adapter: selection `Tree`, lang `Surroundings`, mutants `SourceEdits`/presence/status | cmd (all) | 9 / 603 |
| `internal/metricsdir` | adapter: snapshot `Store` + mutation snapshot codec `Mutations` | cmd, graph | 10 / 1,193 |
| `internal/coveragetools` | adapter: coverage `Measurer`/`MutationMeasurer` over top-level `coverage` | cmd (crap, mutation) | 2 / 316 |
| `internal/worktrees` | mutation Workers, `OwnedExecutor` (also runs crap's coverage command) | cmd | 23 / 2,639 |
| `internal/testtools` | mutation Toolchain/Lister | cmd, freshenv tests | 6 / 676 |
| `internal/testinputs` | mutation Fingerprints/inventories | cmd | 12 / 2,002 |
| `internal/githistory` | mutation History | cmd | 2 / 128 |
| `internal/freshenv` | mutation counted `Fresh` | cmd | 11 / 2,736 |
| `internal/importgraph` | mutation Reach over `graph.TestsImporting` | cmd | 2 / 54 |
| `internal/version` | stamped version. **No importer**; `.goreleaser.yaml:28` stamps it while cmd uses `main.version` (`main.go:21`). Worth checking separately. | none | 2 / 190 |
| `internal/release` (+`releasetest`) | release tools library | `tools/bin/release-*` | 3/527 + 1/143 |

### Top-level libraries (not yet moved, ADR-0037)

| Package | Holds | Used by | Size |
|---|---|---|---|
| `coverage` | coverage runner: each language's tools, per-test, supervised, inventory, `ExecArg` | coveragetools, testtools, testinputs, freshenv, graph, cmd `init` | 18 / 4,327 |
| `project` | discovery, git queries, `--since` diff, `Surroundings` disk side | workdir, metricsdir, mutation adapters, graph, config | 15 / 1,688 |
| `config` | itos-cc.yaml codec; `Reader` (implements `mutants.SettingsReader`), `Exceptions` writer | cmd, freshenv, graph | 8 / 1,030 |
| `metrics` | snapshot JSON bytes, atomic rename | metricsdir, coveragetools, graph | 6 / 487 |
| `graph` | architecture graph build (imports, metrics, mutants freshness, CRAP), HTTP handlers | server, importgraph, cmd serve | 12 / 2,466 |
| `server` | serve's HTTP/watch | cmd serve | 1 / 195 |
| `viewer/` | separate npm web app | serve | n/a |

### features/ (package `features`, 161 files / 41,287 lines, excluded by `itos-cc.yaml:5`)

- Shared harness, about 6.5k lines: `steps.go` (`Driver`, `world`, `Initialize`, `Converted`, `InProcess`,
  `Unstepped`) 1,567, `binary*.go` 677, `build.go`, `git.go`, `fixture_peers*`, `running*`, `signals*`,
  `lock_*`, `inventory*`, `observation*`, `read_observations*`, `timestamps*`, `tool_setup*`,
  `executable*`, `ordered_report_test.go`, `imports.go`, `deferred.go`, `features_test.go`
  (`TestFeatures`), `interrupt_test.go`.
- Per feature: `mutate*` 90 files / 23,439 lines; `coverage*` 2,917; `scrap*` 2,368; `dry*` 1,467;
  `crap*` 1,237; `selection*` 1,188; `metrics_snapshot*` 1,035; `cli*` 956; `units*` 865;
  `complexity.go` 164.
- Feature files: mutate 3,569 lines; architecture_graph 389; coverage 364; cli 358; scrap 327;
  units 265; file_selection 214; viewer 188; dry 172; complexity 132; crap 133;
  metrics_snapshots 111; serve 83; completion 73 (all `@wip`). serve, viewer and architecture_graph
  have no steps and are not in `Converted`.

## 2. Candidate feature slices (what each owns today)

- **units**: `internal/units`; `features/units.feature`, `units.go`, `units_routes.go`; smoke `@ID-UNIT-*`.
  Ports used: selection `Tree`, lang `Surroundings`. No classifier, no snapshot, no domain of its own.
  Shares `cli.Selection` and the selection/lang engines. Cmd edits: `dispatch.go:37,166,170-171`.
  This is the least-shared slice.
- **dry**: `internal/dry` + domain `internal/duplication`; `dry.feature`, `dry*.go` (4); ports Tree,
  Surroundings, snapshot Store. Shares `duplication.Fingerprint/Jaccard` with scrap
  (`internal/teststructure/analyze.go:165,184,202`, about 120 lines of syntax fingerprinting,
  `internal/duplication/dry.go:211-330`). Cmd edits: `dispatch.go:33,161-168,175`.
- **scrap**: `internal/scrap` + `internal/teststructure`; `scrap.feature`, `scrap*.go` (7). Its
  handler is bound eagerly at `dispatch.go:73`, not in `dispatch`. It reads no config.
- **crap**: `internal/crap` + `internal/changerisk`; `crap.feature`, `crap*.go`; also drives
  `complexity.feature` (`features/complexity.go:57` runs `crap --no-coverage`), `coverage.feature`
  and most of `file_selection.feature` (12 of 15 runs are crap). Ports: Tree, Surroundings, Store, coverage `Measurer`.
  Shares the coverage engine with mutation, the `coverageCommand` executor (`cmd/itos-cc/coverage.go:22`,
  built over `worktrees.OwnedExecutor`) and `cli.CommandNeedsReport`. `changerisk.Score` (4 lines,
  `crap.go:34`) is also used by graph (`graph/build.go:452`). Tests: `cmd/itos-cc/crap_unloaded_test.go`,
  `coverage_test.go` (classifier), `scenarios_coverage_trace_test.go`.
- **mutation** (run/check/list/sample/except, one feature): `internal/mutation`, `internal/mutants`
  (+porttest), adapters `worktrees`, `testtools`, `testinputs`, `githistory`, `freshenv`, `importgraph`;
  the mutation codec in `internal/metricsdir` (`Mutations`, wires `snapshotWire` and others); the exceptions codec in
  `config` (`exceptions.go`, `exception_document.go`); `cmd/itos-cc/mutate_counted.go`, the 19
  mutation fields of `adapters` (`adapters.go:37-55,72-79`), the mutation half of `diskAdapters`
  (`:99-109`), `dispatch.go:118-158`, the `mutation_workers` Var (`:64`); about 70 cmd test files
  (`mutate_*`, `mutation_*`, `strict_*`, `except_*`, `orphans_*`, `uncovered_*`, `whole_suite_*`,
  `scenarios_counted/exceptions/listed_setup/workers/...`); `mutate.feature`, `mutate*.go` (90),
  `metrics_snapshots.feature` (mostly mutation); `.metrics/mutate/`. It shares the coverage engine,
  selection, lang, snapshot, the `exclude` setting and `OwnedExecutor`.
- **serve / architecture graph / viewer** (legacy, deferred T-102..T-113): `cmd/itos-cc/serve.go`,
  `cli.go` (hand parser, 269 lines), `legacy` map (`dispatch.go:48`), `graph`, `server`, `viewer/`;
  features serve/viewer/architecture_graph (no steps). It consumes mutation's domain (`graph/build.go:52,
  97,146,438-470`: Snapshot, FreshnessOf, Stale, OnSites, Fingerprints), crap's `changerisk.Score`,
  `config.LoadFrom`, `metricsdir`. Mutation consumes graph through `importgraph`
  (`graph.TestsImporting`, `graph/tests.go:30`). This is the main real cross-feature dependency, in both directions.
- **cli / version / help** (the top-level entry): `internal/cli` (`Version` at `version.go:8`, `Dispatch`,
  `Usage`), `cmd/itos-cc/main.go`, `cli.feature`, `cli*.go`. `completion.feature` is all `@wip` and
  `cli.Completion` (`internal/cli/completion.go:13`) is wired to nothing.
- **file selection** (`file_selection.feature`, `selection*.go`), **coverage** (`coverage.feature`,
  `coverage*.go`), **complexity** and **metrics snapshots** are specs of shared engines
  (selection, coverage, lang, snapshot) exercised through crap, mutation and units. They are not commands. See section 3.

## 3. Shared code

### Engines to keep behind stable explicit APIs

- **lang** (`internal/lang` + port): parsing, units, complexity, mutation-site rules, used by everything.
  Candidate to absorb `duplication.Fingerprint/Jaccard` (syntax-tree fingerprinting, used by dry and scrap)
  rather than duplicating 120 lines.
- **selection** (+port, `workdir` adapter): every command's file choice and `Parse`.
- **coverage engine**: `internal/coverage` (+port, fakes), `coveragetools`, top-level `coverage`. Used by crap
  and mutation, with mutation-specific `MutationMeasurer`/`MeasureListed` living beside the engine
  (`internal/coverage/measurement.go:35`, go.md:841-848).
- **snapshot** (+port, `metricsdir.Store`, `metrics`): `.metrics/` format, shared by dry, scrap, crap,
  mutation and serve.
- **git/history and discovery**: the `project` library, wrapped by `workdir`, `githistory`, `metricsdir`.
- **process ownership**: `worktrees.OwnedExecutor` plus `cli.Interrupts`/`Cleaning` (decision 38). It is used by crap's
  coverage command and mutation. Today it lives in a mutation adapter, which is an accident of location (see below).
- **settings**: itos-cc.yaml `exclude` (decision 45) is read by crap, dry, units and mutation (`exclusionOf`,
  `adapters.go:127`).
- **UI**: `internal/cli` (problems, exit precedence, help/Page, Flags, Dispatch, Interrupts).
- **scenario harness**: `features` `Driver`/`world` and the binary driver, plus the in-process `memory` driver
  in `cmd/itos-cc/scenarios_test.go` (one fake world for all features).
- **import resolution**: `graph.TestsImporting`/`parseDependencies`, used by serve and mutation reach. This is an engine
  that today lives inside serve's package.

### Shared by accident (move to one owner, or duplicate a small piece)

- `mutants.Config` holds `Exclude`, a cross-command setting (`internal/mutants/settings_reader.go:7-16`);
  `config.Reader` implements `mutants.SettingsReader`; crap, dry and units get `exclude` through the
  mutation domain. Split: the exclusion setting goes to selection/settings, exceptions and tests to mutation.
- `config` exceptions codec (`config/exceptions.go`, `exception_document.go`): mutation-only.
- `metricsdir.Mutations` codec: mutation-only, inside the shared snapshot adapter.
- `project/since.go` uses `mutants.ChangedFunctions/LineRange/OverlapsRanges`; `project/project.go`
  uses `selection.Files`: the library imports domains (inward-pointing, but couples the library to mutation).
- `coverage/run.go` uses `mutants.MavenTests/GradleTests` (forwarders noted in go.md:607).
- `worktrees.OwnedExecutor` is used for crap's coverage command (`adapters.go:100`, `coverage.go:22`).
- `changerisk.Score` in graph: a 4-line formula. Duplicate it in graph/serve, or bridge it in assembly.
- graph reads mutants judgment (freshness/stale): serve is a real consumer of mutation's model.
  This is the proposal's Option 2 test case (consumer-owned interface plus a bridge outside both features).
- `cli.CommandNeedsReport` (`internal/cli/coverage.go`): coverage wording in the UI, shared by crap and mutation run.
- `cmd/itos-cc/coverage.go` `classifyCoverage`, `selection.go`, `snapshot.go`: engine classifiers in
  cmd. They can stay as engine-owned classifiers handed to the UI by assembly.
- `internal/version` (unused) and `cli.Completion` (unused): template leftovers.

## 4. Central edit points a new command needs today

1. `cmd/itos-cc/dispatch.go:31-39`: a `commandLine` field with `cmd:""` and the top-level help tag, in name order
   (the unknown-command guess breaks ties by this order).
2. `dispatch.go:10-16`: import of the slice.
3. `dispatch.go:61-75` `parserOf`: eager `kong.Bind` of a handler (scrap `:73`), `kong.Vars` for
   slice values (`mutation_workers` `:64`).
4. `dispatch.go:113-184` `dispatch`: hard-coded `"mutation"` group admission and help (`:118-135`),
   handler binding plus a `switch` case per child (`:141-158`), lazy-config binding for crap, dry and units
   (`:161-175`), `case` per command with `callSafely(..., c.X.Run, c.X.JSON)` (`:170-180`).
5. `cmd/itos-cc/adapters.go:36-80` (`adapters` fields), `:97-110` (`diskAdapters`); a test-side
   twin at `cmd/itos-cc/scenarios_test.go:530-542` (in-process driver) and `run_fakes_test.go:37` (`fakes`).
6. `cmd/itos-cc/main.go:76` `classifiers` (a new engine classifier).
7. `features/steps.go:344-348` `Converted`, `:396` `InProcess`, `:661-670` step registration
   in `initialize`; `features/deferred.go:33` `Deferred`; `:358` `Unstepped`.
8. `features/smoke.yaml` (one smoke scenario per feature file, enforced by `itos tests smoke check scenario`).
9. `.golangci.yml`: `slices` holds a new slice with no edit (`:127-167`, files `**/internal/**`). A new
   domain needs `domain` `:63-88`, `domain-io` `:94-103`, `slices`/`slice-fakes` exclusions `:131-150`/
   `:182-201`, `coverage-fakes` `:228-237`, `rapid` `:418-423`. A new adapter needs a rule (`:268-400`).
   `kong` rule `:428-437` keeps kong out of slices.
10. `tools/testdata/architecture/check_test.go`: the package lists at `:83, :167, :222, :254, :290, :297,
    :318, :325` name every slice (`internal/next` is the generic new-slice probe).
11. `tools/bin/domain-coverage` reads the `domain` rule's files (lines 24-42). A new domain is measured automatically.
12. Docs: `docs/go.md:16-351` (package list), `:359-399` (boundary list), `:401-425` (Adding a
    package), `:471-481` (routing step); `docs/architecture.md`; `README.md:964-976` (Layout table) and
    `## Use` sections; `docs/CLI.md:100+` (Problem rules table naming commands); `features/README.md:137`.
13. CI/tests: `.github/workflows/cd.yml:157-161` splits `./cmd/itos-cc` tests by name, `:170` lists
    packages excluding cmd and features; `itos.yaml:89-107` (scenario root `features`, run commands
    naming `./features ./cmd/itos-cc`).
14. `itos-cc.yaml:5` exclude `features/**` (step code moved elsewhere would start being measured).

## 5. Constraints the move must keep

- **Help/errors/exit codes**: `cli.Dispatch` reads the whole line against one kong model
  (`main.go:112`); `cli.Read` makes help win over mistakes; `ParseProblem` maps the fork's typed errors
  (ADR-0035, go.md:504-522); exit precedence (ADR-0034/0036); interrupt 130 (ADR-0038). The
  top-level help order and guess tie-break follow the order of the model's children. Group admission keeps
  "before-child help/error precedence" and strips a leading group `--json` (go.md:226-229).
- **Lazy config for help**: itos-cc.yaml is read only after parse, never for help or `version`
  (`dispatch.go:50-57,104-108,161-165`); `config.invalid` only fails that command.
- **Fresh per-run parser state**: `run` builds `commandLine` and the parser once per run (`main.go:107-108`);
  `cli.Flags` holds one parse's state. No package-level command structs.
- **Domain purity and inward arrows**: decisions 29, 47 and 50, plus the depguard rules; `porttest` linked only by tests
  (`architecture-check` `linked`). Directory ownership must not merge a domain with its adapters into
  one package: inside a feature, keep `domain`, `port`, `porttest` and adapter subpackages, or the `domain-io` rule breaks.
- **Context**: ctx first and never held (ADR-0030; containedctx, revive).
- **OS as a value**, no `runtime.GOOS` in logic (`countedPlatform`, `Platform` binding).
- **No DI framework, no global init registry, no new dependency without a recorded selection**. Today's
  only `init` is the coverage exec hook (`main.go:90`). A generator must be stdlib (`go/parser`) or itos-native.
- **Scenario IDs, text, notices**: a live scenario's text changes only in feat/fix; IDs stay; `CONTRIBUTORS.md`
  and harvest comments are preserved (`.golangci.yml:19,28`). `.metrics/mutate` snapshots and
  itos-cc.yaml exceptions follow moved files (ADR-0014/0037; today the only exceptions are on
  `internal/release`, and `.metrics/mutate` holds only top-level legacy packages).
- **No local mutation runs** (AGENTS.md); proof stays off (ADR-0040); CI tiers (ADR-0048).
- **kong fork APIs usable for slice-local declarations** (v1.16.2-itos.1, upstream API plus typed errors):
  - `kong.DynamicCommand(name, help, group string, cmd any, tags ...string)` (`options.go:100`): builds a
    root child from a pointer to a slice's CLI. Children are appended in registration order *after* struct
    fields (`kong.go:142-160`), so assembly must register them sorted by name to keep help and guess order.
  - `kong.Plugins` (`build.go:13`, a field of `[]any` embedded structs) and `kong.Embed` (`options.go:76`).
  - `kong.BindToProvider` / `BindSingletonProvider` (`options.go:269,284`; `Context.BindToProvider`
    `context.go:232`): providers run only when a Run's parameters need them, so itos-cc.yaml can be read
    lazily by a `func(ctx) (selection.Exclusion, error)` provider. Provider errors come back unwrapped
    (`callbacks.go:273-274`), so `config.invalid` keeps its problem.
  - `Provide*` methods on command structs are picked up only by `Context.Run/RunNode` (`context.go:935-972`),
    not by `Context.Call`, which cmd uses because `Run` returns `int`.
  - `Context.Call(fn, binds...)` (`context.go:923`) plus `parsed.Selected().Target` lets one generic call reach
    any selected node's `Run` with no per-command `case`.
  - The `kong` depguard rule allows the import only in cmd and internal/cli. Slice declarations must be plain data or
    struct tags, with kong options built in generated cmd code.

## 6. Proposed order of small tasks

Each task lands alone with green CI. Checks are targeted (per memory: no whole suites as done_when).
"ADR" means through `itos decision`.

0. **D0, decision (person)**: a new ADR superseding ADR-0029 (layout), amending ADR-0037 (moves),
   ADR-0044/0039 consequences (where steps and the in-process driver live), and noting that ADR-0047/0050 stand.
   It settles the questions in section 7. No code. Check: `itos config check`.
1. **R1, generic dispatch, no layout change** (cmd only). Replace the `switch`es in `dispatch.go:145-183` with
   one call of `parsed.Selected()`'s `Run` (reflection via `Call`), group admission for any node with children
   instead of `"mutation"`, and `--json` from a method or `line.JSON`. Bind handlers through
   `parsed.BindToProvider` so config is read only when a Run needs it. Check:
   `go test ./cmd/itos-cc -count=1 -run '^(TestScenarios|TestDispatch|TestTheModelNamesEveryCommandInOrder|TestEveryCommandsHelpGivesItsContract|TestRunReportsAConfigThatCannotBeReadOnlyWhereItIsRead|TestCallSafelyReportsABugAsAnInternalFailure)$' -scenarios='^@ID-(CLI|UNIT|DRY|CRAP)-'`.
   Byte equivalence runs once at the end (R14) with `tools/bin/refactor-proof OLD NEW [EVIDENCE] [CASE-REGEX]`,
   following the earlier refactor's "proof once" practice. It is not a per-step check.
2. **R2, generic sibling-feature rule**. Add to `tools/bin/architecture-check` a stdlib `go list -json` check
   that no package under `<root>/X` imports `<root>/Y` (X != Y). depguard cannot say "not a sibling" without
   one rule per feature. Add fixtures for an accepted feature-internal import and a refused sibling import,
   and an accepted import by assembly. Today's slices are already isolated, so this lands green.
   Check: `tools/bin/architecture-check`. Revises the ADR-0029 "slices" arrow (via D0).
3. **R3, slice-local declaration for units** (cheapest, least shared). Move `internal/units` to the chosen
   feature root; the slice exports a declaration (name, help line, fresh `*CLI` constructor, consumer-owned
   `Ports` interface it needs). Assembly registers it with `kong.DynamicCommand`, hand-listed this once. Move
   `features/units*.go` steps only if D0 says so. Update `check_test.go` lists, README/go.md entries.
   Check: `go test ./cmd/itos-cc ./features -count=1 -run '^Test(Features|Scenarios)$' -scenarios='^@ID-(UNIT|CLI)-'`
   plus `tools/bin/architecture-check`.
4. **R4, generated assembly**. A stdlib generator (`tools/bin/assemble` or `go run ./tools/...`) scans feature
   roots for the declaration and writes `cmd/itos-cc/commands_gen.go` (sorted imports, DynamicCommand order,
   BindTo of `adapters` to each feature's `Ports`). Add an `itos.yaml` `ci.steps` static step to check it is current.
   Check: the generator's own test (adds a fixture feature and expects it in the output) plus `-scenarios='^@ID-CLI-'`.
5. **R5, dry**: move `internal/dry` + `internal/duplication` into the dry feature. First extract
   `Fingerprint/Jaccard` to `internal/lang` (engine), or copy them into teststructure (person, see section 7). Update
   `domain`/`domain-io`/`coverage-fakes` globs. Check: `-scenarios='^@ID-(DRY|SNAP)-'` both drivers,
   `tools/bin/domain-coverage`, `architecture-check`.
6. **R6, scrap**: move `internal/scrap` + `internal/teststructure`; remove the eager `kong.Bind` at `dispatch.go:73`.
   Check: `-scenarios='^@ID-SCRAP-'`, `domain-coverage`.
7. **R7, settings split**: move `Exclude` out of `mutants.Config` into a selection-owned (or settings-engine)
   value. `config.Reader` gives both, and `exclusionOf` stops going through mutation's model. This is a prerequisite for crap
   and for mutation leaving cleanly. Check: `-scenarios='^@ID-(SEL|CLI)-'` plus `go test ./config ./internal/selection`.
8. **R8, crap**: move `internal/crap` + `internal/changerisk`; graph gets a 4-line `Score` copy or an
   assembly bridge. Move `OwnedExecutor` (or a coverage-command executor) out of `worktrees` into the coverage engine.
   Check: `-scenarios='^@ID-(CRAP|COV|CPLX|SEL)-'`, `cmd/itos-cc -run 'TestCrap|TestCoverage'`.
9. **R9, version/cli**: make `version` a declaration like the others (stays in `internal/cli` or becomes its own
   feature), decide on unused `cli.Completion` and `internal/version`. Check: `-scenarios='^@ID-CLI-'`.
10. **R10a-e, mutation in pieces**: (a) packages `internal/mutation` + `internal/mutants` (+porttest) into
    the mutation feature root, as subpackages that keep domain/port separation; (b) its six adapters;
    (c) mutation codec out of `metricsdir` into a mutation adapter, exceptions codec out of `config`;
    (d) `project/since.go` and `coverage/run.go` mutants uses: invert or move; (e) cmd mutation tests:
    move or assign ownership (section 7). Each check is
    `-scenarios='^@ID-(MUT|SNAP)-'` (or a narrower range) plus the moved packages' `go test`. Revises
    ADR-0037; `.metrics/mutate` paths are unaffected, since none are recorded for `internal/`.
11. **R11, import-resolution engine**: lift `graph.TestsImporting`/`parseDependencies` into an engine
    (lang-side or its own) so mutation's `importgraph` stops importing serve's package. Check:
    `go test ./internal/importgraph ./graph`, `-scenarios='^@ID-MUT-'` subset on reach.
12. **R12, scenario discovery per feature**: replace `Converted`/`InProcess`/`initialize` hand lists
    (`features/steps.go:344,396,661-670`) with generated or declared registration (same generator as R4).
    Optionally co-locate `.feature` files per D0 (itos `tests.scenario.root` walks one folder
    recursively, `itos/internal/tests/gherkin.go:95-111`). Check: `features` `TestFeatureSelection*` plus
    `itos tests smoke check scenario`.
13. **R13, in-process driver split**: let each feature contribute its fakes to the `memory` driver
    (`scenarios_test.go:489-545`) through a declared hook instead of one adapters literal. Check: `TestScenarios` per area.
14. **R14, ownership doc and close**: a path-ownership map per feature plus integration rules (generated files,
    engines, assembly need a coordinator-assigned task), then a check that a fixture "new command" touches only
    its folder plus generated files. Run one `tools/bin/refactor-proof` (pre-T-86 binary against the final
    one, every command's help, errors and exit codes). Then close T-86 and replace its umbrella `done_when`.

serve/graph stays legacy and outside features (deferred work T-102..T-113). Its bridge to mutants and
changerisk lives in assembly or graph, never inside a feature.

## 7. Needs the person's decision

1. **Feature root and naming**: `internal/features/<f>/` (proposal) or keep `internal/<f>/`. The name
   `features` collides with the top-level `features` scenario package. Mutation's internal shape:
   `internal/features/mutation/{cli,domain,port,porttest,adapters/...}` or similar.
2. **Where `.feature` files and step code live**: stay in `features/` (spec central, one harness) or
   per feature. Moving steps needs an exported `world`/`Driver` API from package `features` (shared
   sentences like "it is scored" at `steps.go:640-655`), changes `itos.yaml` `root`, `smoke.yaml`
   paths and `itos-cc.yaml` `exclude`.
3. **Generator**: a stdlib tool in `tools/`, its CI freshness step, and whether `go generate` is acceptable.
   Or a hand-assembled list in cmd, which T-86 rules out for ordinary commands.
4. **Ports by method set**: today's `adapters` struct exposes fields. Consumer-owned `Ports` interfaces need
   methods (`func (a adapters) Tree() selport.Tree`) or one `BindTo` per port type. Which one?
5. **`Fingerprint/Jaccard`**: into `internal/lang` (engine) or a small copy in teststructure.
6. **`changerisk.Score` and mutants judgment in graph**: copy or bridge (proposal's Option 2 test case).
7. **cmd mutation integration tests (~11k lines)**: they are `package main` tests calling `run`. Either they stay
   in cmd as assembly tests owned by mutation by path, or they move behind an exported test entry point.
8. **Task path ownership**: the ledger and itos have no per-task path field today (`itos.yaml` has only
   `scopes`). Either a convention in docs or an itos feature. If the latter, file an issue on donvargax/itos
   (feedback rule).
9. **Unused `internal/version` vs `main.version`** stamping mismatch (`.goreleaser.yaml:28`, `main.go:21`)
   and unused `cli.Completion`: remove or wire them. Outside T-86's refactor, but they touch its packages.

## 8. Where the moves put each package

Each row is a path this survey names and where its move put it, with the task and the commit that
moved it, so git follows the files as renames from there; a package's `.metrics/mutate` snapshots
moved with it (ADR-0037). The packages section 1 lists and this table leaves out are where they
were.

| Old path | New path | Task | Move commit |
|---|---|---|---|
| `internal/units` | `internal/slices/units` | T-145 | `35e8ebc` |
| `internal/duplication`'s `Fingerprint` and `Jaccard` | `internal/lang` (`fingerprint.go`) | T-148 | `7e5a123` |
| `internal/dry` | `internal/slices/dry` | T-148 | `4a178c7` |
| `internal/duplication` | `internal/slices/dry/domain` | T-148 | `4a178c7` |
| `internal/scrap` | `internal/slices/scrap` | T-150 | `e5c7de6` |
| `internal/teststructure` | `internal/slices/scrap/domain` | T-150 | `e5c7de6` |
| `internal/worktrees`' process ownership (`OwnedExecutor`, the command runner, `proc_*.go`) | `internal/processes` | T-151 | `3d7fa57` |
| `internal/crap` | `internal/slices/crap` | T-152 | `5fbd27b` |
| `internal/changerisk` | `internal/slices/crap/domain` | T-152 | `5fbd27b` |
| `internal/cli`'s `version.go`, the `version` command | `internal/slices/version` | T-155 | `ddc6c18` |
| `internal/mutation` | `internal/slices/mutation` | T-157 | `6ceb923` |
| `internal/mutants` | `internal/slices/mutation/domain` | T-158 | `1f3c18f` |
| `internal/mutants/porttest` | `internal/slices/mutation/domain/porttest` | T-158 | `1f3c18f` |
| `internal/testtools` | `internal/slices/mutation/adapters/testtools` | T-159 | `3ce51a8` |
| `internal/testinputs` | `internal/slices/mutation/adapters/testinputs` | T-159 | `3ce51a8` |
| `internal/worktrees` (its workers) | `internal/slices/mutation/adapters/worktrees` | T-160 | `2f7d4aa` |
| `internal/githistory` | `internal/slices/mutation/adapters/githistory` | T-160 | `2f7d4aa` |
| `internal/importgraph` | `internal/slices/mutation/adapters/importgraph` | T-160 | `2f7d4aa` |
| `internal/freshenv` | `internal/slices/mutation/adapters/freshenv` | T-160 | `2f7d4aa` |
| `internal/metricsdir`'s mutation codec (`Mutations`, `mutations.go`, `mutation_codec.go`) | `internal/slices/mutation/adapters/snapshots` | T-161 | `ba727b6` |
| `config/exceptions.go`, `config/exception_document.go` | `internal/slices/mutation/adapters/exceptions` | T-161 | `ba727b6` |
| `internal/workdir`'s `SourceEdits` (`source_edits.go`) | `internal/slices/mutation/adapters/sources` | T-161 | `ba727b6` |
| `graph/tests.go` (`TestsImporting`), `graph/tsconfig.go` | `internal/imports` (`reach.go`, `tsconfig.go`) | T-162 | `98e3788` |

Section 6's steps and the work that did each: D0 is ADR-0051 (asked as q-72). R1 is T-143, R2
T-144, R3 T-145, R4 T-146, R5 T-148, R6 T-150, R7 T-147, R8 T-151 and T-152, R9 T-155, R10
T-156 to T-161 ((a) T-157 and T-158, (b) T-159 and T-160, (c) T-161, (d) T-156; ADR-0051 settled
(e): cmd's mutation tests stay in cmd as assembly tests mutation owns by path), R11 T-162 and
T-163, R12 T-164, R13 T-165, and R14 slices-close's tasks, T-167 to T-172.
