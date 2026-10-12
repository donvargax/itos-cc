---
status: accepted
date: 2026-10-11
---

# The template is adopted in stages

## Context and Problem Statement

How far does the adoption commit go? Recommend staged: an itos upgrade task (6.5.1 to 7.5.0), then an adoption task taking the template's docs, tools, CI and ADRs (renumbered after ours) while keeping itos-cc's main.go and CLI behaviour, the new gates wired only as far as they pass; then slices porting the code design (kong + internal/cli, exit 130 on interrupt, ports and slices, the godog harness), each new gate a task of its own. Alternative: all at once, main.go rewritten onto kong and internal/cli in the adoption.

Asked as q-49, about template-adoption.

## Considered Options

The options are those the question names.

## Decision Outcome

Staged, as recommended, 2026-10-11.

### Consequences

An itos upgrade task, then one adoption commit keeping itos-cc's main.go and CLI behaviour (T-25, T-26), then refactor slices porting the code design one command at a time, each new gate a task of its own.
