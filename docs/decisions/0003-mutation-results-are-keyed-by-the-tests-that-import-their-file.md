---
status: accepted
date: 2026-10-07
---

# Mutation results are keyed by the tests that import their file

## Context and Problem Statement

Which tests' hash keys a mutation result? (a, recommended) The test files the graph finds importing the function's file (for Go, its package's test files and the test files of packages that import it), whatever command ran the mutants: one rule for every mode and language, cheap, as IDEAS.md planned. A kill made only by an end-to-end test that runs the binary (--all-tests) is not made stale when that test changes. (b) The test files of the command that ran: the package's for Go, vitest related's set, every test file with --all-tests. Exact, but under --all-tests any test edit makes every result stale, so the next nightly reruns everything.

Asked as q-4, about mutate-test-hash.

## Considered Options

The options are those the question names.

## Decision Outcome

(a): the test files the graph finds importing the function's file (for Go, its package's test files and the test files of packages that import it), whatever command ran.

### Consequences

A kill is reused, and a function is fresh, only while its own hash and the hashes of the test files that import its file are unchanged, whatever command ran the mutants. A kill made only by an end-to-end test that runs the binary does not go stale when that test changes; test-attribution is the idea that would close it.
