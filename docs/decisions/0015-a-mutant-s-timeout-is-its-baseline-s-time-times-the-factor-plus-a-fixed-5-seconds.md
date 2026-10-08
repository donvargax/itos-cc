---
status: accepted
date: 2026-10-07
---

# A mutant's timeout is its baseline's time times the factor plus a fixed 5 seconds

## Context and Problem Statement

timeout-allowance: a mutant's timeout is its baseline's time times the factor, at least 2s, and on slow runners building the mutant can exceed it, so a survivor times out and counts as killed. (a, recommended) baseline times factor plus a fixed 5s, replacing the 2s minimum, as PIT and Stryker add a constant. (b) Rerun a timed-out mutant once with twice the timeout. (c) Give the flaky tests room and leave the product.

Asked as q-23, about timeout-allowance.

## Considered Options

The options are those the question names.

## Decision Outcome

(a) Baseline times the factor plus a fixed 5 seconds, replacing the 2-second minimum, for every kind of run.

### Consequences

The 2-second minimum goes. Building a mutated program no longer counts against a timeout sized from a baseline that may have found it built, so a survivor on a slow machine is not counted killed as a timeout; a hanging mutant takes about 5 seconds longer to call. It holds for a file's tests, --test-command and --all-tests runs, each selection of listed tests and mutation sample.
