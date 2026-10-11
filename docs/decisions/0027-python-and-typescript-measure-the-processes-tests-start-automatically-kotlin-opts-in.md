---
status: accepted
date: 2026-10-11
---

# Python and TypeScript measure the processes tests start automatically; Kotlin opts in

## Context and Problem Statement

q-41 follow-up: should Python and TypeScript measure the processes a test starts automatically, or only when a project opts in (Go needs its harness to build with -cover)? Recommend automatic: those lines really are run by tests; call it out in the release notes, since mutants there now run and may survive. Kotlin stays opt-in through ITOS_CC_JACOCO_AGENT.

Asked as q-44.

## Considered Options

- Automatic, with a release-note callout
- Only when the project opts in, as Go builds with -cover

## Decision Outcome

Automatic, with a release-note callout, 2026-10-11.

### Consequences

Lines only a subprocess runs stop being uncovered, so their mutants run and may survive; runs without --fail-uncovered can newly exit 1, and runs take longer. Kotlin opts in through ITOS_CC_JACOCO_AGENT.
