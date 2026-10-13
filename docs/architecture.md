# Architecture

The code is a shared domain behind ports, adapters apart from it, and thin slices named for
their commands (decision 29 in `docs/decisions/`). `docs/go.md` lists the packages and the
import rules that hold them.

The private counting domain, `internal/counting`, owns line and word semantics and its result.
It reads through its `Files` port, whose sealed failures describe the input it needs without
exposing an OS's own errors. The `internal/disk` and `internal/stdin` adapters implement it; an
in-memory fake holds the domain's test inputs and failures. Unit tests assert outcomes, and the
rapid property checks addition for newline-terminated pieces.

The selection domain, `internal/selection`, owns how every command chooses its files: paths,
fragments of a path, `--changed` and the whole working directory when none is given. It reaches
the working tree through its `Tree` port, which also reads the files a command selected, and
whose sealed failures say there is no git, a fragment matched nothing, a walk or a read failed
or the context was done. The `internal/workdir` adapter
implements it over the `project` library and the disk; cmd's `classifySelection` turns its
failures into the problems the command line has always reported.

The snapshot domain, `internal/snapshot`, owns the snapshots under `.metrics/` that the commands
share: their format version, and naming a file from the project root, so a snapshot is the same
from any directory a command runs in. It reads and writes them through its `Store` port, whose
sealed failures say a snapshot could not be read or written, with the library's own message, or
the context was done. The `internal/metricsdir` adapter implements it over the `metrics` and
`project` libraries, which keep every byte and the atomic rename; cmd's `classifySnapshot` turns
its failures into problems.

The duplication domain, `internal/duplication`, finds functions similar enough to be
duplicates. It takes files `internal/lang` has parsed, with the names its caller gives them, and
reads nothing, so it has no port; the `dry` slice, `internal/dry`, parses each source with the
selection domain's `Parse`, which reads it through the selection `Tree` and parses it with
`internal/lang`, and records what was found through the snapshot `Store`.

The test-structure domain, `internal/teststructure`, measures test code: which examples are too
large, branch, mock heavily, assert too little or too much, or repeat each other, and what an
assistant may do about each file. It takes files `internal/lang` has parsed, with the names its
caller gives them, and merges a run's reports into the last snapshot's; it reads nothing, so it
has no port. The `scrap` slice, `internal/scrap`, parses each test file with the selection
domain's `Parse`, as `dry` does, and reads and records its snapshot through the snapshot `Store`.

The coverage domain, `internal/coverage`, owns coverage reports and what they say: it parses
LCOV, Go cover profiles and JaCoCo XML, matches the paths a report names to the sources, and
answers how much of a function the tests ran and which listed tests reach a line. It reads
nothing: what a path a report names is on disk it asks its `Files` port, and an `Unmeasured`
build root or report carries one of the port's sealed gaps as its cause. The top-level
`coverage` runner, which runs each language's tools, is its adapter: it builds every `Report`
through the domain's API and answers the port from the disk (`Disk`).

The change-risk domain, `internal/changerisk`, scores each function by CRAP, its complexity
weighed by how little of it the tests ran. It takes files `internal/lang` has parsed, with the
names its caller gives them, and a coverage `Report`, and reads nothing, so it has no port. cmd
reads and parses each source for the hand-parsed `crap` through the selection domain's `Parse`
over `internal/workdir`.

The `units` slice, `internal/units`, is the first of itos-cc's commands on kong: it selects
through the domain and parses each file with its `Parse`, read through the same port and named
by `internal/lang`, the measure every command shares and a domain of its own. It keeps every
language's rules for naming a file but reads nothing: it asks its `Surroundings` port,
`internal/lang/port`, whether a path is there, what a file holds, the working
directory and a path made absolute, and `internal/workdir` answers it from the disk through
`project`, as it answers the selection port.
cmd routes it through kong, and the commands not moved yet through its own parser.

The domains know no command-line flags, JSON or exit codes. The `count` slice, `internal/count`,
carries the command's domain values to the domain through the port and counts nothing itself;
its classifier turns the port's sealed failures into problems. `internal/cli`, the UI every
command shares, itos-cc's own included, imports no domain: it holds the problem, its rule,
message, fix and subject keys, prints them as the JSON object or as text lines, and picks the
run's exit code from its problems by the precedence `docs/CLI.md` gives, the first present
rather than the highest; an error that neither is a problem nor any classifier claims is an
internal one, exit 70. A second command is a new slice beside `count` that reuses the domain,
never `count`'s package. `cmd/itos-cc` assembles them, handing the UI each domain's classifier
and making the context that a Ctrl-C cancels (decisions 30 and 31). `features/` holds the public behaviour as scenarios, which godog
runs against the binary the harness builds for the platform it is on.

Line separators are LF, with CRLF counted once; a nonempty final unterminated line counts.
Words follow the standard-library Unicode-whitespace rule. No encoding detection or linguistic
word segmentation is performed. File data is not executed or changed.
