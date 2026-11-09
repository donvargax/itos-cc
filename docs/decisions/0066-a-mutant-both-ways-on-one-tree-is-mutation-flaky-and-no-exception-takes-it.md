---
status: accepted
date: 2026-11-09
---

# A mutant both ways on one tree is mutation.flaky, and no exception takes it

## Context and Problem Statement

flaky-kills: a mutant whose tests decide it differently with nothing changed (unseeded random tests, e.g. the rapid properties in internal/selection) turns cd.yml's sample red as mutation.mismatch, whose fix (--mutate-all) is the wrong one. Proposed first item (A): mutation sample re-runs a disagreeing mutant twice more; if a re-run agrees with the recording, it is flaky, else a mismatch as today; agreeing mutants cost nothing extra. Confirm the recommended set or change any: D1 flaky = both outcomes seen within one invocation on one tree (not across runs, which cannot tell a flake from a stale kill). D2 its own rule mutation.flaky (breaking: a rule value changes, minor release) rather than mutation.mismatch with a flaky key. D3 sample exits 1 on it. D4 two confirming runs, no flag. D5 timeout against survived counts as flaky too. D6 no exception for a flaky mutant: the fix is to make the tests deterministic. D7 later item B: run records an additive flaky key and check fails on it. D8 later item C: --repeat N. Also later: seed rapid here (with property-tests).

Asked as q-93, about slice-50.

## Considered Options

The options are those the question names.

## Decision Outcome

Person 2026-10-24: the recommended set D1 to D8 (first item: sample confirms a disagreement with two more runs; mutation.flaky, exit 1; timeout against survived counts; no exception; run/check recording and --repeat N are later items).

### Consequences

Flaky means both outcomes seen within one invocation on one tree, as across runs a flake and a stale kill look alike. It is its own rule, exit 1, and timeout against survived counts. The fix is deterministic tests, never an entry in itos-cc.yaml.
