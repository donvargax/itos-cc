---
status: accepted
date: 2026-10-11
---

# Own tests of Python, Kotlin and TypeScript-fallback files are the tests that import them

## Context and Problem Statement

Language parity: for Python, Kotlin and the TypeScript fallback without Vitest or Jest, a file's "own tests" are the whole suite (pytest -q -x, gradle test, <pm> run test), yet those outcomes are recorded as scope own, whose freshness only follows importing tests; a kill made by a non-importing test is reused after that test changes. Recommend (a): narrow the own command and its coverage to the tests that import the file, as Go uses its package and TypeScript its related tests (pytest -x <files>, python -m unittest <modules>, gradle test --tests <classes>, mvn -Dtest=<classes>), which makes runs faster and kills honest. Alternative (b): keep whole-suite commands and record these outcomes with whole-suite evidence, so any test edit stales every Python and Kotlin outcome.

Asked as q-38.

## Considered Options

- Narrow the own command and its coverage to the importing tests
- Keep whole-suite commands with whole-suite evidence

## Decision Outcome

(a), 2026-10-10: narrow the own command and its coverage to the tests that import the file (pytest -x <files>, python -m unittest <modules>, gradle test --tests <classes>, mvn -Dtest=<classes>), as Go uses its package and TypeScript its related tests.

### Consequences

Runs are faster and own kills honest; tests that reach a module only through a subprocess or CLI no longer count among its own tests, so some kills became survivors or uncovered (v0.10.0 release note).
