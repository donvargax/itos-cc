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

## Unverified

- Kotlin coverage and mutate commands have never run here (no Gradle on
  this machine); parsing and JaCoCo reading are tested.
- Python projects installed in editable mode: mutants should run against
  the worker copy (`PYTHONPATH` is prepended), but this was not tried on a
  real editable install.
- scrap's thresholds were tuned on two codebases (Bob's and this one).

## Release and packaging

- **CI releases.** Linux (static, amd64/arm64) and Windows cross-compile
  from Linux with `zig cc`; macOS needs a macOS runner (Go's macOS link asks
  for `libresolv`, which zig does not ship).
- **Name.** `itos-cc` is a placeholder module path.
- **itos integration.** Use the tools through itos as the agents' gateway,
  either as subprocesses (rtk-style) or by importing the Go packages
  directly.
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
