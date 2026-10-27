---
status: accepted
date: 2026-10-27
---

# Exceptions and extension keys are not coverage inputs

## Context and Problem Statement

Should writing an exception stop invalidating the coverage caches and strict Go evidence? Recommended (a): the itos-cc.yaml fingerprint used for coverage inputs leaves out mutation.exceptions and x- keys everywhere it is used (both caches and strict evidence), recorded strict evidence reading stale once after the upgrade, said in the upgrade note; (b) only the caches; (c) keep the whole file.

Asked as q-105, about exceptions-out-of-coverage-key.

## Considered Options

- Leave mutation.exceptions and x- keys out of itos-cc.yaml's coverage fingerprint everywhere (chosen)
- Only the caches use the narrower fingerprint
- Keep the whole file

## Decision Outcome

Person 2026-10-27: (a). The itos-cc.yaml fingerprint used for coverage inputs leaves out mutation.exceptions and x- keys wherever it is used, both coverage caches and strict Go evidence, since neither changes what coverage measures; recorded strict evidence reads stale once after the upgrade, which the release's upgrade note says.

### Consequences

mutation except no longer invalidates the coverage caches (slices 69, 70) or strict Go evidence; every other itos-cc.yaml key (exclude, mutation.tests, mutation.copy, version) still does. Recorded strict evidence reads stale once after the upgrade.
