---
status: accepted
date: 2026-10-08
---

# Fail-fast owns ordinary test process trees with explicit Unix containment limits

## Context and Problem Statement

Issue #26 needs owned-descendant cleanup, but current Windows cancellation kills only the parent and Unix groups cannot contain intentionally daemonized/session-escaping children. Recommend a separately verified process-supervision prerequisite: race-safe Windows Job Object ownership, Unix process-group ownership and cleanup on cancellation/command completion, joining all owned worker/output waiters. Apply supervision to the new fail-fast mode’s mutation/baseline/selection and list/coverage commands; never kill unrelated processes or by executable-name matching. Explicitly declare intentionally detaching/session-escaping Unix commands unsupported; do not claim container-strength isolation. Cleanup/start/ownership failures must be reported, never silently downgraded or recorded as mutant kills. Alternative: require stronger OS/container isolation to contain arbitrary escaping descendants before this feature can ship. Choose ordinary non-detaching owned process trees with that explicit boundary, or stronger isolation first.

Asked as q-33, about mutation-fail-fast.

## Considered Options

- Windows Job Objects and Unix process groups for non-detaching commands
- Require stronger OS or container isolation for arbitrary detached descendants

## Decision Outcome

Choose supervised ordinary owned process trees: race-safe Windows Job Objects and Unix process groups, with cleanup on cancellation and command completion and joins of owned worker/output waiters. Intentionally daemonizing/session-escaping Unix commands are explicitly unsupported; this is not container-strength containment. Supervise the fail-fast mutation/baseline/selection/list/coverage paths, report startup/ownership/cleanup failures, never silently downgrade containment or turn them into mutant kills, and never target unrelated processes or match by executable name. Verify the supervision prerequisite separately before exposing --fail-fast.

### Consequences

Process supervision is a separately verified prerequisite, not an afterthought to the scheduling flag. Ownership must be established before a Windows command can spawn escaping children, and cleanup must occur on cancellation and normal command completion. Waiters and worker copies remain owned until cleanup is joined. Unix commands that deliberately daemonize or escape their session/process group are unsupported; no full-sandbox guarantee is made. Ownership or cleanup failures are explicit failures, never an unsupervised fallback or a successful mutant kill. Only captured owned process handles/groups may be targeted; unrelated orphaned processes are out of scope. Preparation commands used by fail-fast need the same ownership discipline.
