---
status: accepted
date: 2026-10-07
---

# Each recorded outcome keeps the scope of the tests that decided it

## Context and Problem Statement

Snapshots do not record which tests decided an outcome (the file's own, --all-tests, or a --test-command), so mutation sample over --all-tests results reports false mismatches. (a, recommended) Record the scope per mutant outcome; mutation sample re-runs each mutant with its recorded scope by default, a flag overriding it. (b) Record it per file. (c) Leave it to the flags and drop the idea.

Asked as q-15, about mutation-sample-scope.

## Considered Options

The options are those the question names.

## Decision Outcome

(a) Record the scope per mutant outcome; sample re-runs each with its recorded scope by default.

### Consequences

Snapshots record per mutant whether its own tests, --all-tests or a --test-command decided it (own is left out of the file). A reused outcome keeps its scope, and mutation sample re-runs each mutant in its recorded scope unless a flag overrides it.
