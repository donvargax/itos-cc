# Who owns a path

Which paths each command's slice owns, and which are shared, by rule rather than by list, so a
new command adds nothing here (ADR-0051: path ownership is a docs convention first). It is the
brief for implementing agents working in parallel: a slice task edits only its slice's paths;
every shared path changes in a task of its own that the coordinator assigns, and the generated
files are nobody's. Each command is a slice under `internal/slices` over shared engines, as
`docs/go.md` and `docs/architecture.md` describe.

## What a slice owns

A slice `<x>`, `internal/slices/<x>`, owns:

- `internal/slices/<x>/**`: its CLI and application code, its domain, ports, porttests and
  adapters, and their tests.
- `.metrics/mutate/internal/slices/<x>/**`, the mutation snapshots of its sources. Any snapshot
  under `.metrics/mutate/` is its source's owner's: `.metrics/mutate/<path>.json` follows
  `<path>`.
- `features/<x>.feature`, the Go file in `features/` whose `featureFile` declaration names it,
  and the helper files named for it, those beginning with that Go file's name and `_`
  (`features/dry_report.go`, `features/units_routes.go`).
- Its entry in `features/smoke.yaml`.
- Its section under README's Use (`### <x>`).
- Its cells in `docs/CLI.md`'s tables: a row or cell naming its command alone. One naming
  several commands, or `every`, is shared.
- Its bullets in `docs/go.md`'s Packages, those naming `internal/slices/<x>` or a subpackage of
  it.

`units`, `dry` and `scrap` own what the rule gives them and nothing more.

## Exceptions by name

- `mutation`'s feature is named for the old command: it owns `features/mutate.feature` and
  `features/mutate*.go`, and `features/metrics_snapshots.feature` with its steps,
  `features/metrics_snapshot*.go`. It owns cmd's mutation integration tests too, which stay in
  `cmd/itos-cc` as assembly tests it owns by path (ADR-0051): `mutate_*_test.go`,
  `mutation_*_test.go`, `strict_*_test.go`, `except_renew_test.go`, `exclude_test.go`,
  `orphans_test.go`, `uncovered_*_test.go` and `whole_suite_*_test.go`;
  `language_smoke_test.go` with its fixtures under `testdata/smoke/`; `marker_lock_*_test.go`,
  which only `mutate_workers_abandoned_test.go` uses; `testdata/mutation-help.json`; and
  `mutate_counted.go`, the counted run's roots. Of the in-process driver's files it owns those
  that serve only its fakes: `scenarios_counted_test.go` (`Fresh`, `FreshWorkers` and the
  counted bindings), `scenarios_exceptions_test.go` (`Exceptions`),
  `scenarios_listed_setup_test.go` (listed tests on `Workers`), `scenarios_settings_test.go`
  (mutation's `SettingsReader` and `SourceEdits`) and `scenarios_workers_test.go`. The other
  `scenarios_*_test.go` are assembly's: they serve every command's ports or the driver itself.
  Its settings adapter, `internal/slices/mutation/adapters/settings`, the top-level `config`
  until T-197, is its by the rule above, though cmd gives its `Reader` to every slice:
  `itos-cc.yaml`'s reading codec changes in a mutation task.
- `crap` owns `features/crap.feature` and its steps, `features/crap*.go`, and cmd's crap tests,
  `cmd/itos-cc/crap_*_test.go`. `complexity.feature`, `coverage.feature` and
  `file_selection.feature`, which crap's command drives, are the specs of the lang, coverage
  and selection engines, with their steps (`complexity.go`, `coverage*.go`, `selection*.go`).
- `graph`'s feature is named for what it writes: it owns `features/graph_command.feature` and
  its steps, `features/graph_command*.go`. It owns the binding of its Mutation port to the
  mutation slice's judgment, `cmd/itos-cc/graph_mutation.go`, and cmd's graph tests,
  `cmd/itos-cc/graph_*_test.go`, which import that slice's adapters as no slice may (T-208):
  among them `graph_stale_test.go` and `graph_lookup_test.go` hold the graph to mutation
  check's verdict (GRAPH-34 to GRAPH-40), so a mutation task changing check's rule changes
  them too. `mutation`'s `Graph`, in its domain, is mutation's.
- `version` has no feature file: its rows are in `cli.feature`, which with `completion.feature`
  and their steps (`cli*.go`) belongs to `internal/cli`.

## Shared paths

Each change to one is a task the coordinator assigns.

- **Engines**: `internal/selection`, `internal/lang`, `internal/snapshot` and `internal/coverage`
  with their ports and porttests; `internal/processes`, `internal/imports`, `internal/project`,
  `internal/cli`, `internal/version`, `internal/release`, `internal/workdir`,
  `internal/metricsdir`, `internal/coveragetools` and `internal/covrun`.
  Their feature files, above, are theirs.
- **Assembly**: `cmd/itos-cc`'s hand-written `main.go`, `dispatch.go` and `adapters.go` with
  their tests, `diskAdapters` and `inProcessAdapters` (decision 53); `scenarios_test.go`,
  `scenarios_fakes_test.go`, `run_fakes_test.go`, `adapters_test.go` and the other
  `scenarios_*_test.go` above; the classifiers in `common.go`, `coverage.go`, `selection.go` and
  `snapshot.go` with their tests; `raise_*.go`; and cmd's shared test harness,
  `subprocess_test.go` and `language_tools_test.go`, and `cli_test.go`. Every file in
  `cmd/itos-cc` that no rule here gives to a slice is assembly's.
- **The harness**: `features/steps.go`, `steps_output.go`, `steps_tags.go`, `binary*.go`,
  `build.go`, `deferred.go`, `features_test.go`, `features/README.md` and every other file in
  `features/` not named for a feature file.
- **Rules and tooling**: `.golangci.yml`, `tools/**`, `itos.yaml`, `itos-cc.yaml`,
  `itos-policy.yaml`, `.github/**`, `go.mod`, `go.sum`, `.goreleaser.yaml`,
  `.itos-template.yaml`, `.gitattributes`, `.gitignore`, `testdata/**` (the pinned test tools),
  `AGENTS.md`, `CLAUDE.md`, `.claude/**`, `CONTRIBUTORS.md` and `LICENSE`.
- **Docs**: `docs/go.md`'s shared sections, every part but the slices' bullets;
  `docs/architecture.md`; README's layout table and every part of it but a slice's section
  under Use; `docs/CLI.md` but a slice's cells; every other file under `docs/`, this one
  included; and `IDEAS.md`.
- **Legacy serve**: `cmd/itos-cc/serve.go` and `cli.go`, `server/`, `viewer/`,
  `serve.feature`, `viewer.feature` and `architecture_graph.feature`, their snapshots, and
  serve's deferred work (T-102 to T-113). The graph serve publishes is the graph slice's
  (T-208).
- **Orphaned snapshots**: a snapshot under `.metrics/mutate/` whose source is gone, today
  `.metrics/mutate/mutate/*` and `.metrics/mutate/cmd/itos-cc/`'s `mutate.go.json`,
  `shell.go.json` and `units.go.json`. A mutation run over the repository removes them; nobody
  edits one by hand.
- **itos's records**: `tasks/**` and `docs/decisions/**`, written only through itos (`itos work`,
  `itos task`, `itos decision record`).

