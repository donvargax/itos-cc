---
status: accepted
date: 2026-10-12
---

# Coverage splits into a pure domain and a port whose adapter runs the test tools

## Context and Problem Statement

Coverage in the refactored design (S5): coverage/ (3.8k lines) both runs test tools (go test -cover, Vitest, pytest-cov, Gradle and JaCoCo; subprocesses and run folders) and holds the pure Report that answers which lines are covered, a function's fraction and which tests reach a line; crap and mutation both use it. Recommend splitting it as lang was: the pure Report, its queries and the format parsers become an internal/coverage domain; a port declared there measures coverage for a request and gives a Report or a sealed failure (tool missing, measured nothing, no report, report unreadable), mapped once by cmd for crap and mutation; the adapter wraps the existing runner and its process groups; the port gets a contract suite and crap's scenarios run in process against its fake (decision 44). Alternative: a coverage port declared by crap's domain now and another for mutation when it moves, which keeps the rules but duplicates the model and the failure mapping.

Asked as q-62, about template-adoption.

## Considered Options

- A pure internal/coverage domain with a measuring port shared by crap and mutation
- A coverage port declared by each command's domain as it moves

## Decision Outcome

As recommended (the person, 2026-10-13): the split is the correct solution.

### Consequences

internal/coverage holds Report, its queries and the format parsers and reads nothing; its port measures coverage for a request (ctx first) and gives a Report or a sealed failure that cmd maps once to problems for crap and mutation; the adapter wraps the runner, its subprocesses and run folders and writes progress only to the writer cmd hands it; the port has a contract suite, and crap's scenarios run in process against its fake. Mutation keeps the runner library until it moves, as snapshots did (T-38).
