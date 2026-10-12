---
status: accepted
date: 2026-10-11
---

# Changed-code mutation proof stays off until the refactor ends

## Context and Problem Statement

When does changed-code mutation proof (proof.code) turn on in CI, given code a refactor moves has no mutation results until the nightly run?

Asked as q-53.

## Considered Options

The options are those the question names.

## Decision Outcome

Keep it off until the refactor is done, after its last stage (answered 2026-10-11): the refactor relies on tests and scenarios, and the nightly run refreshes .metrics/mutate. p1-itos-proof-code waits for it.

### Consequences

proof.code and p1-itos-proof-code wait for the refactor's last stage; meanwhile the refactor rests on tests and scenarios, and the nightly run refreshes .metrics/mutate for moved code.
