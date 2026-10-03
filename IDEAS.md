# Ideas

Things we noted but did not build. Rough, unordered within each group.

## Architecture viewer

- **Multi-repo canvas.** Repositories (later: services, plugins) as the top
  level, drilling down to functions. The graph already has a repo level and
  `serve` takes several roots; what is missing is better cross-repo edges.
- **Resolve HTTP hosts.** Requests rarely name their host in source. Read
  env vars and config (`.env`, docker-compose, Kubernetes manifests, Helm
  values) to learn which base URL points at which service, so a call to
  `/health` links to the right one instead of to every service serving it.
- **Backstage.** Read `catalog-info.yaml` (`dependsOn`, `providesApis`,
  `consumesApis`) where a project has it, as a declared source of service
  dependencies next to what the code shows.
- **More cross-service edges.** OpenAPI specs, `.proto` / gRPC services,
  message queues and topics (Kafka, SQS, RabbitMQ), GraphQL.
- **Route prefixes mounted elsewhere.** Express routers mounted at a path,
  FastAPI `APIRouter(prefix=…)`, Flask blueprints with `url_prefix`, Go
  sub-routers. Only Spring class mappings and Ktor `route()` are handled.
- **Ask and change from the canvas.** Select a box and send a question or a
  change request to an agent with the box as context, through itos rather
  than one agent's CLI (uml-viewer uses Grok in tmux).
- **Layers and the Dependency Rule.** Configure levels and mark dependencies
  that point outward, like uml-viewer's `:levels`.
- **Proposals.** Draw a grouping that is not in the code yet, compare it
  with the real one, then ask an agent to make the code match.
- **Test code on the canvas.** Show scrap results and which tests cover a
  module.
- **Move ELK into a Web Worker.** It is most of the 1.8 MB bundle and runs on
  the main thread.
- **Tests for the React components.** The level logic is tested; the
  components are not (scrap and crap both flag `Canvas`).
- **`serve` should ignore a path given twice** (`serve . .` draws the repo
  twice).

## Tools

- **Per-test coverage for mutate.** Run only the tests that reach the mutated
  function instead of the file's whole related suite.
- **Mutation grade weighting.** A module whose few mutated functions all
  pass still looks perfect; weigh the grade by how much was mutated.
- **More languages.** Java and Rust are one file in `lang/` each, plus
  scrap rules and a coverage plan. Bob's tools also cover Clojure.
- **kotest string specs.** `"name" { … }` examples are not recognized yet;
  `test("…")`, `it`, `should` are.
- **EDN output** for Bob's uml-viewer, if anyone wants to feed it our
  metrics.

## Gating and debt

The tools measure, but nothing stops a commit from making code worse where
debt already exists: `crap --threshold` and `mutate`'s survivor exit are
all-or-nothing. Planned, in this order:

- **Killed mutants must notice test changes.** `mutate` keeps a killed
  mutant while its function's hash is unchanged (`remembered` in
  `mutate/snapshot.go`), so deleting the test that killed it changes
  nothing. Key the result by the function's hash and the hashes of the test
  files that import its module; the graph knows them.
- **A debt file and `check`.** `itos-cc-debt.yaml`, at the project root
  and committed, holds limits per function, by ID:
  `crap:<file>#<namespace.name>` with a `max`, `mutation:…` with a `min`.
  It names no owners; owning is the host's job (below). It stays out of
  `.metrics/`, which holds caches every run rewrites and a project may
  ignore or wipe. Only `debt adopt` (measures, writes the first entries)
  and `debt tighten` (lowers limits after an improvement, say nightly)
  write it. `itos-cc check` measures and fails when a function is worse
  than its entry, or than the threshold when it has none, and when an
  entry's function is fixed or gone; it never writes the file.
- **The range check.** `check --range <from>..<to>` reads the debt file at
  each commit and refuses an added entry or a looser limit, except in a
  commit that changes nothing else, so debt grows only on purpose. A function
  with an unchanged hash under a new ID (its file moved) is a move, not new
  debt. `check --staged` does the same for a commit hook.
- **`debt list --at <tree> --json`**: the entry IDs at a tree, each with a
  title and file:line, read from the committed file through git
  (`git show <tree>:itos-cc-debt.yaml`): no checkout, no coverage, no
  tests. No file means no debt. Measuring stays in `check`, so the list can
  lag behind a fix until `check` asks for the stale entry's removal; a host
  then waits on debt already paid, the safe way to be wrong.
