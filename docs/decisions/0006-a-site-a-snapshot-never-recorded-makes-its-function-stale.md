---
status: accepted
date: 2026-10-07
---

# A site a snapshot never recorded makes its function stale

## Context and Problem Statement

mutation check passes a function whose entry lacks sites a newer operator adds. What does check say? (a, recommended) The function is stale: its results predate the operator; check reports mutation.stale naming the unrecorded sites, and mutation run runs only those sites. (b) The function is missing, and every mutant runs again.

Asked as q-13, about mutation-check-unrecorded-sites.

## Considered Options

The options are those the question names.

## Decision Outcome

(a) The function is stale, naming the unrecorded sites; mutation run runs only those sites.

### Consequences

Adding a mutation operator never lets mutation check pass unjudged sites: the function is stale, the problem names the sites, and mutation run runs only those. The graph follows through mutate.FreshnessOf.
