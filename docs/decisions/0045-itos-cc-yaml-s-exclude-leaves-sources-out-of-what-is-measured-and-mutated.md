---
status: accepted
date: 2026-10-12
---

# itos-cc.yaml's exclude leaves sources out of what is measured and mutated

## Context and Problem Statement

Issue #16 (mutation-exclude): test support outside _test.go files (porttest fakes, soon the godog step code in features/) is measured and mutated as product code. Recommend a top-level exclude: list of globs in itos-cc.yaml that leaves sources (never tests) out of crap, dry, units and mutation run, list, check, sample and --count; doublestar/v4 globs from the project root; exclusion always wins, a path or --changed naming an excluded file selecting nothing; snapshots of newly excluded files become orphans (why excluded), exceptions naming one go stale, mutation except on one is refused (exception.excluded). Alternatives: per-command keys; excluding tests too; our own ** matcher; a named file overriding exclusion; leaving old records in place.

Asked as q-61, about mutation-exclude.

## Considered Options

- A top-level exclude of sources, exclusion always wins, old records orphaned
- Per-command exclude keys
- Excluding tests as well as sources
- A file named on the command line overriding exclusion

## Decision Outcome

As recommended (the person, 2026-10-12), all four points.

### Consequences

itos-cc.yaml gains exclude:, read at the edge and handed to the selection domain as an Exclusion value; crap, dry and units read itos-cc.yaml and fail config.invalid when it is broken; the next mutation run removes snapshots of excluded sources. The two-drivers work (decision 44) can then make the step code ordinary files of package features, which this repository's own exclude leaves out with the porttest fakes. An older itos-cc ignores the key.
