---
status: accepted
date: 2026-10-07
---

# Project settings live in itos-cc.yaml at the project root

## Context and Problem Statement

Where do the exceptions live? (a, recommended) itos-cc.yaml at the project root, under mutation.exceptions: itos-cc's first project setting (docs/CLI.md rule 38), reviewed like code. (b) .metrics/mutate/exceptions.yaml beside the snapshots, as the issue proposed, though .metrics holds what itos-cc writes.

Asked as q-9, about mutation-exceptions.

## Considered Options

The options are those the question names.

## Decision Outcome

(a) itos-cc.yaml at the project root, under mutation.exceptions.

### Consequences

itos-cc.yaml is itos-cc's settings file under version control (docs/CLI.md rule 38), reviewed like code; mutation.exceptions is its first key. .metrics/ stays for what itos-cc writes. Since q-16 it is found at the git top level, as .metrics/ is.
