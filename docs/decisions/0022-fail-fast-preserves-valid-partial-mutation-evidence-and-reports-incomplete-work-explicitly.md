---
status: accepted
date: 2026-10-08
---

# Fail-fast preserves valid partial mutation evidence and reports incomplete work explicitly

## Context and Problem Statement

Recommend fail-fast preserve every valid completed/reused judgment and its original scope/freshness evidence, including earlier judgments in a file whose later selection/baseline fails; this is an explicit new-mode exception to default whole-file preservation, and aggregate defaults stay unchanged. Cancelled/unattempted/blocked sites have no fabricated mutation outcome and cannot newly satisfy completeness; check validates recognized outcomes plus current-site enumeration. Genuinely fresh prior cache is not invalidated solely because a forced --mutate-all rerun was cancelled; current-run incompleteness and prior-cache completeness are separate facts. List all selected files in machine output, add stopped/complete/stop reason and disjoint completed/cancelled/unattempted/blocked work counts, plus actual baseline stage status without treating not-run as executed success. Keep mutation counts and judged selection identity meanings; leave source annotations unchanged for incomplete files. Choose preserving valid partial progress and explicit machine states, or discard progress from any interrupted/failed file while reporting it incomplete.

Asked as q-34, about mutation-fail-fast.

## Considered Options

- Preserve valid completed work with truthful partial machine states
- Discard new judgments from every interrupted or failed file

## Decision Outcome

Preserve valid completed/reused judgments and their original scope/freshness evidence in fail-fast mode, even when later work in the file fails. Aggregate behavior keeps existing snapshot rules. Cancelled, blocked and unattempted sites have no mutation outcome and cannot create completeness; check recognizes valid outcomes and enumerates current sites. Fresh prior evidence remains usable after a cancelled forced rerun, separately from the current run being incomplete. Include all selected files and explicit stopped/complete/stop-subject plus disjoint completed/cancelled/unattempted/blocked states and actual baseline stages. Preserve mutation count/judged meanings and leave annotations unchanged for incomplete files.

### Consequences

In fail-fast mode a later failure does not erase already-valid judgments, even in the same file. Pending/cancelled/blocked sites never receive invented mutation outcomes or new freshness proof; missing valid site entries prevent newly incomplete functions from passing check. Previously fresh cache is not made invalid just because a forced rerun stopped, so current-run completeness and cache completeness must be reported separately. Selected-file output distinguishes actual stage/work status, and completed includes trusted reuse and measured coverage judgments, not only test executions. Baseline not-run/cancelled must not be described as executed success. Aggregate defaults retain their established contracts, and incomplete source annotations remain untouched.
