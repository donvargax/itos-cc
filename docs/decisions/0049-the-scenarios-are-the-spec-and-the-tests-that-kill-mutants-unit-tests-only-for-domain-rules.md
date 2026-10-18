---
status: accepted
date: 2026-10-17
---

# The scenarios are the spec and the tests that kill mutants; unit tests only for domain rules

## Context and Problem Statement

How do itos-cc's kinds of test divide the work, and which tests should kill mutants? (a, recommended) The feature files are the spec, exercised as end-to-end tests through two drivers: in memory for fast feedback, through the built binary in CD for the real adapters. Unit tests only for domain rules, where a rule has many cases a feature file would bloat. Port contracts hold every fake to its real adapter, so a green in-memory run means something. Mutants are killed by the in-memory scenarios that reach their line, as listed tests, never by unit tests written for mutation's sake. A survivor is a spec gap (add a scenario) or unobservable code (remove it, or record an exception). (b) Keep Go's package-and-one-hop own tests as the killing tests and add unit tests where mutants survive.

Asked as q-66, about inprocess-mutation-tests.

## Considered Options

- Scenarios as the spec and the killing tests, through the in-memory driver as listed tests; unit tests for domain rules only; port contracts for every fake
- Go's package-and-one-hop own tests kill mutants, with unit tests added where mutants survive

## Decision Outcome

(a), the person on 2026-10-17: mainly end-to-end tests that give fast feedback through the in-memory build, so they run like unit tests; no feature file targets another layer; unit tests are for the domain's business rules, not for mutation testing's sake; the end-to-end scenarios are what prove product quality.

### Consequences

Mutation testing measures the spec: a survivor is a missing scenario or unobservable code, never a missing unit test. itos-cc's Go reach (package plus one hop) does not know which scenarios reach a file, so the idea inprocess-mutation-tests lists the in-memory scenarios as mutation.tests, needs a mode that runs listed tests first or alone, and a list form mutation.tests can read (itos#22); itos-cc#30 makes a listed kill's freshness precise. That makes each mutant cost seconds, which enables a pre-push mutation check (idea prepush-mutation-check) and a nightly without --all-tests (T-46). Port contracts are essential: without them the fakes drift and fast in-memory runs prove nothing; the binary driver in CD is the second net. Extends ADR-0044 and docs/engineering.md's rule that behaviour reachable through acceptance tests is held there. go-template-itos#7 asks the template for the same two drivers and contract suites: revisit it as a backport after the refactor.
