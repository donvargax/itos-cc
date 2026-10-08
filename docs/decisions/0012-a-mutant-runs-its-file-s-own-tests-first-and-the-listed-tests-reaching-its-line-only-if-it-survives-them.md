---
status: accepted
date: 2026-10-07
---

# A mutant runs its file's own tests first, and the listed tests reaching its line only if it survives them

## Context and Problem Statement

test-attribution: what does a mutant run once per-test coverage exists? (a, recommended) Its file's own tests first; only if it survives, the listed tests whose coverage reaches its line, as one selection run; uncovered only when neither covers its line. (b) Only the covering tests, with its own tests in the same pass.

Asked as q-20, about test-attribution.

## Considered Options

The options are those the question names.

## Decision Outcome

(a) Own tests first, then the covering listed tests only if it survives; uncovered only when neither covers its line.

### Consequences

The listed tests that reach the mutant's line run as one selection, with a timeout from that selection's own baseline run. A line a listed test reaches is never uncovered; an outcome they decide has scope listed and records their IDs.