- **Exit codes and JSON as itos extensions use them.** 0 pass, 1 policy
  failure (a threshold, a survivor), 2 usage, 3 environment (a baseline that
  fails). Today usage is 1, threshold and baseline 2, survivor 3. `--json`
  prints one object with `"schema": 1`, and each problem carries a sentence
  and a rule ID (`cc/mutant-survived`, `cc/crap-above-limit`,
  `cc/baseline-failed`). Detail goes in rule IDs, which any extension can
  add; exit codes only say what the caller should do.
- **One finding shape and SARIF**, once there is a gate:
  `{tool, rule, file, symbol, range, value, introduced}`, where `introduced`
  means new against the base commit. SARIF for GitHub annotations.
- **Architecture boundaries.** Zones as globs and rules for which zone may
  import which, checked over the graph's imports, with violations drawn on
  the canvas. The same feature as "Layers and the Dependency Rule" above.
- **Later.** Policy packs as data (banned calls per zone, read like the HTTP
  calls in `lang/http.go`); suppression comments that name one rule and a
  reason, read with tree-sitter, never a regex; reading other tools' reports
  (Stryker's mutation report) next to ours rather than replacing ours;
  dead-code adapters (vulture, Go's `deadcode`).

### With itos

The two share IDs, never formats. itos-cc lists its debt by its own IDs and
never sees a work item; itos claims those IDs as opaque strings and never
reads `.metrics/`. Only itos's config wires them together.

- **A `debt` role in itos**, shaped like its test kinds: each source names a
  `list` command (`itos cc debt list`; itos adds `--at <tree>`) and range
  checks (`staged`, and `range` with `{from}` and `{to}`). Role first, then
  source: `debt.cc`, a name the project picks and claims start with. By
  convention, not enforced, it is the extension's name (`cc`), or the
  tool's for a source that is not an extension, as godog is not one for
  tests. Renaming it means rewriting every claim.
- **Work items claim entries** in itos's registry
  (`pays: ["cc:crap:billing/*"]`). A claim is an exact ID or a prefix
  ending in `*`, never a glob, so itos imposes no syntax on IDs beyond
  running from general to specific. itos refuses an unclaimed entry, an
  item marked done while it still claims one, and a claim that matches
  nothing, so an ID format change fails loudly.
- **Entry IDs are part of itos-cc's public format.** Changing them bumps
  `"schema"`. They are paths from the repository root with no repository
  name: each repository has its own registry, so a claim never reaches
  another one.
- **Measuring runs as late CI steps** (`itos cc check`), never in the hook.
- **Extensions fill roles; they do not invent them.** A role is a protocol
  itos enforces rules over. Anything else an extension wants checked is a
  plain command in `ci.steps` or a task's checks.
- **Open:** what the hook does when `itos-cc` is not installed; pinning
  extensions as itos pins itself; a prefix claim lets new debt join an item
  quietly, so `itos work list` should show how many entries each item
  claims.

## Unverified

- Kotlin coverage and mutate commands have never run here (no Gradle on
  this machine); parsing and JaCoCo reading are tested.
- Python projects installed in editable mode: mutants should run against
  the worker copy (`PYTHONPATH` is prepended), but this was not tried on a
  real editable install.
- scrap's thresholds were tuned on two codebases (Bob's and this one).

## Release and packaging

- **Name.** `itos-cc` is a working name; the module is
  `github.com/donvargax/itos-cc`, so a rename moves the module path too.
- **itos integration.** As an itos extension: `itos cc …` runs `itos-cc …`
  from the `PATH`, a subprocess, not imported Go packages, so neither side
  depends on the other's code. See "With itos" above.
- **Licensing.** The ideas come from Robert C. Martin's crapper, mutator,
  dryer, scrap, and uml-viewer, which have no license file. This is a
  separate implementation; credit him, and ask him to add a license if we
  ever reuse his code directly.
- **Kotlin grammar** is fwcd's, pinned to a commit because it has no tagged
  release with Go bindings. The tree-sitter-grammars one drops classes with
  several annotations and no constructor.

## Notes

- pytest is slow (about 3 s per run) with a system Python that has many
  pytest plugins installed; a project `.venv` avoids it.
- This repository ignores its own `.metrics/`, though the README tells
  projects to commit theirs.
