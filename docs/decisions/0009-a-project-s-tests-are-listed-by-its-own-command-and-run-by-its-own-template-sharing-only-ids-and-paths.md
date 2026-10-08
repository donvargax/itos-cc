---
status: accepted
date: 2026-10-07
---

# A project's tests are listed by its own command and run by its own template, sharing only IDs and paths

## Context and Problem Statement

test-attribution: how does itos-cc learn a project's tests and run a selection? (a, recommended) itos-cc.yaml names a list command printing one test per line, its ID and optionally the file defining it, and a run template with {ids} and how IDs join; itos and itos-cc share only IDs and paths. (b) itos-cc parses itos's own list JSON.

Asked as q-17, about test-attribution.

## Considered Options

The options are those the question names.

## Decision Outcome

(a) itos-cc.yaml commands: a list command printing one test per line (ID, optionally its file), and a run template with {ids} and the join; only IDs and paths are shared.

### Consequences

mutation.tests in itos-cc.yaml holds list (one test a line: its ID, then optionally a tab and the file defining it), run with {pattern}, ids_pattern, join, and optionally whole and support. itos-cc never parses a test framework's own output, so any harness that prints IDs can take part; a list command that fails is tests.list-failed.
