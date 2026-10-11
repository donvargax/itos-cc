---
status: accepted
date: 2026-10-11
---

# Tests reach files through the full transitive import closure in TypeScript, Python and Kotlin

## Context and Problem Statement

Language parity: a Go test reaches its own package and the packages it imports (one hop); TypeScript, Python and Kotlin tests reach only their direct imports, while vitest related and jest --findRelatedTests follow imports transitively, so a test can kill a mutant of a file it reaches indirectly without being among that file's recorded tests. Recommend the full transitive import closure for TypeScript, Python and Kotlin, matching what their runners select; alternative: one hop, matching Go.

Asked as q-39.

## Considered Options

- Full transitive import closure, matching the runners' selection
- One hop, matching Go

## Decision Outcome

Transitive, 2026-10-10: the full transitive import closure for TypeScript, Python and Kotlin, matching what their runners select.

### Consequences

A test that kills a mutant of a file it reaches indirectly is among that file's recorded tests, and its edits stale those outcomes.
