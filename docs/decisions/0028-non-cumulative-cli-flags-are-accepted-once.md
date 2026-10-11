---
status: accepted
date: 2026-10-09
---

# Non-cumulative CLI flags are accepted once

## Context and Problem Statement

For the default Go CLI, should each non-cumulative flag be accepted only once? Recommended: adopt the existing itos-template contract, reject --json --no-json and other repeated once-only flags with usage exit 2, and preserve explicitly cumulative flags as repeatable. Or: retain the current count CLI contract where the last negation wins. This changes ID-COUNT-06, so the selected behavior will be specified before implementation.

Asked as q-1, about cli-contract.

## Considered Options

The options are those the question names.

## Decision Outcome

Reject repeated non-cumulative flags with usage exit 2, matching the existing itos-template contract. Explicitly cumulative flags remain repeatable. Specify the replacement for ID-COUNT-06 before implementation; commit the failing steps first, and document the behavior change for generated CLI users.

### Consequences

None recorded.
