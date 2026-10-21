# Architecture

The code is shared domains behind ports, adapters apart from them, and thin slices named for
their commands (decision 29 in `docs/decisions/`). `docs/go.md` lists the packages and the
import rules that hold them. A port's sealed failures describe what went wrong without exposing
an OS's own errors, and an in-memory fake in its `porttest` stands in for its adapters in the
tests, both passing one contract suite (decision 44).

The selection domain, `internal/selection`, owns how every command chooses its files: paths,
fragments of a path, `--changed` and the whole working directory when none is given. It reaches
the working tree through its `Tree` port, which also reads the files a command selected, and
whose sealed failures say there is no git, a fragment matched nothing, a walk or a read failed
or the context was done. The `internal/workdir` adapter
implements it over the `project` library and the disk; cmd's `classifySelection` turns its
failures into the problems the command line has always reported.

The snapshot domain, `internal/snapshot`, owns the snapshots under `.metrics/` that the commands
share: their format version, and naming a file from the project root, so a snapshot is the same
from any directory a command runs in. It reads, writes, removes and lists them through its
`Store` port, whose sealed failures say a snapshot could not be read, written or removed, or a
folder of them listed, with the library's own message, or the context was done. The
`internal/metricsdir` adapter implements it over the `metrics` and `project` libraries, which
keep every byte and the atomic rename, at the working directory's project root or at a root it
is given; cmd's `classifySnapshot` turns its failures into problems. mutation reads and writes
its snapshots only through the port.

The `dry` slice, `internal/slices/dry`, the second under `internal/slices` (ADR-0051), finds
functions similar enough to be duplicates through its own domain, `internal/slices/dry/domain`,
which takes files `internal/lang` has parsed, with the names its caller gives them, and reads
nothing, so it has no port. It compares them by `internal/lang`'s fingerprints of a unit's
structure (`Fingerprint`, `Jaccard`), which scrap's domain compares a test's examples by too. The
slice parses each source with the selection domain's `Parse`, which reads it through the
selection `Tree` and parses it with `internal/lang`, and records what was found through the
snapshot `Store`, declaring its command and the ports it needs for cmd to assemble, as `units`
does.

The `scrap` slice, `internal/slices/scrap`, the third under `internal/slices`, measures test
code through its own domain, `internal/slices/scrap/domain`: which examples are too large,
branch, mock heavily, assert too little or too much, or repeat each other, and what an assistant
may do about each file. The domain takes files `internal/lang` has parsed, with the names its
caller gives them, and merges a run's reports into the last snapshot's; it reads nothing, so it
has no port. The slice parses each test file with the selection domain's `Parse`, as `dry` does,
and reads and records its snapshot through the snapshot `Store`, declaring its command and its
ports as `dry` does; it selects only tests, which `itos-cc.yaml`'s `exclude` never leaves out,
so it reads no settings.

The coverage domain, `internal/coverage`, owns coverage reports and what they say: it parses
LCOV, Go cover profiles and JaCoCo XML, matches the paths a report names to the sources, and
answers how much of a function the tests ran and which listed tests reach a line. It reads
nothing: what a path a report names is on disk it asks its `Files` port, and an `Unmeasured`
build root or report carries one of the port's sealed gaps as its cause. The commands measure
coverage through the port's `Measurer`, which gives a `Report` or a sealed failure for a
request, what crap's coverage flags ask. The `internal/coveragetools` adapter implements it over
the top-level `coverage` runner, which runs each language's tools, builds every `Report` through
the domain's API and answers the `Files` port from the disk (`Disk`); the adapter keeps the run
folders the tools write under `.metrics/coverage`. As decision 47 lets an adapter, it returns the
domain's own `Report`, with no mapper. cmd's `classifyCoverage` turns each gap and failure into
its problem, for crap, which measures through the port, and mutation, which measures through
its handler's bound coverage capability over the same adapter.

The change-risk domain, `internal/changerisk`, scores each function by CRAP, its complexity
weighed by how little of it the tests ran. It takes files `internal/lang` has parsed, with the
names its caller gives them, and a coverage `Report`, and reads nothing, so it has no port. The
`crap` slice, `internal/crap`, selects the sources and parses each through the selection
domain, as `dry` does, measures their coverage through the coverage port's `Measurer`, and
records its scores through the snapshot `Store`.

The `units` slice, `internal/slices/units`, is the first of itos-cc's commands on kong and the
first under `internal/slices` (ADR-0051), declaring its command and the ports it needs for cmd
to assemble: it selects through the domain and parses each file with its `Parse`, read through
the same port and named by `internal/lang`, the measure every command shares and a domain of its own. It keeps every
language's rules for naming a file but reads nothing: it asks its `Surroundings` port,
`internal/lang/port`, whether a path is there, what a file holds, the working
directory and a path made absolute, and `internal/workdir` answers it from the disk through
`project`, as it answers the selection port.

The mutation group's slice, `internal/mutation`, runs its five commands over the mutation domain,
`internal/mutants`, which owns sites, results and their judgment, and reaches its workers, tools,
inputs, history, frozen environments and import reach through adapters of its own (`docs/go.md`).
cmd routes every command through one kong model but `serve`, which keeps its legacy handler.

The domains know no command-line flags, encoding or exit codes: a domain may carry struct tags
used only for serialization on its own types, and imports no encoding package and holds no
encoding or decoding logic (decision 50). A slice carries its command's
domain values to the domains through their ports and measures nothing itself. `internal/cli`,
the UI every command shares, imports no domain: it holds the problem, its rule,
message, fix and subject keys, prints them as the JSON object or as text lines, and picks the
run's exit code from its problems by the precedence `docs/CLI.md` gives, the first present
rather than the highest; an error that neither is a problem nor any classifier claims is an
internal one, exit 100. A new command is a new slice beside the others that reuses the domains,
never another slice's package. `cmd/itos-cc` assembles them, in `commands_gen.go`, which
`tools/bin/assemble` writes from each slice's declaration (ADR-0051), handing the UI each domain's
classifier and making the one context every command runs under (decision 30), which SIGINT or
SIGTERM cancels, its cause `cli.ErrInterrupted`, every command then reporting the interrupt,
exit 130, and on Linux and macOS ending by that signal (decision 38). `features/` holds the public behaviour as scenarios, which
godog runs through two drivers: against the binary the harness builds for the platform it is on,
and in process against the ports' fakes (decision 44, `features/README.md`).
