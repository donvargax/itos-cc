---
status: accepted
date: 2026-10-07
---

# The hashes of the files mutation.tests.support matches join every listed kill's freshness

## Context and Problem Statement

Step code: itos's scenario checks live in Go step code (features/*_test.go), while q-19 hashes only the files that define the covering tests, so weakening a step leaves old end-to-end kills trusted. (a, recommended) mutation.tests gains support globs (such as features/*_test.go); their hashes join every listed kill's freshness, so a step-code edit stales every listed kill, each rerun running only its covering tests. (b) Only catch it by sampling listed kills nightly. (c) Accept the gap.

Asked as q-21, about listed-tests-freshness.

## Considered Options

The options are those the question names.

## Decision Outcome

(a) A support glob list in mutation.tests whose hashes join every listed kill's freshness. Found by a side agent after q-19 was answered.

### Consequences

A change to step code or any other support file stales every listed kill, and each rerun runs only its covering tests. Weakening a step can no longer leave old end-to-end kills trusted.
