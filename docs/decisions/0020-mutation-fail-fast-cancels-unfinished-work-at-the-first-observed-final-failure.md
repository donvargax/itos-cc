---
status: accepted
date: 2026-10-08
---

# Mutation fail-fast cancels unfinished work at the first observed final failure

## Context and Problem Statement

Recommend --fail-fast stop on the earliest observed actionable FINAL judgment, not source-order determinism; an own-test survivor must finish applicable listed tests first. Publish one stop/admission latch preventing new command launches, cancel unfinished in-flight judgments immediately, and use a shared five-second post-stop cleanup deadline rather than draining baseline-derived timeouts. A parent-aborted mutant is cancelled/undecided, never timeout or killed. Known stale exceptions and strict coverage failures stop before avoidable later preparation. Keep selected planning/global coverage scope unchanged in this first design; do not promise streaming coverage or a total-run latency bound. Choose immediate cancellation with five-second cleanup, or bounded drain-before-cancel with its drain budget to be specified.

Asked as q-32, about mutation-fail-fast.

## Considered Options

- Immediate cancellation with a shared five-second cleanup deadline
- Drain within a separately defined budget before cancellation

## Decision Outcome

Choose immediate cancellation at the earliest observed actionable final judgment, with one shared five-second post-stop cleanup deadline. Finish applicable listed stages before declaring a survivor final, stop new command admissions, and never classify parent-aborted work as timeout or killed. Stop known exception/strict failures before avoidable later work. Keep selected planning/global coverage scope unchanged for now, without claiming a five-second total-runtime or streaming-preparation guarantee.

### Consequences

The first observed actionable final result closes command admission; parallel completion order, not source order, decides which result triggers the stop. An own-test survivor waits for applicable listed tests before it is final. Cancelled judgments have no invented kill/timeout outcome. Known failures avoid later preparation where possible, but global selected planning and coverage retain their current scope and can still take time. The cleanup bound starts at stop publication and is not a total-runtime guarantee. Aggregate scheduling remains the default.
