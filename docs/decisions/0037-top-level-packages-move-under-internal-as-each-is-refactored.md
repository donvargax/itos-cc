---
status: accepted
date: 2026-10-11
---

# Top-level packages move under internal/ as each is refactored

## Context and Problem Statement

Should the top-level packages (mutate, coverage, project, lang, crap, dry, …) move under internal/ as each becomes a domain or an adapter in the template-design refactor?

Asked as q-50.

## Considered Options

The options are those the question names.

## Decision Outcome

Move them, each as it is refactored into a domain or an adapter (answered 2026-10-11). The Go API is not part of the contract; .metrics/mutate paths and exceptions move with them, an exception following a moved file once as ADR-0014 allows.

### Consequences

A package moves in the slice that makes it a domain or an adapter, never in bulk; its .metrics/mutate snapshots and itos-cc.yaml exceptions move with it, an exception following a moved file once as ADR-0014 allows. The Go API is not part of itos-cc's contract.
