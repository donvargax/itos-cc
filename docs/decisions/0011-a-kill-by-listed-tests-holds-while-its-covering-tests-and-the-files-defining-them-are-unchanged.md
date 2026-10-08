---
status: accepted
date: 2026-10-07
---

# A kill by listed tests holds while its covering tests and the files defining them are unchanged

## Context and Problem Statement

test-attribution: when is a kill made by listed tests still fresh, without itos-cc reading the tests' format? (a, recommended) While the set of tests covering its line and the hashes of the files those tests name are unchanged (per-file granularity). (b) The list command prints a content hash per test, computed by the project.

Asked as q-19, about test-attribution.

## Considered Options

The options are those the question names.

## Decision Outcome

(a) The covering tests' set and the hashes of the files they name.

### Consequences

The snapshot records the hash of the file each covering test is defined in; a change to one makes the function stale, and no list command runs to judge it, since deleting a test changes its file. Freshness is per file: an edit to any test in a file stales every kill resting on it.
