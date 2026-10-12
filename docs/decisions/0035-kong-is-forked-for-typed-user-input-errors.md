---
status: accepted
date: 2026-10-11
---

# Kong is forked for typed user-input errors

## Context and Problem Statement

Kong reports unknown flags, unexpected arguments and its other user-input failures as plain strings. How do we get typed errors from it?

Asked as q-55.

## Considered Options

The options are those the question names.

## Decision Outcome

Fork it, donvargax/kong, and make kong's user-input failures exported error types matched with errors.As, Error() text unchanged (answered 2026-10-11). itos-cc uses the fork through a replace until it is proven, then a PR goes to alecthomas/kong with AI assistance disclosed.

### Consequences

As built in T-33, go.mod names github.com/donvargax/kong (branch itos, tag v1.16.2-itos.1) with no replace directive, so go install @latest keeps working; the PR branch typed-errors keeps alecthomas's module path, and the person reviews the PR before it goes upstream.
