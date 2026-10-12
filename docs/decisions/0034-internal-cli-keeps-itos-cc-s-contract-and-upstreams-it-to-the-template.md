---
status: accepted
date: 2026-10-11
---

# internal/cli keeps itos-cc's contract and upstreams it to the template

## Context and Problem Statement

How should internal/cli's contract differ from go-template-itos's sample, given template updates must land cleanly: rule names, exit precedence, the problem's shape, flag errors, and the UI importing a domain's port?

Asked as q-54.

## Considered Options

The options are those the question names.

## Decision Outcome

Keep ours and upstream it to go-template-itos with exact diffs, as for release-version (answered 2026-10-11): dotted lower-case rule IDs; an explicit, documented exit precedence with the interrupt first (130, then usage, internal, environment, retry, the check's no), not the highest code; problems keep subject keys as flat extension members as RFC 9457 has them, with rule, message, fix and exit reserved and a per-problem exit added; the plain line keeps the fix; flag errors keep typed rules and our wording; cmd hands internal/cli one classifier per domain so it imports none. On Unix an interrupted command, once it has reported, re-raises SIGINT with the default action so shells stop their loops; Windows exits 130.

### Consequences

Template updates bring these differences back as conflicts until go-template-itos takes the exact diffs filed on it.
