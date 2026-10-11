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
the working tree through its `Tree` port, whose sealed failures say there is no git, a fragment
matched nothing, a walk failed or the context was done. The `internal/workdir` adapter
implements it over the `project` library and the disk; cmd's `classifySelection` turns its
failures into the problems the command line has always reported.

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
