---
status: accepted
date: 2026-10-11
---

# Every feature file converts to godog, one command at a time

## Context and Problem Statement

The template runs scenarios with godog against the built binary; ours are prose checked by in-process Go tests: hybrid, convert everything over time, or stay with Go tests?

Asked as q-52.

## Considered Options

The options are those the question names.

## Decision Outcome

Convert every feature file to godog over time (answered 2026-10-11), not a hybrid: the harness arrives with the first command moved onto the template design, and each stage that moves a command converts its feature file, mutate.feature last with the mutation group.

### Consequences

No lasting hybrid: the godog harness runs the built binary, each slice moving a command onto the template design converts that command's feature file, mutate.feature last with the mutation group; until then the remaining files stay on the in-process Go steps.
