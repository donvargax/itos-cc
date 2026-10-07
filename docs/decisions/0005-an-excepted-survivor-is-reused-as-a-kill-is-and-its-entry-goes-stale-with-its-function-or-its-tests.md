---
status: accepted
date: 2026-10-07
---

# An excepted survivor is reused as a kill is, and its entry goes stale with its function or its tests

## Context and Problem Statement

What does an excepted survivor do, and when is its entry stale? (a, recommended) It fails nothing, counted excepted, and is reused without running, as a kill is, while its function and the tests that import its file are unchanged; when its tests change it runs again, and if now killed its entry is stale. A changed function or a site gone makes the entry stale without running. Stale fails as mutation.exception-stale, exit 1; --mutate-all runs them too. (b) Always run them. (c) Never run them.

Asked as q-11, about mutation-exceptions.

## Considered Options

The options are those the question names.

## Decision Outcome

(a) Reused as a kill is while its function and its tests are unchanged; rerun when its tests change, stale if now killed; stale without running when its function changed or its site is gone. The person wanted to save time and still catch stale exceptions.

### Consequences

Exceptions cost no run while nothing they depend on changes. A changed function or a site that is gone makes the entry stale without running; a change to its tests reruns the mutant, and a kill makes the entry stale. A stale entry fails as mutation.exception-stale until it is removed or the site is excepted again.
