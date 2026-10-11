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
  `Changed` files, whether a path `Exists`, each taking the context first) and its sealed
  set of failures: `NoGit`, `Unmatched`, carrying the fragment, `Unwalkable` and `Cancelled`.
  `internal/selection/port/porttest` holds the in-memory fake of `Tree`.
- `internal/disk` and `internal/stdin` are the adapters. `disk` reads real files and translates
  OS errors into the port's failures; `stdin` reads standard input for `-`, stops with
  `Cancelled` once the context is done, and hands every other path to a fallback it is given.
  Each imports nothing of ours but the port.
- `internal/workdir` is the selection port's adapter: the working directory as the disk and git
  hold it. It wraps the top-level `project` library's discovery and git query and stats paths,
  translating their errors into the port's failures, and imports nothing else of ours.
- `internal/count` is the `count` command's slice, named for the command. Its kong struct and
  `Run` are the UI; its `Query`, carrying domain values, and `Handler.Handle(ctx, q)`, which
  only calls the domain through the port, are the application. `Run` assembling the `Query`
  from kong's flags is the one copy the layout accepts. Its `Classify` is the counting
  domain's classifier, turning the port's sealed failures into problems.
- `internal/cli` is the UI every slice shares, and imports nothing of ours. It alone turns a
  run's problems into its exit code, by the precedence `docs/CLI.md` gives, and prints the
  plain or JSON answer, and it stops a command on Ctrl-C (decision 31). A domain's failures
  reach it through that domain's classifier, which cmd hands it.
- `cmd/itos-cc` assembles everything: it makes the context, feeds the interrupts to
  `internal/cli`, hands it each domain's classifier, gives `stdin` the disk as its fallback,
  and hands the context and the handler to `Run` through kong's bindings. It is the only place
  that knows every package. Until itos-cc's commands move into slices, it also holds the
  selection domain's classifier, `classifySelection`, since every command selects its files,
  and `files`, which calls the domain through `workdir`.
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
- `slices`: a slice imports nothing of ours but the domain and `internal/cli`, so never another
  slice, an adapter or `cmd`. Its files are every package under `internal/` except the ones the
  rule lists as something else, so a new command's slice is held with no edit. A slice's tests
  are in its own package, since the rule cannot tell a package's import of itself from another's.
- `ui`: `internal/cli` and its tests import nothing of ours, so adding or removing a domain
  touches no file of the UI (q-54).
- `infra` and `workdir`: an adapter imports nothing of ours but the port it implements, and
  `internal/workdir` the `project` library it wraps; only its tests may use `porttest`. Each port's
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
  counting's, `workdir` for selection's), or give a new port's adapters a rule of their own
  allowing that port and any library they wrap; add a `!**/internal/<name>/**` exclusion to
  `slices`. Wire it in `cmd`.
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
