---
status: accepted
date: 2026-11-09
---

# A red nightly means the run failed, not that survivors remain

## Context and Problem Statement

Nightly has passed in none of its last 29 runs. The run of 2026-11-07 ended with exit 1 straight after listing survivors, which appears to be why it is red, so real failures such as the 504 of 2026-11-08 look the same. Should survivors alone leave the nightly red? (a, recommended) no: results are already recorded, so red means the mutation run itself failed, a task keeping every other failure red. (b) yes, leave it, red meaning survivors remain.

Asked as q-108.

## Considered Options

The options are those the question names.

## Decision Outcome

(a) No: survivors alone do not leave the nightly red, as the results are recorded; red means the mutation run itself or another step failed. A task, keeping every other failure red.

### Consequences

T-229 runs mutation run through tools/bin/nightly-survivors, which passes only an exit 1 whose problems are all mutation.survived, warning for each. Survivors stay recorded in .metrics/mutate/. Any other exit, and any other step, keeps the nightly red.
