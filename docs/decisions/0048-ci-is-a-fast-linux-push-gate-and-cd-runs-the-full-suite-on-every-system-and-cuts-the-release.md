---
status: accepted
date: 2026-10-17
---

# CI is a fast Linux push gate and CD runs the full suite on every system and cuts the release

## Context and Problem Statement

How should CI be split so pushes stop waiting on the whole suite? (a, recommended) Two workflows on the same push, in parallel: ci.yml, the push gate itos push and itos work done wait on, on ubuntu-latest alone (the ci job, every Go unit test, the whole in-process suite, the binary smoke set and the scenarios the range names); cd.yml, every test and scenario on all three systems, then the release, the only workflow that cuts one. (b) cd.yml called from ci.yml (needs or a reusable workflow): ci.yml's run stays open until CD finishes, so pushes still wait. (c) cd.yml chained by workflow_run after ci.yml: it starts only after CI, in the default branch's context, GITHUB_SHA not the pushed commit.

Asked as q-65, about T-127.

## Considered Options

- Two workflows on the same push, in parallel: ci.yml (Linux, fast) and cd.yml (all systems, full suite, release)
- cd.yml called from ci.yml, which keeps ci.yml's run open until CD finishes
- cd.yml chained by workflow_run after ci.yml, in the default branch's context

## Decision Outcome

(a), the person on 2026-10-17: CI stays as fast as possible while showing the pushed work is mostly right; Windows, macOS, edge cases and the whole binary suite are CD's, an accepted trade-off, and CD gates every release. The coordinator watches each cd.yml run and hands a red one to a fresh agent for a targeted repair (itos#84 asks itos to watch it; until then gh run watch).

### Consequences

Pushes, itos work done and implementing agents wait only on ci.yml. A green ci.yml no longer means every test passed: a cd.yml failure can land after an item closed, so the coordinator watches cd.yml and treats red as repair work ahead of new items. Only cd.yml runs Windows and macOS jobs, at 5 macOS jobs or fewer at once. A release waits for the full suite on every system, as before. This adds a tier between push CI and the nightly of ADR-0051's tiers in itos: the full suite after every push. T-127 builds it; T-128 shortens cd.yml.

## More Information

Amended by ADR-0052: the tiers stand, and every workflow sets ITOS_CC_FEATURES_BINARY_DRIVER so the binary driver keeps running in ci.yml and cd.yml, while local runs hold the scenarios in process alone.
