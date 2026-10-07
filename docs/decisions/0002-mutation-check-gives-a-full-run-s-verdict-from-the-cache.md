---
status: accepted
date: 2026-10-07
---

# mutation check gives a full run's verdict from the cache

## Context and Problem Statement

What does the check mode fail on? (a, recommended) Missing or stale results, and also cached survivors (and cached uncovered mutants with --fail-uncovered) in the selected functions, so it gives the verdict a full run would give, from the cache: a gate that passes on fresh results showing survivors proves nothing. (b) Missing or stale results only, as issue #8 sketches it, leaving survivors to the run that wrote them.

Asked as q-2, about mutate-check-sample.

## Considered Options

The options are those the question names.

## Decision Outcome

(a): missing or stale results, and cached survivors (and cached uncovered mutants with --fail-uncovered) in the selected functions, so it gives the verdict a full run would give, from the cache.

### Consequences

A gate can rely on mutation check alone: it fails on missing or stale results, on recorded survivors, and on recorded uncovered mutants with --fail-uncovered. Every later freshness rule (tests' hashes, unrecorded sites, stale marks, exceptions) has to be part of that verdict.