## Generated files

`cmd/itos-cc/commands_gen.go` and `features/steps_gen.go` are any task's to regenerate and
nobody's to edit or merge by hand: on a conflict take either side and rerun
`go run ./tools/bin/assemble` on the merged tree, then commit what it writes.
`features/smoke.yaml` keeps one entry per feature file, each its owner's; a conflict keeps both.

## The integration rule

A slice task that needs a `Ports` method `adapters` lacks, a new port, a rule edit, an engine
change or another slice's path stops and records an idea (`itos work add`) instead of editing a
shared path, and the coordinator assigns it as a task of its own.

## What holds the map

- The new-command test, `TestANewCommandTouchesOnlyItsSliceAndTheGeneratedFiles`
  (`tools/testdata/architecture`, run by `tools/bin/architecture-check`): an ordinary command
  needs its slice, its feature file and steps, its smoke entry and the two generated files, and
  no edit elsewhere; a new port fails `go build` until `adapters` answers it.
- The outside-import check, `tools/bin/architecture-check`'s outside: no package outside
  `internal/slices` imports one under it but `cmd` (T-167; config's and graph's rows went with
  T-197 and T-208). Its sibling check keeps a slice from importing another.
- `TestEveryPortIsBoundOnDiskAndInProcess` (`cmd/itos-cc/adapters_test.go`): every `adapters`
  field is bound in `diskAdapters` and `inProcessAdapters`, so a new port is assembly's edit in
  all three (decision 53).

Nothing checks the rest, the docs' sections and cmd's test names among it: it is a convention,
and an itos feature only if the parallel-agents trial after T-86 shows agents straying
(ADR-0051).
